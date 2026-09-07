package repository

import (
	"context"
	"time"
)

type Payment struct {
	ID          string    `json:"id"`
	StudentID   string    `json:"student_id"`
	StudentName string    `json:"student_name"`
	Amount      float64   `json:"amount"`
	Status      string    `json:"status"`
	Method      string    `json:"method"`
	PeriodStart time.Time `json:"period_start"`
	PeriodEnd   time.Time `json:"period_end"`
	CreatedAt   time.Time `json:"created_at"`
}

type PaymentRepo struct{ store *Store }

func NewPaymentRepo(s *Store) *PaymentRepo { return &PaymentRepo{store: s} }

// ListByBranch returns every payment in a branch, most recent first —
// the director's "Төлемдер" view.
func (r *PaymentRepo) ListByBranch(ctx context.Context, branchID string) ([]Payment, error) {
	rows, err := r.store.Pool.Query(ctx, `
		SELECT p.id, p.student_id, s.full_name, p.amount, p.status, p.method,
		       COALESCE(p.period_start, p.created_at::date), COALESCE(p.period_end, p.created_at::date), p.created_at
		FROM payments p
		JOIN students s ON s.id = p.student_id
		WHERE p.branch_id = $1
		ORDER BY p.created_at DESC
	`, branchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Payment
	for rows.Next() {
		var p Payment
		if err := rows.Scan(&p.ID, &p.StudentID, &p.StudentName, &p.Amount, &p.Status, &p.Method, &p.PeriodStart, &p.PeriodEnd, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
