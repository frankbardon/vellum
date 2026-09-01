package deck

import (
	"io"
	"sort"
	"strconv"
	"time"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/opc"
	"github.com/frankbardon/vellum/opc/zipdet"
)

// Part names this writer emits. The indexed parts are named by helper.
const (
	PartPresentation = "/ppt/presentation.xml"
	PartPresProps    = "/ppt/presProps.xml"
	PartTableStyles  = "/ppt/tableStyles.xml"
	PartTheme        = "/ppt/theme/theme1.xml"
	PartNotesTheme   = "/ppt/theme/theme2.xml"
	PartNotesMaster  = "/ppt/notesMasters/notesMaster1.xml"
	PartCoreProps    = "/docProps/core.xml"
	PartAppProps     = "/docProps/app.xml"
	PartCustom       = "/docProps/custom.xml"
)

// WriteOptions configures a write. The zero value is the deterministic default.
type WriteOptions struct {
	// SourceDateEpoch stamps every date the package carries — the zip entry
	// timestamps and the core properties alike. The zero value selects the
	// pinned epoch.
	SourceDateEpoch time.Time

	// Producer names the software in the package's extended properties.
	Producer string
}

// defaultProducer is what appears in a deck's application property when the
// caller names nothing.
const defaultProducer = "Vellum"

// writer carries the state of one package assembly.
//
// Relationship identifiers are resolved here, once, before any part is
// serialised — because presentation.xml references a slide by identifier and
// the relationships part is written afterwards, so the two must agree and only
// one of them can be authoritative. opc derives identifiers from a sorted walk
// of the relationships' own content, which makes opc that authority.
type writer struct {
	deck     *Deck
	epoch    time.Time
	producer string

	// masterRels and slideRels map an index to the identifier presentation.xml
	// references it by.
	masterRels []string
	slideRels  []string

	// layoutRels maps a layout index to the identifier its master references
	// it by.
	layoutRels []string

	// notesMasterRel is the identifier presentation.xml references the notes
	// master by, empty when the deck has no notes.
	notesMasterRel string

	// slideLayoutRel maps a slide index to the identifier that slide
	// references its layout by, and slideMediaRels to the identifiers it
	// references its pictures by.
	slideLayoutRel []string
	slideMediaRels [][]string
	slideNotesRel  []string

	// notesSlides maps a slide index to its notes slide number, or zero for a
	// slide with no notes. Numbered over the slides that have notes rather
	// than over all slides, which is what an authored deck looks like.
	notesSlides []int
	notesCount  int

	// hasTable records whether any slide carries a table, because the table
	// style part exists only when something references it. An unreferenced
	// style part is furniture nobody asked for, and it is the kind of thing
	// that gets written once and carried forever.
	hasTable bool
}

// planTables records whether the deck needs a table style part.
func (w *writer) planTables() {
	for i := range w.deck.Slides {
		for j := range w.deck.Slides[i].Shapes {
			if w.deck.Slides[i].Shapes[j].Table != nil {
				w.hasTable = true
				return
			}
		}
	}
}

