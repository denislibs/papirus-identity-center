package identity_test

import (
	"context"

	domain "github.com/papyrus/platform/internal/domain/identity"
)

// fakeSessionRepo implements domain.SessionRepository for use-case tests.
type fakeSessionRepo struct {
	byID     map[string]*domain.Session
	ended    map[string]bool
	allEnded map[string]bool
}

func newFakeSessionRepo() *fakeSessionRepo {
	return &fakeSessionRepo{byID: map[string]*domain.Session{}, ended: map[string]bool{}, allEnded: map[string]bool{}}
}
func (f *fakeSessionRepo) Create(_ context.Context, s *domain.Session) error {
	cp := *s
	f.byID[s.ID] = &cp
	return nil
}
func (f *fakeSessionRepo) FindByID(_ context.Context, id string) (*domain.Session, error) {
	if s, ok := f.byID[id]; ok {
		cp := *s
		return &cp, nil
	}
	return nil, domain.ErrSessionNotFound
}
func (f *fakeSessionRepo) ListActiveByUser(_ context.Context, userID string) ([]*domain.Session, error) {
	var out []*domain.Session
	for _, s := range f.byID {
		if s.UserID == userID && !f.ended[s.ID] {
			cp := *s
			out = append(out, &cp)
		}
	}
	return out, nil
}
func (f *fakeSessionRepo) MarkEnded(_ context.Context, id string) error { f.ended[id] = true; return nil }
func (f *fakeSessionRepo) MarkEndedByHydraSID(_ context.Context, _ string) error { return nil }
func (f *fakeSessionRepo) MarkAllEndedByUser(_ context.Context, userID string) error {
	f.allEnded[userID] = true
	return nil
}

// fakeHydraAdmin implements the subset of domain.HydraClient used by session use-cases.
// It embeds a no-op for the login/consent methods so it satisfies the full interface.
type fakeHydraAdmin struct {
	revokedSubject string
	revokedSID     string
}

func (f *fakeHydraAdmin) GetLoginRequest(context.Context, string) (*domain.HydraLoginRequest, error) {
	return &domain.HydraLoginRequest{}, nil
}
func (f *fakeHydraAdmin) AcceptLoginRequest(context.Context, string, string, bool) (string, error) {
	return "", nil
}
func (f *fakeHydraAdmin) RejectLoginRequest(context.Context, string, string) (string, error) {
	return "", nil
}
func (f *fakeHydraAdmin) GetConsentRequest(context.Context, string) (*domain.HydraConsentRequest, error) {
	return &domain.HydraConsentRequest{}, nil
}
func (f *fakeHydraAdmin) AcceptConsentRequest(context.Context, string, []string) (string, error) {
	return "", nil
}
func (f *fakeHydraAdmin) RejectConsentRequest(context.Context, string, string) (string, error) {
	return "", nil
}
func (f *fakeHydraAdmin) RevokeLoginSessionsBySubject(_ context.Context, subject string) error {
	f.revokedSubject = subject
	return nil
}
func (f *fakeHydraAdmin) RevokeLoginSessionByID(_ context.Context, sid string) error {
	f.revokedSID = sid
	return nil
}
func (f *fakeHydraAdmin) IntrospectToken(context.Context, string) (bool, string, error) {
	return false, "", nil
}
