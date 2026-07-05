//go:build wireinject
// +build wireinject

package di

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/google/wire"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	appidentity "github.com/papyrus/platform/internal/application/identity"
	"github.com/papyrus/platform/internal/config"
	domainidentity "github.com/papyrus/platform/internal/domain/identity"
	"github.com/papyrus/platform/internal/infrastructure/httpserver"
	"github.com/papyrus/platform/internal/infrastructure/mail"
	pgc "github.com/papyrus/platform/internal/infrastructure/postgres"
	rdc "github.com/papyrus/platform/internal/infrastructure/redis"
	"github.com/papyrus/platform/internal/infrastructure/security"
	apphttp "github.com/papyrus/platform/internal/presentation/http"
)

// App holds the wired application dependencies.
type App struct {
	Config config.Config
	DB     *pgxpool.Pool
	Redis  *goredis.Client
	Server *http.Server
}

func provideDB(ctx context.Context, cfg config.Config) (*pgxpool.Pool, error) {
	return pgc.Connect(ctx, cfg.DB.DSN())
}

func provideRedis(ctx context.Context, cfg config.Config) (*goredis.Client, error) {
	return rdc.Connect(ctx, cfg.Redis.Addr())
}

func provideUserRepo(pool *pgxpool.Pool) domainidentity.UserRepository {
	return pgc.NewUserRepository(pool)
}

func provideTokens(client *goredis.Client) domainidentity.VerificationTokens {
	return rdc.NewTokenStore(client)
}

func provideHasher() domainidentity.PasswordHasher {
	return security.NewBcryptHasher(0)
}

func provideMailer(cfg config.Config) domainidentity.Mailer {
	if cfg.Mail.Mode == "smtp" {
		return mail.NewSMTPMailer(cfg.Mail.Host, cfg.Mail.Port, cfg.Mail.User, cfg.Mail.Password, cfg.Mail.From)
	}
	return mail.NewLogMailer(log.New(os.Stdout, "", log.LstdFlags))
}

func provideIdentityHandlers(cfg config.Config, users domainidentity.UserRepository,
	hasher domainidentity.PasswordHasher, tokens domainidentity.VerificationTokens,
	mailer domainidentity.Mailer) *apphttp.IdentityHandlers {
	return apphttp.NewIdentityHandlers(
		appidentity.NewRegisterUser(users, hasher, tokens, mailer, cfg.BaseURL),
		appidentity.NewVerifyEmail(users, tokens),
		appidentity.NewRequestPasswordReset(users, tokens, mailer, cfg.BaseURL),
		appidentity.NewResetPassword(users, hasher, tokens),
	)
}

func provideServer(cfg config.Config, identity *apphttp.IdentityHandlers) *http.Server {
	return httpserver.NewServer(":"+cfg.Port, httpserver.NewRouter(identity))
}

// InitializeApp builds the full application graph.
func InitializeApp(ctx context.Context, cfg config.Config) (*App, error) {
	wire.Build(
		provideDB,
		provideRedis,
		provideUserRepo,
		provideTokens,
		provideHasher,
		provideMailer,
		provideIdentityHandlers,
		provideServer,
		wire.Struct(new(App), "*"),
	)
	return nil, nil
}
