// Package storetest is the behaviour suite every store.DB implementation
// must pass. Each dialect's tests call Run with a fresh, migrated database.
package storetest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/thomasbudiarjo/envily/internal/store"
)

// Opener returns a new empty, migrated database for one test.
type Opener func(t *testing.T) store.DB

func Run(t *testing.T, open Opener) {
	tests := []struct {
		name string
		fn   func(t *testing.T, s store.DB)
	}{
		{"Meta", testMeta},
		{"Users", testUsers},
		{"Sessions", testSessions},
		{"ProjectsAndMembers", testProjects},
		{"Environments", testEnvironments},
		{"Secrets", testSecrets},
		{"Tokens", testTokens},
		{"Audit", testAudit},
		{"Transactions", testTransactions},
		{"Cascade", testCascade},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := open(t)
			t.Cleanup(func() { _ = s.Close() })
			tc.fn(t, s)
		})
	}
}

var ctx = context.Background()

// t0 is a fixed time with millisecond precision, the precision of storage.
var t0 = time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)

func id() string { return uuid.Must(uuid.NewV7()).String() }

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func wantErr(t *testing.T, err, target error) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Fatalf("got error %v, want %v", err, target)
	}
}

func eq[T comparable](t *testing.T, what string, got, want T) {
	t.Helper()
	if got != want {
		t.Fatalf("%s: got %v, want %v", what, got, want)
	}
}

func mkUser(t *testing.T, s store.Store, email string) store.User {
	t.Helper()
	u := store.User{ID: id(), Email: email, Name: "User " + email, PasswordHash: "$argon2id$x",
		CreatedAt: t0, UpdatedAt: t0}
	must(t, s.Users().Create(ctx, u))
	return u
}

func mkProject(t *testing.T, s store.Store, slug string) store.Project {
	t.Helper()
	p := store.Project{ID: id(), Name: "Project " + slug, Slug: slug, WrappedDEK: []byte{1, 2, 3},
		DEKRootVersion: 1, CreatedAt: t0, UpdatedAt: t0}
	must(t, s.Projects().Create(ctx, p))
	return p
}

func mkEnv(t *testing.T, s store.Store, projectID, slug string, pos int64) store.Environment {
	t.Helper()
	e := store.Environment{ID: id(), ProjectID: projectID, Name: slug, Slug: slug, Position: pos, CreatedAt: t0}
	must(t, s.Environments().Create(ctx, e))
	return e
}

// mkSecret creates a live secret with version 1.
func mkSecret(t *testing.T, s store.Store, envID, key string, value string) store.Secret {
	t.Helper()
	sec := store.Secret{ID: id(), EnvironmentID: envID, Key: key, CurrentVersion: 1, CreatedAt: t0, UpdatedAt: t0}
	must(t, s.Secrets().Create(ctx, sec))
	must(t, s.Secrets().CreateVersion(ctx, store.SecretVersion{ID: id(), SecretID: sec.ID, Version: 1,
		ValueCT: []byte(value), ActorType: "user", ActorID: "u", CreatedAt: t0}))
	return sec
}

func testMeta(t *testing.T, s store.DB) {
	_, err := s.Meta().Get(ctx, store.MetaKeyCheck)
	wantErr(t, err, store.ErrNotFound)
	must(t, s.Meta().Set(ctx, store.MetaKeyCheck, "a"))
	must(t, s.Meta().Set(ctx, store.MetaKeyCheck, "b"))
	v, err := s.Meta().Get(ctx, store.MetaKeyCheck)
	must(t, err)
	eq(t, "meta value", v, "b")
	ok, err := s.Meta().Insert(ctx, store.MetaSetupDone, "1")
	must(t, err)
	eq(t, "first insert", ok, true)
	ok, err = s.Meta().Insert(ctx, store.MetaSetupDone, "2")
	must(t, err)
	eq(t, "second insert", ok, false)
	must(t, s.Ping(ctx))
}

