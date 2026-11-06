package service

import (
	"context"
	"errors"
	"slices"

	"github.com/thomasbudiarjo/envily/internal/authz"
	"github.com/thomasbudiarjo/envily/internal/store"
)

// DefaultEnvironments are created with every project, in this order.
var DefaultEnvironments = []string{"development", "staging", "production"}

// projectCtx is a project resolved for a principal.
type projectCtx struct {
	project store.Project
	role    store.Role // "" when the principal is not a member
}

// resolveProject loads the project by slug for p. Projects p cannot see are
// reported as not found, so their existence does not leak. Instance admins
// see every project (to list them and add themselves) but get no
// permissions without a membership.
func (s *Service) resolveProject(ctx context.Context, st store.Store, p *authz.Principal, slug string) (projectCtx, error) {
	if p == nil {
		return projectCtx{}, errUnauthorized
	}
	proj, err := st.Projects().GetBySlug(ctx, slug)
	if errors.Is(err, store.ErrNotFound) {
		return projectCtx{}, errNotFound
	} else if err != nil {
		return projectCtx{}, err
	}
	pc := projectCtx{project: proj}
	switch {
	case p.IsServiceToken():
		if p.ProjectID != proj.ID {
			return projectCtx{}, errNotFound
		}
	case p.IsUser():
		m, err := st.Projects().GetMember(ctx, proj.ID, p.UserID)
		switch {
		case err == nil:
			pc.role = m.Role
		case !errors.Is(err, store.ErrNotFound):
			return projectCtx{}, err
		case !p.IsAdmin:
			return projectCtx{}, errNotFound
		}
	case !p.IsSystem():
		return projectCtx{}, errNotFound
	}
	return pc, nil
}

// adminObserver reports whether p is an instance admin without a role in
// the project: allowed to see it and its members, nothing more.
func adminObserver(p *authz.Principal, pc projectCtx) bool { return p.IsAdmin && pc.role == "" }

// require checks perm on the project (and environment envID, if set).
func (s *Service) require(p *authz.Principal, pc projectCtx, perm authz.Permission, envID string) error {
	if authz.Can(p, pc.role, perm, pc.project.ID, envID) {
		return nil
	}
	return errForbidden
}

// resolveEnv loads an environment of the project. Environments outside a
// service token's scope are reported as not found.
func (s *Service) resolveEnv(ctx context.Context, st store.Store, p *authz.Principal, pc projectCtx, slug string) (store.Environment, error) {
	env, err := st.Environments().GetBySlug(ctx, pc.project.ID, slug)
	if errors.Is(err, store.ErrNotFound) {
		return store.Environment{}, errNotFound
	} else if err != nil {
		return store.Environment{}, err
	}
	if p.IsServiceToken() && !slices.Contains(p.EnvironmentIDs, env.ID) {
		return store.Environment{}, errNotFound
	}
	return env, nil
}

// visibleEnvs filters environments to those p may see.
func visibleEnvs(p *authz.Principal, envs []store.Environment) []store.Environment {
	if !p.IsServiceToken() {
		return envs
	}
	out := []store.Environment{}
	for _, e := range envs {
		if slices.Contains(p.EnvironmentIDs, e.ID) {
			out = append(out, e)
		}
	}
	return out
}

// ProjectView is a project as seen by one principal.
type ProjectView struct {
	Project      store.Project
	Role         store.Role
	Environments []store.Environment
}

func (s *Service) view(ctx context.Context, st store.Store, p *authz.Principal, pc projectCtx) (ProjectView, error) {
	envs, err := st.Environments().List(ctx, pc.project.ID)
	if err != nil {
		return ProjectView{}, err
	}
	return ProjectView{Project: pc.project, Role: pc.role, Environments: visibleEnvs(p, envs)}, nil
}

