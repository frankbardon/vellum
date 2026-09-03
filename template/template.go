// Package template opens an OOXML package as a fill-mode template.
//
// It is the seam between opc, which reads the package's bytes without
// understanding what they mean, and template/anchor, which walks a
// recognised part looking for the places a binding will later splice data
// into. Opening a template never mutates the package: [Open] returns a
// read-only view over an [opc.Package], and everything downstream of it —
// anchor discovery today, defragmentation, binding and splicing in later
// stories — edits a clone rather than the package Open returned.
//
// template, template/anchor, template/defrag and template/splice are
// firewalled from importing encoding/xml directly by TestNoEncodingXMLInFill:
// every read of a source part's structure goes through xmlcopy.Walk, and
// every edit goes through xmlcopy.Apply, so that fill mode's
// non-destructiveness guarantee — an untouched part survives byte-for-byte —
// is never undermined by a decode-mutate-re-encode shortcut.
package template

import (
	"io"
	"strings"

	verr "github.com/frankbardon/vellum/errors"

	"github.com/frankbardon/vellum/artifact"
	"github.com/frankbardon/vellum/opc"
	"github.com/frankbardon/vellum/template/anchor"
)

// relOfficeDocument is the package-relationship type that names the main
// part of an OOXML document, workbook or presentation. It is the OPC way to
// find that part — reading the relationship graph rather than guessing a
// hardcoded name like "word/document.xml", which a template assembled by a
// tool with looser habits need not use.
const relOfficeDocument = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument"

// Main part content types, one per recognised template format. These are the
// exact content types Vellum's own OOXML writers declare for the part an
// officeDocument relationship targets (see doc/xml.go, sheet/xml.go,
// deck/xml.go); a template opened here is expected to carry the same
// declaration, whether Vellum wrote it or Word, Excel or PowerPoint did.
const (
	contentTypeMainDocument = "application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"
	contentTypeWorkbook     = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"
	contentTypePresentation = "application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"
)

// Template is an opened OOXML package identified as a fill-mode target: its
// format, established from the package's own relationship graph rather than
// assumed, and the name of its main part.
type Template struct {
	pkg      *opc.Package
	format   artifact.Format
	mainPart string
}

// Package returns the underlying OPC package. Fill mode's later stories
// (defrag, bind, splice) read source parts and build replacement packages
// through it; Open itself never mutates it.
func (t *Template) Package() *opc.Package {
	if t == nil {
		return nil
	}
	return t.pkg
}

// Format returns the template's detected format.
func (t *Template) Format() artifact.Format {
	if t == nil {
		return ""
	}
	return t.format
}

// MainPart returns the part name the package's officeDocument relationship
// resolves to — conventionally "/word/document.xml", "/xl/workbook.xml" or
// "/ppt/presentation.xml", but read from the relationship rather than
// assumed.
func (t *Template) MainPart() string {
	if t == nil {
		return ""
	}
	return t.mainPart
}

// OpenOption configures [Open].
type OpenOption func(*openConfig)

type openConfig struct {
	maxPartBytes  int64
	maxTotalBytes int64
}

// WithMaxPartBytes bounds a single part's uncompressed size, mirroring
// [opc.WithMaxPartBytes]. A caller opening an untrusted template needs the
// same decompression-bomb protection opc.Open already offers an opc caller
// directly, without reaching past this package to get it.
func WithMaxPartBytes(n int64) OpenOption {
	return func(c *openConfig) { c.maxPartBytes = n }
}

// WithMaxTotalBytes bounds the sum of all parts' uncompressed sizes,
// mirroring [opc.WithMaxTotalBytes].
func WithMaxTotalBytes(n int64) OpenOption {
	return func(c *openConfig) { c.maxTotalBytes = n }
}