func testUsers(t *testing.T, s store.DB) {
	n, err := s.Users().Count(ctx)
	must(t, err)
	eq(t, "empty count", n, 0)

	a := mkUser(t, s, "b@example.com")
	mkUser(t, s, "a@example.com")
	dup := store.User{ID: id(), Email: "b@example.com", Name: "x", PasswordHash: "x", CreatedAt: t0, UpdatedAt: t0}
	wantErr(t, s.Users().Create(ctx, dup), store.ErrConflict)

	got, err := s.Users().GetByEmail(ctx, "b@example.com")
	must(t, err)
	eq(t, "id", got.ID, a.ID)
	eq(t, "created_at", got.CreatedAt, t0)
	if got.DisabledAt != nil || got.IsAdmin || got.MustChangePassword {
		t.Fatalf("unexpected flags %+v", got)
	}

	disabled := t0.Add(time.Hour)
	got.Name, got.IsAdmin, got.MustChangePassword, got.DisabledAt, got.UpdatedAt = "Renamed", true, true, &disabled, disabled
	must(t, s.Users().Update(ctx, got))
	got, err = s.Users().Get(ctx, a.ID)
	must(t, err)
	if got.Name != "Renamed" || !got.IsAdmin || !got.MustChangePassword || got.DisabledAt == nil || !got.DisabledAt.Equal(disabled) {
		t.Fatalf("update not persisted: %+v", got)
	}

	list, err := s.Users().List(ctx)
	must(t, err)
	if len(list) != 2 || list[0].Email != "a@example.com" {
		t.Fatalf("list order: %+v", list)
	}
	_, err = s.Users().Get(ctx, "missing")
	wantErr(t, err, store.ErrNotFound)
	wantErr(t, s.Users().Update(ctx, store.User{ID: "missing"}), store.ErrNotFound)
}

func testSessions(t *testing.T, s store.DB) {
	u := mkUser(t, s, "a@example.com")
	mk := func(hash string, expires time.Time) store.Session {
		x := store.Session{ID: id(), UserID: u.ID, TokenHash: []byte(hash), CreatedAt: t0, LastSeenAt: t0,
			ExpiresAt: expires, IP: "10.0.0.1", UserAgent: "test"}
		must(t, s.Sessions().Create(ctx, x))
		return x
	}
	keep := mk("h1", t0.Add(time.Hour))
	other := mk("h2", t0.Add(time.Hour))
	expired := mk("h3", t0.Add(-time.Minute))
	wantErr(t, s.Sessions().Create(ctx, store.Session{ID: id(), UserID: u.ID, TokenHash: []byte("h1"),
		CreatedAt: t0, LastSeenAt: t0, ExpiresAt: t0}), store.ErrConflict)
	wantErr(t, s.Sessions().Create(ctx, store.Session{ID: id(), UserID: "missing", TokenHash: []byte("h9"),
		CreatedAt: t0, LastSeenAt: t0, ExpiresAt: t0}), store.ErrNotFound)

	got, err := s.Sessions().GetByHash(ctx, []byte("h1"))
	must(t, err)
	eq(t, "session id", got.ID, keep.ID)
	eq(t, "ip", got.IP, "10.0.0.1")

	must(t, s.Sessions().Touch(ctx, keep.ID, t0.Add(time.Minute)))
	got, _ = s.Sessions().GetByHash(ctx, []byte("h1"))
	eq(t, "last_seen_at", got.LastSeenAt, t0.Add(time.Minute))

	must(t, s.Sessions().DeleteExpired(ctx, t0))
	_, err = s.Sessions().GetByHash(ctx, expired.TokenHash)
	wantErr(t, err, store.ErrNotFound)

	must(t, s.Sessions().DeleteForUser(ctx, u.ID, keep.ID))
	_, err = s.Sessions().GetByHash(ctx, other.TokenHash)
	wantErr(t, err, store.ErrNotFound)
	_, err = s.Sessions().GetByHash(ctx, keep.TokenHash)
	must(t, err)

	must(t, s.Sessions().Delete(ctx, keep.ID))
	_, err = s.Sessions().GetByHash(ctx, keep.TokenHash)
	wantErr(t, err, store.ErrNotFound)
}

