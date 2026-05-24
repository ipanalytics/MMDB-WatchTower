package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"mmdb-watchtower/internal/config"
	"mmdb-watchtower/internal/health"
	"mmdb-watchtower/internal/metrics"
	"mmdb-watchtower/internal/mmdbcheck"
	"mmdb-watchtower/internal/runner"
	"mmdb-watchtower/internal/state"
	"mmdb-watchtower/internal/version"
)

const defaultConfig = "/etc/mmdbwatch.yaml"

func main() {
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	var cfgPath string
	var channel string
	cmd := &cobra.Command{
		Use:     "mmdbwatch",
		Short:   "Production-safe updater for MaxMind DB files",
		Version: fmt.Sprintf("%s commit=%s date=%s", version.Version, version.Commit, version.Date),
	}
	cmd.PersistentFlags().StringVarP(&cfgPath, "config", "c", defaultConfig, "config file path")
	cmd.PersistentFlags().StringVar(&channel, "channel", "", "update channel override: stable, canary, latest")

	cmd.AddCommand(initCmd(&cfgPath))
	cmd.AddCommand(updateCmd(&cfgPath, &channel))
	cmd.AddCommand(statusCmd(&cfgPath, &channel))
	cmd.AddCommand(rollbackCmd(&cfgPath))
	cmd.AddCommand(verifyCmd())
	cmd.AddCommand(runCmd(&cfgPath, &channel))
	return cmd
}

func initCmd(cfgPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Write an example mmdbwatch.yaml",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := configPathForInit(*cfgPath)
			if _, err := os.Stat(path); err == nil {
				return fmt.Errorf("%s already exists", path)
			}
			return os.WriteFile(path, []byte(config.ExampleYAML), 0644)
		},
	}
}

func updateCmd(cfgPath *string, channel *string) *cobra.Command {
	return &cobra.Command{
		Use:   "update [database]",
		Short: "Run one safe update cycle",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(*cfgPath)
			if err != nil {
				return err
			}
			m := metrics.New()
			r := runner.New(cfg, m)
			ctx := cmd.Context()
			results := make([]runner.UpdateResult, 0, len(cfg.Databases))
			for _, db := range cfg.Databases {
				if len(args) == 1 && db.Name != args[0] {
					continue
				}
				results = append(results, r.UpdateChannel(ctx, db, *channel))
			}
			if len(results) == 0 {
				return errors.New("no matching databases")
			}
			return printJSON(results)
		},
	}
}

func statusCmd(cfgPath *string, channel *string) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Print JSON status",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(*cfgPath)
			if err != nil {
				return err
			}
			report := struct {
				Databases []state.DatabaseState `json:"databases"`
			}{Databases: make([]state.DatabaseState, 0, len(cfg.Databases))}
			for _, raw := range cfg.Databases {
				db, err := config.ResolveDatabase(raw, *channel)
				if err != nil {
					return err
				}
				st, _ := state.LoadForDB(db)
				current := st.DatabaseState(db.Name)
				current.Name = db.Name
				current.Path = db.Path
				current.Channel = db.Channel
				if current.Status == "" {
					current.Status = "unknown"
				}
				current.PreviousVersions = len(st.Versions(db.Name))
				report.Databases = append(report.Databases, current)
			}
			return printJSON(report)
		},
	}
}

func rollbackCmd(cfgPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "rollback <database>",
		Short: "Roll back a database to the previous local version",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(*cfgPath)
			if err != nil {
				return err
			}
			m := metrics.New()
			r := runner.New(cfg, m)
			res, err := r.Rollback(args[0])
			if err != nil {
				return err
			}
			return printJSON(res)
		},
	}
}

func verifyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "verify <path>",
		Short: "Open an MMDB file and print metadata",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			info, err := mmdbcheck.Inspect(args[0])
			if err != nil {
				return err
			}
			return printJSON(info)
		},
	}
}

func runCmd(cfgPath *string, channel *string) *cobra.Command {
	return &cobra.Command{
		Use:   "run",
		Short: "Run the update daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(*cfgPath)
			if err != nil {
				return err
			}
			m := metrics.New()
			if cfg.Metrics.Listen != "" {
				go func() {
					if err := metrics.Serve(cfg.Metrics.Listen, m); err != nil {
						fmt.Fprintln(os.Stderr, err)
					}
				}()
			}
			if cfg.Readiness.Listen != "" {
				go func() {
					if err := health.Serve(cfg.Readiness.Listen, cfg, *channel); err != nil {
						fmt.Fprintln(os.Stderr, err)
					}
				}()
			}
			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()
			r := runner.New(cfg, m)
			for _, db := range cfg.Databases {
				r.UpdateChannel(ctx, db, *channel)
			}
			tickers := make([]*time.Ticker, 0, len(cfg.Databases))
			defer func() {
				for _, t := range tickers {
					t.Stop()
				}
			}()
			for _, db := range cfg.Databases {
				db := db
				if db.Interval.Duration <= 0 {
					continue
				}
				t := time.NewTicker(db.Interval.Duration)
				tickers = append(tickers, t)
				go func() {
					for {
						select {
						case <-ctx.Done():
							return
						case <-t.C:
							r.UpdateChannel(ctx, db, *channel)
						}
					}
				}()
			}
			<-ctx.Done()
			return nil
		},
	}
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func configPathForInit(path string) string {
	if path == "" || path == defaultConfig {
		return filepath.Join(".", "mmdbwatch.yaml")
	}
	return path
}
