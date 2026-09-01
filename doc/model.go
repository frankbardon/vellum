package doc

import (
	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/spec"
)

// Document is the WordprocessingML model.
//
// It is public because a consumer may need format-specific reach the block
// spec does not express. Today it is deliberately small; it grows with the
// DOCX epic rather than being sketched ahead of a use.
type Document struct {
	// Title is carried into the package's core properties.
	Title string

	// Body is the ordered paragraph content.
	Body []Paragraph

	// Page describes the section properties — size and margins.
	Page PageSetup
}

// Paragraph is one block-level unit.
type Paragraph struct {
	// OutlineLevel is the heading depth, 1 being the most prominent. Zero
	// means body text.
	OutlineLevel int

	// Runs are the paragraph's inline content, in order.
	Runs []Run
}

// Run is a span of text sharing one set of properties.
type Run struct {
	// Text is the run's content.
	Text string

	// Bold and SizeHalfPoints are direct formatting.
	//
	// Direct formatting is a placeholder, not the intended design: house
	// convention is that a run carries a named style and the styles part
	// defines it. The styles part arrives with the DOCX epic, and until it
	// does, referencing a style that nothing defines would leave a dangling
	// reference in every document. Direct formatting is the honest
	// intermediate.
	Bold           bool
	SizeHalfPoints int
}

// PageSetup describes a section's page geometry, in twentieths of a point —
// the unit WordprocessingML uses natively.
type PageSetup struct {
	WidthTwips, HeightTwips                int
	MarginTop, MarginRight, MarginBottom   int
	MarginLeft, MarginHeader, MarginFooter int
}

// A4Portrait is the default page geometry: A4 with one-inch margins.
func A4Portrait() PageSetup {
	return PageSetup{
		WidthTwips:   11906,
		HeightTwips:  16838,
		MarginTop:    1440,
		MarginRight:  1440,
		MarginBottom: 1440,
		MarginLeft:   1440,
		MarginHeader: 708,
		MarginFooter: 708,
	}
}

// headingSizes maps an outline level to a run size in half-points. A fixed
// table rather than a computation, so the values are inspectable and a change
// to one level cannot quietly move the others.
var headingSizes = [...]int{0, 32, 26, 24, 22, 20, 20}

// Lower converts a specification into the WordprocessingML model.
//
// A block kind this writer does not render yet is a hard error naming the
// kind, its section and its index. Silently dropping it would make a missing
// section something a reader discovers rather than something the caller is
// told, and "never silently drop content" is the product principle this
// enforces first.
func Lower(s *spec.Spec) (*Document, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}

	d := &Document{Title: s.Title, Page: A4Portrait()}

	for si := range s.Sections {
		sec := &s.Sections[si]
		for bi := range sec.Blocks {
			b := &sec.Blocks[bi]
			switch b.Kind {
			case spec.BlockHeading:
				d.Body = append(d.Body, headingParagraph(b.Heading))
			case spec.BlockText:
				d.Body = append(d.Body, Paragraph{Runs: []Run{{Text: b.Text.Content}}})
			default:
				return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_DOC_BLOCK_UNSUPPORTED,
					"the DOCX writer does not render this block kind yet",
					map[string]any{
						"kind":          string(b.Kind),
						"section_index": si,
						"block_index":   bi,
						"section_id":    sec.ID,
					})
			}
		}
	}
	return d, nil
}

func headingParagraph(h *spec.Heading) Paragraph {
	level := h.Level
	if level >= len(headingSizes) {
		level = len(headingSizes) - 1
	}
	return Paragraph{
		OutlineLevel: h.Level,
		Runs: []Run{{
			Text:           h.Content,
			Bold:           true,
			SizeHalfPoints: headingSizes[level],
		}},
	}
}
