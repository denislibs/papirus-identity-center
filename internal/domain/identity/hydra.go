package identity

import "context"

// OAuthClientInfo is the subset of an OAuth2 client we care about.
type OAuthClientInfo struct {
	ID      string
	Name    string
	Trusted bool // our own products → auto-consent
}

// HydraLoginRequest is Hydra's view of a pending login.
type HydraLoginRequest struct {
	Challenge string
	Skip      bool   // Hydra already has an authenticated session
	Subject   string // set when Skip is true
	Client    OAuthClientInfo
}

// HydraConsentRequest is Hydra's view of a pending consent.
type HydraConsentRequest struct {
	Challenge       string
	Skip            bool
	Subject         string
	LoginSessionID  string // Hydra "sid" — stored in our Session
	RequestedScopes []string
	Client          OAuthClientInfo
}

// HydraClient wraps the Ory Hydra admin API operations we use.
type HydraClient interface {
	GetLoginRequest(ctx context.Context, challenge string) (*HydraLoginRequest, error)
	AcceptLoginRequest(ctx context.Context, challenge, subject string, remember bool) (redirectTo string, err error)
	RejectLoginRequest(ctx context.Context, challenge, reason string) (redirectTo string, err error)
	GetConsentRequest(ctx context.Context, challenge string) (*HydraConsentRequest, error)
	AcceptConsentRequest(ctx context.Context, challenge string, grantScopes []string) (redirectTo string, err error)
}
