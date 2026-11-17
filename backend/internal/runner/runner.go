// Package runner generates quizzes with a headless coding agent (Claude Code,
// falling back to Codex), turns its output into progress events and, when it
// finishes, validates and renders the quiz.
package runner

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"os"
	"strings"
	"sync"
	"time"

	"reqtoquiz/internal/quiz"
	"reqtoquiz/internal/store"
)

// Steps, in order, shown by the progress screen.
const (
	StepQueue    = "queue"
	StepStart    = "start"
	StepSkill    = "skill"
	StepRead     = "read"
	StepGenerate = "generate"
	StepWrite    = "write"
	StepValidate = "validate"
	StepDone     = "done"
)

type Event struct {
	Type    string `json:"type"` // progress | done | error
	Step    string `json:"step,omitempty"`
	Message string `json:"message,omitempty"`
	URL     string `json:"url,omitempty"`
	Engine  string `json:"engine,omitempty"` // engine currently working
}

// Job fans out the events of one generation to any number of SSE clients.
// Late subscribers receive the full history first.
type Job struct {
	mu       sync.Mutex
	events   []Event
	subs     map[chan Event]struct{}
	finished bool
}

func newJob() *Job { return &Job{subs: map[chan Event]struct{}{}} }

func (j *Job) emit(e Event) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.finished {
		return
	}
	j.events = append(j.events, e)
	for ch := range j.subs {
		select {
		case ch <- e:
		default: // slow client: drop progress, the final event is resent from history
		}
	}
	if e.Type != "progress" {
		j.finished = true
		for ch := range j.subs {
			close(ch)
		}
		j.subs = nil
	}
}

// Subscribe returns the events so far and a channel with the following ones.
// The channel is closed when the job finishes.
func (j *Job) Subscribe() ([]Event, <-chan Event, func()) {
	j.mu.Lock()
	defer j.mu.Unlock()
	history := append([]Event(nil), j.events...)
	ch := make(chan Event, 64)
	if j.finished {
		close(ch)
		return history, ch, func() {}
	}
	j.subs[ch] = struct{}{}
	return history, ch, func() {
		j.mu.Lock()
		defer j.mu.Unlock()
		if _, ok := j.subs[ch]; ok {
			delete(j.subs, ch)
			close(ch)
		}
	}
}

type Manager struct {
	Root    string   // repo root: cwd of the engines, holds .claude/skills/job-quiz
	Engines []Engine // tried in order until one succeeds
	Timeout time.Duration
	Store   *store.Store

	sem  chan struct{} // one generation at a time
	mu   sync.Mutex
	jobs map[string]*Job
}

func NewManager(root string, engines []Engine, st *store.Store) *Manager {
	return &Manager{
		Root: root, Engines: engines, Store: st,
		Timeout: 10 * time.Minute,
		sem:     make(chan struct{}, 1),
		jobs:    map[string]*Job{},
	}
}

func (m *Manager) Get(id string) *Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.jobs[id]
}

func (m *Manager) Start(meta *store.Meta) *Job {
	job := newJob()
	m.mu.Lock()
	m.jobs[meta.ID] = job
	m.mu.Unlock()
	go m.run(meta, job)
	return job
}

func (m *Manager) run(meta *store.Meta, job *Job) {
	select {
	case m.sem <- struct{}{}:
	default:
		job.emit(Event{Type: "progress", Step: StepQueue, Message: "Esperando a que termine otro quiz en curso…"})
		m.sem <- struct{}{}
	}
	defer func() { <-m.sem }()

	err := m.generate(meta, job)
	if err != nil {
		log.Printf("quiz %s: %v", meta.ID, err)
		_, _ = m.Store.Update(meta.ID, func(mm *store.Meta) {
			mm.Status = store.StatusError
			mm.Error = err.Error()
		})
		job.emit(Event{Type: "error", Message: err.Error()})
		return
	}
	job.emit(Event{Type: "done", Step: StepDone, Message: "¡Quiz listo!", URL: "/quizzes/" + meta.ID + "/"})
}

func (m *Manager) generate(meta *store.Meta, job *Job) error {
	if len(m.Engines) == 0 {
		return errors.New("no hay ningún motor disponible: instalá Claude Code o Codex")
	}
	job.emit(Event{Type: "progress", Step: StepStart, Message: "Preparando la generación…"})

	req := Request{
		Root:       m.Root,
		QuizDir:    m.Store.Path(meta.ID),
		InputPath:  m.Store.Path(meta.ID, meta.InputFile),
		OutputPath: m.Store.Path(meta.ID, "questions.json"),
		Count:      meta.Count,
		Language:   meta.Language,
		Difficulty: meta.Difficulty,
	}

	// Same requirement used before? Collect those questions so they are not repeated.
	previous, err := m.previousQuestions(meta)
	if err != nil {
		return err
	}
	if len(previous) > 0 {
		req.AvoidPath = m.Store.Path(meta.ID, "avoid.md")
		if err := writeAvoidFile(req.AvoidPath, previous); err != nil {
			return err
		}
		prev := "1 quiz"
		if meta.PreviousQuizzes > 1 {
			prev = fmt.Sprintf("%d quizzes", meta.PreviousQuizzes)
		}
		job.emit(Event{Type: "progress", Step: StepStart, Message: fmt.Sprintf(
			"Ya generaste %s con este requerimiento: las %d preguntas anteriores no se van a repetir.", prev, len(previous))})
	}

	// Try each engine in order; fall back to the next one on any failure.
	var failures []string
	for i, engine := range m.Engines {
		if i > 0 {
			job.emit(Event{Type: "progress", Step: StepStart, Engine: engine.ID(), Message: fmt.Sprintf(
				"%s no pudo generar el quiz (%s). Probando con %s…", m.Engines[i-1].Name(), short(failures[i-1]), engine.Name())})
		} else {
			job.emit(Event{Type: "progress", Step: StepStart, Engine: engine.ID(), Message: "Iniciando " + engine.Name() + "…"})
		}
		q, err := m.attempt(engine, req, previous, job)
		if err == nil {
			return m.finish(meta, q, engine)
		}
		log.Printf("quiz %s: %s falló: %v", meta.ID, engine.Name(), err)
		failures = append(failures, err.Error())
	}
	if len(failures) == 1 {
		return errors.New(failures[0])
	}
	parts := make([]string, len(failures))
	for i, f := range failures {
		parts[i] = m.Engines[i].Name() + ": " + f
	}
	return errors.New("ningún motor pudo generar el quiz. " + strings.Join(parts, " — "))
}

