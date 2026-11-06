package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// ---- files ----

// projectFile is .envily.json: safe to commit, it holds no secrets.
type projectFile struct {
	URL     string `json:"url,omitempty"`
	Project string `json:"project,omitempty"`
	Env     string `json:"env,omitempty"`
}

const projectFileName = ".envily.json"

// findProjectFile looks for .envily.json in dir and its parents.
func findProjectFile(dir string) (projectFile, string, error) {
	for {
		path := filepath.Join(dir, projectFileName)
		raw, err := os.ReadFile(path) //nolint:gosec // path built from the working directory
		if err == nil {
			var f projectFile
			if err := json.Unmarshal(raw, &f); err != nil {
				return f, path, fmt.Errorf("%s is not valid JSON", path)
			}
			return f, path, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return projectFile{}, "", err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return projectFile{}, "", nil
		}
		dir = parent
	}
}

type credential struct {
	Token string `json:"token"`
	Email string `json:"email,omitempty"`
}

// credentialsFile is ~/.config/envily/credentials.json, keyed by server URL.
type credentialsFile struct {
	Current string                `json:"current,omitempty"`
	Servers map[string]credential `json:"servers"`
}

func (app *App) configDir() (string, error) {
	if dir, ok := app.Getenv("XDG_CONFIG_HOME"); ok && dir != "" {
		return filepath.Join(dir, "envily"), nil
	}
	home, ok := app.Getenv("HOME")
	if !ok || home == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		home = h
	}
	return filepath.Join(home, ".config", "envily"), nil
}

func (app *App) credentialsPath() (string, error) {
	dir, err := app.configDir()
	return filepath.Join(dir, "credentials.json"), err
}

func (app *App) loadCredentials() (credentialsFile, error) {
	c := credentialsFile{Servers: map[string]credential{}}
	path, err := app.credentialsPath()
	if err != nil {
		return c, err
	}
	raw, err := os.ReadFile(path) //nolint:gosec // the user's own config file
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	} else if err != nil {
		return c, err
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		return c, fmt.Errorf("%s is not valid JSON", path)
	}
	if c.Servers == nil {
		c.Servers = map[string]credential{}
	}
	return c, nil
}

func (app *App) saveCredentials(c credentialsFile) error {
	path, err := app.credentialsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, _ := json.MarshalIndent(c, "", "  ")
	return writeFilePrivate(path, append(raw, '\n'))
}

// writeFilePrivate writes data to path with mode 0600, also tightening the
// mode of an existing file.
func writeFilePrivate(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600) //nolint:gosec // user-chosen path
	if err != nil {
		return err
	}
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// ---- context ----

// clientCtx is the resolved server, credentials, and target of a command.
type clientCtx struct {
	app     *App
	cmd     *cobra.Command
	url     string
	token   string
	project string
	env     string
	json    bool
}

func normalizeURL(u string) string { return strings.TrimRight(strings.TrimSpace(u), "/") }

