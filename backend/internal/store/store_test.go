package store

import (
	"testing"
	"time"
)

func TestHashInputNormalizesText(t *testing.T) {
	md := HashInput("input.md", []byte("\xEF\xBB\xBF# Backend Dev\r\n\r\n- Go\r\n- SQL  \r\n"))
	pasted := HashInput("input.md", []byte("# backend dev\n- go\n- sql"))
	txt := HashInput("input.txt", []byte("# Backend   Dev - Go - SQL"))
	if md != pasted || md != txt {
		t.Error("same text with different case/whitespace/BOM should hash equal")
	}
	if md == HashInput("input.md", []byte("# Frontend Dev\n- Vue")) {
		t.Error("different text should hash differently")
	}
	if HashInput("input.pdf", []byte("%PDF-1.4 A")) == HashInput("input.pdf", []byte("%PDF-1.4  a")) {
		t.Error("PDFs are hashed byte by byte")
	}
}

func TestSameInput(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mk := func(id, status, text string, ago time.Duration) {
		m := &Meta{ID: id, Status: status, CreatedAt: time.Now().Add(-ago)}
		if err := s.Create(m, "input.md", []byte(text)); err != nil {
			t.Fatal(err)
		}
	}
	mk("20260101-000000-aaaaaa", StatusReady, "Go dev", 3*time.Hour)
	mk("20260101-000000-bbbbbb", StatusReady, "Vue dev", 2*time.Hour)
	mk("20260101-000000-cccccc", StatusError, "Go dev", time.Hour)
	mk("20260101-000000-dddddd", StatusReady, "GO   DEV", 30*time.Minute)
	mk("20260101-000000-eeeeee", StatusGenerating, "go dev", 0)

	got, err := s.SameInput(HashInput("input.md", []byte("go dev")), "20260101-000000-eeeeee")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "20260101-000000-aaaaaa" || got[1].ID != "20260101-000000-dddddd" {
		ids := []string{}
		for _, m := range got {
			ids = append(ids, m.ID)
		}
		t.Fatalf("expected ready quizzes aaaaaa and dddddd (oldest first), got %v", ids)
	}
}
