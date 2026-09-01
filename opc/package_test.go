package opc_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/opc"
	"github.com/frankbardon/vellum/opc/zipdet"
)

const (
	ctDocument = "application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"
	relOffDoc  = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument"
	relStyles  = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles"
	relImage   = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/image"
)

func buildPackage(t *testing.T) *opc.Package {
	t.Helper()
	p := opc.New()

	must(t, p.Put(&opc.Part{
		Name:        "/word/document.xml",
		ContentType: ctDocument,
		Data:        []byte(`<?xml version="1.0"?><w:document/>`),
	}))
	must(t, p.Put(&opc.Part{
		Name:        "/word/styles.xml",
		ContentType: "application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml",
		Data:        []byte(`<?xml version="1.0"?><w:styles/>`),
	}))
	must(t, p.Put(&opc.Part{
		Name:        "/word/media/image1.png",
		ContentType: "image/png",
		Data:        []byte("\x89PNG\r\n\x1a\nnot really"),
	}))

	if _, err := p.Relationships("/").Add(relOffDoc, "word/document.xml", opc.TargetInternal); err != nil {
		t.Fatalf("root rel: %v", err)
	}
	if _, err := p.Relationships("/word/document.xml").Add(relStyles, "styles.xml", opc.TargetInternal); err != nil {
		t.Fatalf("doc rel: %v", err)
	}
	if _, err := p.Relationships("/word/document.xml").Add(relImage, "media/image1.png", opc.TargetInternal); err != nil {
		t.Fatalf("doc rel: %v", err)
	}
	return p
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func writePackage(t *testing.T, p *opc.Package) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := p.WriteTo(&buf, zipdet.WriteOptions{}); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	return buf.Bytes()
}

func digest(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func TestPackage_WriteIsDeterministic(t *testing.T) {
	seen := make(map[string]int)
	for range 200 {
		seen[digest(writePackage(t, buildPackage(t)))]++
	}
	if len(seen) != 1 {
		t.Fatalf("200 writes produced %d distinct digests, want 1", len(seen))
	}
}

// TestPackage_ContentTypesIsFirst pins the ordering rule. Some consumers
// tolerate otherwise; none should be relied on to.
func TestPackage_ContentTypesIsFirst(t *testing.T) {
	raw := writePackage(t, buildPackage(t))

	a, err := zipdet.Read(bytes.NewReader(raw), int64(len(raw)), zipdet.ReadOptions{})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got := a.Names()[0]; got != opc.ContentTypesName {
		t.Errorf("first entry = %q, want %q", got, opc.ContentTypesName)
	}
}

// TestPackage_CanonicalOrder pins that a built package emits its parts in the
// documented order: content types, package relationships, then each part
// sorted bytewise with its own relationships part immediately following it.
func TestPackage_CanonicalOrder(t *testing.T) {
	raw := writePackage(t, buildPackage(t))

	a, err := zipdet.Read(bytes.NewReader(raw), int64(len(raw)), zipdet.ReadOptions{})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	want := []string{
		"[Content_Types].xml",
		"_rels/.rels",
		"word/document.xml",
		"word/_rels/document.xml.rels",
		"word/media/image1.png",
		"word/styles.xml",
	}
	got := a.Names()
	if len(got) != len(want) {
		t.Fatalf("entries = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d = %q, want %q\nfull order: %v", i, got[i], want[i], got)
		}
	}
}

// TestPackage_RelationshipIDsAreContentDerived is the load-bearing determinism
// test for relationships. Two packages built by different code paths — here,
// adding the same relationships in opposite orders — must produce identical
// identifiers, because the identifier comes from a sorted walk of the
// relationship's own content rather than from insertion order.
func TestPackage_RelationshipIDsAreContentDerived(t *testing.T) {
	build := func(reverse bool) []byte {
		p := opc.New()
		must(t, p.Put(&opc.Part{Name: "/word/document.xml", ContentType: ctDocument, Data: []byte("<a/>")}))
		r := p.Relationships("/word/document.xml")
		if reverse {
			mustAdd(t, r, relStyles, "styles.xml")
			mustAdd(t, r, relImage, "media/image1.png")
		} else {
			mustAdd(t, r, relImage, "media/image1.png")
			mustAdd(t, r, relStyles, "styles.xml")
		}
		return writePackage(t, p)
	}

	if !bytes.Equal(build(false), build(true)) {
		t.Error("adding the same relationships in a different order produced different bytes; identifiers must come from a sorted walk, not from insertion order")
	}
}

