package httpapi

import (
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSetupLoginMeLogout(t *testing.T) {
	e := newEnv(t)
	anon := e.browser()
	if anon.do("GET", "/api/v1/setup/status", nil).want(200).json()["needsSetup"] != true {
		t.Fatal("fresh instance should need setup")
	}
	admin := e.setupAdmin()
	if anon.do("GET", "/api/v1/setup/status", nil).want(200).json()["needsSetup"] != false {
		t.Fatal("setup status not updated")
	}

	me := admin.do("GET", "/api/v1/auth/me", nil).want(200).json()
	user := me["user"].(map[string]any)
	if user["email"] != "admin@example.com" || me["isAdmin"] != true || me["mustChangePassword"] != false {
		t.Fatalf("me = %v", me)
	}

	// Second setup is refused.
	r := e.browser().do("POST", "/api/v1/setup", map[string]string{"email": "x@example.com", "name": "X", "password": adminPassword})
	if r.status != http.StatusConflict || r.code() != "conflict" {
		t.Fatalf("second setup: %d %s", r.status, r.body)
	}

	// A fresh login gets a new session; both work until logout.
	second := e.login("ADMIN@example.com", adminPassword)
	second.do("GET", "/api/v1/auth/me", nil).want(200)
	second.do("POST", "/api/v1/auth/logout", nil).want(http.StatusNoContent)
	if r := second.do("GET", "/api/v1/auth/me", nil); r.status != 401 || r.code() != "unauthorized" {
		t.Fatalf("after logout: %d %s", r.status, r.body)
	}
	admin.do("GET", "/api/v1/auth/me", nil).want(200)

	want := []string{"user.created", "auth.login", "auth.logout"}
	if got := e.auditActions(); !slices.Equal(got, want) {
		t.Fatalf("audit = %v, want %v", got, want)
	}
}

func TestSetupConcurrentOnlyOneWins(t *testing.T) {
	e := newEnv(t)
	var wg sync.WaitGroup
	statuses := make(chan int, 5)
	for i := range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := e.browser().do("POST", "/api/v1/setup", map[string]string{
				"email": strings.Repeat("a", i+1) + "@example.com", "name": "A", "password": adminPassword})
			statuses <- r.status
		}()
	}
	wg.Wait()
	close(statuses)
	created := 0
	for s := range statuses {
		if s == http.StatusCreated {
			created++
		} else if s != http.StatusConflict && s != http.StatusTooManyRequests {
			t.Fatalf("unexpected status %d", s)
		}
	}
	if created != 1 {
		t.Fatalf("%d setups succeeded", created)
	}
}

func TestSetupToken(t *testing.T) {
	e := newEnv(t, withSetupToken("s3cret-setup"))
	body := map[string]string{"email": "a@example.com", "name": "A", "password": adminPassword, "setupToken": "wrong"}
	if r := e.browser().do("POST", "/api/v1/setup", body); r.status != 400 || r.code() != "validation_failed" {
		t.Fatalf("wrong setup token: %d %s", r.status, r.body)
	}
	body["setupToken"] = "s3cret-setup"
	e.browser().do("POST", "/api/v1/setup", body).want(http.StatusCreated)
}

func TestSetupValidation(t *testing.T) {
	e := newEnv(t)
	r := e.browser().do("POST", "/api/v1/setup", map[string]string{"email": "not-an-email", "name": "", "password": "short"})
	if r.status != 400 || r.code() != "validation_failed" {
		t.Fatalf("%d %s", r.status, r.body)
	}
	details := r.json()["error"].(map[string]any)["details"].(map[string]any)
	for _, f := range []string{"email", "name", "password"} {
		if details[f] == nil {
			t.Errorf("no detail for %s: %v", f, details)
		}
	}
	if strings.Contains(string(r.body), "short") {
		t.Error("validation error echoes the password")
	}
	if r := e.browser().do("POST", "/api/v1/setup", `{"email":`); r.status != 400 || r.code() != "bad_request" {
		t.Fatalf("bad json: %d %s", r.status, r.body)
	}
}

