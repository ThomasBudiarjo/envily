// This file is the dialect-independent store implementation. It is written
// against the sqlc-generated package, whose Go API is identical for both
// dialects; `make generate` copies it to store/postgres/repos_gen.go with only
// the import path and package name changed. Edit it here, not in the copy.

package sqlite

import (
	"context"
	"database/sql"
	"math"
	"time"

	"github.com/thomasbudiarjo/envily/internal/store"
	"github.com/thomasbudiarjo/envily/internal/store/sqlite/db"
)

// base implements store.Store on top of sqlc queries. r serves reads and w
// serves writes; they differ only for SQLite outside a transaction, where
// reads use a pool of connections and writes a single one.
type base struct {
	r, w *db.Queries
	// begin starts a transaction on the write connection; nil inside one.
	begin func(ctx context.Context) (*sql.Tx, error)
}

func (s *base) Users() store.Users               { return users{s} }
func (s *base) Sessions() store.Sessions         { return sessions{s} }
func (s *base) Projects() store.Projects         { return projects{s} }
func (s *base) Environments() store.Environments { return environments{s} }
func (s *base) Secrets() store.Secrets           { return secrets{s} }
func (s *base) Tokens() store.Tokens             { return tokens{s} }
func (s *base) Audit() store.Audit               { return audit{s} }
func (s *base) Meta() store.Meta                 { return meta{s} }

