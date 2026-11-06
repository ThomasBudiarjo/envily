package server

import (
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
