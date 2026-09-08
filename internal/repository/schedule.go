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
// Schedule & Conflict Monitor view. branchID "" (network-owner
// director / super_admin) returns every branch's grid.
func (r *ScheduleRepo) ListByBranch(ctx context.Context, branchID string) ([]ScheduleSlot, error) {
	rows, err := r.store.Pool.Query(ctx, `
		SELECT id, group_id, teacher_id, room_id, weekday, start_time::text, end_time::text
		FROM schedule_slots
		WHERE ($1 = '' OR branch_id = $1::uuid)
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
	rows, err := r.store.Pool.Query(ctx, `
		SELECT id, name FROM rooms WHERE ($1 = '' OR branch_id = $1::uuid) ORDER BY name
	`, branchID)
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
	ID         string `json:"id"`
	Name       string `json:"name"`
	TeacherID  string `json:"teacher_id"`
	CourseType string `json:"course_type"` // "language" | "care_and_prep"
	Subject    string `json:"subject,omitempty"`
	Level      string `json:"level,omitempty"` // CEFR/HSK, empty for care_and_prep
}

// CreateGroup inserts a group. level may be "" (required for
// course_type=language, meaningless for care_and_prep).
func (r *ScheduleRepo) CreateGroup(ctx context.Context, branchID, teacherID, name, courseType, subject, level string) (string, error) {
	var lvl any
	if level != "" {
		lvl = level
	}
	var id string
	err := r.store.Pool.QueryRow(ctx, `
		INSERT INTO groups (branch_id, teacher_id, name, course_type, subject, level)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id
	`, branchID, teacherID, name, courseType, subject, lvl).Scan(&id)
	return id, err
}

// ListGroups lists a branch's groups, or every branch's if branchID is
// "" (super_admin / network-owner director — see middleware.EffectiveBranchID).
func (r *ScheduleRepo) ListGroups(ctx context.Context, branchID string) ([]Group, error) {
	rows, err := r.store.Pool.Query(ctx, `
		SELECT id, name, teacher_id, course_type, COALESCE(subject::text, ''), COALESCE(level::text, '')
		FROM groups
		WHERE ($1 = '' OR branch_id = $1::uuid)
		ORDER BY name
	`, branchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Group
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.Name, &g.TeacherID, &g.CourseType, &g.Subject, &g.Level); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// GetGroup fetches one group — used by test-assignment creation to
// derive subject/level from the target group.
func (r *ScheduleRepo) GetGroup(ctx context.Context, groupID string) (*Group, error) {
	var g Group
	err := r.store.Pool.QueryRow(ctx, `
		SELECT id, name, teacher_id, course_type, COALESCE(subject::text, ''), COALESCE(level::text, '')
		FROM groups WHERE id = $1
	`, groupID).Scan(&g.ID, &g.Name, &g.TeacherID, &g.CourseType, &g.Subject, &g.Level)
	if err != nil {
		return nil, err
	}
	return &g, nil
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
// Management view. Subject doubles as both display label and access
// scope: "english"/"chinese" get test-bank access, "mad"/"prodlenka"
// get daily-log access only, "" means not yet assigned.
type Teacher struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	FullName string `json:"full_name"`
	Subject  string `json:"subject,omitempty"`
}

// CreateTeacher inserts a new teacher account. passwordHash is
// generated and hashed by the caller (handler). subject is "" if not
// yet assigned.
func (r *ScheduleRepo) CreateTeacher(ctx context.Context, branchID, email, passwordHash, fullName, subject string) (string, error) {
	var subj any
	if subject != "" {
		subj = subject
	}
	var id string
	err := r.store.Pool.QueryRow(ctx, `
		INSERT INTO teachers (branch_id, email, password_hash, full_name, subject)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, branchID, email, passwordHash, fullName, subj).Scan(&id)
	return id, err
}

// ListTeachers returns every active teacher in a branch, or every
// branch's if branchID is "" (super_admin / network-owner director).
func (r *ScheduleRepo) ListTeachers(ctx context.Context, branchID string) ([]Teacher, error) {
	rows, err := r.store.Pool.Query(ctx, `
		SELECT id, email, full_name, COALESCE(subject::text, '')
		FROM teachers
		WHERE ($1 = '' OR branch_id = $1::uuid) AND is_active
		ORDER BY full_name
	`, branchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Teacher
	for rows.Next() {
		var t Teacher
		if err := rows.Scan(&t.ID, &t.Email, &t.FullName, &t.Subject); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
