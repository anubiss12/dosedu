package repository

import (
	"context"
	"time"
)

type PracticeAttempt struct {
	ID       string    `json:"id"`
	Language string    `json:"language"`
	Score    int       `json:"score"`
	Total    int       `json:"total"`
	TakenAt  time.Time `json:"taken_at"`
}

type PracticeRepo struct{ store *Store }

func NewPracticeRepo(s *Store) *PracticeRepo { return &PracticeRepo{store: s} }

// RecordAttempt saves one completed practice-test run — called by the
// student's frontend right after a quiz finishes, so the "Нәтижелер"
// tab has real history instead of being frontend-only/ephemeral.
func (r *PracticeRepo) RecordAttempt(ctx context.Context, studentID, language string, score, total int) (string, error) {
	var id string
	err := r.store.Pool.QueryRow(ctx, `
		INSERT INTO practice_attempts (student_id, language, score, total)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, studentID, language, score, total).Scan(&id)
	return id, err
}

// ListByStudent returns a student's attempt history, most recent first.
func (r *PracticeRepo) ListByStudent(ctx context.Context, studentID string, limit int) ([]PracticeAttempt, error) {
	rows, err := r.store.Pool.Query(ctx, `
		SELECT id, language, score, total, taken_at
		FROM practice_attempts
		WHERE student_id = $1
		ORDER BY taken_at DESC
		LIMIT $2
	`, studentID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PracticeAttempt
	for rows.Next() {
		var a PracticeAttempt
		if err := rows.Scan(&a.ID, &a.Language, &a.Score, &a.Total, &a.TakenAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
