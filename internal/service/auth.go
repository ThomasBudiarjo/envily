package service

import (
	"context"
	"crypto/subtle"
	"errors"
	"strings"
	"time"

	"github.com/thomasbudiarjo/envily/internal/authz"
	"github.com/thomasbudiarjo/envily/internal/crypto"
	"github.com/thomasbudiarjo/envily/internal/store"
)

// touchInterval limits last_seen_at / last_used_at writes.
const touchInterval = time.Minute

// SetupStatus reports whether the instance has no users yet.
func (s *Service) SetupStatus(ctx context.Context) (needsSetup bool, err error) {
	n, err := s.db.Users().Count(ctx)
	return n == 0, err
}

type SetupInput struct {
	Email, Name, Password, SetupToken string
}

// Setup creates the first (admin) user and a session for it. It fails with
// conflict once any user exists.
func (s *Service) Setup(ctx context.Context, in SetupInput) (store.User, string, error) {
	if want := s.opts.SetupToken; want != "" &&
		subtle.ConstantTimeCompare([]byte(in.SetupToken), []byte(want)) != 1 {
		return store.User{}, "", invalid("setupToken", "is incorrect")
	}
	v := validation{}
	email := normalizeEmail(v, "email", in.Email)
	name := checkName(v, "name", in.Name, 100)
	checkPassword(v, "password", in.Password)
	if err := v.err(); err != nil {
		return store.User{}, "", err
	}
	hash, err := crypto.HashPassword(in.Password)
	if err != nil {
		return store.User{}, "", err
	}
	now := s.now()
	u := store.User{ID: newID(), Email: email, Name: name, PasswordHash: hash, IsAdmin: true, CreatedAt: now, UpdatedAt: now}
	var token string
	err = s.db.WithTx(ctx, func(tx store.Store) error {
		// The meta row makes concurrent setups fail even where a user count
		// would race (PostgreSQL read committed).
		first, err := tx.Meta().Insert(ctx, store.MetaSetupDone, now.Format(time.RFC3339))
		if err != nil {
			return err
		}
		if n, err := tx.Users().Count(ctx); err != nil {
			return err
		} else if !first || n > 0 {
			return conflict("setup has already been completed")
		}
		if err := tx.Users().Create(ctx, u); err != nil {
			return err
		}
		p := userPrincipal(u, "")
		if err := s.audit(ctx, tx, p, auditEntry{action: "user.created", targetType: "user", targetID: u.ID,
			meta: map[string]any{"email": u.Email, "isAdmin": true, "setup": true}}); err != nil {
			return err
		}
		token, err = s.createSession(ctx, tx, u)
		return err
	})
	return u, token, err
}

func userPrincipal(u store.User, sessionID string) *authz.Principal {
	return &authz.Principal{Type: authz.ActorUser, ID: u.ID, Label: u.Email, UserID: u.ID,
		IsAdmin: u.IsAdmin, MustChangePassword: u.MustChangePassword, SessionID: sessionID}
}

func (s *Service) createSession(ctx context.Context, st store.Store, u store.User) (string, error) {
	token, hash := crypto.NewToken("")
	now := s.now()
	rm := requestMeta(ctx)
	err := st.Sessions().Create(ctx, store.Session{
		ID: newID(), UserID: u.ID, TokenHash: hash, CreatedAt: now, LastSeenAt: now,
		ExpiresAt: now.Add(s.opts.SessionMaxAge), IP: rm.IP, UserAgent: truncate(rm.UserAgent, 512),
	})
	return token, err
}

// checkCredentials returns the user for email/password. Unknown emails,
// wrong passwords, and disabled users fail identically and take the same time.
func (s *Service) checkCredentials(ctx context.Context, email, password string) (store.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	u, err := s.db.Users().GetByEmail(ctx, email)
	if errors.Is(err, store.ErrNotFound) {
		crypto.DummyVerify(password)
	} else if err != nil {
		return store.User{}, err
	}
	ok := false
	if err == nil {
		if ok, err = crypto.VerifyPassword(password, u.PasswordHash); err != nil {
			return store.User{}, err
		}
	}
	if !ok || u.DisabledAt != nil {
		label := email
		if len(label) > 254 {
			label = label[:254]
		}
		p := &authz.Principal{Type: authz.ActorUser, ID: u.ID, Label: label}
		if aerr := s.audit(ctx, s.db, p, auditEntry{action: "auth.login_failed", targetType: "user", targetID: u.ID}); aerr != nil {
			return store.User{}, aerr
		}
		return store.User{}, errInvalidCredentials
	}
	return u, nil
}

