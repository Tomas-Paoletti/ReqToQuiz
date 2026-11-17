// Package quiz validates the questions.json written by Claude, shuffles the
// options and renders the standalone quiz.html.
package quiz

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"math/rand/v2"
	"os"
	"strings"
)

type Question struct {
	ID          int      `json:"id"`
	Topic       string   `json:"topic"`
	Concept     string   `json:"concept,omitempty"` // exact concept tested; used to avoid repeats
	Question    string   `json:"question"`
	Options     []string `json:"options"`
	Answer      int      `json:"answer"`
	Explanation string   `json:"explanation"`
}

type Quiz struct {
	Title      string     `json:"title"`
	Role       string     `json:"role"`
	Seniority  string     `json:"seniority"`
	Language   string     `json:"language"`
	Difficulty string     `json:"difficulty"`
	Questions  []Question `json:"questions"`
}

// Load reads and strictly parses questions.json.
func Load(path string) (*Quiz, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("no se pudo leer questions.json: %w", err)
	}
	return Parse(data)
}

func Parse(data []byte) (*Quiz, error) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")) // UTF-8 BOM
	data = stripFences(bytes.TrimSpace(data))
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var q Quiz
	if err := dec.Decode(&q); err != nil {
		return nil, fmt.Errorf("questions.json no es JSON válido según el schema: %w", err)
	}
	return &q, nil
}

// stripFences removes a surrounding ```json … ``` block, which some engines
// add around a JSON answer.
func stripFences(data []byte) []byte {
	if !bytes.HasPrefix(data, []byte("```")) {
		return data
	}
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		data = data[i+1:]
	}
	data = bytes.TrimSpace(data)
	return bytes.TrimSpace(bytes.TrimSuffix(data, []byte("```")))
}

// Validate checks the quiz against the schema in .claude/skills/job-quiz/schema.md.
// All problems are returned together so a retry can fix them in one pass.
func Validate(q *Quiz, count int) error {
	var errs []string
	add := func(format string, a ...any) { errs = append(errs, fmt.Sprintf(format, a...)) }

	if strings.TrimSpace(q.Title) == "" {
		add("title está vacío")
	}
	if len(q.Questions) != count {
		add("se esperaban exactamente %d preguntas y hay %d", count, len(q.Questions))
	}
	for i, qu := range q.Questions {
		n := i + 1
		if qu.ID != n {
			add("pregunta %d: id debe ser %d (es %d)", n, n, qu.ID)
		}
		if strings.TrimSpace(qu.Question) == "" {
			add("pregunta %d: question vacía", n)
		}
		if strings.TrimSpace(qu.Concept) == "" {
			add("pregunta %d: concept vacío", n)
		}
		if strings.TrimSpace(qu.Explanation) == "" {
			add("pregunta %d: explanation vacía", n)
		}
		if len(qu.Options) != 4 {
			add("pregunta %d: debe tener exactamente 4 opciones (tiene %d)", n, len(qu.Options))
		} else {
			seen := map[string]bool{}
			for j, o := range qu.Options {
				key := strings.ToLower(strings.TrimSpace(o))
				if key == "" {
					add("pregunta %d: la opción %d está vacía", n, j)
				} else if seen[key] {
					add("pregunta %d: opciones repetidas", n)
				}
				seen[key] = true
			}
		}
		if qu.Answer < 0 || qu.Answer > 3 {
			add("pregunta %d: answer debe estar entre 0 y 3 (es %d)", n, qu.Answer)
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

// Shuffle randomizes the order of each question's options and fixes Answer.
func Shuffle(q *Quiz, r *rand.Rand) {
	for i := range q.Questions {
		qu := &q.Questions[i]
		correct := qu.Options[qu.Answer]
		r.Shuffle(len(qu.Options), func(a, b int) {
			qu.Options[a], qu.Options[b] = qu.Options[b], qu.Options[a]
		})
		for j, o := range qu.Options {
			if o == correct {
				qu.Answer = j
				break
			}
		}
	}
}

//go:embed quiz.html.tmpl
var tmplSrc string

var tmpl = template.Must(template.New("quiz").Parse(tmplSrc))

type pageData struct {
	ID   string
	Lang string
	Quiz *Quiz
}

// Render produces the standalone quiz page. id is used to report attempts back
// to the app (/api/quizzes/{id}/attempts); the page still works offline.
func Render(id string, q *Quiz) ([]byte, error) {
	lang := q.Language
	if lang != "en" {
		lang = "es"
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, pageData{ID: id, Lang: lang, Quiz: q}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
