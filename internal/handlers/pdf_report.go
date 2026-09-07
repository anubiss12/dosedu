package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/dosedu/lms/internal/auth"
	"github.com/dosedu/lms/internal/pdfreport"
)

// currentMonthRange returns [first day, last day] of the current month,
// used as the default period for the one-tap monthly report when the
// caller doesn't specify one explicitly.
func currentMonthRange() (time.Time, time.Time) {
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	end := start.AddDate(0, 1, -1)
	return start, end
}

// buildMonthlyReport gathers the data and renders the PDF bytes for one
// student, shared by both the direct-download and Telegram-send routes.
func (d *Deps) buildMonthlyReport(ctx context.Context, studentID string) ([]byte, string, error) {
	summary, err := d.Students.GetSummary(ctx, studentID)
	if err != nil {
		return nil, "", err
	}

	start, end := currentMonthRange()
	stats, err := d.Attendance.GetMonthlySummary(ctx, studentID, start, end)
	if err != nil {
		return nil, "", err
	}

	pdfBytes := pdfreport.Generate(pdfreport.MonthlyReport{
		StudentName:  summary.FullName,
		Level:        summary.Level,
		PeriodStart:  start,
		PeriodEnd:    end,
		PresentCount: stats.PresentCount,
		AbsentCount:  stats.AbsentCount,
		ExcusedCount: stats.ExcusedCount,
		AverageGrade: stats.AverageGrade,
		GeneratedAt:  time.Now(),
	})

	monthLabel := start.Format("2006-01")
	return pdfBytes, monthLabel, nil
}

// GetMonthlyReportPDF handles GET /family/report/pdf — the "бір
// батырмамен PDF" download button in the parent/student cabinet.
func (d *Deps) GetMonthlyReportPDF(c *gin.Context) {
	claims := c.MustGet("claims").(*auth.Claims)

	studentID := c.Query("student_id")
	if studentID == "" {
		resolved, err := d.resolveStudentID(c.Request.Context(), claims)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		studentID = resolved
	}

	pdfBytes, monthLabel, err := d.buildMonthlyReport(c.Request.Context(), studentID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "could not build report"})
		return
	}

	filename := "report_" + monthLabel + ".pdf"
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	c.Data(http.StatusOK, "application/pdf", pdfBytes)
}

// SendMonthlyReportTelegram handles POST /family/report/telegram — sends
// the same PDF straight to the parent's linked Telegram chat, per the
// spec's "оны тікелей Telegram арқылы да алу мүмкіндігі" requirement.
func (d *Deps) SendMonthlyReportTelegram(c *gin.Context) {
	claims := c.MustGet("claims").(*auth.Claims)
	ctx := c.Request.Context()

	studentID := c.Query("student_id")
	if studentID == "" {
		resolved, err := d.resolveStudentID(ctx, claims)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		studentID = resolved
	}

	pdfBytes, monthLabel, err := d.buildMonthlyReport(ctx, studentID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "could not build report"})
		return
	}

	chatID, err := d.Parents.FindParentChatIDByStudent(ctx, studentID)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{
			"error": "parent has not linked Telegram yet — use /start <phone> with the bot first",
		})
		return
	}

	summary, err := d.Students.GetSummary(ctx, studentID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "student not found"})
		return
	}

	if err := d.Notify.SendMonthlyPDFReport(ctx, chatID, summary.FullName, pdfBytes, monthLabel); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to send via Telegram"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "sent", "month": monthLabel})
}
