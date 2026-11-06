package cli

import (
	"fmt"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/thomasbudiarjo/envily/internal/config"
	"github.com/thomasbudiarjo/envily/internal/server"
	"github.com/thomasbudiarjo/envily/internal/version"
)

// serverFlags maps `envily server` flags to the environment variables they
// override.
var serverFlags = []struct{ flag, env, usage string }{
	{"addr", "ENVILY_ADDR", "listen address (default :8080)"},
	{"public-url", "ENVILY_PUBLIC_URL", "public URL of this server"},
	{"db-driver", "ENVILY_DB_DRIVER", "sqlite or postgres"},
	{"db-dsn", "ENVILY_DB_DSN", "SQLite file path or Postgres URL"},
	{"log-level", "ENVILY_LOG_LEVEL", "debug, info, warn, or error"},
	{"log-format", "ENVILY_LOG_FORMAT", "json or text"},
}

// configLookup reads configuration from flags that were set explicitly,
// falling back to the environment.
func configLookup(app *App, flags *pflag.FlagSet) config.Lookup {
	return func(name string) (string, bool) {
		for _, f := range serverFlags {
			if f.env == name && flags.Changed(f.flag) {
				v, _ := flags.GetString(f.flag)
				return v, true
			}
		}
		return app.Getenv(name)
	}
}

func addServerFlags(cmd *cobra.Command) {
	for _, f := range serverFlags {
		cmd.Flags().String(f.flag, "", f.usage+" ["+f.env+"]")
	}
}

func loadConfig(app *App, cmd *cobra.Command) (config.Config, error) {
	cfg, err := config.Load(configLookup(app, cmd.Flags()))
	if err != nil {
		return cfg, fmt.Errorf("invalid configuration:\n%w", err)
	}
	return cfg, nil
}

func newServerCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "server",
		Short: "Run the HTTP server and web UI",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfig(app, cmd)
			if err != nil {
				return err
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()
			return server.Run(ctx, cfg, cfg.Logger(app.Stderr))
		},
	}
	addServerFlags(cmd)
	return cmd
}

func newVersionCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Args:  cobra.NoArgs,
		Run: func(*cobra.Command, []string) {
			fmt.Fprintln(app.Stdout, version.String())
		},
	}
}
