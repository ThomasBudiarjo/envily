package crypto

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

// Token prefixes help secret scanners recognise leaked tokens.
const (
	ServiceTokenPrefix  = "envily_svc_" //nolint:gosec // G101: a prefix, not a credential
	PersonalTokenPrefix = "envily_pat_" //nolint:gosec // G101: a prefix, not a credential
)

// NewToken returns a new random token (32 bytes, base64url) with the given
// prefix, and the SHA-256 hash that is stored instead of the token.
func NewToken(prefix string) (token string, hash []byte) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	token = prefix + base64.RawURLEncoding.EncodeToString(b)
	return token, HashToken(token)
}

// HashToken returns the SHA-256 hash used to look a token up. Tokens have
// 256 bits of entropy, so a slow hash is unnecessary.
func HashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

const passwordAlphabet = "abcdefghjkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// RandomPassword returns a temporary password of n characters drawn
// uniformly from an alphabet without look-alike characters.
func RandomPassword(n int) string {
	out := make([]byte, n)
	buf := make([]byte, 1)
	// 55-letter alphabet; reject bytes >= 220 (4*55) to avoid modulo bias.
	for i := 0; i < n; {
		if _, err := rand.Read(buf); err != nil {
			panic("crypto/rand failed: " + err.Error())
		}
		if buf[0] >= 220 {
			continue
		}
		out[i] = passwordAlphabet[int(buf[0])%len(passwordAlphabet)]
		i++
	}
	return string(out)
}
