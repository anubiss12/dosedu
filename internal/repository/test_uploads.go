package repository

import (
	"context"
	"encoding/json"
	"time"
)

type TestUpload struct {
	ID            string          `json:"id"`
	Level         string          `json:"level"`
	FileName      string          `json:"file_name"`
	FileSizeBytes int64           `json:"file_size_bytes"`
	Status        string          `json:"status"`
	ErrorLog      json.RawMessage `json:"error_log,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
}

type TestUploadRepo struct{ store *Store }

func NewTestUploadRepo(s *Store) *TestUploadRepo { return &TestUploadRepo{store: s} }

// Insert records one upload attempt (metadata + per-row error log) —
// this is the "Error log validation" panel's persistence, previously a
// skeleton no-op.
func (r *TestUploadRepo) Insert(ctx context.Context, branchID, teacherID, level, fileName string, fileSizeBytes int64, status string, errorLog json.RawMessage) (string, error) {
	if errorLog == nil {
		errorLog = json.RawMessage("null")
	}
	var id string
	err := r.store.Pool.QueryRow(ctx, `
		INSERT INTO test_uploads (branch_id, teacher_id, level, file_name, file_size_bytes, status, error_log)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id
	`, branchID, teacherID, level, fileName, fileSizeBytes, status, errorLog).Scan(&id)
	return id, err
}

// ListByTeacher returns a teacher's recent uploads, most recent first —
// backs the "Error Log" panel on the test-upload page.
func (r *TestUploadRepo) ListByTeacher(ctx context.Context, teacherID string, limit int) ([]TestUpload, error) {
	rows, err := r.store.Pool.Query(ctx, `
		SELECT id, level, file_name, file_size_bytes, status, error_log, created_at
		FROM test_uploads
		WHERE teacher_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`, teacherID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TestUpload
	for rows.Next() {
		var u TestUpload
		if err := rows.Scan(&u.ID, &u.Level, &u.FileName, &u.FileSizeBytes, &u.Status, &u.ErrorLog, &u.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}
