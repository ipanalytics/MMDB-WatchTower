package runner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"mmdb-watchtower/internal/config"
	"mmdb-watchtower/internal/download"
	"mmdb-watchtower/internal/metrics"
	"mmdb-watchtower/internal/mmdbcheck"
	"mmdb-watchtower/internal/state"
	"mmdb-watchtower/internal/swap"
	"mmdb-watchtower/internal/verify"
)

type Runner struct {
	cfg     config.Config
	metrics *metrics.Metrics
}

type UpdateResult struct {
	Name        string `json:"name"`
	Channel     string `json:"channel,omitempty"`
	Updated     bool   `json:"updated"`
	NotModified bool   `json:"not_modified,omitempty"`
	Path        string `json:"path"`
	SHA256      string `json:"sha256,omitempty"`
	ETag        string `json:"etag,omitempty"`
	BuildEpoch  uint   `json:"build_epoch,omitempty"`
	Error       string `json:"error,omitempty"`
}

type RollbackResult struct {
	Name       string `json:"name"`
	RolledBack bool   `json:"rolled_back"`
	From       string `json:"from,omitempty"`
	To         string `json:"to,omitempty"`
	Path       string `json:"path"`
}

func New(cfg config.Config, m *metrics.Metrics) *Runner {
	return &Runner{cfg: cfg, metrics: m}
}

func (r *Runner) RunOnce(ctx context.Context) {
	for _, db := range r.cfg.Databases {
		r.Update(ctx, db)
	}
}

func (r *Runner) Update(ctx context.Context, db config.Database) UpdateResult {
	return r.UpdateChannel(ctx, db, "")
}

func (r *Runner) UpdateChannel(ctx context.Context, raw config.Database, channel string) UpdateResult {
	db, err := config.ResolveDatabase(raw, channel)
	if err != nil {
		return UpdateResult{Name: raw.Name, Channel: channel, Path: raw.Path, Error: err.Error()}
	}
	now := time.Now().UTC()
	st, _ := state.LoadForDB(db)
	current := st.DatabaseState(db.Name)
	current.Name = db.Name
	current.Path = db.Path
	current.Channel = db.Channel
	current.LastUpdateAt = now

	fail := func(reason string, err error) UpdateResult {
		current.Status = "update_failed"
		current.LastError = err.Error()
		st.PutDatabase(current)
		_ = st.Save()
		if r.metrics != nil {
			r.metrics.Failure(db.Name, reason)
		}
		return UpdateResult{Name: db.Name, Channel: db.Channel, Path: db.Path, Error: err.Error()}
	}

	tlsConfig := download.TLS{
		ClientCert: db.TLS.ClientCert,
		ClientKey:  db.TLS.ClientKey,
		CACert:     db.TLS.CACert,
		ServerName: db.TLS.ServerName,
	}
	dl, err := download.File(ctx, download.Request{
		URL:         db.URL,
		Headers:     db.Headers,
		Dir:         os.TempDir(),
		IfNoneMatch: current.ETag,
		TLS:         tlsConfig,
	})
	if err != nil {
		return fail("download_failed", err)
	}
	if dl.NotModified {
		current.Status = "healthy"
		current.LastError = ""
		current.ETag = dl.ETag
		st.PutDatabase(current)
		_ = st.Save()
		return UpdateResult{Name: db.Name, Channel: db.Channel, NotModified: true, Path: db.Path, ETag: dl.ETag}
	}
	tmp := dl.Path
	defer os.Remove(tmp)
	if err := verify.Size(tmp, db.MinSizeMB, db.MaxSizeMB); err != nil {
		return fail("verify_failed", err)
	}
	sha := ""
	if db.SHA256URL != "" {
		expected, err := download.Text(ctx, db.SHA256URL, db.Headers, tlsConfig)
		if err != nil {
			return fail("sha256_failed", err)
		}
		sha, err = verify.SHA256Matches(tmp, expected)
		if err != nil {
			return fail("sha256_failed", err)
		}
	} else {
		sha, err = verify.SHA256(tmp)
		if err != nil {
			return fail("sha256_failed", err)
		}
	}
	if db.SignatureURL != "" {
		signature, err := download.Bytes(ctx, db.SignatureURL, db.Headers, tlsConfig)
		if err != nil {
			return fail("signature_failed", err)
		}
		if err := verify.CosignBlob(ctx, tmp, db.CosignKey, signature); err != nil {
			return fail("signature_failed", err)
		}
	}
	info, err := mmdbcheck.Inspect(tmp)
	if err != nil {
		return fail("mmdb_open_failed", err)
	}
	if err := mmdbcheck.Smoke(tmp, db.RequiredFields, db.SmokeTests); err != nil {
		return fail("smoke_failed", err)
	}
	swapRes, err := swap.Atomic(db, tmp, current.SHA256, current.BuildEpoch)
	if err != nil {
		return fail("swap_failed", err)
	}
	if err := r.reload(ctx); err != nil {
		if swapRes.Backup != nil {
			_ = swap.Restore(db, *swapRes.Backup)
		}
		return fail("reload_failed", err)
	}
	if swapRes.Backup != nil {
		swapRes.Backup.Version = current.Version
		st.AddVersion(db.Name, *swapRes.Backup, r.cfg.Retention.KeepVersions)
	}
	current.Status = "healthy"
	current.Version = versionFromBuild(info.BuildEpoch)
	current.DatabaseType = info.DatabaseType
	current.BuildEpoch = info.BuildEpoch
	current.SHA256 = sha
	current.ETag = dl.ETag
	current.LastSuccessAt = time.Now().UTC()
	current.LastError = ""
	current.PreviousVersions = len(st.Versions(db.Name))
	st.PutDatabase(current)
	_ = st.Save()
	r.observeDB(db, info.BuildEpoch)
	if r.metrics != nil {
		r.metrics.Success(db.Name)
	}
	return UpdateResult{Name: db.Name, Channel: db.Channel, Updated: true, Path: db.Path, SHA256: sha, ETag: dl.ETag, BuildEpoch: info.BuildEpoch}
}

