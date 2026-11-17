// Package pdftext extracts plain text from PDFs for engines that cannot read
// PDFs natively (Codex). Claude Code reads the PDF itself.
package pdftext

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/ledongthuc/pdf"
)

// ErrNoText means the PDF has no extractable text (e.g. a scanned image).
var ErrNoText = errors.New("el PDF no tiene texto extraíble (¿es un escaneo o una imagen?)")

// Timeout guards against PDFs that make the parser loop.
var Timeout = 20 * time.Second

type result struct {
	text string
	err  error
}

// Extract returns the plain text of the PDF.
func Extract(path string) (string, error) {
	ch := make(chan result, 1)
	go func() { ch <- extract(path) }()
	select {
	case r := <-ch:
		return r.text, r.err
	case <-time.After(Timeout):
		return "", fmt.Errorf("leer el PDF tardó más de %s", Timeout)
	}
}

func extract(path string) (res result) {
	defer func() {
		// The PDF parser panics on some malformed files.
		if r := recover(); r != nil {
			res = result{err: fmt.Errorf("no se pudo leer el PDF: %v", r)}
		}
	}()
	f, r, err := pdf.Open(path)
	if err != nil {
		return result{err: fmt.Errorf("no se pudo abrir el PDF: %w", err)}
	}
	defer f.Close()
	rd, err := r.GetPlainText()
	if err != nil {
		return result{err: fmt.Errorf("no se pudo leer el PDF: %w", err)}
	}
	data, err := io.ReadAll(rd)
	if err != nil {
		return result{err: fmt.Errorf("no se pudo leer el PDF: %w", err)}
	}
	text := strings.TrimSpace(string(data))
	if len(strings.Fields(text)) < 10 {
		return result{err: ErrNoText}
	}
	return result{text: text}
}
