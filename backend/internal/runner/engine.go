package runner

import (
	"bytes"
	"context"
	"io"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Input is a file the skill must read.
type Input struct {
	Key   string // prompt key, e.g. "input", "cv"
	Path  string // absolute path
	Label string // shown in progress messages, e.g. "el requerimiento del puesto"
}

// Labels are the progress messages that depend on the task.
type Labels struct {
	Think string // engine starts reasoning, e.g. "Analizando el puesto y pensando las preguntas…"
	Write string // engine starts writing the output
	Save  string // output file is being saved
	Noun  string // what is produced, e.g. "el quiz", "el informe"
}

// Request is everything an engine needs for one generation attempt.
// Paths are absolute.
type Request struct {
	Root       string // repo root (cwd of the engine, holds .claude/skills/<skill>)
	QuizDir    string // data/quizzes/<id>
	Skill      string // skill to follow: "job-quiz" | "ats-check"
	Inputs     []Input
	Params     [][2]string // extra "key: value" lines for the prompt, in order
	OutputPath string      // where the JSON must end up
	Labels     Labels
	// CodexSchema is the strict JSON schema for Codex's final answer.
	CodexSchema []byte
	// Count, if > 0, is the number of questions: used for "pregunta N de M".
	Count int
	// FixHint is appended to the retry prompt (task-specific advice).
	FixHint string
	// Fix is set on a retry: the validation errors of the previous attempt.
	Fix string

	// Quiz-only fields, kept for the quiz task and its tests.
	Language   string
	Difficulty string
	AvoidPath  string
}

// inputLabel returns the label of the input at path (as reported by a tool
// call), or "" if path is not one of the inputs.
func (r Request) inputLabel(path string) string {
	path = strings.ToLower(filepath.ToSlash(path))
	for _, in := range r.Inputs {
		if strings.HasSuffix(path, "/"+strings.ToLower(filepath.Base(in.Path))) {
			return in.Label
		}
	}
	return ""
}

// paramLines renders inputs and params as "key: value" prompt lines.
func (r Request) paramLines(inputPath func(Input) string) string {
	var b strings.Builder
	for _, in := range r.Inputs {
		b.WriteString(in.Key + ": " + r.Rel(inputPath(in)) + "\n")
	}
	for _, kv := range r.Params {
		b.WriteString(kv[0] + ": " + kv[1] + "\n")
	}
	return b.String()
}

// Rel returns p relative to the repo root with forward slashes, which is what
// the engines get in their prompts.
func (r Request) Rel(p string) string {
	rel, err := filepath.Rel(r.Root, p)
	if err != nil {
		return filepath.ToSlash(p)
	}
	return filepath.ToSlash(rel)
}

// Progress reports a step change or a message to the UI.
type Progress func(step, message string)

// Engine is a headless coding agent able to follow the repo skills.
type Engine interface {
	ID() string   // "claude" | "codex"
	Name() string // shown to the user
	// Run performs one attempt. session is "" on the first attempt and the
	// value returned by the previous Run on a retry (engines that cannot
	// resume ignore it and start over using req.Fix).
	Run(ctx context.Context, req Request, session string, progress Progress) (newSession string, err error)
}

// startCmd prepares a command that runs in the repo root, reads the prompt
// from stdin, kills its whole process tree on cancel and keeps a bounded
// copy of stderr.
func startCmd(ctx context.Context, bin string, args []string, dir, prompt string) (*exec.Cmd, io.ReadCloser, *bytes.Buffer, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(prompt)
	cmd.Cancel = func() error { return killTree(cmd) }
	cmd.WaitDelay = 5 * time.Second
	stderr := &bytes.Buffer{}
	cmd.Stderr = &limitedWriter{w: stderr, n: 16 << 10}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, nil, err
	}
	return cmd, stdout, stderr, nil
}

// killTree kills the process and its children. On Windows the CLIs are
// usually npm .cmd shims, so killing only cmd.exe would leave node running.
func killTree(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	if runtime.GOOS == "windows" {
		_ = exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
		return nil
	}
	return cmd.Process.Kill()
}

type limitedWriter struct {
	w io.Writer
	n int
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	if l.n > 0 {
		k := min(len(p), l.n)
		_, _ = l.w.Write(p[:k])
		l.n -= k
	}
	return len(p), nil
}

// lastLines returns the last n non-empty lines of s (for error messages).
func lastLines(s string, n int) string {
	var lines []string
	for _, l := range strings.Split(strings.TrimSpace(s), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, " | ")
}