func (r *Runner) Rollback(name string) (RollbackResult, error) {
	for _, db := range r.cfg.Databases {
		if db.Name != name {
			continue
		}
		st, err := state.LoadForDB(db)
		if err != nil {
			return RollbackResult{}, err
		}
		versions := st.Versions(name)
		if len(versions) == 0 {
			return RollbackResult{}, fmt.Errorf("no previous versions for %s", name)
		}
		current := st.DatabaseState(name)
		from := current.Version
		if err := swap.Restore(db, versions[0]); err != nil {
			return RollbackResult{}, err
		}
		current.Name = db.Name
		current.Path = db.Path
		current.Status = "healthy"
		current.SHA256 = versions[0].SHA256
		current.BuildEpoch = versions[0].BuildEpoch
		current.Version = versions[0].Version
		current.LastSuccessAt = time.Now().UTC()
		current.LastError = ""
		st.PutDatabase(current)
		_ = st.Save()
		if r.metrics != nil {
			r.metrics.Rollback(name)
		}
		return RollbackResult{Name: name, RolledBack: true, From: from, To: versions[0].Version, Path: db.Path}, nil
	}
	return RollbackResult{}, fmt.Errorf("unknown database: %s", name)
}

func (r *Runner) reload(ctx context.Context) error {
	if r.cfg.Reload.Command == "" {
		return nil
	}
	timeout := r.cfg.Reload.Timeout.Duration
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	reloadCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var cmd *exec.Cmd
	if len(r.cfg.Reload.Args) > 0 {
		cmd = exec.CommandContext(reloadCtx, r.cfg.Reload.Command, r.cfg.Reload.Args...)
	} else {
		cmd = exec.CommandContext(reloadCtx, "sh", "-c", r.cfg.Reload.Command)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("reload failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (r *Runner) observeDB(db config.Database, buildEpoch uint) {
	if r.metrics == nil {
		return
	}
	st, err := os.Stat(db.Path)
	if err != nil {
		return
	}
	age := time.Since(time.Unix(int64(buildEpoch), 0)).Seconds()
	r.metrics.SetDatabase(db.Name, age, float64(st.Size()), float64(buildEpoch))
}

func versionFromBuild(epoch uint) string {
	if epoch == 0 {
		return ""
	}
	return time.Unix(int64(epoch), 0).UTC().Format("2006.01.02")
}