func (s *base) WithTx(ctx context.Context, fn func(tx store.Store) error) (err error) {
	if s.begin == nil {
		return fn(s)
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return mapErr(err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	defer func() {
		if v := recover(); v != nil {
			_ = tx.Rollback()
			panic(v)
		}
	}()
	q := db.New(tx)
	if err = fn(&base{r: q, w: q}); err != nil {
		return err
	}
	return mapErr(tx.Commit())
}

// ---- conversion helpers ----

func ms(t time.Time) int64            { return store.Millis(t) }
func fromMs(v int64) time.Time        { return store.FromMillis(v) }
func nullStr(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }

func nullTime(t *time.Time) sql.NullInt64 {
	if t == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: ms(*t), Valid: true}
}

func ptrTime(v sql.NullInt64) *time.Time {
	if !v.Valid {
		return nil
	}
	t := fromMs(v.Int64)
	return &t
}

// one maps the error of a single-row query.
func one[T any](v T, err error) (T, error) { return v, mapErr(err) }

// rows maps the result of an :execrows query: zero rows is ErrNotFound.
func rows(n int64, err error) error {
	if err != nil {
		return mapErr(err)
	}
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

// ---- users ----

type users struct{ *base }

func toUser(u db.User) store.User {
	return store.User{
		ID: u.ID, Email: u.Email, Name: u.Name, PasswordHash: u.PasswordHash,
		IsAdmin: u.IsAdmin, MustChangePassword: u.MustChangePassword,
		CreatedAt: fromMs(u.CreatedAt), UpdatedAt: fromMs(u.UpdatedAt), DisabledAt: ptrTime(u.DisabledAt),
	}
}

func (s users) Create(ctx context.Context, u store.User) error {
	return mapErr(s.w.CreateUser(ctx, db.CreateUserParams{
		ID: u.ID, Email: u.Email, Name: u.Name, PasswordHash: u.PasswordHash,
		IsAdmin: u.IsAdmin, MustChangePassword: u.MustChangePassword,
		CreatedAt: ms(u.CreatedAt), UpdatedAt: ms(u.UpdatedAt), DisabledAt: nullTime(u.DisabledAt),
	}))
}

func (s users) Get(ctx context.Context, id string) (store.User, error) {
	u, err := one(s.r.GetUser(ctx, db.GetUserParams{ID: id}))
	return toUser(u), err
}

func (s users) GetByEmail(ctx context.Context, email string) (store.User, error) {
	u, err := one(s.r.GetUserByEmail(ctx, db.GetUserByEmailParams{Email: email}))
	return toUser(u), err
}

func (s users) List(ctx context.Context) ([]store.User, error) {
	list, err := s.r.ListUsers(ctx)
	out := make([]store.User, len(list))
	for i, u := range list {
		out[i] = toUser(u)
	}
	return out, mapErr(err)
}

func (s users) Count(ctx context.Context) (int64, error) { return one(s.r.CountUsers(ctx)) }

func (s users) Update(ctx context.Context, u store.User) error {
	return rows(s.w.UpdateUser(ctx, db.UpdateUserParams{
		ID: u.ID, Name: u.Name, PasswordHash: u.PasswordHash, IsAdmin: u.IsAdmin,
		MustChangePassword: u.MustChangePassword, UpdatedAt: ms(u.UpdatedAt), DisabledAt: nullTime(u.DisabledAt),
	}))
}

// ---- sessions ----

type sessions struct{ *base }

func (s sessions) Create(ctx context.Context, x store.Session) error {
	return mapErr(s.w.CreateSession(ctx, db.CreateSessionParams{
		ID: x.ID, UserID: x.UserID, TokenHash: x.TokenHash, CreatedAt: ms(x.CreatedAt),
		LastSeenAt: ms(x.LastSeenAt), ExpiresAt: ms(x.ExpiresAt), Ip: x.IP, UserAgent: x.UserAgent,
	}))
}

func (s sessions) GetByHash(ctx context.Context, hash []byte) (store.Session, error) {
	x, err := one(s.r.GetSessionByHash(ctx, db.GetSessionByHashParams{TokenHash: hash}))
	return store.Session{
		ID: x.ID, UserID: x.UserID, TokenHash: x.TokenHash, CreatedAt: fromMs(x.CreatedAt),
		LastSeenAt: fromMs(x.LastSeenAt), ExpiresAt: fromMs(x.ExpiresAt), IP: x.Ip, UserAgent: x.UserAgent,
	}, err
}

func (s sessions) Touch(ctx context.Context, id string, at time.Time) error {
	return mapErr(s.w.TouchSession(ctx, db.TouchSessionParams{ID: id, LastSeenAt: ms(at)}))
}

func (s sessions) Delete(ctx context.Context, id string) error {
	return mapErr(s.w.DeleteSession(ctx, db.DeleteSessionParams{ID: id}))
}

func (s sessions) DeleteForUser(ctx context.Context, userID, exceptID string) error {
	return mapErr(s.w.DeleteUserSessions(ctx, db.DeleteUserSessionsParams{UserID: userID, ExceptID: exceptID}))
}

func (s sessions) DeleteExpired(ctx context.Context, now time.Time) error {
	return mapErr(s.w.DeleteExpiredSessions(ctx, db.DeleteExpiredSessionsParams{Now: ms(now)}))
}

// ---- projects and members ----

type projects struct{ *base }

func toProject(p db.Project) store.Project {
	return store.Project{
		ID: p.ID, Name: p.Name, Slug: p.Slug, WrappedDEK: p.WrappedDek, DEKRootVersion: p.DekRootVersion,
		CreatedAt: fromMs(p.CreatedAt), UpdatedAt: fromMs(p.UpdatedAt),
	}
}

func toProjects(list []db.Project, err error) ([]store.Project, error) {
	out := make([]store.Project, len(list))
	for i, p := range list {
		out[i] = toProject(p)
	}
	return out, mapErr(err)
}

func (s projects) Create(ctx context.Context, p store.Project) error {
	return mapErr(s.w.CreateProject(ctx, db.CreateProjectParams{
		ID: p.ID, Name: p.Name, Slug: p.Slug, WrappedDek: p.WrappedDEK, DekRootVersion: p.DEKRootVersion,
		CreatedAt: ms(p.CreatedAt), UpdatedAt: ms(p.UpdatedAt),
	}))
}

func (s projects) Get(ctx context.Context, id string) (store.Project, error) {
	p, err := one(s.r.GetProject(ctx, db.GetProjectParams{ID: id}))
	return toProject(p), err
}

func (s projects) GetBySlug(ctx context.Context, slug string) (store.Project, error) {
	p, err := one(s.r.GetProjectBySlug(ctx, db.GetProjectBySlugParams{Slug: slug}))
	return toProject(p), err
}

func (s projects) List(ctx context.Context) ([]store.Project, error) {
	return toProjects(s.r.ListProjects(ctx))
}

func (s projects) ListForUser(ctx context.Context, userID string) ([]store.Project, error) {
	return toProjects(s.r.ListProjectsForUser(ctx, db.ListProjectsForUserParams{UserID: userID}))
}

func (s projects) Update(ctx context.Context, id, name, slug string, at time.Time) error {
	return rows(s.w.UpdateProject(ctx, db.UpdateProjectParams{ID: id, Name: name, Slug: slug, UpdatedAt: ms(at)}))
}

func (s projects) UpdateDEK(ctx context.Context, id string, wrapped []byte, rootVersion int64) error {
	return rows(s.w.UpdateProjectDEK(ctx, db.UpdateProjectDEKParams{ID: id, WrappedDek: wrapped, DekRootVersion: rootVersion}))
}

func (s projects) Touch(ctx context.Context, id string, at time.Time) error {
	return mapErr(s.w.TouchProject(ctx, db.TouchProjectParams{ID: id, UpdatedAt: ms(at)}))
}

func (s projects) Delete(ctx context.Context, id string) error {
	return rows(s.w.DeleteProject(ctx, db.DeleteProjectParams{ID: id}))
}

func (s projects) AddMember(ctx context.Context, m store.Member) error {
	return mapErr(s.w.AddMember(ctx, db.AddMemberParams{
		ProjectID: m.ProjectID, UserID: m.UserID, Role: string(m.Role), CreatedAt: ms(m.CreatedAt),
	}))
}

func (s projects) GetMember(ctx context.Context, projectID, userID string) (store.Member, error) {
	m, err := one(s.r.GetMember(ctx, db.GetMemberParams{ProjectID: projectID, UserID: userID}))
	return store.Member{ProjectID: m.ProjectID, UserID: m.UserID, Role: store.Role(m.Role), CreatedAt: fromMs(m.CreatedAt)}, err
}

func (s projects) ListMembers(ctx context.Context, projectID string) ([]store.Member, error) {
	list, err := s.r.ListMembers(ctx, db.ListMembersParams{ProjectID: projectID})
	out := make([]store.Member, len(list))
	for i, m := range list {
		out[i] = store.Member{
			ProjectID: m.ProjectID, UserID: m.UserID, Role: store.Role(m.Role), CreatedAt: fromMs(m.CreatedAt),
			Email: m.Email, Name: m.Name,
		}
	}
	return out, mapErr(err)
}

func (s projects) UpdateMemberRole(ctx context.Context, projectID, userID string, role store.Role) error {
	return rows(s.w.UpdateMemberRole(ctx, db.UpdateMemberRoleParams{ProjectID: projectID, UserID: userID, Role: string(role)}))
}

func (s projects) RemoveMember(ctx context.Context, projectID, userID string) error {
	return rows(s.w.RemoveMember(ctx, db.RemoveMemberParams{ProjectID: projectID, UserID: userID}))
}

func (s projects) CountOwners(ctx context.Context, projectID string) (int64, error) {
	return one(s.r.CountOwners(ctx, db.CountOwnersParams{ProjectID: projectID}))
}

// ---- environments ----

type environments struct{ *base }

func toEnv(e db.Environment) store.Environment {
	return store.Environment{
		ID: e.ID, ProjectID: e.ProjectID, Name: e.Name, Slug: e.Slug, Position: e.Position, CreatedAt: fromMs(e.CreatedAt),
	}
}

func (s environments) Create(ctx context.Context, e store.Environment) error {
	return mapErr(s.w.CreateEnvironment(ctx, db.CreateEnvironmentParams{
		ID: e.ID, ProjectID: e.ProjectID, Name: e.Name, Slug: e.Slug, Position: e.Position, CreatedAt: ms(e.CreatedAt),
	}))
}

func (s environments) Get(ctx context.Context, id string) (store.Environment, error) {
	e, err := one(s.r.GetEnvironment(ctx, db.GetEnvironmentParams{ID: id}))
	return toEnv(e), err
}

func (s environments) GetBySlug(ctx context.Context, projectID, slug string) (store.Environment, error) {
	e, err := one(s.r.GetEnvironmentBySlug(ctx, db.GetEnvironmentBySlugParams{ProjectID: projectID, Slug: slug}))
	return toEnv(e), err
}

func (s environments) List(ctx context.Context, projectID string) ([]store.Environment, error) {
	list, err := s.r.ListEnvironments(ctx, db.ListEnvironmentsParams{ProjectID: projectID})
	out := make([]store.Environment, len(list))
	for i, e := range list {
		out[i] = toEnv(e)
	}
	return out, mapErr(err)
}

func (s environments) Update(ctx context.Context, e store.Environment) error {
	return rows(s.w.UpdateEnvironment(ctx, db.UpdateEnvironmentParams{ID: e.ID, Name: e.Name, Slug: e.Slug, Position: e.Position}))
}

func (s environments) Delete(ctx context.Context, id string) error {
	return rows(s.w.DeleteEnvironment(ctx, db.DeleteEnvironmentParams{ID: id}))
}

// ---- secrets ----

type secrets struct{ *base }

func toSecret(x db.Secret) store.Secret {
	return store.Secret{
		ID: x.ID, EnvironmentID: x.EnvironmentID, Key: x.Key, CurrentVersion: x.CurrentVersion,
		CreatedAt: fromMs(x.CreatedAt), UpdatedAt: fromMs(x.UpdatedAt), DeletedAt: ptrTime(x.DeletedAt),
	}
}

func toVersion(v db.SecretVersion) store.SecretVersion {
	return store.SecretVersion{
		ID: v.ID, SecretID: v.SecretID, Version: v.Version, ValueCT: v.ValueCt, CommentCT: v.CommentCt,
		ActorType: v.ActorType, ActorID: v.ActorID, CreatedAt: fromMs(v.CreatedAt),
	}
}

func (s secrets) Create(ctx context.Context, x store.Secret) error {
	return mapErr(s.w.CreateSecret(ctx, db.CreateSecretParams{
		ID: x.ID, EnvironmentID: x.EnvironmentID, Key: x.Key, CurrentVersion: x.CurrentVersion,
		CreatedAt: ms(x.CreatedAt), UpdatedAt: ms(x.UpdatedAt),
	}))
}

func (s secrets) GetLive(ctx context.Context, environmentID, key string) (store.Secret, error) {
	x, err := one(s.r.GetLiveSecret(ctx, db.GetLiveSecretParams{EnvironmentID: environmentID, Key: key}))
	return toSecret(x), err
}

func (s secrets) Update(ctx context.Context, x store.Secret) error {
	return rows(s.w.UpdateSecret(ctx, db.UpdateSecretParams{
		ID: x.ID, Key: x.Key, CurrentVersion: x.CurrentVersion, UpdatedAt: ms(x.UpdatedAt), DeletedAt: nullTime(x.DeletedAt),
	}))
}

func (s secrets) ListLive(ctx context.Context, environmentID string) ([]store.LiveSecret, error) {
	list, err := s.r.ListLiveSecrets(ctx, db.ListLiveSecretsParams{EnvironmentID: environmentID})
	out := make([]store.LiveSecret, len(list))
	for i, x := range list {
		out[i] = store.LiveSecret{
			Secret: store.Secret{
				ID: x.ID, EnvironmentID: x.EnvironmentID, Key: x.Key, CurrentVersion: x.CurrentVersion,
				CreatedAt: fromMs(x.CreatedAt), UpdatedAt: fromMs(x.UpdatedAt),
			},
			ValueCT: x.ValueCt, CommentCT: x.CommentCt, ActorType: x.ActorType, ActorID: x.ActorID,
		}
	}
	return out, mapErr(err)
}

func (s secrets) ListProjectKeys(ctx context.Context, projectID string) ([]store.SecretKey, error) {
	list, err := s.r.ListProjectSecretKeys(ctx, db.ListProjectSecretKeysParams{ProjectID: projectID})
	out := make([]store.SecretKey, len(list))
	for i, x := range list {
		out[i] = store.SecretKey{EnvironmentID: x.EnvironmentID, Key: x.Key, CurrentVersion: x.CurrentVersion, UpdatedAt: fromMs(x.UpdatedAt)}
	}
	return out, mapErr(err)
}

func (s secrets) CreateVersion(ctx context.Context, v store.SecretVersion) error {
	return mapErr(s.w.CreateSecretVersion(ctx, db.CreateSecretVersionParams{
		ID: v.ID, SecretID: v.SecretID, Version: v.Version, ValueCt: v.ValueCT, CommentCt: v.CommentCT,
		ActorType: v.ActorType, ActorID: v.ActorID, CreatedAt: ms(v.CreatedAt),
	}))
}

func (s secrets) GetVersion(ctx context.Context, secretID string, version int64) (store.SecretVersion, error) {
	v, err := one(s.r.GetSecretVersion(ctx, db.GetSecretVersionParams{SecretID: secretID, Version: version}))
	return toVersion(v), err
}

func (s secrets) ListVersions(ctx context.Context, secretID string) ([]store.SecretVersion, error) {
	list, err := s.r.ListSecretVersions(ctx, db.ListSecretVersionsParams{SecretID: secretID})
	out := make([]store.SecretVersion, len(list))
	for i, v := range list {
		out[i] = toVersion(v)
	}
	return out, mapErr(err)
}

func (s secrets) PruneVersions(ctx context.Context, secretID string, minVersion int64) error {
	return mapErr(s.w.PruneSecretVersions(ctx, db.PruneSecretVersionsParams{SecretID: secretID, MinVersion: minVersion}))
}

// ---- tokens ----

type tokens struct{ *base }

func toServiceToken(t db.ServiceToken) store.ServiceToken {
	return store.ServiceToken{
		ID: t.ID, ProjectID: t.ProjectID, Name: t.Name, TokenHash: t.TokenHash, Permission: store.Permission(t.Permission),
		CreatedBy: t.CreatedBy, ExpiresAt: ptrTime(t.ExpiresAt), LastUsedAt: ptrTime(t.LastUsedAt),
		RevokedAt: ptrTime(t.RevokedAt), CreatedAt: fromMs(t.CreatedAt), EnvironmentIDs: []string{},
	}
}

func toPersonalToken(t db.PersonalToken) store.PersonalToken {
	return store.PersonalToken{
		ID: t.ID, UserID: t.UserID, Name: t.Name, TokenHash: t.TokenHash, ExpiresAt: ptrTime(t.ExpiresAt),
		LastUsedAt: ptrTime(t.LastUsedAt), RevokedAt: ptrTime(t.RevokedAt), CreatedAt: fromMs(t.CreatedAt),
	}
}

// withEnvironments fills EnvironmentIDs for tokens of one project.
func (s tokens) withEnvironments(ctx context.Context, projectID string, list []store.ServiceToken) error {
	links, err := s.r.ListServiceTokenEnvironments(ctx, db.ListServiceTokenEnvironmentsParams{ProjectID: projectID})
	if err != nil {
		return mapErr(err)
	}
	index := make(map[string]int, len(list))
	for i := range list {
		index[list[i].ID] = i
	}
	for _, l := range links {
		if i, ok := index[l.TokenID]; ok {
			list[i].EnvironmentIDs = append(list[i].EnvironmentIDs, l.EnvironmentID)
		}
	}
	return nil
}

func (s tokens) CreateService(ctx context.Context, t store.ServiceToken) error {
	return s.WithTx(ctx, func(tx store.Store) error {
		q := tx.(*base).w
		if err := q.CreateServiceToken(ctx, db.CreateServiceTokenParams{
			ID: t.ID, ProjectID: t.ProjectID, Name: t.Name, TokenHash: t.TokenHash, Permission: string(t.Permission),
			CreatedBy: t.CreatedBy, ExpiresAt: nullTime(t.ExpiresAt), CreatedAt: ms(t.CreatedAt),
		}); err != nil {
			return mapErr(err)
		}
		for _, envID := range t.EnvironmentIDs {
			if err := q.AddServiceTokenEnvironment(ctx, db.AddServiceTokenEnvironmentParams{TokenID: t.ID, EnvironmentID: envID}); err != nil {
				return mapErr(err)
			}
		}
		return nil
	})
}

func (s tokens) getService(ctx context.Context, t db.ServiceToken, err error) (store.ServiceToken, error) {
	if err != nil {
		return store.ServiceToken{}, mapErr(err)
	}
	list := []store.ServiceToken{toServiceToken(t)}
	if err := s.withEnvironments(ctx, t.ProjectID, list); err != nil {
		return store.ServiceToken{}, err
	}
	return list[0], nil
}

func (s tokens) GetService(ctx context.Context, id string) (store.ServiceToken, error) {
	t, err := s.r.GetServiceToken(ctx, db.GetServiceTokenParams{ID: id})
	return s.getService(ctx, t, err)
}

func (s tokens) GetServiceByHash(ctx context.Context, hash []byte) (store.ServiceToken, error) {
	t, err := s.r.GetServiceTokenByHash(ctx, db.GetServiceTokenByHashParams{TokenHash: hash})
	return s.getService(ctx, t, err)
}

func (s tokens) ListService(ctx context.Context, projectID string) ([]store.ServiceToken, error) {
	list, err := s.r.ListServiceTokens(ctx, db.ListServiceTokensParams{ProjectID: projectID})
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]store.ServiceToken, len(list))
	for i, t := range list {
		out[i] = toServiceToken(t)
	}
	return out, s.withEnvironments(ctx, projectID, out)
}

