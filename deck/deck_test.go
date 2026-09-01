package deck_test

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/frankbardon/vellum/deck"
	verr "github.com/frankbardon/vellum/errors"
)

func TestAuthor_ProducesOneMasterAndFourLayouts(t *testing.T) {
	d := authored(t)

	if len(d.Masters) != 1 {
		t.Fatalf("want one master, got %d", len(d.Masters))
	}
	if got, want := len(d.Layouts), 4; got != want {
		t.Fatalf("want %d layouts, got %d", want, got)
	}
	for _, l := range d.Layouts {
		if l.MasterID != deck.MasterID {
			t.Errorf("layout %q names master %q, want %q", l.ID, l.MasterID, deck.MasterID)
		}
	}
	if err := d.Validate(); err != nil {
		t.Fatalf("an authored deck must validate: %v", err)
	}
}

// TestAuthor_TheTitleBandIsTwoLinesOfTheTitleSize pins the one measurement in
// the authored geometry that is not simply a margin.
//
// A title band sized to one line puts a wrapped title into the body; sized to a
// constant it is right at one type size and wrong at every other. Two lines of
// whatever size the design declared is the rule, and it is asserted here so a
// change to it is deliberate.
func TestAuthor_TheTitleBandIsTwoLinesOfTheTitleSize(t *testing.T) {
	des := design()
	d := authored(t)

	var title deck.Frame
	for _, s := range d.Masters[0].Shapes {
		if s.Placeholder != nil && s.Placeholder.Type == deck.PlaceholderTitle {
			title = s.Frame
		}
	}

	want := int64(des.LineHeight * 2 * float64(des.TitleSize))
	if diff := title.Height - want; diff > 1 || diff < -1 {
		t.Fatalf("title band is %d EMU, want %d", title.Height, want)
	}
	if title.X != des.MarginLeft || title.Y != des.MarginTop {
		t.Fatalf("title band sits at (%d,%d), want the top-left margin (%d,%d)",
			title.X, title.Y, des.MarginLeft, des.MarginTop)
	}
}

func TestAuthor_RejectsADesignWithNothingToSizeTextBy(t *testing.T) {
	for name, mutate := range map[string]func(*deck.Design){
		"no body sizes": func(d *deck.Design) { d.BodySizes = nil },
		"no title size": func(d *deck.Design) { d.TitleSize = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			des := design()
			mutate(&des)
			if _, err := deck.Author(des); !verr.HasCode(err, verr.VELLUM_THEME_INVALID) {
				t.Fatalf("want VELLUM_THEME_INVALID, got %v", err)
			}
		})
	}
}

// TestWrite_PackageStructure names every part a deck must carry.
func TestWrite_PackageStructure(t *testing.T) {
	p := unzip(t, write(t, sample(t)))

	for _, name := range []string{
		"[Content_Types].xml",
		"_rels/.rels",
		"docProps/app.xml",
		"docProps/core.xml",
		"ppt/presentation.xml",
		"ppt/_rels/presentation.xml.rels",
		"ppt/presProps.xml",
		"ppt/theme/theme1.xml",
		"ppt/slideMasters/slideMaster1.xml",
		"ppt/slideMasters/_rels/slideMaster1.xml.rels",
		"ppt/slideLayouts/slideLayout1.xml",
		"ppt/slideLayouts/_rels/slideLayout1.xml.rels",
		"ppt/slideLayouts/slideLayout4.xml",
		"ppt/slides/slide1.xml",
		"ppt/slides/_rels/slide1.xml.rels",
		"ppt/slides/slide2.xml",
		"ppt/notesMasters/notesMaster1.xml",
		"ppt/notesSlides/notesSlide1.xml",
	} {
		if !p.has(name) {
			t.Errorf("the package has no %s", name)
		}
	}

	if p.names[0] != "[Content_Types].xml" {
		t.Errorf("[Content_Types].xml must be the first entry, got %q", p.names[0])
	}
}

