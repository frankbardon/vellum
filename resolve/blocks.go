package resolve

import (
	"math"
	"time"

	"github.com/frankbardon/vellum/asset"
	"github.com/frankbardon/vellum/capability"
	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/fragment"
	"github.com/frankbardon/vellum/numfmt"
	"github.com/frankbardon/vellum/spec"
	"github.com/frankbardon/vellum/theme"
)

// resolveSection resolves one division against its master layout.
func (r *resolver) resolveSection(index int, s *spec.Section) (fragment.Section, error) {
	layout, err := r.theme.LayoutFor(r.opts.Format, s.Layout)
	if err != nil {
		return fragment.Section{}, withLocation(err, index, s.ID)
	}
	page, err := resolvePage(layout.Page)
	if err != nil {
		return fragment.Section{}, withLocation(err, index, s.ID)
	}

	out := fragment.Section{ID: s.ID, LayoutID: layout.ID, Page: page}

	// Section marks apply to every block in the section, so they are resolved
	// once and layered under each block's own.
	sectionStyle := r.applyMarks(r.baseStyle(theme.FontBody, r.theme.Type.Body), s.Marks, index, -1)

	for i := range s.Blocks {
		b, err := r.resolveBlock(index, s.ID, i, &s.Blocks[i], layout, sectionStyle)
		if err != nil {
			return fragment.Section{}, err
		}
		out.Blocks = append(out.Blocks, b)
	}
	return out, nil
}

func (r *resolver) resolveBlock(
	sectionIndex int, sectionID string, blockIndex int,
	b *spec.Block, layout *theme.Layout, inherited fragment.TextStyle,
) (fragment.Block, error) {
	out := fragment.Block{Kind: b.Kind}

	switch b.Kind {
	case spec.BlockHeading:
		size := r.theme.Type.HeadingSize(b.Heading.Level)
		style := r.baseStyle(theme.FontHeading, size)
		style.Bold = true
		// Every heading is bold, so every heading is a place a format without
		// a bold cut degrades. Reported here as well as from a mark, because a
		// document of nothing but plain headings would otherwise carry the
		// degradation and no warning about it.
		r.degrade(capability.FeatureTextBold, nil)
		style.Color = r.color(theme.ColorHeading, style.Color)
		style = r.applyMarks(style, b.Marks, sectionIndex, blockIndex)

		before, after, err := r.spacing(r.theme.Spacing.HeadingBefore, r.theme.Spacing.HeadingAfter)
		if err != nil {
			return out, withLocation(err, sectionIndex, sectionID)
		}
		out.Paragraph = &fragment.Paragraph{
			OutlineLevel: b.Heading.Level,
			Runs:         []fragment.Run{{Text: b.Heading.Content, Style: style}},
			SpaceBefore:  before,
			SpaceAfter:   after,
			LineHeight:   r.theme.Spacing.LineHeight,
		}

	case spec.BlockText:
		style := r.applyMarks(inherited, b.Marks, sectionIndex, blockIndex)
		before, after, err := r.spacing(r.theme.Spacing.ParagraphBefore, r.theme.Spacing.ParagraphAfter)
		if err != nil {
			return out, withLocation(err, sectionIndex, sectionID)
		}
		out.Paragraph = &fragment.Paragraph{
			Runs:        []fragment.Run{{Text: b.Text.Content, Style: style}},
			SpaceBefore: before,
			SpaceAfter:  after,
			LineHeight:  r.theme.Spacing.LineHeight,
		}

	case spec.BlockAsset:
		ref, err := r.resolveAsset(b.Asset, layout)
		if err != nil {
			return out, withBlockLocation(err, sectionIndex, sectionID, blockIndex)
		}
		out.Asset = ref

	case spec.BlockTable:
		style := r.applyMarks(r.baseStyle(theme.FontBody, r.theme.Type.TableBody), b.Marks, sectionIndex, blockIndex)
		t, err := r.resolveTable(b.Table, style, sectionIndex, blockIndex)
		if err != nil {
			return out, withBlockLocation(err, sectionIndex, sectionID, blockIndex)
		}
		out.Table = t

	case spec.BlockPageBreak:
		out.Break = &fragment.Break{}

	case spec.BlockNotes:
		style := r.baseStyle(theme.FontBody, r.theme.Type.Notes)
		style.Color = r.color(theme.ColorTextMuted, style.Color)
		style = r.applyMarks(style, b.Marks, sectionIndex, blockIndex)
		out.Note = &fragment.Note{Body: fragment.Paragraph{
			Runs:       []fragment.Run{{Text: b.Notes.Content, Style: style}},
			LineHeight: r.theme.Spacing.LineHeight,
		}}

	case spec.BlockSpacer:
		emu, err := b.Spacer.Height.EMU()
		if err != nil {
			return out, withBlockLocation(err, sectionIndex, sectionID, blockIndex)
		}
		out.Space = &fragment.Space{HeightEMU: emu}

	default:
		return out, verr.NewCodedErrorWithDetails(verr.VELLUM_SPEC_BLOCK_KIND_UNKNOWN,
			"the block declares a kind that is not in the vocabulary",
			map[string]any{"kind": string(b.Kind),
				"section_index": sectionIndex, "block_index": blockIndex})
	}
	return out, nil
}

