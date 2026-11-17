package runner

import (
	"bufio"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"reqtoquiz/internal/pdftext"
)

// Codex runs OpenAI's Codex CLI headless (`codex exec`) as a fallback. It has
// no access to Claude's Skill tool, so the prompt tells it to read the same
// SKILL.md. It runs in a read-only sandbox: instead of writing the file, its
// final answer is constrained by --output-schema and saved with -o.
type Codex struct {
	Path  string
	Model string
}

func (c *Codex) ID() string   { return "codex" }
func (c *Codex) Name() string { return "Codex" }

//go:embed codex_schema.json
var codexSchema []byte

// codexLine is the subset of `codex exec --json` events we use.
type codexLine struct {
	Type     string `json:"type"`
	ThreadID string `json:"thread_id"`
	Message  string `json:"message"`
	Error    struct {
		Message string `json:"message"`
	} `json:"error"`
	Item struct {
		Type    string `json:"type"`
		Command string `json:"command"`
		Message string `json:"message"`
	} `json:"item"`
}

func (c *Codex) Run(ctx context.Context, req Request, _ string, progress Progress) (string, error) {
	progress(StepSkill, "Codex: preparando la skill job-quiz…")

	input := req.InputPath
	if strings.EqualFold(filepath.Ext(input), ".pdf") {
		// Codex cannot read PDFs: hand it the extracted text instead.
		txt := filepath.Join(req.QuizDir, "input-extracted.txt")
		if _, err := os.Stat(txt); err != nil {
			text, err := pdftext.Extract(input)
			if err != nil {
				return "", fmt.Errorf("Codex necesita el texto del PDF: %w", err)
			}
			if err := os.WriteFile(txt, []byte(text), 0o644); err != nil {
				return "", err
			}
		}
		input = txt
	}
	schema := filepath.Join(req.QuizDir, "codex-schema.json")
	if err := os.WriteFile(schema, codexSchema, 0o644); err != nil {
		return "", err
	}
	_ = os.Remove(req.OutputPath)

	avoid := ""
	if req.AvoidPath != "" {
		avoid = "avoid: " + req.Rel(req.AvoidPath) + "\n"
	}
	prompt := fmt.Sprintf(`You are following the "job-quiz" skill of this repository.
First read .claude/skills/job-quiz/SKILL.md and .claude/skills/job-quiz/schema.md and follow them exactly.

input: %s
count: %d
language: %s
difficulty: %s
%s
Differences from the skill instructions:
- Do NOT write or modify any file. Return the quiz JSON as your final answer; it is validated against the provided output schema and saved for you.
- There is no human in the loop: do not ask questions.
`, req.Rel(input), req.Count, req.Language, req.Difficulty, avoid)
	if req.Fix != "" {
		prompt += "\nA previous attempt was rejected for these problems, make sure the new quiz does not have them: " + req.Fix + "\n"
	}

	args := []string{
		"exec", "--json",
		"--sandbox", "read-only",
		"--skip-git-repo-check",
		"--ephemeral",
		"--cd", req.Root,
		"--output-schema", schema,
		"--output-last-message", req.OutputPath,
	}
	if c.Model != "" {
		args = append(args, "--model", c.Model)
	}
	args = append(args, "-") // prompt from stdin

	cmd, stdout, stderr, err := startCmd(ctx, c.Path, args, req.Root, prompt)
	if err != nil {
		return "", fmt.Errorf("no se pudo ejecutar Codex (%s): %w", c.Path, err)
	}

	var (
		thread    string
		failed    string
		lastError string
		step      = StepSkill
	)
	setStep := func(s, msg string) {
		if s != step {
			step = s
			progress(s, msg)
		}
	}
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 64<<10), 32<<20)
	for sc.Scan() {
		var ev codexLine
		if json.Unmarshal(sc.Bytes(), &ev) != nil {
			continue
		}
		switch ev.Type {
		case "thread.started":
			thread = ev.ThreadID
		case "item.started", "item.completed":
			switch ev.Item.Type {
			case "command_execution":
				cmdline := filepath.ToSlash(ev.Item.Command)
				if strings.Contains(cmdline, "input") && step != StepGenerate {
					setStep(StepRead, "Codex: leyendo el requerimiento del puesto…")
				}
			case "reasoning":
				if step == StepRead {
					setStep(StepGenerate, "Codex: pensando las preguntas…")
				}
			case "agent_message":
				setStep(StepWrite, "Codex: devolviendo el quiz…")
			case "error":
				lastError = ev.Item.Message
			}
		case "error":
			lastError = ev.Message
		case "turn.failed":
			failed = ev.Error.Message
		}
	}
	waitErr := cmd.Wait()

	if err := ctx.Err(); err != nil {
		return thread, err
	}
	if failed == "" && waitErr != nil {
		failed = lastError
		if failed == "" {
			failed = lastLines(stderr.String(), 3)
		}
		if failed == "" {
			failed = waitErr.Error()
		}
	}
	if failed != "" {
		_ = os.Remove(req.OutputPath) // -o leaves an empty file behind on failure
		if strings.Contains(failed, "401") || strings.Contains(strings.ToLower(failed), "unauthorized") {
			return thread, errors.New("Codex no tiene sesión iniciada: ejecutá `codex login` en una terminal")
		}
		return thread, errors.New("Codex falló: " + failed)
	}
	if fi, err := os.Stat(req.OutputPath); err != nil || fi.Size() == 0 {
		_ = os.Remove(req.OutputPath)
		return thread, errors.New("Codex terminó sin devolver el quiz")
	}
	return thread, nil
}
