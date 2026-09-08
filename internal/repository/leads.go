package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

type Lead struct {
	ID              string `json:"id"`
	BranchID        string `json:"branch_id"`
	FullName        string `json:"full_name"`
	Phone           string `json:"phone"`
	LevelTestResult string `json:"level_test_result"`
	Subject         string `json:"subject,omitempty"` // "english" | "chinese", set by the placement test
	Stage           string `json:"stage"`
}

type LeadRepo struct{ store *Store }

func NewLeadRepo(s *Store) *LeadRepo { return &LeadRepo{store: s} }

// GetByID fetches one lead. branchID "" (network-owner director /
// super_admin) matches any branch.
func (r *LeadRepo) GetByID(ctx context.Context, leadID, branchID string) (*Lead, error) {
	var l Lead
	err := r.store.Pool.QueryRow(ctx, `
		SELECT id, branch_id, full_name, phone, COALESCE(level_test_result, ''), COALESCE(subject::text, ''), stage
		FROM leads WHERE id = $1 AND ($2 = '' OR branch_id = $2::uuid)
	`, leadID, branchID).Scan(&l.ID, &l.BranchID, &l.FullName, &l.Phone, &l.LevelTestResult, &l.Subject, &l.Stage)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &l, nil
}

// Insert creates a new lead in the "new" stage — called from the public
// dosedu.kz landing page form. subject may be "" (general inquiry, not
// tied to a placement-test result).
func (r *LeadRepo) Insert(ctx context.Context, branchID, fullName, phone, levelTestResult, subject string) (string, error) {
	var subj any
	if subject != "" {
		subj = subject
	}
	var id string
	err := r.store.Pool.QueryRow(ctx, `
		INSERT INTO leads (branch_id, full_name, phone, level_test_result, subject, stage)
		VALUES (NULLIF($1, '')::uuid, $2, $3, $4, $5, 'new')
		RETURNING id
	`, branchID, fullName, phone, levelTestResult, subj).Scan(&id)
	return id, err
}

// ListByBranchGroupedByStage returns every lead for a branch, grouped by
// Kanban column, for the director's CRM board. branchID "" returns
// every branch's leads (network-owner director / super_admin).
func (r *LeadRepo) ListByBranchGroupedByStage(ctx context.Context, branchID string) (map[string][]Lead, error) {
	rows, err := r.store.Pool.Query(ctx, `
		SELECT id, branch_id, full_name, phone, COALESCE(level_test_result, ''), COALESCE(subject::text, ''), stage
		FROM leads
		WHERE ($1 = '' OR branch_id = $1::uuid)
		ORDER BY created_at DESC
	`, branchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns := map[string][]Lead{
		"new": {}, "contacted": {}, "trial_scheduled": {}, "paid": {}, "lost": {},
	}
	for rows.Next() {
		var l Lead
		if err := rows.Scan(&l.ID, &l.BranchID, &l.FullName, &l.Phone, &l.LevelTestResult, &l.Subject, &l.Stage); err != nil {
			return nil, err
		}
		columns[l.Stage] = append(columns[l.Stage], l)
	}
	return columns, rows.Err()
}

// UpdateStage moves a lead to a new Kanban column, scoped to the
// director's own branch so one branch can't edit another's leads.
// branchID "" (network-owner director / super_admin) allows any branch.
func (r *LeadRepo) UpdateStage(ctx context.Context, leadID, branchID, newStage string) error {
	tag, err := r.store.Pool.Exec(ctx, `
		UPDATE leads SET stage = $1, updated_at = now()
		WHERE id = $2 AND ($3 = '' OR branch_id = $3::uuid)
	`, newStage, leadID, branchID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
