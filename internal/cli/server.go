package cli

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os/signal"
	"strings"
	"syscall"
	"time"

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

func newHealthcheckCmd(app *App) *cobra.Command {
	var timeout time.Duration
	cmd := &cobra.Command{
		Use:   "healthcheck",
		Short: "Exit 0 when the local server is ready (for container health checks)",
		Long: "Request /readyz from the server listening on ENVILY_ADDR (default :8080)\n" +
			"and exit 0 when it answers 200. Container images have no curl or wget.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			addr, ok := app.Getenv("ENVILY_ADDR")
			if !ok || strings.TrimSpace(addr) == "" {
				addr = ":8080"
			}
			u, err := readyURL(strings.TrimSpace(addr))
			if err != nil {
				return withCode(ExitUsage, err)
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
			if err != nil {
				return err
			}
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				return fmt.Errorf("server not reachable at %s", u)
			}
			res.Body.Close()
			if res.StatusCode != http.StatusOK {
				return fmt.Errorf("server not ready: %s answered %d", u, res.StatusCode)
			}
			return nil
		},
	}
	cmd.Flags().DurationVar(&timeout, "timeout", 3*time.Second, "how long to wait for an answer")
	return cmd
}

// readyURL turns a listen address into the URL of /readyz on this host;
// wildcard and empty hosts become loopback.
func readyURL(addr string) (string, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", fmt.Errorf("ENVILY_ADDR %q is not host:port", addr)
	}
	switch host {
	case "", "0.0.0.0":
		host = "127.0.0.1"
	case "::":
		host = "::1"
	}
	return "http://" + net.JoinHostPort(host, port) + "/readyz", nil
}
