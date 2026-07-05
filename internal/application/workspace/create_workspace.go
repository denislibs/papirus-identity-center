package workspace

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	domain "github.com/denislibs/papirus-identity-center/internal/domain/workspace"
)

type CreateWorkspace struct {
	workspaces domain.WorkspaceRepository
	members    domain.MemberRepository
}

func NewCreateWorkspace(w domain.WorkspaceRepository, m domain.MemberRepository) *CreateWorkspace {
	return &CreateWorkspace{workspaces: w, members: m}
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = nonSlug.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "workspace"
	}
	return s
}

func (uc *CreateWorkspace) Execute(ctx context.Context, userID, name string) (*domain.Workspace, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, domain.ErrInvalidName
	}
	base := slugify(name)
	slug := base + "-" + uuid.NewString()[:8] // short suffix to avoid collisions
	if exists, err := uc.workspaces.SlugExists(ctx, slug); err != nil {
		return nil, err
	} else if exists {
		slug = base + "-" + uuid.NewString()[:8]
	}
	w := &domain.Workspace{ID: uuid.NewString(), Name: name, Slug: slug, CreatedBy: userID, CreatedAt: time.Now().UTC()}
	if err := uc.workspaces.Create(ctx, w); err != nil {
		return nil, err
	}
	owner := &domain.Member{ID: uuid.NewString(), WorkspaceID: w.ID, UserID: userID, Role: domain.RoleOwner, Status: domain.StatusActive, CreatedAt: time.Now().UTC()}
	if err := uc.members.Create(ctx, owner); err != nil {
		return nil, err
	}
	return w, nil
}
