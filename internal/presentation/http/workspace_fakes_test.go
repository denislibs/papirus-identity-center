package http_test

import (
	"context"

	domainws "github.com/denislibs/papirus-identity-center/internal/domain/workspace"
)

// fakeWSHTTP is an in-memory WorkspaceRepository for http package tests.
type fakeWSHTTP struct {
	byID    map[string]*domainws.Workspace
	slugs   map[string]bool
	members *fakeMembersHTTP
}

func newFakeWSHTTP(m *fakeMembersHTTP) *fakeWSHTTP {
	return &fakeWSHTTP{byID: map[string]*domainws.Workspace{}, slugs: map[string]bool{}, members: m}
}

func (f *fakeWSHTTP) Create(_ context.Context, w *domainws.Workspace) error {
	cp := *w
	f.byID[w.ID] = &cp
	f.slugs[w.Slug] = true
	return nil
}

func (f *fakeWSHTTP) FindByID(_ context.Context, id string) (*domainws.Workspace, error) {
	if w, ok := f.byID[id]; ok {
		cp := *w
		return &cp, nil
	}
	return nil, domainws.ErrWorkspaceNotFound
}

func (f *fakeWSHTTP) ListByMember(_ context.Context, userID string) ([]*domainws.Workspace, error) {
	var out []*domainws.Workspace
	for _, m := range f.members.list {
		if m.UserID == userID && m.Status == domainws.StatusActive {
			if w, ok := f.byID[m.WorkspaceID]; ok {
				cp := *w
				out = append(out, &cp)
			}
		}
	}
	return out, nil
}

func (f *fakeWSHTTP) SlugExists(_ context.Context, slug string) (bool, error) {
	return f.slugs[slug], nil
}

// fakeMembersHTTP is an in-memory MemberRepository for http package tests.
type fakeMembersHTTP struct{ list []*domainws.Member }

func newFakeMembersHTTP() *fakeMembersHTTP { return &fakeMembersHTTP{} }

func (f *fakeMembersHTTP) Create(_ context.Context, m *domainws.Member) error {
	cp := *m
	f.list = append(f.list, &cp)
	return nil
}

func (f *fakeMembersHTTP) Find(_ context.Context, wsID, userID string) (*domainws.Member, error) {
	for _, m := range f.list {
		if m.WorkspaceID == wsID && m.UserID == userID {
			cp := *m
			return &cp, nil
		}
	}
	return nil, domainws.ErrNotMember
}

func (f *fakeMembersHTTP) ListByWorkspace(_ context.Context, wsID string) ([]*domainws.Member, error) {
	var out []*domainws.Member
	for _, m := range f.list {
		if m.WorkspaceID == wsID {
			cp := *m
			out = append(out, &cp)
		}
	}
	return out, nil
}

// fakeInvitesHTTP is an in-memory InviteRepository for http package tests.
type fakeInvitesHTTP struct {
	byToken  map[string]*domainws.Invite
	accepted map[string]bool
}

func newFakeInvitesHTTP() *fakeInvitesHTTP {
	return &fakeInvitesHTTP{byToken: map[string]*domainws.Invite{}, accepted: map[string]bool{}}
}

func (f *fakeInvitesHTTP) Create(_ context.Context, inv *domainws.Invite) error {
	cp := *inv
	f.byToken[inv.Token] = &cp
	return nil
}

func (f *fakeInvitesHTTP) FindByToken(_ context.Context, token string) (*domainws.Invite, error) {
	if inv, ok := f.byToken[token]; ok && !f.accepted[inv.ID] {
		cp := *inv
		return &cp, nil
	}
	return nil, domainws.ErrInviteNotFound
}

func (f *fakeInvitesHTTP) MarkAccepted(_ context.Context, id string) error {
	f.accepted[id] = true
	return nil
}

// fakeWSMailer is an in-memory WorkspaceMailer for http package tests.
type fakeWSMailer struct {
	sent []struct{ to, ws, link string }
}

func (f *fakeWSMailer) SendWorkspaceInvite(_ context.Context, to, ws, link string) error {
	f.sent = append(f.sent, struct{ to, ws, link string }{to, ws, link})
	return nil
}
