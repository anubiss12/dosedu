package handlers

import (
	"context"
	"errors"
	"math"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"

	"github.com/dosedu/lms/internal/ai"
	"github.com/dosedu/lms/internal/auth"
	"github.com/dosedu/lms/internal/repository"
	"github.com/dosedu/lms/internal/telegram"
)

// Deps bundles everything handlers need: session manager + repositories.
// Constructed once in main.go and closed over by every route.
type Deps struct {
	Sessions              *auth.SessionManager
	Creds                 *repository.CredentialRepo
	Leads                 *repository.LeadRepo
	Students              *repository.StudentRepo
	Schedule              *repository.ScheduleRepo
	Parents               *repository.ParentRepo
	Attendance            *repository.AttendanceRepo
	TelegramBot           *telegram.Client
	Notify                *telegram.Notifier
	TelegramHook          *telegram.WebhookHandler
	TelegramWebhookSecret string
	Certificates          *repository.CertificateRepo
	Tickets               *repository.TicketRepo
	Branches              *repository.BranchRepo
	AdminDirectors        *repository.AdminDirectorRepo
	SystemLogs            *repository.SystemLogRepo
	Practice              *repository.PracticeRepo
	Quiz                  *repository.QuizRepo
	Questions             *repository.QuestionRepo
	TestUploads           *repository.TestUploadRepo
	Payments              *repository.PaymentRepo
	AI                    *ai.Client
	Redis                 *redis.Client
	MainSiteURL           string // e.g. https://dosedu.kz — self-checked by SystemHealth
	AppSiteURL            string // e.g. https://app.dosedu.kz — self-checked by SystemHealth
}

func parseDate(s string) (time.Time, error) {
	return time.Parse("2006-01-02", s)
}

type LoginRequest struct {
	Identifier string    `json:"identifier" binding:"required"`
	Password   string    `json:"password" binding:"required"`
	Role       auth.Role `json:"role" binding:"required"`
}

// Login godoc
// @Summary		Log in
// @Description	Authenticates any role (student, parent, teacher, director, super_admin) using its own credential format and returns a JWT bearer token.
// @Tags			auth
// @Accept			json
// @Produce		json
// @Param			request	body		LoginRequest	true	"Login credentials — role determines the expected identifier/password format"
// @Success		200		{object}	map[string]string
// @Failure		400		{object}	map[string]string
// @Failure		401		{object}	map[string]string
// @Router			/public/auth/login [post]
func (d *Deps) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	cred, err := d.Creds.FindByRole(c.Request.Context(), req.Role, req.Identifier)
	if err != nil {
		// Same generic error whether the identifier doesn't exist or the
		// password is wrong — avoids leaking which identifiers are valid.
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	if !auth.CheckPassword(cred.PasswordHash, req.Password) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	languageScope := ""
	if cred.LanguageScope != nil {
		languageScope = *cred.LanguageScope
	}
	token, err := d.Sessions.IssueSession(c.Request.Context(), cred.UserID, req.Role, cred.BranchID, languageScope)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create session"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"token": token})
}

// --- System / super-admin ---

// SystemHealth godoc
// @Summary		Live infrastructure health snapshot
// @Description	Real checks, not hardcoded: Postgres/Redis live ping, host CPU/RAM/disk (gopsutil), DB pool stats, and a best-effort self-check of the main site and student/parent portal (skipped as "not_configured" if MAIN_SITE_URL/APP_SITE_URL are unset). Any backing-service outage is also written to system_logs.
// @Tags			s-admin,health
// @Produce		json
// @Security		BearerAuth
// @Success		200	{object}	map[string]any
// @Router			/s-admin/health [get]
func (d *Deps) SystemHealth(c *gin.Context) {
	ctx := c.Request.Context()

	pgStatus := "up"
	if err := d.Creds.Ping(ctx); err != nil {
		pgStatus = "down"
	}

	redisStatus := "up"
	if d.Redis == nil {
		redisStatus = "not_configured"
	} else if err := d.Redis.Ping(ctx).Err(); err != nil {
		redisStatus = "down"
	}

	cpuPercent := 0.0
	if pcts, err := cpu.PercentWithContext(ctx, 200*time.Millisecond, false); err == nil && len(pcts) > 0 {
		cpuPercent = pcts[0]
	}

	ramPercent := 0.0
	if vm, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		ramPercent = vm.UsedPercent
	}

	diskPercent := 0.0
	if du, err := disk.UsageWithContext(ctx, "/"); err == nil {
		diskPercent = du.UsedPercent
	}

	poolStats := d.Creds.PoolStats()

	c.JSON(http.StatusOK, gin.H{
		"postgres":     pgStatus,
		"redis":        redisStatus,
		"cpu_percent":  round1(cpuPercent),
		"ram_percent":  round1(ramPercent),
		"disk_percent": round1(diskPercent),
		"db_pool":      poolStats,
		"main_site":    d.checkSiteStatus(ctx, d.MainSiteURL),
		"app_site":     d.checkSiteStatus(ctx, d.AppSiteURL),
	})

	if pgStatus == "down" || redisStatus == "down" {
		go func() {
			_ = d.SystemLogs.Insert(context.Background(), "error", "system-health",
				"Backing service outage detected: postgres="+pgStatus+" redis="+redisStatus)
		}()
	}
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }

