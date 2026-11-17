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

// Request is everything an engine needs for one generation attempt.
// Paths are absolute.
type Request struct {
	Root       string // repo root (cwd of the engine, holds .claude/skills/job-quiz)
	QuizDir    string // data/quizzes/<id>
	InputPath  string // requirement file (.pdf, .md, .txt)
	OutputPath string // where questions.json must end up
	AvoidPath  string // optional list of already-tested concepts
	Count      int
	Language   string
	Difficulty string
	// Fix is set on a retry: the validation errors of the previous attempt.
	Fix string
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

// Engine is a headless coding agent able to follow the job-quiz skill.
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
