package httpapi

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const secBase = "/api/v1/projects/web-app/environments/development/secrets"

func TestSecretsAreEncryptedAtRest(t *testing.T) {
	f := newProjectFixture(t)
	marker := "plaintext-marker-7f3a9c"
	f.owner.do("PUT", secBase+"/DB_PASSWORD", map[string]string{"value": marker, "comment": "comment-" + marker}).want(201)
	f.owner.do("POST", secBase+":import", "IMPORTED="+marker+"-imp\n").want(200)

	// The value must not appear in the database file or its WAL.
	files, _ := filepath.Glob(f.e.dbPath + "*")
	if len(files) == 0 {
		t.Fatal("no database files")
	}
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(raw, []byte(marker)) {
			t.Fatalf("plaintext found in %s", filepath.Base(path))
		}
		if strings.HasSuffix(path, ".db") && !bytes.Contains(raw, []byte("DB_PASSWORD")) && len(files) == 1 {
			t.Fatal("key name not found; the scan may be reading the wrong file")
		}
	}
	got := f.viewer.do("GET", secBase+"/DB_PASSWORD", nil).want(200).json()
	if got["value"] != marker || got["comment"] != "comment-"+marker {
		t.Fatalf("read back %v", got)
	}
}

func TestSecretCRUDAndVersions(t *testing.T) {
	f := newProjectFixture(t)
	f.owner.do("PUT", secBase+"/API_KEY", map[string]string{"value": "v1"}).want(201)
	r := f.editor.do("PUT", secBase+"/API_KEY", map[string]any{"value": "v2", "comment": "rotated", "expectedVersion": 1}).want(200)
	if r.json()["version"] != float64(2) {
		t.Fatalf("version %v", r.json())
	}

	// List has no values by default; ?values=true includes them.
	list := f.viewer.do("GET", secBase, nil).want(200).json()["items"].([]any)
	item := list[0].(map[string]any)
	if item["value"] != nil || item["hasComment"] != true || item["updatedBy"].(map[string]any)["label"] != "editor@example.com" {
		t.Fatalf("list item %v", item)
	}
	all := f.viewer.do("GET", secBase+"?values=true", nil).want(200).json()["items"].([]any)
	if all[0].(map[string]any)["value"] != "v2" {
		t.Fatalf("values %v", all)
	}

	// Optimistic locking.
	r = f.owner.do("PUT", secBase+"/API_KEY", map[string]any{"value": "stale", "expectedVersion": 1})
	if r.status != 409 || r.code() != "conflict" {
		t.Fatalf("stale write: %d %s", r.status, r.body)
	}
	if r := f.owner.do("PUT", secBase+"/NEW_KEY", map[string]any{"value": "x", "expectedVersion": 3}); r.status != 409 {
		t.Fatalf("expected version on missing key: %d", r.status)
	}

	// Comment is kept when omitted.
	f.owner.do("PUT", secBase+"/API_KEY", map[string]any{"value": "v3"}).want(200)
	if c := f.owner.do("GET", secBase+"/API_KEY", nil).json()["comment"]; c != "rotated" {
		t.Fatalf("comment lost: %v", c)
	}

	// History: viewers see versions without values, editors with values.
	vs := f.viewer.do("GET", secBase+"/API_KEY/versions", nil).want(200).json()["items"].([]any)
	if len(vs) != 3 || vs[0].(map[string]any)["value"] != nil {
		t.Fatalf("viewer versions %v", vs)
	}
	f.viewer.do("GET", secBase+"/API_KEY/versions?values=true", nil).want(403)
	vs = f.editor.do("GET", secBase+"/API_KEY/versions?values=true", nil).want(200).json()["items"].([]any)
	if vs[2].(map[string]any)["value"] != "v1" {
		t.Fatalf("editor versions %v", vs)
	}

	// Rollback creates version 4 with v1's value.
	f.viewer.do("POST", secBase+"/API_KEY/rollback", map[string]int{"version": 1}).want(403)
	if v := f.editor.do("POST", secBase+"/API_KEY/rollback", map[string]int{"version": 1}).want(200).json()["version"]; v != float64(4) {
		t.Fatalf("rollback version %v", v)
	}
	if got := f.owner.do("GET", secBase+"/API_KEY", nil).json(); got["value"] != "v1" || got["version"] != float64(4) {
		t.Fatalf("after rollback %v", got)
	}
	if r := f.editor.do("POST", secBase+"/API_KEY/rollback", map[string]int{"version": 99}); r.status != 400 {
		t.Fatalf("rollback missing version: %d", r.status)
	}

	// Rename keeps the version; delete is soft and the key can be reused.
	f.owner.do("PUT", secBase+"/OTHER", map[string]string{"value": "o"}).want(201)
	if r := f.owner.do("PATCH", secBase+"/API_KEY", map[string]string{"newKey": "OTHER"}); r.status != 409 {
		t.Fatalf("rename onto existing: %d", r.status)
	}
	f.owner.do("PATCH", secBase+"/API_KEY", map[string]string{"newKey": "SERVICE_KEY"}).want(204)
	if v := f.owner.do("GET", secBase+"/SERVICE_KEY", nil).want(200).json()["version"]; v != float64(4) {
		t.Fatalf("version after rename %v", v)
	}
	f.owner.do("GET", secBase+"/API_KEY", nil).want(404)
	f.owner.do("DELETE", secBase+"/OTHER", nil).want(204)
	f.owner.do("GET", secBase+"/OTHER", nil).want(404)
	if v := f.owner.do("PUT", secBase+"/OTHER", map[string]string{"value": "again"}).want(201).json()["version"]; v != float64(1) {
		t.Fatalf("recreated version %v", v)
	}

	// Validation.
	for name, c := range map[string]struct {
		key  string
		body any
	}{
		"bad key":     {"1BAD", map[string]string{"value": "x"}},
		"dash key":    {"MY-KEY", map[string]string{"value": "x"}},
		"too large":   {"BIG", map[string]string{"value": strings.Repeat("x", 64<<10+1)}},
		"big comment": {"C", map[string]string{"value": "x", "comment": strings.Repeat("c", 1025)}},
		"no value":    {"NOVAL", map[string]string{}},
	} {
		if r := f.owner.do("PUT", secBase+"/"+c.key, c.body); r.status != 400 {
			t.Errorf("%s: %d %s", name, r.status, r.body)
		}
	}
	f.owner.do("PUT", secBase+"/EXACT", map[string]string{"value": strings.Repeat("x", 64<<10)}).want(201)

	actions := f.e.auditActions()
	for _, a := range []string{"secret.created", "secret.updated", "secret.read", "secrets.read_all", "secret.rolled_back", "secret.renamed", "secret.deleted"} {
		if !slices.Contains(actions, a) {
			t.Errorf("audit lacks %s", a)
		}
	}
}