// checkSiteStatus does a short, best-effort HTTP HEAD self-check of a
// public-facing URL. Returns "not_configured" if url is empty (e.g. no
// MAIN_SITE_URL/APP_SITE_URL set yet in this environment).
func (d *Deps) checkSiteStatus(ctx context.Context, url string) string {
	if url == "" {
		return "not_configured"
	}
	reqCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodHead, url, nil)
	if err != nil {
		return "down"
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "down"
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return "up"
	}
	return "down"
}

type CreateBranchRequest struct {
	Name    string `json:"name" binding:"required"`
	Address string `json:"address"`
}

// CreateBranch godoc
// @Summary		Create a branch
// @Tags			s-admin
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			request	body		CreateBranchRequest	true	"Branch details"
// @Success		201		{object}	map[string]string
// @Router			/s-admin/branches [post]
func (d *Deps) CreateBranch(c *gin.Context) {
	var req CreateBranchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	id, err := d.Branches.Create(c.Request.Context(), req.Name, req.Address)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create branch"})
		return
	}

	_ = d.SystemLogs.Insert(c.Request.Context(), "info", "s-admin", "Branch created: "+req.Name)
	c.JSON(http.StatusCreated, gin.H{"id": id, "name": req.Name})
}

// ListBranches godoc
// @Summary		List all branches
// @Tags			s-admin
// @Produce		json
// @Security		BearerAuth
// @Success		200	{object}	map[string]any
// @Router			/s-admin/branches [get]
func (d *Deps) ListBranches(c *gin.Context) {
	branches, err := d.Branches.ListAll(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load branches"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"branches": branches})
}

type CreateDirectorRequest struct {
	Email    string `json:"email" binding:"required,email"`
	FullName string `json:"full_name" binding:"required"`
	BranchID string `json:"branch_id" binding:"required"`
}

// CreateDirector godoc
// @Summary		Create a director account
// @Description	Auto-generates an 8-character complex password per the director password policy, returned once in the response.
// @Tags			s-admin
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			request	body		CreateDirectorRequest	true	"Director details"
// @Success		201		{object}	map[string]string
// @Router			/s-admin/directors [post]
func (d *Deps) CreateDirector(c *gin.Context) {
	var req CreateDirectorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tempPassword, err := auth.GenerateComplexPassword(auth.RoleDirector, 8)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate password"})
		return
	}
	hash, err := auth.HashPassword(auth.RoleDirector, tempPassword)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
		return
	}

	id, err := d.AdminDirectors.Create(c.Request.Context(), req.BranchID, req.Email, hash, req.FullName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create director"})
		return
	}

	_ = d.SystemLogs.Insert(c.Request.Context(), "info", "s-admin", "Director created: "+req.Email)
	c.JSON(http.StatusCreated, gin.H{
		"id":                 id,
		"email":              req.Email,
		"temporary_password": tempPassword,
		"credentials_notice": "Бұл құпия сөз тек осы жауапта бір рет көрсетіледі — оны директорға дереу жіберіңіз.",
	})
}

