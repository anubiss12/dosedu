package repository

import (
	"context"
	"time"
)

type Certificate struct {
	ID          string    `json:"id"`
	StudentID   string    `json:"student_id"`
	StudentName string    `json:"student_name"`
	Level       string    `json:"level"`
	QRToken     string    `json:"qr_token"`
	IssuedAt    time.Time `json:"issued_at"`
}

type CertificateRepo struct{ store *Store }

func NewCertificateRepo(s *Store) *CertificateRepo { return &CertificateRepo{store: s} }

// Issue creates a certificate row for a student who completed a level,
// auto-generating a unique qr_token (default in the schema) for the
// public verification link encoded into the certificate's QR code.
func (r *CertificateRepo) Issue(ctx context.Context, branchID, studentID, level, issuedByTeacherID string) (id, qrToken string, err error) {
	err = r.store.Pool.QueryRow(ctx, `
		INSERT INTO certificates (student_id, branch_id, level, issued_by)
		VALUES ($1, $2, $3, $4)
		RETURNING id, qr_token
	`, studentID, branchID, level, issuedByTeacherID).Scan(&id, &qrToken)
	return id, qrToken, err
}

// GetByID fetches one certificate (with student name) for the
// authenticated download route, scoped to the student's own record.
func (r *CertificateRepo) GetByID(ctx context.Context, id string) (*Certificate, error) {
	var c Certificate
	err := r.store.Pool.QueryRow(ctx, `
		SELECT c.id, c.student_id, s.full_name, c.level, c.qr_token, c.issued_at
		FROM certificates c
		JOIN students s ON s.id = c.student_id
		WHERE c.id = $1
	`, id).Scan(&c.ID, &c.StudentID, &c.StudentName, &c.Level, &c.QRToken, &c.IssuedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// GetByQRToken powers the public "scan to verify" endpoint — anyone
// with the QR token (from a physical/PDF certificate) can confirm it's
// genuine, without needing to log in.
func (r *CertificateRepo) GetByQRToken(ctx context.Context, token string) (*Certificate, error) {
	var c Certificate
	err := r.store.Pool.QueryRow(ctx, `
		SELECT c.id, c.student_id, s.full_name, c.level, c.qr_token, c.issued_at
		FROM certificates c
		JOIN students s ON s.id = c.student_id
		WHERE c.qr_token = $1
	`, token).Scan(&c.ID, &c.StudentID, &c.StudentName, &c.Level, &c.QRToken, &c.IssuedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// ListByStudent returns every certificate a student has earned, for
// their "portfolio" view.
func (r *CertificateRepo) ListByStudent(ctx context.Context, studentID string) ([]Certificate, error) {
	rows, err := r.store.Pool.Query(ctx, `
		SELECT c.id, c.student_id, s.full_name, c.level, c.qr_token, c.issued_at
		FROM certificates c
		JOIN students s ON s.id = c.student_id
		WHERE c.student_id = $1
		ORDER BY c.issued_at DESC
	`, studentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Certificate
	for rows.Next() {
		var c Certificate
		if err := rows.Scan(&c.ID, &c.StudentID, &c.StudentName, &c.Level, &c.QRToken, &c.IssuedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
