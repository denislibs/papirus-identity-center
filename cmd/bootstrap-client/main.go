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

	c := ory.NewOAuth2Client()
	c.SetClientId("papyrus")
	c.SetClientName("Papyrus")
	c.SetGrantTypes([]string{"authorization_code", "refresh_token"})
	c.SetResponseTypes([]string{"code"})
	c.SetRedirectUris([]string{redirect})
	c.SetScope("openid profile")
	c.SetTokenEndpointAuthMethod("none") // public client for smoke test

	created, resp, err := api.OAuth2API.CreateOAuth2Client(context.Background()).OAuth2Client(*c).Execute()
	if err != nil {
		if resp != nil && resp.StatusCode == 409 {
			log.Println("client already exists — ok")
			return
		}
		log.Fatalf("create client: %v", err)
	}
	log.Printf("created client: %s", *created.ClientId)
}
