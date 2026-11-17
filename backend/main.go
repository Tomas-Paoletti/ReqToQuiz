// ReqToQuiz: local web app that turns a job requirement into a multiple-choice
// practice quiz using Claude Code headless and the job-quiz skill.
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"reqtoquiz/internal/api"
	"reqtoquiz/internal/runner"
	"reqtoquiz/internal/store"
	"reqtoquiz/web"
)

func main() {
	port := flag.Int("port", 8080, "puerto HTTP (solo escucha en localhost)")
	root := flag.String("root", "", "raíz del repo (donde está .claude/skills/job-quiz); se detecta sola")
	engine := flag.String("engine", "auto", "motor: auto (Claude Code y, si falla, Codex), claude o codex")
	model := flag.String("model", "", "modelo para Claude Code (opcional, ej: sonnet, opus)")
	codexModel := flag.String("codex-model", "", "modelo para Codex (opcional)")
	claudeBin := flag.String("claude", "claude", "ejecutable de Claude Code")
	codexBin := flag.String("codex", "codex", "ejecutable de Codex")
	open := flag.Bool("open", false, "abrir el navegador al iniciar")
	flag.Parse()

	repo, err := findRoot(*root)
	if err != nil {
		log.Fatal(err)
	}
	engines, err := detectEngines(*engine, *claudeBin, *model, *codexBin, *codexModel)
	if err != nil {
		log.Fatal(err)
	}

	st, err := store.New(filepath.Join(repo, "data", "quizzes"))
	if err != nil {
		log.Fatal(err)
	}
	markInterrupted(st)

	srv := &api.Server{Store: st, Runner: runner.NewManager(repo, engines, st), Web: web.FS()}
	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("no se pudo escuchar en %s: %v", addr, err)
	}
	url := "http://localhost:" + fmt.Sprint(*port)
	log.Printf("ReqToQuiz en %s (repo: %s)", url, repo)
	if *open {
		go openBrowser(url)
	}
	s := &http.Server{Handler: srv.Routes(), ReadHeaderTimeout: 10 * time.Second}
	log.Fatal(s.Serve(ln))
}

// detectEngines returns the engines to use, in fallback order, keeping only
// the ones installed. In auto mode Claude Code goes first and Codex is the
// fallback; it is enough for one of them to be installed.
func detectEngines(mode, claudeBin, claudeModel, codexBin, codexModel string) ([]runner.Engine, error) {
	var wanted []string
	switch mode {
	case "auto":
		wanted = []string{"claude", "codex"}
	case "claude", "codex":
		wanted = []string{mode}
	default:
		return nil, fmt.Errorf("-engine inválido %q: usá auto, claude o codex", mode)
	}
	var engines []runner.Engine
	for _, w := range wanted {
		switch w {
		case "claude":
			if p, err := exec.LookPath(claudeBin); err == nil {
				engines = append(engines, &runner.Claude{Path: p, Model: claudeModel})
				log.Printf("motor disponible: Claude Code (%s)", p)
			} else {
				log.Printf("motor no disponible: Claude Code (no se encontró %q en el PATH)", claudeBin)
			}
		case "codex":
			if p, err := exec.LookPath(codexBin); err == nil {
				engines = append(engines, &runner.Codex{Path: p, Model: codexModel})
				log.Printf("motor disponible: Codex (%s)", p)
			} else {
				log.Printf("motor no disponible: Codex (no se encontró %q en el PATH)", codexBin)
			}
		}
	}
	if len(engines) == 0 {
		return nil, errors.New("no se encontró ningún motor. Instalá Claude Code (https://claude.com/claude-code) " +
			"o Codex (`npm install -g @openai/codex`) e iniciá sesión (`claude` / `codex login`)")
	}
	if mode == "auto" && len(engines) == 1 {
		log.Printf("sin motor de respaldo: solo se usará %s", engines[0].Name())
	}
	return engines, nil
}

// findRoot locates the repo root by looking for the job-quiz skill, starting
// from the flag, the working directory and the executable's directory.
func findRoot(flagRoot string) (string, error) {
	const marker = ".claude/skills/job-quiz/SKILL.md"
	var starts []string
	if flagRoot != "" {
		starts = append(starts, flagRoot)
	} else {
		if wd, err := os.Getwd(); err == nil {
			starts = append(starts, wd)
		}
		if exe, err := os.Executable(); err == nil {
			starts = append(starts, filepath.Dir(exe))
		}
	}
	for _, start := range starts {
		dir, _ := filepath.Abs(start)
		for {
			if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(marker))); err == nil {
				return dir, nil
			}
			parent := filepath.Dir(dir)
			if parent == dir || flagRoot != "" {
				break
			}
			dir = parent
		}
	}
	return "", errors.New("no se encontró la raíz del repo (" + marker + "); ejecutá desde el repo o usá -root")
}

// markInterrupted flags quizzes left "generating" by a previous run.
func markInterrupted(st *store.Store) {
	items, err := st.List()
	if err != nil {
		return
	}
	for _, m := range items {
		if m.Status == store.StatusGenerating {
			_, _ = st.Update(m.ID, func(m *store.Meta) {
				m.Status = store.StatusError
				m.Error = "la generación se interrumpió porque se cerró el servidor"
			})
		}
	}
}

func openBrowser(url string) {
	time.Sleep(300 * time.Millisecond)
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}