func testProjects(t *testing.T, s store.DB) {
	alice := mkUser(t, s, "alice@example.com")
	bob := mkUser(t, s, "bob@example.com")
	p := mkProject(t, s, "web")
	q := mkProject(t, s, "api")
	dup := p
	dup.ID = id()
	wantErr(t, s.Projects().Create(ctx, dup), store.ErrConflict)

	got, err := s.Projects().GetBySlug(ctx, "web")
	must(t, err)
	if got.ID != p.ID || !bytes.Equal(got.WrappedDEK, []byte{1, 2, 3}) || got.DEKRootVersion != 1 {
		t.Fatalf("project %+v", got)
	}

	must(t, s.Projects().Update(ctx, p.ID, "Website", "website", t0.Add(time.Hour)))
	wantErr(t, s.Projects().Update(ctx, q.ID, "x", "website", t0), store.ErrConflict)
	must(t, s.Projects().UpdateDEK(ctx, p.ID, []byte{9}, 2))
	got, _ = s.Projects().Get(ctx, p.ID)
	if got.Slug != "website" || got.Name != "Website" || got.DEKRootVersion != 2 || !bytes.Equal(got.WrappedDEK, []byte{9}) {
		t.Fatalf("project after update %+v", got)
	}

	must(t, s.Projects().AddMember(ctx, store.Member{ProjectID: p.ID, UserID: alice.ID, Role: store.RoleOwner, CreatedAt: t0}))
	must(t, s.Projects().AddMember(ctx, store.Member{ProjectID: p.ID, UserID: bob.ID, Role: store.RoleViewer, CreatedAt: t0}))
	must(t, s.Projects().AddMember(ctx, store.Member{ProjectID: q.ID, UserID: bob.ID, Role: store.RoleOwner, CreatedAt: t0}))
	wantErr(t, s.Projects().AddMember(ctx, store.Member{ProjectID: p.ID, UserID: bob.ID, Role: store.RoleEditor, CreatedAt: t0}), store.ErrConflict)
	wantErr(t, s.Projects().AddMember(ctx, store.Member{ProjectID: p.ID, UserID: "missing", Role: store.RoleEditor, CreatedAt: t0}), store.ErrNotFound)

	members, err := s.Projects().ListMembers(ctx, p.ID)
	must(t, err)
	if len(members) != 2 || members[0].Email != "alice@example.com" || members[1].Role != store.RoleViewer {
		t.Fatalf("members %+v", members)
	}
	owners, _ := s.Projects().CountOwners(ctx, p.ID)
	eq(t, "owners", owners, 1)
	must(t, s.Projects().UpdateMemberRole(ctx, p.ID, bob.ID, store.RoleOwner))
	owners, _ = s.Projects().CountOwners(ctx, p.ID)
	eq(t, "owners after promote", owners, 2)
	m, err := s.Projects().GetMember(ctx, p.ID, bob.ID)
	must(t, err)
	eq(t, "role", m.Role, store.RoleOwner)

	mine, err := s.Projects().ListForUser(ctx, alice.ID)
	must(t, err)
	if len(mine) != 1 || mine[0].ID != p.ID {
		t.Fatalf("alice's projects %+v", mine)
	}
	all, _ := s.Projects().List(ctx)
	eq(t, "all projects", len(all), 2)

	must(t, s.Projects().RemoveMember(ctx, p.ID, bob.ID))
	wantErr(t, s.Projects().RemoveMember(ctx, p.ID, bob.ID), store.ErrNotFound)
	_, err = s.Projects().GetMember(ctx, p.ID, bob.ID)
	wantErr(t, err, store.ErrNotFound)

	must(t, s.Projects().Delete(ctx, q.ID))
	wantErr(t, s.Projects().Delete(ctx, q.ID), store.ErrNotFound)
}