// resolveAsset ingests an asset, checks its media against the format, and
// places it in the theme's box.
func (r *resolver) resolveAsset(a *spec.Asset, layout *theme.Layout) (*fragment.AssetRef, error) {
	role := theme.BoxRole(a.Role)
	box, err := layout.BoxFor(role)
	if err != nil {
		return nil, err
	}
	if role == "" {
		role = theme.DefaultBoxRole
	}

	idx, err := r.ingestAsset(a.Handle, verr.VELLUM_ASSET_NOT_FOUND)
	if err != nil {
		return nil, err
	}
	res := &r.assets[idx]

	// The media policy. Asked after ingestion because the type is only known
	// once the bytes have been seen, and asked of the matrix rather than of a
	// list this package wrote down.
	if err := asset.CheckMedia(a.Handle, res.MediaType, r.opts.Format,
		capability.AcceptedMedia(r.opts.Format)); err != nil {
		return nil, err
	}

	// A vector accepted by a format that cannot draw one on its own is
	// accepted as a pair: the raster is what most readers show and the vector
	// rides alongside it. The consumer supplied a vector and will mostly see a
	// raster, which is a difference they have to be told about — the media
	// check above only decides whether it is carried at all.
	if res.MediaType == asset.MediaSVG {
		r.degrade(capability.FeatureAssetSVG, map[string]any{"handle": a.Handle})
	}

	width, err := box.Width.EMU()
	if err != nil {
		return nil, err
	}
	height, err := r.boxHeight(box, res, width)
	if err != nil {
		return nil, err
	}

	if a.AltText != "" {
		// The description is content the consumer wrote. A format with nowhere
		// to attach it drops it, and a drop nobody is told about is
		// indistinguishable from never having written one.
		r.degrade(capability.FeatureAssetAltText, map[string]any{"handle": a.Handle})
	}

	return &fragment.AssetRef{
		AssetIndex: idx,
		Role:       role,
		WidthEMU:   width,
		HeightEMU:  height,
		AltText:    a.AltText,
	}, nil
}

