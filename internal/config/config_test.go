package config

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var testKey = base64.StdEncoding.EncodeToString(make([]byte, 32))

func lookup(m map[string]string) Lookup {
	return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
}

func valid() map[string]string {
	return map[string]string{
		"ENVILY_PUBLIC_URL": "https://secrets.example.com",
		"ENVILY_MASTER_KEY": testKey,
	}
}

func TestLoadDefaults(t *testing.T) {
	c, err := Load(lookup(valid()))
	if err != nil {
		t.Fatal(err)
	}
	if c.Addr != ":8080" || c.DBDriver != "sqlite" || c.DBDSN != "/data/envily.db" || !c.AutoMigrate {
		t.Errorf("unexpected defaults: %+v", c)
	}
	if c.SessionIdleTimeout != 12*time.Hour || c.SessionMaxAge != 168*time.Hour || c.AuditRetentionDays != 365 {
		t.Errorf("unexpected session/audit defaults: %+v", c)
	}
	if !c.SecureCookies() || c.Origin() != "https://secrets.example.com" {
		t.Errorf("SecureCookies=%v Origin=%q", c.SecureCookies(), c.Origin())
	}
}

func TestLoadRequiresMasterKeyAndPublicURL(t *testing.T) {
	_, err := Load(lookup(map[string]string{}))
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{"ENVILY_PUBLIC_URL is required", "ENVILY_MASTER_KEY or ENVILY_MASTER_KEY_FILE is required"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestMasterKeyValidation(t *testing.T) {
	short := base64.StdEncoding.EncodeToString(make([]byte, 16))
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "key")
	if err := os.WriteFile(keyFile, []byte(testKey+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name    string
		env     map[string]string
		wantErr string
	}{
		{"wrong length", map[string]string{"ENVILY_MASTER_KEY": short}, "must decode to 32 bytes, got 16"},
		{"not base64", map[string]string{"ENVILY_MASTER_KEY": "%%%not-base64"}, "is not valid standard base64"},
		{"both sources", map[string]string{"ENVILY_MASTER_KEY": testKey, "ENVILY_MASTER_KEY_FILE": keyFile}, "set only one of"},
		{"missing file", map[string]string{"ENVILY_MASTER_KEY_FILE": filepath.Join(dir, "nope")}, "cannot read key file"},
		{"file ok", map[string]string{"ENVILY_MASTER_KEY_FILE": keyFile}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := map[string]string{"ENVILY_PUBLIC_URL": "http://localhost:8080"}
			for k, v := range tc.env {
				env[k] = v
			}
			c, err := Load(lookup(env))
			if tc.wantErr == "" {
				if err != nil || len(c.MasterKey) != 32 {
					t.Fatalf("err=%v len=%d", err, len(c.MasterKey))
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("got %v, want error containing %q", err, tc.wantErr)
			}
			if v := tc.env["ENVILY_MASTER_KEY"]; v != "" && strings.Contains(err.Error(), v) {
				t.Fatalf("error echoes the key value: %v", err)
			}
		})
	}
}

func TestInvalidValuesAreNamed(t *testing.T) {
	env := valid()
	env["ENVILY_PUBLIC_URL"] = "secrets.example.com"
	env["ENVILY_DB_DRIVER"] = "mysql"
	env["ENVILY_SESSION_IDLE_TIMEOUT"] = "soon"
	env["ENVILY_TRUSTED_PROXIES"] = "10.0.0.0/8, nonsense"
	env["ENVILY_LOG_LEVEL"] = "loud"
	_, err := Load(lookup(env))
	for _, name := range []string{"ENVILY_PUBLIC_URL", "ENVILY_DB_DRIVER", "ENVILY_SESSION_IDLE_TIMEOUT", "ENVILY_TRUSTED_PROXIES", "ENVILY_LOG_LEVEL"} {
		if err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("error %v does not name %s", err, name)
		}
	}
}

func TestTrustedProxies(t *testing.T) {
	env := valid()
	env["ENVILY_TRUSTED_PROXIES"] = "10.0.0.0/8, 192.168.1.7 ,::1"
	c, err := Load(lookup(env))
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, p := range c.TrustedProxies {
		got = append(got, p.String())
	}
	if strings.Join(got, ",") != "10.0.0.0/8,192.168.1.7/32,::1/128" {
		t.Fatalf("got %v", got)
	}
}