func testEnvironments(t *testing.T, s store.DB) {
	p := mkProject(t, s, "web")
	q := mkProject(t, s, "api")
	prod := mkEnv(t, s, p.ID, "production", 2)
	mkEnv(t, s, p.ID, "development", 0)
	mkEnv(t, s, q.ID, "production", 0) // same slug in another project is fine
	wantErr(t, s.Environments().Create(ctx, store.Environment{ID: id(), ProjectID: p.ID, Name: "x", Slug: "production", CreatedAt: t0}), store.ErrConflict)

	list, err := s.Environments().List(ctx, p.ID)
	must(t, err)
	if len(list) != 2 || list[0].Slug != "development" || list[1].Slug != "production" {
		t.Fatalf("environments ordered by position: %+v", list)
	}
	got, err := s.Environments().GetBySlug(ctx, p.ID, "production")
	must(t, err)
	eq(t, "env id", got.ID, prod.ID)

	got.Name, got.Slug, got.Position = "Prod", "prod", 5
	must(t, s.Environments().Update(ctx, got))
	got, _ = s.Environments().Get(ctx, prod.ID)
	if got.Name != "Prod" || got.Slug != "prod" || got.Position != 5 {
		t.Fatalf("env after update %+v", got)
	}
	must(t, s.Environments().Delete(ctx, prod.ID))
	_, err = s.Environments().Get(ctx, prod.ID)
	wantErr(t, err, store.ErrNotFound)
}

func testSecrets(t *testing.T, s store.DB) {
	p := mkProject(t, s, "web")
	dev := mkEnv(t, s, p.ID, "development", 0)
	prod := mkEnv(t, s, p.ID, "production", 1)

	b := mkSecret(t, s, dev.ID, "B_KEY", "b1")
	a := mkSecret(t, s, dev.ID, "A_KEY", "a1")
	mkSecret(t, s, prod.ID, "A_KEY", "prod")
	wantErr(t, s.Secrets().Create(ctx, store.Secret{ID: id(), EnvironmentID: dev.ID, Key: "A_KEY", CurrentVersion: 1,
		CreatedAt: t0, UpdatedAt: t0}), store.ErrConflict)

	// New version with a comment.
	must(t, s.Secrets().CreateVersion(ctx, store.SecretVersion{ID: id(), SecretID: a.ID, Version: 2, ValueCT: []byte("a2"),
		CommentCT: []byte("c2"), ActorType: "service_token", ActorID: "tok", CreatedAt: t0.Add(time.Minute)}))
	wantErr(t, s.Secrets().CreateVersion(ctx, store.SecretVersion{ID: id(), SecretID: a.ID, Version: 2, ValueCT: []byte("x"),
		ActorType: "user", ActorID: "u", CreatedAt: t0}), store.ErrConflict)
	a.CurrentVersion, a.UpdatedAt = 2, t0.Add(time.Minute)
	must(t, s.Secrets().Update(ctx, a))

	live, err := s.Secrets().ListLive(ctx, dev.ID)
	must(t, err)
	if len(live) != 2 || live[0].Key != "A_KEY" || live[1].Key != "B_KEY" {
		t.Fatalf("live secrets: %+v", live)
	}
	if string(live[0].ValueCT) != "a2" || string(live[0].CommentCT) != "c2" || live[0].ActorType != "service_token" || live[0].CurrentVersion != 2 {
		t.Fatalf("current version not joined: %+v", live[0])
	}
	if live[1].CommentCT != nil {
		t.Fatalf("missing comment should be nil, got %q", live[1].CommentCT)
	}

	keys, err := s.Secrets().ListProjectKeys(ctx, p.ID)
	must(t, err)
	eq(t, "project keys", len(keys), 3)

	// Soft delete frees the key for a new secret.
	deleted := t0.Add(2 * time.Minute)
	b.DeletedAt, b.UpdatedAt = &deleted, deleted
	must(t, s.Secrets().Update(ctx, b))
	_, err = s.Secrets().GetLive(ctx, dev.ID, "B_KEY")
	wantErr(t, err, store.ErrNotFound)
	b2 := mkSecret(t, s, dev.ID, "B_KEY", "fresh")
	got, err := s.Secrets().GetLive(ctx, dev.ID, "B_KEY")
	must(t, err)
	eq(t, "recreated secret", got.ID, b2.ID)
	live, _ = s.Secrets().ListLive(ctx, dev.ID)
	eq(t, "live after soft delete", len(live), 2)

	// Renaming onto an existing live key conflicts.
	a.Key = "B_KEY"
	wantErr(t, s.Secrets().Update(ctx, a), store.ErrConflict)

	// Versions and pruning.
	for v := int64(3); v <= 5; v++ {
		must(t, s.Secrets().CreateVersion(ctx, store.SecretVersion{ID: id(), SecretID: a.ID, Version: v,
			ValueCT: []byte(fmt.Sprint("a", v)), ActorType: "user", ActorID: "u", CreatedAt: t0}))
	}
	must(t, s.Secrets().PruneVersions(ctx, a.ID, 4))
	versions, err := s.Secrets().ListVersions(ctx, a.ID)
	must(t, err)
	if len(versions) != 2 || versions[0].Version != 5 || versions[1].Version != 4 {
		t.Fatalf("versions after prune: %+v", versions)
	}
	v4, err := s.Secrets().GetVersion(ctx, a.ID, 4)
	must(t, err)
	eq(t, "v4 value", string(v4.ValueCT), "a4")
	_, err = s.Secrets().GetVersion(ctx, a.ID, 1)
	wantErr(t, err, store.ErrNotFound)
}

