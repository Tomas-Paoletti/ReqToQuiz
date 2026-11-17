package runner

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// Claude runs Claude Code headless (`claude -p`). It uses the job-quiz skill
// directly, reads PDFs natively and writes questions.json itself.
type Claude struct {
	Path  string
	Model string
}

func (c *Claude) ID() string   { return "claude" }
func (c *Claude) Name() string { return "Claude Code" }

func (c *Claude) prompt(req Request, session string) string {
	if session != "" && req.Fix != "" {
		p := fmt.Sprintf("The file %s is invalid: %s\nFix every problem and rewrite the whole file so it matches schema.md exactly, with exactly %d questions.",
			req.Rel(req.OutputPath), req.Fix, req.Count)
		if req.AvoidPath != "" {
			p += "\nRepeated questions must be replaced by questions about different concepts that are NOT in " + req.Rel(req.AvoidPath) + " (not even rephrased)."
		}
		return p
	}
	avoid := ""
	if req.AvoidPath != "" {
		avoid = "avoid: " + req.Rel(req.AvoidPath) + "\n"
	}
	p := fmt.Sprintf(`Use the job-quiz skill to create the quiz.
input: %s
count: %d
language: %s
difficulty: %s
output: %s
%s
There is no human in the loop: do not ask questions, just write the output file.`,
		req.Rel(req.InputPath), req.Count, req.Language, req.Difficulty, req.Rel(req.OutputPath), avoid)
	if req.Fix != "" { // retry without a session to resume
		p += "\nA previous attempt was rejected for these problems, avoid them: " + req.Fix
	}
	return p
}

// claudeLine is the subset of Claude Code's stream-json events we use.
type claudeLine struct {
	Type      string `json:"type"`
	Subtype   string `json:"subtype"`
	SessionID string `json:"session_id"`
	IsError   bool   `json:"is_error"`
	Result    string `json:"result"`
	Message   struct {
		Content []struct {
			Type  string          `json:"type"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		} `json:"content"`
	} `json:"message"`
	Event struct {
		Type         string `json:"type"`
		ContentBlock struct {
			Type string `json:"type"`
			Name string `json:"name"`
		} `json:"content_block"`
		Delta struct {
			Type        string `json:"type"`
			PartialJSON string `json:"partial_json"`
		} `json:"delta"`
	} `json:"event"`
}

func (c *Claude) Run(ctx context.Context, req Request, session string, progress Progress) (string, error) {
	args := []string{
		"-p",
		"--output-format", "stream-json", "--verbose", "--include-partial-messages",
		"--tools", "Read,Write,Skill",
		"--allowedTools", "Read,Write,Skill",
		"--permission-mode", "acceptEdits",
		"--strict-mcp-config",
	}
	if c.Model != "" {
		args = append(args, "--model", c.Model)
	}
	if session != "" {
		args = append(args, "--resume", session)
	}
	cmd, stdout, stderr, err := startCmd(ctx, c.Path, args, req.Root, c.prompt(req, session))
	if err != nil {
		return "", fmt.Errorf("no se pudo ejecutar Claude Code (%s): %w", c.Path, err)
	}

	var (
		sessionID = session
		result    *claudeLine
		step      string
		partial   strings.Builder // JSON of the Write tool call being streamed
		inWrite   bool
		lastCount = -1
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
		var ev claudeLine
		if json.Unmarshal(sc.Bytes(), &ev) != nil {
			continue
		}
		if ev.SessionID != "" {
			sessionID = ev.SessionID
		}
		switch ev.Type {
		case "stream_event":
			switch ev.Event.Type {
			case "content_block_start":
				if ev.Event.ContentBlock.Type == "thinking" && (step == StepRead || step == StepSkill) {
					setStep(StepGenerate, "Analizando el puesto y pensando las preguntas…")
				}
				inWrite = ev.Event.ContentBlock.Type == "tool_use" && ev.Event.ContentBlock.Name == "Write"
				if inWrite {
					partial.Reset()
					lastCount = -1
					setStep(StepGenerate, "Escribiendo las preguntas…")
				}
			case "content_block_delta":
				if inWrite && ev.Event.Delta.Type == "input_json_delta" {
					partial.WriteString(ev.Event.Delta.PartialJSON)
					// Inside the Write input the file content is an escaped JSON string.
					n := strings.Count(partial.String(), `\"question\"`)
					if n != lastCount && n > 0 {
						lastCount = n
						progress(StepGenerate, fmt.Sprintf("Escribiendo pregunta %d de %d…", min(n, req.Count), req.Count))
					}
				}
			case "content_block_stop":
				inWrite = false
			}
		case "assistant":
			for _, b := range ev.Message.Content {
				if b.Type != "tool_use" {
					continue
				}
				switch b.Name {
				case "Skill":
					setStep(StepSkill, "Cargando la skill job-quiz…")
				case "Read":
					var in struct {
						FilePath string `json:"file_path"`
					}
					_ = json.Unmarshal(b.Input, &in)
					if strings.Contains(filepath.ToSlash(in.FilePath), "/input.") {
						setStep(StepRead, "Leyendo el requerimiento del puesto…")
					}
				case "Write":
					setStep(StepWrite, "Guardando el quiz…")
				}
			}
		case "result":
			r := ev
			result = &r
		}
	}
	scanErr := sc.Err()
	waitErr := cmd.Wait()

	if err := ctx.Err(); err != nil {
		return sessionID, err
	}
	if result != nil && result.IsError {
		msg := strings.TrimSpace(result.Result)
		if msg == "" {
			msg = result.Subtype
		}
		return sessionID, fmt.Errorf("Claude Code devolvió un error: %s", msg)
	}
	if waitErr != nil || result == nil {
		detail := lastLines(stderr.String(), 3)
		if detail == "" && waitErr != nil {
			detail = waitErr.Error()
		}
		if detail == "" && scanErr != nil {
			detail = scanErr.Error()
		}
		if detail == "" {
			detail = "terminó sin resultado"
		}
		return sessionID, errors.New("Claude Code falló: " + detail)
	}
	return sessionID, nil
}
