package opc

import (
	"encoding/xml"
	"fmt"
	"sort"
	"strings"

	verr "github.com/frankbardon/vellum/errors"
)

// TargetMode distinguishes a relationship pointing at a part inside the
// package from one pointing outside it.
type TargetMode uint8

const (
	// TargetInternal points at another part in the same package.
	TargetInternal TargetMode = iota

	// TargetExternal points outside the package — a hyperlink, a linked
	// image. External targets are never resolved by Vellum, which performs no
	// network I/O.
	TargetExternal
)

// String implements fmt.Stringer.
func (m TargetMode) String() string {
	if m == TargetExternal {
		return "External"
	}
	return "Internal"
}

// Relationship is one edge from a part (or from the package root) to a target.
type Relationship struct {
	// ID is the relationship identifier referenced from the owning part's
	// markup, conventionally "rId1", "rId2" and so on.
	ID string

	// Type is the relationship type URI.
	Type string

	// Target is the target's location, relative to the owner's directory for
	// an internal relationship.
	Target string

	// Mode distinguishes internal from external targets.
	Mode TargetMode
}

// Relationships is the ordered set of relationships belonging to one part, or
// to the package root.
//
// It is deliberately not a map. Relationship order is part of the serialised
// bytes, and a map would make the emitted order depend on Go's hash seed —
// the single most common way a writer like this becomes non-deterministic.
type Relationships struct {
	rels []Relationship

	// parsed records that this set was read from an existing part rather than
	// built. A parsed set that is never mutated re-emits its original bytes
	// verbatim, which is what makes Open-then-WriteTo byte-identical.
	parsed bool
	raw    []byte
	dirty  bool
}

// Len reports the number of relationships.
func (r *Relationships) Len() int {
	if r == nil {
		return 0
	}
	return len(r.rels)
}

// All returns the relationships in their serialised order. The slice is a
// copy.
func (r *Relationships) All() []Relationship {
	if r == nil {
		return nil
	}
	out := make([]Relationship, len(r.rels))
	copy(out, r.rels)
	return out
}

// Add appends a relationship and returns its assigned identifier.
//
// Identifiers are assigned in insertion order here, but a built set is
// renumbered from a sorted walk before it is serialised — see
// [Relationships.canonicalise]. Deriving the identifier from the relationship's
// own content rather than from the order the calling code happened to add
// things in is what makes the same logical document produce the same bytes
// regardless of which code path assembled it.
func (r *Relationships) Add(relType, target string, mode TargetMode) (string, error) {
	if r == nil {
		return "", verr.NewCodedError(verr.VELLUM_INTERNAL_INVARIANT, "nil relationships")
	}
	if strings.TrimSpace(relType) == "" {
		return "", verr.NewCodedErrorWithDetails(verr.VELLUM_OPC_RELATIONSHIP_INVALID,
			"relationship type is empty", map[string]any{"target": target})
	}
	if strings.TrimSpace(target) == "" {
		return "", verr.NewCodedErrorWithDetails(verr.VELLUM_OPC_RELATIONSHIP_INVALID,
			"relationship target is empty", map[string]any{"type": relType})
	}

	id := fmt.Sprintf("rId%d", len(r.rels)+1)
	r.rels = append(r.rels, Relationship{ID: id, Type: relType, Target: target, Mode: mode})
	r.dirty = true
	return id, nil
}

// ByType returns every relationship of the given type, in serialised order.
func (r *Relationships) ByType(relType string) []Relationship {
	if r == nil {
		return nil
	}
	var out []Relationship
	for _, rel := range r.rels {
		if rel.Type == relType {
			out = append(out, rel)
		}
	}
	return out
}

// Resolve returns the relationship with the given identifier.
func (r *Relationships) Resolve(id string) (Relationship, bool) {
	if r == nil {
		return Relationship{}, false
	}
	for _, rel := range r.rels {
		if rel.ID == id {
			return rel, true
		}
	}
	return Relationship{}, false
}

