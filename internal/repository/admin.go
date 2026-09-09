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
		FROM branches WHERE deleted_at IS NULL ORDER BY created_at DESC
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

// PublicBranch is the minimal, non-sensitive shape shown on the public
// landing page's lead-form branch picker — no address/status/created_at.
type PublicBranch struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ListPublic returns every active branch's id+name for the
// unauthenticated landing page — deliberately narrower than ListAll,
// which is s-admin-only and includes operational fields.
func (r *BranchRepo) ListPublic(ctx context.Context) ([]PublicBranch, error) {
	rows, err := r.store.Pool.Query(ctx, `
		SELECT id, name FROM branches
		WHERE deleted_at IS NULL AND status = 'active'
		ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PublicBranch
	for rows.Next() {
		var b PublicBranch
		if err := rows.Scan(&b.ID, &b.Name); err != nil {
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

// SoftDelete removes a branch from every listing without deleting its
// row (or anything referencing it) — distinct from SetStatus, which is
// a temporary block/unblock toggle. History under this branch
// (students, groups, payments, ...) stays in place.
func (r *BranchRepo) SoftDelete(ctx context.Context, branchID string) error {
	tag, err := r.store.Pool.Exec(ctx, `UPDATE branches SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL`, branchID)
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
	ID             string    `json:"id"`
	BranchID       string    `json:"branch_id"`
	BranchName     string    `json:"branch_name"`
	Email          string    `json:"email"`
	FullName       string    `json:"full_name"`
	IsActive       bool      `json:"is_active"`
	IsNetworkOwner bool      `json:"is_network_owner"`
	CreatedAt      time.Time `json:"created_at"`
}

type AdminDirectorRepo struct{ store *Store }

func NewAdminDirectorRepo(s *Store) *AdminDirectorRepo { return &AdminDirectorRepo{store: s} }

// Create inserts a new director account. passwordHash is generated and
// hashed by the caller (handler), same pattern as CreateFromLead — the
// repository layer never handles plaintext credentials. isNetworkOwner
// grants network-wide access (every branch) via middleware.EffectiveBranchID
// rather than just branchID's "home" branch.
func (r *AdminDirectorRepo) Create(ctx context.Context, branchID, email, passwordHash, fullName string, isNetworkOwner bool) (string, error) {
	var id string
	err := r.store.Pool.QueryRow(ctx, `
		INSERT INTO directors (branch_id, email, password_hash, full_name, is_network_owner)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, branchID, email, passwordHash, fullName, isNetworkOwner).Scan(&id)
	return id, err
}

func (r *AdminDirectorRepo) ListAll(ctx context.Context) ([]DirectorAccount, error) {
	rows, err := r.store.Pool.Query(ctx, `
		SELECT d.id, d.branch_id, b.name, d.email, COALESCE(d.full_name, ''), d.is_active, d.is_network_owner, d.created_at
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
		if err := rows.Scan(&d.ID, &d.BranchID, &d.BranchName, &d.Email, &d.FullName, &d.IsActive, &d.IsNetworkOwner, &d.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// ResetPassword overwrites a director's password hash — the
// super-admin "force reset" flow. The caller (handler) generates and
// hashes the new temporary password, same division of responsibility
// as Create.
func (r *AdminDirectorRepo) ResetPassword(ctx context.Context, directorID, passwordHash string) error {
	tag, err := r.store.Pool.Exec(ctx, `UPDATE directors SET password_hash = $2 WHERE id = $1`, directorID, passwordHash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ============================================================
// Churn alerts
// ============================================================

// ChurnAlert flags a student at risk of dropping out — a language
// student with 3+ consecutive absences, or a care_and_prep child with
// 7+ consecutive absent days. Nothing writes to this table yet (the
// detection Cron job is a follow-up); the read side exists now so the
// API contract is stable once it does.
type ChurnAlert struct {
	ID           string    `json:"id"`
	BranchID     string    `json:"branch_id"`
	StudentID    string    `json:"student_id"`
	AlertType    string    `json:"alert_type"`
	StreakCount  int       `json:"streak_count"`
	Acknowledged bool      `json:"acknowledged"`
	CreatedAt    time.Time `json:"created_at"`
}

type ChurnAlertRepo struct{ store *Store }

func NewChurnAlertRepo(s *Store) *ChurnAlertRepo { return &ChurnAlertRepo{store: s} }

// ListByBranch returns unacknowledged churn alerts for a branch, or
// every branch's if branchID is "" (super_admin / network-owner
// director — see middleware.EffectiveBranchID).
func (r *ChurnAlertRepo) ListByBranch(ctx context.Context, branchID string) ([]ChurnAlert, error) {
	rows, err := r.store.Pool.Query(ctx, `
		SELECT id, branch_id, student_id, alert_type::text, streak_count, acknowledged, created_at
		FROM churn_alerts
		WHERE ($1 = '' OR branch_id = $1::uuid) AND NOT acknowledged
		ORDER BY created_at DESC
	`, branchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ChurnAlert
	for rows.Next() {
		var a ChurnAlert
		if err := rows.Scan(&a.ID, &a.BranchID, &a.StudentID, &a.AlertType, &a.StreakCount, &a.Acknowledged, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
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
