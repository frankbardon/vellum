// Package resolve turns a specification into the resolved intermediate.
//
// This is where theme application, font selection, number formatting, asset
// resolution and mark resolution happen — once, for all four output formats and
// for fill mode's splicer. Doing it here rather than in each writer is the whole
// point: four implementations of the same work are four chances to disagree, and
// the disagreement would show as the same specification rendering differently in
// a document and in the deck built beside it.
//
// It is also where every warning is raised. A degradation, a font substitution
// and an unstyled mark are all things a consumer must be told about, and telling
// them once from one place is what makes the envelope's warnings complete rather
// than whatever each writer remembered to mention.
package resolve

import (
	"context"
	"sort"

	"github.com/frankbardon/vellum/artifact"
	"github.com/frankbardon/vellum/asset"
	"github.com/frankbardon/vellum/capability"
	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/fragment"
	"github.com/frankbardon/vellum/spec"
	"github.com/frankbardon/vellum/theme"
)

// Options configures a resolve.
//
// Every seam has an inert default, so the zero value plus a format is a working
// configuration: the built-in theme, inline assets, and the built-in bounds.
type Options struct {
	// Format is the target this specification is being resolved for.
	//
	// Required, because resolution is format-dependent in ways that are not
	// cosmetic: the layout comes from a per-format master, the accepted asset
	// media differ, and whether a font can be embedded at all is a property of
	// the container.
	Format artifact.Format

	// Themes resolves the theme id. Nil serves the built-in theme.
	Themes theme.Provider

	// Assets resolves asset handles. Nil serves inline data URIs only.
	Assets asset.Resolver

	// AssetOptions bounds asset ingestion. The zero value selects the built-in
	// bound.
	AssetOptions asset.Options
}

// Result is a resolved document and everything the consumer must be told.
type Result struct {
	// Doc is the resolved intermediate.
	Doc *fragment.Doc

	// Warnings are the things that happened which a consumer needs to know
	// about but which did not stop the render: a degraded feature, a
	// substituted font, a mark the theme does not style.
	//
	// Ordered deterministically, because they reach the envelope and the
	// envelope is compared byte for byte.
	Warnings []*verr.CodedError
}

// Resolve produces the resolved intermediate for a specification.
//
// Every failure is raised before any bytes exist, which is the ordering the
// whole library is arranged around: a consumer scheduling an unattended job
// learns about a gap from an error rather than from a reader noticing an
// absence.
func Resolve(ctx context.Context, s *spec.Spec, opts Options) (*Result, error) {
	if s == nil {
		return nil, verr.NewCodedError(verr.VELLUM_SPEC_INVALID, "specification is nil")
	}
	if !artifact.ValidFormat(opts.Format) {
		return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_SPEC_INVALID,
			"resolve requires a target format",
			map[string]any{"format": string(opts.Format)})
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}

	// The capability check runs before any resolution work, so a rejected
	// specification costs nothing beyond the check itself — no theme lookup, no
	// asset fetch.
	check, err := capability.Check(s, opts.Format)
	if err != nil {
		return nil, err
	}
	if err := check.Err(); err != nil {
		return nil, err
	}

	r := &resolver{
		ctx:    ctx,
		spec:   s,
		opts:   opts,
		byHash: make(map[string]int),
		marks:  make(map[string]bool),
	}
	for _, d := range check.Degradations {
		r.warn(verr.NewCodedErrorWithDetails(verr.VELLUM_CAPABILITY_DEGRADED,
			"a feature becomes the stated alternative in this format",
			map[string]any{
				"feature":       string(d.Feature),
				"format":        string(opts.Format),
				"section_index": d.SectionIndex,
				"block_index":   d.BlockIndex,
			}))
	}

	th, err := theme.Resolve(ctx, opts.Themes, s.Theme)
	if err != nil {
		return nil, err
	}
	r.theme = th

	if err := r.resolveFonts(); err != nil {
		return nil, err
	}

	doc := &fragment.Doc{Title: s.Title, ThemeID: th.ID, Fonts: r.faces}
	for i := range s.Sections {
		sec, err := r.resolveSection(i, &s.Sections[i])
		if err != nil {
			return nil, err
		}
		doc.Sections = append(doc.Sections, sec)
	}
	doc.Assets = r.assets

	r.sortWarnings()
	return &Result{Doc: doc, Warnings: r.warnings}, nil
}

// resolver carries the state of one resolve.
type resolver struct {
	ctx   context.Context
	spec  *spec.Spec
	opts  Options
	theme *theme.Theme

	faces  []fragment.Face
	assets []fragment.Asset

	// byHash deduplicates assets by content. Two blocks naming different
	// handles that resolve to the same bytes are one asset in the package, which
	// is what makes a document carrying the same logo six times carry it once.
	byHash map[string]int

	// marks records which unknown marks have already been warned about, so a
	// mark applied to two hundred cells produces one warning rather than two
	// hundred.
	marks map[string]bool

	warnings []*verr.CodedError

	// degraded records which features have already been reported as degrading
	// for this format, so a document with four hundred emphasised runs produces
	// one warning rather than four hundred. The same reason marks are deduped.
	degraded map[capability.Feature]bool
}

func (r *resolver) warn(w *verr.CodedError) { r.warnings = append(r.warnings, w) }

// degrade reports a feature the target format cannot carry, once per document.
//
// The matrix is asked rather than a condition written here, so a row that
// changes from degrades to renders stops the warning without a second edit. A
// feature the format renders produces nothing, which is what makes this safe to
// call unconditionally at the site where the content appears.
func (r *resolver) degrade(f capability.Feature, where map[string]any) {
	e, ok := capability.Lookup(f, r.opts.Format)
	if !ok || e.Outcome != capability.Degrades {
		return
	}
	if r.degraded == nil {
		r.degraded = map[capability.Feature]bool{}
	}
	if r.degraded[f] {
		return
	}
	r.degraded[f] = true

	code := e.Code
	if code == "" {
		code = verr.VELLUM_CAPABILITY_DEGRADED
	}
	details := map[string]any{
		"feature": string(f),
		"format":  string(r.opts.Format),
		"becomes": e.Degrade,
	}
	for k, v := range where {
		details[k] = v
	}
	r.warn(verr.NewCodedErrorWithDetails(code,
		"the target format cannot carry this and it was degraded", details))
}

// sortWarnings puts the warnings in a stable order.
//
// They reach the envelope, and the envelope is compared byte for byte. Emission
// order is already deterministic — the walk is ordered — but sorting by code
// groups them usefully for a reader and makes the order independent of where in
// the walk a warning happened to arise.
func (r *resolver) sortWarnings() {
	sort.SliceStable(r.warnings, func(i, j int) bool {
		return r.warnings[i].Code < r.warnings[j].Code
	})
}
