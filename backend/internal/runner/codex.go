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
	progress(StepSkill, "Codex: preparando la skill "+req.Skill+"…")

	// Codex cannot read PDFs: hand it the extracted text instead.
	paths := map[string]string{}
	for _, in := range req.Inputs {
		paths[in.Key] = in.Path
		if !strings.EqualFold(filepath.Ext(in.Path), ".pdf") {
			continue
		}
		txt := filepath.Join(req.QuizDir, in.Key+"-extracted.txt")
		if _, err := os.Stat(txt); err != nil {
			text, err := pdftext.Extract(in.Path)
			if err != nil {
				return "", fmt.Errorf("Codex necesita el texto del PDF (%s): %w", in.Label, err)
			}
			if err := os.WriteFile(txt, []byte(text), 0o644); err != nil {
				return "", err
			}
		}
		paths[in.Key] = txt
	}
	schemaJSON := req.CodexSchema
	if len(schemaJSON) == 0 {
		schemaJSON = codexSchema
	}
	schema := filepath.Join(req.QuizDir, "codex-schema.json")
	if err := os.WriteFile(schema, schemaJSON, 0o644); err != nil {
		return "", err
	}
	_ = os.Remove(req.OutputPath)

	prompt := fmt.Sprintf(`You are following the "%[1]s" skill of this repository.
First read .claude/skills/%[1]s/SKILL.md and .claude/skills/%[1]s/schema.md and follow them exactly.

%[2]s
Differences from the skill instructions:
- Do NOT write or modify any file. Return the JSON as your final answer; it is validated against the provided output schema and saved for you.
- PDF inputs were converted to plain text for you; read those text files instead.
- There is no human in the loop: do not ask questions.
`, req.Skill, req.paramLines(func(in Input) string { return paths[in.Key] }))
	if req.Fix != "" {
		prompt += "\nA previous attempt was rejected for these problems, make sure the new answer does not have them: " + req.Fix + "\n"
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
				if step != StepGenerate {
					for _, in := range req.Inputs {
						if strings.Contains(ev.Item.Command, strings.TrimSuffix(filepath.Base(paths[in.Key]), filepath.Ext(paths[in.Key]))) {
							step = ""
							setStep(StepRead, "Codex: leyendo "+in.Label+"…")
						}
					}
				}
			case "reasoning":
				if step == StepRead {
					setStep(StepGenerate, "Codex: "+lowerFirst(req.Labels.Think))
				}
			case "agent_message":
				setStep(StepWrite, "Codex: devolviendo "+req.Labels.Noun+"…")
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
		return thread, errors.New("Codex terminó sin devolver " + req.Labels.Noun)
	}
	return thread, nil
}

func lowerFirst(s string) string {
	r := []rune(s)
	if len(r) > 0 {
		r[0] = []rune(strings.ToLower(string(r[0])))[0]
	}
	return string(r)
}
