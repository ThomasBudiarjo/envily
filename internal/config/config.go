// Package config reads and validates server configuration from environment
// variables (optionally overridden by command-line flags).
package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr               string
	PublicURL          *url.URL
	DBDriver           string
	DBDSN              string
	AutoMigrate        bool
	MasterKey          []byte
	SetupToken         string
	SessionIdleTimeout time.Duration
	SessionMaxAge      time.Duration
	TrustedProxies     []netip.Prefix
	AuditRetentionDays int
	LogLevel           slog.Level
	LogFormat          string
}

// SecureCookies reports whether cookies must carry the Secure attribute.
func (c Config) SecureCookies() bool { return c.PublicURL.Scheme == "https" }

// Origin is the scheme://host[:port] browsers send in the Origin header.
func (c Config) Origin() string { return c.PublicURL.Scheme + "://" + c.PublicURL.Host }

// Lookup returns the value of a configuration variable and whether it is set.
type Lookup func(name string) (string, bool)

// Env looks variables up in the process environment.
func Env(name string) (string, bool) { return os.LookupEnv(name) }

// Load reads every variable through lookup and validates it. Errors name the
// offending variable but never echo secret values.
func Load(lookup Lookup) (Config, error) {
	get := func(name, def string) string {
		if v, ok := lookup(name); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
		return def
	}
	var errs []error
	fail := func(format string, a ...any) { errs = append(errs, fmt.Errorf(format, a...)) }

	c := Config{
		Addr:       get("ENVILY_ADDR", ":8080"),
		DBDriver:   get("ENVILY_DB_DRIVER", "sqlite"),
		DBDSN:      get("ENVILY_DB_DSN", "/data/envily.db"),
		SetupToken: get("ENVILY_SETUP_TOKEN", ""),
		LogFormat:  get("ENVILY_LOG_FORMAT", "json"),
	}

	if raw := get("ENVILY_PUBLIC_URL", ""); raw == "" {
		fail("ENVILY_PUBLIC_URL is required (for example https://secrets.example.com)")
	} else if u, err := url.Parse(raw); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		fail("ENVILY_PUBLIC_URL must be an absolute http:// or https:// URL")
	} else {
		c.PublicURL = u
	}

	if c.DBDriver != "sqlite" && c.DBDriver != "postgres" {
		fail("ENVILY_DB_DRIVER must be sqlite or postgres")
	}
	if c.DBDriver == "postgres" && !strings.HasPrefix(c.DBDSN, "postgres://") && !strings.HasPrefix(c.DBDSN, "postgresql://") {
		fail("ENVILY_DB_DSN must be a postgres:// URL when ENVILY_DB_DRIVER=postgres")
	}

	var err error
	if c.AutoMigrate, err = strconv.ParseBool(get("ENVILY_AUTO_MIGRATE", "true")); err != nil {
		fail("ENVILY_AUTO_MIGRATE must be true or false")
	}

	key, keyErr := loadMasterKey(lookup)
	if keyErr != nil {
		errs = append(errs, keyErr)
	}
	c.MasterKey = key

	if c.SessionIdleTimeout, err = time.ParseDuration(get("ENVILY_SESSION_IDLE_TIMEOUT", "12h")); err != nil || c.SessionIdleTimeout <= 0 {
		fail("ENVILY_SESSION_IDLE_TIMEOUT must be a positive Go duration such as 12h")
	}
	if c.SessionMaxAge, err = time.ParseDuration(get("ENVILY_SESSION_MAX_AGE", "168h")); err != nil || c.SessionMaxAge <= 0 {
		fail("ENVILY_SESSION_MAX_AGE must be a positive Go duration such as 168h")
	}

	if raw := get("ENVILY_TRUSTED_PROXIES", ""); raw != "" {
		for _, part := range strings.Split(raw, ",") {
			p, err := parsePrefix(strings.TrimSpace(part))
			if err != nil {
				fail("ENVILY_TRUSTED_PROXIES: %q is not a CIDR or IP address", strings.TrimSpace(part))
				continue
			}
			c.TrustedProxies = append(c.TrustedProxies, p)
		}
	}

	if c.AuditRetentionDays, err = strconv.Atoi(get("ENVILY_AUDIT_RETENTION_DAYS", "365")); err != nil || c.AuditRetentionDays < 0 {
		fail("ENVILY_AUDIT_RETENTION_DAYS must be a whole number of days (0 keeps logs forever)")
	}

	if err := c.LogLevel.UnmarshalText([]byte(get("ENVILY_LOG_LEVEL", "info"))); err != nil {
		fail("ENVILY_LOG_LEVEL must be debug, info, warn, or error")
	}
	if c.LogFormat != "json" && c.LogFormat != "text" {
		fail("ENVILY_LOG_FORMAT must be json or text")
	}

	return c, errors.Join(errs...)
}

func parsePrefix(s string) (netip.Prefix, error) {
	if strings.Contains(s, "/") {
		p, err := netip.ParsePrefix(s)
		return p.Masked(), err
	}
	a, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Prefix{}, err
	}
	return netip.PrefixFrom(a.Unmap(), a.Unmap().BitLen()), nil
}

// loadMasterKey reads ENVILY_MASTER_KEY or ENVILY_MASTER_KEY_FILE (but not
// both) and decodes it to 32 bytes.
func loadMasterKey(lookup Lookup) ([]byte, error) {
	inline, hasInline := lookup("ENVILY_MASTER_KEY")
	path, hasFile := lookup("ENVILY_MASTER_KEY_FILE")
	hasInline = hasInline && strings.TrimSpace(inline) != ""
	hasFile = hasFile && strings.TrimSpace(path) != ""
	source := "ENVILY_MASTER_KEY"
	switch {
	case hasInline && hasFile:
		return nil, errors.New("set only one of ENVILY_MASTER_KEY and ENVILY_MASTER_KEY_FILE")
	case hasFile:
		source = "ENVILY_MASTER_KEY_FILE"
		raw, err := os.ReadFile(strings.TrimSpace(path))
		if err != nil {
			return nil, fmt.Errorf("ENVILY_MASTER_KEY_FILE: cannot read key file: %w", errors.Unwrap(err))
		}
		inline = string(raw)
	case !hasInline:
		return nil, errors.New("ENVILY_MASTER_KEY or ENVILY_MASTER_KEY_FILE is required (generate one with `envily keygen`)")
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(inline))
	if err != nil {
		return nil, fmt.Errorf("%s is not valid standard base64", source)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("%s must decode to 32 bytes, got %d", source, len(key))
	}
	return key, nil
}

// Logger builds the process logger described by the configuration.
func (c Config) Logger(w io.Writer) *slog.Logger {
	opts := &slog.HandlerOptions{Level: c.LogLevel}
	if c.LogFormat == "text" {
		return slog.New(slog.NewTextHandler(w, opts))
	}
	return slog.New(slog.NewJSONHandler(w, opts))
}