// Package assembles the OPC package for this deck.
func (d *Deck) Package(opts WriteOptions) (*opc.Package, error) {
	if err := d.Validate(); err != nil {
		return nil, err
	}

	epoch := opts.SourceDateEpoch
	if epoch.IsZero() {
		epoch = zipdet.PinnedEpoch
	}
	producer := opts.Producer
	if producer == "" {
		producer = defaultProducer
	}

	w := &writer{deck: d, epoch: epoch, producer: producer}
	w.planNotes()
	w.planTables()

	p := opc.New()
	ct := p.ContentTypes()
	ct.SetDefault("xml", ctXML)
	ct.SetDefault("rels", "application/vnd.openxmlformats-package.relationships+xml")

	// Relationships first, for every part that references another by
	// identifier. The identifiers must be settled before any markup naming
	// them is serialised.
	if err := w.buildRelationships(p); err != nil {
		return nil, err
	}

	if err := p.Put(&opc.Part{Name: PartPresentation, ContentType: ctPresentation,
		Data: w.presentationXML()}); err != nil {
		return nil, err
	}
	if err := p.Put(&opc.Part{Name: PartPresProps, ContentType: ctPresProps,
		Data: w.presPropsXML()}); err != nil {
		return nil, err
	}
	if err := p.Put(&opc.Part{Name: PartTheme, ContentType: ctTheme,
		Data: w.themeXML(d.Theme)}); err != nil {
		return nil, err
	}
	if w.hasTable {
		if err := p.Put(&opc.Part{Name: PartTableStyles, ContentType: ctTableStyles,
			Data: w.tableStylesXML()}); err != nil {
			return nil, err
		}
	}

	for i := range d.Masters {
		if err := p.Put(&opc.Part{Name: masterPartName(i), ContentType: ctSlideMaster,
			Data: w.masterXML(i)}); err != nil {
			return nil, err
		}
	}
	for i := range d.Layouts {
		if err := p.Put(&opc.Part{Name: layoutPartName(i), ContentType: ctSlideLayout,
			Data: w.layoutXML(i)}); err != nil {
			return nil, err
		}
	}
	for i := range d.Slides {
		if err := p.Put(&opc.Part{Name: slidePartName(i), ContentType: ctSlide,
			Data: w.slideXML(i)}); err != nil {
			return nil, err
		}
	}

	if w.notesCount > 0 {
		if err := p.Put(&opc.Part{Name: PartNotesTheme, ContentType: ctTheme,
			Data: w.themeXML(d.Theme)}); err != nil {
			return nil, err
		}
		if err := p.Put(&opc.Part{Name: PartNotesMaster, ContentType: ctNotesMaster,
			Data: w.notesMasterXML()}); err != nil {
			return nil, err
		}
		for i := range d.Slides {
			if w.notesSlides[i] == 0 {
				continue
			}
			if err := p.Put(&opc.Part{Name: notesPartName(w.notesSlides[i] - 1),
				ContentType: ctNotesSlide, Data: w.notesSlideXML(i)}); err != nil {
				return nil, err
			}
		}
	}

	for i := range d.Media {
		m := &d.Media[i]
		name := mediaPartName(i, m.MediaType)
		ct.SetOverride(name, m.MediaType)
		if err := p.Put(&opc.Part{Name: name, ContentType: m.MediaType, Data: m.Bytes}); err != nil {
			return nil, err
		}
	}

	if err := p.Put(&opc.Part{Name: PartCoreProps, ContentType: ctCoreProperties,
		Data: w.corePropsXML()}); err != nil {
		return nil, err
	}
	if err := p.Put(&opc.Part{Name: PartAppProps, ContentType: ctExtendedProps,
		Data: w.appPropsXML()}); err != nil {
		return nil, err
	}
	if d.Provenance != nil {
		if err := p.Put(&opc.Part{Name: PartCustom, ContentType: ctCustomProps,
			Data: w.customPropsXML()}); err != nil {
			return nil, err
		}
	}

	root := p.Relationships("/")
	for _, r := range []struct{ typ, target string }{
		{relExtendedProps, "docProps/app.xml"},
		{relOfficeDocument, "ppt/presentation.xml"},
		{relCoreProperties, "docProps/core.xml"},
	} {
		if _, err := root.Add(r.typ, r.target, opc.TargetInternal); err != nil {
			return nil, err
		}
	}
	if d.Provenance != nil {
		if _, err := root.Add(relCustomProps, "docProps/custom.xml", opc.TargetInternal); err != nil {
			return nil, err
		}
	}

	return p, nil
}

// planNotes assigns notes slide numbers.
func (w *writer) planNotes() {
	w.notesSlides = make([]int, len(w.deck.Slides))
	for i := range w.deck.Slides {
		if w.deck.Slides[i].Notes == "" {
			continue
		}
		w.notesCount++
		w.notesSlides[i] = w.notesCount
	}
}

