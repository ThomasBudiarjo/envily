package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/thomasbudiarjo/envily/internal/config"
	"github.com/thomasbudiarjo/envily/internal/crypto"
	"github.com/thomasbudiarjo/envily/internal/store"
	"github.com/thomasbudiarjo/envily/internal/store/postgres"
	"github.com/thomasbudiarjo/envily/internal/store/sqlite"
)

// OpenDB opens the configured database, running migrations when migrate is set.
func OpenDB(ctx context.Context, cfg config.Config, migrate bool, log *slog.Logger) (store.DB, error) {
	switch cfg.DBDriver {
	case "postgres":
		return postgres.Open(ctx, cfg.DBDSN, migrate, log)
	default:
		return sqlite.Open(ctx, cfg.DBDSN, migrate, log)
	}
}

// Keyring verifies the master key against the database and returns the
// keyring to use. On first start (no key_check yet) it records key_check and
// root key version 1.
func Keyring(ctx context.Context, s store.Store, master []byte) (*crypto.Keyring, error) {
	stored, err := s.Meta().Get(ctx, store.MetaKeyCheck)
	if errors.Is(err, store.ErrNotFound) {
		kr, err := crypto.NewKeyring(master, 1)
		if err != nil {
			return nil, err
		}
		check, err := kr.KeyCheck()
		if err != nil {
			return nil, err
		}
		if err := s.WithTx(ctx, func(tx store.Store) error {
			if err := tx.Meta().Set(ctx, store.MetaRootKeyVersion, "1"); err != nil {
				return err
			}
			return tx.Meta().Set(ctx, store.MetaKeyCheck, base64.StdEncoding.EncodeToString(check))
		}); err != nil {
			return nil, fmt.Errorf("initialise key check: %w", err)
		}
		// Re-read in case another instance initialised the database at the same time.
		stored, err = s.Meta().Get(ctx, store.MetaKeyCheck)
		if err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, fmt.Errorf("read key check: %w", err)
	}

	rawVersion, err := s.Meta().Get(ctx, store.MetaRootKeyVersion)
	if err != nil {
		return nil, fmt.Errorf("read root key version: %w", err)
	}
	version, err := strconv.ParseInt(rawVersion, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid root key version in database")
	}
	kr, err := crypto.NewKeyring(master, version)
	if err != nil {
		return nil, err
	}
	sealed, err := base64.StdEncoding.DecodeString(stored)
	if err != nil {
		return nil, crypto.ErrKeyMismatch
	}
	if err := kr.VerifyKeyCheck(sealed); err != nil {
		return nil, err
	}
	return kr, nil
}

// RotateMasterKey re-wraps every project's data encryption key with
// newMaster under the next root key version and rewrites key_check, all in
// one transaction. Secret values are not re-encrypted: their DEKs do not
// change. The server must be stopped while this runs. It returns the number
// of projects re-wrapped and the new root key version.
func RotateMasterKey(ctx context.Context, s store.Store, oldMaster, newMaster []byte) (projects int, rootVersion int64, err error) {
	if bytes.Equal(oldMaster, newMaster) {
		return 0, 0, errors.New("the new master key is the same as the current one")
	}
	old, err := Keyring(ctx, s, oldMaster)
	if err != nil {
		return 0, 0, err
	}
	next, err := crypto.NewKeyring(newMaster, old.RootVersion()+1)
	if err != nil {
		return 0, 0, err
	}
	check, err := next.KeyCheck()
	if err != nil {
		return 0, 0, err
	}
	err = s.WithTx(ctx, func(tx store.Store) error {
		list, err := tx.Projects().List(ctx)
		if err != nil {
			return err
		}
		for _, p := range list {
			dek, err := old.UnwrapDEK(p.ID, p.DEKRootVersion, p.WrappedDEK)
			if err != nil {
				// Name the project, never the key material.
				return fmt.Errorf("project %s: cannot unwrap its data key with the current master key", p.Slug)
			}
			wrapped, version, err := next.WrapDEK(p.ID, dek)
			crypto.Zero(dek)
			if err != nil {
				return err
			}
			if err := tx.Projects().UpdateDEK(ctx, p.ID, wrapped, version); err != nil {
				return err
			}
		}
		projects = len(list)
		if err := tx.Meta().Set(ctx, store.MetaRootKeyVersion, strconv.FormatInt(next.RootVersion(), 10)); err != nil {
			return err
		}
		return tx.Meta().Set(ctx, store.MetaKeyCheck, base64.StdEncoding.EncodeToString(check))
	})
	if err != nil {
		return 0, 0, err
	}
	return projects, next.RootVersion(), nil
}
