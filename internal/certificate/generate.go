// Package certificate generates a one-page PDF course-completion
// certificate with an embedded QR code that links to a public
// verification URL.
//
// Uses github.com/go-pdf/fpdf with an embedded DejaVu Sans Condensed
// TTF (Bitstream Vera-derived, free to embed/redistribute) so Cyrillic
// names/levels render correctly — this replaces the earlier hand-rolled
// PDF writer, which only supported base-14 Helvetica (Latin only).
//
// The embedded DejaVu font and the drawn "DE" badge are placeholders:
// once the organization's real brand font(s) (see frontend/FONTS.md)
// and logo are available, swap RegularFontTTF/BoldFontTTF below for
// the brand font bytes, and replace the drawn badge with
// pdf.ImageOptions(...) for a real logo file. Generate()'s signature
// does not need to change either way.
package certificate

import (
	"bytes"
	_ "embed"
	"fmt"
	"time"

	"github.com/go-pdf/fpdf"
	"rsc.io/qr"
)

//go:embed fonts/DejaVuSansCondensed.ttf
var regularFontTTF []byte

//go:embed fonts/DejaVuSansCondensed-Bold.ttf
var boldFontTTF []byte

type Certificate struct {
	StudentName string
	Level       string
	IssuedAt    time.Time
	VerifyURL   string // encoded into the QR code
}

// brand purple (#7c5cff), matching --accent across every frontend app.
const brandR, brandG, brandB = 124, 92, 255

// Generate renders the certificate PDF and returns its bytes.
func Generate(cert Certificate) ([]byte, error) {
	pdf := fpdf.New("L", "mm", "A4", "")
	pdf.SetTitle("Certificate of Completion", true)
	// This is a fully manual single-page layout — fpdf's default ~2cm
	// bottom-margin auto-page-break would otherwise silently spill the
	// QR caption (positioned near the bottom edge) onto a blank page 2.
	pdf.SetAutoPageBreak(false, 0)
	pdf.AddUTF8FontFromBytes("DejaVu", "", regularFontTTF)
	pdf.AddUTF8FontFromBytes("DejaVu", "B", boldFontTTF)
	pdf.AddPage()

	pageW, pageH := pdf.GetPageSize()

	// --- decorative border ---
	pdf.SetDrawColor(brandR, brandG, brandB)
	pdf.SetLineWidth(1)
	pdf.Rect(8, 8, pageW-16, pageH-16, "D")

	// --- placeholder logo badge (swap for a real image once available) ---
	pdf.SetFillColor(brandR, brandG, brandB)
	pdf.RoundedRect(18, 16, 16, 16, 3, "1234", "F")
	pdf.SetTextColor(255, 255, 255)
	pdf.SetFont("DejaVu", "B", 11)
	pdf.SetXY(18, 16)
	pdf.CellFormat(16, 16, "DE", "", 0, "CM", false, 0, "")

	pdf.SetTextColor(40, 40, 40)
	pdf.SetFont("DejaVu", "B", 11)
	pdf.SetXY(38, 20)
	pdf.CellFormat(0, 8, "DOS EDUCATION", "", 1, "L", false, 0, "")

	// --- title + body ---
	pdf.SetTextColor(20, 20, 20)
	pdf.SetFont("DejaVu", "B", 28)
	pdf.SetY(42)
	pdf.CellFormat(0, 14, "СЕРТИФИКАТ", "", 1, "C", false, 0, "")

	pdf.SetFont("DejaVu", "", 12)
	pdf.SetTextColor(90, 90, 90)
	pdf.CellFormat(0, 8, "Курсты сәтті аяқтағаны үшін беріледі", "", 1, "C", false, 0, "")

	pdf.Ln(8)
	pdf.SetFont("DejaVu", "B", 22)
	pdf.SetTextColor(20, 20, 20)
	pdf.CellFormat(0, 12, cert.StudentName, "", 1, "C", false, 0, "")

	pdf.SetFont("DejaVu", "", 13)
	pdf.SetTextColor(60, 60, 60)
	pdf.CellFormat(0, 9, fmt.Sprintf("\"%s\" деңгейін ойдағыдай аяқтады", cert.Level), "", 1, "C", false, 0, "")

	pdf.SetFont("DejaVu", "", 10)
	pdf.SetTextColor(120, 120, 120)
	pdf.CellFormat(0, 8, "Берілген күні: "+cert.IssuedAt.Format("2006-01-02"), "", 1, "C", false, 0, "")

	// --- QR code, drawn as filled squares (no external image codec
	// needed), bottom-right corner ---
	code, err := qr.Encode(cert.VerifyURL, qr.M)
	if err != nil {
		return nil, fmt.Errorf("encode qr: %w", err)
	}
	qrSizeMM := 26.0
	moduleSize := qrSizeMM / float64(code.Size)
	qrX, qrY := pageW-18-qrSizeMM, pageH-18-qrSizeMM

	pdf.SetFillColor(0, 0, 0)
	for y := 0; y < code.Size; y++ {
		for x := 0; x < code.Size; x++ {
			if !code.Black(x, y) {
				continue
			}
			pdf.Rect(qrX+float64(x)*moduleSize, qrY+float64(y)*moduleSize, moduleSize+0.05, moduleSize+0.05, "F")
		}
	}
	pdf.SetFont("DejaVu", "", 6)
	pdf.SetTextColor(120, 120, 120)
	pdf.SetXY(qrX-10, qrY+qrSizeMM+2)
	pdf.CellFormat(qrSizeMM+20, 4, "Растау үшін сканерлеңіз", "", 0, "C", false, 0, "")

	if pdf.Error() != nil {
		return nil, pdf.Error()
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
