//go:build enterprise

package enterprise

// Package enterprise (rbac.go) implements Role-Based Access Control (RBAC)
// for Grimlocker Enterprise.
//
// Roles:
//   - "admin"  — full access: read, write, delete, manage users, approve entries, panic button
//   - "user"   — restricted: read/write/delete own entries; cannot approve, cannot panic
//
// Row-Level Security:
//   Entries have a VisibleTo []string field. When non-empty, only the listed
//   subject IDs can access the entry. Admins always have access regardless.
//
//   New entries created by users start with PendingApproval=true and are
//   only visible on the local client. An admin must approve the entry before
//   it is synchronized to the shared vault.
package enterprise

// Permission is a fine-grained access control token.
type Permission string

const (
	// Entry-level permissions
	PermEntryRead   Permission = "entry:read"
	PermEntryWrite  Permission = "entry:write"
	PermEntryDelete Permission = "entry:delete"

	// Admin-level permissions
	PermAdminUsers   Permission = "admin:users"   // create, list, revoke users
	PermAdminApprove Permission = "admin:approve" // approve pending entries
	PermPanic        Permission = "admin:panic"   // PANIC BUTTON (DB overwrite)
)

// RoleDefinition maps a role name to its set of permissions.
type RoleDefinition struct {
	Name        string
	Permissions []Permission
}

// DefaultRoles defines the built-in role hierarchy.
var DefaultRoles = map[string]RoleDefinition{
	"admin": {
		Name: "admin",
		Permissions: []Permission{
			PermEntryRead, PermEntryWrite, PermEntryDelete,
			PermAdminUsers, PermAdminApprove, PermPanic,
		},
	},
	"user": {
		Name:        "user",
		Permissions: []Permission{PermEntryRead, PermEntryWrite, PermEntryDelete},
	},
}

// RBAC is the enterprise access control engine.
type RBAC struct{}

// NewRBAC creates an RBAC instance.
func NewRBAC() *RBAC { return &RBAC{} }

// HasPermission returns true if any of the given roles grants the requested permission.
func (r *RBAC) HasPermission(roles []string, perm Permission) bool {
	for _, role := range roles {
		def, ok := DefaultRoles[role]
		if !ok {
			continue
		}
		for _, p := range def.Permissions {
			if p == perm {
				return true
			}
		}
	}
	return false
}

// IsAdmin returns true if the subject holds the "admin" role.
func (r *RBAC) IsAdmin(roles []string) bool {
	for _, role := range roles {
		if role == "admin" {
			return true
		}
	}
	return false
}

// CanReadEntry checks both role permission and row-level security.
// If entry.VisibleTo is non-empty, the subjectID must be in that list
// (admins always pass regardless).
func (r *RBAC) CanReadEntry(roles []string, subjectID string, visibleTo []string) bool {
	if !r.HasPermission(roles, PermEntryRead) {
		return false
	}
	if r.IsAdmin(roles) {
		return true
	}
	if len(visibleTo) == 0 {
		return true
	}
	for _, id := range visibleTo {
		if id == subjectID {
			return true
		}
	}
	return false
}
