package service

import (
	"context"
	"slices"
	"time"

	"github.com/thomasbudiarjo/envily/internal/authz"
	"github.com/thomasbudiarjo/envily/internal/crypto"
	"github.com/thomasbudiarjo/envily/internal/store"
)

// ServiceTokenView is a service token with environment slugs resolved.
type ServiceTokenView struct {
	Token        store.ServiceToken
	Environments []store.Environment
}

func tokenView(t store.ServiceToken, envs []store.Environment) ServiceTokenView {
	v := ServiceTokenView{Token: t, Environments: []store.Environment{}}
	for _, e := range envs {
		if slices.Contains(t.EnvironmentIDs, e.ID) {
			v.Environments = append(v.Environments, e)
		}
	}
	return v
}

func (s *Service) ListServiceTokens(ctx context.Context, p *authz.Principal, slug string) ([]ServiceTokenView, error) {
	pc, err := s.resolveProject(ctx, s.db, p, slug)
	if err != nil {
		return nil, err
	}
	if err := s.require(p, pc, authz.ManageTokens, ""); err != nil {
		return nil, err
	}
	tokens, err := s.db.Tokens().ListService(ctx, pc.project.ID)
	if err != nil {
		return nil, err
	}
	envs, err := s.db.Environments().List(ctx, pc.project.ID)
	if err != nil {
		return nil, err
	}
	out := make([]ServiceTokenView, len(tokens))
	for i, t := range tokens {
		out[i] = tokenView(t, envs)
	}
	return out, nil
}

type CreateServiceTokenInput struct {
	Name         string
	Environments []string // slugs
	Permission   store.Permission
	ExpiresAt    *time.Time
}

// CreateServiceToken returns the plaintext token, shown only once.
func (s *Service) CreateServiceToken(ctx context.Context, p *authz.Principal, slug string, in CreateServiceTokenInput) (string, ServiceTokenView, error) {
	var token string
	var view ServiceTokenView
	err := s.db.WithTx(ctx, func(tx store.Store) error {
		pc, err := s.resolveProject(ctx, tx, p, slug)
		if err != nil {
			return err
		}
		if err := s.require(p, pc, authz.ManageTokens, ""); err != nil {
			return err
		}
		v := validation{}
		name := checkName(v, "name", in.Name, 100)
		if in.Permission != store.PermRead && in.Permission != store.PermReadWrite {
			v.add("permission", "must be read or read_write")
		}
		if in.ExpiresAt != nil && !in.ExpiresAt.After(s.now()) {
			v.add("expiresAt", "must be in the future")
		}
		envs, err := tx.Environments().List(ctx, pc.project.ID)
		if err != nil {
			return err
		}
		var ids []string
		for _, slug := range in.Environments {
			i := slices.IndexFunc(envs, func(e store.Environment) bool { return e.Slug == slug })
			if i < 0 {
				v.add("environments", "unknown environment "+slug)
				continue
			}
			if !slices.Contains(ids, envs[i].ID) {
				ids = append(ids, envs[i].ID)
			}
		}
		if len(in.Environments) == 0 {
			v.add("environments", "must name at least one environment")
		}
		if err := v.err(); err != nil {
			return err
		}
		var hash []byte
		token, hash = crypto.NewToken(crypto.ServiceTokenPrefix)
		t := store.ServiceToken{ID: newID(), ProjectID: pc.project.ID, Name: name, TokenHash: hash, Permission: in.Permission,
			CreatedBy: p.UserID, ExpiresAt: in.ExpiresAt, CreatedAt: s.now(), EnvironmentIDs: ids}
		if err := tx.Tokens().CreateService(ctx, t); err != nil {
			return err
		}
		view = tokenView(t, envs)
		return s.audit(ctx, tx, p, auditEntry{action: "service_token.created", projectID: pc.project.ID,
			targetType: "service_token", targetID: t.ID,
			meta: map[string]any{"name": name, "permission": in.Permission, "environments": in.Environments}})
	})
	return token, view, err
}

func (s *Service) RevokeServiceToken(ctx context.Context, p *authz.Principal, slug, id string) error {
	return s.db.WithTx(ctx, func(tx store.Store) error {
		pc, err := s.resolveProject(ctx, tx, p, slug)
		if err != nil {
			return err
		}
		if err := s.require(p, pc, authz.ManageTokens, ""); err != nil {
			return err
		}
		t, err := tx.Tokens().GetService(ctx, id)
		if err != nil {
			return err
		}
		if t.ProjectID != pc.project.ID {
			return errNotFound
		}
		if err := tx.Tokens().RevokeService(ctx, id, s.now()); err != nil {
			return err
		}
		return s.audit(ctx, tx, p, auditEntry{action: "service_token.revoked", projectID: pc.project.ID,
			targetType: "service_token", targetID: id, meta: map[string]any{"name": t.Name}})
	})
}

// TokenInfo describes the calling service token.
type TokenInfo struct {
	Project      store.Project
	Environments []store.Environment
	Permission   store.Permission
	Name         string
	ExpiresAt    *time.Time
}

func (s *Service) TokenInfo(ctx context.Context, p *authz.Principal) (TokenInfo, error) {
	if !p.IsServiceToken() {
		return TokenInfo{}, newErr(CodeBadRequest, "this endpoint is for service tokens")
	}
	proj, err := s.db.Projects().Get(ctx, p.ProjectID)
	if err != nil {
		return TokenInfo{}, err
	}
	t, err := s.db.Tokens().GetService(ctx, p.ID)
	if err != nil {
		return TokenInfo{}, err
	}
	envs, err := s.db.Environments().List(ctx, proj.ID)
	if err != nil {
		return TokenInfo{}, err
	}
	return TokenInfo{Project: proj, Environments: visibleEnvs(p, envs), Permission: p.Permission, Name: t.Name, ExpiresAt: t.ExpiresAt}, nil
}
