package quiz

import (
	"math/rand/v2"
	"strings"
	"testing"
)

func sample(n int) *Quiz {
	q := &Quiz{Title: "Test", Role: "Dev", Language: "es", Difficulty: "mixed"}
	for i := 1; i <= n; i++ {
		q.Questions = append(q.Questions, Question{
			ID: i, Topic: "Go", Concept: "Concepto", Question: "¿Pregunta?",
			Options: []string{"a", "b", "c", "d"}, Answer: i % 4, Explanation: "Porque sí.",
		})
	}
	return q
}

func TestValidateOK(t *testing.T) {
	if err := Validate(sample(5), 5); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
}

func TestValidateErrors(t *testing.T) {
	cases := map[string]func(*Quiz){
		"preguntas":      func(q *Quiz) { q.Questions = q.Questions[:4] },
		"answer":         func(q *Quiz) { q.Questions[0].Answer = 4 },
		"4 opciones":     func(q *Quiz) { q.Questions[1].Options = q.Questions[1].Options[:3] },
		"repetidas":      func(q *Quiz) { q.Questions[2].Options[1] = "A" },
		"id debe ser":    func(q *Quiz) { q.Questions[3].ID = 9 },
		"explanation":    func(q *Quiz) { q.Questions[0].Explanation = " " },
		"title está vac": func(q *Quiz) { q.Title = "" },
	}
	for want, mutate := range cases {
		q := sample(5)
		mutate(q)
		err := Validate(q, 5)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: expected error containing %q, got %v", want, want, err)
		}
	}
}

func TestParseRejectsUnknownFieldsAndAcceptsBOM(t *testing.T) {
	if _, err := Parse([]byte(`{"title":"x","foo":1}`)); err == nil {
		t.Error("expected unknown field error")
	}
	if _, err := Parse([]byte("\xef\xbb\xbf{\"title\":\"x\",\"questions\":[]}")); err != nil {
		t.Errorf("BOM should be accepted: %v", err)
	}
}

func TestParseStripsMarkdownFences(t *testing.T) {
	q, err := Parse([]byte("```json\n{\"title\":\"x\",\"questions\":[]}\n```\n"))
	if err != nil || q.Title != "x" {
		t.Fatalf("fenced JSON should parse, got %v", err)
	}
}

func TestShuffleKeepsCorrectAnswer(t *testing.T) {
	q := sample(40)
	want := make([]string, len(q.Questions))
	for i, qu := range q.Questions {
		want[i] = qu.Options[qu.Answer]
	}
	Shuffle(q, rand.New(rand.NewPCG(1, 2)))
	for i, qu := range q.Questions {
		if qu.Options[qu.Answer] != want[i] {
			t.Fatalf("question %d: correct answer changed", i+1)
		}
	}
}

func TestRenderEscapesAndEmbedsData(t *testing.T) {
	q := sample(5)
	q.Title = "<script>alert(1)</script>"
	out, err := Render("20261117-101010-abcdef", q)
	if err != nil {
		t.Fatal(err)
	}
	html := string(out)
	if strings.Contains(html, "<script>alert(1)</script>") {
		t.Error("title was not escaped")
	}
	if !strings.Contains(html, `"20261117-101010-abcdef"`) || !strings.Contains(html, `"questions":[`) {
		t.Error("quiz id or data missing from rendered page")
	}
}
