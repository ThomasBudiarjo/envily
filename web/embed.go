// Package web embeds the built SvelteKit app. Run `make web` before
// `go build` to include the UI; without it the server serves a placeholder.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:build
var build embed.FS

// FS returns the contents of web/build.
func FS() fs.FS {
	sub, err := fs.Sub(build, "build")
	if err != nil {
		panic(err)
	}
	return sub
}
