// Package api exposes the HTTP endpoints used by the Vue app, the quiz page
// and the ATS report.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"reqtoquiz/internal/ats"
	"reqtoquiz/internal/runner"
	"reqtoquiz/internal/store"
)

const maxUpload = 20 << 20 // 20 MB

var (
	validCounts      = map[int]bool{5: true, 10: true, 20: true, 30: true, 40: true}
	validLanguages   = map[string]bool{"es": true, "en": true}
	validDifficulty  = map[string]bool{"junior": true, "semi": true, "senior": true, "mixed": true}
	allowedInputExts = map[string]bool{".pdf": true, ".md": true, ".markdown": true, ".txt": true}
)

type Server struct {
	Store  *store.Store
	Runner *runner.Manager
	Web    fs.FS // built Vue app (index.html + assets); may be empty in dev
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/quizzes", s.list)
	mux.HandleFunc("POST /api/quizzes", s.create)
	mux.HandleFunc("GET /api/quizzes/{id}", s.get)
	mux.HandleFunc("DELETE /api/quizzes/{id}", s.delete)
	mux.HandleFunc("GET /api/quizzes/{id}/events", s.events)
	mux.HandleFunc("POST /api/quizzes/{id}/attempts", s.attempt)
	mux.HandleFunc("POST /api/ats", s.createATS)
	mux.HandleFunc("GET /api/ats/{id}/report", s.atsReport)
	mux.HandleFunc("GET /quizzes/{id}/{$}", s.quizPage)
	mux.HandleFunc("GET /ats/{id}/{$}", s.reportPage)
	mux.HandleFunc("GET /ats/{id}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ats/"+r.PathValue("id")+"/", http.StatusMovedPermanently)
	})
	mux.HandleFunc("GET /quizzes/{id}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/quizzes/"+r.PathValue("id")+"/", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/", s.spa)
	return logRequests(mux)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// health reports every known engine: installed or not and, if installed,
// whether it has a session. ?refresh=1 skips the short auth cache.
func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	type engine struct {
		ID        string             `json:"id"`
		Name      string             `json:"name"`
		Installed bool               `json:"installed"`
		Auth      *runner.AuthStatus `json:"auth,omitempty"`
	}
	statuses := s.Runner.AuthStatuses(r.Context(), r.URL.Query().Get("refresh") == "1")
	out := []engine{}
	for _, e := range s.Runner.Engines { // installed, in fallback order
		st := statuses[e.ID()]
		out = append(out, engine{ID: e.ID(), Name: e.Name(), Installed: true, Auth: &st})
	}
	for _, id := range s.Runner.Missing {
		out = append(out, engine{ID: id, Name: runner.EngineNames[id]})
	}
	writeJSON(w, http.StatusOK, map[string]any{"engines": out})
}

func (s *Server) list(w http.ResponseWriter, r *http.Request) {
	items, err := s.Store.List()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) get(w http.ResponseWriter, r *http.Request) {
	m, err := s.Store.Get(r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "quiz no encontrado")
		return
	} else if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (s *Server) create(w http.ResponseWriter, r *http.Request) {
	lang, engineChoice, ok := s.parseForm(w, r)
	if !ok {
		return
	}
	count, _ := strconv.Atoi(r.FormValue("count"))
	diff := r.FormValue("difficulty")
	if !validCounts[count] {
		writeErr(w, http.StatusBadRequest, "cantidad de preguntas inválida (5, 10, 20, 30 o 40)")
		return
	}
	if !validDifficulty[diff] {
		writeErr(w, http.StatusBadRequest, "dificultad inválida")
		return
	}

	input, inputName, source, msg := readUpload(r, "file", "text", "input", "requerimiento")
	if msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}

	m := &store.Meta{
		ID: store.NewID(), Count: count, Language: lang, Difficulty: diff,
		Source: source, Status: store.StatusGenerating, CreatedAt: time.Now(), EngineChoice: engineChoice,
		Title: strings.TrimSuffix(source, filepath.Ext(source)),
	}
	if err := s.Store.Create(m, inputName, input); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.Runner.Start(m)
	writeJSON(w, http.StatusCreated, m)
}

