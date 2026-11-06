package crypto

import (
	"bytes"
	"errors"
	"strconv"
)

const (
	keyCheckPlaintext = "envily-key-check-v1"
	keyCheckAAD       = "meta:key_check"
)

// Keyring holds the master key and the root key version that data
// encryption keys are currently wrapped with.
type Keyring struct {
	master  []byte
	version int64
}

func NewKeyring(master []byte, rootVersion int64) (*Keyring, error) {
	if len(master) != KeySize {
		return nil, errors.New("master key must be 32 bytes")
	}
	if rootVersion < 1 {
		return nil, errors.New("root key version must be at least 1")
	}
	return &Keyring{master: bytes.Clone(master), version: rootVersion}, nil
}

// RootVersion is the version new DEKs are wrapped under.
func (k *Keyring) RootVersion() int64 { return k.version }

// KeyCheck returns the value stored in meta.key_check.
func (k *Keyring) KeyCheck() ([]byte, error) {
	return Seal(k.master, []byte(keyCheckPlaintext), []byte(keyCheckAAD))
}

// VerifyKeyCheck reports ErrKeyMismatch unless sealed was produced by
// KeyCheck with this master key.
func (k *Keyring) VerifyKeyCheck(sealed []byte) error {
	plain, err := Open(k.master, sealed, []byte(keyCheckAAD))
	if err != nil || string(plain) != keyCheckPlaintext {
		return ErrKeyMismatch
	}
	return nil
}

func dekAAD(projectID string, rootVersion int64) []byte {
	return []byte("dek:" + projectID + ":" + strconv.FormatInt(rootVersion, 10))
}

// NewDEK creates a data encryption key for a project and returns it wrapped
// with the master key under the current root version.
func (k *Keyring) NewDEK(projectID string) (wrapped []byte, rootVersion int64, err error) {
	dek := NewKey()
	defer Zero(dek)
	return k.WrapDEK(projectID, dek)
}

// WrapDEK wraps dek for projectID under the current root version.
func (k *Keyring) WrapDEK(projectID string, dek []byte) (wrapped []byte, rootVersion int64, err error) {
	wrapped, err = Seal(k.master, dek, dekAAD(projectID, k.version))
	return wrapped, k.version, err
}

// UnwrapDEK opens a project's wrapped DEK. Callers should Zero the result
// when they are done with it.
func (k *Keyring) UnwrapDEK(projectID string, rootVersion int64, wrapped []byte) ([]byte, error) {
	dek, err := Open(k.master, wrapped, dekAAD(projectID, rootVersion))
	if err != nil {
		return nil, err
	}
	if len(dek) != KeySize {
		Zero(dek)
		return nil, ErrDecrypt
	}
	return dek, nil
}
