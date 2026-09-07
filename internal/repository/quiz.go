package repository

import (
	"context"
	"time"
)

type QuizAttempt struct {
	ID       string    `json:"id"`
	Language string    `json:"language"`
	Kind     string    `json:"kind"`
	Score    int       `json:"score"`
	Total    int       `json:"total"`
	TakenAt  time.Time `json:"taken_at"`
}

type QuizRepo struct{ store *Store }

func NewQuizRepo(s *Store) *QuizRepo { return &QuizRepo{store: s} }

func (r *QuizRepo) RecordAttempt(ctx context.Context, studentID, language, kind string, score, total int) (string, error) {
	var id string
	err := r.store.Pool.QueryRow(ctx, `
		INSERT INTO quiz_attempts (student_id, language, kind, score, total)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, studentID, language, kind, score, total).Scan(&id)
	return id, err
}

// HasOfficialAttempt reports whether the student has already taken the
// one-time official level-check test for this language — the official
// test allows exactly one attempt, unlike unlimited practice.
func (r *QuizRepo) HasOfficialAttempt(ctx context.Context, studentID, language string) (bool, error) {
	var exists bool
	err := r.store.Pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM quiz_attempts
			WHERE student_id = $1 AND language = $2 AND kind = 'official'
		)
	`, studentID, language).Scan(&exists)
	return exists, err
}

func (r *QuizRepo) ListByStudent(ctx context.Context, studentID string, limit int) ([]QuizAttempt, error) {
	rows, err := r.store.Pool.Query(ctx, `
		SELECT id, language, kind, score, total, taken_at
		FROM quiz_attempts WHERE student_id = $1
		ORDER BY taken_at DESC LIMIT $2
	`, studentID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []QuizAttempt
	for rows.Next() {
		var a QuizAttempt
		if err := rows.Scan(&a.ID, &a.Language, &a.Kind, &a.Score, &a.Total, &a.TakenAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