// buildRelationships declares every part's relationships and resolves the
// identifiers the markup will reference.
//
// Declared, then frozen, then read back. The freeze is what makes the read
// meaningful: before it, identifiers are in insertion order and will change
// when the set is serialised.
func (w *writer) buildRelationships(p *opc.Package) error {
	d := w.deck

	// The presentation part.
	pres := p.Relationships(PartPresentation)
	pres.AlwaysEmit()
	for i := range d.Masters {
		if _, err := pres.Add(relSlideMaster, relative(PartPresentation, masterPartName(i)), opc.TargetInternal); err != nil {
			return err
		}
	}
	if w.notesCount > 0 {
		if _, err := pres.Add(relNotesMaster, relative(PartPresentation, PartNotesMaster), opc.TargetInternal); err != nil {
			return err
		}
	}
	for i := range d.Slides {
		if _, err := pres.Add(relSlide, relative(PartPresentation, slidePartName(i)), opc.TargetInternal); err != nil {
			return err
		}
	}
	if _, err := pres.Add(relPresProps, relative(PartPresentation, PartPresProps), opc.TargetInternal); err != nil {
		return err
	}
	if _, err := pres.Add(relTheme, relative(PartPresentation, PartTheme), opc.TargetInternal); err != nil {
		return err
	}
	if w.hasTable {
		if _, err := pres.Add(relTableStyles, relative(PartPresentation, PartTableStyles), opc.TargetInternal); err != nil {
			return err
		}
	}
	pres.Freeze()

	var err error
	w.masterRels, err = resolveEach(pres, relSlideMaster, len(d.Masters), func(i int) string {
		return relative(PartPresentation, masterPartName(i))
	})
	if err != nil {
		return err
	}
	w.slideRels, err = resolveEach(pres, relSlide, len(d.Slides), func(i int) string {
		return relative(PartPresentation, slidePartName(i))
	})
	if err != nil {
		return err
	}
	if w.notesCount > 0 {
		target := relative(PartPresentation, PartNotesMaster)
		id, ok := pres.IDFor(relNotesMaster, target)
		if !ok {
			return missingRelationship(relNotesMaster, target)
		}
		w.notesMasterRel = id
	}

	// The masters. Each names its theme and the layouts below it.
	w.layoutRels = make([]string, len(d.Layouts))
	for mi := range d.Masters {
		rels := p.Relationships(masterPartName(mi))
		rels.AlwaysEmit()
		if _, err := rels.Add(relTheme, relative(masterPartName(mi), PartTheme), opc.TargetInternal); err != nil {
			return err
		}
		for li := range d.Layouts {
			if d.Layouts[li].MasterID != d.Masters[mi].ID {
				continue
			}
			if _, err := rels.Add(relSlideLayout, relative(masterPartName(mi), layoutPartName(li)), opc.TargetInternal); err != nil {
				return err
			}
		}
		rels.Freeze()

		for li := range d.Layouts {
			if d.Layouts[li].MasterID != d.Masters[mi].ID {
				continue
			}
			id, ok := rels.IDFor(relSlideLayout, relative(masterPartName(mi), layoutPartName(li)))
			if !ok {
				return missingRelationship(relSlideLayout, layoutPartName(li))
			}
			w.layoutRels[li] = id
		}
	}

	// The layouts. Each names the master above it.
	for li := range d.Layouts {
		mi := w.masterIndex(d.Layouts[li].MasterID)
		rels := p.Relationships(layoutPartName(li))
		rels.AlwaysEmit()
		if _, err := rels.Add(relSlideMaster, relative(layoutPartName(li), masterPartName(mi)), opc.TargetInternal); err != nil {
			return err
		}
		rels.Freeze()
	}

	// The notes master, when there is one.
	if w.notesCount > 0 {
		rels := p.Relationships(PartNotesMaster)
		rels.AlwaysEmit()
		if _, err := rels.Add(relTheme, relative(PartNotesMaster, PartNotesTheme), opc.TargetInternal); err != nil {
			return err
		}
		rels.Freeze()
	}

	// The slides. Each names its layout, its pictures and its notes.
	w.slideLayoutRel = make([]string, len(d.Slides))
	w.slideMediaRels = make([][]string, len(d.Slides))
	w.slideNotesRel = make([]string, len(d.Slides))
	for si := range d.Slides {
		li := w.layoutIndex(d.Slides[si].LayoutID)
		rels := p.Relationships(slidePartName(si))
		rels.AlwaysEmit()

		layoutTarget := relative(slidePartName(si), layoutPartName(li))
		if _, err := rels.Add(relSlideLayout, layoutTarget, opc.TargetInternal); err != nil {
			return err
		}

		used := w.mediaUsedBy(si)
		for _, mi := range used {
			target := relative(slidePartName(si), mediaPartName(mi, d.Media[mi].MediaType))
			if _, err := rels.Add(relImage, target, opc.TargetInternal); err != nil {
				return err
			}
		}
		if n := w.notesSlides[si]; n > 0 {
			if _, err := rels.Add(relNotesSlide, relative(slidePartName(si), notesPartName(n-1)), opc.TargetInternal); err != nil {
				return err
			}
		}
		rels.Freeze()

		id, ok := rels.IDFor(relSlideLayout, layoutTarget)
		if !ok {
			return missingRelationship(relSlideLayout, layoutPartName(li))
		}
		w.slideLayoutRel[si] = id

		w.slideMediaRels[si] = make([]string, len(d.Media))
		for _, mi := range used {
			target := relative(slidePartName(si), mediaPartName(mi, d.Media[mi].MediaType))
			id, ok := rels.IDFor(relImage, target)
			if !ok {
				return missingRelationship(relImage, target)
			}
			w.slideMediaRels[si][mi] = id
		}
		if n := w.notesSlides[si]; n > 0 {
			target := relative(slidePartName(si), notesPartName(n-1))
			id, ok := rels.IDFor(relNotesSlide, target)
			if !ok {
				return missingRelationship(relNotesSlide, target)
			}
			w.slideNotesRel[si] = id
		}
	}

	// The notes slides. Each names the slide it belongs to.
	for si := range d.Slides {
		n := w.notesSlides[si]
		if n == 0 {
			continue
		}
		rels := p.Relationships(notesPartName(n - 1))
		rels.AlwaysEmit()
		if _, err := rels.Add(relNotesMaster, relative(notesPartName(n-1), PartNotesMaster), opc.TargetInternal); err != nil {
			return err
		}
		if _, err := rels.Add(relSlide, relative(notesPartName(n-1), slidePartName(si)), opc.TargetInternal); err != nil {
			return err
		}
		rels.Freeze()
	}

	return nil
}

