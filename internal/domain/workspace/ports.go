package workspace

import "context"

type WorkspaceRepository interface {
	Create(ctx context.Context, w *Workspace) error
	FindByID(ctx context.Context, id string) (*Workspace, error) // ErrWorkspaceNotFound
	ListByMember(ctx context.Context, userID string) ([]*Workspace, error)
	SlugExists(ctx context.Context, slug string) (bool, error)
}

type MemberRepository interface {
	Create(ctx context.Context, m *Member) error
	Find(ctx context.Context, workspaceID, userID string) (*Member, error) // ErrNotMember
	ListByWorkspace(ctx context.Context, workspaceID string) ([]*Member, error)
	// Assign sets a member's org unit and position (either may be nil to clear).
	Assign(ctx context.Context, workspaceID, userID string, orgUnitID, positionID *string) error
}

type OrgUnitRepository interface {
	Create(ctx context.Context, u *OrgUnit) error
	ListByWorkspace(ctx context.Context, workspaceID string) ([]*OrgUnit, error)
	Exists(ctx context.Context, workspaceID, id string) (bool, error)
}

type PositionRepository interface {
	Create(ctx context.Context, p *Position) error
	ListByWorkspace(ctx context.Context, workspaceID string) ([]*Position, error)
	Exists(ctx context.Context, workspaceID, id string) (bool, error)
}

type InviteRepository interface {
	Create(ctx context.Context, inv *Invite) error
	FindByToken(ctx context.Context, token string) (*Invite, error) // ErrInviteNotFound
	MarkAccepted(ctx context.Context, id string) error
}

// WorkspaceMailer sends workspace invitation emails.
type WorkspaceMailer interface {
	SendWorkspaceInvite(ctx context.Context, to, workspaceName, link string) error
}

type ProductRepository interface {
	ListAll(ctx context.Context) ([]*Product, error)
	Exists(ctx context.Context, key string) (bool, error)
}

type WorkspaceProductRepository interface {
	Enable(ctx context.Context, workspaceID, productKey string) error
	Disable(ctx context.Context, workspaceID, productKey string) error
	ListEnabled(ctx context.Context, workspaceID string) ([]*Product, error)
}
