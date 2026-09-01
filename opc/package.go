package opc

import (
	"archive/zip"
	"io"
	"sort"
	"strings"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/opc/zipdet"
)

// Package is an OPC container: a set of parts, their relationships, and the
// content-type declarations that describe them.
type Package struct {
	// parts indexes parts by name. It is used for lookup only; the write path
	// never iterates it, because map iteration order is the classic source of
	// nondeterminism in a writer like this.
	parts map[string]*Part

	// order is the authoritative write order.
	//
	// For a package read by Open it is the archive order, preserved so that a
	// round trip reproduces the input bytes. For a package built by New it is
	// empty, and the canonical order is computed at write time from the part
	// names.
	order []string

	// rels holds the parsed or built relationship set for each owner, keyed by
	// owner part name with "/" for the package root. Lookup only; never
	// iterated for ordering.
	rels map[string]*Relationships

	ct *ContentTypes
}

// New returns an empty package with the relationships content type declared,
// which every OOXML package needs and none can be opened without.
func New() *Package {
	p := &Package{
		parts: make(map[string]*Part),
		rels:  make(map[string]*Relationships),
		ct:    &ContentTypes{},
	}
	p.ct.SetDefault("rels", relsContentType)
	return p
}

// Open reads an OPC package.
//
// Parts are held as the bytes they were read as and are never re-serialised,
// so [Package.WriteTo] on an unmutated package reproduces the input exactly.
// That identity is fill mode's entire non-destructiveness guarantee, and it is
// established here rather than asserted later.
func Open(r io.ReaderAt, size int64, opts ...OpenOption) (*Package, error) {
	var cfg openConfig
	for _, o := range opts {
		o(&cfg)
	}

	archive, err := zipdet.Read(r, size, zipdet.ReadOptions{
		MaxEntryBytes: cfg.maxPartBytes,
		MaxTotalBytes: cfg.maxTotalBytes,
	})
	if err != nil {
		return nil, err
	}

	p := &Package{
		parts: make(map[string]*Part, archive.Len()),
		order: make([]string, 0, archive.Len()),
		rels:  make(map[string]*Relationships),
	}

	for _, e := range archive.Entries() {
		if e.Name == ContentTypesName {
			ct, err := parseContentTypes(e.Data)
			if err != nil {
				return nil, err
			}
			p.ct = ct
			p.order = append(p.order, ContentTypesName)
			continue
		}

		name := partName(e.Name)
		if err := ValidatePartName(name); err != nil {
			return nil, err
		}
		if _, dup := p.parts[name]; dup {
			return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_OPC_PART_DUPLICATE,
				"two parts claim the same name", map[string]any{"part_name": name})
		}

		part := &Part{
			Name:      name,
			Data:      e.Data,
			method:    e.Method,
			hasMethod: true,
		}
		p.parts[name] = part
		p.order = append(p.order, name)

		if owner, ok := ownerOfRels(name); ok {
			rels, err := parseRels(name, e.Data)
			if err != nil {
				return nil, err
			}
			p.rels[owner] = rels
		}
	}

	if p.ct == nil {
		return nil, verr.NewCodedError(verr.VELLUM_OPC_INVALID,
			"the package has no [Content_Types].xml")
	}

	// Resolve declared content types onto the parts, so a caller reading a
	// part does not have to consult the declaration itself.
	for _, name := range p.order {
		part, ok := p.parts[name]
		if !ok {
			continue
		}
		if ct, ok := p.ct.ContentTypeFor(name); ok {
			part.ContentType = ct
		}
	}

	return p, nil
}

// OpenOption configures [Open].
type OpenOption func(*openConfig)

type openConfig struct {
	maxPartBytes  int64
	maxTotalBytes int64
}

// WithMaxPartBytes bounds a single part's uncompressed size. The bound exists
// so a decompression bomb in an untrusted template is a coded error rather
// than an out-of-memory kill.
func WithMaxPartBytes(n int64) OpenOption {
	return func(c *openConfig) { c.maxPartBytes = n }
}

// WithMaxTotalBytes bounds the sum of all parts' uncompressed sizes.
func WithMaxTotalBytes(n int64) OpenOption {
	return func(c *openConfig) { c.maxTotalBytes = n }
}

// Get returns the named part.
func (p *Package) Get(name string) (*Part, bool) {
	if p == nil {
		return nil, false
	}
	part, ok := p.parts[name]
	return part, ok
}