func (s tokens) RevokeService(ctx context.Context, id string, at time.Time) error {
	return rows(s.w.RevokeServiceToken(ctx, db.RevokeServiceTokenParams{ID: id, RevokedAt: sql.NullInt64{Int64: ms(at), Valid: true}}))
}

func (s tokens) TouchService(ctx context.Context, id string, at time.Time) error {
	return mapErr(s.w.TouchServiceToken(ctx, db.TouchServiceTokenParams{ID: id, LastUsedAt: sql.NullInt64{Int64: ms(at), Valid: true}}))
}

func (s tokens) CreatePersonal(ctx context.Context, t store.PersonalToken) error {
	return mapErr(s.w.CreatePersonalToken(ctx, db.CreatePersonalTokenParams{
		ID: t.ID, UserID: t.UserID, Name: t.Name, TokenHash: t.TokenHash, ExpiresAt: nullTime(t.ExpiresAt), CreatedAt: ms(t.CreatedAt),
	}))
}

func (s tokens) GetPersonalByHash(ctx context.Context, hash []byte) (store.PersonalToken, error) {
	t, err := one(s.r.GetPersonalTokenByHash(ctx, db.GetPersonalTokenByHashParams{TokenHash: hash}))
	return toPersonalToken(t), err
}

func (s tokens) ListPersonal(ctx context.Context, userID string) ([]store.PersonalToken, error) {
	list, err := s.r.ListPersonalTokens(ctx, db.ListPersonalTokensParams{UserID: userID})
	out := make([]store.PersonalToken, len(list))
	for i, t := range list {
		out[i] = toPersonalToken(t)
	}
	return out, mapErr(err)
}

