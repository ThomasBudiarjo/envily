package httpapi

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/thomasbudiarjo/envily/internal/config"
)

func newTestHandler(t *testing.T, ui fstest.MapFS) *Handler {
	t.Helper()
	u, _ := url.Parse("https://secrets.example.com")
	return New(Deps{
		Config: config.Config{PublicURL: u},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		UI:     ui,
	})
}

func get(t *testing.T, h http.Handler, path string) *http.Response {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	return rec.Result()
}

func body(t *testing.T, r *http.Response) string {
	t.Helper()
	b, _ := io.ReadAll(r.Body)
	return string(b)
}

func TestHealthz(t *testing.T) {
	r := get(t, newTestHandler(t, fstest.MapFS{}), "/healthz")
	if r.StatusCode != 200 || !strings.Contains(body(t, r), `"ok"`) {
		t.Fatalf("status %d", r.StatusCode)
	}
	if r.Header.Get("X-Request-Id") == "" {
		t.Fatal("missing X-Request-Id")
	}
}

func TestSPA(t *testing.T) {
	ui := fstest.MapFS{
		"index.html":                  {Data: []byte(`<html><meta http-equiv="content-security-policy" content="script-src 'self' 'sha256-abc='">app</html>`)},
		"_app/immutable/entry/app.js": {Data: []byte("js")},
		"favicon.svg":                 {Data: []byte("<svg/>")},
	}
	h := newTestHandler(t, ui)

	r := get(t, h, "/p/demo/production")
	if r.StatusCode != 200 || !strings.Contains(body(t, r), "app") {
		t.Fatalf("client route: status %d", r.StatusCode)
	}
	if r.Header.Get("Cache-Control") != "no-cache" {
		t.Errorf("index cache-control = %q", r.Header.Get("Cache-Control"))
	}
	if csp := r.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "script-src 'self' 'sha256-abc='") || !strings.Contains(csp, "frame-ancestors 'none'") {
		t.Errorf("csp = %q", csp)
	}
	if r.Header.Get("Strict-Transport-Security") == "" || r.Header.Get("X-Frame-Options") != "DENY" {
		t.Errorf("missing security headers: %v", r.Header)
	}

	r = get(t, h, "/_app/immutable/entry/app.js")
	if r.StatusCode != 200 || r.Header.Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Errorf("immutable asset: status %d cache %q", r.StatusCode, r.Header.Get("Cache-Control"))
	}
	if r := get(t, h, "/_app/immutable/missing.js"); r.StatusCode != 404 {
		t.Errorf("missing asset status %d", r.StatusCode)
	}

	r = get(t, h, "/api/v1/nope")
	if r.StatusCode != 404 || !strings.Contains(body(t, r), `"code":"not_found"`) {
		t.Errorf("unknown api path status %d", r.StatusCode)
	}
	if r.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("api cache-control = %q", r.Header.Get("Cache-Control"))
	}
}

func TestPlaceholderWithoutBuild(t *testing.T) {
	r := get(t, newTestHandler(t, fstest.MapFS{}), "/")
	if r.StatusCode != 200 || !strings.Contains(body(t, r), "make build") {
		t.Fatalf("status %d", r.StatusCode)
	}
}
