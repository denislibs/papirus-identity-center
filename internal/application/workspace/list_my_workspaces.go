package workspace

import (
	"context"

	domain "github.com/denislibs/papirus-identity-center/internal/domain/workspace"
)

type ListMyWorkspaces struct{ workspaces domain.WorkspaceRepository }

func NewListMyWorkspaces(w domain.WorkspaceRepository) *ListMyWorkspaces { return &ListMyWorkspaces{w} }

func (uc *ListMyWorkspaces) Execute(ctx context.Context, userID string) ([]*domain.Workspace, error) {
	return uc.workspaces.ListByMember(ctx, userID)
}
