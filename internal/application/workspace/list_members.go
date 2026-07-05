package workspace

import (
	"context"

	domain "github.com/denislibs/papirus-identity-center/internal/domain/workspace"
)

type ListMembers struct{ members domain.MemberRepository }

func NewListMembers(m domain.MemberRepository) *ListMembers { return &ListMembers{m} }

func (uc *ListMembers) Execute(ctx context.Context, workspaceID, requesterID string) ([]*domain.Member, error) {
	if _, err := uc.members.Find(ctx, workspaceID, requesterID); err != nil {
		return nil, err // ErrNotMember → caller maps to 403/404
	}
	return uc.members.ListByWorkspace(ctx, workspaceID)
}
