package runner

import (
	"os"
	"strings"
	"testing"

	"reqtoquiz/internal/ats"
	"reqtoquiz/internal/store"
)

func goodReport(Request) *ats.Report {
	r := &ats.Report{Title: "CV vs Dev", Role: "Dev", Language: "es", Summary: "Bien.",
		CVMarkdown: "# Ana\n\n## Experiencia\n- Desarrolladora Go con SQL, Docker, Git y Linux en una fintech desde 2021.\n",
		Rewrites: []ats.Rewrite{{Type: "rephrase", Section: "Experiencia", Before: "Desarrolladora Go con SQL",
			After: "Desarrolladora backend en Go con SQL", Why: "Título del aviso."}}}
	for _, id := range []string{"keywords", "experience", "education", "format"} {
		r.Categories = append(r.Categories, ats.Category{ID: id, Name: id, Score: 80, Comment: "ok"})
	}
	for _, k := range []string{"Go", "SQL", "Docker", "Git", "Linux"} {
		r.Keywords = append(r.Keywords, ats.Keyword{Keyword: k, Importance: "required", Status: "found", Evidence: k})
	}
	return r
}

func TestATSReport(t *testing.T) {
	var got Request
	claude := &fakeEngine{id: "claude", report: func(req Request) *ats.Report {
		got = req
		r := goodReport(req)
		if req.Fix == "" {
			r.Categories = r.Categories[:3] // first attempt invalid: retried with the error
		}
		return r
	}}
	m, meta := newTestManager(t, claude)
	meta.Kind, meta.CVFile, meta.CVSource = store.KindATS, "cv.md", "mi-cv.md"
	if err := m.Store.Save(meta); err != nil {
		t.Fatal(err)
	}
	if err := m.Store.WriteFile(meta.ID, "cv.md", []byte("Desarrollador Go con SQL, Docker, Git y Linux")); err != nil {
		t.Fatal(err)
	}

	events := runSync(t, m, meta)
	last := events[len(events)-1]
	if last.Type != "done" || last.URL != "/ats/"+meta.ID+"/" {
		t.Fatalf("expected done with the report URL, got %+v", last)
	}
	if claude.calls != 2 || !strings.Contains(claude.fixes[1], `falta la categoría "format"`) {
		t.Errorf("invalid report should be retried with the error: calls=%d fixes=%q", claude.calls, claude.fixes)
	}
	if got.Skill != "ats-check" || len(got.Inputs) != 2 || got.Inputs[1].Key != "cv" {
		t.Errorf("unexpected request: %+v", got)
	}
	saved, _ := m.Store.Get(meta.ID)
	if saved.Status != store.StatusReady || saved.ATS == nil || saved.ATS.Score != 80 || saved.ATS.Verdict != ats.VerdictPass {
		t.Errorf("unexpected meta: %+v ats=%+v", saved, saved.ATS)
	}
	if _, err := os.Stat(m.Store.Path(meta.ID, "report.html")); err != nil {
		t.Error("report.html not written")
	}
}

func TestClaudePrompt(t *testing.T) {
	req := Request{Root: "/r", Skill: "ats-check", OutputPath: "/r/d/ats.json",
		Inputs: []Input{{Key: "job", Path: "/r/d/input.pdf"}, {Key: "cv", Path: "/r/d/cv.pdf"}},
		Params: [][2]string{{"language", "en"}}}
	p := (&Claude{}).prompt(req, "")
	for _, want := range []string{"Use the ats-check skill.", "job: d/input.pdf\ncv: d/cv.pdf\nlanguage: en\noutput: d/ats.json"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt %q is missing %q", p, want)
		}
	}
	in := Request{Inputs: []Input{{Path: "/x/cv.PDF", Label: "tu CV"}}}
	if label := in.inputLabel("C:/x/cv.pdf"); label != "tu CV" {
		t.Errorf("inputLabel = %q", label)
	}
}
