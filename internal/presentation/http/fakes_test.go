package http_test

import (
	"context"
	"time"

	domain "github.com/denislibs/papirus-identity-center/internal/domain/identity"
)

// fakeHydra implements identity.HydraClient.
type fakeHydra struct {
	login             *domain.HydraLoginRequest
	consent           *domain.HydraConsentRequest
	acceptedSub       string
	grantedScopes     []string
	consentRejected   bool
	redirect          string
	revokedSubject    string
	revokedSID        string
	introspectActive  bool
	introspectSubject string
}

func (f *fakeHydra) GetLoginRequest(_ context.Context, ch string) (*domain.HydraLoginRequest, error) {
	if f.login == nil {
		return &domain.HydraLoginRequest{Challenge: ch}, nil
	}
	f.login.Challenge = ch
	return f.login, nil
}
func (f *fakeHydra) AcceptLoginRequest(_ context.Context, ch, sub string, _ bool) (string, error) {
	f.acceptedSub = sub
	return f.redirect, nil
}
func (f *fakeHydra) RejectLoginRequest(_ context.Context, ch, reason string) (string, error) {
	return f.redirect, nil
}
func (f *fakeHydra) GetConsentRequest(_ context.Context, ch string) (*domain.HydraConsentRequest, error) {
	if f.consent == nil {
		return &domain.HydraConsentRequest{Challenge: ch, Client: domain.OAuthClientInfo{Trusted: true}}, nil
	}
	f.consent.Challenge = ch
	return f.consent, nil
}
func (f *fakeHydra) AcceptConsentRequest(_ context.Context, ch string, scopes []string) (string, error) {
	f.grantedScopes = scopes
	return f.redirect, nil
}
func (f *fakeHydra) RejectConsentRequest(_ context.Context, ch, reason string) (string, error) {
	f.consentRejected = true
	return f.redirect, nil
}
func (f *fakeHydra) RevokeLoginSessionsBySubject(_ context.Context, subject string) error {
	f.revokedSubject = subject
	return nil
}
func (f *fakeHydra) RevokeLoginSessionByID(_ context.Context, sid string) error {
	f.revokedSID = sid
	return nil
}
func (f *fakeHydra) IntrospectToken(_ context.Context, _ string) (bool, string, error) {
	return f.introspectActive, f.introspectSubject, nil
}

// fakeSessions implements identity.SessionRepository as a real in-memory store.
// The `created` slice is kept so existing consent-handler tests that assert
// sessions.created still compile and pass.  Create lazily inits maps so that
// &fakeSessions{} call sites continue to compile.
type fakeSessions struct {
	created []*domain.Session
	byID    map[string]*domain.Session
	ended   map[string]bool
}

func newFakeSessions() *fakeSessions {
	return &fakeSessions{byID: map[string]*domain.Session{}, ended: map[string]bool{}}
}
func (f *fakeSessions) Create(_ context.Context, s *domain.Session) error {
	if f.byID == nil {
		f.byID = map[string]*domain.Session{}
		f.ended = map[string]bool{}
	}
	cp := *s
	f.created = append(f.created, &cp)
	f.byID[s.ID] = &cp
	return nil
}
func (f *fakeSessions) FindByID(_ context.Context, id string) (*domain.Session, error) {
	if s, ok := f.byID[id]; ok {
		cp := *s
		return &cp, nil
	}
	return nil, domain.ErrSessionNotFound
}
func (f *fakeSessions) ListActiveByUser(_ context.Context, userID string) ([]*domain.Session, error) {
	var out []*domain.Session
	for _, s := range f.created {
		if s.UserID == userID && !f.ended[s.ID] {
			cp := *s
			out = append(out, &cp)
		}
	}
	return out, nil
}
func (f *fakeSessions) MarkEnded(_ context.Context, id string) error { f.ended[id] = true; return nil }
func (f *fakeSessions) MarkEndedByHydraSID(_ context.Context, _ string) error { return nil }
func (f *fakeSessions) MarkAllEndedByUser(_ context.Context, userID string) error {
	for _, s := range f.created {
		if s.UserID == userID {
			f.ended[s.ID] = true
		}
	}
	return nil
}

type fakeUsers struct{ byID, byEmail map[string]*domain.User }

func newFakeUsers() *fakeUsers {
	return &fakeUsers{byID: map[string]*domain.User{}, byEmail: map[string]*domain.User{}}
}
func (f *fakeUsers) Create(_ context.Context, u *domain.User) error {
	cp := *u
	f.byID[u.ID] = &cp
	f.byEmail[u.Email] = &cp
	return nil
}
func (f *fakeUsers) FindByEmail(_ context.Context, email string) (*domain.User, error) {
	if u, ok := f.byEmail[email]; ok {
		cp := *u
		return &cp, nil
	}
	return nil, domain.ErrUserNotFound
}
func (f *fakeUsers) FindByID(_ context.Context, id string) (*domain.User, error) {
	if u, ok := f.byID[id]; ok {
		cp := *u
		return &cp, nil
	}
	return nil, domain.ErrUserNotFound
}
func (f *fakeUsers) Update(_ context.Context, u *domain.User) error {
	cp := *u
	f.byID[u.ID] = &cp
	f.byEmail[u.Email] = &cp
	return nil
}

type fakeHasher struct{}

func (fakeHasher) Hash(plain string) (string, error) { return "hashed:" + plain, nil }
func (fakeHasher) Check(hash, plain string) bool      { return hash == "hashed:"+plain }

type fakeTokens struct {
	lastToken string
	store     map[string]string
}

func newFakeTokens() *fakeTokens { return &fakeTokens{store: map[string]string{}} }
func (f *fakeTokens) Issue(_ context.Context, purpose, userID string, _ time.Duration) (string, error) {
	f.lastToken = "tok-" + userID
	f.store[purpose+":"+f.lastToken] = userID
	return f.lastToken, nil
}
func (f *fakeTokens) Consume(_ context.Context, purpose, token string) (string, error) {
	k := purpose + ":" + token
	if uid, ok := f.store[k]; ok {
		delete(f.store, k)
		return uid, nil
	}
	return "", domain.ErrTokenInvalid
}

type sentMail struct{ to, link string }
type fakeMailer struct {
	verifications []sentMail
	resets        []sentMail
}

func newFakeMailer() *fakeMailer { return &fakeMailer{} }
func (f *fakeMailer) SendVerification(_ context.Context, to, link string) error {
	f.verifications = append(f.verifications, sentMail{to, link})
	return nil
}
func (f *fakeMailer) SendPasswordReset(_ context.Context, to, link string) error {
	f.resets = append(f.resets, sentMail{to, link})
	return nil
}
