// Package ats validates the ATS report (ats.json) written by the ats-check
// skill, computes the overall score and renders the standalone report.html.
package ats

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"math"
	"os"
	"sort"
	"strings"
)

// Categories of the score, with their weight in the overall score.
var Categories = []struct {
	ID     string
	Weight int
}{
	{"keywords", 40},   // hard skills, tools and keywords of the posting
	{"experience", 30}, // years, seniority, responsibilities, titles
	{"education", 15},  // degrees, certifications, languages
	{"format", 15},     // how well an ATS can parse the CV
}

// Verdicts, from the overall score (and knockouts).
const (
	VerdictPass       = "pass"
	VerdictBorderline = "borderline"
	VerdictReject     = "reject"
)

type Category struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Score   int    `json:"score"`
	Comment string `json:"comment"`
}

type Knockout struct {
	Requirement string `json:"requirement"`
	Status      string `json:"status"` // met | unclear | not_met
	Detail      string `json:"detail"`
}

type Keyword struct {
	Keyword    string `json:"keyword"`
	Importance string `json:"importance"` // required | preferred
	Status     string `json:"status"`     // found | partial | missing
	Evidence   string `json:"evidence"`   // quote from the CV ("" when missing)
	Suggestion string `json:"suggestion"`
}

type Issue struct {
	Severity string `json:"severity"` // high | medium | low
	Area     string `json:"area"`     // format | keywords | experience | education | content
	Problem  string `json:"problem"`
	Fix      string `json:"fix"`
}

// Rewrite types: a rephrasing of facts already in the CV, or adding a
// keyword of the posting (to be accepted only if it is true).
const (
	RewriteRephrase   = "rephrase"
	RewriteAddKeyword = "add_keyword"
)

type Rewrite struct {
	Type    string `json:"type"` // rephrase | add_keyword
	Section string `json:"section"`
	Before  string `json:"before"`
	After   string `json:"after"`
	Why     string `json:"why"`
}

type Report struct {
	Title      string     `json:"title"`
	Role       string     `json:"role"`
	Company    string     `json:"company"`
	Language   string     `json:"language"`
	Summary    string     `json:"summary"`
	Categories []Category `json:"categories"`
	Knockouts  []Knockout `json:"knockouts"`
	Keywords   []Keyword  `json:"keywords"`
	Issues     []Issue    `json:"issues"`
	Strengths  []string   `json:"strengths"`
	Rewrites   []Rewrite  `json:"rewrites"`
	// CVMarkdown is the whole CV transcribed to simple Markdown. Every
	// rewrite's Before appears in it, so accepted rewrites can be applied
	// to build the improved CV.
	CVMarkdown string `json:"cvMarkdown"`

	// Computed by Finalize, not written by the skill.
	Score   int    `json:"score,omitempty"`
	Verdict string `json:"verdict,omitempty"`
}

func Load(path string) (*Report, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("no se pudo leer ats.json: %w", err)
	}
	return Parse(data)
}

func Parse(data []byte) (*Report, error) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	data = bytes.TrimSpace(data)
	if bytes.HasPrefix(data, []byte("```")) { // ```json … ``` around the answer
		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			data = data[i+1:]
		}
		data = bytes.TrimSuffix(bytes.TrimSpace(data), []byte("```"))
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var r Report
	if err := dec.Decode(&r); err != nil {
		return nil, fmt.Errorf("ats.json no es JSON válido según el schema: %w", err)
	}
	return &r, nil
}

func oneOf(v string, allowed ...string) bool {
	for _, a := range allowed {
		if v == a {
			return true
		}
	}
	return false
}

