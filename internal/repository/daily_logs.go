package repository

import "context"

// DailyLog is one day's attendance + note + homework status for a
// CARE_AND_PREP (mad/prodlenka) child — what the parent's dashboard
// reads instead of any test/level data.
type DailyLog struct {
	ID              string `json:"id"`
	StudentID       string `json:"student_id"`
	LogDate         string `json:"log_date"`
	AttendanceStatus string `json:"attendance_status"`
	TeacherNote     string `json:"teacher_note,omitempty"`
	HomeworkStatus  string `json:"homework_status"`
	CreatedAt       string `json:"created_at"`
}

type DailyLogRepo struct{ store *Store }

func NewDailyLogRepo(s *Store) *DailyLogRepo { return &DailyLogRepo{store: s} }

// Upsert writes (or corrects) one day's log for one child — a teacher
// re-submitting the same student+date updates the earlier entry.
func (r *DailyLogRepo) Upsert(ctx context.Context, studentID, logDate, attendanceStatus, teacherNote, homeworkStatus, teacherID string) (string, error) {
	var note any
	if teacherNote != "" {
		note = teacherNote
	}
	var id string
	err := r.store.Pool.QueryRow(ctx, `
		INSERT INTO daily_logs (student_id, log_date, attendance_status, teacher_note, homework_status, created_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (student_id, log_date) DO UPDATE
		SET attendance_status = EXCLUDED.attendance_status,
		    teacher_note = EXCLUDED.teacher_note,
		    homework_status = EXCLUDED.homework_status,
		    created_by = EXCLUDED.created_by
		RETURNING id
	`, studentID, logDate, attendanceStatus, note, homeworkStatus, teacherID).Scan(&id)
	return id, err
}

// ListByStudent returns a child's daily-log history, most recent
// first — the parent's "attendance / note / homework" view.
func (r *DailyLogRepo) ListByStudent(ctx context.Context, studentID string, limit int) ([]DailyLog, error) {
	rows, err := r.store.Pool.Query(ctx, `
		SELECT id, student_id, log_date::text, attendance_status::text, COALESCE(teacher_note, ''), homework_status::text, created_at::text
		FROM daily_logs
		WHERE student_id = $1
		ORDER BY log_date DESC
		LIMIT $2
	`, studentID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DailyLog
	for rows.Next() {
		var l DailyLog
		if err := rows.Scan(&l.ID, &l.StudentID, &l.LogDate, &l.AttendanceStatus, &l.TeacherNote, &l.HomeworkStatus, &l.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}
