package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/dosedu/lms/internal/auth"
)

// Impersonate godoc
// @Summary		Sign in as any user, incognito
// @Description	Super-admin-only "stealth mode": mints a fully valid session token for the target user. Never written to system_logs and never visible to the impersonated user or to any director/teacher/parent — recorded only in a private, super-admin-only audit trail (impersonation_log).
// @Tags			s-admin,impersonation
// @Produce		json
// @Security		BearerAuth
// @Param			role	path		string	true	"director, teacher, parent, or student"
// @Param			id		path		string	true	"Target user ID"
// @Success		200		{object}	map[string]string
// @Router			/s-admin/impersonate/{role}/{id} [post]
func (d *Deps) Impersonate(c *gin.Context) {
	claims := c.MustGet("claims").(*auth.Claims)

	targetRole := auth.Role(c.Param("role"))
	targetID := c.Param("id")

	target, err := d.Impersonation.FindTarget(c.Request.Context(), targetRole, targetID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	subject := ""
	if target.Subject != nil {
		subject = *target.Subject
	}
	token, err := d.Sessions.IssueSession(c.Request.Context(), target.UserID, targetRole, target.BranchID, subject, target.IsNetworkOwner)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create session"})
		return
	}

	// Deliberately NOT written to system_logs (visible to other staff)
	// — only this private, super-admin-only trail.
	_ = d.Impersonation.LogPrivate(c.Request.Context(), claims.UserID, targetRole, targetID)

	c.JSON(http.StatusOK, gin.H{"token": token, "role": targetRole})
}

// ListImpersonations godoc
// @Summary		The super-admin's own impersonation audit trail
// @Description	Never exposed to any other role — this is the accountability record behind the otherwise-invisible impersonation feature.
// @Tags			s-admin,impersonation
// @Produce		json
// @Security		BearerAuth
// @Success		200	{object}	map[string]any
// @Router			/s-admin/impersonate/log [get]
func (d *Deps) ListImpersonations(c *gin.Context) {
	logs, err := d.Impersonation.ListRecent(c.Request.Context(), 100)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load log"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"log": logs})
}
