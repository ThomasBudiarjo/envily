// Package crypto holds every cryptographic operation in Envily: AEAD
// sealing (XChaCha20-Poly1305), the master-key keyring that wraps per-project
// data encryption keys, token generation, and password hashing (argon2id).
// No other package may use AEAD primitives directly.
package crypto

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"

	"golang.org/x/crypto/chacha20poly1305"
)

// KeySize is the size of the master key and of every data encryption key.
const KeySize = chacha20poly1305.KeySize

// formatV1 is the first byte of every sealed value: 0x01 || nonce || ciphertext+tag.
const formatV1 byte = 0x01

var (
	// ErrDecrypt is returned for any sealed value that fails to open: wrong
	// key, wrong AAD, tampered bytes, or an unknown format.
	ErrDecrypt = errors.New("decryption failed")
	// ErrKeyMismatch means the master key is not the one this database was
	// initialised with.
	ErrKeyMismatch = errors.New("master key does not match this database")
)

// NewKey returns a fresh random 32-byte key.
func NewKey() []byte {
	k := make([]byte, KeySize)
	if _, err := rand.Read(k); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return k
}

// EncodeKey returns key as standard base64, the format of ENVILY_MASTER_KEY.
func EncodeKey(key []byte) string { return base64.StdEncoding.EncodeToString(key) }

// Seal encrypts plaintext with key, binding it to aad.
func Seal(key, plaintext, aad []byte) ([]byte, error) {
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, fmt.Errorf("seal: %w", err)
	}
	out := make([]byte, 1+aead.NonceSize(), 1+aead.NonceSize()+len(plaintext)+aead.Overhead())
	out[0] = formatV1
	if _, err := rand.Read(out[1:]); err != nil {
		return nil, fmt.Errorf("seal: reading nonce: %w", err)
	}
	return aead.Seal(out, out[1:], plaintext, aad), nil
}

// Open decrypts a value produced by Seal with the same key and aad.
func Open(key, sealed, aad []byte) ([]byte, error) {
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, ErrDecrypt
	}
	if len(sealed) < 1+aead.NonceSize()+aead.Overhead() || sealed[0] != formatV1 {
		return nil, ErrDecrypt
	}
	nonce := sealed[1 : 1+aead.NonceSize()]
	plain, err := aead.Open(nil, nonce, sealed[1+aead.NonceSize():], aad)
	if err != nil {
		return nil, ErrDecrypt
	}
	return plain, nil
}

// Zero overwrites b. It is best effort: the Go runtime may have copied it.
func Zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// SecretAAD binds a secret value to its secret row and version.
func SecretAAD(secretID string, version int64) []byte {
	return []byte("secret:" + secretID + ":" + strconv.FormatInt(version, 10))
}

// CommentAAD binds a secret comment to its secret row and version.
func CommentAAD(secretID string, version int64) []byte {
	return []byte("comment:" + secretID + ":" + strconv.FormatInt(version, 10))
}
