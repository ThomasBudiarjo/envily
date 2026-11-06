package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/thomasbudiarjo/envily/internal/config"
	"github.com/thomasbudiarjo/envily/internal/crypto"
	"github.com/thomasbudiarjo/envily/internal/httpapi"
	"github.com/thomasbudiarjo/envily/internal/service"
	"github.com/thomasbudiarjo/envily/internal/store/sqlite"
)

const password = "correct horse battery staple"

type cliEnv struct {
	t       *testing.T
	url     string
	home    string
	vars    map[string]string
	execed  []string // env passed to the last exec
	argv    []string
	execErr error
}

func newCLIEnv(t *testing.T) *cliEnv {
	t.Helper()
	ctx := context.Background()
	d, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "envily.db"), true, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	kr, _ := crypto.NewKeyring(crypto.NewKey(), 1)
	svc := service.New(d, kr, service.Options{})
	if _, _, err := svc.Setup(ctx, service.SetupInput{Email: "admin@example.com", Name: "Admin", Password: password}); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(nil)
	srv.Start()
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	srv.Config.Handler = httpapi.New(httpapi.Deps{Config: config.Config{PublicURL: u}, Service: svc,
		Logger: slog.New(slog.DiscardHandler), UI: fstest.MapFS{}})
	home := t.TempDir()
	return &cliEnv{t: t, url: srv.URL, home: home, vars: map[string]string{"HOME": home, "KEEP": "outer", "A": "outer"}}
}

// run executes the CLI in dir with stdin and returns exit code and output.
func (e *cliEnv) run(dir, stdin string, args ...string) (int, string, string) {
	e.t.Helper()
	var out, errOut bytes.Buffer
	app := &App{
		Stdin: strings.NewReader(stdin), Stdout: &out, Stderr: &errOut,
		Getenv: func(k string) (string, bool) { v, ok := e.vars[k]; return v, ok },
		Environ: func() []string {
			var env []string
			for k, v := range e.vars {
				env = append(env, k+"="+v)
			}
			return env
		},
		Getwd:      func() (string, error) { return dir, nil },
		HTTPClient: &http.Client{},
		LookPath:   func(s string) (string, error) { return "/bin/" + s, nil },
		Exec: func(path string, argv, env []string) error {
			e.argv, e.execed = append([]string{path}, argv...), env
			return e.execErr
		},
	}
	code := Run(context.Background(), app, args)
	return code, out.String(), errOut.String()
}

func (e *cliEnv) must(dir, stdin string, args ...string) string {
	e.t.Helper()
	code, out, errOut := e.run(dir, stdin, args...)
	if code != 0 {
		e.t.Fatalf("envily %s: exit %d\n%s%s", strings.Join(args, " "), code, out, errOut)
	}
	return out
}

func (e *cliEnv) wantExit(want int, dir string, args ...string) string {
	e.t.Helper()
	code, out, errOut := e.run(dir, "", args...)
	if code != want {
		e.t.Fatalf("envily %s: exit %d, want %d\n%s%s", strings.Join(args, " "), code, want, out, errOut)
	}
	return errOut
}

// api calls the server directly with a bearer token.
func (e *cliEnv) api(token, method, path string, body any) map[string]any {
	e.t.Helper()
	raw, _ := json.Marshal(body)
	req, _ := http.NewRequest(method, e.url+path, bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		e.t.Fatalf("%s %s: %d %s", method, path, res.StatusCode, b)
	}
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	return m
}

func envMap(env []string) map[string]string {
	m := map[string]string{}
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		m[k] = v
	}
	return m
}

