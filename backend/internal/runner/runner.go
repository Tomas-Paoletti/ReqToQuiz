// Package runner generates quizzes and ATS reports with a headless coding
// agent (Claude Code, falling back to Codex), turns its output into progress
// events and, when it finishes, validates and renders the result.
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

// Known engines, in default fallback order.
var (
	KnownEngines = []string{"claude", "codex"}
	EngineNames  = map[string]string{"claude": "Claude Code", "codex": "Codex"}
)

// ChoiceAuto tries every installed engine with a session, in order.
const ChoiceAuto = "auto"

type Manager struct {
	Root    string   // repo root: cwd of the engines, holds .claude/skills
	Engines []Engine // installed engines, in fallback order
	Missing []string // ids of known engines that are not installed
	Timeout time.Duration
	Store   *store.Store

	auth authCache
	sem  chan struct{} // one generation at a time
	mu   sync.Mutex
	jobs map[string]*Job
}

func NewManager(root string, engines []Engine, st *store.Store) *Manager {
	m := &Manager{
		Root: root, Engines: engines, Store: st,
		Timeout: 10 * time.Minute,
		auth:    authCache{ttl: 20 * time.Second},
		sem:     make(chan struct{}, 1),
		jobs:    map[string]*Job{},
	}
	for _, id := range KnownEngines {
		if m.engine(id) == nil {
			m.Missing = append(m.Missing, id)
		}
	}
	return m
}

func (m *Manager) engine(id string) Engine {
	for _, e := range m.Engines {
		if e.ID() == id {
			return e
		}
	}
	return nil
}

// AuthStatuses returns the session state of each installed engine.
// force skips the short cache.
func (m *Manager) AuthStatuses(ctx context.Context, force bool) map[string]AuthStatus {
	return m.auth.statuses(ctx, m.Engines, force)
}

// plan returns the engines to try for a choice ("auto" or an engine id).
func (m *Manager) plan(choice string) ([]Engine, error) {
	if choice == "" || choice == ChoiceAuto {
		if len(m.Engines) == 0 {
			return nil, errors.New("no hay ningún motor instalado: instalá Claude Code o Codex")
		}
		return m.Engines, nil
	}
	name, known := EngineNames[choice]
	if !known {
		return nil, fmt.Errorf("motor inválido %q", choice)
	}
	e := m.engine(choice)
	if e == nil {
		return nil, fmt.Errorf("%s no está instalado", name)
	}
	return []Engine{e}, nil
}

// notLoggedIn builds the error for an engine without a session.
func notLoggedIn(e Engine, st AuthStatus) error {
	return fmt.Errorf("%s no tiene sesión iniciada: ejecutá `%s` en una terminal", e.Name(), st.LoginCommand)
}

// CheckChoice validates a choice before a quiz is created, so the user gets
// the error right away instead of after the generation starts.
func (m *Manager) CheckChoice(ctx context.Context, choice string) error {
	engines, err := m.plan(choice)
	if err != nil {
		return err
	}
	statuses := m.AuthStatuses(ctx, false)
	var errs []string
	for _, e := range engines {
		st := statuses[e.ID()]
		if st.Usable() {
			return nil
		}
		errs = append(errs, notLoggedIn(e, st).Error())
	}
	if len(errs) == 1 {
		return errors.New(errs[0])
	}
	return errors.New("ningún motor tiene sesión iniciada. " + strings.Join(errs, " — "))
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
		job.emit(Event{Type: "progress", Step: StepQueue, Message: "Esperando a que termine otra generación en curso…"})
		m.sem <- struct{}{}
	}
	defer func() { <-m.sem }()

	t, err := m.newTask(meta, job)
	if err == nil {
		err = m.execute(meta, job, t)
	}
	if err != nil {
		log.Printf("%s %s: %v", t.kind, meta.ID, err)
		_, _ = m.Store.Update(meta.ID, func(mm *store.Meta) {
			mm.Status = store.StatusError
			mm.Error = err.Error()
		})
		job.emit(Event{Type: "error", Message: err.Error()})
		return
	}
	job.emit(Event{Type: "done", Step: StepDone, Message: t.doneMsg, URL: PageURL(meta)})
}

// PageURL is where a ready quiz or ATS report is served.
func PageURL(meta *store.Meta) string {
	if meta.Kind == store.KindATS {
		return "/ats/" + meta.ID + "/"
	}
	return "/quizzes/" + meta.ID + "/"
}

// task is one kind of generation (quiz or ATS report) run by the engines.
type task struct {
	kind     string // for logs
	req      Request
	doneMsg  string
	validate func() error         // checks the output file; the error is sent back on retry
	finish   func(e Engine) error // renders the page and marks the meta ready
}

func (m *Manager) newTask(meta *store.Meta, job *Job) (task, error) {
	job.emit(Event{Type: "progress", Step: StepStart, Message: "Preparando la generación…"})
	if meta.Kind == store.KindATS {
		return m.atsTask(meta, job)
	}
	return m.quizTask(meta, job)
}

