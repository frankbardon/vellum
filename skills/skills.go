// Package skills embeds Vellum's skill pack: flat markdown, one file per
// concept, loaded via MCP at runtime by an agent composing against this
// library. See CLAUDE.md's "Skill Pack" section for the governing rule this
// package's tests enforce.
//
// # Shape
//
// Files are flat under this directory — no subdirectories — and their
// category comes from a filename prefix, not from where they live:
//
//   - block-<kind>.md   — one per [spec.BlockKind] (kebab of the wire string,
//     e.g. "page_break" -> block-page-break.md).
//   - format-<name>.md  — one per [artifact.Format].
//   - tool-<name>.md    — one per registered MCP tool, "vellum_" stripped.
//   - theme-<topic>.md  — theme guidance (font/colour/box roles, layouts).
//   - fill-<topic>.md   — fill-mode guidance (binding, repeat, anchors).
//   - anything else     — an unprefixed design guide.
//
// # Frontmatter
//
// Every file opens with a YAML frontmatter block delimited by "---" lines,
// decoded into [Frontmatter]:
//
//   - name          — the file's own stem (filename without ".md"), the
//     unique key a caller looks a document up by. Always equal to
//     Category + "-" + Kind, except a block file's Kind may carry
//     underscores (the true spec.BlockKind wire string) where its filename
//     carries hyphens (the kebab convention every filename follows).
//   - description   — one line, for a human or a model deciding whether to
//     read further.
//   - kind          — the specific identifier within the family: a block
//     kind's own wire string, a format's own wire string, a tool's name with
//     "vellum_" stripped, a theme topic, or a fill topic.
//   - category      — the family: "block", "format", "tool", "theme",
//     "fill", or "guide" for an unprefixed file.
//   - type          — always the literal "skill" today. Carried so a future
//     multi-pack tool (this package, examples/, docs/) can tell packs apart
//     by a field rather than by which embed.FS answered.
//   - applies_to    — the [artifact.Format] wire strings this content is
//     relevant to, or ["all"] when it is format-independent.
//   - examples_tags — free-text tags cross-referencing the vellum_examples_search
//     tool's own tag vocabulary (examples/, built in E13-S2).
//
// # Required headings and token budgets
//
// [RequiredHeadings] maps a category to the literal "## "-level headings
// every file in it must contain; [TokenBudget] maps a category to a ceiling.
// Both are enforced by skills_test.go, not by this file — this file only
// supplies the data both read.
package skills

import (
	"embed"
	"fmt"
	"sort"
	"strings"

	"sigs.k8s.io/yaml"
)

// FormatVersion is unversioned today: the skill pack has no wire format a
// consumer decodes against — Frontmatter is read by this package's own
// loader and by nothing else — so there is nothing yet for a version field to
// pin. Named here anyway as the place a future version would go, rather than
// invented as a field on [Frontmatter] no gate reads.
const FormatVersion = "unversioned"

//go:embed *.md
var files embed.FS

// Frontmatter is the YAML block every skill file opens with. See this
// package's doc comment for what each field means.
type Frontmatter struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Kind         string   `json:"kind"`
	Category     string   `json:"category"`
	Type         string   `json:"type"`
	AppliesTo    []string `json:"applies_to,omitempty"`
	ExamplesTags []string `json:"examples_tags,omitempty"`
}

// Doc is one parsed skill file.
type Doc struct {
	// Filename is the embedded file's own name, e.g. "block-heading.md".
	Filename string

	// Frontmatter is the parsed YAML header.
	Frontmatter Frontmatter

	// Body is everything after the closing "---", the file's own content —
	// what a heading search and a token-budget count both read.
	Body string

	// Raw is the whole file, frontmatter and body, exactly as embedded.
	Raw string
}

// Stem returns the filename without its ".md" suffix — the key [Get] and
// the "vellum_skills" tool's own Name input (mcp.SkillsIn.Name) both use.
func (d Doc) Stem() string { return strings.TrimSuffix(d.Filename, ".md") }

// Category returns the file's family, derived from its filename prefix
// rather than trusted from Frontmatter — the two are cross-checked by
// skills_test.go, but a caller of this package gets the derivation that
// cannot disagree with which file it read.
func (d Doc) Category() string { return categoryOf(d.Filename) }

// categoryPrefixes is the ordered set of recognised filename prefixes.
// Ordered so a caller reading it (or a future maintainer extending it) sees
// the same family list CLAUDE.md's "Skill Pack" section names, in that
// order.
var categoryPrefixes = []string{"block", "format", "tool", "theme", "fill"}

// categoryOf derives a family from a filename, defaulting to "guide" for an
// unprefixed design guide — the one category with no required prefix.
func categoryOf(filename string) string {
	stem := strings.TrimSuffix(filename, ".md")
	for _, p := range categoryPrefixes {
		if stem == p || strings.HasPrefix(stem, p+"-") {
			return p
		}
	}
	return "guide"
}

