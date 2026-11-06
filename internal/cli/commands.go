package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

// addClientFlags adds the flags shared by client commands.
func addClientFlags(root *cobra.Command) {
	pf := root.PersistentFlags()
	pf.String("url", "", "server URL [ENVILY_URL]")
	pf.String("project", "", "project slug [ENVILY_PROJECT]")
	pf.String("env", "", "environment slug [ENVILY_ENV]")
	pf.String("token", "", "service or personal token [ENVILY_TOKEN]")
	pf.Bool("json", false, "print machine-readable JSON")
}

// abs resolves path against the app's working directory.
func (app *App) abs(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	if wd, err := app.Getwd(); err == nil {
		return filepath.Join(wd, path)
	}
	return path
}

func (app *App) reader() *bufio.Reader {
	if app.in == nil {
		app.in = bufio.NewReader(app.Stdin)
	}
	return app.in
}

func (app *App) prompt(label string) (string, error) {
	fmt.Fprint(app.Stderr, label)
	line, err := app.reader().ReadString('\n')
	if err != nil && (line == "" || !errors.Is(err, io.EOF)) {
		return "", errors.New("no input")
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func (app *App) promptPassword(label string) (string, error) {
	if app.ReadPassword == nil {
		return app.prompt(label)
	}
	fmt.Fprint(app.Stderr, label)
	pw, err := app.ReadPassword()
	fmt.Fprintln(app.Stderr)
	return pw, err
}

// ---- login, logout, whoami, link ----

func newLoginCmd(app *App) *cobra.Command {
	var email, tokenName string
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Log in and store a personal token in ~/.config/envily/credentials.json",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			server, _ := cmd.Flags().GetString("url")
			if server == "" {
				server, _ = app.Getenv("ENVILY_URL")
			}
			if server == "" {
				wd, err := app.Getwd()
				if err != nil {
					return err
				}
				pf, _, err := findProjectFile(wd)
				if err != nil {
					return err
				}
				server = pf.URL
			}
			server = normalizeURL(server)
			if u, err := url.Parse(server); server == "" || err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
				return withCode(ExitUsage, errors.New("pass the server with --url https://secrets.example.com"))
			}
			var err error
			if email == "" {
				if email, err = app.prompt("Email: "); err != nil {
					return err
				}
			}
			password, err := app.promptPassword("Password: ")
			if err != nil {
				return err
			}
			if tokenName == "" {
				host, _ := os.Hostname()
				tokenName = "envily CLI on " + host
			}
			c := &clientCtx{app: app, cmd: cmd, url: server}
			var res struct {
				ID        string  `json:"id"`
				Token     string  `json:"token"`
				ExpiresAt *string `json:"expiresAt"`
			}
			if err := c.call("POST", "/api/v1/auth/cli-login", map[string]string{"email": email, "password": password, "tokenName": tokenName}, &res); err != nil {
				return err
			}
			creds, err := app.loadCredentials()
			if err != nil {
				return err
			}
			creds.Servers[server] = credential{Token: res.Token, Email: strings.ToLower(strings.TrimSpace(email))}
			creds.Current = server
			if err := app.saveCredentials(creds); err != nil {
				return err
			}
			exp := "never"
			if res.ExpiresAt != nil {
				exp = *res.ExpiresAt
			}
			fmt.Fprintf(app.Stdout, "Logged in to %s as %s (token expires %s)\n", server, email, exp)
			return nil
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "email address (prompted when omitted)")
	cmd.Flags().StringVar(&tokenName, "token-name", "", "name of the personal token to create")
	return cmd
}

func newLogoutCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Revoke the stored personal token and forget it",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			creds, err := app.loadCredentials()
			if err != nil {
				return err
			}
			server, _ := cmd.Flags().GetString("url")
			if server == "" {
				server, _ = app.Getenv("ENVILY_URL")
			}
			if server == "" {
				server = creds.Current
			}
			server = normalizeURL(server)
			cred, ok := creds.Servers[server]
			if !ok {
				return withCode(ExitUnauthorized, fmt.Errorf("not logged in to %s", server))
			}
			c := &clientCtx{app: app, cmd: cmd, url: server, token: cred.Token}
			// "current" revokes the token that authenticates the request.
			revokeErr := c.call("DELETE", "/api/v1/me/tokens/current", nil, nil)
			delete(creds.Servers, server)
			if creds.Current == server {
				creds.Current = ""
			}
			if err := app.saveCredentials(creds); err != nil {
				return err
			}
			if revokeErr != nil {
				fmt.Fprintf(app.Stderr, "warning: could not revoke the token on the server: %v\n", revokeErr)
			}
			fmt.Fprintf(app.Stdout, "Logged out of %s\n", server)
			return nil
		},
	}
}

func newWhoamiCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show the active user or token and server",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := app.resolve(cmd, false)
			if err != nil {
				return err
			}
			if strings.HasPrefix(c.token, "envily_svc_") {
				var info struct {
					Name         string `json:"name"`
					Permission   string `json:"permission"`
					Project      struct{ Slug string }
					Environments []struct{ Slug string }
				}
				if err := c.call("GET", "/api/v1/token/info", nil, &info); err != nil {
					return err
				}
				if c.json {
					return c.printJSON(map[string]any{"server": c.url, "type": "service_token", "name": info.Name,
						"project": info.Project.Slug, "permission": info.Permission, "environments": slugs(info.Environments)})
				}
				fmt.Fprintf(app.Stdout, "service token %q on %s\nproject %s, environments %s, permission %s\n",
					info.Name, c.url, info.Project.Slug, strings.Join(slugs(info.Environments), ", "), info.Permission)
				return nil
			}
			var me struct {
				User    struct{ Email, Name string } `json:"user"`
				IsAdmin bool                         `json:"isAdmin"`
			}
			if err := c.call("GET", "/api/v1/auth/me", nil, &me); err != nil {
				return err
			}
			if c.json {
				return c.printJSON(map[string]any{"server": c.url, "type": "user", "email": me.User.Email, "name": me.User.Name, "isAdmin": me.IsAdmin})
			}
			admin := ""
			if me.IsAdmin {
				admin = " (admin)"
			}
			fmt.Fprintf(app.Stdout, "%s <%s>%s on %s\n", me.User.Name, me.User.Email, admin, c.url)
			return nil
		},
	}
}

func slugs(list []struct{ Slug string }) []string {
	out := make([]string, len(list))
	for i, x := range list {
		out[i] = x.Slug
	}
	return out
}

type projectItem struct {
	Name         string  `json:"name"`
	Slug         string  `json:"slug"`
	Role         *string `json:"role"`
	UpdatedAt    string  `json:"updatedAt"`
	Environments []struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	} `json:"environments"`
}

func (c *clientCtx) projects() ([]projectItem, error) {
	var res struct{ Items []projectItem }
	err := c.call("GET", "/api/v1/projects", nil, &res)
	return res.Items, err
}

// choose prompts for one of options (by number or by value).
func (app *App) choose(label string, options []string) (string, error) {
	if len(options) == 0 {
		return "", fmt.Errorf("no %s available", label)
	}
	if len(options) == 1 {
		return options[0], nil
	}
	for i, o := range options {
		fmt.Fprintf(app.Stderr, "  %d) %s\n", i+1, o)
	}
	ans, err := app.prompt(fmt.Sprintf("Choose a %s [1-%d]: ", label, len(options)))
	if err != nil {
		return "", err
	}
	if n, err := strconv.Atoi(strings.TrimSpace(ans)); err == nil && n >= 1 && n <= len(options) {
		return options[n-1], nil
	}
	for _, o := range options {
		if o == strings.TrimSpace(ans) {
			return o, nil
		}
	}
	return "", withCode(ExitUsage, fmt.Errorf("%q is not a valid choice", ans))
}

func newLinkCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "link",
		Short: "Choose a project and environment for this directory (writes .envily.json)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := app.resolve(cmd, false)
			if err != nil {
				return err
			}
			projects, err := c.projects()
			if err != nil {
				return err
			}
			project, _ := cmd.Flags().GetString("project")
			if project == "" {
				names := make([]string, len(projects))
				for i, p := range projects {
					names[i] = p.Slug
				}
				if project, err = app.choose("project", names); err != nil {
					return err
				}
			}
			var envs []string
			for _, p := range projects {
				if p.Slug == project {
					for _, e := range p.Environments {
						envs = append(envs, e.Slug)
					}
				}
			}
			if envs == nil {
				return withCode(ExitNotFound, fmt.Errorf("project %s not found", project))
			}
			env, _ := cmd.Flags().GetString("env")
			if env == "" {
				if env, err = app.choose("environment", envs); err != nil {
					return err
				}
			}
			wd, err := app.Getwd()
			if err != nil {
				return err
			}
			raw, _ := json.MarshalIndent(projectFile{URL: c.url, Project: project, Env: env}, "", "  ")
			path := filepath.Join(wd, projectFileName)
			if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil { //nolint:gosec // holds no secrets, meant to be committed
				return err
			}
			fmt.Fprintf(app.Stdout, "Linked %s to %s/%s\n", path, project, env)
			return nil
		},
	}
}

// ---- projects, envs ----

func newProjectsCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "projects", Short: "Work with projects"}
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List projects",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := app.resolve(cmd, false)
			if err != nil {
				return err
			}
			list, err := c.projects()
			if err != nil {
				return err
			}
			if c.json {
				return c.printJSON(list)
			}
			tw := tabwriter.NewWriter(app.Stdout, 0, 4, 2, ' ', 0)
			fmt.Fprintln(tw, "SLUG\tNAME\tROLE\tENVIRONMENTS")
			for _, p := range list {
				role := "-"
				if p.Role != nil {
					role = *p.Role
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%d\n", p.Slug, p.Name, role, len(p.Environments))
			}
			return tw.Flush()
		},
	})
	return cmd
}

func newEnvsCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "envs", Short: "Work with environments"}
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List the environments of the active project",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := app.resolve(cmd, false)
			if err != nil {
				return err
			}
			if c.project == "" {
				var info struct{ Project struct{ Slug string } }
				if strings.HasPrefix(c.token, "envily_svc_") && c.call("GET", "/api/v1/token/info", nil, &info) == nil {
					c.project = info.Project.Slug
				}
			}
			if c.project == "" {
				return withCode(ExitUsage, errors.New("no project: pass --project or run `envily link`"))
			}
			var res struct {
				Items []struct {
					Name string `json:"name"`
					Slug string `json:"slug"`
				} `json:"items"`
			}
			if err := c.call("GET", "/api/v1/projects/"+url.PathEscape(c.project)+"/environments", nil, &res); err != nil {
				return err
			}
			if c.json {
				return c.printJSON(res.Items)
			}
			tw := tabwriter.NewWriter(app.Stdout, 0, 4, 2, ' ', 0)
			fmt.Fprintln(tw, "SLUG\tNAME")
			for _, e := range res.Items {
				fmt.Fprintf(tw, "%s\t%s\n", e.Slug, e.Name)
			}
			return tw.Flush()
		},
	})
	return cmd
}

// ---- secrets ----

type secretItem struct {
	Key       string  `json:"key"`
	Version   int64   `json:"version"`
	UpdatedAt string  `json:"updatedAt"`
	Value     *string `json:"value,omitempty"`
	UpdatedBy struct {
		Label string `json:"label"`
	} `json:"updatedBy"`
}

// fetchValues returns every secret of the active environment.
func (c *clientCtx) fetchValues() (map[string]string, error) {
	var res struct{ Items []secretItem }
	if err := c.call("GET", c.secretsPath("?values=true"), nil, &res); err != nil {
		return nil, err
	}
	out := make(map[string]string, len(res.Items))
	for _, s := range res.Items {
		if s.Value != nil {
			out[s.Key] = *s.Value
		}
	}
	return out, nil
}

func newSecretsCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "secrets", Short: "Read and change secrets of the active environment"}

	list := &cobra.Command{
		Use:   "list",
		Short: "List secret keys (without values)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := app.resolve(cmd, true)
			if err != nil {
				return err
			}
			var res struct{ Items []secretItem }
			if err := c.call("GET", c.secretsPath(""), nil, &res); err != nil {
				return err
			}
			if c.json {
				return c.printJSON(res.Items)
			}
			tw := tabwriter.NewWriter(app.Stdout, 0, 4, 2, ' ', 0)
			fmt.Fprintln(tw, "KEY\tVERSION\tUPDATED\tBY")
			for _, s := range res.Items {
				updated := s.UpdatedAt
				if t, err := time.Parse(time.RFC3339Nano, s.UpdatedAt); err == nil {
					updated = t.Local().Format("2006-01-02 15:04")
				}
				fmt.Fprintf(tw, "%s\t%d\t%s\t%s\n", s.Key, s.Version, updated, s.UpdatedBy.Label)
			}
			return tw.Flush()
		},
	}

	var plain bool
	get := &cobra.Command{
		Use:   "get KEY",
		Short: "Print a secret value",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := app.resolve(cmd, true)
			if err != nil {
				return err
			}
			var s secretItem
			if err := c.call("GET", c.secretsPath("/"+url.PathEscape(args[0])), nil, &s); err != nil {
				return err
			}
			if c.json {
				return c.printJSON(map[string]any{"key": s.Key, "value": s.Value, "version": s.Version})
			}
			value := ""
			if s.Value != nil {
				value = *s.Value
			}
			if plain {
				_, err = io.WriteString(app.Stdout, value)
			} else {
				_, err = fmt.Fprintln(app.Stdout, value)
			}
			return err
		},
	}
	get.Flags().BoolVar(&plain, "plain", false, "print the value without a trailing newline")

	set := &cobra.Command{
		Use:   "set KEY=VALUE [KEY2=VALUE2 ...]",
		Short: "Create or update secrets in one atomic change (KEY=- reads the value from stdin)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			upserts := make([]map[string]string, 0, len(args))
			stdinUsed := false
			for _, a := range args {
				key, value, ok := strings.Cut(a, "=")
				if !ok || key == "" {
					return withCode(ExitUsage, fmt.Errorf("argument %d: expected KEY=VALUE", len(upserts)+1))
				}
				if value == "-" {
					if stdinUsed {
						return withCode(ExitUsage, errors.New("only one value can be read from stdin"))
					}
					stdinUsed = true
					raw, err := io.ReadAll(app.reader())
					if err != nil {
						return err
					}
					value = strings.TrimSuffix(strings.TrimSuffix(string(raw), "\n"), "\r")
				}
				upserts = append(upserts, map[string]string{"key": key, "value": value})
			}
			c, err := app.resolve(cmd, true)
			if err != nil {
				return err
			}
			var res map[string]int
			if err := c.call("POST", c.secretsPath(":batch"), map[string]any{"upserts": upserts}, &res); err != nil {
				return err
			}
			if c.json {
				return c.printJSON(res)
			}
			fmt.Fprintf(app.Stdout, "%d created, %d updated in %s/%s\n", res["created"], res["updated"], c.project, c.env)
			return nil
		},
	}

	var yes bool
	del := &cobra.Command{
		Use:   "delete KEY",
		Short: "Delete a secret",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := app.resolve(cmd, true)
			if err != nil {
				return err
			}
			if !yes {
				ans, err := app.prompt(fmt.Sprintf("Delete %s from %s/%s? [y/N] ", args[0], c.project, c.env))
				if err != nil || !strings.EqualFold(strings.TrimSpace(ans), "y") {
					return withCode(ExitError, errors.New("cancelled"))
				}
			}
			if err := c.call("DELETE", c.secretsPath("/"+url.PathEscape(args[0])), nil, nil); err != nil {
				return err
			}
			fmt.Fprintf(app.Stdout, "Deleted %s\n", args[0])
			return nil
		},
	}
	del.Flags().BoolVar(&yes, "yes", false, "do not ask for confirmation")

	cmd.AddCommand(list, get, set, del)
	return cmd
}