func mustAdd(t *testing.T, r *opc.Relationships, relType, target string) {
	t.Helper()
	if _, err := r.Add(relType, target, opc.TargetInternal); err != nil {
		t.Fatalf("Add: %v", err)
	}
}

// TestPackage_OpenWriteIsIdentity is the precondition for fill mode's entire
// non-destructiveness guarantee. It is proven here, in the substrate, rather
// than discovered when fill mode is built on top of it.
func TestPackage_OpenWriteIsIdentity(t *testing.T) {
	original := writePackage(t, buildPackage(t))

	p, err := opc.Open(bytes.NewReader(original), int64(len(original)))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	var round bytes.Buffer
	if err := p.WriteTo(&round, zipdet.WriteOptions{}); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}

	if !bytes.Equal(original, round.Bytes()) {
		t.Errorf("Open then WriteTo changed the bytes (%d -> %d); parts must never be re-serialised",
			len(original), round.Len())
	}
}

// TestPackage_OpenWritePreservesUnknownParts covers what the identity test
// exists for: content Vellum does not model must survive untouched.
func TestPackage_OpenWritePreservesUnknownParts(t *testing.T) {
	p := buildPackage(t)
	must(t, p.Put(&opc.Part{
		Name:        "/customXml/item1.xml",
		ContentType: "application/xml",
		Data:        []byte(`<?xml version="1.0"?><custom attr="  spaced  ">text</custom>`),
	}))
	must(t, p.Put(&opc.Part{
		Name:        "/word/comments.xml",
		ContentType: "application/vnd.openxmlformats-officedocument.wordprocessingml.comments+xml",
		Data:        []byte(`<?xml version="1.0"?><w:comments><w:comment w:id="1"/></w:comments>`),
	}))
	original := writePackage(t, p)

	reopened, err := opc.Open(bytes.NewReader(original), int64(len(original)))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	var round bytes.Buffer
	if err := reopened.WriteTo(&round, zipdet.WriteOptions{}); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	if !bytes.Equal(original, round.Bytes()) {
		t.Fatal("a round trip changed a package containing parts Vellum does not model")
	}

	got, ok := reopened.Get("/customXml/item1.xml")
	if !ok {
		t.Fatal("the custom XML part did not survive the round trip")
	}
	data, err := got.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	if !strings.Contains(string(data), `attr="  spaced  "`) {
		t.Error("attribute whitespace was normalised; parts must not be re-serialised")
	}
}

// TestPackage_MutatingOnePartLeavesOthersAlone is the shape of the eventual
// non-destructiveness gate: touching one part must not disturb any other.
func TestPackage_MutatingOnePartLeavesOthersAlone(t *testing.T) {
	original := writePackage(t, buildPackage(t))

	p, err := opc.Open(bytes.NewReader(original), int64(len(original)))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	must(t, p.Put(&opc.Part{
		Name:        "/word/document.xml",
		ContentType: ctDocument,
		Data:        []byte(`<?xml version="1.0"?><w:document><w:body/></w:document>`),
	}))

	var modified bytes.Buffer
	if err := p.WriteTo(&modified, zipdet.WriteOptions{}); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}

	before := entryMap(t, original)
	after := entryMap(t, modified.Bytes())

	for name, want := range before {
		got, ok := after[name]
		if !ok {
			t.Errorf("part %q disappeared", name)
			continue
		}
		if name == "word/document.xml" {
			if bytes.Equal(got, want) {
				t.Error("the edited part was not changed")
			}
			continue
		}
		if !bytes.Equal(got, want) {
			t.Errorf("part %q changed even though it was not touched", name)
		}
	}
}

func entryMap(t *testing.T, raw []byte) map[string][]byte {
	t.Helper()
	a, err := zipdet.Read(bytes.NewReader(raw), int64(len(raw)), zipdet.ReadOptions{})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	out := make(map[string][]byte)
	for _, e := range a.Entries() {
		out[e.Name] = e.Data
	}
	return out
}

func TestPackage_RejectsUndeclaredContentType(t *testing.T) {
	p := opc.New()
	must(t, p.Put(&opc.Part{Name: "/word/mystery.bin", Data: []byte("x")}))

	err := p.WriteTo(&bytes.Buffer{}, zipdet.WriteOptions{})
	if !verr.HasCode(err, verr.VELLUM_OPC_CONTENT_TYPE_MISSING) {
		t.Fatalf("error = %v, want VELLUM_OPC_CONTENT_TYPE_MISSING", err)
	}
}