// mediaUsedBy returns the media indices a slide's pictures reference, in
// ascending order and without repeats.
//
// Ascending rather than in the order the shapes mention them, so a slide that
// draws the same two pictures in a different order still declares the same
// relationships. Both are deterministic; only one makes two such slides
// comparable.
func (w *writer) mediaUsedBy(slide int) []int {
	seen := make([]bool, len(w.deck.Media))
	var out []int
	for _, sh := range w.deck.Slides[slide].Shapes {
		if sh.Picture == nil {
			continue
		}
		for _, idx := range []int{sh.Picture.MediaIndex, sh.Picture.SVGMediaIndex - 1} {
			if idx < 0 || idx >= len(seen) || seen[idx] {
				continue
			}
			seen[idx] = true
			out = append(out, idx)
		}
	}
	sort.Ints(out)
	return out
}

// resolveEach reads back the identifiers for a run of same-typed relationships.
func resolveEach(rels *opc.Relationships, typ string, n int, target func(int) string) ([]string, error) {
	out := make([]string, n)
	for i := 0; i < n; i++ {
		id, ok := rels.IDFor(typ, target(i))
		if !ok {
			return nil, missingRelationship(typ, target(i))
		}
		out[i] = id
	}
	return out, nil
}

func missingRelationship(typ, target string) error {
	return verr.NewCodedErrorWithDetails(verr.VELLUM_OPC_RELATIONSHIP_INVALID,
		"a relationship the presentation markup references was not declared",
		map[string]any{"type": typ, "target": target})
}