// resolve applies the precedence: flags, environment variables, .envily.json,
// the credentials file, then (for service tokens) the token's own scope.
func (app *App) resolve(cmd *cobra.Command, needTarget bool) (*clientCtx, error) {
	flag := func(name string) string {
		v, _ := cmd.Flags().GetString(name)
		return strings.TrimSpace(v)
	}
	envVar := func(name string) string {
		v, _ := app.Getenv(name)
		return strings.TrimSpace(v)
	}
	first := func(vals ...string) string {
		for _, v := range vals {
			if v != "" {
				return v
			}
		}
		return ""
	}
	wd, err := app.Getwd()
	if err != nil {
		return nil, err
	}
	pf, _, err := findProjectFile(wd)
	if err != nil {
		return nil, err
	}
	creds, err := app.loadCredentials()
	if err != nil {
		return nil, err
	}
	jsonOut, _ := cmd.Flags().GetBool("json")
	c := &clientCtx{app: app, cmd: cmd, json: jsonOut,
		url:     normalizeURL(first(flag("url"), envVar("ENVILY_URL"), pf.URL, creds.Current)),
		project: first(flag("project"), envVar("ENVILY_PROJECT"), pf.Project),
		env:     first(flag("env"), envVar("ENVILY_ENV"), pf.Env),
		token:   first(flag("token"), envVar("ENVILY_TOKEN")),
	}
	if c.url == "" {
		return nil, withCode(ExitUsage, errors.New("no server: pass --url, set ENVILY_URL, or run `envily login --url URL`"))
	}
	if c.token == "" {
		c.token = creds.Servers[c.url].Token
	}
	if c.token == "" {
		return nil, withCode(ExitUnauthorized, fmt.Errorf("not logged in to %s: run `envily login` or set ENVILY_TOKEN", c.url))
	}
	if needTarget && (c.project == "" || c.env == "") && strings.HasPrefix(c.token, "envily_svc_") {
		var info struct {
			Project      struct{ Slug string }
			Environments []struct{ Slug string }
		}
		if err := c.call("GET", "/api/v1/token/info", nil, &info); err != nil {
			return nil, err
		}
		if c.project == "" {
			c.project = info.Project.Slug
		}
		if c.env == "" && len(info.Environments) == 1 {
			c.env = info.Environments[0].Slug
		}
	}
	if needTarget && (c.project == "" || c.env == "") {
		return nil, withCode(ExitUsage, errors.New("no project or environment: pass --project and --env, set ENVILY_PROJECT and ENVILY_ENV, or run `envily link`"))
	}
	return c, nil
}

// ---- HTTP ----

// apiErr is an error response from the server.
type apiErr struct {
	Status  int
	Code    string
	Message string
	Details map[string]string
}

func (e *apiErr) Error() string {
	msg := e.Message
	for f, m := range e.Details {
		msg += fmt.Sprintf("\n  %s: %s", f, m)
	}
	return msg
}

func exitCodeFor(status int) int {
	switch status {
	case http.StatusUnauthorized:
		return ExitUnauthorized
	case http.StatusForbidden:
		return ExitForbidden
	case http.StatusNotFound:
		return ExitNotFound
	}
	return ExitError
}

// do sends a request and returns the raw response body. body may be nil, a
// []byte (sent as text), or a value encoded as JSON.
func (c *clientCtx) do(method, path string, body any) ([]byte, error) {
	var rd io.Reader
	contentType := ""
	switch b := body.(type) {
	case nil:
	case []byte:
		rd, contentType = bytes.NewReader(b), "text/plain; charset=utf-8"
	default:
		raw, err := json.Marshal(b)
		if err != nil {
			return nil, err
		}
		rd, contentType = bytes.NewReader(raw), "application/json"
	}
	ctx, cancel := context.WithTimeout(c.cmd.Context(), 60*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, c.url+path, rd)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header.Set("User-Agent", "envily-cli")
	res, err := c.app.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot reach %s: %w", c.url, errors.Unwrap(err))
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 64<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode >= 400 {
		var env struct {
			Error apiErr `json:"error"`
		}
		e := &apiErr{Status: res.StatusCode, Message: res.Status}
		if json.Unmarshal(raw, &env) == nil && env.Error.Message != "" {
			*e = env.Error
			e.Status = res.StatusCode
		}
		return nil, withCode(exitCodeFor(res.StatusCode), e)
	}
	return raw, nil
}

// call sends a request and decodes a JSON response into out (if not nil).
func (c *clientCtx) call(method, path string, body, out any) error {
	raw, err := c.do(method, path, body)
	if err != nil || out == nil || len(raw) == 0 {
		return err
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("unexpected response from %s", c.url)
	}
	return nil
}

func (c *clientCtx) secretsPath(suffix string) string {
	return "/api/v1/projects/" + url.PathEscape(c.project) + "/environments/" + url.PathEscape(c.env) + "/secrets" + suffix
}

// printJSON writes v as indented JSON.
func (c *clientCtx) printJSON(v any) error {
	enc := json.NewEncoder(c.app.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
