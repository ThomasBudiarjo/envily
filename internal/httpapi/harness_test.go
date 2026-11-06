package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/thomasbudiarjo/envily/internal/config"
	"github.com/thomasbudiarjo/envily/internal/crypto"
	"github.com/thomasbudiarjo/envily/internal/service"
	"github.com/thomasbudiarjo/envily/internal/store"
	"github.com/thomasbudiarjo/envily/internal/store/sqlite"
)

// fakeClock is a settable clock shared by the service under test.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time      { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *fakeClock) Add(d time.Duration) { c.mu.Lock(); c.t = c.t.Add(d); c.mu.Unlock() }

type env struct {
	t      *testing.T
	srv    *httptest.Server
	db     store.DB
	dbPath string
	clock  *fakeClock
	logs   *bytes.Buffer
	logMu  *sync.Mutex
	origin string
}

type lockedWriter struct {
	mu *sync.Mutex
	w  io.Writer
}

func (l lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

type envSettings struct {
	cfg  config.Config
	svc  service.Options
	real bool // keep production rate limits
}

type envOption func(*envSettings)

func withSetupToken(tok string) envOption {
	return func(s *envSettings) { s.cfg.SetupToken, s.svc.SetupToken = tok, tok }
}

// withRealLimits keeps the production rate limits; other tests raise them
// because every test client shares one IP.
func withRealLimits() envOption { return func(s *envSettings) { s.real = true } }

// newEnv starts the full API on a temporary SQLite database.
func newEnv(t *testing.T, opts ...envOption) *env {
	t.Helper()
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "envily.db")
	d, err := sqlite.Open(ctx, dbPath, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	kr, err := crypto.NewKeyring(crypto.NewKey(), 1)
	if err != nil {
		t.Fatal(err)
	}
	e := &env{t: t, db: d, dbPath: dbPath, clock: &fakeClock{t: time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC)},
		logs: &bytes.Buffer{}, logMu: &sync.Mutex{}}
	log := slog.New(slog.NewJSONHandler(lockedWriter{e.logMu, e.logs}, &slog.HandlerOptions{Level: slog.LevelDebug}))

	srv := httptest.NewUnstartedServer(nil)
	srv.Start()
	t.Cleanup(srv.Close)
	e.srv, e.origin = srv, srv.URL
	u, _ := url.Parse(srv.URL)
	cfg := config.Config{PublicURL: u, SessionIdleTimeout: 12 * time.Hour, SessionMaxAge: 168 * time.Hour}
	so := service.Options{SessionIdleTimeout: cfg.SessionIdleTimeout, SessionMaxAge: cfg.SessionMaxAge,
		Clock: e.clock, Logger: log}
	st := envSettings{cfg: cfg, svc: so}
	for _, o := range opts {
		o(&st)
	}
	svc := service.New(d, kr, st.svc)
	h := New(Deps{Config: st.cfg, Logger: log, UI: fstest.MapFS{}, Service: svc})
	if !st.real {
		h.limits.loginIP = newLimiterSet(1000, 1000)
		h.limits.loginEmail = newLimiterSet(1000, 1000)
		h.limits.api = newLimiterSet(10000, 10000)
	}
	srv.Config.Handler = h
	return e
}

func (e *env) logOutput() string { e.logMu.Lock(); defer e.logMu.Unlock(); return e.logs.String() }

// client is a browser-like (cookie) or bearer API client.
type client struct {
	e      *env
	hc     *http.Client
	bearer string
	web    bool // send Origin and X-Envily-Client like the UI
}

func (e *env) browser() *client {
	jar, _ := cookiejar.New(nil)
	return &client{e: e, hc: &http.Client{Jar: jar}, web: true}
}

func (e *env) bearer(token string) *client {
	return &client{e: e, hc: &http.Client{}, bearer: token}
}

type resp struct {
	t      *testing.T
	status int
	header http.Header
	body   []byte
}

func (r resp) json() map[string]any {
	r.t.Helper()
	var m map[string]any
	if err := json.Unmarshal(r.body, &m); err != nil {
		r.t.Fatalf("decode %q: %v", r.body, err)
	}
	return m
}

func (r resp) code() string {
	m := r.json()
	e, _ := m["error"].(map[string]any)
	c, _ := e["code"].(string)
	return c
}

func (r resp) want(status int) resp {
	r.t.Helper()
	if r.status != status {
		r.t.Fatalf("status %d, want %d: %s", r.status, status, r.body)
	}
	return r
}

func (c *client) do(method, path string, body any, headers ...string) resp {
	c.e.t.Helper()
	var rd io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		rd = bytes.NewBufferString(b)
	default:
		raw, _ := json.Marshal(b)
		rd = bytes.NewReader(raw)
	}
	req, _ := http.NewRequest(method, c.e.srv.URL+path, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.web {
		req.Header.Set("Origin", c.e.origin)
		req.Header.Set("X-Envily-Client", "web")
	}
	if c.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearer)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		if headers[i+1] == "" {
			req.Header.Del(headers[i])
		} else {
			req.Header.Set(headers[i], headers[i+1])
		}
	}
	res, err := c.hc.Do(req)
	if err != nil {
		c.e.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return resp{t: c.e.t, status: res.StatusCode, header: res.Header, body: b}
}

const adminPassword = "correct horse battery staple"

// setupAdmin completes setup and returns a signed-in browser.
func (e *env) setupAdmin() *client {
	e.t.Helper()
	c := e.browser()
	c.do("POST", "/api/v1/setup", map[string]string{"email": "Admin@Example.com", "name": "Admin", "password": adminPassword}).
		want(http.StatusCreated)
	return c
}

// login signs a new browser in.
func (e *env) login(email, password string) *client {
	e.t.Helper()
	c := e.browser()
	c.do("POST", "/api/v1/auth/login", map[string]string{"email": email, "password": password}).want(http.StatusOK)
	return c
}

// newUser creates a user via the admin and returns a browser signed in
// with a permanent password.
func (e *env) newUser(admin *client, email string, isAdmin bool) *client {
	e.t.Helper()
	r := admin.do("POST", "/api/v1/users", map[string]any{"email": email, "name": email, "isAdmin": isAdmin}).want(http.StatusCreated)
	tmp := r.json()["temporaryPassword"].(string)
	c := e.login(email, tmp)
	c.do("POST", "/api/v1/auth/password", map[string]string{"currentPassword": tmp, "newPassword": adminPassword}).
		want(http.StatusNoContent)
	return c
}

func (e *env) auditActions() []string {
	e.t.Helper()
	rows, _, err := e.db.Audit().List(context.Background(), store.AuditFilter{Limit: 1000})
	if err != nil {
		e.t.Fatal(err)
	}
	out := make([]string, len(rows))
	for i, r := range rows {
		out[len(rows)-1-i] = r.Action // oldest first
	}
	return out
}

func auditFilterAll() store.AuditFilter { return store.AuditFilter{Limit: 100000} }
