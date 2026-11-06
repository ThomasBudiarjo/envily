package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/thomasbudiarjo/envily/internal/config"
	"github.com/thomasbudiarjo/envily/internal/crypto"
	"github.com/thomasbudiarjo/envily/internal/server"
	"github.com/thomasbudiarjo/envily/internal/store"
)

// addDBFlags adds the database flags used by local admin commands.
func addDBFlags(cmd *cobra.Command) {
	for _, f := range serverFlags {
		if f.flag == "db-driver" || f.flag == "db-dsn" {
			cmd.Flags().String(f.flag, "", f.usage+" ["+f.env+"]")
		}
	}
}

// openLocalDB opens the database directly, for admin commands that run on
// the server host.
func openLocalDB(app *App, cmd *cobra.Command, migrate bool) (store.DB, config.Config, error) {
	cfg, err := config.LoadDatabase(configLookup(app, cmd.Flags()))
	if err != nil {
		return nil, cfg, fmt.Errorf("invalid configuration:\n%w", err)
	}
	d, err := server.OpenDB(cmd.Context(), cfg, migrate, nil)
	return d, cfg, err
}

func newKeygenCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "keygen",
		Short: "Print a new random master key (base64)",
		Args:  cobra.NoArgs,
		Run: func(*cobra.Command, []string) {
			key := crypto.NewKey()
			defer crypto.Zero(key)
			fmt.Fprintln(app.Stdout, crypto.EncodeKey(key))
		},
	}
}

func newMigrateCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Apply database migrations and exit",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			d, _, err := openLocalDB(app, cmd, true)
			if err != nil {
				return err
			}
			defer d.Close()
			fmt.Fprintln(app.Stdout, "migrations applied")
			return nil
		},
	}
	addDBFlags(cmd)
	return cmd
}

func newBackupCmd(app *App) *cobra.Command {
	var out string
	cmd := &cobra.Command{
		Use:   "backup --out FILE",
		Short: "Write a consistent snapshot of the SQLite database",
		Long: "Write a consistent snapshot of the SQLite database using VACUUM INTO.\n" +
			"The snapshot holds encrypted secrets; store the master key separately.\n" +
			"For PostgreSQL use pg_dump.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if out == "" {
				return withCode(ExitUsage, errors.New("--out is required"))
			}
			cfg, err := config.LoadDatabase(configLookup(app, cmd.Flags()))
			if err != nil {
				return fmt.Errorf("invalid configuration:\n%w", err)
			}
			if cfg.DBDriver == "sqlite" {
				if _, err := os.Stat(cfg.DBDSN); err != nil {
					return fmt.Errorf("database %s: %w", cfg.DBDSN, errors.Unwrap(err))
				}
			}
			d, err := server.OpenDB(cmd.Context(), cfg, false, nil)
			if err != nil {
				return err
			}
			defer d.Close()
			if err := d.Backup(cmd.Context(), out); err != nil {
				return err
			}
			fmt.Fprintf(app.Stdout, "backup written to %s\n", out)
			return nil
		},
	}
	cmd.Flags().StringVar(&out, "out", "", "output file (must not exist)")
	addDBFlags(cmd)
	return cmd
}
