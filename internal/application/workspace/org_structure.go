package workspace

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	domain "github.com/denislibs/papirus-identity-center/internal/domain/workspace"
)

// requireManager checks that the requester is an active owner/admin of the workspace.
func requireManager(ctx context.Context, members domain.MemberRepository, wsID, userID string) error {
	m, err := members.Find(ctx, wsID, userID)
	if err != nil {
		return err // ErrNotMember
	}
	if !domain.CanManageMembers(m.Role) {
		return domain.ErrForbidden
	}
	return nil
}

// requireMember checks that the requester is a member of the workspace.
func requireMember(ctx context.Context, members domain.MemberRepository, wsID, userID string) error {
	if _, err := members.Find(ctx, wsID, userID); err != nil {
		return err
	}
	return nil
}

// CreateOrgUnit creates a new organisational unit in a workspace.
type CreateOrgUnit struct {
	members domain.MemberRepository
	units   domain.OrgUnitRepository
}

func NewCreateOrgUnit(m domain.MemberRepository, u domain.OrgUnitRepository) *CreateOrgUnit {
	return &CreateOrgUnit{members: m, units: u}
}

func (uc *CreateOrgUnit) Execute(ctx context.Context, wsID, requesterID, name string, parentID *string) (*domain.OrgUnit, error) {
	if err := requireManager(ctx, uc.members, wsID, requesterID); err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, domain.ErrInvalidName
	}
	if parentID != nil {
		ok, err := uc.units.Exists(ctx, wsID, *parentID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, domain.ErrOrgUnitNotFound
		}
	}
	u := &domain.OrgUnit{
		ID:          uuid.NewString(),
		WorkspaceID: wsID,
		ParentID:    parentID,
		Name:        name,
		CreatedAt:   time.Now().UTC(),
	}
	if err := uc.units.Create(ctx, u); err != nil {
		return nil, err
	}
	return u, nil
}

// ListOrgUnits lists all organisational units for a workspace (any member may read).
type ListOrgUnits struct {
	members domain.MemberRepository
	units   domain.OrgUnitRepository
}

func NewListOrgUnits(m domain.MemberRepository, u domain.OrgUnitRepository) *ListOrgUnits {
	return &ListOrgUnits{members: m, units: u}
}

func (uc *ListOrgUnits) Execute(ctx context.Context, wsID, requesterID string) ([]*domain.OrgUnit, error) {
	if err := requireMember(ctx, uc.members, wsID, requesterID); err != nil {
		return nil, err
	}
	return uc.units.ListByWorkspace(ctx, wsID)
}

// CreatePosition creates a new position in a workspace.
type CreatePosition struct {
	members   domain.MemberRepository
	positions domain.PositionRepository
}

func NewCreatePosition(m domain.MemberRepository, p domain.PositionRepository) *CreatePosition {
	return &CreatePosition{members: m, positions: p}
}

func (uc *CreatePosition) Execute(ctx context.Context, wsID, requesterID, title string) (*domain.Position, error) {
	if err := requireManager(ctx, uc.members, wsID, requesterID); err != nil {
		return nil, err
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, domain.ErrInvalidTitle
	}
	p := &domain.Position{
		ID:          uuid.NewString(),
		WorkspaceID: wsID,
		Title:       title,
		CreatedAt:   time.Now().UTC(),
	}
	if err := uc.positions.Create(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

// ListPositions lists all positions for a workspace (any member may read).
type ListPositions struct {
	members   domain.MemberRepository
	positions domain.PositionRepository
}

func NewListPositions(m domain.MemberRepository, p domain.PositionRepository) *ListPositions {
	return &ListPositions{members: m, positions: p}
}

func (uc *ListPositions) Execute(ctx context.Context, wsID, requesterID string) ([]*domain.Position, error) {
	if err := requireMember(ctx, uc.members, wsID, requesterID); err != nil {
		return nil, err
	}
	return uc.positions.ListByWorkspace(ctx, wsID)
}

// AssignMember assigns a workspace member to an org unit and/or position.
// Both orgUnitID and positionID are optional (nil clears the field).
// The requester must be an owner/admin; both IDs (if provided) must belong to the workspace.
type AssignMember struct {
	members   domain.MemberRepository
	units     domain.OrgUnitRepository
	positions domain.PositionRepository
}

func NewAssignMember(m domain.MemberRepository, u domain.OrgUnitRepository, p domain.PositionRepository) *AssignMember {
	return &AssignMember{members: m, units: u, positions: p}
}

func (uc *AssignMember) Execute(ctx context.Context, wsID, requesterID, targetUserID string, orgUnitID, positionID *string) error {
	if err := requireManager(ctx, uc.members, wsID, requesterID); err != nil {
		return err
	}
	// Target user must already be a member.
	if _, err := uc.members.Find(ctx, wsID, targetUserID); err != nil {
		return err
	}
	if orgUnitID != nil {
		ok, err := uc.units.Exists(ctx, wsID, *orgUnitID)
		if err != nil {
			return err
		}
		if !ok {
			return domain.ErrOrgUnitNotFound
		}
	}
	if positionID != nil {
		ok, err := uc.positions.Exists(ctx, wsID, *positionID)
		if err != nil {
			return err
		}
		if !ok {
			return domain.ErrPositionNotFound
		}
	}
	return uc.members.Assign(ctx, wsID, targetUserID, orgUnitID, positionID)
}
