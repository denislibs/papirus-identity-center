package main

import (
	"context"
	"log"

	"github.com/denislibs/papirus-identity-center/internal/config"
	"github.com/denislibs/papirus-identity-center/internal/infrastructure/di"
	"github.com/denislibs/papirus-identity-center/internal/infrastructure/postgres"
)

func main() {
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	if err := postgres.RunMigrations(cfg.DB.DSN()); err != nil {
		log.Fatalf("migrations: %v", err)
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
