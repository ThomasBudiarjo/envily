package httpapi

import (
	"strings"
	"testing"
	"time"
)

const tokBase = "/api/v1/projects/web-app/service-tokens"

func (f *projectFixture) serviceToken(t *testing.T, perm string, envs ...string) (string, string) {
	t.Helper()
	r := f.owner.do("POST", tokBase, map[string]any{"name": "ci-" + perm, "environments": envs, "permission": perm}).want(201).json()
	return r["token"].(string), r["info"].(map[string]any)["id"].(string)
}

func TestServiceTokenScopes(t *testing.T) {
	f := newProjectFixture(t)
	f.owner.do("PUT", secBase+"/DEV", map[string]string{"value": "d"}).want(201)
	prod := "/api/v1/projects/web-app/environments/production/secrets"
	f.owner.do("PUT", prod+"/PROD", map[string]string{"value": "p"}).want(201)
	f.owner.do("POST", "/api/v1/projects", map[string]string{"name": "Other"}).want(201)

	readTok, _ := f.serviceToken(t, "read", "development")
	rwTok, rwID := f.serviceToken(t, "read_write", "development")
	read, rw := f.e.bearer(readTok), f.e.bearer(rwTok)
	if !strings.HasPrefix(readTok, "envily_svc_") {
		t.Fatalf("token %q", readTok)
	}

	// Read in scope; nothing out of scope (404 so other environments do not leak).
	if v := read.do("GET", secBase+"/DEV", nil).want(200).json()["value"]; v != "d" {
		t.Fatalf("value %v", v)
	}
	read.do("GET", prod+"/PROD", nil).want(404)
	read.do("GET", prod+":export", nil).want(404)
	read.do("GET", "/api/v1/projects/other", nil).want(404)
	read.do("PUT", secBase+"/NEW", map[string]string{"value": "x"}).want(403)
	read.do("GET", secBase+"/DEV/versions", nil).want(403)

	rw.do("PUT", secBase+"/NEW", map[string]string{"value": "x"}).want(201)
	rw.do("PUT", prod+"/NEW", map[string]string{"value": "x"}).want(404)
	rw.do("POST", secBase+"/NEW/rollback", map[string]int{"version": 1}).want(403)
	rw.do("GET", "/api/v1/projects/web-app/members", nil).want(403)
	rw.do("GET", tokBase, nil).want(403)
	rw.do("POST", "/api/v1/projects", map[string]string{"name": "Mine"}).want(403)
	rw.do("GET", "/api/v1/auth/me", nil).want(403)

	// The project shows only environments in scope.
	envs := read.do("GET", "/api/v1/projects/web-app", nil).want(200).json()["environments"].([]any)
	if len(envs) != 1 || envs[0].(map[string]any)["slug"] != "development" {
		t.Fatalf("visible envs %v", envs)
	}
	projects := read.do("GET", "/api/v1/projects", nil).want(200).json()["items"].([]any)
	if len(projects) != 1 {
		t.Fatalf("projects %v", projects)
	}
	info := read.do("GET", "/api/v1/token/info", nil).want(200).json()
	if info["permission"] != "read" || info["project"].(map[string]any)["slug"] != "web-app" || len(info["environments"].([]any)) != 1 {
		t.Fatalf("token info %v", info)
	}
	f.owner.do("GET", "/api/v1/token/info", nil).want(400)

	// Versions written by a token are attributed to it.
	item := f.owner.do("GET", secBase, nil).json()["items"].([]any)
	for _, it := range item {
		m := it.(map[string]any)
		if m["key"] == "NEW" && m["updatedBy"].(map[string]any)["label"] != "ci-read_write" {
			t.Fatalf("updatedBy %v", m["updatedBy"])
		}
	}

	// Listing never includes the token value; revoke takes effect immediately.
	list := f.owner.do("GET", tokBase, nil).want(200)
	if strings.Contains(string(list.body), rwTok) || strings.Contains(string(list.body), rwTok[11:]) {
		t.Fatal("token value listed")
	}
	if f.viewer.do("GET", tokBase, nil).status != 403 || f.editor.do("POST", tokBase, map[string]any{}).status != 403 {
		t.Fatal("non-owners manage tokens")
	}
	f.owner.do("DELETE", tokBase+"/"+rwID, nil).want(204)
	rw.do("GET", secBase+"/DEV", nil).want(401)
	f.owner.do("DELETE", tokBase+"/"+rwID, nil).want(404)
	f.owner.do("DELETE", "/api/v1/projects/other/service-tokens/"+rwID, nil).want(404)
}

func TestServiceTokenValidationAndExpiry(t *testing.T) {
	f := newProjectFixture(t)
	for name, body := range map[string]map[string]any{
		"no envs":     {"name": "x", "permission": "read", "environments": []string{}},
		"unknown env": {"name": "x", "permission": "read", "environments": []string{"qa"}},
		"bad perm":    {"name": "x", "permission": "admin", "environments": []string{"development"}},
		"past expiry": {"name": "x", "permission": "read", "environments": []string{"development"}, "expiresAt": f.e.clock.Now().Add(-time.Hour)},
	} {
		if r := f.owner.do("POST", tokBase, body); r.status != 400 {
			t.Errorf("%s: %d %s", name, r.status, r.body)
		}
	}
	r := f.owner.do("POST", tokBase, map[string]any{"name": "short", "permission": "read", "environments": []string{"development"},
		"expiresAt": f.e.clock.Now().Add(time.Hour)}).want(201)
	c := f.e.bearer(r.json()["token"].(string))
	c.do("GET", secBase, nil).want(200)
	f.e.clock.Add(time.Hour)
	c.do("GET", secBase, nil).want(401)

	// Deleting the environment removes it from the scope.
	tok, _ := f.serviceToken(t, "read", "staging")
	f.owner.do("DELETE", "/api/v1/projects/web-app/environments/staging", map[string]string{"confirm": "staging"}).want(204)
	f.e.bearer(tok).do("GET", "/api/v1/projects/web-app/environments/staging/secrets", nil).want(404)
	for _, a := range []string{"service_token.created"} {
		found := false
		for _, x := range f.e.auditActions() {
			found = found || x == a
		}
		if !found {
			t.Errorf("audit lacks %s", a)
		}
	}
}
