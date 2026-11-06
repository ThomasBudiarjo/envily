package httpapi

import (
	"bytes"
	"io/fs"
	"net/http"
	"path"
	"regexp"
	"strings"
)

const placeholderPage = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>Envily</title></head>
<body style="font-family:system-ui,sans-serif;max-width:40rem;margin:4rem auto;padding:0 1rem">
<h1>Envily</h1>
<p>The web UI is not part of this build. Run <code>make build</code> to embed it.</p>
<p>The API is available under <code>/api/v1</code>.</p>
</body></html>`

// spa serves the embedded SvelteKit build: existing files as they are,
// every other path as index.html so that client-side routing works.
type spa struct {
	files fs.FS
	index []byte
}

func newSPA(files fs.FS) *spa {
	index, err := fs.ReadFile(files, "index.html")
	if err != nil {
		index = []byte(placeholderPage)
	}
	return &spa{files: files, index: index}
}

var scriptHash = regexp.MustCompile(`'sha256-[A-Za-z0-9+/=]+'`)

// scriptHashes returns the CSP hashes SvelteKit computed for the inline
// bootstrap script, so the CSP header can allow exactly that script.
func (s *spa) scriptHashes() []string {
	var out []string
	for _, m := range scriptHash.FindAll(s.index, -1) {
		out = append(out, string(m))
	}
	return out
}

func (s *spa) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name != "" && name != "index.html" {
		if st, err := fs.Stat(s.files, name); err == nil && !st.IsDir() {
			if strings.HasPrefix(name, "_app/immutable/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				w.Header().Set("Cache-Control", "no-cache")
			}
			http.ServeFileFS(w, r, s.files, name) //nolint:gosec // fs.FS rejects paths outside its root
			return
		}
		// Unknown asset paths are real 404s; anything else is a client route.
		if strings.HasPrefix(name, "_app/") || path.Ext(name) != "" {
			http.NotFound(w, r)
			return
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, "index.html", startTime, bytes.NewReader(s.index))
}