// masterIndex returns the index of a master by ID, or zero.
func (w *writer) masterIndex(id string) int {
	for i := range w.deck.Masters {
		if w.deck.Masters[i].ID == id {
			return i
		}
	}
	return 0
}

// layoutIndex returns the index of a layout by ID, or zero.
func (w *writer) layoutIndex(id string) int {
	for i := range w.deck.Layouts {
		if w.deck.Layouts[i].ID == id {
			return i
		}
	}
	return 0
}

// Validate reports a deck the writer cannot serialise.
//
// Checked before anything is written rather than discovered part by part,
// because the failures here — a slide naming a layout that does not exist, a
// picture indexing past the media — all produce a package that opens and is
// silently wrong rather than one that fails.
func (d *Deck) Validate() error {
	if len(d.Masters) == 0 {
		return verr.NewCodedError(verr.VELLUM_DECK_INVALID,
			"the deck has no slide master; PresentationML has no notion of an unmastered slide")
	}
	if len(d.Layouts) == 0 {
		return verr.NewCodedError(verr.VELLUM_DECK_INVALID,
			"the deck has no slide layout")
	}

	masters := make(map[string]bool, len(d.Masters))
	for i := range d.Masters {
		if d.Masters[i].ID == "" {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_DECK_INVALID,
				"a slide master has no identifier", map[string]any{"master_index": i})
		}
		if masters[d.Masters[i].ID] {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_DECK_INVALID,
				"two slide masters share an identifier",
				map[string]any{"master_id": d.Masters[i].ID})
		}
		masters[d.Masters[i].ID] = true
	}

	layouts := make(map[string]bool, len(d.Layouts))
	for i := range d.Layouts {
		l := &d.Layouts[i]
		if l.ID == "" {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_DECK_INVALID,
				"a slide layout has no identifier", map[string]any{"layout_index": i})
		}
		if layouts[l.ID] {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_DECK_INVALID,
				"two slide layouts share an identifier", map[string]any{"layout_id": l.ID})
		}
		layouts[l.ID] = true
		if !masters[l.MasterID] {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_DECK_INVALID,
				"a slide layout names a master the deck does not carry",
				map[string]any{"layout_id": l.ID, "master_id": l.MasterID})
		}
	}

	for i := range d.Slides {
		s := &d.Slides[i]
		if !layouts[s.LayoutID] {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_DECK_INVALID,
				"a slide names a layout the deck does not carry",
				map[string]any{"slide_index": i, "layout_id": s.LayoutID})
		}
		for j := range s.Shapes {
			if err := d.validateShape(i, j, &s.Shapes[j]); err != nil {
				return err
			}
		}
	}
	return nil
}