// Open reads an OPC package and identifies it as a fill-mode template.
//
// The format is detected from the package-root relationships: the target of
// the officeDocument relationship, and that target's own declared content
// type, name DOCX, XLSX or PPTX. A package with no such relationship, one
// whose target the package does not contain, or one whose target's content
// type is not one of the three recognised main-part types — including PDF,
// which is not an OPC package to begin with and so never carries one — is
// rejected with [verr.VELLUM_TEMPLATE_INVALID]. Fill mode edits an OPC
// package surgically; a format outside that set is not one.
func Open(r io.ReaderAt, size int64, opts ...OpenOption) (*Template, error) {
	var cfg openConfig
	for _, o := range opts {
		o(&cfg)
	}

	var opcOpts []opc.OpenOption
	if cfg.maxPartBytes > 0 {
		opcOpts = append(opcOpts, opc.WithMaxPartBytes(cfg.maxPartBytes))
	}
	if cfg.maxTotalBytes > 0 {
		opcOpts = append(opcOpts, opc.WithMaxTotalBytes(cfg.maxTotalBytes))
	}

	pkg, err := opc.Open(r, size, opcOpts...)
	if err != nil {
		return nil, err
	}

	format, mainPart, err := detectFormat(pkg)
	if err != nil {
		return nil, err
	}

	return &Template{pkg: pkg, format: format, mainPart: mainPart}, nil
}

// detectFormat resolves the package's format and main part from its own
// relationship graph, rather than from a hardcoded part-name guess.
func detectFormat(pkg *opc.Package) (artifact.Format, string, error) {
	rels, ok := pkg.RelationshipsFor("/")
	if !ok {
		return "", "", invalidTemplate("the package declares no root relationships", nil)
	}

	matches := rels.ByType(relOfficeDocument)
	if len(matches) == 0 {
		return "", "", invalidTemplate(
			"the package declares no officeDocument relationship", nil)
	}
	// A well-formed package carries exactly one; ByType returns them in
	// serialised order, which for a parsed set is the document order the
	// package was written in, so taking the first is deterministic even for
	// a package that (incorrectly) declares more than one.
	rel := matches[0]

	target := resolveRootTarget(rel.Target)
	part, ok := pkg.Get(target)
	if !ok {
		return "", "", invalidTemplate(
			"the officeDocument relationship targets a part the package does not contain",
			map[string]any{"target": target})
	}

	switch part.ContentType {
	case contentTypeMainDocument:
		return artifact.FormatDOCX, target, nil
	case contentTypeWorkbook:
		return artifact.FormatXLSX, target, nil
	case contentTypePresentation:
		return artifact.FormatPPTX, target, nil
	default:
		return "", "", invalidTemplate(
			"the officeDocument relationship's target has an unrecognized content type",
			map[string]any{"target": target, "content_type": part.ContentType})
	}
}

// resolveRootTarget resolves a relationship target declared by the package
// root against the package root — the one case template.Open needs, since
// the officeDocument relationship is always root-owned. An absolute target
// (a leading slash) is used as-is; a relative one has "/" prefixed, and any
// "./" or "../" segment is normalised away exactly as [opc.Package.Validate]
// would for a root-owned relationship.
func resolveRootTarget(target string) string {
	if strings.HasPrefix(target, "/") {
		return target
	}
	var segments []string
	for _, seg := range strings.Split(target, "/") {
		switch seg {
		case "", ".":
			// doubled slash or current-directory segment: nothing to do.
		case "..":
			if len(segments) > 0 {
				segments = segments[:len(segments)-1]
			}
		default:
			segments = append(segments, seg)
		}
	}
	return "/" + strings.Join(segments, "/")
}

func invalidTemplate(message string, details map[string]any) error {
	if details == nil {
		return verr.NewCodedError(verr.VELLUM_TEMPLATE_INVALID, message)
	}
	return verr.NewCodedErrorWithDetails(verr.VELLUM_TEMPLATE_INVALID, message, details)
}

// Inspect discovers every fillable anchor in t and returns them as an
// [anchor.Inventory]. It is FR-F7's entry point: the same call a human-facing
// CLI table and an agent-facing JSON envelope both build on (that formatting
// is CLI work, epic E12, not here).
//
// Font-requirement reporting, also part of FR-F7, is a deliberately deferred
// gap: no story on the board calls it out yet and no anchor kind built so far
// needs it, so it is not stubbed.
func Inspect(t *Template) (*anchor.Inventory, error) {
	if t == nil {
		return nil, verr.NewCodedError(verr.VELLUM_INTERNAL_INVARIANT, "nil template")
	}
	return anchor.Discover(t.pkg, t.format, t.mainPart)
}