// Login checks credentials and creates a browser session.
func (s *Service) Login(ctx context.Context, email, password string) (store.User, string, error) {
	u, err := s.checkCredentials(ctx, email, password)
	if err != nil {
		return store.User{}, "", err
	}
	var token string
	err = s.db.WithTx(ctx, func(tx store.Store) error {
		var err error
		if token, err = s.createSession(ctx, tx, u); err != nil {
			return err
		}
		return s.audit(ctx, tx, userPrincipal(u, ""), auditEntry{action: "auth.login", targetType: "user", targetID: u.ID,
			meta: map[string]any{"method": "password"}})
	})
	return u, token, err
}

// CLILogin checks credentials and returns a new personal token.
func (s *Service) CLILogin(ctx context.Context, email, password, tokenName string) (string, store.PersonalToken, error) {
	u, err := s.checkCredentials(ctx, email, password)
	if err != nil {
		return "", store.PersonalToken{}, err
	}
	if strings.TrimSpace(tokenName) == "" {
		tokenName = "envily CLI" //nolint:gosec // G101: a display name
	}
	p := userPrincipal(u, "")
	if err := s.audit(ctx, s.db, p, auditEntry{action: "auth.login", targetType: "user", targetID: u.ID,
		meta: map[string]any{"method": "cli"}}); err != nil {
		return "", store.PersonalToken{}, err
	}
	// A temporary password must be changed before tokens are issued.
	if u.MustChangePassword {
		return "", store.PersonalToken{}, newErr(CodePasswordChangeRequired, "change your password in the web UI before logging in from the CLI")
	}
	exp := s.now().Add(s.opts.PersonalTokenTTL)
	return s.CreatePersonalToken(ctx, p, tokenName, &exp)
}

// Logout deletes the principal's session, if it has one.
func (s *Service) Logout(ctx context.Context, p *authz.Principal) error {
	if p == nil || p.SessionID == "" {
		return nil
	}
	return s.db.WithTx(ctx, func(tx store.Store) error {
		if err := tx.Sessions().Delete(ctx, p.SessionID); err != nil {
			return err
		}
		return s.audit(ctx, tx, p, auditEntry{action: "auth.logout", targetType: "user", targetID: p.UserID})
	})
}

// AuthenticateSession resolves a session cookie. Unknown, expired, or idle
// sessions and disabled users yield unauthorized.
func (s *Service) AuthenticateSession(ctx context.Context, token string) (*authz.Principal, error) {
	sess, err := s.db.Sessions().GetByHash(ctx, crypto.HashToken(token))
	if errors.Is(err, store.ErrNotFound) {
		return nil, errUnauthorized
	} else if err != nil {
		return nil, err
	}
	now := s.now()
	if !now.Before(sess.ExpiresAt) || !now.Before(sess.LastSeenAt.Add(s.opts.SessionIdleTimeout)) {
		_ = s.db.Sessions().Delete(ctx, sess.ID)
		return nil, errUnauthorized
	}
	u, err := s.db.Users().Get(ctx, sess.UserID)
	if err != nil {
		return nil, errUnauthorized
	}
	if u.DisabledAt != nil {
		return nil, errUnauthorized
	}
	if now.Sub(sess.LastSeenAt) >= touchInterval {
		if err := s.db.Sessions().Touch(ctx, sess.ID, now); err != nil {
			return nil, err
		}
	}
	return userPrincipal(u, sess.ID), nil
}

func expired(at *time.Time, now time.Time) bool { return at != nil && !now.Before(*at) }

// AuthenticateBearer resolves a service or personal token.
func (s *Service) AuthenticateBearer(ctx context.Context, token string) (*authz.Principal, error) {
	now := s.now()
	hash := crypto.HashToken(token)
	switch {
	case strings.HasPrefix(token, crypto.ServiceTokenPrefix):
		t, err := s.db.Tokens().GetServiceByHash(ctx, hash)
		if errors.Is(err, store.ErrNotFound) {
			return nil, errUnauthorized
		} else if err != nil {
			return nil, err
		}
		if t.RevokedAt != nil || expired(t.ExpiresAt, now) {
			return nil, errUnauthorized
		}
		if t.LastUsedAt == nil || now.Sub(*t.LastUsedAt) >= touchInterval {
			if err := s.db.Tokens().TouchService(ctx, t.ID, now); err != nil {
				return nil, err
			}
		}
		return &authz.Principal{Type: authz.ActorServiceToken, ID: t.ID, Label: t.Name, ProjectID: t.ProjectID,
			EnvironmentIDs: t.EnvironmentIDs, Permission: t.Permission}, nil
	case strings.HasPrefix(token, crypto.PersonalTokenPrefix):
		t, err := s.db.Tokens().GetPersonalByHash(ctx, hash)
		if errors.Is(err, store.ErrNotFound) {
			return nil, errUnauthorized
		} else if err != nil {
			return nil, err
		}
		if t.RevokedAt != nil || expired(t.ExpiresAt, now) {
			return nil, errUnauthorized
		}
		u, err := s.db.Users().Get(ctx, t.UserID)
		if err != nil || u.DisabledAt != nil {
			return nil, errUnauthorized
		}
		if t.LastUsedAt == nil || now.Sub(*t.LastUsedAt) >= touchInterval {
			if err := s.db.Tokens().TouchPersonal(ctx, t.ID, now); err != nil {
				return nil, err
			}
		}
		return &authz.Principal{Type: authz.ActorPersonalToken, ID: t.ID, Label: u.Email + " (" + t.Name + ")",
			UserID: u.ID, IsAdmin: u.IsAdmin, MustChangePassword: u.MustChangePassword}, nil
	}
	return nil, errUnauthorized
}

