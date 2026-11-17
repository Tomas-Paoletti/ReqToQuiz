// Package api exposes the HTTP endpoints used by the Vue app and the quiz page.
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
	mux.HandleFunc("GET /quizzes/{id}/{$}", s.quizPage)
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

// health reports which engines are available, in fallback order.
func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	type engine struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	out := []engine{}
	for _, e := range s.Runner.Engines {
		out = append(out, engine{e.ID(), e.Name()})
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
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload+1<<20)
	if err := r.ParseMultipartForm(maxUpload); err != nil {
		writeErr(w, http.StatusBadRequest, "el archivo es demasiado grande (máx. 20 MB) o el formulario es inválido")
		return
	}
	count, _ := strconv.Atoi(r.FormValue("count"))
	lang, diff := r.FormValue("language"), r.FormValue("difficulty")
	if !validCounts[count] {
		writeErr(w, http.StatusBadRequest, "cantidad de preguntas inválida (5, 10, 20, 30 o 40)")
		return
	}
	if !validLanguages[lang] {
		writeErr(w, http.StatusBadRequest, "idioma inválido")
		return
	}
	if !validDifficulty[diff] {
		writeErr(w, http.StatusBadRequest, "dificultad inválida")
		return
	}

	var (
		input     []byte
		inputName string
		source    string
	)
	if file, hdr, err := r.FormFile("file"); err == nil {
		defer file.Close()
		ext := strings.ToLower(filepath.Ext(hdr.Filename))
		if !allowedInputExts[ext] {
			writeErr(w, http.StatusBadRequest, "formato no soportado: subí un .pdf, .md o .txt")
			return
		}
		if ext == ".markdown" {
			ext = ".md"
		}
		if input, err = io.ReadAll(file); err != nil {
			writeErr(w, http.StatusBadRequest, "no se pudo leer el archivo")
			return
		}
		if ext == ".pdf" && !strings.HasPrefix(string(input[:min(len(input), 5)]), "%PDF") {
			writeErr(w, http.StatusBadRequest, "el archivo no parece ser un PDF válido")
			return
		}
		inputName, source = "input"+ext, filepath.Base(hdr.Filename)
	} else if text := strings.TrimSpace(r.FormValue("text")); text != "" {
		input, inputName, source = []byte(text), "input.md", "texto pegado"
	} else {
		writeErr(w, http.StatusBadRequest, "subí un archivo o pegá el texto del requerimiento")
		return
	}
	if len(strings.TrimSpace(string(input))) < 40 && !strings.HasSuffix(inputName, ".pdf") {
		writeErr(w, http.StatusBadRequest, "el requerimiento es demasiado corto")
		return
	}

	m := &store.Meta{
		ID: store.NewID(), Count: count, Language: lang, Difficulty: diff,
		Source: source, Status: store.StatusGenerating, CreatedAt: time.Now(),
		Title: strings.TrimSuffix(source, filepath.Ext(source)),
	}
	if err := s.Store.Create(m, inputName, input); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.Runner.Start(m)
	writeJSON(w, http.StatusCreated, m)
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
			send(runner.Event{Type: "done", Step: runner.StepDone, URL: "/quizzes/" + id + "/"})
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
	id := r.PathValue("id")
	m, err := s.Store.Get(id)
	if err != nil || m.Status != store.StatusReady {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, s.Store.Path(id, "quiz.html"))
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
