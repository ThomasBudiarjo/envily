package server

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/thomasbudiarjo/envily/internal/config"
	"github.com/thomasbudiarjo/envily/internal/crypto"
	"github.com/thomasbudiarjo/envily/internal/store"
)

func TestKeyring(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "envily.db")
	d, err := OpenDB(ctx, config.Config{DBDriver: "sqlite", DBDSN: path}, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if st, err := os.Stat(path); err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("database file mode %v (%v)", st.Mode().Perm(), err)
	}

	key := crypto.NewKey()
	kr, err := Keyring(ctx, d, key)
	if err != nil {
		t.Fatal(err)
	}
	if kr.RootVersion() != 1 {
		t.Fatalf("root version %d", kr.RootVersion())
	}
	check, err := d.Meta().Get(ctx, store.MetaKeyCheck)
	if err != nil || check == "" {
		t.Fatalf("key_check not written: %v", err)
	}

	// Same key on the next start verifies without rewriting key_check.
	if _, err := Keyring(ctx, d, key); err != nil {
		t.Fatal(err)
	}
	if again, _ := d.Meta().Get(ctx, store.MetaKeyCheck); again != check {
		t.Fatal("key_check rewritten on second start")
	}

	// Another key is refused.
	if _, err := Keyring(ctx, d, crypto.NewKey()); !errors.Is(err, crypto.ErrKeyMismatch) {
		t.Fatalf("wrong key: %v", err)
	}
}

func TestRotateMasterKey(t *testing.T) {
	ctx := context.Background()
	d, err := OpenDB(ctx, config.Config{DBDriver: "sqlite", DBDSN: filepath.Join(t.TempDir(), "envily.db")}, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	oldKey, newKey := crypto.NewKey(), crypto.NewKey()
	kr, err := Keyring(ctx, d, oldKey)
	if err != nil {
		t.Fatal(err)
	}
	// Two projects with known DEKs.
	deks := map[string][]byte{}
	for _, id := range []string{"p1", "p2"} {
		dek := crypto.NewKey()
		wrapped, v, err := kr.WrapDEK(id, dek)
		if err != nil {
			t.Fatal(err)
		}
		if err := d.Projects().Create(ctx, store.Project{ID: id, Name: id, Slug: id, WrappedDEK: wrapped, DEKRootVersion: v}); err != nil {
			t.Fatal(err)
		}
		deks[id] = dek
	}

	// A wrong current key changes nothing.
	if _, _, err := RotateMasterKey(ctx, d, crypto.NewKey(), newKey); !errors.Is(err, crypto.ErrKeyMismatch) {
		t.Fatalf("wrong old key: %v", err)
	}
	if _, _, err := RotateMasterKey(ctx, d, oldKey, oldKey); err == nil {
		t.Fatal("rotating to the same key must fail")
	}

	n, version, err := RotateMasterKey(ctx, d, oldKey, newKey)
	if err != nil || n != 2 || version != 2 {
		t.Fatalf("rotate: n=%d version=%d err=%v", n, version, err)
	}

	// The old key no longer opens the database; the new one does, and every
	// project's DEK (and so every secret) is unchanged.
	if _, err := Keyring(ctx, d, oldKey); !errors.Is(err, crypto.ErrKeyMismatch) {
		t.Fatalf("old key after rotation: %v", err)
	}
	nk, err := Keyring(ctx, d, newKey)
	if err != nil || nk.RootVersion() != 2 {
		t.Fatalf("new key: version=%v err=%v", nk, err)
	}
	for id, want := range deks {
		p, err := d.Projects().Get(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		got, err := nk.UnwrapDEK(p.ID, p.DEKRootVersion, p.WrappedDEK)
		if err != nil || !bytes.Equal(got, want) || p.DEKRootVersion != 2 {
			t.Fatalf("%s: DEK changed or unreadable (version %d, err %v)", id, p.DEKRootVersion, err)
		}
	}

	// A project whose DEK cannot be unwrapped aborts the whole rotation.
	if err := d.Projects().UpdateDEK(ctx, "p2", []byte("garbage"), 2); err != nil {
		t.Fatal(err)
	}
	if _, _, err := RotateMasterKey(ctx, d, newKey, crypto.NewKey()); err == nil {
		t.Fatal("rotation with a broken DEK must fail")
	}
	p1, _ := d.Projects().Get(ctx, "p1")
	if p1.DEKRootVersion != 2 {
		t.Fatalf("p1 was re-wrapped by a failed rotation (version %d)", p1.DEKRootVersion)
	}
	if _, err := Keyring(ctx, d, newKey); err != nil {
		t.Fatalf("failed rotation changed key_check: %v", err)
	}
}
