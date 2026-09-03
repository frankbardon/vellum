// Package examples embeds Vellum's runnable example pack: a JSON
// specification per [spec.BlockKind], a fuller JSON specification per
// [artifact.Format], and a JSON [template/bind.Binding] per
// [bind.StatementKind] — every one of them proven, by examples_test.go, to
// actually compose or fill through the real facade rather than merely to
// parse. See CLAUDE.md's "Skill Pack" section and this story's sibling
// package skills for the governing convention this package's tests mirror.
//
// # Shape
//
// Files are flat under this directory — no subdirectories — and their
// category comes from a filename prefix, not from where they live, the same
// convention [skills] establishes:
//
//   - block-<kind>.json  — one per [spec.BlockKind] (kebab of the wire
//     string, e.g. "page_break" -> block-page-break.json). A minimal
//     [spec.Spec] whose content demonstrates that one block kind.
//   - format-<name>.json — one per [artifact.Format]. A fuller [spec.Spec] —
//     several block kinds together, a realistic small report — that
//     composes cleanly to that specific format without demonstrating a
//     capability-matrix degradation as though it were the golden path.
//   - fill-<kind>.json    — one per [bind.StatementKind] ("bind", "repeat",
//     "if", "with"). A [bind.Binding] document, focused on that one
//     statement kind, that reconciles and fills against the real docx
//     template [internal/dettest.FillDOCXFixture] already exercises in the
//     determinism harness — see examples_test.go for the data it binds
//     against and why the other statement kinds this template's anchors
//     would otherwise demand are marked OptionalAnchors rather than bound.
//
// Every file is decoded and exercised — [spec.Decode] then
// [vellum.Vellum.Compose], or [bind.Decode] then [vellum.Vellum.Fill] — by
// examples_test.go's coverage gate, never merely parsed: a committed example
// that silently bit-rotted as the spec or binding schema evolved would be
// worse than no example at all, discovered only once a consumer copied it.
package examples

import (
	"embed"
	"fmt"
	"sort"
	"strings"
)

//go:embed *.json
var files embed.FS

// Doc is one parsed example file.
type Doc struct {
	// Filename is the embedded file's own name, e.g. "block-heading.json".
	Filename string

	// Raw is the file's own bytes, exactly as embedded — a JSON
	// [spec.Spec] for a block-*/format-* file, a JSON
	// [template/bind.Binding] for a fill-*.json file.
	Raw []byte
}

// Stem returns the filename without its ".json" suffix — the key a caller
// (and this package's own coverage gate) looks a document up by.
func (d Doc) Stem() string { return strings.TrimSuffix(d.Filename, ".json") }

// categoryPrefixes is the ordered set of recognised filename prefixes,
// mirroring [skills]'s own list restricted to the three families this
// package's story covers.
var categoryPrefixes = []string{"block", "format", "fill"}

// Category derives an example's family from its filename prefix, the same
// derive-don't-trust convention [skills.Doc.Category] follows: nothing here
// reads a value a file claims about itself, because there is no frontmatter
// to claim one.
func (d Doc) Category() string {
	stem := d.Stem()
	for _, p := range categoryPrefixes {
		if stem == p || strings.HasPrefix(stem, p+"-") {
			return p
		}
	}
	return "guide"
}

// All returns every embedded example document, sorted by filename —
// deterministic regardless of the filesystem's own directory order, per
// this codebase's "no unordered iteration on a path that must not change
// between runs" convention.
func All() ([]Doc, error) {
	entries, err := files.ReadDir(".")
	if err != nil {
		return nil, fmt.Errorf("examples: reading embedded directory: %w", err)
	}

	out := make([]Doc, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, err := files.ReadFile(e.Name())
		if err != nil {
			return nil, fmt.Errorf("examples: reading %s: %w", e.Name(), err)
		}
		out = append(out, Doc{Filename: e.Name(), Raw: raw})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Filename < out[j].Filename })
	return out, nil
}

// Get returns the document named name (a [Doc.Stem] value, e.g.
// "block-heading"), and whether it was found.
func Get(name string) (Doc, bool, error) {
	all, err := All()
	if err != nil {
		return Doc{}, false, err
	}
	for _, d := range all {
		if d.Stem() == name {
			return d, true, nil
		}
	}
	return Doc{}, false, nil
}