// TestWrite_ADeckWithoutNotesCarriesNoNotesParts is the other half of the
// structure check.
//
// An authored package is one whose parts are all doing something. A notes
// master in a deck with no notes is furniture nobody asked for, and it is the
// kind of thing that gets written once and then carried forever because
// removing it looks risky.
func TestWrite_ADeckWithoutNotesCarriesNoNotesParts(t *testing.T) {
	d := sample(t)
	for i := range d.Slides {
		d.Slides[i].Notes = ""
	}
	p := unzip(t, write(t, d))

	for _, name := range []string{
		"ppt/notesMasters/notesMaster1.xml",
		"ppt/notesSlides/notesSlide1.xml",
		"ppt/theme/theme2.xml",
	} {
		if p.has(name) {
			t.Errorf("a deck with no notes carries %s", name)
		}
	}
	if strings.Contains(p.part(t, "ppt/presentation.xml"), "notesMasterIdLst") {
		t.Error("a deck with no notes declares a notes master in presentation.xml")
	}
}

// TestWrite_EveryRelationshipResolves walks every r:id in every part and checks
// it names a relationship of that part, and that the relationship's target is a
// part the package carries.
//
// This is the check the format most needs. PresentationML is a graph of parts
// referring to one another by identifier, and a dangling identifier produces a
// slide with no layout or a picture that draws nothing — a deck that opens,
// with no diagnostic anywhere, missing the thing the reference pointed at.
func TestWrite_EveryRelationshipResolves(t *testing.T) {
	p := unzip(t, write(t, sample(t)))

	for _, name := range p.sorted() {
		if !strings.HasSuffix(name, ".xml") || strings.Contains(name, "_rels/") {
			continue
		}
		rels := relationshipsOf(t, p, name)

		found := referencedIDs(t, p.parts[name])
		if name == "ppt/presentation.xml" && len(found) == 0 {
			t.Fatal("presentation.xml references nothing; the walker is not reading r:id attributes")
		}
		for _, id := range found {
			target, ok := rels[id]
			if !ok {
				t.Errorf("%s references %s, which its relationships part does not declare", name, id)
				continue
			}
			resolved := resolveTarget(name, target)
			if !p.has(resolved) {
				t.Errorf("%s references %s, which resolves to %q — a part the package does not carry",
					name, id, resolved)
			}
		}
	}
}

// TestWrite_IsDeterministic emits the same deck many times and requires one
// digest.
//
// In-process repetition, which is what catches a map ranged on the output path.
// The cross-process arm lives in the determinism harness, where the golden case
// registers.
func TestWrite_IsDeterministic(t *testing.T) {
	first := write(t, sample(t))
	for i := 0; i < 50; i++ {
		if got := write(t, sample(t)); !equalBytes(first, got) {
			t.Fatalf("write %d differs from the first", i+1)
		}
	}
}

// TestWrite_ColoursOnSlidesAreSchemeReferences is the restylability invariant,
// asserted rather than described.
//
// Every colour outside the theme part is a scheme reference. A literal on a
// master, a layout or a slide is a colour the theme cannot change, and the
// deck it produces looks correct and cannot be restyled — which is worse than
// one that looks wrong, because nothing reports it.
func TestWrite_ColoursOnSlidesAreSchemeReferences(t *testing.T) {
	p := unzip(t, write(t, sample(t)))

	for _, name := range p.sorted() {
		switch {
		case !strings.HasPrefix(name, "ppt/"):
			continue
		case strings.HasPrefix(name, "ppt/theme/"):
			// The theme part is where the literals belong. It is the one part
			// whose whole job is to state them.
			continue
		}
		if i := strings.Index(string(p.parts[name]), "srgbClr"); i >= 0 {
			t.Errorf("%s carries a literal colour, which the theme cannot restyle:\n  %s",
				name, excerpt(string(p.parts[name]), i))
		}
	}

	// The other half, so the check cannot pass by there being no colours to
	// state. A master whose text styles name no colour at all would satisfy
	// every assertion above and produce a deck with no stated text colour.
	master := p.part(t, "ppt/slideMasters/slideMaster1.xml")
	if !strings.Contains(master, `<a:schemeClr val="tx1"/>`) {
		t.Error("the master states no scheme colour, so the check above had nothing to look at")
	}
}

