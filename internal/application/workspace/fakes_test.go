package workspace_test

import (
	"context"
	"strings"
	"time"

	domain "github.com/denislibs/papirus-identity-center/internal/domain/workspace"
)

type fakeWS struct{ byID map[string]*domain.Workspace; slugs map[string]bool; members *fakeMembers }

func newFakeWS(m *fakeMembers) *fakeWS {
	return &fakeWS{byID: map[string]*domain.Workspace{}, slugs: map[string]bool{}, members: m}
}
func (f *fakeWS) Create(_ context.Context, w *domain.Workspace) error {
	cp := *w; f.byID[w.ID] = &cp; f.slugs[w.Slug] = true; return nil
}
func (f *fakeWS) FindByID(_ context.Context, id string) (*domain.Workspace, error) {
	if w, ok := f.byID[id]; ok { cp := *w; return &cp, nil }
	return nil, domain.ErrWorkspaceNotFound
}
func (f *fakeWS) ListByMember(_ context.Context, userID string) ([]*domain.Workspace, error) {
	var out []*domain.Workspace
	for _, m := range f.members.list {
		if m.UserID == userID && m.Status == domain.StatusActive {
			if w, ok := f.byID[m.WorkspaceID]; ok { cp := *w; out = append(out, &cp) }
		}
	}
	return out, nil
}
func (f *fakeWS) SlugExists(_ context.Context, slug string) (bool, error) { return f.slugs[slug], nil }

type fakeMembers struct{ list []*domain.Member }

func newFakeMembers() *fakeMembers { return &fakeMembers{} }
func (f *fakeMembers) Create(_ context.Context, m *domain.Member) error { cp := *m; f.list = append(f.list, &cp); return nil }
func (f *fakeMembers) Find(_ context.Context, wsID, userID string) (*domain.Member, error) {
	for _, m := range f.list { if m.WorkspaceID == wsID && m.UserID == userID { cp := *m; return &cp, nil } }
	return nil, domain.ErrNotMember
}
func (f *fakeMembers) ListByWorkspace(_ context.Context, wsID string) ([]*domain.Member, error) {
	var out []*domain.Member
	for _, m := range f.list { if m.WorkspaceID == wsID { cp := *m; out = append(out, &cp) } }
	return out, nil
}
func (f *fakeMembers) Assign(_ context.Context, wsID, userID string, orgUnitID, positionID *string) error {
	for _, m := range f.list {
		if m.WorkspaceID == wsID && m.UserID == userID {
			m.OrgUnitID = orgUnitID
			m.PositionID = positionID
			return nil
		}
	}
	return domain.ErrNotMember
}

type fakeOrgUnits struct{ list []*domain.OrgUnit }

func (f *fakeOrgUnits) Create(_ context.Context, u *domain.OrgUnit) error { cp := *u; f.list = append(f.list, &cp); return nil }
func (f *fakeOrgUnits) ListByWorkspace(_ context.Context, wsID string) ([]*domain.OrgUnit, error) {
	var out []*domain.OrgUnit
	for _, u := range f.list { if u.WorkspaceID == wsID { cp := *u; out = append(out, &cp) } }
	return out, nil
}
func (f *fakeOrgUnits) Exists(_ context.Context, wsID, id string) (bool, error) {
	for _, u := range f.list { if u.WorkspaceID == wsID && u.ID == id { return true, nil } }
	return false, nil
}

type fakePositions struct{ list []*domain.Position }

func (f *fakePositions) Create(_ context.Context, p *domain.Position) error { cp := *p; f.list = append(f.list, &cp); return nil }
func (f *fakePositions) ListByWorkspace(_ context.Context, wsID string) ([]*domain.Position, error) {
	var out []*domain.Position
	for _, p := range f.list { if p.WorkspaceID == wsID { cp := *p; out = append(out, &cp) } }
	return out, nil
}
func (f *fakePositions) Exists(_ context.Context, wsID, id string) (bool, error) {
	for _, p := range f.list { if p.WorkspaceID == wsID && p.ID == id { return true, nil } }
	return false, nil
}

type fakeInvites struct{ byToken map[string]*domain.Invite; accepted map[string]bool }

func newFakeInvites() *fakeInvites { return &fakeInvites{byToken: map[string]*domain.Invite{}, accepted: map[string]bool{}} }
func (f *fakeInvites) Create(_ context.Context, inv *domain.Invite) error { cp := *inv; f.byToken[inv.Token] = &cp; return nil }
func (f *fakeInvites) FindByToken(_ context.Context, token string) (*domain.Invite, error) {
	if inv, ok := f.byToken[token]; ok && !f.accepted[inv.ID] { cp := *inv; return &cp, nil }
	return nil, domain.ErrInviteNotFound
}
func (f *fakeInvites) MarkAccepted(_ context.Context, id string) error { f.accepted[id] = true; return nil }

type sentInvite struct{ to, ws, link string }
type fakeMailer struct{ invites []sentInvite }

func (f *fakeMailer) SendWorkspaceInvite(_ context.Context, to, ws, link string) error {
	f.invites = append(f.invites, sentInvite{to, ws, link}); return nil
}

// slug helper mirror for assertions
func slugContains(s, sub string) bool { return strings.Contains(s, sub) }
var _ = time.Now

type fakeProducts struct{ list []*domain.Product }

func (f *fakeProducts) ListAll(_ context.Context) ([]*domain.Product, error) { return f.list, nil }
func (f *fakeProducts) Exists(_ context.Context, key string) (bool, error) {
	for _, p := range f.list {
		if p.Key == key {
			return true, nil
		}
	}
	return false, nil
}

type fakeWorkspaceProducts struct{ enabled map[string]map[string]bool } // wsID -> key -> true

func newFakeWorkspaceProducts() *fakeWorkspaceProducts {
	return &fakeWorkspaceProducts{enabled: map[string]map[string]bool{}}
}
func (f *fakeWorkspaceProducts) Enable(_ context.Context, wsID, key string) error {
	if f.enabled[wsID] == nil {
		f.enabled[wsID] = map[string]bool{}
	}
	f.enabled[wsID][key] = true
	return nil
}
func (f *fakeWorkspaceProducts) Disable(_ context.Context, wsID, key string) error {
	if f.enabled[wsID] != nil {
		delete(f.enabled[wsID], key)
	}
	return nil
}
func (f *fakeWorkspaceProducts) ListEnabled(_ context.Context, wsID string) ([]*domain.Product, error) {
	var out []*domain.Product
	for key := range f.enabled[wsID] {
		out = append(out, &domain.Product{Key: key, Name: key})
	}
	return out, nil
}