// ListDirectors godoc
// @Summary		List all director accounts
// @Tags			s-admin
// @Produce		json
// @Security		BearerAuth
// @Success		200	{object}	map[string]any
// @Router			/s-admin/directors [get]
func (d *Deps) ListDirectors(c *gin.Context) {
	directors, err := d.AdminDirectors.ListAll(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load directors"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"directors": directors})
}

// ListSystemLogs godoc
// @Summary		List recent system/security logs
// @Tags			s-admin
// @Produce		json
// @Security		BearerAuth
// @Success		200	{object}	map[string]any
// @Router			/s-admin/logs [get]
func (d *Deps) ListSystemLogs(c *gin.Context) {
	logs, err := d.SystemLogs.ListRecent(c.Request.Context(), 100)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load logs"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"logs": logs})
}

// --- Director ---

type CreateTeacherRequest struct {
	Email    string `json:"email" binding:"required,email"`
	FullName string `json:"full_name" binding:"required"`
	Subject  string `json:"subject"`
	// LanguageScope restricts this teacher to one language's tests/
	// content ("en"/"zh"). Leave empty for a mad/prodlenka teacher, who
	// gets attendance-only access with no test-upload tab at all.
	LanguageScope string `json:"language_scope" binding:"omitempty,oneof=en zh"`
}

// CreateTeacher godoc
// @Summary		Create a teacher account
// @Description	Auto-generates a 6+ character policy-compliant password, returned once in the response.
// @Tags			director,staff
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			request	body		CreateTeacherRequest	true	"Teacher details"
// @Success		201		{object}	map[string]string
// @Router			/director/teachers [post]
func (d *Deps) CreateTeacher(c *gin.Context) {
	claims := c.MustGet("claims").(*auth.Claims)

	var req CreateTeacherRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tempPassword, err := auth.GenerateTeacherPassword(8)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate password"})
		return
	}
	hash, err := auth.HashPassword(auth.RoleTeacher, tempPassword)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
		return
	}

	id, err := d.Schedule.CreateTeacher(c.Request.Context(), claims.BranchID, req.Email, hash, req.FullName, req.Subject, req.LanguageScope)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create teacher"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":                 id,
		"email":              req.Email,
		"temporary_password": tempPassword,
		"credentials_notice": "Бұл құпия сөз тек осы жауапта бір рет көрсетіледі — оны мұғалімге дереу жіберіңіз.",
	})
}