// TestWrite_ThemeCarriesTheDesignsColours checks the other direction: the
// literals the design supplied do reach the theme part.
func TestWrite_ThemeCarriesTheDesignsColours(t *testing.T) {
	p := unzip(t, write(t, sample(t)))
	part := p.part(t, "ppt/theme/theme1.xml")

	des := design()
	for slot, want := range map[string]string{
		"dk1":      des.Colors.Dark1,
		"lt1":      des.Colors.Light1,
		"dk2":      des.Colors.Dark2,
		"accent1":  des.Colors.Accent1,
		"folHlink": des.Colors.FollowedHyperlink,
	} {
		fragment := `<a:` + slot + `><a:srgbClr val="` + want + `"/></a:` + slot + `>`
		if !strings.Contains(part, fragment) {
			t.Errorf("theme1.xml does not carry %s as %s", slot, want)
		}
	}
	for _, family := range []string{des.HeadingFamily, des.BodyFamily} {
		if !strings.Contains(part, `<a:latin typeface="`+family+`"/>`) {
			t.Errorf("theme1.xml does not name the family %q", family)
		}
	}
}

// TestWrite_TitlePlaceholdersCarryNoIndex pins the asymmetry that is easiest to
// get wrong and hardest to see.
//
// A title matches its layout counterpart by type and every other placeholder by
// index. A title carrying an index, or a body carrying none, inherits from the
// wrong shape — the slide opens with its placeholders stacked on one another
// and nothing reports a problem.
func TestWrite_TitlePlaceholdersCarryNoIndex(t *testing.T) {
	p := unzip(t, write(t, sample(t)))

	seen := 0
	for _, name := range p.sorted() {
		if !strings.HasPrefix(name, "ppt/slide") && !strings.HasPrefix(name, "ppt/notes") {
			continue
		}
		if strings.Contains(name, "_rels/") {
			continue
		}
		for _, ph := range placeholders(t, p.parts[name]) {
			seen++
			isTitle := ph.kind == "title" || ph.kind == "ctrTitle"
			if isTitle && ph.hasIndex {
				t.Errorf("%s: a %s placeholder carries idx=%q; a title matches by type alone",
					name, ph.kind, ph.index)
			}
			if !isTitle && !ph.hasIndex {
				t.Errorf("%s: a %q placeholder carries no idx; everything but a title matches by index",
					name, ph.kind)
			}
		}
	}

	// Both arms of the rule need an example, or a walker that found nothing
	// would report the deck correct.
	if seen < 6 {
		t.Fatalf("only %d placeholders were examined; the walker is not finding them", seen)
	}
}

// TestWrite_NotesReachTheNotesSlide checks the note's own paragraphs survive.
func TestWrite_NotesReachTheNotesSlide(t *testing.T) {
	p := unzip(t, write(t, sample(t)))
	part := p.part(t, "ppt/notesSlides/notesSlide1.xml")

	for _, want := range []string{"Speak to the three models.", "Do not read the slide."} {
		if !strings.Contains(part, want) {
			t.Errorf("the notes slide does not carry %q", want)
		}
	}
	// Three paragraphs: two lines and the blank between them. A note whose
	// blank line is dropped reads as one paragraph.
	if got := strings.Count(part, "<a:p>") + strings.Count(part, "<a:p/>"); got != 3 {
		t.Errorf("want three paragraphs in the notes slide, got %d", got)
	}
}

