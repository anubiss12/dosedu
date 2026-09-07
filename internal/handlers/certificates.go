package handlers

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/dosedu/lms/internal/auth"
	"github.com/dosedu/lms/internal/certificate"
)

type IssueCertificateRequest struct {
	StudentID string `json:"student_id" binding:"required"`
	Level     string `json:"level" binding:"required"`
}

// IssueCertificate godoc
// @Summary		Issue a completion certificate
// @Description	Teacher marks a student as having completed a level/course, auto-generating a PDF certificate with a unique QR verification code.
// @Tags			teacher,certificates
// @Accept			json
// @Produce		json
// @Security		BearerAuth
// @Param			request	body		IssueCertificateRequest	true	"Student and level"
// @Success		201		{object}	map[string]string
// @Router			/teacher/certificates [post]
func (d *Deps) IssueCertificate(c *gin.Context) {
	claims := c.MustGet("claims").(*auth.Claims)

	var req IssueCertificateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	id, qrToken, err := d.Certificates.Issue(c.Request.Context(), claims.BranchID, req.StudentID, req.Level, claims.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to issue certificate"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"id": id, "qr_token": qrToken})
}

// buildVerifyURL constructs the public verification link encoded into
// the certificate's QR code. It always points at the root domain
// (dosedu.kz / dosedu.local), even when the issuing request came from
// a subdomain like teacher.dosedu.kz, since verification is meant to
// be checked from the main public site.
func buildVerifyURL(c *gin.Context, qrToken string) string {
	scheme := "https"
	if c.Request.TLS == nil {
		scheme = "http"
	}

	host := c.Request.Host
	parts := strings.Split(host, ".")
	if len(parts) > 2 {
		host = strings.Join(parts[1:], ".") // drop the subdomain label
	}

	return scheme + "://" + host + "/api/public/certificates/verify/" + qrToken
}

// GetCertificatePDF handles GET /family/certificates/:id/pdf — the
// student/parent downloads their PDF certificate.
func (d *Deps) GetCertificatePDF(c *gin.Context) {
	id := c.Param("id")

	cert, err := d.Certificates.GetByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "certificate not found"})
		return
	}

	pdfBytes, err := certificate.Generate(certificate.Certificate{
		StudentName: cert.StudentName,
		Level:       cert.Level,
		IssuedAt:    cert.IssuedAt,
		VerifyURL:   buildVerifyURL(c, cert.QRToken),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to render certificate"})
		return
	}

	c.Header("Content-Disposition", `attachment; filename="certificate.pdf"`)
	c.Data(http.StatusOK, "application/pdf", pdfBytes)
}

// ListMyCertificates handles GET /family/certificates — the student's
// portfolio of earned certificates.
func (d *Deps) ListMyCertificates(c *gin.Context) {
	claims := c.MustGet("claims").(*auth.Claims)
	studentID, err := d.resolveStudentID(c.Request.Context(), claims)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	certs, err := d.Certificates.ListByStudent(c.Request.Context(), studentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load certificates"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"certificates": certs})
}

// VerifyCertificate godoc
// @Summary		Verify a certificate (public, QR-scan target)
// @Description	What the QR code on a physical/PDF certificate points to. No auth required.
// @Tags			public,certificates
// @Produce		json
// @Param			token	path		string	true	"QR verification token"
// @Success		200		{object}	map[string]any
// @Failure		404		{object}	map[string]any
// @Router			/public/certificates/verify/{token} [get]
func (d *Deps) VerifyCertificate(c *gin.Context) {
	token := c.Param("token")

	cert, err := d.Certificates.GetByQRToken(c.Request.Context(), token)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"valid": false, "error": "certificate not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"valid":        true,
		"student_name": cert.StudentName,
		"level":        cert.Level,
		"issued_at":    cert.IssuedAt.Format(time.RFC3339),
	})
}
