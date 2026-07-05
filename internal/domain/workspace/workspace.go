package workspace

import (
	"errors"
	"time"
)

// Roles within a workspace (generic, platform-level).
const (
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleMember = "member"
)

// Member statuses.
const (
	StatusActive  = "active"
	StatusInvited = "invited"
)

type Workspace struct {
	ID        string
	Name      string
	Slug      string
	CreatedBy string
	CreatedAt time.Time
}

type Member struct {
	ID          string
	WorkspaceID string
	UserID      string
	Role        string
	Status      string
	CreatedAt   time.Time
	OrgUnitID  *string
	PositionID *string
}

type OrgUnit struct {
	ID          string
	WorkspaceID string
	ParentID    *string
	Name        string
	SortOrder   int
	CreatedAt   time.Time
}

type Position struct {
	ID          string
	WorkspaceID string
	Title       string
	CreatedAt   time.Time
}

type Invite struct {
	ID          string
	WorkspaceID string
	Email       string
	Role        string
	Token       string
	ExpiresAt   time.Time
	AcceptedAt  *time.Time
}

var (
	ErrWorkspaceNotFound = errors.New("workspace: not found")
	ErrInviteNotFound    = errors.New("workspace: invite not found or expired")
	ErrNotMember         = errors.New("workspace: user is not a member")
	ErrAlreadyMember     = errors.New("workspace: user is already a member")
	ErrForbidden         = errors.New("workspace: insufficient role")
	ErrInvalidName       = errors.New("workspace: invalid name")
	ErrInvalidRole       = errors.New("workspace: invalid role")
	ErrOrgUnitNotFound  = errors.New("workspace: org unit not found")
	ErrPositionNotFound = errors.New("workspace: position not found")
	ErrInvalidTitle     = errors.New("workspace: invalid title")
)

// CanManageMembers reports whether a role may invite/manage members.
func CanManageMembers(role string) bool { return role == RoleOwner || role == RoleAdmin }

// ValidRole reports whether role is one of the known roles.
func ValidRole(role string) bool {
	return role == RoleOwner || role == RoleAdmin || role == RoleMember
}