// boxHeight resolves a box's height, applying the asset's aspect ratio when the
// box declares an intrinsic one.
//
// A writer never asks an asset how tall it is. By the time a fragment reaches
// one, both dimensions are concrete — because "the height follows the asset"
// is a rule about resolution, and a writer that had to apply it would be a
// fourth place for it to be applied slightly differently.
func (r *resolver) boxHeight(box theme.Box, a *fragment.Asset, widthEMU int64) (int64, error) {
	if !box.IntrinsicHeight() {
		return box.Height.EMU()
	}
	ratio := 0.0
	if a.WidthPx > 0 && a.HeightPx > 0 {
		ratio = a.WidthPx / a.HeightPx
	}
	if ratio <= 0 {
		// An asset that declares no size in a box that declares no height. The
		// theme's own box is square in that case rather than zero-high: a
		// zero-high picture is invisible, which is the one outcome worse than
		// a wrongly-proportioned one, and it would be silent.
		r.warn(verr.NewCodedErrorWithDetails(verr.VELLUM_CAPABILITY_DEGRADED,
			"the asset declares no intrinsic size and the theme box declares no height, so it is placed square",
			map[string]any{"handle": a.Handle, "media_type": a.MediaType,
				"box_role": string(box.Role), "format": string(r.opts.Format)}))
		return widthEMU, nil
	}
	// Rounded half away from zero, in int64 throughout. A measurement that
	// round-trips through float64 accumulates error that shows as a one-EMU
	// disagreement between two runs of the same specification — a determinism
	// failure wearing a rounding bug's clothes.
	return int64(math.Round(float64(widthEMU) / ratio)), nil
}

// resolveTable renders every cell's value through its format code.
func (r *resolver) resolveTable(t *spec.Table, base fragment.TextStyle, sectionIndex, blockIndex int) (*fragment.Table, error) {
	width, err := t.ColumnHeaders.Width()
	if err != nil {
		return nil, err
	}

	// Whether a long table is split by Vellum or left to the reader is the
	// format's answer, and it is one a consumer planning a document needs
	// before the render rather than after it: a deck's table is cut where the
	// theme says and a document's is cut where Word says.
	r.degrade(capability.FeatureOverflowContinue, nil)

	headerStyle := base
	headerStyle.Bold = true
	r.degrade(capability.FeatureTextBold, nil)
	headerStyle.Color = r.color(theme.ColorTableHeaderText, headerStyle.Color)
	headerStyle.Background = r.color(theme.ColorTableHeaderBackground, "")

	out := &fragment.Table{
		Width:         width,
		ColumnHeaders: r.resolveHeaders(t.ColumnHeaders, headerStyle, sectionIndex, blockIndex),
		RowHeaders:    r.resolveHeaders(t.RowHeaders, headerStyle, sectionIndex, blockIndex),
	}
	if t.Caption != "" {
		captionStyle := r.baseStyle(theme.FontBody, r.theme.Type.Caption)
		captionStyle.Color = r.color(theme.ColorTextMuted, captionStyle.Color)
		out.Caption = &fragment.Paragraph{
			Runs:       []fragment.Run{{Text: t.Caption, Style: captionStyle}},
			LineHeight: r.theme.Spacing.LineHeight,
		}
	}

	for i := range t.Body {
		row := make([]fragment.Cell, 0, len(t.Body[i]))
		for j := range t.Body[i] {
			cell, err := r.resolveCell(&t.Body[i][j], base, sectionIndex, blockIndex, i, j)
			if err != nil {
				return nil, err
			}
			row = append(row, cell)
		}
		out.Body = append(out.Body, row)
	}
	return out, nil
}

func (r *resolver) resolveHeaders(tree spec.HeaderTree, style fragment.TextStyle, sectionIndex, blockIndex int) fragment.HeaderTree {
	if len(tree) == 0 {
		return nil
	}
	out := make(fragment.HeaderTree, 0, len(tree))
	for i := range tree {
		n := &tree[i]
		out = append(out, fragment.HeaderNode{
			Label:    n.Label,
			Span:     n.Span,
			Children: r.resolveHeaders(n.Children, style, sectionIndex, blockIndex),
			Style:    r.applyMarks(style, n.Marks, sectionIndex, blockIndex),
		})
	}
	return out
}