// ListProjects returns the projects p belongs to (all projects for admins).
func (s *Service) ListProjects(ctx context.Context, p *authz.Principal) ([]ProjectView, error) {
	var projects []store.Project
	var err error
	switch {
	case p.IsServiceToken():
		var proj store.Project
		if proj, err = s.db.Projects().Get(ctx, p.ProjectID); err == nil {
			projects = []store.Project{proj}
		}
	case p.IsUser() && p.IsAdmin:
		projects, err = s.db.Projects().List(ctx)
	case p.IsUser():
		projects, err = s.db.Projects().ListForUser(ctx, p.UserID)
	default:
		return nil, errForbidden
	}
	if err != nil {
		return nil, err
	}
	out := make([]ProjectView, 0, len(projects))
	for _, proj := range projects {
		pc, err := s.resolveProject(ctx, s.db, p, proj.Slug)
		if err != nil {
			return nil, err
		}
		v, err := s.view(ctx, s.db, p, pc)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func (s *Service) GetProject(ctx context.Context, p *authz.Principal, slug string) (ProjectView, error) {
	pc, err := s.resolveProject(ctx, s.db, p, slug)
	if err != nil {
		return ProjectView{}, err
	}
	return s.view(ctx, s.db, p, pc)
}

type CreateProjectInput struct {
	Name string
	Slug string // derived from Name when empty
}

// CreateProject creates a project with the default environments and makes
// the creator its owner.
func (s *Service) CreateProject(ctx context.Context, p *authz.Principal, in CreateProjectInput) (ProjectView, error) {
	if !p.IsUser() || p.MustChangePassword {
		return ProjectView{}, errForbidden
	}
	v := validation{}
	name := checkName(v, "name", in.Name, 100)
	slug := in.Slug
	if slug == "" {
		slug = Slugify(name)
	}
	checkSlug(v, "slug", slug)
	if err := v.err(); err != nil {
		return ProjectView{}, err
	}
	now := s.now()
	proj := store.Project{ID: newID(), Name: name, Slug: slug, CreatedAt: now, UpdatedAt: now}
	var err error
	if proj.WrappedDEK, proj.DEKRootVersion, err = s.kr.NewDEK(proj.ID); err != nil {
		return ProjectView{}, err
	}
	var view ProjectView
	err = s.db.WithTx(ctx, func(tx store.Store) error {
		if err := tx.Projects().Create(ctx, proj); err != nil {
			if errors.Is(err, store.ErrConflict) {
				return invalid("slug", "is already used by another project")
			}
			return err
		}
		envs := make([]store.Environment, len(DefaultEnvironments))
		for i, n := range DefaultEnvironments {
			envs[i] = store.Environment{ID: newID(), ProjectID: proj.ID, Name: n, Slug: n, Position: int64(i), CreatedAt: now}
			if err := tx.Environments().Create(ctx, envs[i]); err != nil {
				return err
			}
		}
		if err := tx.Projects().AddMember(ctx, store.Member{ProjectID: proj.ID, UserID: p.UserID, Role: store.RoleOwner, CreatedAt: now}); err != nil {
			return err
		}
		view = ProjectView{Project: proj, Role: store.RoleOwner, Environments: envs}
		return s.audit(ctx, tx, p, auditEntry{action: "project.created", projectID: proj.ID, targetType: "project",
			targetID: proj.ID, meta: map[string]any{"name": name, "slug": slug}})
	})
	return view, err
}

type UpdateProjectInput struct {
	Name, Slug *string
}

func (s *Service) UpdateProject(ctx context.Context, p *authz.Principal, slug string, in UpdateProjectInput) (ProjectView, error) {
	var view ProjectView
	err := s.db.WithTx(ctx, func(tx store.Store) error {
		pc, err := s.resolveProject(ctx, tx, p, slug)
		if err != nil {
			return err
		}
		if err := s.require(p, pc, authz.ManageProject, ""); err != nil {
			return err
		}
		v := validation{}
		proj := pc.project
		meta := map[string]any{}
		if in.Name != nil {
			proj.Name = checkName(v, "name", *in.Name, 100)
			meta["name"] = proj.Name
		}
		if in.Slug != nil {
			proj.Slug = *in.Slug
			checkSlug(v, "slug", proj.Slug)
			meta["slug"] = proj.Slug
		}
		if err := v.err(); err != nil {
			return err
		}
		proj.UpdatedAt = s.now()
		if err := tx.Projects().Update(ctx, proj.ID, proj.Name, proj.Slug, proj.UpdatedAt); err != nil {
			if errors.Is(err, store.ErrConflict) {
				return invalid("slug", "is already used by another project")
			}
			return err
		}
		pc.project = proj
		if view, err = s.view(ctx, tx, p, pc); err != nil {
			return err
		}
		return s.audit(ctx, tx, p, auditEntry{action: "project.updated", projectID: proj.ID, targetType: "project",
			targetID: proj.ID, meta: meta})
	})
	return view, err
}

// DeleteProject deletes a project and everything in it. confirm must equal
// the project slug.
func (s *Service) DeleteProject(ctx context.Context, p *authz.Principal, slug, confirm string) error {
	return s.db.WithTx(ctx, func(tx store.Store) error {
		pc, err := s.resolveProject(ctx, tx, p, slug)
		if err != nil {
			return err
		}
		if err := s.require(p, pc, authz.ManageProject, ""); err != nil {
			return err
		}
		if confirm != pc.project.Slug {
			return invalid("confirm", "must be the project slug")
		}
		if err := tx.Projects().Delete(ctx, pc.project.ID); err != nil {
			return err
		}
		return s.audit(ctx, tx, p, auditEntry{action: "project.deleted", projectID: pc.project.ID, targetType: "project",
			targetID: pc.project.ID, meta: map[string]any{"name": pc.project.Name, "slug": pc.project.Slug}})
	})
}

// ---- members ----

func (s *Service) ListMembers(ctx context.Context, p *authz.Principal, slug string) ([]store.Member, error) {
	pc, err := s.resolveProject(ctx, s.db, p, slug)
	if err != nil {
		return nil, err
	}
	if p.IsServiceToken() {
		return nil, errForbidden
	}
	// Admins may see who is in a project before adding themselves.
	if !adminObserver(p, pc) {
		if err := s.require(p, pc, authz.ViewProject, ""); err != nil {
			return nil, err
		}
	}
	return s.db.Projects().ListMembers(ctx, pc.project.ID)
}

func validRole(r store.Role) bool {
	return r == store.RoleOwner || r == store.RoleEditor || r == store.RoleViewer
}

type AddMemberInput struct {
	UserID string
	Email  string // alternative to UserID
	Role   store.Role
}

// AddMember adds a user to a project. Owners may add anyone; an instance
// admin who is not a member may add only themselves.
func (s *Service) AddMember(ctx context.Context, p *authz.Principal, slug string, in AddMemberInput) (store.Member, error) {
	var m store.Member
	err := s.db.WithTx(ctx, func(tx store.Store) error {
		pc, err := s.resolveProject(ctx, tx, p, slug)
		if err != nil {
			return err
		}
		v := validation{}
		if !validRole(in.Role) {
			v.add("role", "must be owner, editor, or viewer")
		}
		var u store.User
		switch {
		case in.UserID != "":
			u, err = tx.Users().Get(ctx, in.UserID)
		case in.Email != "":
			u, err = tx.Users().GetByEmail(ctx, normalizeEmailLoose(in.Email))
		default:
			v.add("userId", "is required")
		}
		if errors.Is(err, store.ErrNotFound) {
			v.add("userId", "no such user")
		} else if err != nil {
			return err
		}
		selfAdd := p.IsUser() && adminObserver(p, pc) && u.ID == p.UserID
		if !selfAdd {
			if err := s.require(p, pc, authz.ManageMembers, ""); err != nil {
				return err
			}
		}
		if err := v.err(); err != nil {
			return err
		}
		if u.DisabledAt != nil {
			return invalid("userId", "the user is disabled")
		}
		m = store.Member{ProjectID: pc.project.ID, UserID: u.ID, Role: in.Role, CreatedAt: s.now(), Email: u.Email, Name: u.Name}
		if err := tx.Projects().AddMember(ctx, m); err != nil {
			if errors.Is(err, store.ErrConflict) {
				return conflict("the user is already a member")
			}
			return err
		}
		return s.audit(ctx, tx, p, auditEntry{action: "member.added", projectID: pc.project.ID, targetType: "user",
			targetID: u.ID, meta: map[string]any{"email": u.Email, "role": in.Role, "selfAdd": selfAdd}})
	})
	return m, err
}

var errLastOwner = newErr(CodeLastOwner, "a project must keep at least one owner")

// ensureOwnerRemains fails when userID is the project's only owner.
func ensureOwnerRemains(ctx context.Context, tx store.Store, projectID string, current store.Role) error {
	if current != store.RoleOwner {
		return nil
	}
	n, err := tx.Projects().CountOwners(ctx, projectID)
	if err != nil {
		return err
	}
	if n <= 1 {
		return errLastOwner
	}
	return nil
}

func (s *Service) UpdateMember(ctx context.Context, p *authz.Principal, slug, userID string, role store.Role) (store.Member, error) {
	var m store.Member
	err := s.db.WithTx(ctx, func(tx store.Store) error {
		pc, err := s.resolveProject(ctx, tx, p, slug)
		if err != nil {
			return err
		}
		if err := s.require(p, pc, authz.ManageMembers, ""); err != nil {
			return err
		}
		if !validRole(role) {
			return invalid("role", "must be owner, editor, or viewer")
		}
		if m, err = tx.Projects().GetMember(ctx, pc.project.ID, userID); err != nil {
			return err
		}
		if role != store.RoleOwner {
			if err := ensureOwnerRemains(ctx, tx, pc.project.ID, m.Role); err != nil {
				return err
			}
		}
		from := m.Role
		m.Role = role
		if err := tx.Projects().UpdateMemberRole(ctx, pc.project.ID, userID, role); err != nil {
			return err
		}
		return s.audit(ctx, tx, p, auditEntry{action: "member.updated", projectID: pc.project.ID, targetType: "user",
			targetID: userID, meta: map[string]any{"from": from, "to": role}})
	})
	return m, err
}

// RemoveMember removes a user from a project. Owners may remove anyone;
// any member may leave.
func (s *Service) RemoveMember(ctx context.Context, p *authz.Principal, slug, userID string) error {
	return s.db.WithTx(ctx, func(tx store.Store) error {
		pc, err := s.resolveProject(ctx, tx, p, slug)
		if err != nil {
			return err
		}
		self := p.IsUser() && p.UserID == userID && pc.role != ""
		if !self {
			if err := s.require(p, pc, authz.ManageMembers, ""); err != nil {
				return err
			}
		}
		m, err := tx.Projects().GetMember(ctx, pc.project.ID, userID)
		if err != nil {
			return err
		}
		if err := ensureOwnerRemains(ctx, tx, pc.project.ID, m.Role); err != nil {
			return err
		}
		if err := tx.Projects().RemoveMember(ctx, pc.project.ID, userID); err != nil {
			return err
		}
		return s.audit(ctx, tx, p, auditEntry{action: "member.removed", projectID: pc.project.ID, targetType: "user",
			targetID: userID, meta: map[string]any{"role": m.Role}})
	})
}

// ---- environments ----

func (s *Service) ListEnvironments(ctx context.Context, p *authz.Principal, slug string) ([]store.Environment, error) {
	v, err := s.GetProject(ctx, p, slug)
	if err != nil {
		return nil, err
	}
	pc := projectCtx{project: v.Project, role: v.Role}
	if !adminObserver(p, pc) {
		if err := s.require(p, pc, authz.ViewProject, ""); err != nil {
			return nil, err
		}
	}
	return v.Environments, nil
}

func (s *Service) CreateEnvironment(ctx context.Context, p *authz.Principal, slug, name, envSlug string) (store.Environment, error) {
	var env store.Environment
	err := s.db.WithTx(ctx, func(tx store.Store) error {
		pc, err := s.resolveProject(ctx, tx, p, slug)
		if err != nil {
			return err
		}
		if err := s.require(p, pc, authz.ManageEnvironments, ""); err != nil {
			return err
		}
		v := validation{}
		name = checkName(v, "name", name, 64)
		if envSlug == "" {
			envSlug = Slugify(name)
		}
		checkSlug(v, "slug", envSlug)
		if err := v.err(); err != nil {
			return err
		}
		envs, err := tx.Environments().List(ctx, pc.project.ID)
		if err != nil {
			return err
		}
		pos := int64(0)
		for _, e := range envs {
			pos = max(pos, e.Position+1)
		}
		env = store.Environment{ID: newID(), ProjectID: pc.project.ID, Name: name, Slug: envSlug, Position: pos, CreatedAt: s.now()}
		if err := tx.Environments().Create(ctx, env); err != nil {
			if errors.Is(err, store.ErrConflict) {
				return invalid("slug", "is already used by another environment")
			}
			return err
		}
		return s.audit(ctx, tx, p, auditEntry{action: "environment.created", projectID: pc.project.ID, envID: env.ID,
			targetType: "environment", targetID: env.ID, meta: map[string]any{"name": name, "slug": envSlug}})
	})
	return env, err
}

type UpdateEnvironmentInput struct {
	Name, Slug *string
	Position   *int64
}

func (s *Service) UpdateEnvironment(ctx context.Context, p *authz.Principal, slug, envSlug string, in UpdateEnvironmentInput) (store.Environment, error) {
	var env store.Environment
	err := s.db.WithTx(ctx, func(tx store.Store) error {
		pc, err := s.resolveProject(ctx, tx, p, slug)
		if err != nil {
			return err
		}
		if env, err = s.resolveEnv(ctx, tx, p, pc, envSlug); err != nil {
			return err
		}
		if err := s.require(p, pc, authz.ManageEnvironments, ""); err != nil {
			return err
		}
		v := validation{}
		meta := map[string]any{}
		if in.Name != nil {
			env.Name = checkName(v, "name", *in.Name, 64)
			meta["name"] = env.Name
		}
		if in.Slug != nil {
			env.Slug = *in.Slug
			checkSlug(v, "slug", env.Slug)
			meta["slug"] = env.Slug
		}
		if in.Position != nil {
			if *in.Position < 0 || *in.Position > 1000 {
				v.add("position", "must be between 0 and 1000")
			}
			env.Position = *in.Position
			meta["position"] = env.Position
		}
		if err := v.err(); err != nil {
			return err
		}
		if err := tx.Environments().Update(ctx, env); err != nil {
			if errors.Is(err, store.ErrConflict) {
				return invalid("slug", "is already used by another environment")
			}
			return err
		}
		return s.audit(ctx, tx, p, auditEntry{action: "environment.updated", projectID: pc.project.ID, envID: env.ID,
			targetType: "environment", targetID: env.ID, meta: meta})
	})
	return env, err
}

// DeleteEnvironment deletes an environment and its secrets. confirm must
// equal the environment slug.
func (s *Service) DeleteEnvironment(ctx context.Context, p *authz.Principal, slug, envSlug, confirm string) error {
	return s.db.WithTx(ctx, func(tx store.Store) error {
		pc, err := s.resolveProject(ctx, tx, p, slug)
		if err != nil {
			return err
		}
		env, err := s.resolveEnv(ctx, tx, p, pc, envSlug)
		if err != nil {
			return err
		}
		if err := s.require(p, pc, authz.ManageEnvironments, ""); err != nil {
			return err
		}
		if confirm != env.Slug {
			return invalid("confirm", "must be the environment slug")
		}
		if err := tx.Environments().Delete(ctx, env.ID); err != nil {
			return err
		}
		return s.audit(ctx, tx, p, auditEntry{action: "environment.deleted", projectID: pc.project.ID, envID: env.ID,
			targetType: "environment", targetID: env.ID, meta: map[string]any{"name": env.Name, "slug": env.Slug}})
	})
}

// ProjectAudit lists a project's audit log (editors and owners).
func (s *Service) ProjectAudit(ctx context.Context, p *authz.Principal, slug string, q AuditQuery) (AuditPage, error) {
	pc, err := s.resolveProject(ctx, s.db, p, slug)
	if err != nil {
		return AuditPage{}, err
	}
	if err := s.require(p, pc, authz.ViewAudit, ""); err != nil {
		return AuditPage{}, err
	}
	return s.listAudit(ctx, pc.project.ID, q)
}