func testTokens(t *testing.T, s store.DB) {
	u := mkUser(t, s, "a@example.com")
	p := mkProject(t, s, "web")
	dev := mkEnv(t, s, p.ID, "development", 0)
	prod := mkEnv(t, s, p.ID, "production", 1)
	exp := t0.Add(24 * time.Hour)

	svc := store.ServiceToken{ID: id(), ProjectID: p.ID, Name: "ci", TokenHash: []byte("svc1"), Permission: store.PermRead,
		CreatedBy: u.ID, ExpiresAt: &exp, CreatedAt: t0, EnvironmentIDs: []string{dev.ID, prod.ID}}
	must(t, s.Tokens().CreateService(ctx, svc))
	svc2 := store.ServiceToken{ID: id(), ProjectID: p.ID, Name: "deploy", TokenHash: []byte("svc2"),
		Permission: store.PermReadWrite, CreatedBy: u.ID, CreatedAt: t0.Add(time.Second), EnvironmentIDs: []string{prod.ID}}
	must(t, s.Tokens().CreateService(ctx, svc2))

	// A failing environment link rolls the whole token back.
	bad := store.ServiceToken{ID: id(), ProjectID: p.ID, Name: "bad", TokenHash: []byte("svc3"), Permission: store.PermRead,
		CreatedBy: u.ID, CreatedAt: t0, EnvironmentIDs: []string{dev.ID, "missing"}}
	wantErr(t, s.Tokens().CreateService(ctx, bad), store.ErrNotFound)
	_, err := s.Tokens().GetService(ctx, bad.ID)
	wantErr(t, err, store.ErrNotFound)

	got, err := s.Tokens().GetServiceByHash(ctx, []byte("svc1"))
	must(t, err)
	if got.ID != svc.ID || got.Permission != store.PermRead || got.ExpiresAt == nil || !got.ExpiresAt.Equal(exp) || len(got.EnvironmentIDs) != 2 {
		t.Fatalf("service token %+v", got)
	}
	list, err := s.Tokens().ListService(ctx, p.ID)
	must(t, err)
	if len(list) != 2 || list[0].ID != svc2.ID || len(list[0].EnvironmentIDs) != 1 || list[0].EnvironmentIDs[0] != prod.ID {
		t.Fatalf("service tokens %+v", list)
	}

	must(t, s.Tokens().TouchService(ctx, svc.ID, t0.Add(time.Hour)))
	must(t, s.Tokens().RevokeService(ctx, svc.ID, t0.Add(2*time.Hour)))
	wantErr(t, s.Tokens().RevokeService(ctx, svc.ID, t0.Add(3*time.Hour)), store.ErrNotFound)
	got, _ = s.Tokens().GetService(ctx, svc.ID)
	if got.LastUsedAt == nil || got.RevokedAt == nil || !got.RevokedAt.Equal(t0.Add(2*time.Hour)) {
		t.Fatalf("revoked token %+v", got)
	}

	// Deleting an environment drops it from token scopes.
	must(t, s.Environments().Delete(ctx, dev.ID))
	got, _ = s.Tokens().GetService(ctx, svc.ID)
	eq(t, "scopes after env delete", len(got.EnvironmentIDs), 1)

	pat := store.PersonalToken{ID: id(), UserID: u.ID, Name: "laptop", TokenHash: []byte("pat1"), CreatedAt: t0}
	must(t, s.Tokens().CreatePersonal(ctx, pat))
	must(t, s.Tokens().CreatePersonal(ctx, store.PersonalToken{ID: id(), UserID: u.ID, Name: "ci", TokenHash: []byte("pat2"), CreatedAt: t0}))
	wantErr(t, s.Tokens().CreatePersonal(ctx, store.PersonalToken{ID: id(), UserID: u.ID, Name: "dup", TokenHash: []byte("pat1"), CreatedAt: t0}), store.ErrConflict)
	gotPat, err := s.Tokens().GetPersonalByHash(ctx, []byte("pat1"))
	must(t, err)
	if gotPat.ID != pat.ID || gotPat.ExpiresAt != nil {
		t.Fatalf("personal token %+v", gotPat)
	}
	must(t, s.Tokens().TouchPersonal(ctx, pat.ID, t0.Add(time.Minute)))
	wantErr(t, s.Tokens().RevokePersonal(ctx, "someone-else", pat.ID, t0), store.ErrNotFound)
	must(t, s.Tokens().RevokePersonal(ctx, u.ID, pat.ID, t0))
	must(t, s.Tokens().RevokeAllPersonal(ctx, u.ID, t0.Add(time.Hour)))
	pats, err := s.Tokens().ListPersonal(ctx, u.ID)
	must(t, err)
	for _, x := range pats {
		if x.RevokedAt == nil {
			t.Fatalf("token %s not revoked", x.Name)
		}
		if x.ID == pat.ID && (!x.RevokedAt.Equal(t0) || x.LastUsedAt == nil) {
			t.Fatalf("first revocation overwritten or touch lost: %+v", x)
		}
	}
}

