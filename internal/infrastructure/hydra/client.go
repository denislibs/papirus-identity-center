package hydra

import (
	"context"
	"fmt"

	ory "github.com/ory/hydra-client-go/v2"

	"github.com/papyrus/platform/internal/domain/identity"
)

// Client implements identity.HydraClient using the Ory Hydra admin API.
type Client struct {
	api *ory.APIClient
	// trusted holds OAuth client IDs treated as first-party (auto-consent).
	trusted map[string]bool
}

// New builds a Hydra admin client pointed at adminURL (e.g. http://hydra:4445),
// treating the given client IDs as trusted.
func New(adminURL string, trustedClientIDs []string) *Client {
	cfg := ory.NewConfiguration()
	cfg.Servers = ory.ServerConfigurations{{URL: adminURL}}
	trusted := make(map[string]bool, len(trustedClientIDs))
	for _, id := range trustedClientIDs {
		trusted[id] = true
	}
	return &Client{api: ory.NewAPIClient(cfg), trusted: trusted}
}

// clientInfo maps an SDK OAuth2Client (value, not pointer) to our OAuthClientInfo DTO.
// In the login request the client is a value type; in the consent request it is a pointer.
func (c *Client) clientInfoFromValue(cl ory.OAuth2Client) identity.OAuthClientInfo {
	info := identity.OAuthClientInfo{}
	if cl.ClientId != nil {
		info.ID = *cl.ClientId
	}
	if cl.ClientName != nil {
		info.Name = *cl.ClientName
	}
	info.Trusted = c.trusted[info.ID]
	return info
}

// clientInfoFromPtr is the same mapping but accepts a *OAuth2Client (consent request).
func (c *Client) clientInfoFromPtr(cl *ory.OAuth2Client) identity.OAuthClientInfo {
	if cl == nil {
		return identity.OAuthClientInfo{}
	}
	return c.clientInfoFromValue(*cl)
}

// GetLoginRequest fetches a Hydra login request by challenge.
func (c *Client) GetLoginRequest(ctx context.Context, challenge string) (*identity.HydraLoginRequest, error) {
	req, _, err := c.api.OAuth2API.GetOAuth2LoginRequest(ctx).LoginChallenge(challenge).Execute()
	if err != nil {
		return nil, fmt.Errorf("hydra: get login request: %w", err)
	}
	// In SDK v2.2.1, OAuth2LoginRequest.Skip and Subject are non-pointer value types.
	return &identity.HydraLoginRequest{
		Challenge: challenge,
		Skip:      req.Skip,
		Subject:   req.Subject,
		Client:    c.clientInfoFromValue(req.Client),
	}, nil
}

// AcceptLoginRequest accepts a pending login request in Hydra and returns the redirect URL.
func (c *Client) AcceptLoginRequest(ctx context.Context, challenge, subject string, remember bool) (string, error) {
	body := ory.NewAcceptOAuth2LoginRequest(subject)
	body.SetRemember(remember)
	res, _, err := c.api.OAuth2API.AcceptOAuth2LoginRequest(ctx).
		LoginChallenge(challenge).AcceptOAuth2LoginRequest(*body).Execute()
	if err != nil {
		return "", fmt.Errorf("hydra: accept login: %w", err)
	}
	return res.RedirectTo, nil
}

// RejectLoginRequest rejects a pending login request in Hydra and returns the redirect URL.
func (c *Client) RejectLoginRequest(ctx context.Context, challenge, reason string) (string, error) {
	body := ory.NewRejectOAuth2Request()
	body.SetError(reason)
	res, _, err := c.api.OAuth2API.RejectOAuth2LoginRequest(ctx).
		LoginChallenge(challenge).RejectOAuth2Request(*body).Execute()
	if err != nil {
		return "", fmt.Errorf("hydra: reject login: %w", err)
	}
	return res.RedirectTo, nil
}

// GetConsentRequest fetches a Hydra consent request by challenge.
func (c *Client) GetConsentRequest(ctx context.Context, challenge string) (*identity.HydraConsentRequest, error) {
	req, _, err := c.api.OAuth2API.GetOAuth2ConsentRequest(ctx).ConsentChallenge(challenge).Execute()
	if err != nil {
		return nil, fmt.Errorf("hydra: get consent request: %w", err)
	}
	// In SDK v2.2.1, OAuth2ConsentRequest.Skip, Subject, and LoginSessionId are pointer types.
	out := &identity.HydraConsentRequest{
		Challenge:       challenge,
		RequestedScopes: req.RequestedScope,
		Client:          c.clientInfoFromPtr(req.Client),
	}
	if req.Skip != nil {
		out.Skip = *req.Skip
	}
	if req.Subject != nil {
		out.Subject = *req.Subject
	}
	if req.LoginSessionId != nil {
		out.LoginSessionID = *req.LoginSessionId
	}
	return out, nil
}

// AcceptConsentRequest accepts a pending consent request in Hydra and returns the redirect URL.
func (c *Client) AcceptConsentRequest(ctx context.Context, challenge string, grantScopes []string) (string, error) {
	body := ory.NewAcceptOAuth2ConsentRequest()
	body.SetGrantScope(grantScopes)
	res, _, err := c.api.OAuth2API.AcceptOAuth2ConsentRequest(ctx).
		ConsentChallenge(challenge).AcceptOAuth2ConsentRequest(*body).Execute()
	if err != nil {
		return "", fmt.Errorf("hydra: accept consent: %w", err)
	}
	return res.RedirectTo, nil
}

// RejectConsentRequest rejects a pending consent request in Hydra and returns the redirect URL.
func (c *Client) RejectConsentRequest(ctx context.Context, challenge, reason string) (string, error) {
	body := ory.NewRejectOAuth2Request()
	body.SetError(reason)
	res, _, err := c.api.OAuth2API.RejectOAuth2ConsentRequest(ctx).
		ConsentChallenge(challenge).RejectOAuth2Request(*body).Execute()
	if err != nil {
		return "", fmt.Errorf("hydra: reject consent: %w", err)
	}
	return res.RedirectTo, nil
}

// RevokeLoginSessionsBySubject terminates all Hydra login sessions for a subject.
func (c *Client) RevokeLoginSessionsBySubject(ctx context.Context, subject string) error {
	_, err := c.api.OAuth2API.RevokeOAuth2LoginSessions(ctx).Subject(subject).Execute()
	if err != nil {
		return fmt.Errorf("hydra: revoke sessions by subject: %w", err)
	}
	return nil
}

// RevokeLoginSessionByID terminates a single Hydra login session by its sid.
func (c *Client) RevokeLoginSessionByID(ctx context.Context, sid string) error {
	_, err := c.api.OAuth2API.RevokeOAuth2LoginSessions(ctx).Sid(sid).Execute()
	if err != nil {
		return fmt.Errorf("hydra: revoke session by sid: %w", err)
	}
	return nil
}

// IntrospectToken validates an access token and returns whether it is active and the subject.
func (c *Client) IntrospectToken(ctx context.Context, token string) (bool, string, error) {
	res, _, err := c.api.OAuth2API.IntrospectOAuth2Token(ctx).Token(token).Execute()
	if err != nil {
		return false, "", fmt.Errorf("hydra: introspect token: %w", err)
	}
	if !res.Active {
		return false, "", nil
	}
	subject := ""
	if res.Sub != nil {
		subject = *res.Sub
	}
	return true, subject, nil
}

// Compile-time assertion: Client must implement identity.HydraClient.
var _ identity.HydraClient = (*Client)(nil)
