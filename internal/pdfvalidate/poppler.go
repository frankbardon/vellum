// Package pdfvalidate reads Vellum's PDF output with readers Vellum did not
// write.
//
// Two oracles, for two different questions. Poppler answers "does a real reader
// see what we put in the file" — it is small, fast and widely installed, so its
// checks run in the ordinary suite. veraPDF answers "is the PDF/A conformance
// claim true", which is the claim the file makes about itself in its own
// metadata; that one is behind the `verapdf` build tag because the validator is
// a JVM application nobody has installed by accident.
//
// Neither is compared byte for byte. See [exttool] for why borrowing a reader
// is not the same as depending on a renderer.
package pdfvalidate

import (
	"context"
	"strings"

	"github.com/frankbardon/vellum/internal/exttool"
)

// popplerText describes pdftotext.
var popplerText = exttool.Spec{
	Name:     "pdftotext",
	Commands: []string{"pdftotext"},
	Install:  "brew install poppler, or apt install poppler-utils",
}

// popplerInfo describes pdfinfo.
var popplerInfo = exttool.Spec{
	Name:     "pdfinfo",
	Commands: []string{"pdfinfo"},
	Install:  "brew install poppler, or apt install poppler-utils",
}

// Poppler is a located poppler installation.
type Poppler struct {
	text exttool.Tool
	info exttool.Tool
}

// FindPoppler locates pdftotext and pdfinfo.
//
// Both are required rather than one being optional, because the two checks they
// support answer different halves of the same question — that the document
// parses, and that its content survived — and a suite reporting only half of
// that is reporting something other than what it claims.
func FindPoppler() (Poppler, error) {
	text, err := exttool.Find(popplerText)
	if err != nil {
		return Poppler{}, err
	}
	info, err := exttool.Find(popplerInfo)
	if err != nil {
		return Poppler{}, err
	}
	return Poppler{text: text, info: info}, nil
}

// Text extracts the document's text.
//
// This is the check that catches content reaching the file but not reaching a
// reader. It exercises the whole text path at once: a composite font's encoding,
// the ToUnicode mapping, and the glyph identifiers in the content stream all
// have to agree for a single word to come back correct.
func (p Poppler) Text(ctx context.Context, path string) (string, error) {
	res, err := p.text.Run(ctx, nil, "-layout", path, "-")
	if err != nil {
		return "", err
	}
	if res.ExitCode != 0 {
		return "", &ReadError{Tool: "pdftotext", ExitCode: res.ExitCode, Output: res.Combined()}
	}
	return string(res.Stdout), nil
}

// Info reads the document's catalogue and information dictionary.
func (p Poppler) Info(ctx context.Context, path string) (Info, error) {
	res, err := p.info.Run(ctx, nil, path)
	if err != nil {
		return Info{}, err
	}
	if res.ExitCode != 0 {
		return Info{}, &ReadError{Tool: "pdfinfo", ExitCode: res.ExitCode, Output: res.Combined()}
	}

	var out Info
	for _, line := range strings.Split(string(res.Stdout), "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		out.fields = append(out.fields, field{
			key:   strings.TrimSpace(key),
			value: strings.TrimSpace(value),
		})
	}
	return out, nil
}

// Info is what pdfinfo reported, in the order it reported it.
//
// A slice rather than a map, so a failure message listing the fields reads the
// same on every run.
type Info struct {
	fields []field
}

type field struct{ key, value string }

// Get returns the named field, or the empty string.
func (i Info) Get(key string) string {
	for _, f := range i.fields {
		if f.key == key {
			return f.value
		}
	}
	return ""
}

// String renders the fields for a failure message.
func (i Info) String() string {
	var b strings.Builder
	for _, f := range i.fields {
		b.WriteString("    " + f.key + ": " + f.value + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// ReadError reports that a reader refused the document.
type ReadError struct {
	Tool     string
	ExitCode int
	Output   string
}

func (e *ReadError) Error() string {
	msg := e.Tool + " could not read the document (exit " + itoa(e.ExitCode) + ")"
	if e.Output != "" {
		msg += "\n    " + strings.ReplaceAll(strings.TrimRight(e.Output, "\n"), "\n", "\n    ")
	}
	return msg
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