// ListTeachers godoc
// @Summary		List branch teachers
// @Tags			director,staff
// @Produce		json
// @Security		BearerAuth
// @Success		200	{object}	map[string]any
// @Router			/director/teachers [get]
func (d *Deps) ListTeachers(c *gin.Context) {
	claims := c.MustGet("claims").(*auth.Claims)
	teachers, err := d.Schedule.ListTeachers(c.Request.Context(), claims.BranchID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load teachers"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"teachers": teachers})
}

func (d *Deps) GetScheduleGrid(c *gin.Context) {
	claims := c.MustGet("claims").(*auth.Claims)
	slots, err := d.Schedule.ListByBranch(c.Request.Context(), claims.BranchID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load schedule"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"schedule": slots})
}

type CreateScheduleSlotRequest struct {
	GroupID   string `json:"group_id" binding:"required"`
	TeacherID string `json:"teacher_id" binding:"required"`
	RoomID    string `json:"room_id" binding:"required"`
	Weekday   int    `json:"weekday" binding:"required,min=1,max=7"`
	StartTime string `json:"start_time" binding:"required"` // "HH:MM"
	EndTime   string `json:"end_time" binding:"required"`
}

// CreateScheduleSlot godoc
// @Summary		Create a schedule slot (Conflict Checker)
// @Description	Books a group into a room/time slot. Returns 409 if the room or teacher is already booked at an overlapping time on the same weekday — the schedule "conflict checker".
// @Tags			director,schedule
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			request	body		CreateScheduleSlotRequest	true	"Slot details"
// @Success		201		{object}	map[string]string
// @Failure		409		{object}	map[string]string	"conflict — room/teacher already booked"
// @Router			/director/schedule [post]
func (d *Deps) CreateScheduleSlot(c *gin.Context) {
	claims := c.MustGet("claims").(*auth.Claims)

	var req CreateScheduleSlotRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	start, err1 := time.Parse("15:04", req.StartTime)
	end, err2 := time.Parse("15:04", req.EndTime)
	if err1 != nil || err2 != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid time format, expected HH:MM"})
		return
	}

	id, err := d.Schedule.CreateSlot(c.Request.Context(), claims.BranchID, req.GroupID, req.TeacherID, req.RoomID, req.Weekday, start, end)
	if err != nil {
		if errors.Is(err, repository.ErrScheduleConflict) {
			c.JSON(http.StatusConflict, gin.H{
				"error":   "conflict",
				"message": "Бұл кабинет/уақыт аралығы басқа сабаққа тағайындалған — конфликт бар.",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create schedule slot"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"id": id, "status": "created"})
}

func (d *Deps) ListRooms(c *gin.Context) {
	claims := c.MustGet("claims").(*auth.Claims)
	rooms, err := d.Schedule.ListRooms(c.Request.Context(), claims.BranchID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load rooms"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"rooms": rooms})
}

type CreateRoomRequest struct {
	Name string `json:"name" binding:"required"`
}

func (d *Deps) CreateRoom(c *gin.Context) {
	claims := c.MustGet("claims").(*auth.Claims)
	var req CreateRoomRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	id, err := d.Schedule.CreateRoom(c.Request.Context(), claims.BranchID, req.Name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create room"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id})
}

func (d *Deps) ListGroups(c *gin.Context) {
	claims := c.MustGet("claims").(*auth.Claims)
	groups, err := d.Schedule.ListGroups(c.Request.Context(), claims.BranchID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load groups"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"groups": groups})
}

type CreateGroupRequest struct {
	Name      string `json:"name" binding:"required"`
	TeacherID string `json:"teacher_id" binding:"required"`
	Program   string `json:"program" binding:"required,oneof=language_course prodlenka"`
	Level     string `json:"level"`
}

func (d *Deps) CreateGroup(c *gin.Context) {
	claims := c.MustGet("claims").(*auth.Claims)
	var req CreateGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	id, err := d.Schedule.CreateGroup(c.Request.Context(), claims.BranchID, req.TeacherID, req.Name, req.Program, req.Level)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create group"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id})
}

func (d *Deps) GetMonthlyReports(c *gin.Context) {
	claims := c.MustGet("claims").(*auth.Claims)
	overdue, err := d.Students.ListOverdue(c.Request.Context(), claims.BranchID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load reports"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"overdue_students": overdue})
}

// ListBranchStudents godoc
// @Summary		List every student in the director's branch
// @Tags			director,students
// @Produce		json
// @Security		BearerAuth
// @Success		200	{object}	map[string]any
// @Router			/director/students [get]
func (d *Deps) ListBranchStudents(c *gin.Context) {
	claims := c.MustGet("claims").(*auth.Claims)
	students, err := d.Students.ListByBranch(c.Request.Context(), claims.BranchID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load students"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"students": students})
}

// ListBranchPayments godoc
// @Summary		List every payment in the director's branch
// @Tags			director,payments
// @Produce		json
// @Security		BearerAuth
// @Success		200	{object}	map[string]any
// @Router			/director/payments [get]
func (d *Deps) ListBranchPayments(c *gin.Context) {
	claims := c.MustGet("claims").(*auth.Claims)
	payments, err := d.Payments.ListByBranch(c.Request.Context(), claims.BranchID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load payments"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"payments": payments})
}

// --- Teacher ---

func (d *Deps) GetTeacherSchedule(c *gin.Context) {
	claims := c.MustGet("claims").(*auth.Claims)
	slots, err := d.Schedule.ListByTeacher(c.Request.Context(), claims.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load schedule"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"schedule": slots})
}

type MarkAttendanceRequest struct {
	StudentID string   `json:"student_id" binding:"required"`
	Status    string   `json:"status" binding:"required,oneof=present absent excused"`
	Grade     *float64 `json:"grade"`
	Date      string   `json:"date"` // YYYY-MM-DD, defaults to today if omitted
}

// MarkAttendance godoc
// @Summary		Mark a student's attendance/grade
// @Description	Teacher gradebook write: records present/absent/excused and an optional grade for one lesson day. Upserts — re-marking the same day corrects the earlier entry.
// @Tags			teacher,gradebook
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			request	body		MarkAttendanceRequest	true	"Attendance entry"
// @Success		200		{object}	map[string]string
// @Router			/teacher/attendance [post]
func (d *Deps) MarkAttendance(c *gin.Context) {
	claims := c.MustGet("claims").(*auth.Claims)

	var req MarkAttendanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	lessonDate := time.Now()
	if req.Date != "" {
		parsed, err := parseDate(req.Date)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid date format, expected YYYY-MM-DD"})
			return
		}
		lessonDate = parsed
	}

	if err := d.Attendance.Mark(c.Request.Context(), req.StudentID, req.Status, req.Grade, claims.UserID, lessonDate); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to mark attendance"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "marked"})
}

// ListGradebook godoc
// @Summary		Get a group's gradebook for a lesson day
// @Tags			teacher,gradebook
// @Produce		json
// @Security		BearerAuth
// @Param			groupId	path		string	true	"Group ID"
// @Param			date	query		string	false	"YYYY-MM-DD, defaults to today"
// @Success		200		{object}	map[string]any
// @Router			/teacher/groups/{groupId}/gradebook [get]
func (d *Deps) ListGradebook(c *gin.Context) {
	groupID := c.Param("groupId")

	lessonDate := time.Now()
	if dateParam := c.Query("date"); dateParam != "" {
		parsed, err := parseDate(dateParam)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid date format, expected YYYY-MM-DD"})
			return
		}
		lessonDate = parsed
	}

	rows, err := d.Attendance.ListByGroupAndDate(c.Request.Context(), groupID, lessonDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load gradebook"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"students": rows})
}

type ConfirmPaymentRequest struct {
	Amount      float64 `json:"amount" binding:"required"`
	PeriodStart string  `json:"period_start" binding:"required"` // YYYY-MM-DD
	PeriodEnd   string  `json:"period_end" binding:"required"`
	Method      string  `json:"method" binding:"omitempty,oneof=cash transfer"`
}

func (d *Deps) ConfirmPayment(c *gin.Context) {
	claims := c.MustGet("claims").(*auth.Claims)
	studentID := c.Param("studentId")

	var req ConfirmPaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	start, err1 := parseDate(req.PeriodStart)
	end, err2 := parseDate(req.PeriodEnd)
	if err1 != nil || err2 != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid date format, expected YYYY-MM-DD"})
		return
	}

	method := req.Method
	if method == "" {
		method = "cash"
	}
	if err := d.Students.ConfirmPayment(c.Request.Context(), studentID, claims.UserID, req.Amount, start, end, method); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to confirm payment"})
		return
	}

	// Best-effort Telegram ping — a missing/unlinked parent chat should
	// never fail the payment confirmation itself.
	go func() {
		ctx := context.Background()
		summary, err := d.Students.GetSummary(ctx, studentID)
		if err != nil {
			return
		}
		chatID, err := d.Parents.FindParentChatIDByStudent(ctx, studentID)
		if err != nil {
			return
		}
		_ = d.Notify.NotifyPaymentConfirmed(ctx, chatID, summary.FullName, req.Amount, end)
	}()

	c.JSON(http.StatusOK, gin.H{"status": "paid", "student_id": studentID})
}

// --- Family (student / parent) ---

// resolveStudentID returns the student ID a family-scoped request should
// act on. A student's own JWT already carries their student ID
// (claims.UserID). A parent's JWT carries the *parent's* ID instead, so
// this looks up their (first) linked child. Every family-facing read
// endpoint (progress, balance, report, certificates) must go through
// this instead of using claims.UserID directly, or parent logins 404.
func (d *Deps) resolveStudentID(ctx context.Context, claims *auth.Claims) (string, error) {
	if claims.Role == auth.RoleStudent {
		return claims.UserID, nil
	}
	ids, err := d.Students.FindIDsByParentID(ctx, claims.UserID)
	if err != nil {
		return "", err
	}
	if len(ids) == 0 {
		return "", errors.New("no student linked to this parent account yet")
	}
	// TODO: parents with more than one child currently only see the
	// first — a ?student_id= selector for multi-child families is a
	// follow-up, not handled here yet.
	return ids[0], nil
}

type RecordPracticeAttemptRequest struct {
	Language string `json:"language" binding:"required,oneof=en zh"`
	Score    int    `json:"score" binding:"min=0"`
	Total    int    `json:"total" binding:"required,min=1"`
}

// RecordPracticeAttempt godoc
// @Summary		Record a completed practice-test attempt
// @Description	Student-only. Saves one finished quiz run to their "Нәтижелер" history.
// @Tags			family,practice
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			request	body		RecordPracticeAttemptRequest	true	"Attempt result"
// @Success		201		{object}	map[string]string
// @Router			/family/practice/attempts [post]
func (d *Deps) RecordPracticeAttempt(c *gin.Context) {
	claims := c.MustGet("claims").(*auth.Claims)
	if claims.Role != auth.RoleStudent {
		c.JSON(http.StatusForbidden, gin.H{"error": "only students record practice attempts"})
		return
	}

	var req RecordPracticeAttemptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	id, err := d.Practice.RecordAttempt(c.Request.Context(), claims.UserID, req.Language, req.Score, req.Total)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save attempt"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id})
}

// ListPracticeAttempts godoc
// @Summary		List a student's practice-test history
// @Tags			family,practice
// @Produce		json
// @Security		BearerAuth
// @Success		200	{object}	map[string]any
// @Router			/family/practice/attempts [get]
func (d *Deps) ListPracticeAttempts(c *gin.Context) {
	claims := c.MustGet("claims").(*auth.Claims)
	studentID, err := d.resolveStudentID(c.Request.Context(), claims)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	attempts, err := d.Practice.ListByStudent(c.Request.Context(), studentID, 50)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load attempts"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"attempts": attempts})
}

func (d *Deps) GetStudentProgress(c *gin.Context) {
	claims := c.MustGet("claims").(*auth.Claims)
	studentID, err := d.resolveStudentID(c.Request.Context(), claims)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	summary, err := d.Students.GetSummary(c.Request.Context(), studentID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "student not found"})
		return
	}
	c.JSON(http.StatusOK, summary)
}

func (d *Deps) GetBalance(c *gin.Context) {
	claims := c.MustGet("claims").(*auth.Claims)
	studentID, err := d.resolveStudentID(c.Request.Context(), claims)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	summary, err := d.Students.GetSummary(c.Request.Context(), studentID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "student not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"balance": summary.Balance, "payment_status": summary.PaymentStatus})
}

// --- Family: language track, question bank, quiz submit, AI hint ---

type SetLanguageRequest struct {
	Language string `json:"language" binding:"required,oneof=en zh"`
}

// SetStudentLanguage lets a student self-declare their language track
// once (locked after that). A bridge until Phase 4 gives directors a
// real enrollment-time assignment UI.
func (d *Deps) SetStudentLanguage(c *gin.Context) {
	claims := c.MustGet("claims").(*auth.Claims)
	if claims.Role != auth.RoleStudent {
		c.JSON(http.StatusForbidden, gin.H{"error": "only a student can set their own language track"})
		return
	}

	var req SetLanguageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := d.Students.SetLanguageIfUnset(c.Request.Context(), claims.UserID, req.Language); err != nil {
		if errors.Is(err, repository.ErrLanguageAlreadySet) {
			c.JSON(http.StatusConflict, gin.H{"error": "already_set", "message": "Тіл бағыты бұрын таңдалған."})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to set language"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"language": req.Language})
}

// ListQuestions godoc
// @Summary		Get a practice or official question set
// @Description	Student-only. kind=official is rejected with 409 if the student already has an official attempt for this language (one-time only).
// @Tags			family,quiz
// @Produce		json
// @Security		BearerAuth
// @Param			language	query		string	true	"en or zh"
// @Param			level		query		string	true	"e.g. Beginner, HSK1"
// @Param			kind		query		string	true	"practice or official"
// @Success		200			{object}	map[string]any
// @Router			/family/questions [get]
func (d *Deps) ListQuestions(c *gin.Context) {
	claims := c.MustGet("claims").(*auth.Claims)
	if claims.Role != auth.RoleStudent {
		c.JSON(http.StatusForbidden, gin.H{"error": "only students take tests"})
		return
	}

	language := c.Query("language")
	level := c.Query("level")
	kind := c.Query("kind")
	if (language != "en" && language != "zh") || level == "" || (kind != "practice" && kind != "official") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "language must be en/zh, level required, kind must be practice/official"})
		return
	}

	if kind == "official" {
		has, err := d.Quiz.HasOfficialAttempt(c.Request.Context(), claims.UserID, language)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check prior attempts"})
			return
		}
		if has {
			c.JSON(http.StatusConflict, gin.H{"error": "already_taken", "message": "Сіз бұл деңгей тестін бұрын тапсырғансыз — ол бір рет қана тапсырылады."})
			return
		}
	}

	questions, err := d.Questions.ListForStudent(c.Request.Context(), language, level, kind == "official")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load questions"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"questions": questions})
}

type SubmitQuizAnswer struct {
	QuestionID string `json:"question_id" binding:"required"`
	Selected   int    `json:"selected"`
}

type SubmitQuizRequest struct {
	Language string             `json:"language" binding:"required,oneof=en zh"`
	Kind     string             `json:"kind" binding:"required,oneof=practice official"`
	Answers  []SubmitQuizAnswer `json:"answers" binding:"required,min=1"`
}

type QuizResultItem struct {
	QuestionID string   `json:"question_id"`
	Question   string   `json:"question"`
	Options    []string `json:"options"`
	Selected   int      `json:"selected"`
	Correct    int      `json:"correct"`
	IsCorrect  bool     `json:"is_correct"`
}

// SubmitQuiz godoc
// @Summary		Grade and record a finished practice/official attempt
// @Description	Student-only. official is rejected with 409 if already taken once. Returns a per-question breakdown so the UI can offer an AI explanation on each miss.
// @Tags			family,quiz
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			request	body		SubmitQuizRequest	true	"Answers"
// @Success		201		{object}	map[string]any
// @Router			/family/quiz/submit [post]
func (d *Deps) SubmitQuiz(c *gin.Context) {
	claims := c.MustGet("claims").(*auth.Claims)
	if claims.Role != auth.RoleStudent {
		c.JSON(http.StatusForbidden, gin.H{"error": "only students take tests"})
		return
	}

	var req SubmitQuizRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Kind == "official" {
		has, err := d.Quiz.HasOfficialAttempt(c.Request.Context(), claims.UserID, req.Language)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check prior attempts"})
			return
		}
		if has {
			c.JSON(http.StatusConflict, gin.H{"error": "already_taken", "message": "Сіз бұл деңгей тестін бұрын тапсырғансыз — ол бір рет қана тапсырылады."})
			return
		}
	}

	ids := make([]string, len(req.Answers))
	selectedByID := make(map[string]int, len(req.Answers))
	for i, a := range req.Answers {
		ids[i] = a.QuestionID
		selectedByID[a.QuestionID] = a.Selected
	}

	questions, err := d.Questions.GetByIDs(c.Request.Context(), ids)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load questions"})
		return
	}

	score := 0
	results := make([]QuizResultItem, 0, len(questions))
	for _, q := range questions {
		selected := selectedByID[q.ID]
		isCorrect := selected == q.CorrectOption
		if isCorrect {
			score++
		}
		options := []string{q.OptionA, q.OptionB}
		if q.OptionC != "" {
			options = append(options, q.OptionC)
		}
		if q.OptionD != "" {
			options = append(options, q.OptionD)
		}
		results = append(results, QuizResultItem{
			QuestionID: q.ID,
			Question:   q.Question,
			Options:    options,
			Selected:   selected,
			Correct:    q.CorrectOption,
			IsCorrect:  isCorrect,
		})
	}
	total := len(questions)

	var attemptID string
	if req.Kind == "official" {
		attemptID, err = d.Quiz.RecordAttempt(c.Request.Context(), claims.UserID, req.Language, "official", score, total)
	} else {
		attemptID, err = d.Practice.RecordAttempt(c.Request.Context(), claims.UserID, req.Language, score, total)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save attempt"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"id": attemptID, "score": score, "total": total, "results": results})
}

type ExplainMistakeRequest struct {
	Question        string `json:"question" binding:"required"`
	ChosenAnswer    string `json:"chosen_answer" binding:"required"`
	CorrectAnswer   string `json:"correct_answer" binding:"required"`
	Language        string `json:"language" binding:"required,oneof=en zh"`         // language being learned
	ExplainLanguage string `json:"explain_language" binding:"required,oneof=kk ru en zh"` // language the student wants the explanation in
}

// ExplainQuizMistake godoc
// @Summary		AI explanation for one missed quiz question
// @Description	Student-only. Explains in explain_language (the student's choice), independent of the language being learned.
// @Tags			family,quiz,ai
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			request	body		ExplainMistakeRequest	true	"Missed question detail"
// @Success		200		{object}	map[string]string
// @Router			/family/quiz/explain [post]
func (d *Deps) ExplainQuizMistake(c *gin.Context) {
	claims := c.MustGet("claims").(*auth.Claims)
	if claims.Role != auth.RoleStudent {
		c.JSON(http.StatusForbidden, gin.H{"error": "only students request quiz explanations"})
		return
	}

	var req ExplainMistakeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	explanation, err := d.AI.ExplainMistake(c.Request.Context(), req.Question, req.ChosenAnswer, req.CorrectAnswer, req.Language, req.ExplainLanguage)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get AI explanation"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"explanation": explanation})
}
