package deck_test

import (
	"archive/zip"
	"bytes"
	"io"
	"sort"
	"strings"
	"testing"

	"github.com/frankbardon/vellum/deck"
)

const pt = 12700

// design is the design every test authors from.
//
// Concrete numbers rather than round ones, so a test asserting a derived
// measurement is asserting arithmetic rather than recognising a constant.
func design() deck.Design {
	return deck.Design{
		Name:           "Vellum Test",
		MarginTop:      457200,
		MarginRight:    457200,
		MarginBottom:   457200,
		MarginLeft:     457200,
		HeadingFamily:  "Helvetica Neue",
		BodyFamily:     "Helvetica",
		TitleSize:      40 * pt,
		BodySizes:      []int64{20 * pt, 18 * pt, 16 * pt},
		LineHeight:     1.2,
		ParagraphSpace: 6 * pt,
		TitleGap:       6 * pt,
		Colors: deck.ColorScheme{
			Dark1: "1A1A1A", Light1: "FFFFFF",
			Dark2: "0B3D91", Light2: "F2F2F2",
			Accent1: "0B3D91", Accent2: "C8102E", Accent3: "007A5E",
			Accent4: "F2A900", Accent5: "6B4E9B", Accent6: "5B6770",
			Hyperlink: "0B3D91", FollowedHyperlink: "6B4E9B",
		},
	}
}

// authored returns the deck Author produces for the test design.
func authored(t *testing.T) *deck.Deck {
	t.Helper()

	d, err := deck.Author(design())
	if err != nil {
		t.Fatalf("Author: %v", err)
	}
	return d
}

// sample is an authored deck carrying two slides, one of them with notes.
func sample(t *testing.T) *deck.Deck {
	t.Helper()

	d := authored(t)
	d.Title = "Sample"
	d.Slides = []deck.Slide{
		{
			LayoutID: deck.LayoutIDTitle,
			Shapes: []deck.Shape{
				{
					Placeholder: &deck.Placeholder{Type: deck.PlaceholderCenterTitle},
					Text:        body("Vellum"),
				},
				{
					Placeholder: &deck.Placeholder{Type: deck.PlaceholderSubTitle, Index: 1},
					Text:        body("spec in, document out"),
				},
			},
		},
		{
			LayoutID: deck.LayoutIDContent,
			Notes:    "Speak to the three models.\n\nDo not read the slide.",
			Shapes: []deck.Shape{
				{
					Placeholder: &deck.Placeholder{Type: deck.PlaceholderTitle},
					Text:        body("Three content models"),
				},
				{
					Placeholder: &deck.Placeholder{Type: deck.PlaceholderContent, Index: 1},
					Text: &deck.TextBody{Paragraphs: []deck.Paragraph{
						{Runs: []deck.Run{{Text: "spec is unresolved and hashable"}}},
						{Level: 1, Runs: []deck.Run{{Text: "theme by reference"}}},
						{Runs: []deck.Run{{Text: "fragment is resolved and format neutral"}}},
					}},
				},
			},
		},
	}
	return d
}

// body builds a single-paragraph text body.
func body(text string) *deck.TextBody {
	return &deck.TextBody{Paragraphs: []deck.Paragraph{{Runs: []deck.Run{{Text: text}}}}}
}

// write emits a deck and fails the test if it cannot.
func write(t *testing.T, d *deck.Deck) []byte {
	t.Helper()

	var buf bytes.Buffer
	if err := d.WriteTo(&buf, deck.WriteOptions{}); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	return buf.Bytes()
}

// pkg is a written package read back as a map of part name to bytes.
type pkg struct {
	names []string
	parts map[string][]byte
}

// unzip reads a written deck.
func unzip(t *testing.T, raw []byte) pkg {
	t.Helper()

	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("reading the package: %v", err)
	}

	out := pkg{parts: make(map[string][]byte, len(zr.File))}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("opening %s: %v", f.Name, err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("reading %s: %v", f.Name, err)
		}
		out.names = append(out.names, f.Name)
		out.parts[f.Name] = data
	}
	return out
}

// part returns a part's bytes as a string, failing when it is absent.
func (p pkg) part(t *testing.T, name string) string {
	t.Helper()

	data, ok := p.parts[name]
	if !ok {
		t.Fatalf("the package has no part %q.\nIt holds:\n  %s", name, strings.Join(p.sorted(), "\n  "))
	}
	return string(data)
}

func (p pkg) has(name string) bool {
	_, ok := p.parts[name]
	return ok
}

func (p pkg) sorted() []string {
	out := append([]string(nil), p.names...)
	sort.Strings(out)
	return out
}
