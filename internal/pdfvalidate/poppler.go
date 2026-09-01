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

// popplerImages describes pdfimages.
var popplerImages = exttool.Spec{
	Name:     "pdfimages",
	Commands: []string{"pdfimages"},
	Install:  "brew install poppler, or apt install poppler-utils",
}

// Poppler is a located poppler installation.
type Poppler struct {
	text   exttool.Tool
	info   exttool.Tool
	images exttool.Tool
}

// FindPoppler locates pdftotext, pdfinfo and pdfimages.
//
// All three are required rather than any being optional, because the checks
// they support answer different parts of one question — that the document
// parses, that its text survived, and that its pictures did — and a suite
// reporting some of that is reporting something other than what it claims.
func FindPoppler() (Poppler, error) {
	text, err := exttool.Find(popplerText)
	if err != nil {
		return Poppler{}, err
	}
	info, err := exttool.Find(popplerInfo)
	if err != nil {
		return Poppler{}, err
	}
	images, err := exttool.Find(popplerImages)
	if err != nil {
		return Poppler{}, err
	}
	return Poppler{text: text, info: info, images: images}, nil
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

// PageText extracts one page's text.
//
// The whole document's text answers whether a reader found the content; one
// page's answers where it found it, and that difference is the entire content
// of an overflow split. A table's rows are all present whichever page they
// landed on, so a document-wide check cannot tell a working split from one that
// put every row on the first page and ran them off the bottom of it.
func (p Poppler) PageText(ctx context.Context, path string, page int) (string, error) {
	res, err := p.text.Run(ctx, nil, "-layout", "-f", itoa(page), "-l", itoa(page), path, "-")
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

// Images lists the image XObjects a reader finds, in page order.
//
// Text extraction says nothing about pictures, and a picture is the one part of
// a document where "it opened" and "it is correct" come apart most quietly: an
// image whose colour space, bit depth or soft mask is wrong still draws. This
// asks a reader that has decoded them what it found.
func (p Poppler) Images(ctx context.Context, path string) ([]ImageInfo, error) {
	res, err := p.images.Run(ctx, nil, "-list", path)
	if err != nil {
		return nil, err
	}
	if res.ExitCode != 0 {
		return nil, &ReadError{Tool: "pdfimages", ExitCode: res.ExitCode, Output: res.Combined()}
	}

	var out []ImageInfo
	for _, line := range strings.Split(string(res.Stdout), "\n") {
		f := strings.Fields(line)
		// page num type width height color comp bpc enc interp object ID ...
		if len(f) < 9 || f[0] == "page" {
			continue
		}
		out = append(out, ImageInfo{
			Kind:   f[2],
			Width:  f[3],
			Height: f[4],
			Color:  f[5],
			BPC:    f[7],
			Enc:    f[8],
			Raw:    strings.TrimSpace(line),
		})
	}
	return out, nil
}

// ImageInfo is one row of what pdfimages reported.
//
// The numbers stay strings. They are compared for equality against what the
// fixture put in, and parsing them would add a failure mode — a malformed
// column becoming a zero — between the reader's answer and the assertion.
type ImageInfo struct {
	// Kind is "image" for a painted image and "smask" for a soft mask, which is
	// how a reader reports that the alpha channel arrived as its own image.
	Kind string

	Width, Height string
	Color         string
	BPC           string
	Enc           string

	// Raw is the whole line, for a failure message.
	Raw string
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