// Has reports whether the package contains the named part.
func (p *Package) Has(name string) bool {
	_, ok := p.Get(name)
	return ok
}

// Len reports the number of parts, not counting [Content_Types].xml, which is
// not a part.
func (p *Package) Len() int {
	if p == nil {
		return 0
	}
	return len(p.parts)
}

// Put adds or replaces a part.
//
// A part added to a package that was opened is appended to the existing
// archive order rather than sorted into it, so the parts that were already
// there keep their positions and stay byte-identical.
func (p *Package) Put(part *Part) error {
	if p == nil || part == nil {
		return verr.NewCodedError(verr.VELLUM_INTERNAL_INVARIANT, "nil package or part")
	}
	if err := ValidatePartName(part.Name); err != nil {
		return err
	}
	if (part.Data == nil) == (part.Open == nil) {
		return verr.NewCodedErrorWithDetails(verr.VELLUM_INTERNAL_INVARIANT,
			"part must set exactly one of Data and Open",
			map[string]any{"part_name": part.Name})
	}

	if _, exists := p.parts[part.Name]; !exists && len(p.order) > 0 {
		p.order = append(p.order, part.Name)
	}
	p.parts[part.Name] = part

	if part.ContentType != "" {
		if declared, ok := p.ct.ContentTypeFor(part.Name); !ok || declared != part.ContentType {
			p.ct.SetOverride(part.Name, part.ContentType)
		}
	}
	return nil
}

// Delete removes the named part, its content-type override, and any
// relationship set it owned.
func (p *Package) Delete(name string) error {
	if p == nil {
		return verr.NewCodedError(verr.VELLUM_INTERNAL_INVARIANT, "nil package")
	}
	if _, ok := p.parts[name]; !ok {
		return verr.NewCodedErrorWithDetails(verr.VELLUM_OPC_PART_NOT_FOUND,
			"the package does not contain the named part",
			map[string]any{"part_name": name})
	}
	delete(p.parts, name)
	delete(p.rels, name)
	p.ct.removeOverride(name)
	for i, n := range p.order {
		if n == name {
			p.order = append(p.order[:i], p.order[i+1:]...)
			break
		}
	}
	return nil
}

// Walk calls fn for each part in write order. A non-nil return from fn stops
// the walk and is returned.
func (p *Package) Walk(fn func(*Part) error) error {
	if p == nil {
		return nil
	}
	for _, name := range p.writeOrder() {
		part, ok := p.parts[name]
		if !ok {
			continue
		}
		if err := fn(part); err != nil {
			return err
		}
	}
	return nil
}

// Names returns the part names in write order.
func (p *Package) Names() []string {
	if p == nil {
		return nil
	}
	order := p.writeOrder()
	out := make([]string, 0, len(order))
	for _, n := range order {
		if n == ContentTypesName {
			continue
		}
		out = append(out, n)
	}
	return out
}

// ContentTypes returns the package's content-type declaration.
func (p *Package) ContentTypes() *ContentTypes {
	if p == nil {
		return nil
	}
	return p.ct
}

// Relationships returns the relationship set owned by the named part, creating
// an empty one if it does not exist yet. Pass "/" or the empty string for the
// package-level relationships.
func (p *Package) Relationships(owner string) *Relationships {
	if p == nil {
		return nil
	}
	if owner == "" {
		owner = "/"
	}
	if r, ok := p.rels[owner]; ok {
		return r
	}
	r := &Relationships{}
	p.rels[owner] = r
	return r
}

// RelationshipsFor returns the relationship set owned by the named part
// without creating one. The second result reports existence.
func (p *Package) RelationshipsFor(owner string) (*Relationships, bool) {
	if p == nil {
		return nil, false
	}
	if owner == "" {
		owner = "/"
	}
	r, ok := p.rels[owner]
	return r, ok
}

