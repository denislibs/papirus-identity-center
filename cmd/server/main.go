package main

import (
	"context"
	"log"

	"github.com/papyrus/platform/internal/config"
	"github.com/papyrus/platform/internal/infrastructure/di"
)

func main() {
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	app, err := di.InitializeApp(ctx, cfg)
	if err != nil {
		log.Fatalf("init app: %v", err)
	}
	defer app.DB.Close()
	defer func() { _ = app.Redis.Close() }()

	log.Printf("platform-core listening on :%s", cfg.Port)
	if err := app.Server.ListenAndServe(); err != nil {
		log.Fatalf("server: %v", err)
	}
}
