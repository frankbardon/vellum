package dettest

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"sort"
	"strings"

	"github.com/frankbardon/vellum/opc/zipdet"
)

// DescribeMismatch renders a human-readable account of how two artifacts
// differ, at the part level, with XML normalised.
//
// The normalisation is for reading only. Nothing here feeds an assertion —
// callers compare raw bytes and use this to explain a failure they have
// already detected. Normalising before comparing would quietly excuse exactly
// the differences the suite exists to catch.
func DescribeMismatch(want, got []byte) string {
	var b strings.Builder
	fmt.Fprintf(&b, "artifact differs: %d bytes want, %d bytes got\n", len(want), len(got))

	wantParts, wErr := readParts(want)
	gotParts, gErr := readParts(got)
	if wErr != nil || gErr != nil {
		// Not both readable as archives — a raw summary is the best available.
		if wErr != nil {
			fmt.Fprintf(&b, "  expected side is not a readable archive: %v\n", wErr)
		}
		if gErr != nil {
			fmt.Fprintf(&b, "  actual side is not a readable archive: %v\n", gErr)
		}
		return b.String()
	}

	names := unionNames(wantParts, gotParts)
	same := 0
	for _, name := range names {
		w, inWant := wantParts[name]
		g, inGot := gotParts[name]

		switch {
		case !inWant:
			fmt.Fprintf(&b, "  + %s (%d bytes, only in actual)\n", name, len(g))
		case !inGot:
			fmt.Fprintf(&b, "  - %s (%d bytes, only in expected)\n", name, len(w))
		case bytes.Equal(w, g):
			same++
		default:
			fmt.Fprintf(&b, "  ~ %s (%d -> %d bytes)\n", name, len(w), len(g))
			for _, line := range describePart(w, g) {
				fmt.Fprintf(&b, "      %s\n", line)
			}
		}
	}
	fmt.Fprintf(&b, "  %d parts identical\n", same)
	return b.String()
}

func readParts(raw []byte) (map[string][]byte, error) {
	a, err := zipdet.Read(bytes.NewReader(raw), int64(len(raw)), zipdet.ReadOptions{})
	if err != nil {
		return nil, err
	}
	out := make(map[string][]byte, a.Len())
	for _, e := range a.Entries() {
		out[e.Name] = e.Data
	}
	return out, nil
}

func unionNames(a, b map[string][]byte) []string {
	seen := make(map[string]bool, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for n := range a {
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	for n := range b {
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

// describePart returns up to a handful of lines locating the first differences
// between two part payloads.
func describePart(want, got []byte) []string {
	wl := normaliseLines(want)
	gl := normaliseLines(got)

	const maxReported = 6
	var out []string
	for i := 0; i < len(wl) || i < len(gl); i++ {
		var w, g string
		if i < len(wl) {
			w = wl[i]
		}
		if i < len(gl) {
			g = gl[i]
		}
		if w == g {
			continue
		}
		out = append(out, fmt.Sprintf("line %d:", i+1), "  want: "+truncate(w), "  got:  "+truncate(g))
		if len(out) >= maxReported {
			out = append(out, "...")
			break
		}
	}
	if out == nil {
		// Byte-different but normalisation-identical: whitespace, attribute
		// order, or self-closing form. Worth saying plainly, because it is
		// exactly the class of change a re-serialisation introduces.
		out = append(out, "identical once normalised — the difference is whitespace, attribute order or self-closing form")
	}
	return out
}

func truncate(s string) string {
	const limit = 160
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "..."
}

// normaliseLines renders a payload as indented lines with attributes sorted.
// Non-XML payloads are described by size and a short hex prefix instead.
func normaliseLines(data []byte) []string {
	if !looksLikeXML(data) {
		return []string{fmt.Sprintf("<binary, %d bytes, starts %x>", len(data), data[:min(16, len(data))])}
	}

	var b strings.Builder
	dec := xml.NewDecoder(bytes.NewReader(data))
	depth := 0
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			b.WriteString(strings.Repeat("  ", depth))
			b.WriteString("<" + t.Name.Local)
			attrs := make([]xml.Attr, len(t.Attr))
			copy(attrs, t.Attr)
			sort.Slice(attrs, func(i, j int) bool {
				if attrs[i].Name.Space != attrs[j].Name.Space {
					return attrs[i].Name.Space < attrs[j].Name.Space
				}
				return attrs[i].Name.Local < attrs[j].Name.Local
			})
			for _, a := range attrs {
				b.WriteString(" " + a.Name.Local + "=" + quoteAttr(a.Value))
			}
			b.WriteString(">\n")
			depth++
		case xml.EndElement:
			depth--
			if depth < 0 {
				depth = 0
			}
			b.WriteString(strings.Repeat("  ", depth) + "</" + t.Name.Local + ">\n")
		case xml.CharData:
			s := strings.TrimSpace(string(t))
			if s != "" {
				b.WriteString(strings.Repeat("  ", depth) + s + "\n")
			}
		}
	}
	return strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
}

func quoteAttr(s string) string { return "\"" + s + "\"" }

// xmlBOM is the UTF-8 byte order mark, written as bytes rather than as a
// source literal because a literal BOM in Go source is a compile error.
var xmlBOM = []byte{0xEF, 0xBB, 0xBF}

func looksLikeXML(data []byte) bool {
	trimmed := bytes.TrimPrefix(data, xmlBOM)
	trimmed = bytes.TrimLeft(trimmed, " \t\r\n")
	return len(trimmed) > 0 && trimmed[0] == '<'
}