func (m *Manager) quizTask(meta *store.Meta, job *Job) (task, error) {
	req := Request{
		Root:        m.Root,
		QuizDir:     m.Store.Path(meta.ID),
		Skill:       "job-quiz",
		Inputs:      []Input{{Key: "input", Path: m.Store.Path(meta.ID, meta.InputFile), Label: "el requerimiento del puesto"}},
		OutputPath:  m.Store.Path(meta.ID, "questions.json"),
		CodexSchema: codexSchema,
		Count:       meta.Count,
		Language:    meta.Language,
		Difficulty:  meta.Difficulty,
		Labels: Labels{
			Think: "Analizando el puesto y pensando las preguntas…",
			Write: "Escribiendo las preguntas…",
			Save:  "Guardando el quiz…",
			Noun:  "el quiz",
		},
	}
	t := task{kind: "quiz", doneMsg: "¡Quiz listo!"}

	// Same requirement used before? Collect those questions so they are not repeated.
	previous, err := m.previousQuestions(meta)
	if err != nil {
		return t, err
	}
	if len(previous) > 0 {
		req.AvoidPath = m.Store.Path(meta.ID, "avoid.md")
		if err := writeAvoidFile(req.AvoidPath, previous); err != nil {
			return t, err
		}
		prev := "1 quiz"
		if meta.PreviousQuizzes > 1 {
			prev = fmt.Sprintf("%d quizzes", meta.PreviousQuizzes)
		}
		job.emit(Event{Type: "progress", Step: StepStart, Message: fmt.Sprintf(
			"Ya generaste %s con este requerimiento: las %d preguntas anteriores no se van a repetir.", prev, len(previous))})
	}
	req.Params = [][2]string{
		{"count", fmt.Sprint(req.Count)},
		{"language", req.Language},
		{"difficulty", req.Difficulty},
	}
	req.FixHint = fmt.Sprintf("The file must have exactly %d questions.", req.Count)
	if req.AvoidPath != "" {
		req.Params = append(req.Params, [2]string{"avoid", req.Rel(req.AvoidPath)})
		req.FixHint += "\nRepeated questions must be replaced by questions about different concepts that are NOT in " +
			req.Rel(req.AvoidPath) + " (not even rephrased)."
	}

	var q *quiz.Quiz
	t.req = req
	t.validate = func() (err error) {
		q, err = loadValid(req.OutputPath, req.Count, previous)
		return err
	}
	t.finish = func(e Engine) error { return m.finish(meta, q, e) }
	return t, nil
}

// execute tries each planned engine in order; in auto mode it falls back to
// the next one on any failure.
func (m *Manager) execute(meta *store.Meta, job *Job, t task) error {
	engines, err := m.plan(meta.EngineChoice)
	if err != nil {
		return err
	}
	auto := meta.EngineChoice == "" || meta.EngineChoice == ChoiceAuto
	noun := t.req.Labels.Noun

	// Fresh session check: the user may have logged in/out since the page loaded.
	statuses := m.AuthStatuses(context.Background(), true)

	var failures []string
	for _, engine := range engines {
		if st := statuses[engine.ID()]; !st.Usable() {
			err := notLoggedIn(engine, st)
			if !auto {
				return err
			}
			job.emit(Event{Type: "progress", Step: StepStart, Engine: engine.ID(), Message: err.Error() + ". Se omite."})
			failures = append(failures, engine.Name()+": "+err.Error())
			continue
		}
		if len(failures) > 0 {
			job.emit(Event{Type: "progress", Step: StepStart, Engine: engine.ID(), Message: fmt.Sprintf(
				"%s. Probando con %s…", short(failures[len(failures)-1]), engine.Name())})
		} else {
			job.emit(Event{Type: "progress", Step: StepStart, Engine: engine.ID(), Message: "Iniciando " + engine.Name() + "…"})
		}
		err := m.attempt(engine, t, job)
		if err == nil {
			return t.finish(engine)
		}
		log.Printf("%s %s: %s falló: %v", t.kind, meta.ID, engine.Name(), err)
		failures = append(failures, engine.Name()+" no pudo generar "+noun+": "+err.Error())
	}
	if len(failures) == 1 {
		return errors.New(failures[0])
	}
	return errors.New("ningún motor pudo generar " + noun + ". " + strings.Join(failures, " — "))
}

// attempt runs one engine: a first try and, if the result is invalid, one
// retry telling the engine what was wrong.
func (m *Manager) attempt(engine Engine, t task, job *Job) error {
	ctx, cancel := context.WithTimeout(context.Background(), m.Timeout)
	defer cancel()
	progress := func(step, msg string) {
		job.emit(Event{Type: "progress", Step: step, Message: msg, Engine: engine.ID()})
	}
	req := t.req
	_ = os.Remove(req.OutputPath) // never validate a file left by another engine

	session, err := engine.Run(ctx, req, "", progress)
	if err != nil {
		return timeoutErr(ctx, m.Timeout, err)
	}
	progress(StepValidate, "Validando el resultado…")
	verr := t.validate()
	if verr == nil {
		return nil
	}

	progress(StepValidate, "Hay cosas para corregir, pidiéndole a "+engine.Name()+" que las arregle…")
	req.Fix = verr.Error()
	if _, err := engine.Run(ctx, req, session, progress); err != nil {
		return timeoutErr(ctx, m.Timeout, err)
	}
	progress(StepValidate, "Validando el resultado…")
	return t.validate()
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