func TestLoginFailuresAreUniform(t *testing.T) {
	e := newEnv(t)
	e.setupAdmin()
	unknown := e.browser().do("POST", "/api/v1/auth/login", map[string]string{"email": "nobody@example.com", "password": adminPassword})
	wrong := e.browser().do("POST", "/api/v1/auth/login", map[string]string{"email": "admin@example.com", "password": "wrong password!!"})
	if unknown.status != 401 || wrong.status != 401 || string(unknown.body) != string(wrong.body) {
		t.Fatalf("responses differ:\n%d %s\n%d %s", unknown.status, unknown.body, wrong.status, wrong.body)
	}
	if unknown.code() != "invalid_credentials" {
		t.Fatalf("code %s", unknown.code())
	}
	if n := strings.Count(strings.Join(e.auditActions(), ","), "auth.login_failed"); n != 2 {
		t.Fatalf("login_failed audit rows: %d", n)
	}
	if len(unknown.header.Values("Set-Cookie")) != 0 {
		t.Fatal("failed login set a cookie")
	}
}

func TestLoginRateLimit(t *testing.T) {
	e := newEnv(t, withRealLimits())
	e.setupAdmin() // uses one token of the per-IP bucket
	var last resp
	for i := 0; i < 6; i++ {
		last = e.browser().do("POST", "/api/v1/auth/login", map[string]string{"email": "admin@example.com", "password": "wrong password!!"})
	}
	if last.status != http.StatusTooManyRequests || last.code() != "rate_limited" || last.header.Get("Retry-After") == "" {
		t.Fatalf("expected 429 with Retry-After, got %d %v %s", last.status, last.header, last.body)
	}
}

func TestCSRF(t *testing.T) {
	e := newEnv(t)
	admin := e.setupAdmin()
	for name, hdr := range map[string][]string{
		"foreign origin": {"Origin", "https://evil.example"},
		"no client hdr":  {"X-Envily-Client", ""},
		"no origin":      {"Origin", ""},
	} {
		r := admin.do("POST", "/api/v1/users", map[string]any{"email": "x@example.com", "name": "x"}, hdr...)
		if r.status != 403 || r.code() != "forbidden" {
			t.Errorf("%s: %d %s", name, r.status, r.body)
		}
	}
	// Referer works when Origin is absent.
	admin.do("POST", "/api/v1/users", map[string]any{"email": "x@example.com", "name": "x"},
		"Origin", "", "Referer", e.origin+"/admin/users").want(http.StatusCreated)
	// GETs are not CSRF-checked.
	admin.do("GET", "/api/v1/auth/me", nil, "Origin", "https://evil.example").want(200)
	// Anonymous login from another site is rejected; the CLI sends no Origin.
	if r := e.browser().do("POST", "/api/v1/auth/login", map[string]string{"email": "a@b.co", "password": "x"},
		"Origin", "https://evil.example"); r.status != 403 {
		t.Errorf("cross-site login: %d", r.status)
	}
}

func TestSessionCookieAndTimeouts(t *testing.T) {
	e := newEnv(t)
	c := e.browser()
	r := c.do("POST", "/api/v1/setup", map[string]string{"email": "a@example.com", "name": "A", "password": adminPassword}).want(201)
	cookie := r.header.Get("Set-Cookie")
	for _, want := range []string{"envily_session=", "HttpOnly", "SameSite=Strict", "Path=/"} {
		if !strings.Contains(cookie, want) {
			t.Errorf("cookie %q lacks %s", cookie, want)
		}
	}
	if strings.Contains(cookie, "Secure") {
		t.Error("Secure set for an http:// public URL")
	}

	// Activity within the idle timeout keeps the session alive.
	for i := 0; i < 3; i++ {
		e.clock.Add(11 * time.Hour)
		c.do("GET", "/api/v1/auth/me", nil).want(200)
	}
	// Idle for longer than 12h ends it.
	e.clock.Add(13 * time.Hour)
	c.do("GET", "/api/v1/auth/me", nil).want(401)

	// The absolute limit applies even with constant activity.
	c = e.login("a@example.com", adminPassword)
	for i := 0; i < 16; i++ {
		e.clock.Add(10 * time.Hour)
		c.do("GET", "/api/v1/auth/me", nil).want(200)
	}
	e.clock.Add(10 * time.Hour) // 170h > 168h
	c.do("GET", "/api/v1/auth/me", nil).want(401)
}

