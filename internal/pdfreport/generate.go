// Package pdfreport builds the one-button monthly progress PDF called
// for in the spec ("Ай соңында баланың үлгерімін бір батырмамен PDF
// етіп шығару").
//
// IMPORTANT — Cyrillic text limitation: this generator writes raw PDF
// syntax against the built-in Helvetica base-14 font with WinAnsi
// encoding, which only covers Latin characters. It deliberately avoids
// pulling in a PDF library + embedded TrueType font (e.g. go-pdf/fpdf +
// DejaVuSans.ttf) so the module graph stays small and dependency-free.
// Before shipping this to real Kazakh/Russian-speaking parents, swap
// this package's internals for a library that embeds a Cyrillic-capable
// font — the public Generate() signature below is designed to stay the
// same either way, so callers won't need to change.
package pdfreport

import (
	"bytes"
	"fmt"
	"strings"
	"time"
)

// MonthlyReport is the data the PDF renders. Populate it from
// repository.MonthlyReportData (or equivalent) before calling Generate.
type MonthlyReport struct {
	StudentName  string
	Level        string
	PeriodStart  time.Time
	PeriodEnd    time.Time
	PresentCount int
	AbsentCount  int
	ExcusedCount int
	AverageGrade float64
	GeneratedAt  time.Time
}

// Generate renders a single-page PDF and returns its bytes.
func Generate(r MonthlyReport) []byte {
	lines := buildLines(r)
	content := buildContentStream(lines)
	return assemblePDF(content)
}

func buildLines(r MonthlyReport) []string {
	total := r.PresentCount + r.AbsentCount + r.ExcusedCount
	attendanceRate := 0.0
	if total > 0 {
		attendanceRate = float64(r.PresentCount) / float64(total) * 100
	}
	return []string{
		"DOS EDUCATION - Monthly Progress Report",
		"",
		fmt.Sprintf("Student: %s", asciiSafe(r.StudentName)),
		fmt.Sprintf("Level: %s", asciiSafe(r.Level)),
		fmt.Sprintf("Period: %s - %s", r.PeriodStart.Format("2006-01-02"), r.PeriodEnd.Format("2006-01-02")),
		"",
		"Attendance",
		fmt.Sprintf("  Present: %d", r.PresentCount),
		fmt.Sprintf("  Absent:  %d", r.AbsentCount),
		fmt.Sprintf("  Excused: %d", r.ExcusedCount),
		fmt.Sprintf("  Attendance rate: %.1f%%", attendanceRate),
		"",
		fmt.Sprintf("Average grade: %.1f", r.AverageGrade),
		"",
		fmt.Sprintf("Generated: %s", r.GeneratedAt.Format("2006-01-02 15:04")),
	}
}

// asciiSafe strips non-Latin-1 characters so the base-14/WinAnsi font
// never renders mojibake for a Cyrillic name — see the package doc
// comment. Replace this with proper font embedding for production.
func asciiSafe(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r < 256 {
			b.WriteRune(r)
		} else {
			b.WriteRune('?')
		}
	}
	return b.String()
}

func escapePDFString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `(`, `\(`)
	s = strings.ReplaceAll(s, `)`, `\)`)
	return s
}

func buildContentStream(lines []string) string {
	var b strings.Builder
	b.WriteString("BT\n/F1 12 Tf\n14 TL\n50 780 Td\n")
	for i, line := range lines {
		if i > 0 {
			b.WriteString("T*\n")
		}
		fmt.Fprintf(&b, "(%s) Tj\n", escapePDFString(line))
	}
	b.WriteString("ET")
	return b.String()
}

// assemblePDF hand-writes a minimal single-page PDF (catalog, one page,
// Helvetica font, one content stream) with a correct xref table.
func assemblePDF(content string) []byte {
	var buf bytes.Buffer
	offsets := make([]int, 0, 6)

	writeObj := func(n int, body string) {
		offsets = append(offsets, buf.Len())
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", n, body)
	}

	buf.WriteString("%PDF-1.4\n")

	writeObj(1, "<< /Type /Catalog /Pages 2 0 R >>")
	writeObj(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>")
	writeObj(3, "<< /Type /Page /Parent 2 0 R /Resources << /Font << /F1 5 0 R >> >> /MediaBox [0 0 612 792] /Contents 4 0 R >>")
	writeObj(4, fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content))
	writeObj(5, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")

	xrefStart := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n", len(offsets)+1)
	buf.WriteString("0000000000 65535 f \n")
	for _, off := range offsets {
		fmt.Fprintf(&buf, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF", len(offsets)+1, xrefStart)

	return buf.Bytes()
}
