package dettest

import (
	"archive/zip"
	"bytes"
	"io"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/frankbardon/vellum/opc/zipdet"
)

// slideRows is what one slide of a split table carries.
type slideRows struct {
	// Slide is the slide's one-based number, as its part is named.
	Slide int

	// Rows is how many table rows the slide draws, banner included.
	Rows int

	// FirstLabel is the first label the slide draws, which is the banner's
	// leading heading — the thing that has to be there on every part.
	//
	// LastLabel is the last, which is the row the boundary falls after.
	FirstLabel, LastLabel string
}

// pinnedSplits is where each overflowing golden's rows land.
//
// Committed as values rather than left implicit in the golden's bytes. The
// determinism harness already proves the same specification produces the same
// package on every run and in every process, so the split cannot drift between
// runs — what it cannot do is make the boundary visible. A change to the row
// height arithmetic moves the boundary, the golden is rebaselined with -update,
// and the diff is a few thousand bytes of compressed XML in which nothing says
// "this table now breaks five rows earlier".
//
// Here it says so, in the one line of a diff a person will read.
var pinnedSplits = map[string][]slideRows{
	"pptx-table": {
		{Slide: 1, Rows: 20, FirstLabel: "Region", LastLabel: "Band 18"},
		{Slide: 2, Rows: 10, FirstLabel: "Region", LastLabel: "Band 26"},
	},
}

// TestDeterminismOverflowIsPinned checks each overflowing golden against the
// boundary it was reviewed at.
//
// The banner count is part of it. A split that carried the right rows and
// stopped repeating the header would satisfy every other check in this suite
// and produce a deck whose second slide is a grid of numbers with no column
// names — which is the failure the whole policy exists to prevent.
func TestDeterminismOverflowIsPinned(t *testing.T) {
	for name, want := range pinnedSplits {
		c, ok := caseNamed(name)
		if !ok {
			t.Errorf("pinnedSplits names %q, which is not a registered case", name)
			continue
		}

		t.Run(name, func(t *testing.T) {
			body, err := c.Bytes(zipdet.PinnedEpoch)
			if err != nil {
				t.Fatalf("emit: %v", err)
			}

			got := readSlideRows(t, body)
			if len(got) != len(want) {
				t.Fatalf("the table occupies %d slides, want %d:\n%s",
					len(got), len(want), renderSplits(got))
			}
			for i := range want {
				if got[i] != want[i] {
					t.Errorf("slide %d landed as %+v, want %+v", want[i].Slide, got[i], want[i])
				}
			}

			// Every slide repeats the banner, which is what makes the rows on
			// the later ones readable.
			for _, s := range got {
				if s.FirstLabel != want[0].FirstLabel {
					t.Errorf("slide %d opens with %q rather than the banner's %q; the header was not repeated",
						s.Slide, s.FirstLabel, want[0].FirstLabel)
				}
			}
		})
	}
}

// caseNamed finds a registered case by name.
func caseNamed(name string) (Case, bool) {
	for _, c := range Cases() {
		if c.Name == name {
			return c, true
		}
	}
	return Case{}, false
}

// readSlideRows counts each slide's table rows and reads its first and last
// row-header label.
//
// Read out of the written package rather than off the model, so what is pinned
// is what a reader will find. A split that is correct in the model and lost on
// the way to the bytes is the failure this catches.
func readSlideRows(t *testing.T, body []byte) []slideRows {
	t.Helper()

	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("reading the package: %v", err)
	}

	var out []slideRows
	for _, f := range zr.File {
		const prefix, suffix = "ppt/slides/slide", ".xml"
		if !strings.HasPrefix(f.Name, prefix) || !strings.HasSuffix(f.Name, suffix) {
			continue
		}
		number, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(f.Name, prefix), suffix))
		if err != nil {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			t.Fatalf("opening %s: %v", f.Name, err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("reading %s: %v", f.Name, err)
		}

		xml := string(data)
		rows := strings.Count(xml, "<a:tr ")
		if rows == 0 {
			continue
		}

		labels := stubLabels(xml)
		entry := slideRows{Slide: number, Rows: rows}
		if len(labels) > 0 {
			entry.FirstLabel, entry.LastLabel = labels[0], labels[len(labels)-1]
		}
		out = append(out, entry)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Slide < out[j].Slide })
	return out
}

// stubLabels reads the first cell's text from every row of a slide's table.
//
// A scanner rather than a parser. What is wanted is the label at each boundary,
// and the label is the first a:t inside each a:tr — which is exactly what a
// person checking a printed deck would look at.
func stubLabels(slide string) []string {
	var out []string
	for _, row := range strings.Split(slide, "<a:tr ")[1:] {
		start := strings.Index(row, "<a:t ")
		if start < 0 {
			continue
		}
		start = strings.Index(row[start:], ">")
		if start < 0 {
			continue
		}
		from := strings.Index(row, "<a:t ") + start + 1
		end := strings.Index(row[from:], "</a:t>")
		if end < 0 {
			continue
		}
		if label := row[from : from+end]; label != "" {
			out = append(out, label)
		}
	}
	return out
}

func renderSplits(got []slideRows) string {
	var b strings.Builder
	for _, s := range got {
		b.WriteString("    slide " + strconv.Itoa(s.Slide) +
			": " + strconv.Itoa(s.Rows) + " rows, " +
			s.FirstLabel + " .. " + s.LastLabel + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}
