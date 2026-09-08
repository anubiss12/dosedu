package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/dosedu/lms/internal/auth"
)

// ImpersonationTarget is what's needed to mint a valid session as this
// user — mirrors Credential but looked up by ID (no password check).
type ImpersonationTarget struct {
	UserID         string
	BranchID       string
	Subject        *string
	IsNetworkOwner bool
}

type ImpersonationRepo struct{ store *Store }

func NewImpersonationRepo(s *Store) *ImpersonationRepo { return &ImpersonationRepo{store: s} }

// FindTarget looks up the branch/subject/network-owner context needed
// to issue a session as the given user. super_admin is deliberately
// not impersonable.
func (r *ImpersonationRepo) FindTarget(ctx context.Context, role auth.Role, id string) (*ImpersonationTarget, error) {
	var query string
	switch role {
	case auth.RoleDirector:
		query = `SELECT id, branch_id, NULL::text, is_network_owner FROM directors WHERE id = $1 AND is_active`
	case auth.RoleTeacher:
		query = `SELECT id, branch_id, subject::text, false FROM teachers WHERE id = $1 AND is_active`
	case auth.RoleParent:
		query = `SELECT id, '', NULL::text, false FROM parents WHERE id = $1`
	case auth.RoleStudent:
		query = `SELECT id, branch_id, NULL::text, false FROM students WHERE id = $1`
	default:
		return nil, errors.New("this role cannot be impersonated")
	}

	var t ImpersonationTarget
	err := r.store.Pool.QueryRow(ctx, query, id).Scan(&t.UserID, &t.BranchID, &t.Subject, &t.IsNetworkOwner)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// LogPrivate records that a super_admin impersonated someone — this
// table is never read by any endpoint other than the super-admin's own
// audit view (not system_logs, not visible to the impersonated user or
// any director/teacher/parent).
func (r *ImpersonationRepo) LogPrivate(ctx context.Context, superAdminID string, targetRole auth.Role, targetID string) error {
	_, err := r.store.Pool.Exec(ctx, `
		INSERT INTO impersonation_log (super_admin_id, target_role, target_id) VALUES ($1, $2, $3)
	`, superAdminID, string(targetRole), targetID)
	return err
}

// ListRecent returns the super-admin's own audit trail of past
// impersonations — for their own accountability, never exposed
// elsewhere.
func (r *ImpersonationRepo) ListRecent(ctx context.Context, limit int) ([]ImpersonationLogEntry, error) {
	rows, err := r.store.Pool.Query(ctx, `
		SELECT id, super_admin_id, target_role, target_id, created_at::text
		FROM impersonation_log ORDER BY created_at DESC LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ImpersonationLogEntry
	for rows.Next() {
		var e ImpersonationLogEntry
		if err := rows.Scan(&e.ID, &e.SuperAdminID, &e.TargetRole, &e.TargetID, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

type ImpersonationLogEntry struct {
	ID           string `json:"id"`
	SuperAdminID string `json:"super_admin_id"`
	TargetRole   string `json:"target_role"`
	TargetID     string `json:"target_id"`
	CreatedAt    string `json:"created_at"`
}
