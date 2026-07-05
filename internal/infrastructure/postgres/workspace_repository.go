package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/denislibs/papirus-identity-center/internal/domain/workspace"
)

type WorkspaceRepository struct{ pool *pgxpool.Pool }

func NewWorkspaceRepository(pool *pgxpool.Pool) *WorkspaceRepository { return &WorkspaceRepository{pool} }

func (r *WorkspaceRepository) Create(ctx context.Context, w *workspace.Workspace) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO workspaces (id, name, slug, created_by, created_at) VALUES ($1,$2,$3,$4,$5)`,
		w.ID, w.Name, w.Slug, w.CreatedBy, w.CreatedAt)
	if err != nil {
		return fmt.Errorf("postgres: create workspace: %w", err)
	}
	return nil
}

func (r *WorkspaceRepository) FindByID(ctx context.Context, id string) (*workspace.Workspace, error) {
	var w workspace.Workspace
	err := r.pool.QueryRow(ctx, `SELECT id, name, slug, created_by, created_at FROM workspaces WHERE id=$1`, id).
		Scan(&w.ID, &w.Name, &w.Slug, &w.CreatedBy, &w.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, workspace.ErrWorkspaceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: find workspace: %w", err)
	}
	return &w, nil
}

func (r *WorkspaceRepository) ListByMember(ctx context.Context, userID string) ([]*workspace.Workspace, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT w.id, w.name, w.slug, w.created_by, w.created_at
		 FROM workspaces w JOIN workspace_members m ON m.workspace_id = w.id
		 WHERE m.user_id=$1 AND m.status='active' ORDER BY w.created_at`, userID)
	if err != nil {
		return nil, fmt.Errorf("postgres: list workspaces: %w", err)
	}
	defer rows.Close()
	var out []*workspace.Workspace
	for rows.Next() {
		var w workspace.Workspace
		if err := rows.Scan(&w.ID, &w.Name, &w.Slug, &w.CreatedBy, &w.CreatedAt); err != nil {
			return nil, fmt.Errorf("postgres: scan workspace: %w", err)
		}
		out = append(out, &w)
	}
	return out, rows.Err()
}

func (r *WorkspaceRepository) SlugExists(ctx context.Context, slug string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM workspaces WHERE slug=$1)`, slug).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("postgres: slug exists: %w", err)
	}
	return exists, nil
}

type MemberRepository struct{ pool *pgxpool.Pool }

func NewMemberRepository(pool *pgxpool.Pool) *MemberRepository { return &MemberRepository{pool} }

func (r *MemberRepository) Create(ctx context.Context, m *workspace.Member) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO workspace_members (id, workspace_id, user_id, role, status, created_at, org_unit_id, position_id) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		m.ID, m.WorkspaceID, m.UserID, m.Role, m.Status, m.CreatedAt, m.OrgUnitID, m.PositionID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return workspace.ErrAlreadyMember
		}
		return fmt.Errorf("postgres: create member: %w", err)
	}
	return nil
}

func (r *MemberRepository) Find(ctx context.Context, workspaceID, userID string) (*workspace.Member, error) {
	var m workspace.Member
	err := r.pool.QueryRow(ctx, `SELECT id, workspace_id, user_id, role, status, created_at, org_unit_id, position_id FROM workspace_members WHERE workspace_id=$1 AND user_id=$2`, workspaceID, userID).
		Scan(&m.ID, &m.WorkspaceID, &m.UserID, &m.Role, &m.Status, &m.CreatedAt, &m.OrgUnitID, &m.PositionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, workspace.ErrNotMember
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: find member: %w", err)
	}
	return &m, nil
}

func (r *MemberRepository) ListByWorkspace(ctx context.Context, workspaceID string) ([]*workspace.Member, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, workspace_id, user_id, role, status, created_at, org_unit_id, position_id FROM workspace_members WHERE workspace_id=$1 ORDER BY created_at`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("postgres: list members: %w", err)
	}
	defer rows.Close()
	var out []*workspace.Member
	for rows.Next() {
		var m workspace.Member
		if err := rows.Scan(&m.ID, &m.WorkspaceID, &m.UserID, &m.Role, &m.Status, &m.CreatedAt, &m.OrgUnitID, &m.PositionID); err != nil {
			return nil, fmt.Errorf("postgres: scan member: %w", err)
		}
		out = append(out, &m)
	}
	return out, rows.Err()
}

