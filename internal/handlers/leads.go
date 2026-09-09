package handlers

import (
	"crypto/rand"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/dosedu/lms/internal/auth"
	"github.com/dosedu/lms/internal/middleware"
	"github.com/dosedu/lms/internal/repository"
)

type CreateLeadRequest struct {
	FullName        string `json:"full_name" binding:"required"`
	Phone           string `json:"phone" binding:"required"`
	LevelTestResult string `json:"level_test_result"`
	Subject         string `json:"subject" binding:"omitempty,oneof=english chinese"`
	BranchID        string `json:"branch_id"`
}

// CreateLead godoc
// @Summary		Submit a lead
// @Description	Public "get in touch" form from the dosedu.kz landing page — creates a CRM lead in the "new" Kanban stage. Rate-limited at the Nginx layer.
// @Tags			public
// @Accept			json
// @Produce		json
// @Param			request	body		CreateLeadRequest	true	"Lead details"
// @Success		201		{object}	map[string]any
// @Router			/public/leads [post]
func (d *Deps) CreateLead(c *gin.Context) {
	var req CreateLeadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	id, err := d.Leads.Insert(c.Request.Context(), req.BranchID, req.FullName, req.Phone, req.LevelTestResult, req.Subject)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not save lead"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":      id,
		"status":  "received",
		"message": "Thank you, our team will contact you shortly.",
	})
}

// ListPublicBranches godoc
// @Summary		Branches for the public lead/placement-test form
// @Description	Unauthenticated, id+name only — populates the landing page's branch picker.
// @Tags			public
// @Produce		json
// @Success		200	{object}	map[string]any
// @Router			/public/branches [get]
func (d *Deps) ListPublicBranches(c *gin.Context) {
	branches, err := d.Branches.ListPublic(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load branches"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"branches": branches})
}

type MoveLeadStageRequest struct {
	Stage string `json:"stage" binding:"required,oneof=new contacted trial_scheduled paid lost"`
}

// MoveLeadStage godoc
// @Summary		Move a lead's Kanban stage
// @Description	Transitions a lead between CRM stages. Moving into "paid" auto-provisions a student account (CRM->LMS funnel) and returns one-time login credentials in the response.
// @Tags			director,crm
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			id		path		string					true	"Lead ID"
// @Param			request	body		MoveLeadStageRequest	true	"New stage"
// @Success		200		{object}	map[string]any
// @Failure		404		{object}	map[string]string
// @Router			/director/leads/{id}/stage [patch]
func (d *Deps) MoveLeadStage(c *gin.Context) {
	leadID := c.Param("id")

	var req MoveLeadStageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	effectiveBranch := middleware.EffectiveBranchID(c)
	if err := d.Leads.UpdateStage(c.Request.Context(), leadID, effectiveBranch, req.Stage); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "lead not found in your branch"})
		return
	}

	resp := gin.H{"id": leadID, "stage": req.Stage}

	if req.Stage == "paid" {
		lead, err := d.Leads.GetByID(c.Request.Context(), leadID, effectiveBranch)
		if err == nil {
			loginCode, pin, studentID, provErr := d.provisionStudentFromLead(c, lead.BranchID, lead)
			if provErr == nil {
				resp["student_created"] = true
				resp["student_id"] = studentID
				resp["login_code"] = loginCode
				resp["temporary_password"] = pin
				resp["credentials_notice"] = "Бұл құпия сөз тек осы жауапта бір рет көрсетіледі — оны отбасына дереу жіберіңіз."
			}
		}
	}

	c.JSON(http.StatusOK, resp)
}

// provisionStudentFromLead generates a login code + 4-digit PIN,
// hashes the PIN per the student password policy, and creates the
// student row. Returns the plaintext login/PIN once (never stored in
// plaintext) so the caller can relay them to the family.
func (d *Deps) provisionStudentFromLead(c *gin.Context, branchID string, lead *repository.Lead) (loginCode, pin, studentID string, err error) {
	loginCode = generateLoginCode()

	pin, err = generatePIN()
	if err != nil {
		return "", "", "", err
	}

	hash, err := auth.HashPassword(auth.RoleStudent, pin)
	if err != nil {
		return "", "", "", err
	}

	subject := lead.Subject
	if subject == "" {
		subject = "english" // fallback for a lead that never took the placement test
	}

	studentID, err = d.Students.CreateFromLead(c.Request.Context(), branchID, lead.FullName, loginCode, hash, subject)
	if err != nil {
		return "", "", "", err
	}

	return loginCode, pin, studentID, nil
}

func generateLoginCode() string {
	return "std-" + uuid.NewString()[:8]
}

func generatePIN() (string, error) {
	b := make([]byte, 2)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	n := (int(b[0])<<8 | int(b[1])) % 10000
	return fmt.Sprintf("%04d", n), nil
}

// ListLeads handles GET /director/leads — the full board grouped by
// stage, for rendering the Kanban columns on the frontend.
func (d *Deps) ListLeads(c *gin.Context) {
	claims := c.MustGet("claims").(*auth.Claims)

	columns, err := d.Leads.ListByBranchGroupedByStage(c.Request.Context(), middleware.EffectiveBranchID(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load leads"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"branch_id": claims.BranchID,
		"columns":   columns,
	})
}
