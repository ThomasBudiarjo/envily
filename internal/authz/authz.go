// Package authz decides what a principal may do. It is pure: the service
// layer looks up project membership and passes the role in.
package authz

import (
	"slices"

	"github.com/thomasbudiarjo/envily/internal/store"
)

type ActorType string

const (
	ActorUser          ActorType = "user"
	ActorServiceToken  ActorType = "service_token"
	ActorPersonalToken ActorType = "personal_token"
	ActorSystem        ActorType = "system"
)

// Principal is the authenticated caller of a request.
type Principal struct {
	Type  ActorType
	ID    string // user ID, or token ID for tokens
	Label string // email, or token name

	// Users and personal tokens.
	UserID             string
	IsAdmin            bool
	MustChangePassword bool
	SessionID          string // set for cookie sessions

	// Service tokens.
	ProjectID      string
	EnvironmentIDs []string
	Permission     store.Permission
}

// IsUser reports whether p acts as a user (session or personal token).
func (p *Principal) IsUser() bool {
	return p != nil && (p.Type == ActorUser || p.Type == ActorPersonalToken)
}

// IsServiceToken reports whether p is a service token.
func (p *Principal) IsServiceToken() bool { return p != nil && p.Type == ActorServiceToken }

// IsSystem reports whether p is a local admin command.
func (p *Principal) IsSystem() bool { return p != nil && p.Type == ActorSystem }

// System is the principal of local admin commands (envily admin ...).
var System = &Principal{Type: ActorSystem, ID: "system", Label: "system", IsAdmin: true}

// Permission is something a principal can do within a project.
type Permission int

const (
	ViewProject        Permission = iota // see project, environments, secret keys
	ReadSecrets                          // read secret values
	WriteSecrets                         // create, update, rename, delete, import
	ViewHistory                          // list versions without values
	ReadHistoryValues                    // see values of old versions
	Rollback                             // restore an old version
	ManageEnvironments                   // create, rename, reorder, delete environments
	ManageMembers                        // add, change, remove members
	ManageTokens                         // create, list, revoke service tokens
	ViewAudit                            // read the project audit log
	ManageProject                        // rename or delete the project
)

var rolePermissions = map[store.Role][]Permission{
	store.RoleViewer: {ViewProject, ReadSecrets, ViewHistory},
	store.RoleEditor: {ViewProject, ReadSecrets, WriteSecrets, ViewHistory, ReadHistoryValues, Rollback, ViewAudit},
	store.RoleOwner: {ViewProject, ReadSecrets, WriteSecrets, ViewHistory, ReadHistoryValues, Rollback,
		ManageEnvironments, ManageMembers, ManageTokens, ViewAudit, ManageProject},
}

// Can reports whether p may perform perm in project projectID. role is p's
// membership role in that project ("" when p is not a member). envID is
// the environment the action targets, or "" for project-level actions.
func Can(p *Principal, role store.Role, perm Permission, projectID, envID string) bool {
	switch {
	case p == nil:
		return false
	case p.IsServiceToken():
		if p.ProjectID != projectID {
			return false
		}
		if envID != "" && !slices.Contains(p.EnvironmentIDs, envID) {
			return false
		}
		switch perm {
		case ViewProject:
			return true
		case ReadSecrets:
			return envID != ""
		case WriteSecrets:
			return envID != "" && p.Permission == store.PermReadWrite
		}
		return false
	case p.IsUser():
		if p.MustChangePassword {
			return false
		}
		return slices.Contains(rolePermissions[role], perm)
	}
	return false
}

// CanSeeEnvironment reports whether p may see environment envID at all.
// Users see every environment of a project they belong to; service tokens
// only those in scope.
func CanSeeEnvironment(p *Principal, role store.Role, projectID, envID string) bool {
	return Can(p, role, ViewProject, projectID, envID)
}
