package main

import (
	"context"
	"log"
	"os"

	ory "github.com/ory/hydra-client-go/v2"
)

func main() {
	adminURL := os.Getenv("HYDRA_ADMIN_URL")
	if adminURL == "" {
		adminURL = "http://localhost:4445"
	}
	redirect := os.Getenv("CLIENT_REDIRECT_URI")
	if redirect == "" {
		redirect = "http://localhost:5555/callback"
	}

	cfg := ory.NewConfiguration()
	cfg.Servers = ory.ServerConfigurations{{URL: adminURL}}
	api := ory.NewAPIClient(cfg)

	ctx := context.Background()

	// Register papyrus (public OIDC client — smoke test / SPA).
	papyrus := ory.NewOAuth2Client()
	papyrus.SetClientId("papyrus")
	papyrus.SetClientName("Papyrus")
	papyrus.SetGrantTypes([]string{"authorization_code", "refresh_token"})
	papyrus.SetResponseTypes([]string{"code"})
	papyrus.SetRedirectUris([]string{redirect})
	papyrus.SetScope("openid profile")
	papyrus.SetTokenEndpointAuthMethod("none") // public client for smoke test

	created, resp, err := api.OAuth2API.CreateOAuth2Client(ctx).OAuth2Client(*papyrus).Execute()
	if err != nil {
		if resp != nil && resp.StatusCode == 409 {
			log.Println("papyrus client already exists — ok")
		} else {
			log.Fatalf("create papyrus client: %v", err)
		}
	} else {
		log.Printf("created client: %s", *created.ClientId)
	}

	// Register hub (confidential OIDC client — account hub server-side flow).
	hubRedirect := os.Getenv("HUB_REDIRECT_URI")
	if hubRedirect == "" {
		hubRedirect = "http://localhost:8090/auth/callback"
	}
	hubSecret := os.Getenv("HUB_CLIENT_SECRET")
	if hubSecret == "" {
		hubSecret = "hub-secret"
	}

	hub := ory.NewOAuth2Client()
	hub.SetClientId("hub")
	hub.SetClientName("Account Hub")
	hub.SetGrantTypes([]string{"authorization_code", "refresh_token"})
	hub.SetResponseTypes([]string{"code"})
	hub.SetRedirectUris([]string{hubRedirect})
	hub.SetScope("openid profile")
	hub.SetTokenEndpointAuthMethod("client_secret_post")
	hub.SetClientSecret(hubSecret)

	createdHub, respHub, errHub := api.OAuth2API.CreateOAuth2Client(ctx).OAuth2Client(*hub).Execute()
	if errHub != nil {
		if respHub != nil && respHub.StatusCode == 409 {
			log.Println("hub client already exists — ok")
		} else {
			log.Fatalf("create hub client: %v", errHub)
		}
	} else {
		log.Printf("created client: %s", *createdHub.ClientId)
	}
}
