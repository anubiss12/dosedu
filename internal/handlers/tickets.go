package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/dosedu/lms/internal/auth"
)

type CreateTicketRequest struct {
	Subject string `json:"subject" binding:"required"`
	Message string `json:"message" binding:"required"`
	// BranchID is required for parent/student callers, who have no
	// branch on their own JWT claims (their student record does, but
	// that's resolved server-side in a fuller implementation — for now
	// family callers must pass it explicitly).
	BranchID string `json:"branch_id"`
}

// CreateTicket godoc
// @Summary		Open a support ticket
// @Description	Opens a new communication thread between teacher, director, and parent. Available to teacher/director/family roles.
// @Tags			tickets
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			request	body		CreateTicketRequest	true	"Subject + first message"
// @Success		201		{object}	map[string]string
// @Router			/director/tickets [post]
func (d *Deps) CreateTicket(c *gin.Context) {
	claims := c.MustGet("claims").(*auth.Claims)

	var req CreateTicketRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	branchID := claims.BranchID
	if branchID == "" {
		branchID = req.BranchID
	}
	if branchID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "branch_id is required"})
		return
	}

	id, err := d.Tickets.Create(c.Request.Context(), branchID, req.Subject, string(claims.Role), claims.UserID, req.Message)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create ticket"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"id": id})
}

// ListTickets godoc
// @Summary		List branch tickets
// @Description	The director/teacher branch-wide support inbox.
// @Tags			tickets
// @Produce		json
// @Security		BearerAuth
// @Success		200	{object}	map[string]any
// @Router			/director/tickets [get]
func (d *Deps) ListTickets(c *gin.Context) {
	claims := c.MustGet("claims").(*auth.Claims)
	tickets, err := d.Tickets.ListByBranch(c.Request.Context(), claims.BranchID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load tickets"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"tickets": tickets})
}

// GetTicketThread handles GET /{role}/tickets/:id/messages — the full
// message thread for one ticket.
func (d *Deps) GetTicketThread(c *gin.Context) {
	ticketID := c.Param("id")
	messages, err := d.Tickets.ListMessages(c.Request.Context(), ticketID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load thread"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"messages": messages})
}

type ReplyTicketRequest struct {
	Message string `json:"message" binding:"required"`
}

// ReplyTicket handles POST /{role}/tickets/:id/messages — adds a reply
// and reopens the ticket to "in_progress" if it was closed.
func (d *Deps) ReplyTicket(c *gin.Context) {
	claims := c.MustGet("claims").(*auth.Claims)
	ticketID := c.Param("id")

	var req ReplyTicketRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := d.Tickets.AddMessage(c.Request.Context(), ticketID, string(claims.Role), claims.UserID, req.Message); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to add reply"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

type CloseTicketRequest struct {
	Status string `json:"status" binding:"required,oneof=open in_progress closed"`
}

// SetTicketStatus handles PATCH /{teacher,director}/tickets/:id/status
func (d *Deps) SetTicketStatus(c *gin.Context) {
	claims := c.MustGet("claims").(*auth.Claims)
	ticketID := c.Param("id")

	var req CloseTicketRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := d.Tickets.SetStatus(c.Request.Context(), ticketID, claims.BranchID, req.Status); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "ticket not found in your branch"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": req.Status})
}
