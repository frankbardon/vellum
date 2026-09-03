package skills_test

import (
	"strings"
	"testing"

	"github.com/frankbardon/vellum/artifact"
	"github.com/frankbardon/vellum/capability"
	"github.com/frankbardon/vellum/mcp/toolmeta"
	"github.com/frankbardon/vellum/skills"
	"github.com/frankbardon/vellum/spec"
	"github.com/frankbardon/vellum/template/anchor"
	"github.com/frankbardon/vellum/template/bind"
	"github.com/frankbardon/vellum/theme"
)

// allDocs is a small test helper, not a gate itself: every gate below reads
// the same parsed set, so a file that fails to parse fails every gate at
// once rather than being silently skipped by one and caught by another.
func allDocs(t *testing.T) []skills.Doc {
	t.Helper()
	docs, err := skills.All()
	if err != nil {
		t.Fatalf("skills.All(): %v", err)
	}
	if len(docs) == 0 {
		t.Fatal("skills.All() returned no documents")
	}
	return docs
}

// rawCorpus concatenates every file's raw content — frontmatter and body —
// into one string, for a substring search that does not care which file
// happens to carry a given mention.
func rawCorpus(docs []skills.Doc) string {
	var b strings.Builder
	for _, d := range docs {
		b.WriteString(d.Raw)
		b.WriteString("\n")
	}
	return b.String()
}

func docsByCategory(docs []skills.Doc, category string) []skills.Doc {
	var out []skills.Doc
	for _, d := range docs {
		if d.Category() == category {
			out = append(out, d)
		}
	}
	return out
}

// blockKebab mirrors this package's own filename convention: a block kind's
// wire string (which may carry underscores, e.g. "page_break") becomes a
// filename with hyphens.
func blockKebab(k spec.BlockKind) string {
	return "block-" + strings.ReplaceAll(string(k), "_", "-")
}

// TestSkillsCoverAllBlockKinds asserts every spec.AllBlockKinds() value has
// a matching block-<kind>.md file.
func TestSkillsCoverAllBlockKinds(t *testing.T) {
	docs := allDocs(t)
	have := make(map[string]bool, len(docs))
	for _, d := range docs {
		have[d.Stem()] = true
	}
	for _, k := range spec.AllBlockKinds() {
		want := blockKebab(k)
		if !have[want] {
			t.Errorf("block kind %q has no %s.md", k, want)
		}
	}
}

// TestSkillsCoverAllFormats asserts every artifact.AllFormats() value has a
// matching format-<name>.md file.
func TestSkillsCoverAllFormats(t *testing.T) {
	docs := allDocs(t)
	have := make(map[string]bool, len(docs))
	for _, d := range docs {
		have[d.Stem()] = true
	}
	for _, f := range artifact.AllFormats() {
		want := "format-" + string(f)
		if !have[want] {
			t.Errorf("format %q has no %s.md", f, want)
		}
	}
}

// TestSkillsCoverAllFeatures asserts every capability.AllFeatures() value's
// wire string literal appears somewhere in the pack's body text — most
// naturally the block-*/format-*/fill-* file for the thing it constrains.
func TestSkillsCoverAllFeatures(t *testing.T) {
	docs := allDocs(t)
	corpus := rawCorpus(docs)
	for _, f := range capability.AllFeatures() {
		if !strings.Contains(corpus, string(f)) {
			t.Errorf("feature %q is not mentioned by name anywhere in the skill pack", f)
		}
	}
}

// TestSkillsCoverAllMCPTools asserts every mcp/toolmeta.AllTools() entry has
// a matching tool-<name>.md file, "vellum_" stripped.
func TestSkillsCoverAllMCPTools(t *testing.T) {
	docs := allDocs(t)
	have := make(map[string]bool, len(docs))
	for _, d := range docs {
		have[d.Stem()] = true
	}
	for _, tl := range toolmeta.AllTools() {
		name, ok := strings.CutPrefix(tl.Name, "vellum_")
		if !ok {
			t.Fatalf("tool name %q does not carry the vellum_ prefix the Update Demand table requires", tl.Name)
		}
		want := "tool-" + name
		if !have[want] {
			t.Errorf("mcp tool %q has no %s.md", tl.Name, want)
		}
	}
}

// TestSkillsCoverAllBindModes asserts every bind.StatementKind and every
// bind.RepeatTarget value is mentioned by name somewhere under a fill-*
// file.
func TestSkillsCoverAllBindModes(t *testing.T) {
	docs := allDocs(t)
	fillDocs := docsByCategory(docs, "fill")
	if len(fillDocs) == 0 {
		t.Fatal("no fill-*.md files found")
	}
	corpus := rawCorpus(fillDocs)

	for _, k := range bind.AllStatementKinds() {
		if !strings.Contains(corpus, string(k)) {
			t.Errorf("bind.StatementKind %q is not mentioned by name under any fill-*.md file", k)
		}
	}
	for _, tg := range bind.AllRepeatTargets() {
		if !strings.Contains(corpus, string(tg)) {
			t.Errorf("bind.RepeatTarget %q is not mentioned by name under any fill-*.md file", tg)
		}
	}
	// "An anchor kind, binding mode, or repeat semantic" is one Update
	// Demand row (CLAUDE.md); anchor.Kind rides along with the same gate
	// rather than a separate one, since fill-anchors.md is itself a
	// fill-*.md file and the row names all three together.
	kinds := []anchor.Kind{
		anchor.KindNative,
		anchor.KindMarker,
		anchor.KindDefinedName,
		anchor.KindTableColumn,
		anchor.KindShape,
	}
	for _, k := range kinds {
		if !strings.Contains(corpus, string(k)) {
			t.Errorf("anchor.Kind %q is not mentioned by name under any fill-*.md file", k)
		}
	}
}