func TestUserManagementAndPasswordChange(t *testing.T) {
	e := newEnv(t)
	admin := e.setupAdmin()

	r := admin.do("POST", "/api/v1/users", map[string]any{"email": "bob@example.com", "name": "Bob"}).want(201)
	tmp := r.json()["temporaryPassword"].(string)
	bobID := r.json()["user"].(map[string]any)["id"].(string)
	if len(tmp) < 12 || r.json()["user"].(map[string]any)["mustChangePassword"] != true {
		t.Fatalf("create user: %s", r.body)
	}
	if r := admin.do("POST", "/api/v1/users", map[string]any{"email": "BOB@example.com", "name": "Bob"}); r.status != 400 {
		t.Fatalf("duplicate email: %d", r.status)
	}

	bob := e.login("bob@example.com", tmp)
	// Only me, password and logout until the password is changed.
	if r := bob.do("GET", "/api/v1/me/tokens", nil); r.status != 403 || r.code() != "password_change_required" {
		t.Fatalf("must-change gate: %d %s", r.status, r.body)
	}
	bob.do("GET", "/api/v1/auth/me", nil).want(200)
	if r := bob.do("POST", "/api/v1/auth/password", map[string]string{"currentPassword": "wrong", "newPassword": "a new long password"}); r.status != 400 {
		t.Fatalf("wrong current password: %d", r.status)
	}
	other := e.login("bob@example.com", tmp)
	bob.do("POST", "/api/v1/auth/password", map[string]string{"currentPassword": tmp, "newPassword": "a new long password"}).want(204)
	bob.do("GET", "/api/v1/me/tokens", nil).want(200)
	other.do("GET", "/api/v1/auth/me", nil).want(401) // other sessions signed out

	// Non-admins cannot manage users.
	if r := bob.do("GET", "/api/v1/users", nil); r.status != 403 {
		t.Fatalf("non-admin list users: %d", r.status)
	}
	if r := bob.do("GET", "/api/v1/audit", nil); r.status != 403 {
		t.Fatalf("non-admin audit: %d", r.status)
	}

	// Personal token, then disable: the session and token stop working at once.
	tok := bob.do("POST", "/api/v1/me/tokens", map[string]any{"name": "laptop"}).want(201).json()["token"].(string)
	if !strings.HasPrefix(tok, "envily_pat_") {
		t.Fatalf("token %q", tok)
	}
	e.bearer(tok).do("GET", "/api/v1/auth/me", nil).want(200)
	admin.do("PATCH", "/api/v1/users/"+bobID, map[string]any{"disabled": true}).want(200)
	bob.do("GET", "/api/v1/auth/me", nil).want(401)
	e.bearer(tok).do("GET", "/api/v1/auth/me", nil).want(401)
	if r := e.browser().do("POST", "/api/v1/auth/login", map[string]string{"email": "bob@example.com", "password": "a new long password"}); r.status != 401 {
		t.Fatalf("disabled login: %d", r.status)
	}

	// Re-enable with a password reset.
	r = admin.do("PATCH", "/api/v1/users/"+bobID, map[string]any{"disabled": false, "resetPassword": true}).want(200)
	e.login("bob@example.com", r.json()["temporaryPassword"].(string))

	// Admins cannot lock themselves out.
	adminID := admin.do("GET", "/api/v1/auth/me", nil).json()["user"].(map[string]any)["id"].(string)
	if r := admin.do("PATCH", "/api/v1/users/"+adminID, map[string]any{"isAdmin": false}); r.status != 400 {
		t.Fatalf("self demote: %d", r.status)
	}

	users := admin.do("GET", "/api/v1/users", nil).want(200).json()["items"].([]any)
	if len(users) != 2 {
		t.Fatalf("users: %v", users)
	}
	page := admin.do("GET", "/api/v1/audit?limit=2", nil).want(200).json()
	if len(page["items"].([]any)) != 2 || page["nextCursor"] == nil {
		t.Fatalf("audit page: %v", page)
	}
	next := admin.do("GET", "/api/v1/audit?limit=200&cursor="+page["nextCursor"].(string), nil).want(200).json()
	if len(next["items"].([]any)) == 0 {
		t.Fatal("second audit page empty")
	}
	for _, a := range []string{"user.created", "auth.password_changed", "personal_token.created", "user.disabled", "user.updated"} {
		if !slices.Contains(e.auditActions(), a) {
			t.Errorf("audit lacks %s: %v", a, e.auditActions())
		}
	}
}

