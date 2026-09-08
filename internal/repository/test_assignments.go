package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// ErrAlreadyAttempted is returned when a student tries a second
// attempt at an official test assignment — the DB's
// uq_official_one_attempt index is the actual source of truth; this
// just gives it a friendly Go name.
var ErrAlreadyAttempted = errors.New("official test already attempted")

type TestAssignment struct {
	ID               string `json:"id"`
	GroupID          string `json:"group_id"`
	Subject          string `json:"subject"`
	Level            string `json:"level"`
	AttemptsAllowed  int    `json:"attempts_allowed"`
	QuestionIDs      []string `json:"question_ids"`
	CreatedAt        string `json:"created_at"`
}

type TestAssignmentRepo struct{ store *Store }

func NewTestAssignmentRepo(s *Store) *TestAssignmentRepo { return &TestAssignmentRepo{store: s} }

// Create assigns a one-time official test to a group. questionIDs is
// the teacher's chosen/uploaded question set for this test.
func (r *TestAssignmentRepo) Create(ctx context.Context, branchID, groupID, teacherID, subject, level string, questionIDs []string) (string, error) {
	var id string
	err := r.store.Pool.QueryRow(ctx, `
		INSERT INTO test_assignments (branch_id, group_id, created_by, subject, level, question_ids)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`, branchID, groupID, teacherID, subject, level, questionIDs).Scan(&id)
	return id, err
}

// ListByGroup returns every official test ever assigned to a group.
func (r *TestAssignmentRepo) ListByGroup(ctx context.Context, groupID string) ([]TestAssignment, error) {
	rows, err := r.store.Pool.Query(ctx, `
		SELECT id, group_id, subject::text, level::text, attempts_allowed, question_ids, created_at::text
		FROM test_assignments WHERE group_id = $1 ORDER BY created_at DESC
	`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TestAssignment
	for rows.Next() {
		var a TestAssignment
		if err := rows.Scan(&a.ID, &a.GroupID, &a.Subject, &a.Level, &a.AttemptsAllowed, &a.QuestionIDs, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// GetByID fetches one assignment (for the student to answer, and for
// grading on submit).
func (r *TestAssignmentRepo) GetByID(ctx context.Context, id string) (*TestAssignment, error) {
	var a TestAssignment
	err := r.store.Pool.QueryRow(ctx, `
		SELECT id, group_id, subject::text, level::text, attempts_allowed, question_ids, created_at::text
		FROM test_assignments WHERE id = $1
	`, id).Scan(&a.ID, &a.GroupID, &a.Subject, &a.Level, &a.AttemptsAllowed, &a.QuestionIDs, &a.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// ListActiveForStudent returns official tests assigned to any group
// the student belongs to, that they have not yet attempted.
func (r *TestAssignmentRepo) ListActiveForStudent(ctx context.Context, studentID string) ([]TestAssignment, error) {
	rows, err := r.store.Pool.Query(ctx, `
		SELECT ta.id, ta.group_id, ta.subject::text, ta.level::text, ta.attempts_allowed, ta.question_ids, ta.created_at::text
		FROM test_assignments ta
		JOIN group_students gs ON gs.group_id = ta.group_id
		WHERE gs.student_id = $1
		  AND NOT EXISTS (
		    SELECT 1 FROM test_results tr
		    WHERE tr.test_assignment_id = ta.id AND tr.student_id = $1 AND tr.kind = 'official'
		  )
		ORDER BY ta.created_at DESC
	`, studentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TestAssignment
	for rows.Next() {
		var a TestAssignment
		if err := rows.Scan(&a.ID, &a.GroupID, &a.Subject, &a.Level, &a.AttemptsAllowed, &a.QuestionIDs, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// RecordOfficialAttempt records a student's official-test result. The
// DB's uq_official_one_attempt unique index is what actually enforces
// "one attempt only" — a second try hits a unique_violation, which
// this translates into ErrAlreadyAttempted.
func (r *TestAssignmentRepo) RecordOfficialAttempt(ctx context.Context, studentID, assignmentID, subject, level string, score, total int) (string, error) {
	var id string
	err := r.store.Pool.QueryRow(ctx, `
		INSERT INTO test_results (kind, student_id, test_assignment_id, subject, level, score, total)
		VALUES ('official', $1, $2, $3, $4, $5, $6)
		RETURNING id
	`, studentID, assignmentID, subject, level, score, total).Scan(&id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
			return "", ErrAlreadyAttempted
		}
		return "", err
	}
	return id, nil
}
