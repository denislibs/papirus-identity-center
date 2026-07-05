package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/papyrus/platform/internal/domain/identity"
)

// UserRepository is a pgx-backed identity.UserRepository.
type UserRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

func (r *UserRepository) Create(ctx context.Context, u *identity.User) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO users (id, email, email_verified, password_hash, name, avatar_url, locale, timezone, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		u.ID, u.Email, u.EmailVerified, u.PasswordHash, u.Name, u.AvatarURL, u.Locale, u.Timezone, u.CreatedAt)
	if err != nil {
		return fmt.Errorf("postgres: create user: %w", err)
	}
	return nil
}

func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*identity.User, error) {
	return r.scanOne(ctx,
		`SELECT id, email, email_verified, password_hash, name, avatar_url, locale, timezone, created_at
		 FROM users WHERE email = $1`, email)
}

func (r *UserRepository) FindByID(ctx context.Context, id string) (*identity.User, error) {
	return r.scanOne(ctx,
		`SELECT id, email, email_verified, password_hash, name, avatar_url, locale, timezone, created_at
		 FROM users WHERE id = $1`, id)
}

func (r *UserRepository) Update(ctx context.Context, u *identity.User) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE users SET email=$2, email_verified=$3, password_hash=$4, name=$5, avatar_url=$6, locale=$7, timezone=$8
		 WHERE id=$1`,
		u.ID, u.Email, u.EmailVerified, u.PasswordHash, u.Name, u.AvatarURL, u.Locale, u.Timezone)
	if err != nil {
		return fmt.Errorf("postgres: update user: %w", err)
	}
	return nil
}

func (r *UserRepository) scanOne(ctx context.Context, query string, arg any) (*identity.User, error) {
	var u identity.User
	err := r.pool.QueryRow(ctx, query, arg).Scan(
		&u.ID, &u.Email, &u.EmailVerified, &u.PasswordHash, &u.Name, &u.AvatarURL, &u.Locale, &u.Timezone, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, identity.ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: find user: %w", err)
	}
	return &u, nil
}
