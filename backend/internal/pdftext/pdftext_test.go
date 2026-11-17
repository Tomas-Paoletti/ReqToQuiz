package pdftext

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// minimalPDF builds a one-page PDF with the given lines of ASCII text.
func minimalPDF(lines []string) []byte {
	var content strings.Builder
	content.WriteString("BT /F1 12 Tf 50 750 Td 14 TL\n")
	for _, l := range lines {
		content.WriteString("(" + l + ") Tj T*\n")
	}
	content.WriteString("ET")
	s := content.String()
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>",
		"<< /Length " + itoa(len(s)) + " >>\nstream\n" + s + "\nendstream",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	out := "%PDF-1.4\n"
	var offs []int
	for i, o := range objs {
		offs = append(offs, len(out))
		out += itoa(i+1) + " 0 obj\n" + o + "\nendobj\n"
	}
	xref := len(out)
	out += "xref\n0 " + itoa(len(objs)+1) + "\n0000000000 65535 f \n"
	for _, o := range offs {
		out += pad10(o) + " 00000 n \n"
	}
	out += "trailer\n<< /Size " + itoa(len(objs)+1) + " /Root 1 0 R >>\nstartxref\n" + itoa(xref) + "\n%%EOF"
	return []byte(out)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for ; n > 0; n /= 10 {
		s = string(rune('0'+n%10)) + s
	}
	return s
}

func pad10(n int) string {
	s := itoa(n)
	return strings.Repeat("0", 10-len(s)) + s
}

func TestExtract(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.pdf")
	lines := []string{"Backend Developer Go", "Requisitos: Go, PostgreSQL, Docker, Kubernetes, Kafka", "Experiencia de 3 anios en sistemas de pagos"}
	if err := os.WriteFile(p, minimalPDF(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	text, err := Extract(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "PostgreSQL") || !strings.Contains(text, "pagos") {
		t.Errorf("unexpected text: %q", text)
	}
}

func TestExtractNoText(t *testing.T) {
	p := filepath.Join(t.TempDir(), "empty.pdf")
	if err := os.WriteFile(p, minimalPDF(nil), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Extract(p); err == nil {
		t.Error("expected error for a PDF without text")
	}
}

func TestExtractRealFile(t *testing.T) {
	p := os.Getenv("PDF")
	if p == "" {
		t.Skip("set PDF=<path> to try a real file")
	}
	text, err := Extract(p)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%d chars", len(text))
}
