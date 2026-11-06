package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/thomasbudiarjo/envily/internal/authz"
	"github.com/thomasbudiarjo/envily/internal/config"
	"github.com/thomasbudiarjo/envily/internal/crypto"
	"github.com/thomasbudiarjo/envily/internal/server"
	"github.com/thomasbudiarjo/envily/internal/service"
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

func newAdminCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "admin", Short: "Local administration commands (run on the server host)"}
	cmd.AddCommand(newAdminCreateUserCmd(app), newAdminResetPasswordCmd(app))
	return cmd
}

// adminService opens the database for a local admin command. It needs no
// master key: user management never touches secret values.
func adminService(app *App, cmd *cobra.Command) (*service.Service, func(), error) {
	d, _, err := openLocalDB(app, cmd, true)
	if err != nil {
		return nil, nil, err
	}
	return service.New(d, nil, service.Options{}), func() { d.Close() }, nil
}

func newAdminCreateUserCmd(app *App) *cobra.Command {
	var email, name string
	var isAdmin bool
	cmd := &cobra.Command{
		Use:   "create-user --email EMAIL [--name NAME] [--admin]",
		Short: "Create a user with a temporary password",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if email == "" {
				return withCode(ExitUsage, errors.New("--email is required"))
			}
			if name == "" {
				name = strings.SplitN(email, "@", 2)[0]
			}
			svc, done, err := adminService(app, cmd)
			if err != nil {
				return err
			}
			defer done()
			u, pw, err := svc.CreateUser(cmd.Context(), authz.System, service.CreateUserInput{Email: email, Name: name, IsAdmin: isAdmin})
			if err != nil {
				return serviceErr(err)
			}
			fmt.Fprintf(app.Stdout, "created user %s\ntemporary password: %s\n(it must be changed at first login)\n", u.Email, pw)
			return nil
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "email address")
	cmd.Flags().StringVar(&name, "name", "", "display name (default: part of the email before @)")
	cmd.Flags().BoolVar(&isAdmin, "admin", false, "make the user an instance admin")
	addDBFlags(cmd)
	return cmd
}

func newAdminResetPasswordCmd(app *App) *cobra.Command {
	var email string
	cmd := &cobra.Command{
		Use:   "reset-password --email EMAIL",
		Short: "Set a random temporary password and sign the user out everywhere",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if email == "" {
				return withCode(ExitUsage, errors.New("--email is required"))
			}
			svc, done, err := adminService(app, cmd)
			if err != nil {
				return err
			}
			defer done()
			u, err := svc.AdminUserByEmail(cmd.Context(), email)
			if err != nil {
				return serviceErr(err)
			}
			_, pw, err := svc.UpdateUser(cmd.Context(), authz.System, u.ID, service.UpdateUserInput{ResetPassword: true})
			if err != nil {
				return serviceErr(err)
			}
			fmt.Fprintf(app.Stdout, "temporary password for %s: %s\n(it must be changed at next login)\n", u.Email, pw)
			return nil
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "email address")
	addDBFlags(cmd)
	return cmd
}

// serviceErr turns service errors into readable messages and exit codes.
func serviceErr(err error) error {
	se := service.AsError(err)
	if se == nil {
		return err
	}
	msg := se.Message
	for f, m := range se.Details {
		msg += fmt.Sprintf("\n  %s: %s", f, m)
	}
	code := ExitError
	switch se.Code {
	case service.CodeNotFound:
		code = ExitNotFound
	case service.CodeForbidden:
		code = ExitForbidden
	case service.CodeUnauthorized, service.CodeInvalidCredentials:
		code = ExitUnauthorized
	}
	return withCode(code, errors.New(msg))
}

func newRotateMasterKeyCmd(app *App) *cobra.Command {
	var newKeyFile string
	cmd := &cobra.Command{
		Use:   "rotate-master-key --new-key-file PATH",
		Short: "Re-wrap every project key with a new master key (server must be stopped)",
		Long: "Re-wrap every project's data encryption key with the master key in PATH\n" +
			"and record it as the current key, in one transaction. The current key is\n" +
			"read from ENVILY_MASTER_KEY or ENVILY_MASTER_KEY_FILE. Secret values are not\n" +
			"re-encrypted. Stop the server first; afterwards, configure the new key and\n" +
			"start the server again. Keep the old key until the new one is confirmed to work.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if newKeyFile == "" {
				return withCode(ExitUsage, errors.New("--new-key-file is required (create one with `envily keygen`)"))
			}
			lookup := configLookup(app, cmd.Flags())
			oldKey, err := config.LoadMasterKey(lookup, "ENVILY_MASTER_KEY", "ENVILY_MASTER_KEY_FILE")
			if err != nil {
				return err
			}
			defer crypto.Zero(oldKey)
			newKey, err := config.LoadMasterKey(func(name string) (string, bool) {
				return newKeyFile, name == "--new-key-file"
			}, "", "--new-key-file")
			if err != nil {
				return err
			}
			defer crypto.Zero(newKey)
			d, _, err := openLocalDB(app, cmd, true)
			if err != nil {
				return err
			}
			defer d.Close()
			n, version, err := server.RotateMasterKey(cmd.Context(), d, oldKey, newKey)
			if err != nil {
				return err
			}
			fmt.Fprintf(app.Stdout, "re-wrapped the data keys of %d project(s); root key version is now %d\n"+
				"configure the new key (ENVILY_MASTER_KEY or ENVILY_MASTER_KEY_FILE) and start the server\n", n, version)
			return nil
		},
	}
	cmd.Flags().StringVar(&newKeyFile, "new-key-file", "", "file containing the new base64 master key")
	addDBFlags(cmd)
	return cmd
}