// TestValidate_RejectsADeckTheWriterWouldSilentlyGetWrong drives every arm of
// the validator.
func TestValidate_RejectsADeckTheWriterWouldSilentlyGetWrong(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*deck.Deck)
	}{
		{"no master", func(d *deck.Deck) { d.Masters = nil }},
		{"no layout", func(d *deck.Deck) { d.Layouts = nil }},
		{"a master with no id", func(d *deck.Deck) { d.Masters[0].ID = "" }},
		{"two masters sharing an id", func(d *deck.Deck) {
			d.Masters = append(d.Masters, d.Masters[0])
		}},
		{"two layouts sharing an id", func(d *deck.Deck) {
			d.Layouts = append(d.Layouts, d.Layouts[0])
		}},
		{"a layout naming no master", func(d *deck.Deck) { d.Layouts[0].MasterID = "absent" }},
		{"a slide naming no layout", func(d *deck.Deck) { d.Slides[0].LayoutID = "absent" }},
		{"a shape carrying nothing", func(d *deck.Deck) {
			d.Slides[0].Shapes[0].Text = nil
		}},
		{"a shape carrying two things", func(d *deck.Deck) {
			d.Slides[0].Shapes[0].Picture = &deck.Picture{}
		}},
		{"a free shape with no frame", func(d *deck.Deck) {
			d.Slides[0].Shapes[0].Placeholder = nil
		}},
		{"a picture past the media", func(d *deck.Deck) {
			d.Slides[0].Shapes[0].Text = nil
			d.Slides[0].Shapes[0].Picture = &deck.Picture{MediaIndex: 3}
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := sample(t)
			tc.mutate(d)
			if err := d.Validate(); !verr.HasCode(err, verr.VELLUM_DECK_INVALID) {
				t.Fatalf("want VELLUM_DECK_INVALID, got %v", err)
			}
		})
	}
}

// placeholderRef is one p:ph element read back.
type placeholderRef struct {
	kind     string
	index    string
	hasIndex bool
}

func placeholders(t *testing.T, part []byte) []placeholderRef {
	t.Helper()

	var out []placeholderRef
	dec := xml.NewDecoder(strings.NewReader(string(part)))
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		start, ok := tok.(xml.StartElement)
		if !ok || start.Name.Local != "ph" {
			continue
		}
		var ref placeholderRef
		for _, a := range start.Attr {
			switch a.Name.Local {
			case "type":
				ref.kind = a.Value
			case "idx":
				ref.index, ref.hasIndex = a.Value, true
			}
		}
		out = append(out, ref)
	}
	return out
}

// referencedIDs returns every r:id and r:embed value a part carries.
func referencedIDs(t *testing.T, part []byte) []string {
	t.Helper()

	var out []string
	dec := xml.NewDecoder(strings.NewReader(string(part)))
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		for _, a := range start.Attr {
			if a.Name.Space == "http://schemas.openxmlformats.org/officeDocument/2006/relationships" {
				out = append(out, a.Value)
			}
		}
	}
	return out
}

// relationshipsOf reads the relationships declared for a part.
func relationshipsOf(t *testing.T, p pkg, part string) map[string]string {
	t.Helper()

	dir, file := splitName(part)
	relsName := dir + "_rels/" + file + ".rels"
	raw, ok := p.parts[relsName]
	if !ok {
		return nil
	}

	var doc struct {
		Rel []struct {
			ID     string `xml:"Id,attr"`
			Target string `xml:"Target,attr"`
			Mode   string `xml:"TargetMode,attr"`
		} `xml:"Relationship"`
	}
	if err := xml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parsing %s: %v", relsName, err)
	}

	out := make(map[string]string, len(doc.Rel))
	for _, r := range doc.Rel {
		if r.Mode == "External" {
			continue
		}
		out[r.ID] = r.Target
	}
	return out
}

// resolveTarget resolves a relationship target relative to the part that owns
// it, as a package reader does.
func resolveTarget(owner, target string) string {
	if strings.HasPrefix(target, "/") {
		return strings.TrimPrefix(target, "/")
	}
	dir, _ := splitName(owner)
	segments := strings.Split(dir+target, "/")

	out := make([]string, 0, len(segments))
	for _, s := range segments {
		switch s {
		case ".", "":
			continue
		case "..":
			if len(out) > 0 {
				out = out[:len(out)-1]
			}
		default:
			out = append(out, s)
		}
	}
	return strings.Join(out, "/")
}

func splitName(name string) (dir, file string) {
	i := strings.LastIndex(name, "/")
	if i < 0 {
		return "", name
	}
	return name[:i+1], name[i+1:]
}

func excerpt(s string, at int) string {
	from := at - 60
	if from < 0 {
		from = 0
	}
	to := at + 60
	if to > len(s) {
		to = len(s)
	}
	return s[from:to]
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
