package httpapi

import (
	"net/http"
	"slices"
	"testing"
)

// projectFixture is one project with an owner, editor, viewer, a user who
// is not a member, and an instance admin who is not a member.
type projectFixture struct {
	e                                      *env
	admin, owner, editor, viewer, outsider *client
	ids                                    map[string]string // email -> user id
}

func newProjectFixture(t *testing.T) *projectFixture {
	t.Helper()
	e := newEnv(t)
	f := &projectFixture{e: e, admin: e.setupAdmin(), ids: map[string]string{}}
	f.owner = e.newUser(f.admin, "owner@example.com", false)
	f.editor = e.newUser(f.admin, "editor@example.com", false)
	f.viewer = e.newUser(f.admin, "viewer@example.com", false)
	f.outsider = e.newUser(f.admin, "outsider@example.com", false)
	for _, u := range f.admin.do("GET", "/api/v1/users", nil).want(200).json()["items"].([]any) {
		m := u.(map[string]any)
		f.ids[m["email"].(string)] = m["id"].(string)
	}
	f.owner.do("POST", "/api/v1/projects", map[string]string{"name": "Web App"}).want(http.StatusCreated)
	f.owner.do("POST", "/api/v1/projects/web-app/members", map[string]string{"email": "editor@example.com", "role": "editor"}).want(201)
	f.owner.do("POST", "/api/v1/projects/web-app/members", map[string]string{"userId": f.ids["viewer@example.com"], "role": "viewer"}).want(201)
	return f
}

func TestProjectAuthorizationMatrix(t *testing.T) {
	f := newProjectFixture(t)
	anon := f.e.browser()
	viewerID := f.ids["viewer@example.com"]

	type call struct {
		name, method, path string
		body               any
	}
	calls := []call{
		{"get project", "GET", "/api/v1/projects/web-app", nil},
		{"rename project", "PATCH", "/api/v1/projects/web-app", map[string]string{"name": "Web App"}},
		{"delete project", "DELETE", "/api/v1/projects/web-app", map[string]string{"confirm": "wrong"}},
		{"list members", "GET", "/api/v1/projects/web-app/members", nil},
		{"add member", "POST", "/api/v1/projects/web-app/members", map[string]string{"userId": viewerID, "role": "bogus"}},
		{"update member", "PATCH", "/api/v1/projects/web-app/members/" + viewerID, map[string]string{"role": "viewer"}},
		{"remove member", "DELETE", "/api/v1/projects/web-app/members/missing-user", nil},
		{"list envs", "GET", "/api/v1/projects/web-app/environments", nil},
		{"create env", "POST", "/api/v1/projects/web-app/environments", map[string]string{"name": ""}},
		{"update env", "PATCH", "/api/v1/projects/web-app/environments/staging", map[string]string{"name": "staging"}},
		{"delete env", "DELETE", "/api/v1/projects/web-app/environments/staging", map[string]string{"confirm": "wrong"}},
		{"project audit", "GET", "/api/v1/projects/web-app/audit", nil},
	}
	// Expected status per principal, in the order of calls.
	principals := []struct {
		name string
		c    *client
		want []int
	}{
		{"anonymous", anon, []int{401, 401, 401, 401, 401, 401, 401, 401, 401, 401, 401, 401}},
		{"outsider", f.outsider, []int{404, 404, 404, 404, 404, 404, 404, 404, 404, 404, 404, 404}},
		{"admin non-member", f.admin, []int{200, 403, 403, 200, 403, 403, 403, 200, 403, 403, 403, 403}},
		{"viewer", f.viewer, []int{200, 403, 403, 200, 403, 403, 403, 200, 403, 403, 403, 403}},
		{"editor", f.editor, []int{200, 403, 403, 200, 403, 403, 403, 200, 403, 403, 403, 200}},
		{"owner", f.owner, []int{200, 200, 400, 200, 400, 200, 404, 200, 400, 200, 400, 200}},
	}
	for _, p := range principals {
		for i, c := range calls {
			if r := p.c.do(c.method, c.path, c.body); r.status != p.want[i] {
				t.Errorf("%s / %s: status %d, want %d (%s)", p.name, c.name, r.status, p.want[i], r.body)
			}
		}
	}
	// A project that does not exist looks the same as one you cannot see.
	if r := f.outsider.do("GET", "/api/v1/projects/nope", nil); r.status != 404 {
		t.Errorf("missing project: %d", r.status)
	}
}

