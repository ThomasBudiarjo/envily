package crypto

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestSealOpenRoundTrip(t *testing.T) {
	key := NewKey()
	for _, plain := range [][]byte{{}, []byte("x"), bytes.Repeat([]byte("secret"), 20000)} {
		sealed, err := Seal(key, plain, []byte("aad"))
		if err != nil {
			t.Fatal(err)
		}
		if sealed[0] != 0x01 || len(sealed) != 1+24+len(plain)+16 {
			t.Fatalf("unexpected format: first byte %#x, len %d", sealed[0], len(sealed))
		}
		got, err := Open(key, sealed, []byte("aad"))
		if err != nil || !bytes.Equal(got, plain) {
			t.Fatalf("round trip failed: %v", err)
		}
	}
}

func TestOpenRejectsEveryTamperedBit(t *testing.T) {
	key := NewKey()
	sealed, err := Seal(key, []byte("hunter2-value"), []byte("secret:abc:1"))
	if err != nil {
		t.Fatal(err)
	}
	for i := range sealed {
		for bit := 0; bit < 8; bit++ {
			tampered := bytes.Clone(sealed)
			tampered[i] ^= 1 << bit
			if _, err := Open(key, tampered, []byte("secret:abc:1")); !errors.Is(err, ErrDecrypt) {
				t.Fatalf("byte %d bit %d: tampered value opened (err=%v)", i, bit, err)
			}
		}
	}
}

func TestOpenRejectsWrongKeyAADAndFormat(t *testing.T) {
	key := NewKey()
	sealed, _ := Seal(key, []byte("v"), SecretAAD("s1", 1))

	if _, err := Open(NewKey(), sealed, SecretAAD("s1", 1)); !errors.Is(err, ErrDecrypt) {
		t.Error("opened with wrong key")
	}
	for _, aad := range [][]byte{SecretAAD("s2", 1), SecretAAD("s1", 2), CommentAAD("s1", 1), nil} {
		if _, err := Open(key, sealed, aad); !errors.Is(err, ErrDecrypt) {
			t.Errorf("opened with aad %q", aad)
		}
	}
	unknown := bytes.Clone(sealed)
	unknown[0] = 0x02
	if _, err := Open(key, unknown, SecretAAD("s1", 1)); !errors.Is(err, ErrDecrypt) {
		t.Error("opened unknown format version")
	}
	if _, err := Open(key, sealed[:20], SecretAAD("s1", 1)); !errors.Is(err, ErrDecrypt) {
		t.Error("opened truncated value")
	}
}

func TestSealUsesFreshNonces(t *testing.T) {
	key := NewKey()
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		sealed, _ := Seal(key, []byte("same"), nil)
		nonce := string(sealed[1:25])
		if seen[nonce] {
			t.Fatal("nonce reused")
		}
		seen[nonce] = true
	}
}

func TestKeyring(t *testing.T) {
	master := NewKey()
	kr, err := NewKeyring(master, 1)
	if err != nil {
		t.Fatal(err)
	}
	check, err := kr.KeyCheck()
	if err != nil {
		t.Fatal(err)
	}
	if err := kr.VerifyKeyCheck(check); err != nil {
		t.Fatalf("own key check: %v", err)
	}
	other, _ := NewKeyring(NewKey(), 1)
	if err := other.VerifyKeyCheck(check); !errors.Is(err, ErrKeyMismatch) {
		t.Fatalf("other key accepted: %v", err)
	}

	wrapped, ver, err := kr.NewDEK("proj-1")
	if err != nil || ver != 1 {
		t.Fatalf("NewDEK: %v ver=%d", err, ver)
	}
	dek, err := kr.UnwrapDEK("proj-1", 1, wrapped)
	if err != nil || len(dek) != KeySize {
		t.Fatalf("unwrap: %v", err)
	}
	if _, err := kr.UnwrapDEK("proj-2", 1, wrapped); err == nil {
		t.Error("DEK unwrapped for another project")
	}
	if _, err := kr.UnwrapDEK("proj-1", 2, wrapped); err == nil {
		t.Error("DEK unwrapped under another root version")
	}

	// Rotation: rewrap under a new master key and version.
	next, _ := NewKeyring(NewKey(), 2)
	rewrapped, ver2, err := next.WrapDEK("proj-1", dek)
	if err != nil || ver2 != 2 {
		t.Fatalf("rewrap: %v", err)
	}
	dek2, err := next.UnwrapDEK("proj-1", 2, rewrapped)
	if err != nil || !bytes.Equal(dek, dek2) {
		t.Fatal("rewrapped DEK differs")
	}
	if _, err := kr.UnwrapDEK("proj-1", 2, rewrapped); err == nil {
		t.Error("old master key opened rewrapped DEK")
	}

	if _, err := NewKeyring(make([]byte, 16), 1); err == nil {
		t.Error("short master key accepted")
	}
}

func TestTokens(t *testing.T) {
	tok, hash := NewToken(ServiceTokenPrefix)
	if !strings.HasPrefix(tok, "envily_svc_") || len(tok) != len("envily_svc_")+43 {
		t.Fatalf("token %q", tok)
	}
	if !bytes.Equal(hash, HashToken(tok)) || len(hash) != 32 {
		t.Fatal("hash mismatch")
	}
	tok2, _ := NewToken(ServiceTokenPrefix)
	if tok == tok2 {
		t.Fatal("tokens repeat")
	}
	pw := RandomPassword(20)
	if len(pw) != 20 || strings.ContainsAny(pw, "0O1lI") {
		t.Fatalf("random password %q", pw)
	}
}

func TestPasswordHash(t *testing.T) {
	h, err := HashPassword("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Fatalf("hash format %q", h)
	}
	if ok, err := VerifyPassword("correct horse battery", h); !ok || err != nil {
		t.Fatalf("verify: %v %v", ok, err)
	}
	if ok, _ := VerifyPassword("correct horse batterY", h); ok {
		t.Fatal("wrong password accepted")
	}
	h2, _ := HashPassword("correct horse battery")
	if h == h2 {
		t.Fatal("salt not random")
	}
	for _, bad := range []string{"", "$argon2i$v=19$m=1,t=1,p=1$AA$AA", "$argon2id$v=19$m=99999999,t=1,p=1$AA$AA", h[:len(h)-3] + "!!!"} {
		if _, err := VerifyPassword("x", bad); err == nil {
			t.Errorf("malformed hash %q accepted", bad)
		}
	}
}