// Validate checks the report against schema.md and returns every problem.
func Validate(r *Report) error {
	var errs []string
	add := func(format string, a ...any) { errs = append(errs, fmt.Sprintf(format, a...)) }
	blank := func(s string) bool { return strings.TrimSpace(s) == "" }

	if blank(r.Title) {
		add("title vacío")
	}
	if blank(r.Role) {
		add("role vacío")
	}
	if blank(r.Summary) {
		add("summary vacío")
	}
	if !oneOf(r.Language, "es", "en") {
		add("language debe ser es o en")
	}

	seen := map[string]bool{}
	for i, c := range r.Categories {
		switch {
		case seen[c.ID]:
			add("categories[%d]: id %q repetido", i, c.ID)
		case !oneOf(c.ID, "keywords", "experience", "education", "format"):
			add("categories[%d]: id %q inválido (keywords, experience, education o format)", i, c.ID)
		}
		seen[c.ID] = true
		if c.Score < 0 || c.Score > 100 {
			add("categories[%d]: score %d fuera de 0-100", i, c.Score)
		}
		if blank(c.Name) || blank(c.Comment) {
			add("categories[%d]: name y comment son obligatorios", i)
		}
	}
	for _, c := range Categories {
		if !seen[c.ID] {
			add("falta la categoría %q", c.ID)
		}
	}

	for i, k := range r.Knockouts {
		if blank(k.Requirement) || blank(k.Detail) {
			add("knockouts[%d]: requirement y detail son obligatorios", i)
		}
		if !oneOf(k.Status, "met", "unclear", "not_met") {
			add("knockouts[%d]: status %q inválido (met, unclear o not_met)", i, k.Status)
		}
	}

	if len(r.Keywords) < 5 {
		add("se esperaban al menos 5 keywords, hay %d", len(r.Keywords))
	}
	kw := map[string]bool{}
	for i, k := range r.Keywords {
		key := strings.ToLower(strings.TrimSpace(k.Keyword))
		if key == "" {
			add("keywords[%d]: keyword vacía", i)
		} else if kw[key] {
			add("keywords[%d]: %q repetida", i, k.Keyword)
		}
		kw[key] = true
		if !oneOf(k.Importance, "required", "preferred") {
			add("keywords[%d]: importance %q inválida (required o preferred)", i, k.Importance)
		}
		if !oneOf(k.Status, "found", "partial", "missing") {
			add("keywords[%d]: status %q inválido (found, partial o missing)", i, k.Status)
		}
		if k.Status != "missing" && blank(k.Evidence) {
			add("keywords[%d] (%s): evidence es obligatoria si no es missing", i, k.Keyword)
		}
		if k.Status != "found" && blank(k.Suggestion) {
			add("keywords[%d] (%s): suggestion es obligatoria si no es found", i, k.Keyword)
		}
	}

	for i, is := range r.Issues {
		if !oneOf(is.Severity, "high", "medium", "low") {
			add("issues[%d]: severity %q inválida (high, medium o low)", i, is.Severity)
		}
		if !oneOf(is.Area, "format", "keywords", "experience", "education", "content") {
			add("issues[%d]: area %q inválida", i, is.Area)
		}
		if blank(is.Problem) || blank(is.Fix) {
			add("issues[%d]: problem y fix son obligatorios", i)
		}
	}
	for i, s := range r.Strengths {
		if blank(s) {
			add("strengths[%d] vacío", i)
		}
	}
	cv := Normalize(r.CVMarkdown)
	if len(cv) < 80 {
		add("cvMarkdown vacío o demasiado corto: debe ser el CV completo transcripto")
	}
	if len(r.Rewrites) == 0 {
		add("se esperaba al menos 1 rewrite")
	}
	for i, rw := range r.Rewrites {
		if !oneOf(rw.Type, RewriteRephrase, RewriteAddKeyword) {
			add("rewrites[%d]: type %q inválido (rephrase o add_keyword)", i, rw.Type)
		}
		if blank(rw.Section) || blank(rw.Before) || blank(rw.After) || blank(rw.Why) {
			add("rewrites[%d]: section, before, after y why son obligatorios", i)
			continue
		}
		if Normalize(rw.Before) == Normalize(rw.After) {
			add("rewrites[%d]: after es igual a before", i)
		}
		if len(cv) >= 80 && !strings.Contains(cv, Normalize(rw.Before)) {
			add("rewrites[%d]: before %q no aparece textual en cvMarkdown (copiá el texto exacto)", i, short(rw.Before))
		}
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

// Normalize collapses whitespace, which is how rewrites are matched against
// the CV (the frontend applies them the same way).
func Normalize(s string) string { return strings.Join(strings.Fields(s), " ") }

func short(s string) string {
	r := []rune(s)
	if len(r) > 60 {
		return string(r[:60]) + "…"
	}
	return s
}

// Finalize computes the overall score (weighted category scores) and the
// verdict, and sorts issues by severity. A knockout that is not met caps the
// verdict at borderline: many ATS filter on those before any score.
func Finalize(r *Report) {
	weights := map[string]int{}
	for _, c := range Categories {
		weights[c.ID] = c.Weight
	}
	total, sum := 0, 0.0
	for _, c := range r.Categories {
		total += weights[c.ID]
		sum += float64(c.Score * weights[c.ID])
	}
	if total > 0 {
		r.Score = int(math.Round(sum / float64(total)))
	}
	switch {
	case r.Score >= 75:
		r.Verdict = VerdictPass
	case r.Score >= 55:
		r.Verdict = VerdictBorderline
	default:
		r.Verdict = VerdictReject
	}
	if r.Verdict == VerdictPass && r.KnockoutsFailed() > 0 {
		r.Verdict = VerdictBorderline
	}

	order := map[string]int{"knockouts": 0, "keywords": 1, "experience": 2, "education": 3, "format": 4}
	sort.SliceStable(r.Categories, func(i, j int) bool { return order[r.Categories[i].ID] < order[r.Categories[j].ID] })
	sev := map[string]int{"high": 0, "medium": 1, "low": 2}
	sort.SliceStable(r.Issues, func(i, j int) bool { return sev[r.Issues[i].Severity] < sev[r.Issues[j].Severity] })
	status := map[string]int{"missing": 0, "partial": 1, "found": 2}
	sort.SliceStable(r.Keywords, func(i, j int) bool {
		a, b := r.Keywords[i], r.Keywords[j]
		if a.Importance != b.Importance {
			return a.Importance == "required"
		}
		return status[a.Status] < status[b.Status]
	})
}

func (r *Report) KnockoutsFailed() int {
	n := 0
	for _, k := range r.Knockouts {
		if k.Status == "not_met" {
			n++
		}
	}
	return n
}

// KeywordStats counts keywords by status for one importance level.
func (r *Report) KeywordStats(importance string) (found, partial, total int) {
	for _, k := range r.Keywords {
		if k.Importance != importance {
			continue
		}
		total++
		switch k.Status {
		case "found":
			found++
		case "partial":
			partial++
		}
	}
	return
}

//go:embed report.html.tmpl
var tmplSrc string

var tmpl = template.Must(template.New("report").Funcs(template.FuncMap{
	"t": func(lang, key string) string {
		if v, ok := texts[lang][key]; ok {
			return v
		}
		return texts["es"][key]
	},
	"band": func(score int) string {
		switch {
		case score >= 75:
			return "good"
		case score >= 55:
			return "mid"
		default:
			return "low"
		}
	},
	"weight": func(id string) int {
		for _, c := range Categories {
			if c.ID == id {
				return c.Weight
			}
		}
		return 0
	},
}).Parse(tmplSrc))

type pageData struct {
	ID, Lang                          string
	CV                                string
	R                                 *Report
	ReqFound, ReqPartial, ReqTotal    int
	PrefFound, PrefPartial, PrefTotal int
}

// Render produces the standalone report page. cv is the original CV filename.
func Render(id, cv string, r *Report) ([]byte, error) {
	lang := r.Language
	if lang != "en" {
		lang = "es"
	}
	d := pageData{ID: id, Lang: lang, CV: cv, R: r}
	d.ReqFound, d.ReqPartial, d.ReqTotal = r.KeywordStats("required")
	d.PrefFound, d.PrefPartial, d.PrefTotal = r.KeywordStats("preferred")
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, d); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

var texts = map[string]map[string]string{
	"es": {
		"heading": "Filtro ATS", "cv": "CV", "print": "Imprimir informe", "back": "Volver",
		"score": "Puntaje ATS", "pass": "Pasaría el filtro", "borderline": "En el límite", "reject": "Probablemente filtrado",
		"verdict_pass":       "Tu CV cubre bien lo que pide el puesto. Ajustá los detalles de abajo para destacarte.",
		"verdict_borderline": "Podrías pasar o no según el corte de la empresa. Con los cambios de abajo subís bastante.",
		"verdict_reject":     "Así como está, un ATS probablemente lo descarte. Empezá por los problemas de prioridad alta.",
		"knockout_warn":      "requisito excluyente no cumplido: muchos ATS descartan antes de mirar el puntaje.",
		"knockout_warn_n":    "requisitos excluyentes no cumplidos: muchos ATS descartan antes de mirar el puntaje.",
		"categories":         "Desglose", "weight": "peso",
		"knockouts": "Requisitos excluyentes", "met": "Cumple", "unclear": "No queda claro", "not_met": "No cumple",
		"keywords": "Palabras clave", "required": "Obligatorias", "preferred": "Deseables",
		"found": "Está", "partial": "Parcial", "missing": "Falta", "evidence": "En tu CV", "suggestion": "Qué hacer",
		"of": "de", "present": "presentes",
		"issues": "Dónde puede fallar", "high": "Alta", "medium": "Media", "low": "Baja", "none_issues": "No se encontraron problemas importantes.",
		"strengths": "Puntos fuertes", "rewrites": "Reescrituras sugeridas", "before": "Antes", "after": "Después",
		"edit_cta": "Elegir cambios y descargar CV", "rephrase": "Reformula", "add_keyword": "Agrega keyword: solo si es cierto",
		"honesty":     "Las sugerencias solo reformulan lo que ya está en tu CV. No agregues experiencia o skills que no tengas: se nota en la entrevista.",
		"area_format": "Formato", "area_keywords": "Keywords", "area_experience": "Experiencia", "area_education": "Formación", "area_content": "Contenido",
	},
	"en": {
		"heading": "ATS check", "cv": "CV", "print": "Print report", "back": "Back",
		"score": "ATS score", "pass": "Would pass the filter", "borderline": "Borderline", "reject": "Likely filtered out",
		"verdict_pass":       "Your CV covers what the job asks for. Polish the details below to stand out.",
		"verdict_borderline": "You may or may not pass depending on the company's cutoff. The changes below raise it a lot.",
		"verdict_reject":     "As it is, an ATS would probably discard it. Start with the high-priority issues.",
		"knockout_warn":      "knockout requirement not met: many ATS reject before looking at the score.",
		"knockout_warn_n":    "knockout requirements not met: many ATS reject before looking at the score.",
		"categories":         "Breakdown", "weight": "weight",
		"knockouts": "Knockout requirements", "met": "Met", "unclear": "Unclear", "not_met": "Not met",
		"keywords": "Keywords", "required": "Required", "preferred": "Preferred",
		"found": "Found", "partial": "Partial", "missing": "Missing", "evidence": "In your CV", "suggestion": "What to do",
		"of": "of", "present": "present",
		"issues": "Where it can fail", "high": "High", "medium": "Medium", "low": "Low", "none_issues": "No important issues found.",
		"strengths": "Strengths", "rewrites": "Suggested rewrites", "before": "Before", "after": "After",
		"edit_cta": "Pick changes and download CV", "rephrase": "Rephrase", "add_keyword": "Adds keyword: only if true",
		"honesty":     "Suggestions only rephrase what is already in your CV. Do not add experience or skills you do not have: it shows in the interview.",
		"area_format": "Format", "area_keywords": "Keywords", "area_experience": "Experience", "area_education": "Education", "area_content": "Content",
	},
}