func requireUser(p *authz.Principal) error {
	if !p.IsUser() {
		return errForbidden
	}
	return nil
}

// Me returns the principal's user record.
func (s *Service) Me(ctx context.Context, p *authz.Principal) (store.User, error) {
	if err := requireUser(p); err != nil {
		return store.User{}, err
	}
	return s.db.Users().Get(ctx, p.UserID)
}

// ChangePassword sets a new password and signs out every other session.
func (s *Service) ChangePassword(ctx context.Context, p *authz.Principal, current, next string) error {
	if err := requireUser(p); err != nil {
		return err
	}
	v := validation{}
	checkPassword(v, "newPassword", next)
	if err := v.err(); err != nil {
		return err
	}
	u, err := s.db.Users().Get(ctx, p.UserID)
	if err != nil {
		return err
	}
	if ok, err := crypto.VerifyPassword(current, u.PasswordHash); err != nil {
		return err
	} else if !ok {
		return invalid("currentPassword", "is incorrect")
	}
	if current == next {
		return invalid("newPassword", "must differ from the current password")
	}
	hash, err := crypto.HashPassword(next)
	if err != nil {
		return err
	}
	u.PasswordHash, u.MustChangePassword, u.UpdatedAt = hash, false, s.now()
	return s.db.WithTx(ctx, func(tx store.Store) error {
		if err := tx.Users().Update(ctx, u); err != nil {
			return err
		}
		if err := tx.Sessions().DeleteForUser(ctx, u.ID, p.SessionID); err != nil {
			return err
		}
		return s.audit(ctx, tx, p, auditEntry{action: "auth.password_changed", targetType: "user", targetID: u.ID})
	})
}

// ---- personal tokens ----

func (s *Service) ListPersonalTokens(ctx context.Context, p *authz.Principal) ([]store.PersonalToken, error) {
	if err := requireUser(p); err != nil {
		return nil, err
	}
	return s.db.Tokens().ListPersonal(ctx, p.UserID)
}

// CreatePersonalToken returns the plaintext token, shown only once.
func (s *Service) CreatePersonalToken(ctx context.Context, p *authz.Principal, name string, expiresAt *time.Time) (string, store.PersonalToken, error) {
	if err := requireUser(p); err != nil {
		return "", store.PersonalToken{}, err
	}
	v := validation{}
	name = checkName(v, "name", name, 100)
	if expiresAt != nil && !expiresAt.After(s.now()) {
		v.add("expiresAt", "must be in the future")
	}
	if err := v.err(); err != nil {
		return "", store.PersonalToken{}, err
	}
	token, hash := crypto.NewToken(crypto.PersonalTokenPrefix)
	t := store.PersonalToken{ID: newID(), UserID: p.UserID, Name: name, TokenHash: hash, ExpiresAt: expiresAt, CreatedAt: s.now()}
	err := s.db.WithTx(ctx, func(tx store.Store) error {
		if err := tx.Tokens().CreatePersonal(ctx, t); err != nil {
			return err
		}
		return s.audit(ctx, tx, p, auditEntry{action: "personal_token.created", targetType: "personal_token", targetID: t.ID,
			meta: map[string]any{"name": name}})
	})
	return token, t, err
}

func (s *Service) RevokePersonalToken(ctx context.Context, p *authz.Principal, id string) error {
	if err := requireUser(p); err != nil {
		return err
	}
	return s.db.WithTx(ctx, func(tx store.Store) error {
		if err := tx.Tokens().RevokePersonal(ctx, p.UserID, id, s.now()); err != nil {
			return err
		}
		return s.audit(ctx, tx, p, auditEntry{action: "personal_token.revoked", targetType: "personal_token", targetID: id})
	})
}
