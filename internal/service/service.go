// Package service holds Envily's business rules: authentication, users,
// projects, secrets, tokens, and the audit log. It checks authorization with
// authz, persists through store, and is the only layer that encrypts or
// decrypts secrets.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/thomasbudiarjo/envily/internal/authz"
	"github.com/thomasbudiarjo/envily/internal/crypto"
	"github.com/thomasbudiarjo/envily/internal/store"
)

// Clock returns the current time; tests inject a fake one.
type Clock interface{ Now() time.Time }

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

type Options struct {
	SessionIdleTimeout time.Duration
	SessionMaxAge      time.Duration
	SetupToken         string
	PersonalTokenTTL   time.Duration
	AuditRetentionDays int
	Clock              Clock
	Logger             *slog.Logger
}

type Service struct {
	db   store.DB
	kr   *crypto.Keyring
	opts Options
	now  func() time.Time
	log  *slog.Logger
}

// New returns a service. kr may be nil for local admin commands that never
// touch secret values.
func New(db store.DB, kr *crypto.Keyring, opts Options) *Service {
	if opts.SessionIdleTimeout == 0 {
		opts.SessionIdleTimeout = 12 * time.Hour
	}
	if opts.SessionMaxAge == 0 {
		opts.SessionMaxAge = 7 * 24 * time.Hour
	}
	if opts.PersonalTokenTTL == 0 {
		opts.PersonalTokenTTL = 90 * 24 * time.Hour
	}
	if opts.Clock == nil {
		opts.Clock = realClock{}
	}
	if opts.Logger == nil {
		opts.Logger = slog.New(slog.DiscardHandler)
	}
	clock := opts.Clock
	return &Service{db: db, kr: kr, opts: opts, log: opts.Logger, now: func() time.Time {
		// Storage keeps milliseconds; truncating here keeps values round-trippable.
		return clock.Now().UTC().Truncate(time.Millisecond)
	}}
}

// ---- errors ----

// Error is a failure the API reports to clients. Code is one of the
// snake_case codes in the API spec.
type Error struct {
	Code    string
	Message string
	Details map[string]string
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// Error codes.
const (
	CodeBadRequest             = "bad_request"
	CodeValidationFailed       = "validation_failed"
	CodeUnauthorized           = "unauthorized"
	CodeInvalidCredentials     = "invalid_credentials" //nolint:gosec // G101: an error code
	CodeForbidden              = "forbidden"
	CodeNotFound               = "not_found"
	CodeConflict               = "conflict"
	CodeLastOwner              = "last_owner"
	CodeRateLimited            = "rate_limited"
	CodeSetupRequired          = "setup_required"
	CodePasswordChangeRequired = "password_change_required"
	CodeDecryptionFailed       = "decryption_failed"
	CodeInternal               = "internal"
)

func newErr(code, msg string) *Error { return &Error{Code: code, Message: msg} }

var (
	errNotFound           = newErr(CodeNotFound, "not found")
	errForbidden          = newErr(CodeForbidden, "you do not have permission to do this")
	errUnauthorized       = newErr(CodeUnauthorized, "authentication required")
	errInvalidCredentials = newErr(CodeInvalidCredentials, "invalid email or password")
)

func conflict(msg string) *Error { return newErr(CodeConflict, msg) }

// validation collects per-field validation errors.
type validation map[string]string

func (v validation) add(field, msg string) { v[field] = msg }

func (v validation) err() error {
	if len(v) == 0 {
		return nil
	}
	return &Error{Code: CodeValidationFailed, Message: "some fields are invalid", Details: v}
}

func invalid(field, msg string) error { return validation{field: msg}.err() }

// AsError returns err as an *Error, mapping store errors; other errors map
// to nil (the caller treats them as internal).
func AsError(err error) *Error {
	var e *Error
	switch {
	case errors.As(err, &e):
		return e
	case errors.Is(err, store.ErrNotFound):
		return errNotFound
	case errors.Is(err, store.ErrConflict):
		return conflict("a record with the same name already exists")
	}
	return nil
}

// ---- request metadata ----

// RequestMeta describes the client, for the audit log.
type RequestMeta struct {
	IP        string
	UserAgent string
}

type requestMetaKey struct{}

func WithRequestMeta(ctx context.Context, m RequestMeta) context.Context {
	return context.WithValue(ctx, requestMetaKey{}, m)
}

func requestMeta(ctx context.Context) RequestMeta {
	m, _ := ctx.Value(requestMetaKey{}).(RequestMeta)
	return m
}

// ---- audit ----

type auditEntry struct {
	action     string
	projectID  string
	envID      string
	targetType string
	targetID   string
	meta       map[string]any // never secret values
}

func newID() string { return uuid.Must(uuid.NewV7()).String() }

func (s *Service) audit(ctx context.Context, st store.Store, p *authz.Principal, e auditEntry) error {
	meta := "{}"
	if len(e.meta) > 0 {
		b, err := json.Marshal(e.meta)
		if err != nil {
			return err
		}
		meta = string(b)
	}
	rm := requestMeta(ctx)
	actorType, actorID, label := string(authz.ActorSystem), "", ""
	if p != nil {
		actorType, actorID, label = string(p.Type), p.ID, p.Label
	}
	return st.Audit().Insert(ctx, store.AuditLog{
		ID: newID(), At: s.now(), ActorType: actorType, ActorID: actorID, ActorLabel: label,
		Action: e.action, ProjectID: e.projectID, EnvironmentID: e.envID, TargetType: e.targetType,
		TargetID: e.targetID, Metadata: meta, IP: rm.IP, UserAgent: truncate(rm.UserAgent, 512),
	})
}

// ---- validation helpers ----

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

func normalizeEmail(v validation, field, email string) string {
	email = strings.ToLower(strings.TrimSpace(email))
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email || len(email) > 254 {
		v.add(field, "must be a valid email address")
	}
	return email
}

func checkName(v validation, field, name string, maxLen int) string {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > maxLen {
		v.add(field, fmt.Sprintf("must be 1-%d characters", maxLen))
	}
	return name
}

func checkPassword(v validation, field, pw string) {
	if n := utf8.RuneCountInString(pw); n < 12 || n > 256 {
		v.add(field, "must be 12-256 characters")
	}
}

var (
	slugRe    = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
	nonSlugRe = regexp.MustCompile(`[^a-z0-9]+`)
	keyRe     = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,254}$`)
)

// Slugify derives a slug from a display name.
func Slugify(name string) string {
	s := strings.Trim(nonSlugRe.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if len(s) > 63 {
		s = strings.TrimRight(s[:63], "-")
	}
	return s
}

func checkSlug(v validation, field, slug string) {
	if !slugRe.MatchString(slug) {
		v.add(field, "must be 1-63 lowercase letters, digits, or dashes, starting with a letter or digit")
	}
}

// reservedEnvSlugs are UI routes next to /p/{project}/{env}; an environment
// with one of these slugs could never be opened in the web UI.
var reservedEnvSlugs = map[string]bool{"compare": true, "settings": true, "audit": true}

func checkEnvSlug(v validation, field, slug string) {
	checkSlug(v, field, slug)
	if reservedEnvSlugs[slug] {
		v.add(field, "is reserved")
	}
}

// ValidKey reports whether key is a valid secret key (environment variable name).
func ValidKey(key string) bool { return keyRe.MatchString(key) }

func ptr[T any](v T) *T { return &v }
