package handlers

import (
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"github.com/dosedu/lms/internal/auth"
	"github.com/dosedu/lms/internal/middleware"
)

// SetupRouter wires every subdomain's routes to d's methods. In
// production, Nginx routes admin./director./teacher./student.dosedu.kz
// to this same Gin instance (or to separate deployments sharing this
// codebase). RequireSubdomain is a defense-in-depth check on top of
// each group's role restriction: a token's role must also match the
// Host header's subdomain.
func SetupRouter(d *Deps) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())

	// Interactive API docs, generated from the @-annotations on handlers
	// by `swag init` (see Dockerfile). Visit /swagger/index.html.
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	sm := d.Sessions
	subdomain := middleware.RequireSubdomain()

	// ---- dosedu.kz (public landing) ----
	public := r.Group("/public")
	{
		public.POST("/leads", d.CreateLead)
		public.POST("/auth/login", d.Login)
		public.POST("/telegram/webhook", d.TelegramWebhook)
		public.GET("/certificates/verify/:token", d.VerifyCertificate)
		public.GET("/placement-test/questions", d.GetPlacementQuestions)
		public.POST("/placement-test/submit", d.SubmitPlacementTest)
		public.GET("/branches", d.ListPublicBranches)
	}

	// ---- admin.dosedu.kz (super admin — full network access) ----
	superAdmin := r.Group("/s-admin", middleware.RequireAuth(sm, auth.RoleSuperAdmin), subdomain)
	{
		superAdmin.GET("/health", d.SystemHealth)
		superAdmin.POST("/branches", d.CreateBranch)
		superAdmin.GET("/branches", d.ListBranches)
		superAdmin.POST("/directors", d.CreateDirector)
		superAdmin.GET("/directors", d.ListDirectors)
		superAdmin.POST("/directors/:id/force-reset-password", d.ForceResetDirectorPassword)
		superAdmin.DELETE("/branches/:id", d.DeleteBranch)
		superAdmin.GET("/logs", d.ListSystemLogs)
		superAdmin.POST("/impersonate/:role/:id", d.Impersonate)
		superAdmin.GET("/impersonate/log", d.ListImpersonations)
	}

	// ---- director.dosedu.kz (single branch OR whole network) ----
	director := r.Group("/director", middleware.RequireAuth(sm, auth.RoleDirector), subdomain)
	{
		director.GET("/leads", d.ListLeads)
		director.PATCH("/leads/:id/stage", d.MoveLeadStage)
		director.POST("/teachers", d.CreateTeacher)
		director.GET("/teachers", d.ListTeachers)
		director.POST("/teachers/:id/force-reset-password", d.ForceResetTeacherPassword)
		director.GET("/students", d.ListBranchStudents)
		director.GET("/payments", d.ListBranchPayments)
		director.GET("/schedule", d.GetScheduleGrid)
		director.POST("/schedule", d.CreateScheduleSlot)
		director.GET("/rooms", d.ListRooms)
		director.POST("/rooms", d.CreateRoom)
		director.GET("/branches", d.ListDirectorBranches)
		director.GET("/groups", d.ListGroups)
		director.POST("/groups", d.CreateGroup)
		director.GET("/groups/:groupId/students", d.ListGroupStudents)
		director.POST("/groups/:groupId/students", d.AddGroupStudent)
		director.DELETE("/groups/:groupId/students/:studentId", d.RemoveGroupStudent)
		director.GET("/reports", d.GetMonthlyReports)
		director.GET("/churn-alerts", d.ListChurnAlerts)
		director.GET("/tickets", d.ListTickets)
		director.POST("/tickets", d.CreateTicket)
		director.GET("/tickets/:id/messages", d.GetTicketThread)
		director.POST("/tickets/:id/messages", d.ReplyTicket)
		director.PATCH("/tickets/:id/status", d.SetTicketStatus)
	}

	// ---- teacher.dosedu.kz (language teachers + mad/prodlenka) ----
	teacher := r.Group("/teacher", middleware.RequireAuth(sm, auth.RoleTeacher), subdomain)
	{
		teacher.GET("/schedule", d.GetTeacherSchedule)
		teacher.POST("/attendance", d.MarkAttendance)
		teacher.GET("/groups/:groupId/gradebook", d.ListGradebook)
		teacher.POST("/attendance/qr-checkin", d.QRCheckIn)
		teacher.POST("/attendance/qr-checkout", d.QRCheckOut)
		teacher.POST("/payments/:studentId/confirm", d.ConfirmPayment)
		teacher.POST("/tests/:level/upload", d.UploadLevelTest)
		teacher.GET("/tests/uploads", d.ListTestUploads)
		teacher.GET("/questions", d.ListQuestionBank)
		teacher.POST("/groups/:groupId/test-assignments", d.CreateTestAssignment)
		teacher.POST("/daily-logs", d.UpsertDailyLog)
		teacher.POST("/certificates", d.IssueCertificate)
		teacher.GET("/tickets", d.ListTickets)
		teacher.POST("/tickets", d.CreateTicket)
		teacher.GET("/tickets/:id/messages", d.GetTicketThread)
		teacher.POST("/tickets/:id/messages", d.ReplyTicket)
	}

	// ---- student.dosedu.kz (student / parent, 4-digit PIN login) ----
	family := r.Group("/family", middleware.RequireAuth(sm, auth.RoleStudent, auth.RoleParent), subdomain)
	{
		family.GET("/progress", d.GetStudentProgress)
		family.GET("/balance", d.GetBalance)
		family.GET("/today-status", d.GetTodayStatus)
		family.GET("/schedule", middleware.RequireRole(auth.RoleStudent), d.GetMySchedule)
		family.GET("/qr-code", middleware.RequireRole(auth.RoleStudent), d.GetMyQRCode)
		family.GET("/report/pdf", d.GetMonthlyReportPDF)
		family.POST("/report/telegram", d.SendMonthlyReportTelegram)
		family.GET("/certificates", d.ListMyCertificates)
		family.GET("/certificates/:id/pdf", d.GetCertificatePDF)
		family.GET("/daily-logs", middleware.RequireRole(auth.RoleStudent, auth.RoleParent), d.ListDailyLogs) // care_and_prep: attendance + note + homework

		// Every test-related endpoint is student-only — a parent is
		// never allowed to reach /family/practice/* or
		// /family/test-assignments/*, read-only or not.
		family.GET("/practice/questions", middleware.RequireRole(auth.RoleStudent), d.GetPracticeQuestions)
		family.POST("/practice/submit", middleware.RequireRole(auth.RoleStudent), d.SubmitPracticeTest)
		family.GET("/practice/history", middleware.RequireRole(auth.RoleStudent), d.ListPracticeHistory)
		family.GET("/test-assignments/active", middleware.RequireRole(auth.RoleStudent), d.ListActiveTestAssignments)
		family.GET("/test-assignments/:id/questions", middleware.RequireRole(auth.RoleStudent), d.GetTestAssignmentQuestions)
		family.POST("/test-assignments/:id/submit", middleware.RequireRole(auth.RoleStudent), d.SubmitTestAssignment)
		family.POST("/tests/explain", middleware.RequireRole(auth.RoleStudent), d.ExplainQuizMistake)
		family.POST("/tickets", d.CreateTicket)
		family.GET("/tickets/:id/messages", d.GetTicketThread)
		family.POST("/tickets/:id/messages", d.ReplyTicket)
	}

	return r
}
