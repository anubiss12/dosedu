package handlers

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/dosedu/lms/internal/auth"
	"github.com/dosedu/lms/internal/telegram"
)

type QRCheckInRequest struct {
	StudentID string `json:"student_id" binding:"required"`
}

// QRCheckIn handles POST /teacher/attendance/qr-checkin — a teacher (or
// an unattended kiosk using a teacher/service account) scans a
// student's QR code. Records the timestamp and, best-effort, pings the
// parent on Telegram per the spec's "QR ескертулер" requirement.
func (d *Deps) QRCheckIn(c *gin.Context) {
	claims := c.MustGet("claims").(*auth.Claims)

	var req QRCheckInRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	checkedAt, err := d.Attendance.MarkQRCheckIn(c.Request.Context(), req.StudentID, claims.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to record check-in"})
		return
	}

	go func() {
		ctx := context.Background()
		summary, err := d.Students.GetSummary(ctx, req.StudentID)
		if err != nil {
			return
		}
		chatID, err := d.Parents.FindParentChatIDByStudent(ctx, req.StudentID)
		if err != nil {
			return
		}
		_ = d.Notify.NotifyQRCheckIn(ctx, chatID, summary.FullName, true, checkedAt)
	}()

	c.JSON(http.StatusOK, gin.H{"status": "checked_in", "student_id": req.StudentID, "at": checkedAt})
}

// QRCheckOut handles POST /teacher/attendance/qr-checkout — the
// departure-side scan, symmetric with QRCheckIn. Used for a
// care_and_prep child's 3-hour session so the parent portal can show
// an actual "picked up at" time instead of just an open-ended timer.
func (d *Deps) QRCheckOut(c *gin.Context) {
	var req QRCheckInRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := d.Attendance.MarkQRCheckOut(c.Request.Context(), req.StudentID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to record check-out"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "checked_out", "student_id": req.StudentID})
}

// TelegramWebhook handles POST /public/telegram/webhook — Telegram's
// servers push bot updates here. Protected by comparing the
// X-Telegram-Bot-Api-Secret-Token header against the configured secret
// (set via setWebhook's secret_token param when registering the URL
// with Telegram), since this path must otherwise stay unauthenticated.
func (d *Deps) TelegramWebhook(c *gin.Context) {
	if d.TelegramWebhookSecret != "" {
		if c.GetHeader("X-Telegram-Bot-Api-Secret-Token") != d.TelegramWebhookSecret {
			c.Status(http.StatusUnauthorized)
			return
		}
	}

	var update telegram.Update
	if err := c.ShouldBindJSON(&update); err != nil {
		// Telegram doesn't care about the response body, just 200 vs not.
		c.Status(http.StatusOK)
		return
	}

	d.TelegramHook.Handle(c.Request.Context(), update)
	c.Status(http.StatusOK)
}