func TestVersionsArePrunedTo100(t *testing.T) {
	f := newProjectFixture(t)
	for i := 1; i <= 103; i++ {
		f.owner.do("PUT", secBase+"/K", map[string]string{"value": fmt.Sprint(i)}).want(map[bool]int{true: 201, false: 200}[i == 1])
	}
	vs := f.owner.do("GET", secBase+"/K/versions", nil).want(200).json()["items"].([]any)
	if len(vs) != 100 || vs[0].(map[string]any)["version"] != float64(103) || vs[99].(map[string]any)["version"] != float64(4) {
		t.Fatalf("%d versions, newest %v oldest %v", len(vs), vs[0], vs[len(vs)-1])
	}
}

func TestBatchIsAtomic(t *testing.T) {
	f := newProjectFixture(t)
	f.owner.do("POST", secBase+":batch", map[string]any{"upserts": []map[string]string{{"key": "A", "value": "1"}, {"key": "B", "value": "2"}, {"key": "C", "value": "3"}}}).want(200)

	snapshot := func() string {
		return string(f.owner.do("GET", secBase+"?values=true", nil).want(200).body)
	}
	before := snapshot()

	// One invalid item rejects the whole batch.
	r := f.owner.do("POST", secBase+":batch", map[string]any{
		"upserts": []map[string]string{{"key": "A", "value": "changed"}, {"key": "bad-key", "value": "x"}},
		"deletes": []string{"B"},
	})
	if r.status != 400 {
		t.Fatalf("invalid batch: %d %s", r.status, r.body)
	}
	// A failure inside the transaction (missing key) also rolls back.
	r = f.owner.do("POST", secBase+":batch", map[string]any{
		"upserts": []map[string]string{{"key": "A", "value": "changed"}},
		"deletes": []string{"B", "MISSING"},
	})
	if r.status != 404 {
		t.Fatalf("batch with missing delete: %d %s", r.status, r.body)
	}
	// A version conflict rolls back too.
	r = f.owner.do("POST", secBase+":batch", map[string]any{
		"upserts": []map[string]any{{"key": "A", "value": "changed"}, {"key": "C", "value": "x", "expectedVersion": 7}},
	})
	if r.status != 409 || !strings.Contains(string(r.body), `"C"`) {
		t.Fatalf("batch conflict: %d %s", r.status, r.body)
	}
	if after := snapshot(); after != before {
		t.Fatalf("failed batches changed state:\nbefore %s\nafter  %s", before, after)
	}

	// Deletes, renames, and upserts together.
	res := f.owner.do("POST", secBase+":batch", map[string]any{
		"deletes": []string{"A"},
		"renames": []map[string]string{{"from": "B", "to": "A"}},
		"upserts": []map[string]string{{"key": "C", "value": "33"}, {"key": "D", "value": "4"}},
	}).want(200).json()
	if res["deleted"] != float64(1) || res["renamed"] != float64(1) || res["updated"] != float64(1) || res["created"] != float64(1) {
		t.Fatalf("batch result %v", res)
	}
	out := string(f.owner.do("GET", secBase+":export", nil).want(200).body)
	if out != "A=2\nC=33\nD=4\n" {
		t.Fatalf("export after batch %q", out)
	}
	if r := f.viewer.do("POST", secBase+":batch", map[string]any{"deletes": []string{"A"}}); r.status != 403 {
		t.Fatalf("viewer batch: %d", r.status)
	}
	if r := f.owner.do("POST", secBase+":batch", map[string]any{}); r.status != 400 {
		t.Fatalf("empty batch: %d", r.status)
	}
}