func testAudit(t *testing.T, s store.DB) {
	p := mkProject(t, s, "web")
	for i := 0; i < 7; i++ {
		l := store.AuditLog{ID: id(), At: t0.Add(time.Duration(i/2) * time.Minute), ActorType: "user", ActorID: "u1",
			ActorLabel: "a@example.com", Action: "secret.read", TargetType: "secret", TargetID: "s", Metadata: `{"key":"A"}`}
		if i%2 == 0 {
			l.ProjectID, l.Action = p.ID, "secret.updated"
		}
		must(t, s.Audit().Insert(ctx, l))
	}

	// Page through everything: rows sharing a timestamp must not be skipped or repeated.
	var all []store.AuditLog
	var after *store.AuditCursor
	for page := 0; ; page++ {
		rows, next, err := s.Audit().List(ctx, store.AuditFilter{Limit: 3, After: after})
		must(t, err)
		all = append(all, rows...)
		if next == nil {
			break
		}
		after = next
		if page > 5 {
			t.Fatal("pagination does not terminate")
		}
	}
	eq(t, "paged rows", len(all), 7)
	seen := map[string]bool{}
	for i, l := range all {
		if seen[l.ID] {
			t.Fatalf("row %s repeated", l.ID)
		}
		seen[l.ID] = true
		if i > 0 && l.At.After(all[i-1].At) {
			t.Fatal("rows not newest first")
		}
	}

	rows, _, err := s.Audit().List(ctx, store.AuditFilter{ProjectID: p.ID, Limit: 50})
	must(t, err)
	eq(t, "project rows", len(rows), 4)
	eq(t, "project id round trip", rows[0].ProjectID, p.ID)
	rows, _, _ = s.Audit().List(ctx, store.AuditFilter{Action: "secret.read", Limit: 50})
	eq(t, "action filter", len(rows), 3)
	rows, _, _ = s.Audit().List(ctx, store.AuditFilter{From: t0.Add(time.Minute), To: t0.Add(2 * time.Minute), Limit: 50})
	eq(t, "time filter", len(rows), 4)
	rows, _, _ = s.Audit().List(ctx, store.AuditFilter{ActorID: "nobody", Limit: 50})
	eq(t, "actor filter", len(rows), 0)

	n, err := s.Audit().DeleteBefore(ctx, t0.Add(time.Minute))
	must(t, err)
	eq(t, "deleted", n, 2)

	// Audit rows survive project deletion (no foreign key).
	must(t, s.Projects().Delete(ctx, p.ID))
	rows, _, _ = s.Audit().List(ctx, store.AuditFilter{ProjectID: p.ID, Limit: 50})
	eq(t, "rows after project delete", len(rows), 3)
}