// ---- run, export, import ----

func newRunCmd(app *App) *cobra.Command {
	var noOverride bool
	cmd := &cobra.Command{
		Use:   "run [flags] -- COMMAND [ARGS...]",
		Short: "Run a command with the environment's secrets as environment variables",
		Long: "Fetch every secret of the active environment and run COMMAND with them added to\n" +
			"the current environment. Secrets replace existing variables unless --no-override\n" +
			"is set. Secrets are never written to disk.",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := app.resolve(cmd, true)
			if err != nil {
				return err
			}
			secrets, err := c.fetchValues()
			if err != nil {
				return err
			}
			path, err := app.LookPath(args[0])
			if err != nil {
				return withCode(ExitError, fmt.Errorf("%s: command not found", args[0]))
			}
			return app.Exec(path, args, mergeEnv(app.Environ(), secrets, noOverride))
		},
	}
	cmd.Flags().BoolVar(&noOverride, "no-override", false, "keep variables that are already set")
	cmd.Flags().SetInterspersed(false)
	return cmd
}

// mergeEnv adds secrets to environ. ENVILY_TOKEN is dropped so the child
// does not inherit the credential.
func mergeEnv(environ []string, secrets map[string]string, noOverride bool) []string {
	out := make([]string, 0, len(environ)+len(secrets))
	existing := map[string]bool{}
	for _, kv := range environ {
		k, _, _ := strings.Cut(kv, "=")
		if k == "ENVILY_TOKEN" {
			continue
		}
		if _, isSecret := secrets[k]; isSecret && !noOverride {
			continue
		}
		existing[k] = true
		out = append(out, kv)
	}
	keys := make([]string, 0, len(secrets))
	for k := range secrets {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if !existing[k] {
			out = append(out, k+"="+secrets[k])
		}
	}
	return out
}

func newExportCmd(app *App) *cobra.Command {
	var format, out string
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Print or save the environment's secrets as dotenv, json, or yaml",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if format != "dotenv" && format != "json" && format != "yaml" {
				return withCode(ExitUsage, errors.New("--format must be dotenv, json, or yaml"))
			}
			c, err := app.resolve(cmd, true)
			if err != nil {
				return err
			}
			raw, err := c.do("GET", c.secretsPath(":export?format="+format), nil)
			if err != nil {
				return err
			}
			if out == "" || out == "-" {
				_, err = app.Stdout.Write(raw)
				return err
			}
			if err := writeFilePrivate(app.abs(out), raw); err != nil {
				return err
			}
			fmt.Fprintf(app.Stderr, "Wrote %s (mode 0600)\n", out)
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", "dotenv", "dotenv, json, or yaml")
	cmd.Flags().StringVar(&out, "out", "", "write to FILE (mode 0600) instead of stdout")
	return cmd
}

func newImportCmd(app *App) *cobra.Command {
	var mode string
	cmd := &cobra.Command{
		Use:   "import FILE",
		Short: "Import secrets from a .env or JSON file (- for stdin)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if mode != "merge" && mode != "overwrite" {
				return withCode(ExitUsage, errors.New("--mode must be merge or overwrite"))
			}
			var raw []byte
			var err error
			if args[0] == "-" {
				raw, err = io.ReadAll(app.reader())
			} else {
				raw, err = os.ReadFile(app.abs(args[0]))
			}
			if err != nil {
				return err
			}
			c, err := app.resolve(cmd, true)
			if err != nil {
				return err
			}
			var res map[string]int
			if err := c.call("POST", c.secretsPath(":import?mode="+mode), raw, &res); err != nil {
				return err
			}
			if c.json {
				return c.printJSON(res)
			}
			fmt.Fprintf(app.Stdout, "%d created, %d updated, %d unchanged, %d deleted in %s/%s\n",
				res["created"], res["updated"], res["unchanged"], res["deleted"], c.project, c.env)
			return nil
		},
	}
	cmd.Flags().StringVar(&mode, "mode", "merge", "merge (keep other keys) or overwrite (delete keys not in FILE)")
	return cmd
}