func (r *resolver) resolveCell(c *spec.Cell, base fragment.TextStyle, sectionIndex, blockIndex, row, col int) (fragment.Cell, error) {
	out := fragment.Cell{
		FormatCode: c.Format,
		RowSpan:    max(c.RowSpan, 1),
		ColSpan:    max(c.ColSpan, 1),
		Class:      c.Class,
		Style:      r.applyMarks(base, c.Marks, sectionIndex, blockIndex),
	}

	format, err := numfmt.Parse(c.Format)
	if err != nil {
		return out, verr.WrapCodedErrorWithDetails(err, verr.VELLUM_TABLE_FORMAT_INVALID,
			"a cell's number-format code does not parse",
			map[string]any{"section_index": sectionIndex, "block_index": blockIndex,
				"row": row, "column": col, "format": c.Format})
	}

	if c.Value != nil {
		v, err := convertValue(c.Value, sectionIndex, blockIndex, row, col)
		if err != nil {
			return out, err
		}
		out.Value = &v
		out.Text = format.Apply(v)
	}

	// A consumer's own text wins over the formatted value. They have already
	// decided how the number should read — a locale rendering, a suppressed
	// low base — and Vellum's job is to carry that decision, not to second-guess
	// it. The typed value stays alongside, so a spreadsheet target can still
	// write a live cell.
	if c.Text != "" {
		out.Text = c.Text
	}

	for i := range c.Annotations {
		a := &c.Annotations[i]
		position := a.Position
		if position == "" {
			position = spec.AnnotationSuperscript
		}
		out.Annotations = append(out.Annotations, fragment.Annotation{
			Text:     a.Text,
			Position: position,
			Style:    r.applyMarks(out.Style, a.Marks, sectionIndex, blockIndex),
		})
	}
	return out, nil
}

// convertValue turns a specification value into a formatting value, parsing the
// date once here rather than in each writer.
func convertValue(v *spec.Value, sectionIndex, blockIndex, row, col int) (numfmt.Value, error) {
	switch v.Kind {
	case spec.ValueNumber:
		return numfmt.Value{Kind: numfmt.KindNumber, Number: v.Number}, nil
	case spec.ValueText:
		return numfmt.Value{Kind: numfmt.KindText, Text: v.Text}, nil
	case spec.ValueBool:
		return numfmt.Value{Kind: numfmt.KindBool, Bool: v.Bool}, nil
	case spec.ValueDate:
		t, err := time.Parse(time.RFC3339, v.Date)
		if err != nil {
			// A date-only form is the common case and is accepted, because
			// refusing it would make every consumer append a meaningless
			// midnight-UTC suffix.
			t, err = time.Parse("2006-01-02", v.Date)
			if err != nil {
				return numfmt.Value{}, verr.WrapCodedErrorWithDetails(err, verr.VELLUM_TABLE_INVALID,
					"a cell's date value is not RFC 3339 or a bare date",
					map[string]any{"section_index": sectionIndex, "block_index": blockIndex,
						"row": row, "column": col, "date": v.Date})
			}
		}
		return numfmt.Value{Kind: numfmt.KindDate, Time: t.UTC()}, nil
	default:
		return numfmt.Value{Kind: numfmt.KindEmpty}, nil
	}
}

// baseStyle is the theme's default appearance for a font role and size.
func (r *resolver) baseStyle(role theme.FontRole, size spec.Length) fragment.TextStyle {
	emu, err := size.EMU()
	if err != nil {
		// Unreachable: the theme's own validation refuses a size that is not a
		// valid length. Falling back to the body size keeps a future change
		// from producing a zero-height run, which renders as nothing.
		emu, _ = r.theme.Type.Body.EMU()
	}
	return fragment.TextStyle{
		FaceIndex: r.faceIndex(role),
		SizeEMU:   emu,
		Color:     r.color(theme.ColorText, ""),
	}
}

