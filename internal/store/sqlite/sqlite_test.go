package sqlite_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/thomasbudiarjo/envily/internal/store"
	"github.com/thomasbudiarjo/envily/internal/store/sqlite"
	"github.com/thomasbudiarjo/envily/internal/store/storetest"
)

func open(t *testing.T) store.DB {
	t.Helper()
	d, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "envily.db"), true, nil)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestStore(t *testing.T) { storetest.Run(t, open) }

func TestBackup(t *testing.T) {
	ctx := context.Background()
	d := open(t)
	defer d.Close()
	if err := d.Meta().Set(ctx, store.MetaKeyCheck, "check"); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := d.Users().Create(ctx, store.User{ID: "u1", Email: "a@example.com", Name: "A", PasswordHash: "h",
		CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "backup.db")
	if err := d.Backup(ctx, out); err != nil {
		t.Fatal(err)
	}
	if err := d.Backup(ctx, out); err == nil {
		t.Fatal("backup overwrote an existing file")
	}

	// The snapshot opens on its own (no pending migrations) and holds the data.
	b, err := sqlite.Open(ctx, out, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if v, err := b.Meta().Get(ctx, store.MetaKeyCheck); err != nil || v != "check" {
		t.Fatalf("meta in backup: %q %v", v, err)
	}
	if u, err := b.Users().GetByEmail(ctx, "a@example.com"); err != nil || u.ID != "u1" {
		t.Fatalf("user in backup: %v", err)
	}
}

func TestPragmas(t *testing.T) {
	path := filepath.Join(t.TempDir(), "envily.db")
	d, err := sqlite.Open(context.Background(), path, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	d.Close()
	conn, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var mode string
	if err := conn.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil || mode != "wal" {
		t.Fatalf("journal_mode = %q (%v)", mode, err)
	}
}
