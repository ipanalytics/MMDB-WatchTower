package swap

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"mmdb-watchtower/internal/config"
	"mmdb-watchtower/internal/state"
)

type Result struct {
	Backup *state.VersionEntry
}

func Atomic(db config.Database, newPath string, sha string, buildEpoch uint) (Result, error) {
	if err := os.MkdirAll(filepath.Dir(db.Path), 0755); err != nil {
		return Result{}, err
	}
	var backup *state.VersionEntry
	if _, err := os.Stat(db.Path); err == nil {
		dir := state.VersionDir(db.Path, db.Name)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return Result{}, err
		}
		dst := filepath.Join(dir, fmt.Sprintf("%s.mmdb", time.Now().UTC().Format("20060102T150405Z")))
		if err := os.Rename(db.Path, dst); err != nil {
			return Result{}, err
		}
		backup = &state.VersionEntry{Path: dst, SHA256: sha, BuildEpoch: buildEpoch, CreatedAt: time.Now().UTC()}
	}
	if err := os.Rename(newPath, db.Path); err != nil {
		if backup != nil {
			_ = os.Rename(backup.Path, db.Path)
		}
		return Result{}, err
	}
	return Result{Backup: backup}, nil
}

func Restore(db config.Database, version state.VersionEntry) error {
	tmp := db.Path + ".rollback"
	if err := copyFile(version.Path, tmp); err != nil {
		return err
	}
	return os.Rename(tmp, db.Path)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}
