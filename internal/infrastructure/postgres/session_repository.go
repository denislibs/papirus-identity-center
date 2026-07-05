package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/denislibs/papirus-identity-center/internal/domain/identity"
)

// SessionRepository is a pgx-backed identity.SessionRepository.
type SessionRepository struct {
	pool *pgxpool.Pool
}

func NewSessionRepository(pool *pgxpool.Pool) *SessionRepository {
	return &SessionRepository{pool: pool}
}

func (r *SessionRepository) Create(ctx context.Context, s *identity.Session) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO sessions (id, user_id, hydra_session_id, device_name, user_agent, ip, location, created_at, last_seen_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		s.ID, s.UserID, s.HydraSessionID, s.DeviceName, s.UserAgent, s.IP, s.Location, s.CreatedAt, s.LastSeenAt)
	if err != nil {
		return fmt.Errorf("postgres: create session: %w", err)
	}
	return nil
}

func (r *SessionRepository) FindByID(ctx context.Context, id string) (*identity.Session, error) {
	var s identity.Session
	err := r.pool.QueryRow(ctx,
		`SELECT id, user_id, hydra_session_id, device_name, user_agent, ip, location, created_at, last_seen_at, ended_at
		 FROM sessions WHERE id=$1`, id).
		Scan(&s.ID, &s.UserID, &s.HydraSessionID, &s.DeviceName, &s.UserAgent, &s.IP, &s.Location, &s.CreatedAt, &s.LastSeenAt, &s.EndedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, identity.ErrSessionNotFound
	}
	if err != nil {
		var pgErr *pgconn.PgError
		// 22P02 = invalid_text_representation (e.g. non-UUID string passed as UUID)
		if errors.As(err, &pgErr) && pgErr.Code == "22P02" {
			return nil, identity.ErrSessionNotFound
		}
		return nil, fmt.Errorf("postgres: find session: %w", err)
	}
	return &s, nil
}

func (r *SessionRepository) ListActiveByUser(ctx context.Context, userID string) ([]*identity.Session, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, user_id, hydra_session_id, device_name, user_agent, ip, location, created_at, last_seen_at, ended_at
		 FROM sessions WHERE user_id=$1 AND ended_at IS NULL ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("postgres: list sessions: %w", err)
	}
	defer rows.Close()

	var out []*identity.Session
	for rows.Next() {
		var s identity.Session
		if err := rows.Scan(&s.ID, &s.UserID, &s.HydraSessionID, &s.DeviceName, &s.UserAgent, &s.IP, &s.Location, &s.CreatedAt, &s.LastSeenAt, &s.EndedAt); err != nil {
			return nil, fmt.Errorf("postgres: scan session: %w", err)
		}
		out = append(out, &s)
	}
	return out, rows.Err()
}

func (r *SessionRepository) MarkEnded(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `UPDATE sessions SET ended_at=now() WHERE id=$1 AND ended_at IS NULL`, id)
	if err != nil {
		return fmt.Errorf("postgres: end session: %w", err)
	}
	return nil
}

func (r *SessionRepository) MarkEndedByHydraSID(ctx context.Context, sid string) error {
	_, err := r.pool.Exec(ctx, `UPDATE sessions SET ended_at=now() WHERE hydra_session_id=$1 AND ended_at IS NULL`, sid)
	if err != nil {
		return fmt.Errorf("postgres: end session by sid: %w", err)
	}
	return nil
}

func (r *SessionRepository) MarkAllEndedByUser(ctx context.Context, userID string) error {
	_, err := r.pool.Exec(ctx, `UPDATE sessions SET ended_at=now() WHERE user_id=$1 AND ended_at IS NULL`, userID)
	if err != nil {
		return fmt.Errorf("postgres: end all sessions: %w", err)
	}
	return nil
}