func (r *MemberRepository) Assign(ctx context.Context, workspaceID, userID string, orgUnitID, positionID *string) error {
	_, err := r.pool.Exec(ctx, `UPDATE workspace_members SET org_unit_id=$3, position_id=$4 WHERE workspace_id=$1 AND user_id=$2`,
		workspaceID, userID, orgUnitID, positionID)
	if err != nil {
		return fmt.Errorf("postgres: assign member: %w", err)
	}
	return nil
}

type InviteRepository struct{ pool *pgxpool.Pool }

func NewInviteRepository(pool *pgxpool.Pool) *InviteRepository { return &InviteRepository{pool} }

func (r *InviteRepository) Create(ctx context.Context, inv *workspace.Invite) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO workspace_invites (id, workspace_id, email, role, token, expires_at) VALUES ($1,$2,$3,$4,$5,$6)`,
		inv.ID, inv.WorkspaceID, inv.Email, inv.Role, inv.Token, inv.ExpiresAt)
	if err != nil {
		return fmt.Errorf("postgres: create invite: %w", err)
	}
	return nil
}

func (r *InviteRepository) FindByToken(ctx context.Context, token string) (*workspace.Invite, error) {
	var inv workspace.Invite
	err := r.pool.QueryRow(ctx, `SELECT id, workspace_id, email, role, token, expires_at, accepted_at FROM workspace_invites WHERE token=$1 AND accepted_at IS NULL AND expires_at > now()`, token).
		Scan(&inv.ID, &inv.WorkspaceID, &inv.Email, &inv.Role, &inv.Token, &inv.ExpiresAt, &inv.AcceptedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, workspace.ErrInviteNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: find invite: %w", err)
	}
	return &inv, nil
}

func (r *InviteRepository) MarkAccepted(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `UPDATE workspace_invites SET accepted_at=now() WHERE id=$1`, id)
	if err != nil {
		return fmt.Errorf("postgres: mark invite accepted: %w", err)
	}
	return nil
}

type ProductRepository struct{ pool *pgxpool.Pool }

func NewProductRepository(pool *pgxpool.Pool) *ProductRepository { return &ProductRepository{pool} }

func (r *ProductRepository) ListAll(ctx context.Context) ([]*workspace.Product, error) {
	rows, err := r.pool.Query(ctx, `SELECT key, name FROM products ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("postgres: list products: %w", err)
	}
	defer rows.Close()
	var out []*workspace.Product
	for rows.Next() {
		var p workspace.Product
		if err := rows.Scan(&p.Key, &p.Name); err != nil {
			return nil, fmt.Errorf("postgres: scan product: %w", err)
		}
		out = append(out, &p)
	}
	return out, rows.Err()
}

func (r *ProductRepository) Exists(ctx context.Context, key string) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM products WHERE key=$1)`, key).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("postgres: product exists: %w", err)
	}
	return ok, nil
}

type WorkspaceProductRepository struct{ pool *pgxpool.Pool }

func NewWorkspaceProductRepository(pool *pgxpool.Pool) *WorkspaceProductRepository {
	return &WorkspaceProductRepository{pool}
}

func (r *WorkspaceProductRepository) Enable(ctx context.Context, workspaceID, productKey string) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO workspace_products (workspace_id, product_key) VALUES ($1,$2) ON CONFLICT DO NOTHING`, workspaceID, productKey)
	if err != nil {
		return fmt.Errorf("postgres: enable product: %w", err)
	}
	return nil
}

