package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

var ErrScheduleConflict = errors.New("room already booked for an overlapping time slot")

type ScheduleSlot struct {
	ID        string `json:"id"`
	GroupID   string `json:"group_id"`
	TeacherID string `json:"teacher_id"`
	RoomID    string `json:"room_id"`
	Weekday   int    `json:"weekday"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
}

type ScheduleRepo struct{ store *Store }

func NewScheduleRepo(s *Store) *ScheduleRepo { return &ScheduleRepo{store: s} }

// CreateSlot inserts a schedule slot. The DB's EXCLUDE constraint on
// schedule_slots is the source of truth for conflict prevention; we
// translate its unique_violation/exclusion_violation into a friendly
// domain error instead of leaking a raw SQL error to the API caller.
func (r *ScheduleRepo) CreateSlot(ctx context.Context, branchID, groupID, teacherID, roomID string, weekday int, start, end time.Time) (string, error) {
	var id string
	err := r.store.Pool.QueryRow(ctx, `
		INSERT INTO schedule_slots (branch_id, group_id, teacher_id, room_id, weekday, start_time, end_time)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id
	`, branchID, groupID, teacherID, roomID, weekday, start, end).Scan(&id)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23P01" { // exclusion_violation
			return "", ErrScheduleConflict
		}
		return "", err
	}
	return id, nil
}

// ListByBranch returns the full weekly grid for the director's
// Schedule & Conflict Monitor view.
func (r *ScheduleRepo) ListByBranch(ctx context.Context, branchID string) ([]ScheduleSlot, error) {
	rows, err := r.store.Pool.Query(ctx, `
		SELECT id, group_id, teacher_id, room_id, weekday, start_time::text, end_time::text
		FROM schedule_slots
		WHERE branch_id = $1
		ORDER BY weekday, start_time
	`, branchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ScheduleSlot
	for rows.Next() {
		var s ScheduleSlot
		if err := rows.Scan(&s.ID, &s.GroupID, &s.TeacherID, &s.RoomID, &s.Weekday, &s.StartTime, &s.EndTime); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// --- Rooms ---

type Room struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (r *ScheduleRepo) CreateRoom(ctx context.Context, branchID, name string) (string, error) {
	var id string
	err := r.store.Pool.QueryRow(ctx, `
		INSERT INTO rooms (branch_id, name) VALUES ($1, $2) RETURNING id
	`, branchID, name).Scan(&id)
	return id, err
}

func (r *ScheduleRepo) ListRooms(ctx context.Context, branchID string) ([]Room, error) {
	rows, err := r.store.Pool.Query(ctx, `SELECT id, name FROM rooms WHERE branch_id = $1 ORDER BY name`, branchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Room
	for rows.Next() {
		var rm Room
		if err := rows.Scan(&rm.ID, &rm.Name); err != nil {
			return nil, err
		}
		out = append(out, rm)
	}
	return out, rows.Err()
}

// --- Groups ---

type Group struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	TeacherID string `json:"teacher_id"`
	Program   string `json:"program"`
	Level     string `json:"level"`
}

func (r *ScheduleRepo) CreateGroup(ctx context.Context, branchID, teacherID, name, program, level string) (string, error) {
	var id string
	err := r.store.Pool.QueryRow(ctx, `
		INSERT INTO groups (branch_id, teacher_id, name, program, level)
		VALUES ($1, $2, $3, $4, $5) RETURNING id
	`, branchID, teacherID, name, program, level).Scan(&id)
	return id, err
}

func (r *ScheduleRepo) ListGroups(ctx context.Context, branchID string) ([]Group, error) {
	rows, err := r.store.Pool.Query(ctx, `
		SELECT id, name, teacher_id, program, level FROM groups WHERE branch_id = $1 ORDER BY name
	`, branchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Group
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.Name, &g.TeacherID, &g.Program, &g.Level); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// Daily Schedule view.
func (r *ScheduleRepo) ListByTeacher(ctx context.Context, teacherID string) ([]ScheduleSlot, error) {
	rows, err := r.store.Pool.Query(ctx, `
		SELECT id, group_id, teacher_id, room_id, weekday, start_time::text, end_time::text
		FROM schedule_slots
		WHERE teacher_id = $1
		ORDER BY weekday, start_time
	`, teacherID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ScheduleSlot
	for rows.Next() {
		var s ScheduleSlot
		if err := rows.Scan(&s.ID, &s.GroupID, &s.TeacherID, &s.RoomID, &s.Weekday, &s.StartTime, &s.EndTime); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// Teacher is a lightweight staff record for the director's Staff
// Management view.
type Teacher struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	FullName      string `json:"full_name"`
	Subject       string `json:"subject"`
	LanguageScope string `json:"language_scope,omitempty"` // "en" | "zh" | "" (mad/prodlenka)
}

// CreateTeacher inserts a new teacher account. passwordHash is
// generated and hashed by the caller (handler). languageScope is ""
// for a mad/prodlenka teacher (no test-upload access at all).
func (r *ScheduleRepo) CreateTeacher(ctx context.Context, branchID, email, passwordHash, fullName, subject, languageScope string) (string, error) {
	var scope *string
	if languageScope != "" {
		scope = &languageScope
	}
	var id string
	err := r.store.Pool.QueryRow(ctx, `
		INSERT INTO teachers (branch_id, email, password_hash, full_name, subject, language_scope)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`, branchID, email, passwordHash, fullName, subject, scope).Scan(&id)
	return id, err
}

// ListTeachers returns every active teacher in a branch.
func (r *ScheduleRepo) ListTeachers(ctx context.Context, branchID string) ([]Teacher, error) {
	rows, err := r.store.Pool.Query(ctx, `
		SELECT id, email, full_name, COALESCE(subject, ''), COALESCE(language_scope, '')
		FROM teachers WHERE branch_id = $1 AND is_active
		ORDER BY full_name
	`, branchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Teacher
	for rows.Next() {
		var t Teacher
		if err := rows.Scan(&t.ID, &t.Email, &t.FullName, &t.Subject, &t.LanguageScope); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
