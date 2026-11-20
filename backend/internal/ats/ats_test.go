package ats

import (
	"strings"
	"testing"
)

// Sample returns a valid report (exported for the runner tests via a copy).
func sample() *Report {
	return &Report{
		Title: "CV vs Backend Go Developer — Acme", Role: "Backend Go Developer", Company: "Acme", Language: "es",
		Summary: "Buen ajuste general.",
		Categories: []Category{
			{ID: "format", Name: "Formato", Score: 40, Comment: "Dos columnas."},
			{ID: "keywords", Name: "Palabras clave", Score: 60, Comment: "Faltan dos."},
			{ID: "experience", Name: "Experiencia", Score: 80, Comment: "Bien."},
			{ID: "education", Name: "Formación", Score: 100, Comment: "Bien."},
		},
		Knockouts: []Knockout{{Requirement: "3+ años de Go", Status: "met", Detail: "Desde 2021."}},
		Keywords: []Keyword{
			{Keyword: "Go", Importance: "required", Status: "found", Evidence: "microservicios en Go"},
			{Keyword: "Kubernetes", Importance: "required", Status: "missing", Suggestion: "Agregalo si lo usaste."},
			{Keyword: "CI/CD", Importance: "preferred", Status: "partial", Evidence: "automaticé deploys", Suggestion: "Escribí CI/CD."},
			{Keyword: "PostgreSQL", Importance: "required", Status: "found", Evidence: "PostgreSQL"},
			{Keyword: "Kafka", Importance: "preferred", Status: "missing", Suggestion: "Es un gap."},
		},
		Issues: []Issue{
			{Severity: "low", Area: "content", Problem: "Resumen largo.", Fix: "Acortalo."},
			{Severity: "high", Area: "format", Problem: "Dos columnas.", Fix: "Una columna."},
		},
		Strengths: []string{"Go y PostgreSQL."},
		Rewrites: []Rewrite{
			{Type: "rephrase", Section: "Experiencia", Before: "Trabajé en la  API de pagos.", After: "Desarrollé la API de pagos en Go.", Why: "Keywords."},
			{Type: "add_keyword", Section: "Skills", Before: "Go, PostgreSQL", After: "Go, PostgreSQL, CI/CD", Why: "Lo piden."},
		},
		CVMarkdown: sampleCV,
	}
}

const sampleCV = `# Juan Pérez
Backend · juan@example.com

## Experiencia
**Desarrollador Backend** — Pagos S.A.
- Trabajé en la API
  de pagos.

## Skills
Go, PostgreSQL
`

func TestValidateRewritesAgainstCV(t *testing.T) {
	r := sample()
	r.Rewrites[0].Before = "Lideré un equipo de 10 personas" // not in the CV
	r.Rewrites[1].Type = "invent"
	r.Rewrites = append(r.Rewrites, Rewrite{Type: "rephrase", Section: "x", Before: "Go, PostgreSQL", After: "Go,  PostgreSQL", Why: "y"})
	err := Validate(r)
	if err == nil {
		t.Fatal("expected errors")
	}
	for _, want := range []string{"no aparece textual en cvMarkdown", `type "invent"`, "after es igual a before"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in %v", want, err)
		}
	}
	r = sample()
	r.CVMarkdown = ""
	if err := Validate(r); err == nil || !strings.Contains(err.Error(), "cvMarkdown") {
		t.Errorf("empty cvMarkdown must fail, got %v", err)
	}
}

func TestValidateAcceptsSample(t *testing.T) {
	if err := Validate(sample()); err != nil {
		t.Fatal(err)
	}
}

func TestValidateReportsProblems(t *testing.T) {
	r := sample()
	r.Categories = r.Categories[:3]                // missing education
	r.Categories[0].Score = 120                    // out of range
	r.Keywords[1].Suggestion = ""                  // missing without suggestion
	r.Keywords[0].Evidence = ""                    // found without evidence
	r.Keywords = append(r.Keywords, r.Keywords[3]) // duplicate
	r.Issues[0].Severity = "critical"
	err := Validate(r)
	if err == nil {
		t.Fatal("expected errors")
	}
	for _, want := range []string{`falta la categoría "education"`, "fuera de 0-100", "suggestion es obligatoria", "evidence es obligatoria", "repetida", "severity"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in %v", want, err)
		}
	}
}

func TestFinalizeScoreAndVerdict(t *testing.T) {
	r := sample()
	Finalize(r)
	// 60*40 + 80*30 + 100*15 + 40*15 = 6900 / 100
	if r.Score != 69 || r.Verdict != VerdictBorderline {
		t.Errorf("got score %d verdict %s", r.Score, r.Verdict)
	}
	if r.Categories[0].ID != "keywords" || r.Issues[0].Severity != "high" {
		t.Error("categories and issues should be sorted")
	}
	if k := r.Keywords[0]; k.Importance != "required" || k.Status != "missing" {
		t.Errorf("missing required keywords should come first, got %+v", k)
	}

	r = sample()
	for i := range r.Categories {
		r.Categories[i].Score = 90
	}
	r.Knockouts[0].Status = "not_met"
	Finalize(r)
	if r.Verdict != VerdictBorderline {
		t.Errorf("a failed knockout must cap the verdict, got %s", r.Verdict)
	}
}

func TestRender(t *testing.T) {
	r := sample()
	r.Summary = "<script>alert(1)</script>"
	Finalize(r)
	html, err := Render("20260101-000000-abcdef", "mi-cv.pdf", r)
	if err != nil {
		t.Fatal(err)
	}
	s := string(html)
	for _, want := range []string{"En el límite", "mi-cv.pdf", "Kubernetes", "Obligatorias: <strong>2 de 3</strong>", "Imprimir informe"} {
		if !strings.Contains(s, want) {
			t.Errorf("render is missing %q", want)
		}
	}
	if strings.Contains(s, "<script>alert") {
		t.Error("report text must be escaped")
	}
}

func TestParseStripsFences(t *testing.T) {
	if _, err := Parse([]byte("```json\n{\"title\":\"x\"}\n```")); err != nil {
		t.Fatal(err)
	}
	if _, err := Parse([]byte(`{"unknown": 1}`)); err == nil {
		t.Error("unknown fields must be rejected")
	}
}