func (d *Deck) validateShape(slide, index int, s *Shape) error {
	arms := 0
	for _, set := range []bool{s.Text != nil, s.Picture != nil, s.Table != nil} {
		if set {
			arms++
		}
	}
	if arms != 1 {
		return verr.NewCodedErrorWithDetails(verr.VELLUM_DECK_INVALID,
			"a shape must carry exactly one of text, a picture or a table",
			map[string]any{"slide_index": slide, "shape_index": index, "arms": arms})
	}
	if s.Placeholder == nil && s.Frame.IsZero() {
		return verr.NewCodedErrorWithDetails(verr.VELLUM_DECK_INVALID,
			"a shape that fills no placeholder must carry its own frame, or it is drawn at the origin with no size",
			map[string]any{"slide_index": slide, "shape_index": index})
	}
	if t := s.Table; t != nil {
		for r := range t.Rows {
			// One cell per grid column, always. DrawingML differs from
			// WordprocessingML here: a spanning cell does not replace the
			// cells it covers, it declares gridSpan and the covered cells stay
			// present carrying hMerge. A row with a spanning cell and no
			// covered cells is short, and a reader shifts everything after it
			// left.
			if got := len(t.Rows[r].Cells); got != len(t.Columns) {
				return verr.NewCodedErrorWithDetails(verr.VELLUM_DECK_INVALID,
					"a table row does not tile the grid; a reader draws a hole or shifts the remaining cells left",
					map[string]any{"slide_index": slide, "shape_index": index,
						"row": r, "row_span": got, "grid_width": len(t.Columns)})
			}
		}
	}
	if p := s.Picture; p != nil {
		if p.MediaIndex < 0 || p.MediaIndex >= len(d.Media) {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_DECK_INVALID,
				"a picture names a media index the deck does not carry",
				map[string]any{"slide_index": slide, "shape_index": index,
					"media_index": p.MediaIndex, "media_count": len(d.Media)})
		}
		if p.SVGMediaIndex > len(d.Media) {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_DECK_INVALID,
				"a picture names a vector rendition the deck does not carry",
				map[string]any{"slide_index": slide, "shape_index": index,
					"svg_media_index": p.SVGMediaIndex, "media_count": len(d.Media)})
		}
	}
	return nil
}

// WriteTo emits the deck as a .pptx.
func (d *Deck) WriteTo(w io.Writer, opts WriteOptions) error {
	p, err := d.Package(opts)
	if err != nil {
		return err
	}
	epoch := opts.SourceDateEpoch
	if epoch.IsZero() {
		epoch = zipdet.PinnedEpoch
	}
	return p.WriteTo(w, zipdet.WriteOptions{SourceDateEpoch: epoch})
}

func masterPartName(i int) string {
	return "/ppt/slideMasters/slideMaster" + strconv.Itoa(i+1) + ".xml"
}

func layoutPartName(i int) string {
	return "/ppt/slideLayouts/slideLayout" + strconv.Itoa(i+1) + ".xml"
}

func slidePartName(i int) string {
	return "/ppt/slides/slide" + strconv.Itoa(i+1) + ".xml"
}

func notesPartName(i int) string {
	return "/ppt/notesSlides/notesSlide" + strconv.Itoa(i+1) + ".xml"
}

// mediaPartName returns the OPC part name for an embedded image.
//
// Indexed by position in a slice that is itself ordered by content hash, so a
// deck's media part names are a function of what is in them rather than of the
// order the content mentioned them.
func mediaPartName(i int, mediaType string) string {
	return "/ppt/media/image" + strconv.Itoa(i+1) + "." + mediaExtension(mediaType)
}

// relative renders a part name as a relationship target relative to the part
// that owns the relationship.
//
// OPC permits an absolute target and every reader accepts one, but no authored
// package uses them, and a package whose targets do not look authored is one
// whose differences from an authored package have to be argued rather than
// observed.
func relative(owner, target string) string {
	ownerDir := owner[:lastSlash(owner)+1]
	if len(target) > len(ownerDir) && target[:len(ownerDir)] == ownerDir {
		return target[len(ownerDir):]
	}

	// Walk up from the owner's directory until the target is below it, then
	// down. Both names are absolute and slash-separated, so this is textual.
	up := 0
	dir := ownerDir
	for dir != "/" {
		dir = dir[:lastSlash(dir[:len(dir)-1])+1]
		up++
		if len(target) > len(dir) && target[:len(dir)] == dir {
			prefix := ""
			for i := 0; i < up; i++ {
				prefix += "../"
			}
			return prefix + target[len(dir):]
		}
	}
	return target
}

func lastSlash(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '/' {
			return i
		}
	}
	return -1
}

// sortedProvenanceKeys orders the custom properties. Declared here so the
// property order is one decision in one place.
func sortedProvenanceKeys(props []customProperty) []customProperty {
	out := append([]customProperty(nil), props...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
