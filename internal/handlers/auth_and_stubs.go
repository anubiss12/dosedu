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
	"github.com/dosedu/lms/internal/middleware"
	"github.com/dosedu/lms/internal/qrcode"
	"github.com/dosedu/lms/internal/ratelimit"
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
	Questions             *repository.QuestionRepo
	TestAssignments       *repository.TestAssignmentRepo
	DailyLogs             *repository.DailyLogRepo
	Impersonation         *repository.ImpersonationRepo
	ChurnAlerts           *repository.ChurnAlertRepo
	TestUploads           *repository.TestUploadRepo
	Payments              *repository.PaymentRepo
	AI                    *ai.Client
	Redis                 *redis.Client
	RateLimit             *ratelimit.Limiter
	MainSiteURL           string // e.g. https://dosedu.kz — self-checked by SystemHealth
	AppSiteURL            string // e.g. https://app.dosedu.kz — self-checked by SystemHealth
	TeacherSiteURL        string
	DirectorSiteURL       string
	AdminSiteURL          string
}

func parseDate(s string) (time.Time, error) {
	return time.Parse("2006-01-02", s)
}

// resolveBranchID picks which branch a WRITE targets. A regular
// (single-branch) director always writes into their own branch; a
// network-owner director (or super_admin, though they don't normally
// hit these routes) must say explicitly which branch via `requested`
// — a new branch is not implied.
func resolveBranchID(claims *auth.Claims, requested string) (string, error) {
	if claims.Role == auth.RoleSuperAdmin || claims.IsNetworkOwner {
		if requested == "" {
			return "", errors.New("branch_id is required for a network-wide account")
		}
		return requested, nil
	}
	return claims.BranchID, nil
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

	subject := ""
	if cred.Subject != nil {
		subject = *cred.Subject
	}
	token, err := d.Sessions.IssueSession(c.Request.Context(), cred.UserID, req.Role, cred.BranchID, subject, cred.IsNetworkOwner)
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
		"postgres":      pgStatus,
		"redis":         redisStatus,
		"cpu_percent":   round1(cpuPercent),
		"ram_percent":   round1(ramPercent),
		"disk_percent":  round1(diskPercent),
		"db_pool":       poolStats,
		"main_site":     d.checkSiteStatus(ctx, d.MainSiteURL),
		"student_site":  d.checkSiteStatus(ctx, d.AppSiteURL),
		"teacher_site":  d.checkSiteStatus(ctx, d.TeacherSiteURL),
		"director_site": d.checkSiteStatus(ctx, d.DirectorSiteURL),
		"admin_site":    d.checkSiteStatus(ctx, d.AdminSiteURL),
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

// DeleteBranch godoc
// @Summary		Soft-delete a branch
// @Description	Removes the branch from every listing without deleting its row or anything referencing it — history (students, groups, payments, ...) is preserved. Distinct from a temporary block, which would use branch status instead.
// @Tags			s-admin
// @Produce		json
// @Security		BearerAuth
// @Param			id	path		string	true	"Branch ID"
// @Success		200	{object}	map[string]string
// @Failure		404	{object}	map[string]string
// @Router			/s-admin/branches/{id} [delete]
func (d *Deps) DeleteBranch(c *gin.Context) {
	if err := d.Branches.SoftDelete(c.Request.Context(), c.Param("id")); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "branch not found"})
		return
	}
	_ = d.SystemLogs.Insert(c.Request.Context(), "info", "s-admin", "Branch deleted: "+c.Param("id"))
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

