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

	appidentity "github.com/denislibs/papirus-identity-center/internal/application/identity"
	"github.com/denislibs/papirus-identity-center/internal/config"
	domainidentity "github.com/denislibs/papirus-identity-center/internal/domain/identity"
	"github.com/denislibs/papirus-identity-center/internal/infrastructure/httpserver"
	"github.com/denislibs/papirus-identity-center/internal/infrastructure/hydra"
	"github.com/denislibs/papirus-identity-center/internal/infrastructure/mail"
	pgc "github.com/denislibs/papirus-identity-center/internal/infrastructure/postgres"
	rdc "github.com/denislibs/papirus-identity-center/internal/infrastructure/redis"
	"github.com/denislibs/papirus-identity-center/internal/infrastructure/security"
	apphttp "github.com/denislibs/papirus-identity-center/internal/presentation/http"
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

func provideSessionRepo(pool *pgxpool.Pool) domainidentity.SessionRepository {
	return pgc.NewSessionRepository(pool)
}

func provideHydraClient(cfg config.Config) domainidentity.HydraClient {
	return hydra.New(cfg.Hydra.AdminURL, cfg.TrustedClientIDs)
}

func provideAuthHandlers(users domainidentity.UserRepository, hasher domainidentity.PasswordHasher,
	hydraClient domainidentity.HydraClient, sessions domainidentity.SessionRepository) *apphttp.AuthHandlers {
	return apphttp.NewAuthHandlers(
		appidentity.NewAuthenticate(users, hasher),
		hydraClient, sessions, apphttp.MustLoadTemplates(),
	)
}

func provideSessionHandlers(sessions domainidentity.SessionRepository, hydraClient domainidentity.HydraClient) *apphttp.SessionHandlers {
	return apphttp.NewSessionHandlers(
		appidentity.NewListSessions(sessions),
		appidentity.NewTerminateSession(sessions, hydraClient),
		appidentity.NewTerminateAllSessions(sessions, hydraClient),
	)
}

func provideServer(cfg config.Config, identity *apphttp.IdentityHandlers, auth *apphttp.AuthHandlers,
	sessions *apphttp.SessionHandlers, hydraClient domainidentity.HydraClient) *http.Server {
	return httpserver.NewServer(":"+cfg.Port, httpserver.NewRouter(identity, auth, sessions, hydraClient))
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
		provideSessionRepo,
		provideHydraClient,
		provideAuthHandlers,
		provideSessionHandlers,
		provideServer,
		wire.Struct(new(App), "*"),
	)
	return nil, nil
}