func TestImportMergeOverwriteAndExport(t *testing.T) {
	f := newProjectFixture(t)
	f.owner.do("POST", secBase+":import", "KEEP=1\nSAME=s\n").want(200)

	res := f.owner.do("POST", secBase+":import", "# merge\nexport SAME=s\nNEW='n v'\nMULTI=\"a\\nb\"\n").want(200).json()
	if res["created"] != float64(2) || res["unchanged"] != float64(1) || res["deleted"] != float64(0) {
		t.Fatalf("merge result %v", res)
	}
	keys := func() []string {
		var ks []string
		for _, it := range f.owner.do("GET", secBase, nil).want(200).json()["items"].([]any) {
			ks = append(ks, it.(map[string]any)["key"].(string))
		}
		return ks
	}
	if !slices.Equal(keys(), []string{"KEEP", "MULTI", "NEW", "SAME"}) {
		t.Fatalf("after merge %v", keys())
	}

	res = f.owner.do("POST", secBase+":import?mode=overwrite", `{"SAME":"changed","NEW":"n v"}`).want(200).json()
	if res["updated"] != float64(1) || res["unchanged"] != float64(1) || res["deleted"] != float64(2) {
		t.Fatalf("overwrite result %v", res)
	}
	if !slices.Equal(keys(), []string{"NEW", "SAME"}) {
		t.Fatalf("after overwrite %v", keys())
	}

	for _, bad := range []string{"NOEQUALS", "A=1\nA=2", "bad-key=1", `{"A": 1}`, `{"A":`} {
		if r := f.owner.do("POST", secBase+":import", bad); r.status != 400 {
			t.Errorf("import %q: %d %s", bad, r.status, r.body)
		}
	}
	if r := f.owner.do("POST", secBase+":import?mode=replace", "A=1"); r.status != 400 {
		t.Errorf("bad mode: %d", r.status)
	}

	r := f.viewer.do("GET", secBase+":export?format=json", nil).want(200)
	if r.header.Get("Content-Type") != "application/json; charset=utf-8" || !strings.Contains(r.header.Get("Content-Disposition"), "web-app-development.json") {
		t.Fatalf("export headers %v", r.header)
	}
	if string(r.body) != "{\n  \"NEW\": \"n v\",\n  \"SAME\": \"changed\"\n}\n" {
		t.Fatalf("json export %q", r.body)
	}
	if y := string(f.viewer.do("GET", secBase+":export?format=yaml", nil).want(200).body); y != "NEW: \"n v\"\nSAME: \"changed\"\n" {
		t.Fatalf("yaml export %q", y)
	}
	if d := string(f.viewer.do("GET", secBase+":export", nil).want(200).body); d != "NEW='n v'\nSAME=changed\n" {
		t.Fatalf("dotenv export %q", d)
	}
	f.viewer.do("GET", secBase+":export?format=xml", nil).want(400)
	f.outsider.do("GET", secBase+":export", nil).want(404)
	if !slices.Contains(f.e.auditActions(), "secrets.imported") || !slices.Contains(f.e.auditActions(), "secrets.export") {
		t.Fatal("import/export not audited")
	}
}