type CreateDirectorRequest struct {
	Email    string `json:"email" binding:"required,email"`
	FullName string `json:"full_name" binding:"required"`
	BranchID string `json:"branch_id" binding:"required"`
	// IsNetworkOwner grants this director every branch (via
	// middleware.EffectiveBranchID) instead of just BranchID's "home" branch.
	IsNetworkOwner bool `json:"is_network_owner"`
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

	id, err := d.AdminDirectors.Create(c.Request.Context(), req.BranchID, req.Email, hash, req.FullName, req.IsNetworkOwner)
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

// ForceResetDirectorPassword godoc
// @Summary		Force-reset a director's password
// @Description	Super-admin-only. Generates a new policy-compliant password and overwrites the director's hash immediately — for a locked-out director, no "forgot password" flow exists yet. Returned once, same one-time-display pattern as account creation.
// @Tags			s-admin
// @Produce		json
// @Security		BearerAuth
// @Param			id	path		string	true	"Director ID"
// @Success		200	{object}	map[string]string
// @Router			/s-admin/directors/{id}/force-reset-password [post]
func (d *Deps) ForceResetDirectorPassword(c *gin.Context) {
	directorID := c.Param("id")

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

	if err := d.AdminDirectors.ResetPassword(c.Request.Context(), directorID, hash); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "director not found"})
		return
	}

	_ = d.SystemLogs.Insert(c.Request.Context(), "info", "s-admin", "Password force-reset for director: "+directorID)
	c.JSON(http.StatusOK, gin.H{
		"temporary_password": tempPassword,
		"credentials_notice": "Бұл құпия сөз тек осы жауапта бір рет көрсетіледі — оны директорға дереу жіберіңіз.",
	})
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
	// Subject determines this teacher's access: "english"/"chinese" get
	// the question-bank/test-upload tabs; "mad"/"prodlenka" get
	// daily-log (attendance/note/homework) access only, no tests at
	// all. Leave empty if not yet assigned.
	Subject string `json:"subject" binding:"omitempty,oneof=english chinese mad prodlenka"`
	// BranchID is required only for a network-owner director (which
	// branch this teacher belongs to); ignored for a single-branch
	// director, who can only ever create within their own branch.
	BranchID string `json:"branch_id"`
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

	branchID, err := resolveBranchID(claims, req.BranchID)
	if err != nil {
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

	id, err := d.Schedule.CreateTeacher(c.Request.Context(), branchID, req.Email, hash, req.FullName, req.Subject)
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
	teachers, err := d.Schedule.ListTeachers(c.Request.Context(), middleware.EffectiveBranchID(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load teachers"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"teachers": teachers})
}

// ForceResetTeacherPassword godoc
// @Summary		Force-reset a teacher's password
// @Description	Director-only, scoped to the director's own branch (or any branch for a network-owner director). Generates a new policy-compliant password and overwrites the teacher's hash immediately.
// @Tags			director,staff
// @Produce		json
// @Security		BearerAuth
// @Param			id	path		string	true	"Teacher ID"
// @Success		200	{object}	map[string]string
// @Router			/director/teachers/{id}/force-reset-password [post]
func (d *Deps) ForceResetTeacherPassword(c *gin.Context) {
	teacherID := c.Param("id")

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

	if err := d.Schedule.ResetTeacherPassword(c.Request.Context(), teacherID, middleware.EffectiveBranchID(c), hash); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "teacher not found in your branch"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"temporary_password": tempPassword,
		"credentials_notice": "Бұл құпия сөз тек осы жауапта бір рет көрсетіледі — оны мұғалімге дереу жіберіңіз.",
	})
}