func (s tokens) RevokePersonal(ctx context.Context, userID, id string, at time.Time) error {
	return rows(s.w.RevokePersonalToken(ctx, db.RevokePersonalTokenParams{
		ID: id, UserID: userID, RevokedAt: sql.NullInt64{Int64: ms(at), Valid: true},
	}))
}

func (s tokens) RevokeAllPersonal(ctx context.Context, userID string, at time.Time) error {
	return mapErr(s.w.RevokeUserPersonalTokens(ctx, db.RevokeUserPersonalTokensParams{
		UserID: userID, RevokedAt: sql.NullInt64{Int64: ms(at), Valid: true},
	}))
}

func (s tokens) TouchPersonal(ctx context.Context, id string, at time.Time) error {
	return mapErr(s.w.TouchPersonalToken(ctx, db.TouchPersonalTokenParams{ID: id, LastUsedAt: sql.NullInt64{Int64: ms(at), Valid: true}}))
}

// ---- audit ----

type audit struct{ *base }

func (s audit) Insert(ctx context.Context, l store.AuditLog) error {
	return mapErr(s.w.InsertAuditLog(ctx, db.InsertAuditLogParams{
		ID: l.ID, At: ms(l.At), ActorType: l.ActorType, ActorID: l.ActorID, ActorLabel: l.ActorLabel, Action: l.Action,
		ProjectID: nullStr(l.ProjectID), EnvironmentID: nullStr(l.EnvironmentID), TargetType: l.TargetType,
		TargetID: l.TargetID, Metadata: l.Metadata, Ip: l.IP, UserAgent: l.UserAgent,
	}))
}

