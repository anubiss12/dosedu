package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/dosedu/lms/internal/auth"
)

var ErrNotFound = errors.New("credential not found")

// Credential is what the login handler needs to verify a password and
// issue a session: the stored hash, the user's UUID, their branch
// scope (empty for super_admin, which is global), and — for teachers
// only — their language track ("en"/"zh"/nil for mad/prodlenka).
type Credential struct {
	UserID        string
	PasswordHash  string
	BranchID      string
	LanguageScope *string
}

type CredentialRepo struct{ store *Store }

func NewCredentialRepo(s *Store) *CredentialRepo { return &CredentialRepo{store: s} }

// Ping does a trivial round-trip to confirm the DB is reachable, used by
// the super-admin System Health Dashboard.
func (r *CredentialRepo) Ping(ctx context.Context) error {
	return r.store.Pool.Ping(ctx)
}

// DBPoolStats is a snapshot of the pgx connection pool, shown on the
// super-admin System Health Dashboard.
type DBPoolStats struct {
	TotalConns    int32 `json:"total"`
	IdleConns     int32 `json:"idle"`
	AcquiredConns int32 `json:"acquired"`
}

func (r *CredentialRepo) PoolStats() DBPoolStats {
	stat := r.store.Pool.Stat()
	return DBPoolStats{
		TotalConns:    stat.TotalConns(),
		IdleConns:     stat.IdleConns(),
		AcquiredConns: stat.AcquiredConns(),
	}
}

// FindByRole looks up a login credential by role + identifier.
// Identifier meaning depends on role:
//
//	student    -> login_code
//	parent     -> phone
//	teacher    -> email
//	director   -> email
//	super_admin -> email
func (r *CredentialRepo) FindByRole(ctx context.Context, role auth.Role, identifier string) (*Credential, error) {
	var query string
	switch role {
	case auth.RoleStudent:
		query = `SELECT id, password_hash, branch_id, NULL::varchar FROM students WHERE login_code = $1`
	case auth.RoleParent:
		query = `SELECT id, password_hash, '', NULL::varchar FROM parents WHERE phone = $1`
	case auth.RoleTeacher:
		query = `SELECT id, password_hash, branch_id, language_scope FROM teachers WHERE email = $1 AND is_active`
	case auth.RoleDirector:
		query = `SELECT id, password_hash, branch_id, NULL::varchar FROM directors WHERE email = $1 AND is_active`
	case auth.RoleSuperAdmin:
		query = `SELECT id, password_hash, '', NULL::varchar FROM super_admins WHERE email = $1 AND is_active`
	default:
		return nil, errors.New("unknown role")
	}

	var cred Credential
	err := r.store.Pool.QueryRow(ctx, query, identifier).Scan(&cred.UserID, &cred.PasswordHash, &cred.BranchID, &cred.LanguageScope)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &cred, nil
}