func (r *WorkspaceProductRepository) Disable(ctx context.Context, workspaceID, productKey string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM workspace_products WHERE workspace_id=$1 AND product_key=$2`, workspaceID, productKey)
	if err != nil {
		return fmt.Errorf("postgres: disable product: %w", err)
	}
	return nil
}

func (r *WorkspaceProductRepository) ListEnabled(ctx context.Context, workspaceID string) ([]*workspace.Product, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT p.key, p.name FROM products p JOIN workspace_products wp ON wp.product_key = p.key
		 WHERE wp.workspace_id=$1 ORDER BY p.name`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("postgres: list enabled products: %w", err)
	}
	defer rows.Close()
	var out []*workspace.Product
	for rows.Next() {
		var p workspace.Product
		if err := rows.Scan(&p.Key, &p.Name); err != nil {
			return nil, fmt.Errorf("postgres: scan enabled product: %w", err)
		}
		out = append(out, &p)
	}
	return out, rows.Err()
}

type OrgUnitRepository struct{ pool *pgxpool.Pool }

func NewOrgUnitRepository(pool *pgxpool.Pool) *OrgUnitRepository { return &OrgUnitRepository{pool} }

func (r *OrgUnitRepository) Create(ctx context.Context, u *workspace.OrgUnit) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO org_units (id, workspace_id, parent_id, name, sort_order, created_at) VALUES ($1,$2,$3,$4,$5,$6)`,
		u.ID, u.WorkspaceID, u.ParentID, u.Name, u.SortOrder, u.CreatedAt)
	if err != nil {
		return fmt.Errorf("postgres: create org unit: %w", err)
	}
	return nil
}

func (r *OrgUnitRepository) ListByWorkspace(ctx context.Context, workspaceID string) ([]*workspace.OrgUnit, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, workspace_id, parent_id, name, sort_order, created_at FROM org_units WHERE workspace_id=$1 ORDER BY sort_order, name`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("postgres: list org units: %w", err)
	}
	defer rows.Close()
	var out []*workspace.OrgUnit
	for rows.Next() {
		var u workspace.OrgUnit
		if err := rows.Scan(&u.ID, &u.WorkspaceID, &u.ParentID, &u.Name, &u.SortOrder, &u.CreatedAt); err != nil {
			return nil, fmt.Errorf("postgres: scan org unit: %w", err)
		}
		out = append(out, &u)
	}
	return out, rows.Err()
}

func (r *OrgUnitRepository) Exists(ctx context.Context, workspaceID, id string) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM org_units WHERE workspace_id=$1 AND id=$2)`, workspaceID, id).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("postgres: org unit exists: %w", err)
	}
	return ok, nil
}

type PositionRepository struct{ pool *pgxpool.Pool }

func NewPositionRepository(pool *pgxpool.Pool) *PositionRepository { return &PositionRepository{pool} }

func (r *PositionRepository) Create(ctx context.Context, p *workspace.Position) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO positions (id, workspace_id, title, created_at) VALUES ($1,$2,$3,$4)`,
		p.ID, p.WorkspaceID, p.Title, p.CreatedAt)
	if err != nil {
		return fmt.Errorf("postgres: create position: %w", err)
	}
	return nil
}

func (r *PositionRepository) ListByWorkspace(ctx context.Context, workspaceID string) ([]*workspace.Position, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, workspace_id, title, created_at FROM positions WHERE workspace_id=$1 ORDER BY title`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("postgres: list positions: %w", err)
	}
	defer rows.Close()
	var out []*workspace.Position
	for rows.Next() {
		var p workspace.Position
		if err := rows.Scan(&p.ID, &p.WorkspaceID, &p.Title, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("postgres: scan position: %w", err)
		}
		out = append(out, &p)
	}
	return out, rows.Err()
}

func (r *PositionRepository) Exists(ctx context.Context, workspaceID, id string) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM positions WHERE workspace_id=$1 AND id=$2)`, workspaceID, id).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("postgres: position exists: %w", err)
	}
	return ok, nil
}
