package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadReadsEnv(t *testing.T) {
	t.Setenv("PORT", "9999")
	t.Setenv("DB_HOST", "db.example")
	t.Setenv("DB_PORT", "5432")
	t.Setenv("DB_USER", "u")
	t.Setenv("DB_PASSWORD", "p")
	t.Setenv("DB_NAME", "n")
	t.Setenv("REDIS_HOST", "redis.example")
	t.Setenv("REDIS_PORT", "6379")

	cfg, err := Load()

	require.NoError(t, err)
	require.Equal(t, "9999", cfg.Port)
	require.Equal(t, "db.example", cfg.DB.Host)
	require.Equal(t, "redis.example", cfg.Redis.Host)
	require.Equal(t, "postgres://u:p@db.example:5432/n?sslmode=disable", cfg.DB.DSN())
}

func TestLoadFailsOnMissingRequired(t *testing.T) {
	t.Setenv("PORT", "")
	t.Setenv("DB_HOST", "")
	_, err := Load()
	require.Error(t, err)
}

func TestLoadReadsBaseURLAndMail(t *testing.T) {
	t.Setenv("PORT", "9999")
	t.Setenv("DB_HOST", "db")
	t.Setenv("DB_PORT", "5432")
	t.Setenv("DB_USER", "u")
	t.Setenv("DB_PASSWORD", "p")
	t.Setenv("DB_NAME", "n")
	t.Setenv("REDIS_HOST", "r")
	t.Setenv("REDIS_PORT", "6379")
	t.Setenv("BASE_URL", "https://acc.example")
	t.Setenv("MAIL_MODE", "log")

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, "https://acc.example", cfg.BaseURL)
	require.Equal(t, "log", cfg.Mail.Mode)
}

func TestLoadReadsTrustedClients(t *testing.T) {
	t.Setenv("PORT", "9999")
	t.Setenv("DB_HOST", "db")
	t.Setenv("DB_PORT", "5432")
	t.Setenv("DB_USER", "u")
	t.Setenv("DB_PASSWORD", "p")
	t.Setenv("DB_NAME", "n")
	t.Setenv("REDIS_HOST", "r")
	t.Setenv("REDIS_PORT", "6379")
	t.Setenv("TRUSTED_CLIENT_IDS", "papyrus,lite")

	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, []string{"papyrus", "lite"}, cfg.TrustedClientIDs)
}
