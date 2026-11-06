package service

import (
	"context"

	"github.com/thomasbudiarjo/envily/internal/authz"
	"github.com/thomasbudiarjo/envily/internal/crypto"
	"github.com/thomasbudiarjo/envily/internal/store"
)

func requireAdmin(p *authz.Principal) error {
	if p.IsSystem() {
		return nil
	}
	if !p.IsUser() || !p.IsAdmin || p.MustChangePassword {
		return errForbidden
	}
	return nil
}

func (s *Service) ListUsers(ctx context.Context, p *authz.Principal) ([]store.User, error) {
	if err := requireAdmin(p); err != nil {
		return nil, err
	}
	return s.db.Users().List(ctx)
}

type CreateUserInput struct {
	Email, Name string
	IsAdmin     bool
}

// CreateUser creates a user with a random temporary password, returned
// once. The user must change it at first login.
func (s *Service) CreateUser(ctx context.Context, p *authz.Principal, in CreateUserInput) (store.User, string, error) {
	if err := requireAdmin(p); err != nil {
		return store.User{}, "", err
	}
	v := validation{}
	email := normalizeEmail(v, "email", in.Email)
	name := checkName(v, "name", in.Name, 100)
	if err := v.err(); err != nil {
		return store.User{}, "", err
	}
	password := crypto.RandomPassword(20)
	hash, err := crypto.HashPassword(password)
	if err != nil {
		return store.User{}, "", err
	}
	now := s.now()
	u := store.User{ID: newID(), Email: email, Name: name, PasswordHash: hash, IsAdmin: in.IsAdmin,
		MustChangePassword: true, CreatedAt: now, UpdatedAt: now}
	err = s.db.WithTx(ctx, func(tx store.Store) error {
		if err := tx.Users().Create(ctx, u); err != nil {
			if AsError(err) != nil && AsError(err).Code == CodeConflict {
				return invalid("email", "is already in use")
			}
			return err
		}
		return s.audit(ctx, tx, p, auditEntry{action: "user.created", targetType: "user", targetID: u.ID,
			meta: map[string]any{"email": email, "isAdmin": in.IsAdmin}})
	})
	return u, password, err
}

type UpdateUserInput struct {
	Name          *string
	IsAdmin       *bool
	Disabled      *bool
	ResetPassword bool
}

// UpdateUser changes a user. Disabling a user revokes all of their sessions
// and personal tokens. When ResetPassword is set the new temporary password
// is returned.
func (s *Service) UpdateUser(ctx context.Context, p *authz.Principal, id string, in UpdateUserInput) (store.User, string, error) {
	if err := requireAdmin(p); err != nil {
		return store.User{}, "", err
	}
	var password string
	var u store.User
	err := s.db.WithTx(ctx, func(tx store.Store) error {
		var err error
		if u, err = tx.Users().Get(ctx, id); err != nil {
			return err
		}
		v := validation{}
		if in.Name != nil {
			u.Name = checkName(v, "name", *in.Name, 100)
		}
		self := p.IsUser() && p.UserID == id
		if self && ((in.IsAdmin != nil && !*in.IsAdmin) || (in.Disabled != nil && *in.Disabled)) {
			v.add("user", "you cannot remove your own admin rights or disable yourself")
		}
		if err := v.err(); err != nil {
			return err
		}
		meta := map[string]any{}
		now := s.now()
		if in.IsAdmin != nil {
			u.IsAdmin = *in.IsAdmin
			meta["isAdmin"] = u.IsAdmin
		}
		if in.Name != nil {
			meta["name"] = true
		}
		action := "user.updated"
		disable := false
		if in.Disabled != nil {
			switch {
			case *in.Disabled && u.DisabledAt == nil:
				u.DisabledAt, action, disable = ptr(now), "user.disabled", true
			case !*in.Disabled:
				u.DisabledAt = nil
			}
			meta["disabled"] = *in.Disabled
		}
		if in.ResetPassword {
			password = crypto.RandomPassword(20)
			hash, err := crypto.HashPassword(password)
			if err != nil {
				return err
			}
			u.PasswordHash, u.MustChangePassword = hash, true
			meta["passwordReset"] = true
		}
		u.UpdatedAt = now
		if err := tx.Users().Update(ctx, u); err != nil {
			return err
		}
		if disable || in.ResetPassword {
			if err := tx.Sessions().DeleteForUser(ctx, u.ID, ""); err != nil {
				return err
			}
			if err := tx.Tokens().RevokeAllPersonal(ctx, u.ID, now); err != nil {
				return err
			}
		}
		return s.audit(ctx, tx, p, auditEntry{action: action, targetType: "user", targetID: u.ID, meta: meta})
	})
	return u, password, err
}

// AdminUserByEmail finds a user for local admin commands.
func (s *Service) AdminUserByEmail(ctx context.Context, email string) (store.User, error) {
	return s.db.Users().GetByEmail(ctx, normalizeEmailLoose(email))
}

func normalizeEmailLoose(email string) string {
	v := validation{}
	return normalizeEmail(v, "email", email)
}
