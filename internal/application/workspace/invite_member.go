package workspace

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	domain "github.com/denislibs/papirus-identity-center/internal/domain/workspace"
)

const inviteTTL = 7 * 24 * time.Hour

type InviteMember struct {
	workspaces domain.WorkspaceRepository
	members    domain.MemberRepository
	invites    domain.InviteRepository
	mailer     domain.WorkspaceMailer
	baseURL    string
}

func NewInviteMember(w domain.WorkspaceRepository, m domain.MemberRepository, i domain.InviteRepository,
	mailer domain.WorkspaceMailer, baseURL string) *InviteMember {
	return &InviteMember{workspaces: w, members: m, invites: i, mailer: mailer, baseURL: baseURL}
}

func (uc *InviteMember) Execute(ctx context.Context, workspaceID, inviterID, email, role string) error {
	if !domain.ValidRole(role) {
		return domain.ErrInvalidRole
	}
	inviter, err := uc.members.Find(ctx, workspaceID, inviterID)
	if err != nil {
		return err // ErrNotMember
	}
	if !domain.CanManageMembers(inviter.Role) {
		return domain.ErrForbidden
	}
	ws, err := uc.workspaces.FindByID(ctx, workspaceID)
	if err != nil {
		return err
	}
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Errorf("workspace: gen invite token: %w", err)
	}
	token := hex.EncodeToString(buf)
	inv := &domain.Invite{
		ID: uuid.NewString(), WorkspaceID: workspaceID, Email: strings.ToLower(strings.TrimSpace(email)),
		Role: role, Token: token, ExpiresAt: time.Now().Add(inviteTTL).UTC(),
	}
	if err := uc.invites.Create(ctx, inv); err != nil {
		return err
	}
	link := uc.baseURL + "/invites/" + token
	return uc.mailer.SendWorkspaceInvite(ctx, inv.Email, ws.Name, link)
}
