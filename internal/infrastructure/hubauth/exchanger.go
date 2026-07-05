package hubauth

import (
	"context"
	"fmt"

	"golang.org/x/oauth2"

	"github.com/denislibs/papirus-identity-center/internal/domain/identity"
)

// Exchanger implements identity.CodeExchanger using the OAuth2 code flow against
// Hydra, then resolves the subject via token introspection.
type Exchanger struct {
	oauth      *oauth2.Config
	introspect identity.HydraClient
}

// New builds an Exchanger. authBaseURL is the BROWSER-facing Hydra base (for the
// redirect to /oauth2/auth, e.g. http://localhost:4444); tokenBaseURL is the
// server-reachable base for the token exchange (in Docker http://hydra:4444).
func New(clientID, clientSecret, redirectURL, authBaseURL, tokenBaseURL string, introspect identity.HydraClient) *Exchanger {
	return &Exchanger{
		oauth: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Scopes:       []string{"openid", "profile"},
			Endpoint: oauth2.Endpoint{
				AuthURL:  authBaseURL + "/oauth2/auth",
				TokenURL: tokenBaseURL + "/oauth2/token",
			},
		},
		introspect: introspect,
	}
}

// AuthCodeURL returns the URL to redirect the browser to, with the given state.
func (e *Exchanger) AuthCodeURL(state string) string {
	return e.oauth.AuthCodeURL(state)
}

// ExchangeForSubject swaps the code for a token and introspects it for the subject.
func (e *Exchanger) ExchangeForSubject(ctx context.Context, code string) (string, error) {
	tok, err := e.oauth.Exchange(ctx, code)
	if err != nil {
		return "", fmt.Errorf("hubauth: exchange code: %w", err)
	}
	active, subject, err := e.introspect.IntrospectToken(ctx, tok.AccessToken)
	if err != nil {
		return "", fmt.Errorf("hubauth: introspect: %w", err)
	}
	if !active || subject == "" {
		return "", fmt.Errorf("hubauth: token inactive")
	}
	return subject, nil
}

var _ identity.CodeExchanger = (*Exchanger)(nil)