func TestPackage_PutRejectsBadNames(t *testing.T) {
	tests := []string{
		"word/document.xml", // not absolute
		"/word/../etc/passwd",
		`/word\document.xml`,
		"/word/",
		"",
		"/word//document.xml",
	}
	p := opc.New()
	for _, name := range tests {
		t.Run(name, func(t *testing.T) {
			err := p.Put(&opc.Part{Name: name, ContentType: "application/xml", Data: []byte("x")})
			if !verr.HasCode(err, verr.VELLUM_OPC_PART_NAME_INVALID) {
				t.Errorf("error = %v, want VELLUM_OPC_PART_NAME_INVALID", err)
			}
		})
	}
}

func TestPackage_GetPutDeleteWalk(t *testing.T) {
	p := buildPackage(t)

	if _, ok := p.Get("/word/document.xml"); !ok {
		t.Error("Get missed a part that was put")
	}
	if !p.Has("/word/styles.xml") {
		t.Error("Has missed a part that was put")
	}
	if p.Len() != 3 {
		t.Errorf("Len = %d, want 3", p.Len())
	}

	must(t, p.Delete("/word/styles.xml"))
	if p.Has("/word/styles.xml") {
		t.Error("a deleted part is still present")
	}
	if err := p.Delete("/word/styles.xml"); !verr.HasCode(err, verr.VELLUM_OPC_PART_NOT_FOUND) {
		t.Errorf("deleting a missing part = %v, want VELLUM_OPC_PART_NOT_FOUND", err)
	}

	var walked []string
	must(t, p.Walk(func(part *opc.Part) error {
		walked = append(walked, part.Name)
		return nil
	}))
	if len(walked) != 2 {
		t.Errorf("Walk visited %d parts, want 2: %v", len(walked), walked)
	}
}

func TestPackage_CloneIsIndependent(t *testing.T) {
	p := buildPackage(t)
	before := writePackage(t, p)

	c := p.Clone()
	must(t, c.Put(&opc.Part{Name: "/word/extra.xml", ContentType: "application/xml", Data: []byte("<x/>")}))
	mustAdd(t, c.Relationships("/word/document.xml"), "http://example.invalid/rel/extra", "extra.xml")

	after := writePackage(t, p)
	if !bytes.Equal(before, after) {
		t.Error("mutating a clone changed the original; fill mode depends on the opened template being untouched")
	}
	if !c.Has("/word/extra.xml") {
		t.Error("the clone did not receive the new part")
	}
}

func TestPackage_LazyPartMatchesInline(t *testing.T) {
	payload := []byte("\x89PNG\r\n\x1a\nlazy")

	inline := opc.New()
	must(t, inline.Put(&opc.Part{Name: "/word/media/image1.png", ContentType: "image/png", Data: payload}))

	lazy := opc.New()
	must(t, lazy.Put(&opc.Part{Name: "/word/media/image1.png", ContentType: "image/png",
		Open: func() (readCloser, error) { return nopCloser{bytes.NewReader(payload)}, nil }}))

	if !bytes.Equal(writePackage(t, inline), writePackage(t, lazy)) {
		t.Error("a lazily-opened part produced different bytes from the same inline data")
	}
}

func TestPackage_MediaIsStoredNotDeflated(t *testing.T) {
	raw := writePackage(t, buildPackage(t))
	a, err := zipdet.Read(bytes.NewReader(raw), int64(len(raw)), zipdet.ReadOptions{})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	png, ok := a.Get("word/media/image1.png")
	if !ok {
		t.Fatal("media part missing")
	}
	if png.Method != 0 { // zip.Store
		t.Errorf("PNG method = %d, want Store; compression follows the declared content type by rule", png.Method)
	}
	doc, ok := a.Get("word/document.xml")
	if !ok {
		t.Fatal("document part missing")
	}
	if doc.Method != 8 { // zip.Deflate
		t.Errorf("XML method = %d, want Deflate", doc.Method)
	}
}

func TestPackage_NilReceiverIsSafe(t *testing.T) {
	var p *opc.Package
	if p.Len() != 0 || p.Names() != nil || p.ContentTypes() != nil {
		t.Error("nil package accessors misbehaved")
	}
	if _, ok := p.Get("/x"); ok {
		t.Error("Get on nil package reported a hit")
	}
	if err := p.Walk(func(*opc.Part) error { return nil }); err != nil {
		t.Errorf("Walk on nil package = %v", err)
	}
	if p.Clone() != nil {
		t.Error("Clone on nil package returned non-nil")
	}
}