// writeOrder returns the part names to emit, in order.
//
// For an opened package this is the recorded archive order. For a built one it
// is the canonical order: [Content_Types].xml first — some consumers tolerate
// otherwise and none should be relied on to — then the package relationships,
// then every other part sorted bytewise with each part's own relationships
// part immediately following its owner.
func (p *Package) writeOrder() []string {
	if len(p.order) > 0 {
		out := make([]string, len(p.order))
		copy(out, p.order)
		return out
	}

	owners := make([]string, 0, len(p.parts))
	relsParts := make(map[string]bool, len(p.parts))
	for name := range p.parts {
		if IsRelsPart(name) {
			relsParts[name] = true
			continue
		}
		owners = append(owners, name)
	}
	sort.Strings(owners)

	out := make([]string, 0, len(p.parts)+2)
	out = append(out, ContentTypesName)
	if p.parts[RootRelsName] != nil || p.rels["/"].Len() > 0 {
		out = append(out, RootRelsName)
		delete(relsParts, RootRelsName)
	}
	for _, name := range owners {
		out = append(out, name)
		rn := RelsNameFor(name)
		if relsParts[rn] || p.rels[name].Len() > 0 {
			out = append(out, rn)
			delete(relsParts, rn)
		}
	}

	// Any relationships part whose owner is absent. Emitted rather than
	// dropped: a package Vellum did not build may legitimately contain one,
	// and silently discarding content is the failure this library exists to
	// avoid.
	if len(relsParts) > 0 {
		orphans := make([]string, 0, len(relsParts))
		for name := range relsParts {
			orphans = append(orphans, name)
		}
		sort.Strings(orphans)
		out = append(out, orphans...)
	}
	return out
}

// materialiseRels serialises every relationship set that was built or mutated
// into its part. Sets that were parsed and left alone are untouched, so their
// original bytes survive.
func (p *Package) materialiseRels() error {
	owners := make([]string, 0, len(p.rels))
	for owner := range p.rels {
		owners = append(owners, owner)
	}
	sort.Strings(owners)

	for _, owner := range owners {
		r := p.rels[owner]
		if r.Len() == 0 && !r.dirty {
			continue
		}
		if r.parsed && !r.dirty {
			continue
		}
		name := RelsNameFor(owner)
		if err := p.Put(&Part{
			Name:        name,
			ContentType: relsContentType,
			Data:        r.marshal(),
		}); err != nil {
			return err
		}
		// The relationships content type is carried by the package-level
		// default, so an override for every rels part would be redundant
		// noise in a file consumers read.
		p.ct.SetDefault("rels", relsContentType)
		p.ct.removeOverride(name)
	}
	return nil
}

// WriteTo emits the package as a deterministic OPC archive.
//
// The same package and the same options produce byte-identical output, and an
// unmutated package read by [Open] reproduces its input exactly.
func (p *Package) WriteTo(w io.Writer, opts zipdet.WriteOptions) error {
	if p == nil {
		return verr.NewCodedError(verr.VELLUM_INTERNAL_INVARIANT, "nil package")
	}
	if err := p.materialiseRels(); err != nil {
		return err
	}

	order := p.writeOrder()
	entries := make([]zipdet.Entry, 0, len(order)+1)
	wroteContentTypes := false

	for _, name := range order {
		if name == ContentTypesName {
			entries = append(entries, zipdet.Entry{
				Name: ContentTypesName,
				Kind: zipdet.KindCompressible,
				Data: p.ct.marshal(),
			})
			wroteContentTypes = true
			continue
		}

		part, ok := p.parts[name]
		if !ok {
			continue
		}
		ct, declared := p.ct.ContentTypeFor(name)
		if !declared {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_OPC_CONTENT_TYPE_MISSING,
				"a part has no declared content type and none can be derived from its extension",
				map[string]any{"part_name": name, "extension": extension(name)})
		}

		entry := zipdet.Entry{Name: entryName(name), Kind: kindFor(ct)}
		if m, ok := part.srcMethod(); ok {
			// Preserve the method the part was read with, so a round trip
			// reproduces it exactly.
			if m == zip.Store {
				entry.Kind = zipdet.KindPrecompressed
			} else {
				entry.Kind = zipdet.KindCompressible
			}
		}
		if part.Data != nil {
			entry.Data = part.Data
		} else {
			entry.Open = part.Open
		}
		entries = append(entries, entry)
	}

	if !wroteContentTypes {
		// [Content_Types].xml must be the first entry. Some consumers tolerate
		// otherwise; none should be relied on to.
		entries = append([]zipdet.Entry{{
			Name: ContentTypesName,
			Kind: zipdet.KindCompressible,
			Data: p.ct.marshal(),
		}}, entries...)
	}

	return zipdet.Write(w, entries, opts)
}

// precompressedTypes are the content types whose payloads are already
// compressed and would only grow under deflate.
var precompressedTypes = []string{
	"image/png",
	"image/jpeg",
	"image/gif",
	"image/webp",
	"video/",
	"audio/",
	"application/zip",
	"font/woff",
	"font/woff2",
}