func TestProjectLifecycle(t *testing.T) {
	f := newProjectFixture(t)

	p := f.owner.do("GET", "/api/v1/projects/web-app", nil).want(200).json()
	var slugs []string
	for _, e := range p["environments"].([]any) {
		slugs = append(slugs, e.(map[string]any)["slug"].(string))
	}
	if !slices.Equal(slugs, []string{"development", "staging", "production"}) || p["role"] != "owner" {
		t.Fatalf("project %v", p)
	}

	// Listing: members see their projects, admins all, outsiders none.
	count := func(c *client) int {
		return len(c.do("GET", "/api/v1/projects", nil).want(200).json()["items"].([]any))
	}
	if count(f.viewer) != 1 || count(f.outsider) != 0 || count(f.admin) != 1 {
		t.Fatal("project listing visibility wrong")
	}
	if r := f.admin.do("GET", "/api/v1/projects", nil).json()["items"].([]any)[0].(map[string]any); r["role"] != nil {
		t.Fatalf("admin non-member role %v", r["role"])
	}

	// Duplicate slug.
	if r := f.outsider.do("POST", "/api/v1/projects", map[string]string{"name": "web app"}); r.status != 400 || r.code() != "validation_failed" {
		t.Fatalf("duplicate slug: %d %s", r.status, r.body)
	}
	if r := f.outsider.do("POST", "/api/v1/projects", map[string]string{"name": "x", "slug": "Bad Slug"}); r.status != 400 {
		t.Fatalf("bad slug: %d", r.status)
	}

	// Environments: create, rename, reorder, delete with confirmation.
	f.owner.do("POST", "/api/v1/projects/web-app/environments", map[string]string{"name": "QA Env"}).want(201)
	if r := f.owner.do("POST", "/api/v1/projects/web-app/environments", map[string]string{"name": "qa env"}); r.status != 400 {
		t.Fatalf("duplicate env: %d", r.status)
	}
	f.owner.do("PATCH", "/api/v1/projects/web-app/environments/qa-env", map[string]any{"slug": "qa", "position": 0}).want(200)
	envs := f.owner.do("GET", "/api/v1/projects/web-app/environments", nil).want(200).json()["items"].([]any)
	if envs[0].(map[string]any)["slug"] != "development" || len(envs) != 4 {
		t.Fatalf("envs %v", envs)
	}
	f.owner.do("DELETE", "/api/v1/projects/web-app/environments/qa", map[string]string{"confirm": "qa"}).want(204)

	// Rename the project slug; the old one stops resolving.
	f.owner.do("PATCH", "/api/v1/projects/web-app", map[string]string{"slug": "web"}).want(200)
	f.owner.do("GET", "/api/v1/projects/web-app", nil).want(404)
	f.viewer.do("GET", "/api/v1/projects/web", nil).want(200)

	f.owner.do("DELETE", "/api/v1/projects/web", map[string]string{"confirm": "web"}).want(204)
	f.owner.do("GET", "/api/v1/projects/web", nil).want(404)

	actions := f.e.auditActions()
	for _, a := range []string{"project.created", "member.added", "environment.created", "environment.updated",
		"environment.deleted", "project.updated", "project.deleted"} {
		if !slices.Contains(actions, a) {
			t.Errorf("audit lacks %s", a)
		}
	}
}

func TestLastOwnerAndMembership(t *testing.T) {
	f := newProjectFixture(t)
	ownerID, editorID := f.ids["owner@example.com"], f.ids["editor@example.com"]
	base := "/api/v1/projects/web-app/members/"

	// The only owner cannot be demoted, removed, or leave.
	for _, r := range []resp{
		f.owner.do("PATCH", base+ownerID, map[string]string{"role": "editor"}),
		f.owner.do("DELETE", base+ownerID, nil),
	} {
		if r.status != 409 || r.code() != "last_owner" {
			t.Fatalf("last owner: %d %s", r.status, r.body)
		}
	}
	// With a second owner, the first may step down.
	f.owner.do("PATCH", base+editorID, map[string]string{"role": "owner"}).want(200)
	f.owner.do("PATCH", base+ownerID, map[string]string{"role": "viewer"}).want(200)
	f.owner.do("PATCH", base+editorID, map[string]string{"role": "viewer"}).want(403) // no longer owner
	if r := f.editor.do("PATCH", base+editorID, map[string]string{"role": "editor"}); r.code() != "last_owner" {
		t.Fatalf("new sole owner demoting self: %s", r.body)
	}

	// Any member may leave.
	f.viewer.do("DELETE", base+f.ids["viewer@example.com"], nil).want(204)
	f.viewer.do("GET", "/api/v1/projects/web-app", nil).want(404)

	// An admin who is not a member may add only themselves.
	adminID := f.ids["admin@example.com"]
	f.admin.do("POST", "/api/v1/projects/web-app/members", map[string]string{"userId": f.ids["outsider@example.com"], "role": "viewer"}).want(403)
	f.admin.do("POST", "/api/v1/projects/web-app/members", map[string]string{"userId": adminID, "role": "editor"}).want(201)
	f.admin.do("GET", "/api/v1/projects/web-app/audit", nil).want(200)
	if r := f.editor.do("POST", "/api/v1/projects/web-app/members", map[string]string{"userId": adminID, "role": "viewer"}); r.status != 409 {
		t.Fatalf("duplicate member: %d", r.status)
	}

	members := f.editor.do("GET", "/api/v1/projects/web-app/members", nil).want(200).json()["items"].([]any)
	if len(members) != 3 {
		t.Fatalf("members %v", members)
	}
}
