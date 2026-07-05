package http_test

import (
	"context"
	"time"

	domain "github.com/papyrus/platform/internal/domain/identity"
)

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