const (
	delimOpen  = "---\n"
	delimClose = "\n---\n"
)

// parse splits raw into Frontmatter and Body. The frontmatter block is the
// text between the file's opening "---" line and the next line that is
// exactly "---" — plain string splitting rather than a line-oriented state
// machine, because the shape is fixed and simple: every file in this pack
// is hand-authored, never generated, so tolerating a variant delimiter would
// only hide a typo.
func parse(filename string, raw []byte) (Doc, error) {
	content := string(raw)
	if !strings.HasPrefix(content, delimOpen) {
		return Doc{}, fmt.Errorf("skills: %s: does not open with a %q frontmatter delimiter", filename, "---")
	}
	rest := content[len(delimOpen):]
	idx := strings.Index(rest, delimClose)
	if idx < 0 {
		return Doc{}, fmt.Errorf("skills: %s: frontmatter is never closed with a %q line", filename, "---")
	}
	fmBlock := rest[:idx]
	body := rest[idx+len(delimClose):]

	fm, err := decodeFrontmatter(fmBlock)
	if err != nil {
		return Doc{}, fmt.Errorf("skills: %s: invalid frontmatter: %w", filename, err)
	}

	return Doc{Filename: filename, Frontmatter: fm, Body: body, Raw: content}, nil
}

// decodeFrontmatter parses a YAML frontmatter block through
// sigs.k8s.io/yaml — already a Vellum dependency for the same reason CLAUDE.md
// gives it: it routes through JSON, so this loader's strict [Frontmatter]
// struct rejects an unknown field the same way [spec.Spec]'s own decode does,
// rather than silently ignoring an author's typo.
func decodeFrontmatter(block string) (Frontmatter, error) {
	var fm Frontmatter
	if err := yaml.UnmarshalStrict([]byte(block), &fm); err != nil {
		return Frontmatter{}, err
	}
	return fm, nil
}

// All returns every embedded skill document, parsed, sorted by filename —
// deterministic regardless of the filesystem's own directory order, per
// this codebase's "no unordered iteration on a path that must not change
// between runs" convention.
func All() ([]Doc, error) {
	entries, err := files.ReadDir(".")
	if err != nil {
		return nil, fmt.Errorf("skills: reading embedded directory: %w", err)
	}

	out := make([]Doc, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		raw, err := files.ReadFile(e.Name())
		if err != nil {
			return nil, fmt.Errorf("skills: reading %s: %w", e.Name(), err)
		}
		doc, err := parse(e.Name(), raw)
		if err != nil {
			return nil, err
		}
		out = append(out, doc)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Filename < out[j].Filename })
	return out, nil
}

// Get returns the document named name (a [Doc.Stem] value, e.g.
// "block-heading"), and whether it was found. See mcp.SkillsIn.Name and
// mcp/handlers.go's handleSkills, the "vellum_skills" tool handler that
// calls this.
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

// RequiredHeadings maps a category to the literal "## "-level Markdown
// headings every file in that category must contain, in no particular
// order. A guide (the unprefixed category) has no required heading: it is
// the one family CLAUDE.md does not name a shape for.
var RequiredHeadings = map[string][]string{
	"block":  {"## What it is", "## JSON shape", "## Gotchas", "## See"},
	"format": {"## What it emits", "## Capability notes", "## Gotchas", "## See"},
	"tool":   {"## What it does", "## Input", "## Output", "## See"},
	"theme":  {"## Font roles", "## Color roles", "## Box roles", "## Gotchas", "## See"},
	"fill":   {"## Semantics", "## Example", "## Gotchas", "## See"},
}

// TokenBudget maps a category to its word-count ceiling — a stand-in for a
// real tokenizer this package deliberately does not depend on. Word count
// under-counts English prose tokens (a common rule of thumb is roughly 1.3
// tokens per word, more for identifier-heavy text like the JSON fences these
// files carry), so a ceiling set as a word count is stricter than the same
// number would be under a real tokenizer — it biases toward terse files
// rather than toward a budget that merely looks safe. Budgets differ by
// family because the families are not the same density: a tool file states
// an input and an output shape; a theme file has to name every font, colour
// and box role by name and stays the largest as a result.
var TokenBudget = map[string]int{
	"block":  400,
	"format": 550,
	"tool":   300,
	"theme":  750,
	"fill":   550,
	"guide":  400,
}

// WordCount is the token-count proxy [TokenBudget] is measured against:
// whitespace-delimited fields over the whole file, frontmatter included,
// because the frontmatter is also what a caller downloads.
func WordCount(s string) int { return len(strings.Fields(s)) }

// Headings returns every "## "-level Markdown heading in body, in the order
// they appear, trimmed of trailing whitespace.
func Headings(body string) []string {
	var out []string
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimRight(line, " \t\r")
		if strings.HasPrefix(trimmed, "## ") {
			out = append(out, trimmed)
		}
	}
	return out
}
