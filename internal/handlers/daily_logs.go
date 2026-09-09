package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/dosedu/lms/internal/auth"
)

type UpsertDailyLogRequest struct {
	StudentID        string `json:"student_id" binding:"required"`
	Date             string `json:"date" binding:"required"` // YYYY-MM-DD
	AttendanceStatus string `json:"attendance_status" binding:"required,oneof=present absent excused"`
	TeacherNote      string `json:"teacher_note"`
	HomeworkStatus   string `json:"homework_status" binding:"omitempty,oneof=done not_done partial n_a"`
}

// UpsertDailyLog godoc
// @Summary		Record a CARE_AND_PREP child's daily log
// @Description	Teacher-only (mad/prodlenka subject). Attendance + a free-text note + homework checklist status, one row per student per day — what the parent's dashboard reads. Re-submitting the same student+date corrects the earlier entry.
// @Tags			teacher,daily-logs
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			request	body		UpsertDailyLogRequest	true	"Daily log entry"
// @Success		200		{object}	map[string]string
// @Router			/teacher/daily-logs [post]
func (d *Deps) UpsertDailyLog(c *gin.Context) {
	// Role is already enforced by the /teacher route group's
	// RequireAuth(sm, auth.RoleTeacher) — only the subject check below
	// (mad/prodlenka vs english/chinese) is specific to this endpoint.
	claims := c.MustGet("claims").(*auth.Claims)
	if claims.Subject != "mad" && claims.Subject != "prodlenka" {
		c.JSON(http.StatusForbidden, gin.H{"error": "daily logs are only for mad/prodlenka teachers"})
		return
	}

	var req UpsertDailyLogRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	homeworkStatus := req.HomeworkStatus
	if homeworkStatus == "" {
		homeworkStatus = "n_a"
	}

	date, err := parseDate(req.Date)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid date format, expected YYYY-MM-DD"})
		return
	}

	id, err := d.DailyLogs.Upsert(c.Request.Context(), req.StudentID, date.Format("2006-01-02"), req.AttendanceStatus, req.TeacherNote, homeworkStatus, claims.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save daily log"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id})
}

// ListDailyLogs godoc
// @Summary		A CARE_AND_PREP child's daily-log history
// @Description	Parent/student-scoped. What a MAD/prodlenka parent sees instead of any test/level data: attendance, teacher notes, homework status.
// @Tags			family,daily-logs
// @Produce		json
// @Security		BearerAuth
// @Success		200	{object}	map[string]any
// @Router			/family/daily-logs [get]
func (d *Deps) ListDailyLogs(c *gin.Context) {
	claims := c.MustGet("claims").(*auth.Claims)
	studentID, err := d.resolveStudentID(c.Request.Context(), claims)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	logs, err := d.DailyLogs.ListByStudent(c.Request.Context(), studentID, 60)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load daily logs"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"logs": logs})
}
