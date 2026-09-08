package repository

import "context"

// Question is what a student/anonymous test-taker sees before
// answering — no correct_option, so the answer key never reaches the
// client.
type Question struct {
	ID       string `json:"id"`
	Question string `json:"question"`
	OptionA  string `json:"option_a"`
	OptionB  string `json:"option_b"`
	OptionC  string `json:"option_c,omitempty"`
	OptionD  string `json:"option_d,omitempty"`
}

// QuestionFull additionally carries the correct answer — used
// server-side only, for grading a submitted attempt. Deliberately not
// embedding Question: an embedded field named "Question" would shadow
// the string field of the same name and make q.Question ambiguous.
type QuestionFull struct {
	ID            string
	Question      string
	OptionA       string
	OptionB       string
	OptionC       string
	OptionD       string
	CorrectOption int
}

type QuestionRepo struct{ store *Store }

func NewQuestionRepo(s *Store) *QuestionRepo { return &QuestionRepo{store: s} }

// NewQuestion is one parsed-and-validated row from a teacher's test
// upload, ready to insert.
type NewQuestion struct {
	Question      string
	OptionA       string
	OptionB       string
	OptionC       string
	OptionD       string
	CorrectOption int
}

// BulkInsert writes every question from one successful upload in a
// single transaction — called by the teacher test-upload handler once
// a file passes row validation. pool is "practice" or "official".
func (r *QuestionRepo) BulkInsert(ctx context.Context, branchID, teacherID, subject, level, pool string, questions []NewQuestion) error {
	if len(questions) == 0 {
		return nil
	}
	tx, err := r.store.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for _, q := range questions {
		var optC, optD any
		if q.OptionC != "" {
			optC = q.OptionC
		}
		if q.OptionD != "" {
			optD = q.OptionD
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO questions (branch_id, created_by, subject, level, pool, question, option_a, option_b, option_c, option_d, correct_option)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		`, branchID, teacherID, subject, level, pool, q.Question, q.OptionA, q.OptionB, optC, optD, q.CorrectOption); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// ListBank returns every question a teacher has for one subject+
// level+pool — used to pick which ones go into a new test assignment.
// Unlike ListPractice/ListPlacement this is NOT randomized and DOES
// include the answer key (teacher-facing, not student-facing).
func (r *QuestionRepo) ListBank(ctx context.Context, branchID, subject, level, pool string) ([]QuestionFull, error) {
	rows, err := r.store.Pool.Query(ctx, `
		SELECT id, question, option_a, option_b, COALESCE(option_c, ''), COALESCE(option_d, ''), correct_option
		FROM questions
		WHERE branch_id = $1 AND subject = $2 AND level = $3 AND pool = $4
		ORDER BY created_at
	`, branchID, subject, level, pool)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []QuestionFull
	for rows.Next() {
		var q QuestionFull
		if err := rows.Scan(&q.ID, &q.Question, &q.OptionA, &q.OptionB, &q.OptionC, &q.OptionD, &q.CorrectOption); err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

// ListPractice returns up to `limit` random questions from the
// student's own subject+level, drawn from the 'practice' pool —
// unlimited retakes, a fresh random set every time.
func (r *QuestionRepo) ListPractice(ctx context.Context, subject, level string, limit int) ([]Question, error) {
	return r.listRandom(ctx, `
		SELECT id, question, option_a, option_b, COALESCE(option_c, ''), COALESCE(option_d, '')
		FROM questions
		WHERE subject = $1 AND level = $2 AND pool = 'practice'
		ORDER BY random() LIMIT $3
	`, subject, level, limit)
}

// ListPlacement returns `limit` random questions spanning every level
// of `subject` (deliberately mixed — the point is to *find* the
// level), drawn from the same 'practice' pool. Public, unauthenticated.
func (r *QuestionRepo) ListPlacement(ctx context.Context, subject string, limit int) ([]Question, error) {
	return r.listRandomOneArg(ctx, `
		SELECT id, question, option_a, option_b, COALESCE(option_c, ''), COALESCE(option_d, '')
		FROM questions
		WHERE subject = $1 AND pool = 'practice'
		ORDER BY random() LIMIT $2
	`, subject, limit)
}

func (r *QuestionRepo) listRandom(ctx context.Context, query, subject, level string, limit int) ([]Question, error) {
	rows, err := r.store.Pool.Query(ctx, query, subject, level, limit)
	if err != nil {
		return nil, err
	}
	return scanQuestions(rows)
}

func (r *QuestionRepo) listRandomOneArg(ctx context.Context, query, subject string, limit int) ([]Question, error) {
	rows, err := r.store.Pool.Query(ctx, query, subject, limit)
	if err != nil {
		return nil, err
	}
	return scanQuestions(rows)
}

func scanQuestions(rows interface {
	Next() bool
	Scan(dest ...any) error
	Close()
	Err() error
}) ([]Question, error) {
	defer rows.Close()
	var out []Question
	for rows.Next() {
		var q Question
		if err := rows.Scan(&q.ID, &q.Question, &q.OptionA, &q.OptionB, &q.OptionC, &q.OptionD); err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

// GetByIDs fetches full rows (correct answer included) for grading a
// submitted attempt — never exposed directly to a client response.
func (r *QuestionRepo) GetByIDs(ctx context.Context, ids []string) ([]QuestionFull, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := r.store.Pool.Query(ctx, `
		SELECT id, question, option_a, option_b, COALESCE(option_c, ''), COALESCE(option_d, ''), correct_option
		FROM questions
		WHERE id = ANY($1)
	`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []QuestionFull
	for rows.Next() {
		var q QuestionFull
		if err := rows.Scan(&q.ID, &q.Question, &q.OptionA, &q.OptionB, &q.OptionC, &q.OptionD, &q.CorrectOption); err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

// --- Test results (unified placement/practice/official attempt log) ---

type TestResult struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Subject  string `json:"subject"`
	Level    string `json:"level,omitempty"`
	Score    int    `json:"score"`
	Total    int    `json:"total"`
	TakenAt  string `json:"taken_at"`
}

// RecordPlacement saves a public placement-test result, linked to the
// Lead it produced (kind='placement', student_id NULL).
func (r *QuestionRepo) RecordPlacement(ctx context.Context, leadID, subject string, score, total int) (string, error) {
	var id string
	err := r.store.Pool.QueryRow(ctx, `
		INSERT INTO test_results (kind, lead_id, subject, score, total)
		VALUES ('placement', $1, $2, $3, $4)
		RETURNING id
	`, leadID, subject, score, total).Scan(&id)
	return id, err
}

// RecordPractice saves an unlimited practice-test result for a student.
func (r *QuestionRepo) RecordPractice(ctx context.Context, studentID, subject, level string, score, total int) (string, error) {
	var id string
	err := r.store.Pool.QueryRow(ctx, `
		INSERT INTO test_results (kind, student_id, subject, level, score, total)
		VALUES ('practice', $1, $2, $3, $4, $5)
		RETURNING id
	`, studentID, subject, level, score, total).Scan(&id)
	return id, err
}

// ListPracticeHistory returns a student's practice-test history, most
// recent first.
func (r *QuestionRepo) ListPracticeHistory(ctx context.Context, studentID string, limit int) ([]TestResult, error) {
	rows, err := r.store.Pool.Query(ctx, `
		SELECT id, kind, subject::text, COALESCE(level::text, ''), score, total, taken_at::text
		FROM test_results
		WHERE student_id = $1 AND kind = 'practice'
		ORDER BY taken_at DESC LIMIT $2
	`, studentID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TestResult
	for rows.Next() {
		var t TestResult
		if err := rows.Scan(&t.ID, &t.Kind, &t.Subject, &t.Level, &t.Score, &t.Total, &t.TakenAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