func TestCLILoginAndPersonalTokens(t *testing.T) {
	e := newEnv(t)
	e.setupAdmin()
	cli := &client{e: e, hc: &http.Client{}} // no Origin, no cookie
	r := cli.do("POST", "/api/v1/auth/cli-login", map[string]string{"email": "admin@example.com", "password": adminPassword, "tokenName": "laptop"}).want(201)
	tok := r.json()["token"].(string)
	exp, err := time.Parse(time.RFC3339Nano, r.json()["expiresAt"].(string))
	if err != nil || exp.Sub(e.clock.Now()) != 90*24*time.Hour {
		t.Fatalf("expiresAt %v %v", exp, err)
	}
	pat := e.bearer(tok)
	pat.do("GET", "/api/v1/users", nil).want(200) // inherits admin

	// Bearer requests skip CSRF.
	pat.do("POST", "/api/v1/me/tokens", map[string]any{"name": "second"}, "Origin", "https://evil.example").want(201)

	list := pat.do("GET", "/api/v1/me/tokens", nil).want(200).json()["items"].([]any)
	if len(list) != 2 {
		t.Fatalf("tokens %v", list)
	}
	if strings.Contains(string(pat.do("GET", "/api/v1/me/tokens", nil).body), tok) {
		t.Fatal("token value listed")
	}
	var id string
	for _, x := range list {
		if x.(map[string]any)["name"] == "laptop" {
			id = x.(map[string]any)["id"].(string)
		}
	}
	pat.do("DELETE", "/api/v1/me/tokens/"+id, nil).want(204)
	pat.do("GET", "/api/v1/auth/me", nil).want(401)

	// Expired tokens are rejected.
	tok2 := e.login("admin@example.com", adminPassword).do("POST", "/api/v1/me/tokens",
		map[string]any{"name": "short", "expiresAt": e.clock.Now().Add(time.Hour)}).want(201).json()["token"].(string)
	e.bearer(tok2).do("GET", "/api/v1/auth/me", nil).want(200)
	e.clock.Add(time.Hour)
	e.bearer(tok2).do("GET", "/api/v1/auth/me", nil).want(401)
	e.bearer("envily_pat_garbage").do("GET", "/api/v1/auth/me", nil).want(401)
	e.bearer("nonsense").do("GET", "/api/v1/auth/me", nil).want(401)
}

func TestRequestIDAndHeadersOnAPI(t *testing.T) {
	e := newEnv(t)
	r := e.browser().do("GET", "/api/v1/setup/status", nil, "X-Request-Id", "abc-123").want(200)
	if r.header.Get("X-Request-Id") != "abc-123" || r.header.Get("Cache-Control") != "no-store" {
		t.Fatalf("headers %v", r.header)
	}
	if !strings.Contains(e.logOutput(), `"request_id":"abc-123"`) {
		t.Fatal("request id not logged")
	}
}
