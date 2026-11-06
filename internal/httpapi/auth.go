package httpapi

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/thomasbudiarjo/envily/internal/authz"
	"github.com/thomasbudiarjo/envily/internal/service"
)

const (
	sessionCookie = "envily_session"
	clientHeader  = "X-Envily-Client"
)

type principalKey struct{}

// principal returns the authenticated caller, or nil for anonymous requests.
func principal(r *http.Request) *authz.Principal {
	p, _ := r.Context().Value(principalKey{}).(*authz.Principal)
	return p
}

// passwordChangeAllowed lists what a user with a temporary password may call.
var passwordChangeAllowed = map[string]bool{
	"GET /api/v1/auth/me":        true,
	"POST /api/v1/auth/password": true,
	"POST /api/v1/auth/logout":   true,
}

func unsafeMethod(m string) bool {
	return m != http.MethodGet && m != http.MethodHead && m != http.MethodOptions
}

// authenticate resolves the session cookie or bearer token into a principal
// and enforces CSRF and rate limits for /api requests.
func (h *Handler) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		ip := h.clientIP(r)
		ctx := service.WithRequestMeta(r.Context(), service.RequestMeta{IP: ip, UserAgent: r.UserAgent()})

		var p *authz.Principal
		bearer, hasBearer := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if hasBearer {
			var err error
			if p, err = h.svc.AuthenticateBearer(ctx, strings.TrimSpace(bearer)); err != nil {
				h.fail(w, r, err)
				return
			}
		} else if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
			// A stale cookie is treated as anonymous; endpoints that need a
			// user then answer 401.
			if sp, err := h.svc.AuthenticateSession(ctx, c.Value); err == nil {
				p = sp
			} else if service.AsError(err) == nil {
				h.fail(w, r, err)
				return
			}
		}

		if !hasBearer && unsafeMethod(r.Method) && !h.sameOrigin(r) {
			writeError(w, r, errCSRF)
			return
		}

		key := "ip:" + ip
		if p != nil {
			key = string(p.Type) + ":" + p.ID
			info(ctx).principalID = key
		}
		if ok, wait := h.limits.api.allow(key); !ok {
			w.Header().Set("Retry-After", retryAfter(wait))
			h.fail(w, r, &service.Error{Code: service.CodeRateLimited, Message: "too many requests"})
			return
		}

		if p != nil && p.MustChangePassword && !passwordChangeAllowed[r.Method+" "+r.URL.Path] {
			h.fail(w, r, &service.Error{Code: service.CodePasswordChangeRequired, Message: "you must change your password first"})
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(ctx, principalKey{}, p)))
	})
}

// sameOrigin implements the CSRF check for requests without a bearer token.
// When a session cookie is present the request must come from the web UI:
// X-Envily-Client: web and an Origin (or Referer) of ENVILY_PUBLIC_URL.
// Without a cookie (login, setup, CLI login) only a present Origin must match,
// which blocks cross-site form posts while letting the CLI log in.
func (h *Handler) sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		if u, err := url.Parse(r.Header.Get("Referer")); err == nil && u.Host != "" {
			origin = u.Scheme + "://" + u.Host
		}
	}
	originOK := origin == h.cfg.Origin()
	if _, err := r.Cookie(sessionCookie); err == nil {
		return originOK && r.Header.Get(clientHeader) == "web"
	}
	return origin == "" || originOK
}

// fail writes err as a JSON error, logging unexpected errors without detail
// that could include request data.
func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	e, expected := toAPIError(err)
	if !expected {
		h.log.Error("request failed", "request_id", RequestID(r.Context()), "method", r.Method,
			"path", r.URL.Path, "error", err)
	}
	writeError(w, r, e)
}

func (h *Handler) setSessionCookie(w http.ResponseWriter, token string) {
	// Secure follows ENVILY_PUBLIC_URL (off only for http:// development URLs).
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // G124: see above
		Name: sessionCookie, Value: token, Path: "/", HttpOnly: true, Secure: h.cfg.SecureCookies(),
		SameSite: http.SameSiteStrictMode, MaxAge: int(h.cfg.SessionMaxAge.Seconds()),
	})
}

func (h *Handler) clearSessionCookie(w http.ResponseWriter) {
	// Secure follows ENVILY_PUBLIC_URL (off only for http:// development URLs).
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // G124: see above
		Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, Secure: h.cfg.SecureCookies(),
		SameSite: http.SameSiteStrictMode, MaxAge: -1,
	})
}

// handlerFunc is an API handler that returns its error instead of writing it.
type handlerFunc func(w http.ResponseWriter, r *http.Request) error

func (h *Handler) api(fn handlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := fn(w, r); err != nil {
			h.fail(w, r, err)
		}
	}
}

// authed requires an authenticated principal.
func (h *Handler) authed(fn handlerFunc) http.HandlerFunc {
	return h.api(func(w http.ResponseWriter, r *http.Request) error {
		if principal(r) == nil {
			return errAuthMissing
		}
		return fn(w, r)
	})
}
