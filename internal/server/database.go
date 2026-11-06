package server

import (
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
