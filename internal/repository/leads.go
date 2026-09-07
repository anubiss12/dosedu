package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

type Lead struct {
	ID              string `json:"id"`
	FullName        string `json:"full_name"`
	Phone           string `json:"phone"`
	LevelTestResult string `json:"level_test_result"`
	Stage           string `json:"stage"`
}

type LeadRepo struct{ store *Store }

func NewLeadRepo(s *Store) *LeadRepo { return &LeadRepo{store: s} }

// GetByID fetches one lead, scoped to the director's branch.
func (r *LeadRepo) GetByID(ctx context.Context, leadID, branchID string) (*Lead, error) {
	var l Lead
	err := r.store.Pool.QueryRow(ctx, `
		SELECT id, full_name, phone, COALESCE(level_test_result, ''), stage
		FROM leads WHERE id = $1 AND branch_id = $2
	`, leadID, branchID).Scan(&l.ID, &l.FullName, &l.Phone, &l.LevelTestResult, &l.Stage)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &l, nil
}

// Insert creates a new lead in the "new" stage — called from the public
// dosedu.kz landing page form.
func (r *LeadRepo) Insert(ctx context.Context, branchID, fullName, phone, levelTestResult string) (string, error) {
	var id string
	err := r.store.Pool.QueryRow(ctx, `
		INSERT INTO leads (branch_id, full_name, phone, level_test_result, stage)
		VALUES (NULLIF($1, '')::uuid, $2, $3, $4, 'new')
		RETURNING id
	`, branchID, fullName, phone, levelTestResult).Scan(&id)
	return id, err
}

// ListByBranchGroupedByStage returns every lead for a branch, grouped by
// Kanban column, for the director's CRM board.
func (r *LeadRepo) ListByBranchGroupedByStage(ctx context.Context, branchID string) (map[string][]Lead, error) {
	rows, err := r.store.Pool.Query(ctx, `
		SELECT id, full_name, phone, COALESCE(level_test_result, ''), stage
		FROM leads
		WHERE branch_id = $1
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
		if err := rows.Scan(&l.ID, &l.FullName, &l.Phone, &l.LevelTestResult, &l.Stage); err != nil {
			return nil, err
		}
		columns[l.Stage] = append(columns[l.Stage], l)
	}
	return columns, rows.Err()
}

// UpdateStage moves a lead to a new Kanban column, scoped to the
// director's own branch so one branch can't edit another's leads.
func (r *LeadRepo) UpdateStage(ctx context.Context, leadID, branchID, newStage string) error {
	tag, err := r.store.Pool.Exec(ctx, `
		UPDATE leads SET stage = $1, updated_at = now()
		WHERE id = $2 AND branch_id = $3
	`, newStage, leadID, branchID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
