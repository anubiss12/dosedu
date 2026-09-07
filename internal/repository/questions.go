package repository

import "context"

// Question is what a student sees before answering — no correct_option,
// so the answer key never reaches the client.
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
// a file passes row validation.
func (r *QuestionRepo) BulkInsert(ctx context.Context, branchID, teacherID, language, level string, isOfficial bool, questions []NewQuestion) error {
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
			INSERT INTO test_questions (branch_id, created_by, language, level, is_official, question, option_a, option_b, option_c, option_d, correct_option)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		`, branchID, teacherID, language, level, isOfficial, q.Question, q.OptionA, q.OptionB, optC, optD, q.CorrectOption); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// ListForStudent returns one language+level question set (practice or
// official) for the student to answer.
func (r *QuestionRepo) ListForStudent(ctx context.Context, language, level string, official bool) ([]Question, error) {
	rows, err := r.store.Pool.Query(ctx, `
		SELECT id, question, option_a, option_b, COALESCE(option_c, ''), COALESCE(option_d, '')
		FROM test_questions
		WHERE language = $1 AND level = $2 AND is_official = $3
		ORDER BY created_at
	`, language, level, official)
	if err != nil {
		return nil, err
	}
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
		FROM test_questions
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
