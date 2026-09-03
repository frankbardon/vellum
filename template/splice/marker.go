package splice

import (
	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/fragment"
	"github.com/frankbardon/vellum/template/anchor"
	"github.com/frankbardon/vellum/template/defrag"
	"github.com/frankbardon/vellum/xmlcopy"
)

// spliceMarker replaces a {{marker}}'s exact byte span — resolved through
// template/defrag's flatten-and-locate algorithm — with the rendered runs of
// exactly one Paragraph block.
//
// A marker sits mid-run, inline inside an existing paragraph, so only inline
// (run-level) content can go there: this is the permanent scope boundary
// documented on [verr.VELLUM_TEMPLATE_MARKER_BLOCK_UNSUPPORTED] and in the
// package doc, not a "not implemented yet" gap.
func spliceMarker(a anchor.Anchor, src []byte, seq fragment.Sequence) (xmlcopy.Replacement, error) {
	if len(seq.Blocks) != 1 || seq.Blocks[0].Paragraph == nil {
		return xmlcopy.Replacement{}, markerBlockErr(a, seq)
	}
	para := seq.Blocks[0].Paragraph

	flat, err := defrag.Flatten(src, a.Span)
	if err != nil {
		return xmlcopy.Replacement{}, err
	}

	literal := "{{" + a.Name + "}}"
	matches := flat.FindAll(literal)
	if len(matches) != 1 {
		// anchor.Discover already rejects two anchors sharing a name
		// (VELLUM_ANCHOR_DUPLICATE) and this package is handed the same
		// unmodified part bytes Discover walked — E10's orchestration
		// collects every anchor's Replacement against the pristine source
		// and applies them all in one xmlcopy.Apply pass, rather than
		// splicing one and re-walking the result for the next — so finding
		// anything other than exactly one occurrence here is a caller bug,
		// not something an untrusted template's own bytes can produce.
		return xmlcopy.Replacement{}, verr.NewCodedErrorWithDetails(verr.VELLUM_INTERNAL_INVARIANT,
			"the marker anchor's own literal text was not found exactly once by re-flattening its paragraph",
			map[string]any{"anchor": a.Name, "part": a.Part, "match_count": len(matches)})
	}
	m := matches[0]

	site, err := flat.Locate(m.Start, m.End)
	if err != nil {
		return xmlcopy.Replacement{}, err
	}

	// site.RunRPr — the first touched run's own rPr, populated unconditionally
	// by template/defrag regardless of whether Prefix or Suffix survived — is
	// the formatting basis for every new run, closing the gap the story
	// exists to close: when the match consumes one or more runs *entirely*
	// (Prefix and Suffix both nil, the common case for a marker that is the
	// whole of its own run), there is otherwise nothing to draw formatting
	// from at all.
	basis := site.RunRPr

	var data []byte
	if site.Prefix != nil {
		data = append(data, defrag.RenderRun(site.Prefix)...)
	}
	for i := range para.Runs {
		r := &para.Runs[i]
		data = append(data, renderRun(r.Text, basis, r.Style)...)
	}
	if site.Suffix != nil {
		data = append(data, defrag.RenderRun(site.Suffix)...)
	}

	return site.Affected.Replace(data), nil
}

func markerBlockErr(a anchor.Anchor, seq fragment.Sequence) error {
	return verr.NewCodedErrorWithDetails(verr.VELLUM_TEMPLATE_MARKER_BLOCK_UNSUPPORTED,
		"a {{marker}} anchor accepts exactly one heading-or-text block, and nothing else",
		map[string]any{
			"anchor":      a.Name,
			"part":        a.Part,
			"block_count": len(seq.Blocks),
		})
}
