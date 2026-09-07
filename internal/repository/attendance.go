package repository

import (
	"context"
	"time"
)

type AttendanceRepo struct{ store *Store }

func NewAttendanceRepo(s *Store) *AttendanceRepo { return &AttendanceRepo{store: s} }

// MarkQRCheckIn records a QR scan at the door. If today's attendance row
// for this student doesn't exist yet it's created with status "present";
// otherwise just the QR timestamp is stamped. markedBy is the scanning
// teacher's ID (or a service account ID for an unattended kiosk).
func (r *AttendanceRepo) MarkQRCheckIn(ctx context.Context, studentID, markedBy string) (time.Time, error) {
	now := time.Now()
	_, err := r.store.Pool.Exec(ctx, `
		INSERT INTO attendance (student_id, lesson_date, status, marked_by, qr_checked_in_at)
		VALUES ($1, CURRENT_DATE, 'present', $2, $3)
		ON CONFLICT (student_id, lesson_date) DO NOTHING
	`, studentID, markedBy, now)
	if err != nil {
		return time.Time{}, err
	}

	// If a row already existed for today (e.g. teacher had already
	// marked attendance manually), stamp the QR time on it instead.
	_, err = r.store.Pool.Exec(ctx, `
		UPDATE attendance
		SET qr_checked_in_at = $1
		WHERE student_id = $2 AND lesson_date = CURRENT_DATE AND qr_checked_in_at IS NULL
	`, now, studentID)
	if err != nil {
		return time.Time{}, err
	}

	return now, nil
}

// Mark records (or updates) one student's attendance status and
// optional grade for a given lesson date — the teacher gradebook flow,
// as opposed to MarkQRCheckIn's door-scan flow. Upserts on the
// (student_id, lesson_date) unique index, so re-marking the same day
// just corrects the earlier entry instead of erroring.
func (r *AttendanceRepo) Mark(ctx context.Context, studentID, status string, grade *float64, markedBy string, lessonDate time.Time) error {
	_, err := r.store.Pool.Exec(ctx, `
		INSERT INTO attendance (student_id, lesson_date, status, grade, marked_by)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (student_id, lesson_date)
		DO UPDATE SET status = EXCLUDED.status, grade = EXCLUDED.grade, marked_by = EXCLUDED.marked_by
	`, studentID, lessonDate, status, grade, markedBy)
	return err
}

// ListByGroupAndDate returns every student's attendance/grade row for
// one lesson date, joined with each student's name — the teacher
// gradebook view for a specific day.
func (r *AttendanceRepo) ListByGroupAndDate(ctx context.Context, groupID string, lessonDate time.Time) ([]GradebookRow, error) {
	rows, err := r.store.Pool.Query(ctx, `
		SELECT s.id, s.full_name,
		       a.status, a.grade
		FROM group_students gs
		JOIN students s ON s.id = gs.student_id
		LEFT JOIN attendance a ON a.student_id = s.id AND a.lesson_date = $2
		WHERE gs.group_id = $1
		ORDER BY s.full_name
	`, groupID, lessonDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []GradebookRow
	for rows.Next() {
		var row GradebookRow
		var status *string
		var grade *float64
		if err := rows.Scan(&row.StudentID, &row.FullName, &status, &grade); err != nil {
			return nil, err
		}
		if status != nil {
			row.Status = *status
		}
		row.Grade = grade
		out = append(out, row)
	}
	return out, rows.Err()
}

// GradebookRow is one student's attendance/grade entry for a lesson day.
type GradebookRow struct {
	StudentID string   `json:"student_id"`
	FullName  string   `json:"full_name"`
	Status    string   `json:"status"` // "" if not yet marked today
	Grade     *float64 `json:"grade"`
}
type MonthlySummary struct {
	PresentCount int
	AbsentCount  int
	ExcusedCount int
	AverageGrade float64
}

// GetMonthlySummary aggregates one student's attendance record and
// average grade over [periodStart, periodEnd] for the "Ай соңында ...
// PDF" one-tap report.
func (r *AttendanceRepo) GetMonthlySummary(ctx context.Context, studentID string, periodStart, periodEnd time.Time) (*MonthlySummary, error) {
	var s MonthlySummary
	var avgGrade *float64
	err := r.store.Pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE status = 'present'),
			COUNT(*) FILTER (WHERE status = 'absent'),
			COUNT(*) FILTER (WHERE status = 'excused'),
			AVG(grade) FILTER (WHERE grade IS NOT NULL)
		FROM attendance
		WHERE student_id = $1 AND lesson_date BETWEEN $2 AND $3
	`, studentID, periodStart, periodEnd).Scan(&s.PresentCount, &s.AbsentCount, &s.ExcusedCount, &avgGrade)
	if err != nil {
		return nil, err
	}
	if avgGrade != nil {
		s.AverageGrade = *avgGrade
	}
	return &s, nil
}
