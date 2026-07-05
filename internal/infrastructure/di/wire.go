//go:build wireinject
// +build wireinject

package di

import (
	"context"
	"net/http"

	"github.com/google/wire"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/papyrus/platform/internal/config"
	"github.com/papyrus/platform/internal/infrastructure/httpserver"
	pgc "github.com/papyrus/platform/internal/infrastructure/postgres"
	rdc "github.com/papyrus/platform/internal/infrastructure/redis"
)

// App holds the wired application dependencies.
type App struct {
	Config config.Config
	DB     *pgxpool.Pool
	Redis  *redis.Client
	Server *http.Server
}

func provideDB(ctx context.Context, cfg config.Config) (*pgxpool.Pool, error) {
	return pgc.Connect(ctx, cfg.DB.DSN())
}

func provideRedis(ctx context.Context, cfg config.Config) (*redis.Client, error) {
	return rdc.Connect(ctx, cfg.Redis.Addr())
}

func provideServer(cfg config.Config) *http.Server {
	return httpserver.NewServer(":"+cfg.Port, httpserver.NewRouter())
}

// InitializeApp builds the full application graph.
func InitializeApp(ctx context.Context, cfg config.Config) (*App, error) {
	wire.Build(
		provideDB,
		provideRedis,
		provideServer,
		wire.Struct(new(App), "*"),
	)
	return nil, nil
}
