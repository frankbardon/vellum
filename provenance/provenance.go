// Package provenance records what produced an artifact.
//
// Every Vellum artifact carries a machine-readable account of its own origin,
// so the question "what produced this number, and can you prove this file has
// not changed?" has an answer. For regulated work that question is the whole
// reason a document pipeline is auditable rather than merely repeatable.
//
// # Status
//
// The record and its hashing land here. Embedding it into OOXML document
// properties and into PDF XMP metadata arrives with the format epics, because
// each format carries it differently and there is nothing useful to share
// between them beyond this type.
package provenance

import (
	"time"

	"github.com/frankbardon/vellum/canon"
)

// hashTag namespaces a provenance hash. See [canon.CanonicalHash].
const hashTag = "provenance"

// Record describes what produced an artifact.
//
// Everything here is an input or an identity. Deliberately absent: anything
// read from the machine that happened to run the render — a hostname, a user,
// a working directory. Those would make the record vary between two runs that
// produced identical bytes, which is the opposite of what it is for.
type Record struct {
	// VellumVersion is the library version that produced the artifact.
	VellumVersion string `json:"vellum_version"`

	// GeneratedAt is the wall-clock time of the render.
	//
	// Nil in deterministic mode, which is the default. A real timestamp makes
	// two identical renders produce different bytes, so it is an explicit
	// opt-out rather than something that happens by accident.
	GeneratedAt *time.Time `json:"generated_at,omitempty"`

	// SourceDateEpoch is the pinned timestamp every date in the artifact was
	// stamped from.
	SourceDateEpoch time.Time `json:"source_date_epoch"`

	// SpecHash identifies the specification. Together with the asset hashes it
	// names the artifact, and both are inputs — which is what makes the name
	// knowable before the render runs.
	SpecHash string `json:"spec_hash,omitempty"`

	// ThemeHash identifies the theme document.
	ThemeHash string `json:"theme_hash,omitempty"`

	// BindingHash and TemplateHash identify a fill-mode render.
	BindingHash  string `json:"binding_hash,omitempty"`
	TemplateHash string `json:"template_hash,omitempty"`

	// Assets are the resolved assets, sorted by handle.
	Assets []AssetRef `json:"assets,omitempty"`

	// Fonts is the font manifest — every face the artifact used, and whether
	// it was embedded or substituted. A substitution that is not recorded here
	// is a substitution nobody can audit later.
	Fonts []FontRef `json:"fonts,omitempty"`

	// Sources are caller-supplied identifiers for whatever produced the
	// content: an upstream envelope, a session, a job. Vellum does not
	// interpret them; it carries them so a reader can trace the artifact back
	// past Vellum's own boundary.
	Sources []Source `json:"sources,omitempty"`
}

// AssetRef records one resolved asset.
type AssetRef struct {
	Handle string `json:"handle"`
	Media  string `json:"media"`
	Hash   string `json:"hash"`
}

// FontRef records one font the artifact used.
type FontRef struct {
	// Family is the requested family.
	Family string `json:"family"`

	// Embedded reports whether the face was embedded in the artifact.
	Embedded bool `json:"embedded"`

	// SubstitutedWith names the face actually used, when the theme declared
	// the requested one non-embeddable. Empty when no substitution occurred.
	SubstitutedWith string `json:"substituted_with,omitempty"`

	// SubsetProfile records how the face was subsetted, because the same
	// document embedded under two profiles is two different byte streams and a
	// reader comparing them deserves to know which they have.
	SubsetProfile string `json:"subset_profile,omitempty"`
}

// Source is a caller-supplied upstream identifier.
type Source struct {
	// Kind names the sort of thing being identified, in the caller's own
	// vocabulary.
	Kind string `json:"kind"`

	// ID identifies it.
	ID string `json:"id"`
}

// Hash returns the record's canonical content hash.
func (r *Record) Hash() string {
	if r == nil {
		return canon.CanonicalHash(hashTag, (*Record)(nil))
	}
	return canon.CanonicalHash(hashTag, r)
}

// Deterministic reports whether the record describes a reproducible render.
//
// A record with a wall-clock generation time does not: two runs of the same
// specification produced different bytes, and any consumer comparing digests
// needs to know that before concluding the document changed.
func (r *Record) Deterministic() bool {
	return r != nil && r.GeneratedAt == nil
}