func testTransactions(t *testing.T, s store.DB) {
	boom := errors.New("boom")
	err := s.WithTx(ctx, func(tx store.Store) error {
		mkUser(t, tx, "rolled-back@example.com")
		// Nested WithTx joins the outer transaction.
		return tx.WithTx(ctx, func(inner store.Store) error {
			mkProject(t, inner, "rolled-back")
			return boom
		})
	})
	wantErr(t, err, boom)
	n, _ := s.Users().Count(ctx)
	eq(t, "users after rollback", n, 0)
	_, err = s.Projects().GetBySlug(ctx, "rolled-back")
	wantErr(t, err, store.ErrNotFound)

	func() {
		defer func() { _ = recover() }()
		_ = s.WithTx(ctx, func(tx store.Store) error {
			mkUser(t, tx, "panicked@example.com")
			panic("boom")
		})
	}()
	n, _ = s.Users().Count(ctx)
	eq(t, "users after panic", n, 0)

	must(t, s.WithTx(ctx, func(tx store.Store) error {
		u := mkUser(t, tx, "committed@example.com")
		got, err := tx.Users().Get(ctx, u.ID) // reads inside a tx see its writes
		if err != nil || got.Email != u.Email {
			return fmt.Errorf("read inside tx: %w", err)
		}
		return nil
	}))
	n, _ = s.Users().Count(ctx)
	eq(t, "users after commit", n, 1)
}

func testCascade(t *testing.T, s store.DB) {
	u := mkUser(t, s, "a@example.com")
	p := mkProject(t, s, "web")
	e := mkEnv(t, s, p.ID, "development", 0)
	sec := mkSecret(t, s, e.ID, "KEY", "v")
	must(t, s.Projects().AddMember(ctx, store.Member{ProjectID: p.ID, UserID: u.ID, Role: store.RoleOwner, CreatedAt: t0}))
	must(t, s.Tokens().CreateService(ctx, store.ServiceToken{ID: id(), ProjectID: p.ID, Name: "ci", TokenHash: []byte("h"),
		Permission: store.PermRead, CreatedBy: u.ID, CreatedAt: t0, EnvironmentIDs: []string{e.ID}}))

	must(t, s.Projects().Delete(ctx, p.ID))
	_, err := s.Environments().Get(ctx, e.ID)
	wantErr(t, err, store.ErrNotFound)
	_, err = s.Secrets().GetVersion(ctx, sec.ID, 1)
	wantErr(t, err, store.ErrNotFound)
	_, err = s.Tokens().GetServiceByHash(ctx, []byte("h"))
	wantErr(t, err, store.ErrNotFound)
	mine, _ := s.Projects().ListForUser(ctx, u.ID)
	eq(t, "memberships after delete", len(mine), 0)
}