// readUpload reads an uploaded file (fileField) or, if there is none, pasted
// text (textField, "" to disallow). The file is stored as base+ext. what names
// the document in error messages. msg is non-empty on a user error.
func readUpload(r *http.Request, fileField, textField, base, what string) (data []byte, name, source, msg string) {
	if file, hdr, err := r.FormFile(fileField); err == nil {
		defer file.Close()
		ext := strings.ToLower(filepath.Ext(hdr.Filename))
		if !allowedInputExts[ext] {
			return nil, "", "", "formato no soportado para el " + what + ": subí un .pdf, .md o .txt"
		}
		if ext == ".markdown" {
			ext = ".md"
		}
		if data, err = io.ReadAll(file); err != nil {
			return nil, "", "", "no se pudo leer el archivo del " + what
		}
		if ext == ".pdf" && !strings.HasPrefix(string(data[:min(len(data), 5)]), "%PDF") {
			return nil, "", "", "el " + what + " no parece ser un PDF válido"
		}
		name, source = base+ext, filepath.Base(hdr.Filename)
	} else if text := strings.TrimSpace(r.FormValue(textField)); textField != "" && text != "" {
		data, name, source = []byte(text), base+".md", "texto pegado"
	} else if textField != "" {
		return nil, "", "", "subí un archivo o pegá el texto del " + what
	} else {
		return nil, "", "", "subí el " + what
	}
	if len(strings.TrimSpace(string(data))) < 40 && !strings.HasSuffix(name, ".pdf") {
		return nil, "", "", "el " + what + " es demasiado corto"
	}
	return data, name, source, ""
}

// parseForm parses the multipart form and the fields shared by every kind.
func (s *Server) parseForm(w http.ResponseWriter, r *http.Request) (lang, engine string, ok bool) {
	r.Body = http.MaxBytesReader(w, r.Body, 2*maxUpload+1<<20)
	if err := r.ParseMultipartForm(maxUpload); err != nil {
		writeErr(w, http.StatusBadRequest, "los archivos son demasiado grandes (máx. 20 MB cada uno) o el formulario es inválido")
		return "", "", false
	}
	lang = r.FormValue("language")
	if !validLanguages[lang] {
		writeErr(w, http.StatusBadRequest, "idioma inválido")
		return "", "", false
	}
	engine = r.FormValue("engine")
	if engine == "" {
		engine = runner.ChoiceAuto
	}
	// Fail fast: chosen engine not installed or without a session.
	if err := s.Runner.CheckChoice(r.Context(), engine); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return "", "", false
	}
	return lang, engine, true
}