// attempt runs one engine: a first try and, if the result is invalid, one
// retry telling the engine what was wrong.
func (m *Manager) attempt(engine Engine, req Request, previous []quiz.Question, job *Job) (*quiz.Quiz, error) {
	ctx, cancel := context.WithTimeout(context.Background(), m.Timeout)
	defer cancel()
	progress := func(step, msg string) {
		job.emit(Event{Type: "progress", Step: step, Message: msg, Engine: engine.ID()})
	}
	_ = os.Remove(req.OutputPath) // never validate a file left by another engine

	session, err := engine.Run(ctx, req, "", progress)
	if err != nil {
		return nil, timeoutErr(ctx, m.Timeout, err)
	}
	progress(StepValidate, "Validando las preguntas…")
	q, verr := loadValid(req.OutputPath, req.Count, previous)
	if verr == nil {
		return q, nil
	}

	progress(StepValidate, "Hay preguntas para corregir, pidiéndole a "+engine.Name()+" que las reemplace…")
	req.Fix = verr.Error()
	if _, err := engine.Run(ctx, req, session, progress); err != nil {
		return nil, timeoutErr(ctx, m.Timeout, err)
	}
	progress(StepValidate, "Validando las preguntas…")
	return loadValid(req.OutputPath, req.Count, previous)
}

func timeoutErr(ctx context.Context, d time.Duration, err error) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("tardó más de %s y se canceló", d)
	}
	return err
}

func (m *Manager) finish(meta *store.Meta, q *quiz.Quiz, engine Engine) error {
	quiz.Shuffle(q, rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64())))
	html, err := quiz.Render(meta.ID, q)
	if err != nil {
		return fmt.Errorf("no se pudo generar el HTML del quiz: %w", err)
	}
	if err := os.WriteFile(m.Store.Path(meta.ID, "quiz.html"), html, 0o644); err != nil {
		return err
	}
	_, err = m.Store.Update(meta.ID, func(mm *store.Meta) {
		mm.Title, mm.Role, mm.Seniority = q.Title, q.Role, q.Seniority
		mm.Status, mm.Error = store.StatusReady, ""
		mm.Engine = engine.ID()
	})
	return err
}

func short(s string) string {
	r := []rune(s)
	if len(r) > 140 {
		return string(r[:140]) + "…"
	}
	return s
}

func loadValid(path string, count int, previous []quiz.Question) (*quiz.Quiz, error) {
	q, err := quiz.Load(path)
	if err != nil {
		return nil, err
	}
	if err := quiz.Validate(q, count); err != nil {
		return nil, fmt.Errorf("questions.json inválido: %w", err)
	}
	if dups := quiz.Duplicates(q, previous); len(dups) > 0 {
		return nil, fmt.Errorf("preguntas repetidas: %s", strings.Join(dups, "; "))
	}
	return q, nil
}

// previousQuestions returns every question of earlier quizzes generated from
// the same requirement and records how many quizzes that was.
func (m *Manager) previousQuestions(meta *store.Meta) ([]quiz.Question, error) {
	if meta.InputHash == "" {
		return nil, nil
	}
	others, err := m.Store.SameInput(meta.InputHash, meta.ID)
	if err != nil {
		return nil, err
	}
	var out []quiz.Question
	used := 0
	for _, o := range others {
		q, err := quiz.Load(m.Store.Path(o.ID, "questions.json"))
		if err != nil {
			continue
		}
		used++
		out = append(out, q.Questions...)
	}
	if used > 0 {
		meta.PreviousQuizzes = used
		_, _ = m.Store.Update(meta.ID, func(mm *store.Meta) { mm.PreviousQuizzes = used })
	}
	return out, nil
}

func writeAvoidFile(path string, previous []quiz.Question) error {
	var b strings.Builder
	b.WriteString("# Concepts already tested for this job requirement\n\n")
	b.WriteString("Do NOT test any of these concepts again, not even with a different scenario, wording or language.\n")
	b.WriteString("Pick other concepts, sub-topics or skills from the posting.\n\n")
	for i, p := range previous {
		line := strings.Join(strings.Fields(p.Question), " ")
		if p.Concept != "" {
			fmt.Fprintf(&b, "%d. [%s] **%s** — %s\n", i+1, p.Topic, p.Concept, line)
		} else {
			fmt.Fprintf(&b, "%d. [%s] %s\n", i+1, p.Topic, line)
		}
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}
