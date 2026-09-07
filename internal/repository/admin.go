package repository

import (
	"context"
	"time"
)

// ============================================================
// Branches
// ============================================================

type Branch struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Address   string    `json:"address"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type BranchRepo struct{ store *Store }

func NewBranchRepo(s *Store) *BranchRepo { return &BranchRepo{store: s} }

func (r *BranchRepo) Create(ctx context.Context, name, address string) (string, error) {
	var id string
	err := r.store.Pool.QueryRow(ctx, `
		INSERT INTO branches (name, address, status)
		VALUES ($1, $2, 'active')
		RETURNING id
	`, name, address).Scan(&id)
	return id, err
}

func (r *BranchRepo) ListAll(ctx context.Context) ([]Branch, error) {
	rows, err := r.store.Pool.Query(ctx, `
		SELECT id, name, COALESCE(address, ''), status, created_at
		FROM branches ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Branch
	for rows.Next() {
		var b Branch
		if err := rows.Scan(&b.ID, &b.Name, &b.Address, &b.Status, &b.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *BranchRepo) SetStatus(ctx context.Context, branchID, status string) error {
	tag, err := r.store.Pool.Exec(ctx, `UPDATE branches SET status = $1 WHERE id = $2`, status, branchID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ============================================================
// Directors (super-admin's account-creation view — distinct from the
// director-facing routes, which never need to list/create other
// directors)
// ============================================================

type DirectorAccount struct {
	ID         string    `json:"id"`
	BranchID   string    `json:"branch_id"`
	BranchName string    `json:"branch_name"`
	Email      string    `json:"email"`
	FullName   string    `json:"full_name"`
	IsActive   bool      `json:"is_active"`
	CreatedAt  time.Time `json:"created_at"`
}

type AdminDirectorRepo struct{ store *Store }

func NewAdminDirectorRepo(s *Store) *AdminDirectorRepo { return &AdminDirectorRepo{store: s} }

// Create inserts a new director account. passwordHash is generated and
// hashed by the caller (handler), same pattern as CreateFromLead — the
// repository layer never handles plaintext credentials.
func (r *AdminDirectorRepo) Create(ctx context.Context, branchID, email, passwordHash, fullName string) (string, error) {
	var id string
	err := r.store.Pool.QueryRow(ctx, `
		INSERT INTO directors (branch_id, email, password_hash, full_name)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, branchID, email, passwordHash, fullName).Scan(&id)
	return id, err
}

func (r *AdminDirectorRepo) ListAll(ctx context.Context) ([]DirectorAccount, error) {
	rows, err := r.store.Pool.Query(ctx, `
		SELECT d.id, d.branch_id, b.name, d.email, COALESCE(d.full_name, ''), d.is_active, d.created_at
		FROM directors d
		JOIN branches b ON b.id = d.branch_id
		ORDER BY d.created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DirectorAccount
	for rows.Next() {
		var d DirectorAccount
		if err := rows.Scan(&d.ID, &d.BranchID, &d.BranchName, &d.Email, &d.FullName, &d.IsActive, &d.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// ============================================================
// System logs
// ============================================================

type SystemLog struct {
	ID        int64     `json:"id"`
	Level     string    `json:"level"`
	Source    string    `json:"source"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

type SystemLogRepo struct{ store *Store }

func NewSystemLogRepo(s *Store) *SystemLogRepo { return &SystemLogRepo{store: s} }

// Insert records a system/security event. Called from handlers on
// meaningful actions (branch/director creation, failed logins), so the
// Super Admin's log view reflects real activity, not a placeholder.
func (r *SystemLogRepo) Insert(ctx context.Context, level, source, message string) error {
	_, err := r.store.Pool.Exec(ctx, `
		INSERT INTO system_logs (level, source, message) VALUES ($1, $2, $3)
	`, level, source, message)
	return err
}

func (r *SystemLogRepo) ListRecent(ctx context.Context, limit int) ([]SystemLog, error) {
	rows, err := r.store.Pool.Query(ctx, `
		SELECT id, level, source, message, created_at
		FROM system_logs ORDER BY created_at DESC LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SystemLog
	for rows.Next() {
		var l SystemLog
		if err := rows.Scan(&l.ID, &l.Level, &l.Source, &l.Message, &l.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}
