package workspace

import (
	"context"
	"time"

	"github.com/google/uuid"

	domain "github.com/denislibs/papirus-identity-center/internal/domain/workspace"
)

type AcceptInvite struct {
	invites domain.InviteRepository
	members domain.MemberRepository
}

func NewAcceptInvite(i domain.InviteRepository, m domain.MemberRepository) *AcceptInvite {
	return &AcceptInvite{invites: i, members: m}
}

func (uc *AcceptInvite) Execute(ctx context.Context, token, userID string) error {
	inv, err := uc.invites.FindByToken(ctx, token)
	if err != nil {
		return err // ErrInviteNotFound
	}
	m := &domain.Member{ID: uuid.NewString(), WorkspaceID: inv.WorkspaceID, UserID: userID, Role: inv.Role, Status: domain.StatusActive, CreatedAt: time.Now().UTC()}
	if err := uc.members.Create(ctx, m); err != nil {
		return err
	}
	return uc.invites.MarkAccepted(ctx, inv.ID)
}