// TestSkillsCoverThemeSlots asserts every theme.FontRole, theme.ColorRole
// and theme.BoxRole value is mentioned by name somewhere under a theme-*
// file.
func TestSkillsCoverThemeSlots(t *testing.T) {
	docs := allDocs(t)
	themeDocs := docsByCategory(docs, "theme")
	if len(themeDocs) == 0 {
		t.Fatal("no theme-*.md files found")
	}
	corpus := rawCorpus(themeDocs)

	for _, r := range theme.AllFontRoles() {
		if !strings.Contains(corpus, string(r)) {
			t.Errorf("theme.FontRole %q is not mentioned by name under any theme-*.md file", r)
		}
	}
	for _, r := range theme.AllColorRoles() {
		if !strings.Contains(corpus, string(r)) {
			t.Errorf("theme.ColorRole %q is not mentioned by name under any theme-*.md file", r)
		}
	}
	for _, r := range theme.AllBoxRoles() {
		if !strings.Contains(corpus, string(r)) {
			t.Errorf("theme.BoxRole %q is not mentioned by name under any theme-*.md file", r)
		}
	}
}

// TestSkillsHaveRequiredSections asserts every file carries the literal
// "## " headings its category requires. skills.RequiredHeadings is the
// data; this is the only thing that reads it as a gate.
func TestSkillsHaveRequiredSections(t *testing.T) {
	docs := allDocs(t)
	for _, d := range docs {
		required, ok := skills.RequiredHeadings[d.Category()]
		if !ok {
			// A guide (or any future category with no declared requirement)
			// carries no required heading.
			continue
		}
		present := make(map[string]bool)
		for _, h := range skills.Headings(d.Body) {
			present[h] = true
		}
		for _, want := range required {
			if !present[want] {
				t.Errorf("%s: missing required heading %q", d.Filename, want)
			}
		}
	}
}

// TestSkillTokenBudget asserts every file stays under its category's
// word-count ceiling — the documented token-count proxy. See
// skills.TokenBudget's own doc comment for why word count and not a real
// tokenizer.
func TestSkillTokenBudget(t *testing.T) {
	docs := allDocs(t)
	for _, d := range docs {
		budget, ok := skills.TokenBudget[d.Category()]
		if !ok {
			t.Errorf("%s: category %q has no entry in skills.TokenBudget", d.Filename, d.Category())
			continue
		}
		n := skills.WordCount(d.Raw)
		if n > budget {
			t.Errorf("%s: %d words exceeds the %q budget of %d", d.Filename, n, d.Category(), budget)
		}
	}
}

// TestFrontmatter_IsWellFormed is not a named gate CLAUDE.md lists, but
// every gate above trusts skills.All() to have parsed cleanly and trusts
// Frontmatter.Category to agree with the filename prefix it is derived
// from independently of what a file's own frontmatter claims — this checks
// that the frontmatter a file carries does not silently disagree with the
// derivation the gates above actually use.
func TestFrontmatter_IsWellFormed(t *testing.T) {
	docs := allDocs(t)
	seen := make(map[string]string, len(docs))
	for _, d := range docs {
		fm := d.Frontmatter
		if fm.Name != d.Stem() {
			t.Errorf("%s: frontmatter name %q does not match the filename stem %q", d.Filename, fm.Name, d.Stem())
		}
		if fm.Category != d.Category() {
			t.Errorf("%s: frontmatter category %q does not match the filename-derived category %q", d.Filename, fm.Category, d.Category())
		}
		if fm.Description == "" {
			t.Errorf("%s: frontmatter carries no description", d.Filename)
		}
		if fm.Kind == "" {
			t.Errorf("%s: frontmatter carries no kind", d.Filename)
		}
		if fm.Type != "skill" {
			t.Errorf("%s: frontmatter type %q, want %q", d.Filename, fm.Type, "skill")
		}
		if len(fm.AppliesTo) == 0 {
			t.Errorf("%s: frontmatter carries no applies_to", d.Filename)
		}
		if len(fm.ExamplesTags) == 0 {
			t.Errorf("%s: frontmatter carries no examples_tags", d.Filename)
		}
		if other, dup := seen[d.Stem()]; dup {
			t.Errorf("duplicate skill name %q: %s and %s", d.Stem(), other, d.Filename)
		}
		seen[d.Stem()] = d.Filename
	}
}

// TestGet_FindsEveryDocumentByItsOwnStem is a sanity check on the lookup
// path a future MCP handler is expected to call.
func TestGet_FindsEveryDocumentByItsOwnStem(t *testing.T) {
	docs := allDocs(t)
	for _, d := range docs {
		got, ok, err := skills.Get(d.Stem())
		if err != nil {
			t.Fatalf("skills.Get(%q): %v", d.Stem(), err)
		}
		if !ok {
			t.Errorf("skills.Get(%q) reported not found", d.Stem())
			continue
		}
		if got.Filename != d.Filename {
			t.Errorf("skills.Get(%q).Filename = %q, want %q", d.Stem(), got.Filename, d.Filename)
		}
	}
	if _, ok, err := skills.Get("does-not-exist"); err != nil || ok {
		t.Errorf("skills.Get(\"does-not-exist\") = (_, %v, %v), want (_, false, nil)", ok, err)
	}
}
