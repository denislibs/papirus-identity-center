package config

import (
	"fmt"
	"os"
)

type DBConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
}

func (c DBConfig) DSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		c.User, c.Password, c.Host, c.Port, c.Name)
}

type RedisConfig struct {
	Host string
	Port string
}

func (c RedisConfig) Addr() string {
	return fmt.Sprintf("%s:%s", c.Host, c.Port)
}

type HydraConfig struct {
	AdminURL  string
	PublicURL string
}

type MailConfig struct {
	Mode     string // "log" (default) or "smtp"
	Host     string
	Port     string
	User     string
	Password string
	From     string
}

type Config struct {
	Port    string
	DB      DBConfig
	Redis   RedisConfig
	Hydra   HydraConfig
	BaseURL string
	Mail    MailConfig
}

func Load() (Config, error) {
	cfg := Config{
		Port: os.Getenv("PORT"),
		DB: DBConfig{
			Host:     os.Getenv("DB_HOST"),
			Port:     os.Getenv("DB_PORT"),
			User:     os.Getenv("DB_USER"),
			Password: os.Getenv("DB_PASSWORD"),
			Name:     os.Getenv("DB_NAME"),
		},
		Redis: RedisConfig{
			Host: os.Getenv("REDIS_HOST"),
			Port: os.Getenv("REDIS_PORT"),
		},
		Hydra: HydraConfig{
			AdminURL:  os.Getenv("HYDRA_ADMIN_URL"),
			PublicURL: os.Getenv("HYDRA_PUBLIC_URL"),
		},
	}

	if cfg.Port == "" || cfg.DB.Host == "" || cfg.DB.Port == "" ||
		cfg.DB.User == "" || cfg.DB.Name == "" || cfg.Redis.Host == "" ||
		cfg.Redis.Port == "" {
		return Config{}, fmt.Errorf("config: missing required environment variables")
	}

	cfg.BaseURL = os.Getenv("BASE_URL")
	if cfg.BaseURL == "" {
		cfg.BaseURL = "http://localhost:" + cfg.Port
	}
	cfg.Mail = MailConfig{
		Mode:     os.Getenv("MAIL_MODE"),
		Host:     os.Getenv("SMTP_HOST"),
		Port:     os.Getenv("SMTP_PORT"),
		User:     os.Getenv("SMTP_USER"),
		Password: os.Getenv("SMTP_PASSWORD"),
		From:     os.Getenv("SMTP_FROM"),
	}
	if cfg.Mail.Mode == "" {
		cfg.Mail.Mode = "log"
	}

	return cfg, nil
}