// canonicalise sorts a built relationship set and renumbers it.
//
// The sort key is (Type, Mode, Target) — the relationship's own content. It is
// deliberately not insertion order: insertion order depends on which code path
// assembled the document, so two paths producing the same logical document
// would otherwise produce different identifiers and different bytes.
//
// A parsed set is left exactly as it was read. Renumbering someone else's
// package would rewrite identifiers their markup already references.
func (r *Relationships) canonicalise() {
	if r == nil || r.parsed {
		return
	}
	sort.SliceStable(r.rels, func(i, j int) bool {
		a, b := r.rels[i], r.rels[j]
		if a.Type != b.Type {
			return a.Type < b.Type
		}
		if a.Mode != b.Mode {
			return a.Mode < b.Mode
		}
		return a.Target < b.Target
	})
	for i := range r.rels {
		r.rels[i].ID = fmt.Sprintf("rId%d", i+1)
	}
}

const (
	relsNamespace   = "http://schemas.openxmlformats.org/package/2006/relationships"
	relsContentType = "application/vnd.openxmlformats-package.relationships+xml"
	xmlDeclaration  = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n"
)

// marshal serialises the relationship set.
//
// A parsed, unmutated set returns its original bytes. That is the whole
// mechanism behind the byte-preserving round trip: re-emitting even a
// semantically identical document would change whitespace, attribute order and
// self-closing form, and fill mode promises not to.
func (r *Relationships) marshal() []byte {
	if r == nil {
		return nil
	}
	if r.parsed && !r.dirty {
		return r.raw
	}

	r.canonicalise()

	var b strings.Builder
	b.WriteString(xmlDeclaration)
	b.WriteString(`<Relationships xmlns="`)
	b.WriteString(relsNamespace)
	b.WriteString(`">`)
	for _, rel := range r.rels {
		b.WriteString(`<Relationship Id="`)
		b.WriteString(escapeAttr(rel.ID))
		b.WriteString(`" Type="`)
		b.WriteString(escapeAttr(rel.Type))
		b.WriteString(`" Target="`)
		b.WriteString(escapeAttr(rel.Target))
		if rel.Mode == TargetExternal {
			b.WriteString(`" TargetMode="External`)
		}
		b.WriteString(`"/>`)
	}
	b.WriteString(`</Relationships>`)
	return []byte(b.String())
}

// relsXML mirrors the on-disk shape for unmarshalling. It is used only to read
// an existing part; writing goes through marshal, which controls the bytes
// exactly.
type relsXML struct {
	XMLName xml.Name `xml:"Relationships"`
	Rels    []struct {
		ID         string `xml:"Id,attr"`
		Type       string `xml:"Type,attr"`
		Target     string `xml:"Target,attr"`
		TargetMode string `xml:"TargetMode,attr"`
	} `xml:"Relationship"`
}

// parseRels reads a relationships part, retaining the original bytes so an
// unmutated set can be re-emitted verbatim.
func parseRels(name string, data []byte) (*Relationships, error) {
	var doc relsXML
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, verr.WrapCodedErrorWithDetails(err, verr.VELLUM_OPC_RELATIONSHIP_INVALID,
			"the relationships part is not well-formed XML",
			map[string]any{"part_name": name})
	}

	r := &Relationships{parsed: true, raw: data}
	for _, x := range doc.Rels {
		if strings.TrimSpace(x.ID) == "" || strings.TrimSpace(x.Type) == "" || strings.TrimSpace(x.Target) == "" {
			return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_OPC_RELATIONSHIP_INVALID,
				"a relationship is missing its identifier, type or target",
				map[string]any{"part_name": name, "id": x.ID, "type": x.Type, "target": x.Target})
		}
		mode := TargetInternal
		if strings.EqualFold(x.TargetMode, "External") {
			mode = TargetExternal
		}
		r.rels = append(r.rels, Relationship{ID: x.ID, Type: x.Type, Target: x.Target, Mode: mode})
	}
	return r, nil
}

// escapeAttr escapes a string for use in an XML attribute value.
//
// Hand-rolled rather than via xml.EscapeText because that function also
// escapes newlines and tabs as character references, which is correct but
// noisier than necessary, and because the exact output bytes here are part of
// a determinism contract worth controlling directly.
func escapeAttr(s string) string {
	if !strings.ContainsAny(s, `&<>"'`) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\'':
			b.WriteString("&apos;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