func (d *Deps) GetScheduleGrid(c *gin.Context) {
	slots, err := d.Schedule.ListByBranch(c.Request.Context(), middleware.EffectiveBranchID(c))
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
	rooms, err := d.Schedule.ListRooms(c.Request.Context(), middleware.EffectiveBranchID(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load rooms"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"rooms": rooms})
}

type CreateRoomRequest struct {
	Name     string `json:"name" binding:"required"`
	BranchID string `json:"branch_id"` // required only for a network-owner director
}

func (d *Deps) CreateRoom(c *gin.Context) {
	claims := c.MustGet("claims").(*auth.Claims)
	var req CreateRoomRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	branchID, err := resolveBranchID(claims, req.BranchID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	id, err := d.Schedule.CreateRoom(c.Request.Context(), branchID, req.Name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create room"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id})
}

func (d *Deps) ListGroups(c *gin.Context) {
	groups, err := d.Schedule.ListGroups(c.Request.Context(), middleware.EffectiveBranchID(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load groups"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"groups": groups})
}

type CreateGroupRequest struct {
	Name       string `json:"name" binding:"required"`
	TeacherID  string `json:"teacher_id" binding:"required"`
	CourseType string `json:"course_type" binding:"required,oneof=language care_and_prep"`
	Subject    string `json:"subject" binding:"required,oneof=english chinese mad prodlenka"`
	Level      string `json:"level" binding:"omitempty,oneof=A1 A2 B1 B2 C1 C2 HSK1 HSK2 HSK3 HSK4 HSK5 HSK6"`
	BranchID   string `json:"branch_id"` // required only for a network-owner director
}

func (d *Deps) CreateGroup(c *gin.Context) {
	claims := c.MustGet("claims").(*auth.Claims)
	var req CreateGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	branchID, err := resolveBranchID(claims, req.BranchID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	id, err := d.Schedule.CreateGroup(c.Request.Context(), branchID, req.TeacherID, req.Name, req.CourseType, req.Subject, req.Level)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create group"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id})
}

// ListDirectorBranches godoc
// @Summary		Branches this director may act on
// @Description	A network-owner director gets every branch (for the frontend's branch switcher and for picking a target branch_id on writes); a single-branch director gets just their own.
// @Tags			director
// @Produce		json
// @Security		BearerAuth
// @Success		200	{object}	map[string]any
// @Router			/director/branches [get]
func (d *Deps) ListDirectorBranches(c *gin.Context) {
	claims := c.MustGet("claims").(*auth.Claims)

	branches, err := d.Branches.ListAll(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load branches"})
		return
	}

	if claims.IsNetworkOwner {
		c.JSON(http.StatusOK, gin.H{"branches": branches})
		return
	}
	for _, b := range branches {
		if b.ID == claims.BranchID {
			c.JSON(http.StatusOK, gin.H{"branches": []repository.Branch{b}})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"branches": []repository.Branch{}})
}

// AddGroupStudentRequest is the body for enrolling a student into a group.
type AddGroupStudentRequest struct {
	StudentID string `json:"student_id" binding:"required"`
}

// AddGroupStudent godoc
// @Summary		Enroll a student into a group
// @Tags			director,groups
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			groupId	path		string					true	"Group ID"
// @Param			request	body		AddGroupStudentRequest	true	"Student to enroll"
// @Success		201		{object}	map[string]string
// @Router			/director/groups/{groupId}/students [post]
func (d *Deps) AddGroupStudent(c *gin.Context) {
	var req AddGroupStudentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := d.Schedule.AddGroupStudent(c.Request.Context(), c.Param("groupId"), req.StudentID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to enroll student"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"status": "enrolled"})
}

// RemoveGroupStudent godoc
// @Summary		Remove a student from a group
// @Tags			director,groups
// @Produce		json
// @Security		BearerAuth
// @Param			groupId		path		string	true	"Group ID"
// @Param			studentId	path		string	true	"Student ID"
// @Success		200			{object}	map[string]string
// @Router			/director/groups/{groupId}/students/{studentId} [delete]
func (d *Deps) RemoveGroupStudent(c *gin.Context) {
	if err := d.Schedule.RemoveGroupStudent(c.Request.Context(), c.Param("groupId"), c.Param("studentId")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to remove student"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "removed"})
}

// ListGroupStudents godoc
// @Summary		A group's current roster
// @Tags			director,groups
// @Produce		json
// @Security		BearerAuth
// @Param			groupId	path		string	true	"Group ID"
// @Success		200		{object}	map[string]any
// @Router			/director/groups/{groupId}/students [get]
func (d *Deps) ListGroupStudents(c *gin.Context) {
	students, err := d.Schedule.ListGroupStudents(c.Request.Context(), c.Param("groupId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load roster"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"students": students})
}

func (d *Deps) GetMonthlyReports(c *gin.Context) {
	overdue, err := d.Students.ListOverdue(c.Request.Context(), middleware.EffectiveBranchID(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load reports"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"overdue_students": overdue})
}

// ListChurnAlerts godoc
// @Summary		Students at risk of dropping out
// @Description	Language students with a 3+ lesson absence streak, or care_and_prep children with a 7+ day absence streak. Currently always empty — nothing writes to churn_alerts yet (the detection job is a follow-up); this endpoint exists so the contract is stable once it does.
// @Tags			director
// @Produce		json
// @Security		BearerAuth
// @Success		200	{object}	map[string]any
// @Router			/director/churn-alerts [get]
func (d *Deps) ListChurnAlerts(c *gin.Context) {
	alerts, err := d.ChurnAlerts.ListByBranch(c.Request.Context(), middleware.EffectiveBranchID(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load churn alerts"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"alerts": alerts})
}

// ListBranchStudents godoc
// @Summary		List every student in the director's branch
// @Tags			director,students
// @Produce		json
// @Security		BearerAuth
// @Success		200	{object}	map[string]any
// @Router			/director/students [get]
func (d *Deps) ListBranchStudents(c *gin.Context) {
	students, err := d.Students.ListByBranch(c.Request.Context(), middleware.EffectiveBranchID(c))
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
	payments, err := d.Payments.ListByBranch(c.Request.Context(), middleware.EffectiveBranchID(c))
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

// GetMySchedule godoc
// @Summary		A student's own weekly schedule
// @Description	Student-only — every slot for any group they're enrolled in.
// @Tags			family
// @Produce		json
// @Security		BearerAuth
// @Success		200	{object}	map[string]any
// @Router			/family/schedule [get]
func (d *Deps) GetMySchedule(c *gin.Context) {
	claims := c.MustGet("claims").(*auth.Claims)
	slots, err := d.Schedule.ListByStudent(c.Request.Context(), claims.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load schedule"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"schedule": slots})
}

// GetMyQRCode godoc
// @Summary		A student's own scannable check-in QR code
// @Description	Student-only. Returns the stable qr_token plus a ready-to-display PNG data URL — shown on screen at the door kiosk.
// @Tags			family
// @Produce		json
// @Security		BearerAuth
// @Success		200	{object}	map[string]string
// @Router			/family/qr-code [get]
func (d *Deps) GetMyQRCode(c *gin.Context) {
	claims := c.MustGet("claims").(*auth.Claims)
	token, err := d.Students.GetQRToken(c.Request.Context(), claims.UserID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "student not found"})
		return
	}
	dataURL, err := qrcode.DataURL(token)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to render qr code"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"qr_token": token, "qr_image_base64": dataURL})
}

// GetTodayStatus godoc
// @Summary		Today's arrival/departure timing
// @Description	Student or parent. For a care_and_prep child, expected_end_at is checked_in_at + 3 hours (the fixed daily session length) — null for a language student, and null for either role before check-in.
// @Tags			family
// @Produce		json
// @Security		BearerAuth
// @Success		200	{object}	map[string]any
// @Router			/family/today-status [get]
func (d *Deps) GetTodayStatus(c *gin.Context) {
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

	today, err := d.Attendance.GetToday(c.Request.Context(), studentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load today's status"})
		return
	}

	var expectedEnd *time.Time
	if summary.CourseType == "care_and_prep" && today.CheckedInAt != nil {
		t := today.CheckedInAt.Add(3 * time.Hour)
		expectedEnd = &t
	}

	c.JSON(http.StatusOK, gin.H{
		"checked_in_at":   today.CheckedInAt,
		"checked_out_at":  today.CheckedOutAt,
		"expected_end_at": expectedEnd,
	})
}