// createATS starts an ATS check: job requirement (file or text) + CV (file).
func (s *Server) createATS(w http.ResponseWriter, r *http.Request) {
	lang, engine, ok := s.parseForm(w, r)
	if !ok {
		return
	}
	job, jobName, jobSource, msg := readUpload(r, "file", "text", "input", "requerimiento")
	if msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	cv, cvName, cvSource, msg := readUpload(r, "cv", "", "cv", "CV")
	if msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	m := &store.Meta{
		ID: store.NewID(), Kind: store.KindATS, Language: lang, EngineChoice: engine,
		Source: jobSource, CVFile: cvName, CVSource: cvSource,
		Status: store.StatusGenerating, CreatedAt: time.Now(),
		Title: "CV vs " + strings.TrimSuffix(jobSource, filepath.Ext(jobSource)),
	}
	if err := s.Store.Create(m, jobName, job); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.Store.WriteFile(m.ID, cvName, cv); err != nil {
		_ = s.Store.Delete(m.ID)
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.Runner.Start(m)
	writeJSON(w, http.StatusCreated, m)
}

// atsReport returns a ready ATS report (with the CV transcription and the
// suggested rewrites) for the CV editor.
func (s *Server) atsReport(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	m, err := s.Store.Get(id)
	if err != nil || m.Kind != store.KindATS || m.Status != store.StatusReady {
		writeErr(w, http.StatusNotFound, "análisis ATS no encontrado")
		return
	}
	rep, err := ats.Load(s.Store.Path(id, "ats.json"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	ats.Finalize(rep)
	writeJSON(w, http.StatusOK, map[string]any{"meta": m, "report": rep})
}

func (s *Server) delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if m, err := s.Store.Get(id); err == nil && m.Status == store.StatusGenerating && s.Runner.Get(id) != nil {
		writeErr(w, http.StatusConflict, "el quiz se está generando, esperá a que termine")
		return
	}
	if err := s.Store.Delete(id); err != nil {
		writeErr(w, http.StatusNotFound, "quiz no encontrado")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) attempt(w http.ResponseWriter, r *http.Request) {
	var a store.Attempt
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&a); err != nil || a.Total <= 0 || a.Correct < 0 || a.Correct > a.Total {
		writeErr(w, http.StatusBadRequest, "intento inválido")
		return
	}
	a.At = time.Now()
	m, err := s.Store.Update(r.PathValue("id"), func(m *store.Meta) {
		m.LastAttempt = &a
		m.Attempts++
	})
	if err != nil {
		writeErr(w, http.StatusNotFound, "quiz no encontrado")
		return
	}
	writeJSON(w, http.StatusOK, m)
}

// events streams generation progress as Server-Sent Events.
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	m, err := s.Store.Get(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "quiz no encontrado")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "streaming no soportado")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	send := func(e runner.Event) {
		data, _ := json.Marshal(e)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e.Type, data)
		flusher.Flush()
	}

	job := s.Runner.Get(id)
	if job == nil {
		// Not generating in this process: answer from what is on disk.
		switch m.Status {
		case store.StatusReady:
			send(runner.Event{Type: "done", Step: runner.StepDone, URL: runner.PageURL(m)})
		case store.StatusError:
			send(runner.Event{Type: "error", Message: m.Error})
		default:
			send(runner.Event{Type: "error", Message: "la generación se interrumpió (¿se reinició el servidor?). Volvé a generar el quiz."})
		}
		return
	}

	history, ch, unsubscribe := job.Subscribe()
	defer unsubscribe()
	last := ""
	for _, e := range history {
		send(e)
		last = e.Type
	}
	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case e, open := <-ch:
			if !open {
				// Make sure the final event is delivered even if it was dropped.
				if last == "progress" || last == "" {
					if h, _, _ := job.Subscribe(); len(h) > 0 {
						send(h[len(h)-1])
					}
				}
				return
			}
			send(e)
			last = e.Type
		}
	}
}

func (s *Server) quizPage(w http.ResponseWriter, r *http.Request) {
	s.page(w, r, store.KindQuiz, "quiz.html")
}

func (s *Server) reportPage(w http.ResponseWriter, r *http.Request) {
	s.page(w, r, store.KindATS, "report.html")
}

func (s *Server) page(w http.ResponseWriter, r *http.Request, kind, file string) {
	id := r.PathValue("id")
	m, err := s.Store.Get(id)
	if err != nil || m.Status != store.StatusReady || m.Kind != kind {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, s.Store.Path(id, file))
}

// spa serves the built Vue app, falling back to index.html for client routes.
func (s *Server) spa(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeErr(w, http.StatusNotFound, "endpoint no encontrado")
		return
	}
	p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if p != "" {
		if f, err := fs.Stat(s.Web, p); err == nil && !f.IsDir() {
			if strings.HasPrefix(p, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			http.ServeFileFS(w, r, s.Web, p)
			return
		}
	}
	index, err := fs.ReadFile(s.Web, "index.html")
	if err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<p>El frontend no está compilado. En desarrollo abrí <a href="http://localhost:5173">http://localhost:5173</a>, o ejecutá <code>npm run build</code>.</p>`)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(index)
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") && r.Method != http.MethodGet {
			log.Printf("%s %s", r.Method, r.URL.Path)
		}
		next.ServeHTTP(w, r)
	})
}
