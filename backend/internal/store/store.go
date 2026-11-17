// Package store keeps every quiz in its own folder under data/quizzes/<id>/.
// No database: meta.json holds the state, questions.json the generated
// questions and quiz.html the rendered quiz.
package store

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	StatusGenerating = "generating"
	StatusReady      = "ready"
	StatusError      = "error"
)

type Attempt struct {
	Score   float64   `json:"score"`
	Correct int       `json:"correct"`
	Total   int       `json:"total"`
	At      time.Time `json:"at"`
}

type Meta struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Role       string `json:"role"`
	Seniority  string `json:"seniority"`
	Count      int    `json:"count"`
	Language   string `json:"language"`
	Difficulty string `json:"difficulty"`
	Source     string `json:"source"`    // original filename or "texto pegado"
	InputFile  string `json:"inputFile"` // file name inside the quiz folder
	InputHash  string `json:"inputHash"` // see HashInput: same requirement ⇒ same hash
	// PreviousQuizzes is how many earlier quizzes used the same requirement;
	// their questions were excluded from this one.
	PreviousQuizzes int       `json:"previousQuizzes"`
	Engine          string    `json:"engine,omitempty"`       // "claude" | "codex": which one generated it
	EngineChoice    string    `json:"engineChoice,omitempty"` // what the user picked: "auto" | "claude" | "codex"
	Status          string    `json:"status"`
	Error           string    `json:"error,omitempty"`
	CreatedAt       time.Time `json:"createdAt"`
	LastAttempt     *Attempt  `json:"lastAttempt,omitempty"`
	Attempts        int       `json:"attempts"`
}

var ErrNotFound = errors.New("quiz not found")

var idPattern = regexp.MustCompile(`^[0-9]{8}-[0-9]{6}-[0-9a-f]{6}$`)

type Store struct {
	Dir string // absolute path to data/quizzes
	mu  sync.Mutex
}

func New(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Store{Dir: dir}, nil
}

func NewID() string {
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	return time.Now().Format("20060102-150405") + "-" + hex.EncodeToString(b)
}

func ValidID(id string) bool { return idPattern.MatchString(id) }

// HashInput identifies a requirement regardless of how it was provided.
// Text inputs (.md, .txt, pasted text) are normalized first (case, whitespace,
// line endings) so an uploaded .md and the same text pasted match. PDFs are
// hashed byte by byte, so the same PDF file matches itself.
func HashInput(inputName string, data []byte) string {
	if strings.ToLower(filepath.Ext(inputName)) != ".pdf" {
		text := string(bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})) // UTF-8 BOM
		data = []byte(strings.Join(strings.Fields(strings.ToLower(text)), " "))
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// EnsureHash fills InputHash for quizzes created before it existed.
func (s *Store) EnsureHash(m *Meta) {
	if m.InputHash != "" || m.InputFile == "" {
		return
	}
	data, err := os.ReadFile(s.Path(m.ID, m.InputFile))
	if err != nil {
		return
	}
	m.InputHash = HashInput(m.InputFile, data)
	_ = s.Save(m)
}

// SameInput returns the ready quizzes (other than exceptID) generated from
// the same requirement, oldest first.
func (s *Store) SameInput(hash, exceptID string) ([]*Meta, error) {
	all, err := s.List()
	if err != nil {
		return nil, err
	}
	var out []*Meta
	for i := len(all) - 1; i >= 0; i-- {
		m := all[i]
		if m.ID == exceptID || m.Status != StatusReady {
			continue
		}
		s.EnsureHash(m)
		if m.InputHash == hash {
			out = append(out, m)
		}
	}
	return out, nil
}

// Path returns the absolute path of a file inside a quiz folder.
func (s *Store) Path(id string, name ...string) string {
	return filepath.Join(append([]string{s.Dir, id}, name...)...)
}

func (s *Store) Create(m *Meta, inputName string, input []byte) error {
	if err := os.MkdirAll(s.Path(m.ID), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(s.Path(m.ID, inputName), input, 0o644); err != nil {
		return err
	}
	m.InputFile = inputName
	m.InputHash = HashInput(inputName, input)
	return s.Save(m)
}

func (s *Store) Save(m *Meta) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.Path(m.ID, "meta.json.tmp")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.Path(m.ID, "meta.json"))
}

func (s *Store) Get(id string) (*Meta, error) {
	if !ValidID(id) {
		return nil, ErrNotFound
	}
	data, err := os.ReadFile(s.Path(id, "meta.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var m Meta
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("meta.json de %s corrupto: %w", id, err)
	}
	return &m, nil
}

// Update loads a quiz, applies fn and saves it.
func (s *Store) Update(id string, fn func(*Meta)) (*Meta, error) {
	m, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	fn(m)
	return m, s.Save(m)
}

// List returns all quizzes, newest first. Broken folders are skipped.
func (s *Store) List() ([]*Meta, error) {
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		return nil, err
	}
	out := []*Meta{}
	for _, e := range entries {
		if !e.IsDir() || !ValidID(e.Name()) {
			continue
		}
		if m, err := s.Get(e.Name()); err == nil {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) Delete(id string) error {
	if !ValidID(id) {
		return ErrNotFound
	}
	return os.RemoveAll(s.Path(id))
}