func TestCompare(t *testing.T) {
	f := newProjectFixture(t)
	f.owner.do("PUT", secBase+"/SHARED", map[string]string{"value": "d"}).want(201)
	f.owner.do("PUT", secBase+"/DEV_ONLY", map[string]string{"value": "d"}).want(201)
	f.owner.do("PUT", "/api/v1/projects/web-app/environments/production/secrets/SHARED", map[string]string{"value": "p"}).want(201)

	r := f.viewer.do("GET", "/api/v1/projects/web-app/compare?envs=development,production", nil).want(200)
	if strings.Contains(string(r.body), `"d"`) || strings.Contains(string(r.body), `"p"`) {
		t.Fatal("compare leaks values")
	}
	keys := r.json()["keys"].(map[string]any)
	dev := keys["DEV_ONLY"].(map[string]any)
	if dev["development"].(map[string]any)["exists"] != true || dev["production"].(map[string]any)["exists"] != false {
		t.Fatalf("compare %v", keys)
	}
	if _, ok := keys["SHARED"].(map[string]any)["staging"]; ok {
		t.Fatal("unrequested environment included")
	}
	f.viewer.do("GET", "/api/v1/projects/web-app/compare?envs=nope", nil).want(400)
	f.outsider.do("GET", "/api/v1/projects/web-app/compare", nil).want(404)
}

func TestSecretPermissions(t *testing.T) {
	f := newProjectFixture(t)
	f.owner.do("PUT", secBase+"/K", map[string]string{"value": "v"}).want(201)
	for _, c := range []struct {
		name string
		cl   *client
		read int
		put  int
	}{
		{"viewer", f.viewer, 200, 403},
		{"editor", f.editor, 200, 200},
		{"admin non-member", f.admin, 403, 403},
		{"outsider", f.outsider, 404, 404},
		{"anonymous", f.e.browser(), 401, 401},
	} {
		if r := c.cl.do("GET", secBase+"/K", nil); r.status != c.read {
			t.Errorf("%s read: %d", c.name, r.status)
		}
		if r := c.cl.do("PUT", secBase+"/K", map[string]string{"value": "w"}); r.status != c.put {
			t.Errorf("%s write: %d", c.name, r.status)
		}
		if r := c.cl.do("DELETE", secBase+"/K", nil); c.put != 200 && r.status != c.put {
			t.Errorf("%s delete: %d", c.name, r.status)
		}
	}
	f.owner.do("GET", "/api/v1/projects/web-app/environments/nope/secrets", nil).want(404)
}

// TestNoSecretsInLogs runs a full flow with unique marker values and checks
// that none of them reaches the log output or the audit metadata.
func TestNoSecretsInLogs(t *testing.T) {
	f := newProjectFixture(t)
	markers := []string{"mk-value-91c2", "mk-comment-77ad", "mk-import-5b1e", "mk-batch-c0de", "mk-json-e9f0"}
	f.owner.do("PUT", secBase+"/A?x="+markers[0], map[string]string{"value": markers[0], "comment": markers[1]}).want(201)
	f.owner.do("POST", secBase+":import", "B="+markers[2]+"\n").want(200)
	f.owner.do("POST", secBase+":batch", map[string]any{"upserts": []map[string]string{{"key": "C", "value": markers[3]}}}).want(200)
	f.owner.do("POST", secBase+":import", `{"D":"`+markers[4]+`"}`).want(200)
	f.owner.do("GET", secBase+"?values=true", nil).want(200)
	f.owner.do("GET", secBase+":export?format=yaml", nil).want(200)
	f.owner.do("GET", secBase+"/A/versions?values=true", nil).want(200)
	f.owner.do("POST", secBase+"/A/rollback", map[string]int{"version": 1}).want(200)
	// Failing requests must not echo values either.
	r := f.owner.do("POST", secBase+":import", "bad-key="+markers[2])
	r2 := f.owner.do("PUT", secBase+"/bad-key", map[string]string{"value": markers[0]})
	r3 := f.owner.do("PUT", secBase+"/A", `{"value": "`+markers[0]+`", "bogus": 1}`)
	// Passwords and tokens.
	tok := f.owner.do("POST", "/api/v1/me/tokens", map[string]string{"name": "t"}).want(201).json()["token"].(string)

	rows, _, _ := f.e.db.Audit().List(t.Context(), auditFilterAll())
	var audit strings.Builder
	for _, row := range rows {
		audit.WriteString(row.Metadata)
	}
	logs := f.e.logOutput()
	for _, m := range append(markers, adminPassword, tok) {
		if strings.Contains(logs, m) {
			t.Errorf("log contains %q", m)
		}
		if strings.Contains(audit.String(), m) {
			t.Errorf("audit metadata contains %q", m)
		}
	}
	for _, x := range []resp{r, r2, r3} {
		if x.status != 400 || bytes.Contains(x.body, []byte("mk-")) {
			t.Errorf("error response %d echoes input: %s", x.status, x.body)
		}
	}
	if !strings.Contains(logs, `"path":"`+secBase+`/A"`) {
		t.Fatal("access log missing; the leak check would be vacuous")
	}
}
