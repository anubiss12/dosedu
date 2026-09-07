package handlers

import (
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"github.com/dosedu/lms/internal/auth"
	"github.com/dosedu/lms/internal/middleware"
)

// SetupRouter wires every subdomain's routes to d's methods. In production,
// Nginx routes s-admin./director./teacher.dosedu.kz to this same Gin
// instance (or to separate deployments sharing this codebase).
func SetupRouter(d *Deps) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())

	// Interactive API docs, generated from the @-annotations on handlers
	// by `swag init` (see Dockerfile). Visit /swagger/index.html.
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	sm := d.Sessions

	// ---- dosedu.kz (public landing) ----
	public := r.Group("/public")
	{
		public.POST("/leads", d.CreateLead)
		public.POST("/auth/login", d.Login)
		public.POST("/telegram/webhook", d.TelegramWebhook)
		public.GET("/certificates/verify/:token", d.VerifyCertificate)
	}

	// ---- s-admin.dosedu.kz ----
	superAdmin := r.Group("/s-admin", middleware.RequireAuth(sm, auth.RoleSuperAdmin))
	{
		superAdmin.GET("/health", d.SystemHealth)
		superAdmin.POST("/branches", d.CreateBranch)
		superAdmin.GET("/branches", d.ListBranches)
		superAdmin.POST("/directors", d.CreateDirector)
		superAdmin.GET("/directors", d.ListDirectors)
		superAdmin.GET("/logs", d.ListSystemLogs)
	}

	// ---- director.dosedu.kz ----
	director := r.Group("/director", middleware.RequireAuth(sm, auth.RoleDirector))
	{
		director.GET("/leads", d.ListLeads)
		director.PATCH("/leads/:id/stage", d.MoveLeadStage)
		director.POST("/teachers", d.CreateTeacher)
		director.GET("/teachers", d.ListTeachers)
		director.GET("/students", d.ListBranchStudents)
		director.GET("/payments", d.ListBranchPayments)
		director.GET("/schedule", d.GetScheduleGrid)
		director.POST("/schedule", d.CreateScheduleSlot)
		director.GET("/rooms", d.ListRooms)
		director.POST("/rooms", d.CreateRoom)
		director.GET("/groups", d.ListGroups)
		director.POST("/groups", d.CreateGroup)
		director.GET("/reports", d.GetMonthlyReports)
		director.GET("/tickets", d.ListTickets)
		director.POST("/tickets", d.CreateTicket)
		director.GET("/tickets/:id/messages", d.GetTicketThread)
		director.POST("/tickets/:id/messages", d.ReplyTicket)
		director.PATCH("/tickets/:id/status", d.SetTicketStatus)
	}

	// ---- teacher.dosedu.kz ----
	teacher := r.Group("/teacher", middleware.RequireAuth(sm, auth.RoleTeacher))
	{
		teacher.GET("/schedule", d.GetTeacherSchedule)
		teacher.POST("/attendance", d.MarkAttendance)
		teacher.GET("/groups/:groupId/gradebook", d.ListGradebook)
		teacher.POST("/attendance/qr-checkin", d.QRCheckIn)
		teacher.POST("/payments/:studentId/confirm", d.ConfirmPayment)
		teacher.POST("/tests/:level/upload", d.UploadLevelTest)
		teacher.GET("/tests/uploads", d.ListTestUploads)
		teacher.POST("/certificates", d.IssueCertificate)
		teacher.GET("/tickets", d.ListTickets)
		teacher.POST("/tickets", d.CreateTicket)
		teacher.GET("/tickets/:id/messages", d.GetTicketThread)
		teacher.POST("/tickets/:id/messages", d.ReplyTicket)
	}

	// ---- student / parent (4-digit PIN login, e.g. app.dosedu.kz) ----
	family := r.Group("/family", middleware.RequireAuth(sm, auth.RoleStudent, auth.RoleParent))
	{
		family.GET("/progress", d.GetStudentProgress)
		family.GET("/balance", d.GetBalance)
		family.GET("/report/pdf", d.GetMonthlyReportPDF)
		family.POST("/report/telegram", d.SendMonthlyReportTelegram)
		family.GET("/certificates", d.ListMyCertificates)
		family.GET("/certificates/:id/pdf", d.GetCertificatePDF)
		family.POST("/practice/attempts", d.RecordPracticeAttempt)
		family.GET("/practice/attempts", d.ListPracticeAttempts)
		family.POST("/profile/language", d.SetStudentLanguage)
		family.GET("/questions", d.ListQuestions)
		family.POST("/quiz/submit", d.SubmitQuiz)
		family.POST("/quiz/explain", d.ExplainQuizMistake)
		family.POST("/tickets", d.CreateTicket)
		family.GET("/tickets/:id/messages", d.GetTicketThread)
		family.POST("/tickets/:id/messages", d.ReplyTicket)
	}

	return r
}
