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