// faceIndex finds a role in the manifest, falling back to body.
func (r *resolver) faceIndex(role theme.FontRole) int {
	for i := range r.faces {
		if r.faces[i].Role == role {
			return i
		}
	}
	for i := range r.faces {
		if r.faces[i].Role == theme.FontBody {
			return i
		}
	}
	return 0
}

func (r *resolver) color(role theme.ColorRole, fallback string) string {
	if v, ok := r.theme.LookupColor(role); ok {
		return v
	}
	return fallback
}

// applyMarks layers a theme's mark styles onto a base style.
//
// Marks apply in the order the specification lists them, so a later mark wins a
// conflict — the same rule as a stylesheet, and the one an author will assume.
// A mark the theme does not style is a warning rather than an error, once per
// distinct name however many times it is used: marks are the consumer's
// vocabulary and a theme need not style every one, but an invisible flag is
// indistinguishable from no flag.
func (r *resolver) applyMarks(base fragment.TextStyle, marks []string, sectionIndex, blockIndex int) fragment.TextStyle {
	out := base
	for _, name := range marks {
		style, ok := r.theme.LookupMark(name)
		if !ok {
			if !r.marks[name] {
				r.marks[name] = true
				r.warn(verr.NewCodedErrorWithDetails(verr.VELLUM_MARK_UNKNOWN,
					"the theme declares no style for this mark, so the marked content renders unstyled",
					map[string]any{"mark": name, "theme_id": r.theme.ID,
						"section_index": sectionIndex, "block_index": blockIndex,
						"styled_marks": r.theme.MarkNames()}))
			}
			continue
		}
		if style.Bold {
			out.Bold = true
			// Asked at the point the content appears rather than at the end,
			// so the warning can say which mark and where. A format that
			// renders bold produces nothing here.
			r.degrade(capability.FeatureTextBold, map[string]any{
				"mark": name, "section_index": sectionIndex, "block_index": blockIndex})
		}
		if style.Italic {
			out.Italic = true
			r.degrade(capability.FeatureTextItalic, map[string]any{
				"mark": name, "section_index": sectionIndex, "block_index": blockIndex})
		}
		if style.Underline {
			out.Underline = true
		}
		if style.Color != "" {
			out.Color = r.color(style.Color, out.Color)
		}
		if style.Background != "" {
			out.Background = r.color(style.Background, out.Background)
		}
	}
	return out
}

func (r *resolver) spacing(before, after spec.Length) (int64, int64, error) {
	b, err := before.EMU()
	if err != nil {
		return 0, 0, err
	}
	a, err := after.EMU()
	if err != nil {
		return 0, 0, err
	}
	return b, a, nil
}

func resolvePage(p theme.Page) (fragment.Page, error) {
	var out fragment.Page
	dims := []struct {
		into *int64
		from spec.Length
	}{
		{&out.Width, p.Width}, {&out.Height, p.Height},
		{&out.MarginTop, p.MarginTop}, {&out.MarginRight, p.MarginRight},
		{&out.MarginBottom, p.MarginBottom}, {&out.MarginLeft, p.MarginLeft},
	}
	for _, d := range dims {
		emu, err := d.from.EMU()
		if err != nil {
			return out, err
		}
		*d.into = emu
	}
	return out, nil
}

// withLocation and withBlockLocation add position to an error whose own layer
// did not know it — a length parser knows a length was bad, not which block it
// was in. Annotating rather than wrapping keeps the code and the message the
// inner layer chose, which are the better account of what went wrong.
func withLocation(err error, sectionIndex int, sectionID string) error {
	return verr.Annotate(err, map[string]any{
		"section_index": sectionIndex, "section_id": sectionID})
}

func withBlockLocation(err error, sectionIndex int, sectionID string, blockIndex int) error {
	return verr.Annotate(err, map[string]any{
		"section_index": sectionIndex, "section_id": sectionID, "block_index": blockIndex})
}