// kindFor maps a content type to a compression kind. A rule over the declared
// type, never content sniffing: sniffing would make the method depend on the
// bytes, and two near-identical inputs could then compress differently.
func kindFor(contentType string) zipdet.Kind {
	ct := strings.ToLower(contentType)
	for _, t := range precompressedTypes {
		if strings.HasPrefix(ct, t) {
			return zipdet.KindPrecompressed
		}
	}
	return zipdet.KindCompressible
}

// Clone returns a deep copy of the package's structure.
//
// Part content is shared rather than duplicated — parts are treated as
// immutable, and copying every payload would make cloning a large deck
// proportional to its size for no benefit. What is duplicated is everything a
// caller might mutate: the part index, the write order, the relationship sets
// and the content-type declaration.
//
// Fill mode depends on this: filling a template returns a new package and
// leaves the opened one untouched, so the same template can be filled twice
// with different data.
func (p *Package) Clone() *Package {
	if p == nil {
		return nil
	}
	c := &Package{
		parts: make(map[string]*Part, len(p.parts)),
		order: make([]string, len(p.order)),
		rels:  make(map[string]*Relationships, len(p.rels)),
	}
	copy(c.order, p.order)
	for name, part := range p.parts {
		c.parts[name] = part.clone()
	}
	for owner, r := range p.rels {
		rc := *r
		rc.rels = make([]Relationship, len(r.rels))
		copy(rc.rels, r.rels)
		c.rels[owner] = &rc
	}
	ct := *p.ct
	ct.defaults = make([]Default, len(p.ct.defaults))
	copy(ct.defaults, p.ct.defaults)
	ct.overrides = make([]Override, len(p.ct.overrides))
	copy(ct.overrides, p.ct.overrides)
	c.ct = &ct
	return c
}

// Validate checks the package's referential integrity: that every internal
// relationship target resolves to a part the package actually carries.
//
// It is deliberately not called by [Open]. A package Vellum did not build may
// legitimately reference a part it does not carry — a template stripped of an
// unused header, a document assembled by a tool with looser habits — and
// refusing to open such a file would leave fill mode unable to inspect the
// very documents it exists to work with. Reading is permissive about semantic
// consistency; writing is not.
//
// It is also not called by [Package.WriteTo], because writing back a package
// that was read must reproduce it exactly, including whatever inconsistencies
// it arrived with. Compose paths, which build a package from nothing and have
// no such excuse, call it before emitting.
func (p *Package) Validate() error {
	if p == nil {
		return verr.NewCodedError(verr.VELLUM_INTERNAL_INVARIANT, "nil package")
	}

	owners := make([]string, 0, len(p.rels))
	for owner := range p.rels {
		owners = append(owners, owner)
	}
	sort.Strings(owners)

	for _, owner := range owners {
		for _, rel := range p.rels[owner].All() {
			if rel.Mode == TargetExternal {
				// External targets are never resolved. Vellum performs no
				// network I/O and has no business deciding whether a hyperlink
				// points anywhere.
				continue
			}
			target := resolveTarget(owner, rel.Target)
			if !p.Has(target) {
				return verr.NewCodedErrorWithDetails(verr.VELLUM_OPC_RELATIONSHIP_INVALID,
					"a relationship points at a part the package does not contain",
					map[string]any{
						"owner":         owner,
						"relationship":  rel.ID,
						"target":        rel.Target,
						"resolved_part": target,
					})
			}
		}
	}
	return nil
}

// resolveTarget resolves a relationship target against its owner's directory.
//
// An absolute target is package-rooted; a relative one is resolved against the
// directory holding the owning part, which is why the owner's own name — not
// the relationships part's name — is the base.
func resolveTarget(owner, target string) string {
	if strings.HasPrefix(target, "/") {
		return target
	}

	base := "/"
	if owner != "/" && owner != "" {
		if i := strings.LastIndexByte(owner, '/'); i >= 0 {
			base = owner[:i+1]
		}
	}

	segments := strings.Split(strings.TrimPrefix(base, "/"), "/")
	if len(segments) > 0 && segments[len(segments)-1] == "" {
		segments = segments[:len(segments)-1]
	}
	for _, seg := range strings.Split(target, "/") {
		switch seg {
		case "", ".":
			// Nothing to do; an empty segment is a doubled slash.
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
