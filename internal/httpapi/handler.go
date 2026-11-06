// Package httpapi exposes the service layer over HTTP: JSON endpoints under
// /api/v1, health checks, and the embedded single-page app.
package httpapi

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/thomasbudiarjo/envily/internal/config"
	"github.com/thomasbudiarjo/envily/internal/service"
)

var startTime = time.Now()

type Deps struct {
	Config config.Config
	Logger *slog.Logger
	UI     fs.FS
	// Service may be nil in tests that only exercise static routes.
	Service *service.Service
	// Ready reports whether the server can serve requests (database
	// reachable, master key valid).
	Ready func(ctx context.Context) error
}

type Handler struct {
	cfg    config.Config
	log    *slog.Logger
	ready  func(ctx context.Context) error
	csp    string
	root   http.Handler
	svc    *service.Service
	limits *limits
}

func New(d Deps) *Handler {
	h := &Handler{cfg: d.Config, log: d.Logger, ready: d.Ready, svc: d.Service, limits: newLimits()}
	ui := newSPA(d.UI)
	h.csp = buildCSP(ui.scriptHashes())

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /readyz", h.readyz)
	if h.svc != nil {
		h.routes(mux)
	}
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, r, errNotFound)
	})
	mux.Handle("/", ui)

	var root http.Handler = mux
	if h.svc != nil {
		root = h.authenticate(root)
	}
	root = bodyLimit(root)
	root = h.accessLog(root)
	root = h.securityHeaders(root)
	root = requestID(root)
	root = h.recoverer(root)
	h.root = root
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.root.ServeHTTP(w, r) }

func (h *Handler) readyz(w http.ResponseWriter, r *http.Request) {
	if h.ready != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if err := h.ready(ctx); err != nil {
			h.log.Warn("readiness check failed", "error", err)
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func buildCSP(hashes []string) string {
	script := "script-src 'self'"
	if len(hashes) > 0 {
		script += " " + strings.Join(hashes, " ")
	}
	return strings.Join([]string{
		"default-src 'self'",
		script,
		"style-src 'self' 'unsafe-inline'",
		"img-src 'self' data:",
		"font-src 'self'",
		"connect-src 'self'",
		"frame-ancestors 'none'",
		"base-uri 'self'",
		"form-action 'self'",
	}, "; ")
}

func (h *Handler) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/setup/status", h.api(h.setupStatus))
	mux.HandleFunc("POST /api/v1/setup", h.api(h.setup))
	mux.HandleFunc("POST /api/v1/auth/login", h.api(h.login))
	mux.HandleFunc("POST /api/v1/auth/cli-login", h.api(h.cliLogin))
	mux.HandleFunc("POST /api/v1/auth/logout", h.authed(h.logout))
	mux.HandleFunc("GET /api/v1/auth/me", h.authed(h.me))
	mux.HandleFunc("POST /api/v1/auth/password", h.authed(h.changePassword))
	mux.HandleFunc("GET /api/v1/me/tokens", h.authed(h.listMyTokens))
	mux.HandleFunc("POST /api/v1/me/tokens", h.authed(h.createMyToken))
	mux.HandleFunc("DELETE /api/v1/me/tokens/{id}", h.authed(h.revokeMyToken))
	mux.HandleFunc("GET /api/v1/users", h.authed(h.listUsers))
	mux.HandleFunc("POST /api/v1/users", h.authed(h.createUser))
	mux.HandleFunc("PATCH /api/v1/users/{id}", h.authed(h.updateUser))
	mux.HandleFunc("GET /api/v1/audit", h.authed(h.instanceAudit))
}
