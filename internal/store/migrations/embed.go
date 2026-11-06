// Package migrations embeds the goose migrations for each SQL dialect.
package migrations

import (
	"embed"
	"io/fs"
)

//go:embed sqlite/*.sql postgres/*.sql
var files embed.FS

// FS returns the migrations for dialect ("sqlite" or "postgres").
func FS(dialect string) fs.FS {
	sub, err := fs.Sub(files, dialect)
	if err != nil {
		panic(err)
	}
	return sub
}
