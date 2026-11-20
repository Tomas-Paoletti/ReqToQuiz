package runner

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"reqtoquiz/internal/ats"
	"reqtoquiz/internal/pdftext"
	"reqtoquiz/internal/store"
)

//go:embed codex_ats_schema.json
var codexATSSchema []byte

// cvTextFile is what an ATS parser would extract from a PDF CV.
const cvTextFile = "cv-parsed.txt"

func (m *Manager) atsTask(meta *store.Meta, job *Job) (task, error) {
	t := task{kind: "ats", doneMsg: "¡Análisis listo!"}
	if meta.CVFile == "" {
		return t, errors.New("falta el CV")
	}
	cvPath := m.Store.Path(meta.ID, meta.CVFile)
	req := Request{
		Root:    m.Root,
		QuizDir: m.Store.Path(meta.ID),
		Skill:   "ats-check",
		Inputs: []Input{
			{Key: "job", Path: m.Store.Path(meta.ID, meta.InputFile), Label: "el requerimiento del puesto"},
			{Key: "cv", Path: cvPath, Label: "tu CV"},
		},
		OutputPath:  m.Store.Path(meta.ID, "ats.json"),
		CodexSchema: codexATSSchema,
		Language:    meta.Language,
		Labels: Labels{
			Think: "Comparando tu CV con el puesto…",
			Write: "Escribiendo el informe…",
			Save:  "Guardando el informe…",
			Noun:  "el informe",
		},
		FixHint: "Keep every category, keyword and finding that was valid; only fix the listed problems.",
	}

	// A PDF CV is also run through a plain text extractor, like an ATS does, so
	// the skill can spot what gets lost (columns, tables, text in images…).
	if strings.EqualFold(filepath.Ext(cvPath), ".pdf") {
		job.emit(Event{Type: "progress", Step: StepStart, Message: "Extrayendo el texto del CV como lo haría un ATS…"})
		text, err := pdftext.Extract(cvPath)
		if err != nil {
			req.Params = append(req.Params, [2]string{"cv_text", fmt.Sprintf(
				"NONE — a standard ATS text extractor could not read this PDF (%v). Treat it as a critical format problem.", err)})
		} else {
			if err := os.WriteFile(m.Store.Path(meta.ID, cvTextFile), []byte(text), 0o644); err != nil {
				return t, err
			}
			req.Inputs = append(req.Inputs, Input{Key: "cv_text", Path: m.Store.Path(meta.ID, cvTextFile), Label: "el texto que ve el ATS"})
		}
	}
	req.Params = append(req.Params, [2]string{"language", meta.Language})

	var r *ats.Report
	t.req = req
	t.validate = func() error {
		rep, err := ats.Load(req.OutputPath)
		if err != nil {
			return err
		}
		if err := ats.Validate(rep); err != nil {
			return fmt.Errorf("ats.json inválido: %w", err)
		}
		r = rep
		return nil
	}
	t.finish = func(e Engine) error {
		ats.Finalize(r)
		html, err := ats.Render(meta.ID, meta.CVSource, r)
		if err != nil {
			return fmt.Errorf("no se pudo generar el HTML del informe: %w", err)
		}
		if err := os.WriteFile(m.Store.Path(meta.ID, "report.html"), html, 0o644); err != nil {
			return err
		}
		_, err = m.Store.Update(meta.ID, func(mm *store.Meta) {
			mm.Title, mm.Role = r.Title, r.Role
			mm.ATS = &store.ATSResult{Score: r.Score, Verdict: r.Verdict}
			mm.Status, mm.Error = store.StatusReady, ""
			mm.Engine = e.ID()
		})
		return err
	}
	return t, nil
}