func TestCLIEndToEnd(t *testing.T) {
	e := newCLIEnv(t)
	work := t.TempDir()

	// login stores a personal token in a private credentials file.
	out := e.must(work, password+"\n", "login", "--url", e.url+"/", "--email", "admin@example.com")
	if !strings.Contains(out, "Logged in") {
		t.Fatalf("login output %q", out)
	}
	credPath := filepath.Join(e.home, ".config", "envily", "credentials.json")
	st, err := os.Stat(credPath)
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("credentials mode %v (%v)", st.Mode().Perm(), err)
	}
	var creds credentialsFile
	raw, _ := os.ReadFile(credPath)
	_ = json.Unmarshal(raw, &creds)
	pat := creds.Servers[e.url].Token
	if creds.Current != e.url || !strings.HasPrefix(pat, "envily_pat_") {
		t.Fatalf("credentials %s", raw)
	}
	if out := e.must(work, "", "whoami"); !strings.Contains(out, "admin@example.com") {
		t.Fatalf("whoami %q", out)
	}
	if code, _, _ := e.run(work, "wrong password\n", "login", "--url", e.url, "--email", "admin@example.com"); code != ExitUnauthorized {
		t.Fatalf("bad login exit %d", code)
	}

	// link writes .envily.json; with one project it is chosen automatically.
	e.api(pat, "POST", "/api/v1/projects", map[string]string{"name": "Web App"})
	e.must(work, "1\n", "link")
	var pf projectFile
	raw, _ = os.ReadFile(filepath.Join(work, ".envily.json"))
	if json.Unmarshal(raw, &pf) != nil || pf != (projectFile{URL: e.url, Project: "web-app", Env: "development"}) {
		t.Fatalf(".envily.json %s", raw)
	}
	sub := filepath.Join(work, "src", "pkg") // found from subdirectories too
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	// secrets set / get / list / delete.
	e.must(sub, "", "secrets", "set", "A=1", "B=two words", "EMPTY=")
	e.must(sub, "line one\nline two\n", "secrets", "set", "MULTI=-")
	if out := e.must(sub, "", "secrets", "get", "A", "--plain"); out != "1" {
		t.Fatalf("get --plain %q", out)
	}
	if out := e.must(sub, "", "secrets", "get", "MULTI"); out != "line one\nline two\n" {
		t.Fatalf("get multi %q", out)
	}
	list := e.must(sub, "", "secrets", "list")
	for _, k := range []string{"KEY", "A", "B", "EMPTY", "MULTI"} {
		if !strings.Contains(list, k) {
			t.Fatalf("list lacks %s:\n%s", k, list)
		}
	}
	if strings.Contains(list, "two words") {
		t.Fatal("list shows values")
	}
	var items []map[string]any
	if err := json.Unmarshal([]byte(e.must(sub, "", "secrets", "list", "--json")), &items); err != nil || len(items) != 4 {
		t.Fatalf("list --json: %v %v", items, err)
	}
	e.must(sub, "y\n", "secrets", "delete", "EMPTY")
	e.wantExit(ExitNotFound, sub, "secrets", "get", "EMPTY")
	e.wantExit(ExitUsage, sub, "secrets", "set", "NOEQUALS")
	if msg := e.wantExit(ExitError, sub, "secrets", "set", "bad-key=x"); !strings.Contains(msg, "must start with a letter") {
		t.Fatalf("validation message %q", msg)
	}

	// run: secrets override existing variables unless --no-override.
	e.vars["ENVILY_TOKEN"] = "" // present but empty: must not reach the child
	e.must(sub, "", "run", "--", "printenv", "-0")
	got := envMap(e.execed)
	if got["A"] != "1" || got["B"] != "two words" || got["MULTI"] != "line one\nline two" || got["KEEP"] != "outer" {
		t.Fatalf("run env %v", got)
	}
	if _, ok := got["ENVILY_TOKEN"]; ok {
		t.Fatal("ENVILY_TOKEN passed to the child")
	}
	if !slices.Equal(e.argv, []string{"/bin/printenv", "printenv", "-0"}) {
		t.Fatalf("argv %v", e.argv)
	}
	e.must(sub, "", "run", "--no-override", "--", "env")
	if envMap(e.execed)["A"] != "outer" {
		t.Fatal("--no-override replaced A")
	}
	delete(e.vars, "ENVILY_TOKEN")

	// export to a private file, import back with overwrite.
	exp := filepath.Join(work, "out.env")
	e.must(sub, "", "export", "--out", exp)
	st, _ = os.Stat(exp)
	data, _ := os.ReadFile(exp)
	if st.Mode().Perm() != 0o600 || string(data) != "A=1\nB='two words'\nMULTI=\"line one\\nline two\"\n" {
		t.Fatalf("export %v %q", st.Mode().Perm(), data)
	}
	if out := e.must(sub, "", "export", "--format", "json"); !strings.Contains(out, `"B": "two words"`) {
		t.Fatalf("json export %q", out)
	}
	if err := os.WriteFile(filepath.Join(work, "new.env"), []byte("A=1\nC=3\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if out := e.must(work, "", "import", "new.env", "--mode", "overwrite"); !strings.Contains(out, "1 created, 0 updated, 1 unchanged, 2 deleted") {
		t.Fatalf("import %q", out)
	}
	e.must(work, "D=4\n", "import", "-")

	// A service token works without login or .envily.json: project and the
	// only environment come from the token itself.
	svc := e.api(pat, "POST", "/api/v1/projects/web-app/service-tokens",
		map[string]any{"name": "deploy", "environments": []string{"development"}, "permission": "read"})
	token := svc["token"].(string)
	elsewhere := t.TempDir()
	e.vars["XDG_CONFIG_HOME"] = t.TempDir() // no stored credentials
	e.vars["ENVILY_URL"], e.vars["ENVILY_TOKEN"] = e.url, token
	e.must(elsewhere, "", "run", "--", "env")
	if m := envMap(e.execed); m["C"] != "3" || m["D"] != "4" || m["ENVILY_TOKEN"] != "" {
		t.Fatalf("service token run env %v", m)
	}
	if out := e.must(elsewhere, "", "whoami"); !strings.Contains(out, `service token "deploy"`) {
		t.Fatalf("whoami %q", out)
	}
	e.wantExit(ExitNotFound, elsewhere, "run", "--env", "production", "--", "env")
	e.wantExit(ExitForbidden, elsewhere, "secrets", "set", "X=1")
	e.api(pat, "DELETE", "/api/v1/projects/web-app/service-tokens/"+svc["info"].(map[string]any)["id"].(string), nil)
	e.wantExit(ExitUnauthorized, elsewhere, "run", "--", "env")
	delete(e.vars, "ENVILY_TOKEN")
	delete(e.vars, "ENVILY_URL")
	delete(e.vars, "XDG_CONFIG_HOME")

	// logout revokes the token on the server and forgets it.
	e.must(work, "", "logout")
	e.wantExit(ExitUnauthorized, work, "whoami")
	req, _ := http.NewRequest("GET", e.url+"/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+pat)
	res, err := http.DefaultClient.Do(req)
	if err != nil || res.StatusCode != 401 {
		t.Fatalf("token still valid after logout: %v %v", res.StatusCode, err)
	}
	res.Body.Close()
}

func TestCLIUsageErrors(t *testing.T) {
	e := newCLIEnv(t)
	dir := t.TempDir()
	e.wantExit(ExitUsage, dir, "secrets", "list") // no server
	e.wantExit(ExitUsage, dir, "secrets", "get")  // missing argument
	e.wantExit(ExitUsage, dir, "--bogus-flag")
	e.vars["ENVILY_URL"] = e.url
	e.wantExit(ExitUnauthorized, dir, "secrets", "list") // no token
	e.vars["ENVILY_TOKEN"] = "envily_pat_nope"
	e.wantExit(ExitUsage, dir, "secrets", "list") // no project/env for a personal token
	e.wantExit(ExitUnauthorized, dir, "secrets", "list", "--project", "x", "--env", "y")
	e.wantExit(ExitUsage, dir, "export", "--format", "xml")
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ln.Close() // nothing listens here any more
	e.vars["ENVILY_URL"] = "http://" + ln.Addr().String()
	if msg := e.wantExit(ExitError, dir, "whoami"); !strings.Contains(msg, "cannot reach") {
		t.Fatalf("unreachable message %q", msg)
	}
}

func TestMergeEnv(t *testing.T) {
	got := mergeEnv([]string{"PATH=/bin", "A=old", "ENVILY_TOKEN=t", "B=keep"}, map[string]string{"A": "new", "C": "c"}, false)
	if !slices.Equal(got, []string{"PATH=/bin", "B=keep", "A=new", "C=c"}) {
		t.Fatalf("override: %v", got)
	}
	got = mergeEnv([]string{"A=old"}, map[string]string{"A": "new", "C": "c"}, true)
	if !slices.Equal(got, []string{"A=old", "C=c"}) {
		t.Fatalf("no override: %v", got)
	}
}

func TestReadyURL(t *testing.T) {
	for addr, want := range map[string]string{
		":8080":          "http://127.0.0.1:8080/readyz",
		"0.0.0.0:9000":   "http://127.0.0.1:9000/readyz",
		"[::]:8080":      "http://[::1]:8080/readyz",
		"10.0.0.5:8080":  "http://10.0.0.5:8080/readyz",
		"localhost:8081": "http://localhost:8081/readyz",
	} {
		if got, err := readyURL(addr); err != nil || got != want {
			t.Errorf("readyURL(%q) = %q, %v; want %q", addr, got, err, want)
		}
	}
	if _, err := readyURL("8080"); err == nil {
		t.Error("readyURL accepted an address without a port separator")
	}
}
