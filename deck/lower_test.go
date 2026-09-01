package deck_test

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/frankbardon/vellum/artifact"
	"github.com/frankbardon/vellum/deck"
	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/fragment"
	"github.com/frankbardon/vellum/resolve"
	"github.com/frankbardon/vellum/spec"
	"github.com/frankbardon/vellum/theme"
)

var onePixelPNG = mustDecode(`iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==`)

func mustDecode(s string) []byte {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

func pngURI() string {
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(onePixelPNG)
}

// lower resolves a specification for PPTX and lowers it, which is the path a
// caller composing from blocks actually takes.
func lower(t *testing.T, blocks ...spec.Block) *deck.Deck {
	t.Helper()

	d, err := deck.Lower(resolved(t, blocks...))
	if err != nil {
		t.Fatalf("Lower: %v", err)
	}
	return d
}

func resolved(t *testing.T, blocks ...spec.Block) *fragment.Doc {
	t.Helper()

	s := &spec.Spec{
		FormatVersion: spec.FormatVersion,
		Title:         "Report",
		Sections:      []spec.Section{{ID: "s1", Blocks: blocks}},
	}
	res, err := resolve.Resolve(context.Background(), s, resolve.Options{Format: artifact.FormatPPTX})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	return res.Doc
}

func heading(level int, content string) spec.Block {
	return spec.Block{Kind: spec.BlockHeading, Heading: &spec.Heading{Level: level, Content: content}}
}

func text(content string, marks ...string) spec.Block {
	return spec.Block{Kind: spec.BlockText, Marks: marks, Text: &spec.Text{Content: content}}
}

// TestLower_AHeadingStartsASlideAndBecomesItsTitle is the central mapping rule.
func TestLower_AHeadingStartsASlideAndBecomesItsTitle(t *testing.T) {
	d := lower(t,
		heading(1, "First"),
		text("One."),
		heading(1, "Second"),
		text("Two."),
	)

	if got := len(d.Slides); got != 2 {
		t.Fatalf("want two slides, got %d", got)
	}
	for i, want := range []string{"First", "Second"} {
		if got := titleOf(t, d.Slides[i]); got != want {
			t.Errorf("slide %d is titled %q, want %q", i+1, got, want)
		}
		if d.Slides[i].LayoutID != deck.LayoutIDContent {
			t.Errorf("slide %d uses layout %q, want %q", i+1, d.Slides[i].LayoutID, deck.LayoutIDContent)
		}
	}
	if got := bodyText(t, d.Slides[0]); got != "One." {
		t.Errorf("slide 1 body is %q", got)
	}
}

// TestLower_AHeadingWithNothingUnderItIsATitleOnlySlide checks the layout
// follows what is on the slide rather than what level the heading was.
func TestLower_AHeadingWithNothingUnderItIsATitleOnlySlide(t *testing.T) {
	d := lower(t, heading(1, "Part One"), heading(2, "Detail"), text("Body."))

	if got := len(d.Slides); got != 2 {
		t.Fatalf("want two slides, got %d", got)
	}
	if d.Slides[0].LayoutID != deck.LayoutIDTitleOnly {
		t.Errorf("a heading with nothing under it should be title-only, got %q", d.Slides[0].LayoutID)
	}
	if d.Slides[1].LayoutID != deck.LayoutIDContent {
		t.Errorf("a heading with body text should be a content slide, got %q", d.Slides[1].LayoutID)
	}
}

// TestLower_APageBreakContinuesUnderTheSameTitle pins the declared degradation.
//
// The matrix says a page break becomes a new slide. The title carrying over is
// the part worth asserting: content after a break is the same subject
// continued, and a slide with no title is one an audience cannot place.
func TestLower_APageBreakContinuesUnderTheSameTitle(t *testing.T) {
	d := lower(t,
		heading(1, "Findings"),
		text("Before."),
		spec.Block{Kind: spec.BlockPageBreak, PageBreak: &spec.PageBreak{}},
		text("After."),
	)

	if got := len(d.Slides); got != 2 {
		t.Fatalf("want two slides, got %d", got)
	}
	for i := range d.Slides {
		if got := titleOf(t, d.Slides[i]); got != "Findings" {
			t.Errorf("slide %d is titled %q, want the title carried over", i+1, got)
		}
	}
	if got := bodyText(t, d.Slides[1]); got != "After." {
		t.Errorf("the continuation slide holds %q", got)
	}
}

// TestLower_APageBreakAtTheEndLeavesNoEmptySlide.
func TestLower_APageBreakAtTheEndLeavesNoEmptySlide(t *testing.T) {
	d := lower(t,
		heading(1, "Findings"),
		text("Body."),
		spec.Block{Kind: spec.BlockPageBreak, PageBreak: &spec.PageBreak{}},
	)
	if got := len(d.Slides); got != 1 {
		t.Fatalf("want one slide, got %d", got)
	}
}

// TestLower_AnAssetGetsItsOwnSlide pins the rule that avoids measuring text.
func TestLower_AnAssetGetsItsOwnSlide(t *testing.T) {
	d := lower(t,
		heading(1, "Results"),
		text("Prose."),
		spec.Block{Kind: spec.BlockAsset, Asset: &spec.Asset{Handle: pngURI(), AltText: "a chart"}},
		text("More prose."),
	)

	if got := len(d.Slides); got != 3 {
		t.Fatalf("want three slides — text, picture, text — got %d", got)
	}
	for i := range d.Slides {
		if got := titleOf(t, d.Slides[i]); got != "Results" {
			t.Errorf("slide %d is titled %q, want the title on every slide of the run", i+1, got)
		}
	}

	picture := pictureOf(t, d.Slides[1])
	if picture.AltText != "a chart" {
		t.Errorf("the picture's alt text is %q", picture.AltText)
	}
	if picture.MediaIndex != 0 || len(d.Media) != 1 {
		t.Errorf("want one media part referenced at index 0, got index %d of %d",
			picture.MediaIndex, len(d.Media))
	}
	// No text alongside it. A slide holds one or the other, which is what makes
	// the placement need no measurement.
	if got := bodyText(t, d.Slides[1]); got != "" {
		t.Errorf("the picture slide also carries text: %q", got)
	}
}

// TestLower_APictureIsCentredInTheLayoutsOwnContentRegion checks the frame
// comes from the layout rather than from a second computation of the same area.
func TestLower_APictureIsCentredInTheLayoutsOwnContentRegion(t *testing.T) {
	d := lower(t,
		heading(1, "Results"),
		spec.Block{Kind: spec.BlockAsset, Asset: &spec.Asset{Handle: pngURI()}},
	)

	var region deck.Frame
	for _, l := range d.Layouts {
		if l.ID != deck.LayoutIDContent {
			continue
		}
		for _, s := range l.Shapes {
			if s.Placeholder != nil && s.Placeholder.Type == deck.PlaceholderContent {
				region = s.Frame
			}
		}
	}

	frame := pictureShape(t, d.Slides[0]).Frame
	if got, want := frame.X+frame.Width/2, region.X+region.Width/2; got != want {
		t.Errorf("the picture's centre is at x=%d, the region's at x=%d", got, want)
	}
	if got, want := frame.Y+frame.Height/2, region.Y+region.Height/2; got != want {
		t.Errorf("the picture's centre is at y=%d, the region's at y=%d", got, want)
	}
}

// TestLower_NotesBecomeTheSpeakerNotesOfTheSlideTheyFollow.
func TestLower_NotesBecomeTheSpeakerNotesOfTheSlideTheyFollow(t *testing.T) {
	d := lower(t,
		heading(1, "One"),
		text("Body."),
		spec.Block{Kind: spec.BlockNotes, Notes: &spec.Notes{Content: "Say this."}},
		heading(1, "Two"),
		text("More."),
	)

	if got := len(d.Slides); got != 2 {
		t.Fatalf("want two slides, got %d", got)
	}
	if got := d.Slides[0].Notes; got != "Say this." {
		t.Errorf("the note landed on %q, want the slide it follows", got)
	}
	if d.Slides[1].Notes != "" {
		t.Errorf("the second slide picked up a note it did not have")
	}
}

// TestLower_ANoteAfterTheLastContentStillLands.
//
// There is no slide after it to annotate, so it belongs to the slide before —
// the only reading available, and one worth pinning because the obvious
// implementation drops it.
func TestLower_ANoteAfterTheLastContentStillLands(t *testing.T) {
	d := lower(t,
		heading(1, "One"),
		text("Body."),
		spec.Block{Kind: spec.BlockNotes, Notes: &spec.Notes{Content: "Trailing."}},
	)

	if got := len(d.Slides); got != 1 {
		t.Fatalf("want one slide, got %d", got)
	}
	if got := d.Slides[0].Notes; got != "Trailing." {
		t.Errorf("the trailing note is %q, want it on the last slide", got)
	}
}

// TestLower_ASectionBoundaryDoesNotCarryATitleAcross.
func TestLower_ASectionBoundaryDoesNotCarryATitleAcross(t *testing.T) {
	s := &spec.Spec{
		FormatVersion: spec.FormatVersion,
		Title:         "Report",
		Sections: []spec.Section{
			{ID: "a", Blocks: []spec.Block{heading(1, "Alpha"), text("One.")}},
			{ID: "b", Blocks: []spec.Block{text("Two.")}},
		},
	}
	res, err := resolve.Resolve(context.Background(), s, resolve.Options{Format: artifact.FormatPPTX})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	d, err := deck.Lower(res.Doc)
	if err != nil {
		t.Fatalf("Lower: %v", err)
	}

	if got := len(d.Slides); got != 2 {
		t.Fatalf("want two slides, got %d", got)
	}
	if got := titleOf(t, d.Slides[1]); got != "" {
		t.Errorf("the second section's slide is titled %q; a section keeps its own heading", got)
	}
}

// TestLower_NoTitleSlideIsInvented.
//
// A document title is metadata. Turning it into a cover means deciding that the
// first thing in a deck is a cover and inventing a subtitle to put under it,
// which is a section vocabulary arriving by the back door.
func TestLower_NoTitleSlideIsInvented(t *testing.T) {
	d := lower(t, heading(1, "Findings"), text("Body."))

	if d.Title != "Report" {
		t.Errorf("the document title should reach the package properties, got %q", d.Title)
	}
	for i, s := range d.Slides {
		if s.LayoutID == deck.LayoutIDTitle {
			t.Errorf("slide %d uses the title layout; the lowering invented a cover", i+1)
		}
	}
}

// TestLower_ARunInTheThemesOwnStyleStatesNothing is the overrides-only rule,
// asserted where it matters.
//
// A slide that restates the size, family and colour its master already gives it
// is a slide that stops following the master: changing the theme changes
// nothing, because every run overrides it. The failure is invisible until
// somebody tries to restyle the deck.
func TestLower_ARunInTheThemesOwnStyleStatesNothing(t *testing.T) {
	d := lower(t, heading(1, "Findings"), text("Plain prose."))

	title := d.Slides[0].Shapes[0].Text.Paragraphs[0].Runs[0]
	if !title.Style.IsZero() {
		t.Errorf("the title run carries %+v; a level-one heading is exactly what the title style gives", title.Style)
	}

	body := d.Slides[0].Shapes[1].Text.Paragraphs[0].Runs[0]
	if !body.Style.IsZero() {
		t.Errorf("the body run carries %+v; plain prose is exactly what the body style gives", body.Style)
	}
}

// TestLower_ADeeperHeadingKeepsItsOwnSize.
//
// The other half of the rule above. A level-two heading is smaller than a
// level-one, so its title run has to say so — promoting it to the title size
// would be overruling the author's outline.
func TestLower_ADeeperHeadingKeepsItsOwnSize(t *testing.T) {
	d := lower(t, heading(2, "Detail"), text("Body."))

	run := d.Slides[0].Shapes[0].Text.Paragraphs[0].Runs[0]
	if run.Style.SizeEMU == 0 {
		t.Fatal("a level-two heading states no size, so it renders at the level-one size")
	}
	if run.Style.SizeEMU >= titleStyleSize(d) {
		t.Errorf("a level-two heading is %d EMU against a title style of %d",
			run.Style.SizeEMU, titleStyleSize(d))
	}
}

// TestLower_AMarkedRunStatesOnlyWhatTheMarkChanged.
func TestLower_AMarkedRunStatesOnlyWhatTheMarkChanged(t *testing.T) {
	d := lower(t, heading(1, "Findings"), text("Emphasised.", "flagged"))

	run := d.Slides[0].Shapes[1].Text.Paragraphs[0].Runs[0]
	if run.Style.SizeEMU != 0 {
		t.Errorf("a marked run restates its size as %d", run.Style.SizeEMU)
	}
	if run.Style.Font != "" {
		t.Errorf("a marked run restates its family as %q", run.Style.Font)
	}
	if run.Style.Bold == deck.ToggleInherit && run.Style.Italic == deck.ToggleInherit && run.Style.Color == "" {
		t.Error("the mark changed nothing at all, so this test is asserting the absence of an absence")
	}
	if c := run.Style.Color; c != "" && !strings.HasPrefix(c, "+") {
		t.Errorf("a marked run states the literal colour %q rather than a scheme reference", c)
	}
}

// TestLower_ASpacerBecomesAnEmptyParagraphOfThatHeight.
func TestLower_ASpacerBecomesAnEmptyParagraphOfThatHeight(t *testing.T) {
	d := lower(t,
		heading(1, "Findings"),
		text("Above."),
		spec.Block{Kind: spec.BlockSpacer, Spacer: &spec.Spacer{Height: spec.Points(24)}},
		text("Below."),
	)

	paragraphs := d.Slides[0].Shapes[1].Text.Paragraphs
	if len(paragraphs) != 3 {
		t.Fatalf("want three paragraphs, got %d", len(paragraphs))
	}
	gap := paragraphs[1]
	if len(gap.Runs) != 0 {
		t.Error("the spacer carries text")
	}
	if got, want := gap.EndStyle.SizeEMU, int64(24*12700); got != want {
		t.Errorf("the spacer is %d EMU tall, want %d", got, want)
	}
}

// TestLower_ATableIsRefusedRatherThanDropped.
//
// The matrix declares that a table renders in a deck, and the writer does not
// do it yet. A loud failure naming the block is the honest state; a silently
// missing table is the failure mode this library exists to prevent.
func TestLower_ATableIsRefusedRatherThanDropped(t *testing.T) {
	_, err := deck.Lower(resolved(t, heading(1, "Findings"), spec.Block{
		Kind: spec.BlockTable,
		Table: &spec.Table{
			ColumnHeaders: spec.HeaderTree{{Label: "h"}},
			Body:          [][]spec.Cell{{{Text: "1"}}},
		},
	}))
	if !verr.HasCode(err, verr.VELLUM_DECK_BLOCK_UNSUPPORTED) {
		t.Fatalf("want VELLUM_DECK_BLOCK_UNSUPPORTED, got %v", err)
	}
}

// TestLower_TheSchemeIsBuiltFromTheThemesRoles checks the mapping table.
func TestLower_TheSchemeIsBuiltFromTheThemesRoles(t *testing.T) {
	d := resolved(t, heading(1, "Findings"), text("Body."))
	out, err := deck.Lower(d)
	if err != nil {
		t.Fatalf("Lower: %v", err)
	}

	for _, c := range []struct {
		slot string
		got  string
		role theme.ColorRole
	}{
		{"dk1", out.Theme.Colors.Dark1, theme.ColorText},
		{"lt1", out.Theme.Colors.Light1, theme.ColorBackground},
		{"dk2", out.Theme.Colors.Dark2, theme.ColorHeading},
		{"accent1", out.Theme.Colors.Accent1, theme.ColorAccent},
		{"accent2", out.Theme.Colors.Accent2, theme.ColorTableHeaderBackground},
	} {
		want, ok := d.Palette.Lookup(c.role)
		if !ok {
			t.Fatalf("the built-in theme declares no %s role", c.role)
		}
		if c.got != want {
			t.Errorf("%s is %q, want the theme's %s role %q", c.slot, c.got, c.role, want)
		}
	}
}

// TestLower_TheDesignComesFromTheThemeNotFromTheContent.
//
// A document with no body text still needs a body size: the master declares one
// and a slide's text resolves against it. Inferring it from the runs gives no
// answer at all here, which is the defect this replaced.
func TestLower_TheDesignComesFromTheThemeNotFromTheContent(t *testing.T) {
	d := lower(t, heading(1, "Nothing But A Heading"))

	levels := d.Masters[0].TextStyles.Body.Levels
	if len(levels) == 0 || levels[0].SizeEMU == 0 {
		t.Fatal("a deck of one heading has no body size, so its body text would render invisible")
	}
	if got := len(levels); got != 9 {
		t.Errorf("want nine outline levels declared, got %d; a level the master omits falls to the reader's defaults", got)
	}
}

// TestLower_IsDeterministic.
func TestLower_IsDeterministic(t *testing.T) {
	blocks := []spec.Block{
		heading(1, "Findings"),
		text("Prose."),
		spec.Block{Kind: spec.BlockAsset, Asset: &spec.Asset{Handle: pngURI()}},
		spec.Block{Kind: spec.BlockNotes, Notes: &spec.Notes{Content: "Say this."}},
	}

	first := write(t, lower(t, blocks...))
	for i := 0; i < 25; i++ {
		if got := write(t, lower(t, blocks...)); !equalBytes(first, got) {
			t.Fatalf("lowering %d produced different bytes", i+1)
		}
	}
}

// titleOf returns the text of a slide's title placeholder, or the empty string.
func titleOf(t *testing.T, s deck.Slide) string {
	t.Helper()

	for _, sh := range s.Shapes {
		if sh.Placeholder == nil || sh.Placeholder.Type != deck.PlaceholderTitle {
			continue
		}
		return textOf(sh.Text)
	}
	return ""
}

// bodyText returns the text of a slide's content placeholder.
func bodyText(t *testing.T, s deck.Slide) string {
	t.Helper()

	for _, sh := range s.Shapes {
		if sh.Placeholder == nil || sh.Placeholder.Type != deck.PlaceholderContent {
			continue
		}
		return textOf(sh.Text)
	}
	return ""
}

func textOf(b *deck.TextBody) string {
	if b == nil {
		return ""
	}
	var out strings.Builder
	for _, p := range b.Paragraphs {
		for _, r := range p.Runs {
			out.WriteString(r.Text)
		}
	}
	return out.String()
}

func pictureShape(t *testing.T, s deck.Slide) deck.Shape {
	t.Helper()

	for _, sh := range s.Shapes {
		if sh.Picture != nil {
			return sh
		}
	}
	t.Fatal("the slide carries no picture")
	return deck.Shape{}
}

func pictureOf(t *testing.T, s deck.Slide) *deck.Picture {
	t.Helper()
	return pictureShape(t, s).Picture
}

func titleStyleSize(d *deck.Deck) int64 {
	levels := d.Masters[0].TextStyles.Title.Levels
	if len(levels) == 0 {
		return 0
	}
	return levels[0].SizeEMU
}