func (s audit) List(ctx context.Context, f store.AuditFilter) ([]store.AuditLog, *store.AuditCursor, error) {
	p := db.ListAuditLogsParams{
		ProjectID: f.ProjectID, Action: f.Action, ActorID: f.ActorID,
		FromAt: math.MinInt64, ToAt: math.MaxInt64, CursorAt: math.MaxInt64, RowLimit: int64(f.Limit) + 1,
	}
	if !f.From.IsZero() {
		p.FromAt = ms(f.From)
	}
	if !f.To.IsZero() {
		p.ToAt = ms(f.To)
	}
	if f.After != nil {
		p.CursorAt, p.CursorID = ms(f.After.At), f.After.ID
	}
	list, err := s.r.ListAuditLogs(ctx, p)
	if err != nil {
		return nil, nil, mapErr(err)
	}
	var next *store.AuditCursor
	if len(list) > f.Limit {
		list = list[:f.Limit]
		last := list[len(list)-1]
		next = &store.AuditCursor{At: fromMs(last.At), ID: last.ID}
	}
	out := make([]store.AuditLog, len(list))
	for i, l := range list {
		out[i] = store.AuditLog{
			ID: l.ID, At: fromMs(l.At), ActorType: l.ActorType, ActorID: l.ActorID, ActorLabel: l.ActorLabel,
			Action: l.Action, ProjectID: l.ProjectID.String, EnvironmentID: l.EnvironmentID.String,
			TargetType: l.TargetType, TargetID: l.TargetID, Metadata: l.Metadata, IP: l.Ip, UserAgent: l.UserAgent,
		}
	}
	return out, next, nil
}

func (s audit) DeleteBefore(ctx context.Context, before time.Time) (int64, error) {
	return one(s.w.DeleteAuditLogsBefore(ctx, db.DeleteAuditLogsBeforeParams{Before: ms(before)}))
}

// ---- meta ----

type meta struct{ *base }

func (s meta) Get(ctx context.Context, key string) (string, error) {
	return one(s.r.GetMeta(ctx, db.GetMetaParams{Key: key}))
}

func (s meta) Set(ctx context.Context, key, value string) error {
	return mapErr(s.w.SetMeta(ctx, db.SetMetaParams{Key: key, Value: value}))
}

func (s meta) Insert(ctx context.Context, key, value string) (bool, error) {
	n, err := s.w.InsertMeta(ctx, db.InsertMetaParams{Key: key, Value: value})
	return n == 1, mapErr(err)
}
