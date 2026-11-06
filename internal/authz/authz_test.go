package authz

import (
	"testing"

	"github.com/thomasbudiarjo/envily/internal/store"
)

func TestMatrix(t *testing.T) {
	user := &Principal{Type: ActorUser, ID: "u", UserID: "u"}
	pat := &Principal{Type: ActorPersonalToken, ID: "t", UserID: "u"}
	admin := &Principal{Type: ActorUser, ID: "a", UserID: "a", IsAdmin: true}
	mustChange := &Principal{Type: ActorUser, ID: "m", UserID: "m", MustChangePassword: true}
	read := &Principal{Type: ActorServiceToken, ID: "s1", ProjectID: "p", EnvironmentIDs: []string{"dev"}, Permission: store.PermRead}
	rw := &Principal{Type: ActorServiceToken, ID: "s2", ProjectID: "p", EnvironmentIDs: []string{"dev"}, Permission: store.PermReadWrite}

	all := []Permission{ViewProject, ReadSecrets, WriteSecrets, ViewHistory, ReadHistoryValues, Rollback,
		ManageEnvironments, ManageMembers, ManageTokens, ViewAudit, ManageProject}
	names := []string{"ViewProject", "ReadSecrets", "WriteSecrets", "ViewHistory", "ReadHistoryValues", "Rollback",
		"ManageEnvironments", "ManageMembers", "ManageTokens", "ViewAudit", "ManageProject"}

	// Expected answers, one letter per permission in the order of all: y = allowed.
	cases := []struct {
		name string
		p    *Principal
		role store.Role
		env  string
		want string
	}{
		{"viewer", user, store.RoleViewer, "dev", "yynynnnnnnn"},
		{"editor", user, store.RoleEditor, "dev", "yyyyyynnnyn"},
		{"owner", user, store.RoleOwner, "dev", "yyyyyyyyyyy"},
		{"personal token inherits owner", pat, store.RoleOwner, "dev", "yyyyyyyyyyy"},
		{"non-member", user, "", "dev", "nnnnnnnnnnn"},
		{"admin non-member", admin, "", "dev", "nnnnnnnnnnn"},
		{"must change password", mustChange, store.RoleOwner, "dev", "nnnnnnnnnnn"},
		{"read token in scope", read, "", "dev", "yynnnnnnnnn"},
		{"read token out of scope", read, "", "prod", "nnnnnnnnnnn"},
		{"read token project level", read, "", "", "ynnnnnnnnnn"},
		{"rw token in scope", rw, "", "dev", "yyynnnnnnnn"},
		{"rw token out of scope", rw, "", "prod", "nnnnnnnnnnn"},
		{"nil principal", nil, store.RoleOwner, "dev", "nnnnnnnnnnn"},
	}
	for _, tc := range cases {
		for i, perm := range all {
			want := tc.want[i] == 'y'
			if got := Can(tc.p, tc.role, perm, "p", tc.env); got != want {
				t.Errorf("%s / %s: got %v, want %v", tc.name, names[i], got, want)
			}
		}
	}
	if Can(rw, "", ReadSecrets, "other", "dev") {
		t.Error("service token allowed in another project")
	}
}
