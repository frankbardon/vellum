package provenance_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/frankbardon/vellum/canon"
	"github.com/frankbardon/vellum/pdf/xmp"
	"github.com/frankbardon/vellum/provenance"
)

// embedded is a filled record, so a test reads a document rather than a zero
// value. A zero value would exercise the branch where nothing is written, which
// is the branch that proves least.
func embedded() *provenance.Record {
	return &provenance.Record{
		VellumVersion:   "0.0.0-test",
		SourceDateEpoch: time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC),
		SpecHash:        "0123456789abcdef0123456789abcdef",
		ThemeHash:       "fedcba9876543210fedcba9876543210",
		Assets: []provenance.AssetRef{
			{Handle: "chart/1", Media: "image/png", Hash: "aaaa"},
		},
		Fonts: []provenance.FontRef{
			{Family: "Go Regular", Embedded: true, SubsetProfile: "glyf"},
		},
		Sources: []provenance.Source{{Kind: "envelope", ID: "e-1"}},
	}
}

// TestXMPSchema_EveryWrittenPropertyIsDescribed is the condition the whole
// schema mechanism exists for.
//
// A property outside the vocabularies XMP itself defines must be described by a
// PDF/A extension schema. One written without a description produces a file
// that opens, whose metadata reads correctly in every tool, and whose
// conformance claim about itself is false — reported by nothing but a
// validator, which is why it is checked here as well.
func TestXMPSchema_EveryWrittenPropertyIsDescribed(t *testing.T) {
	s := embedded().XMPSchema()

	packet := string(xmp.Metadata{Extensions: []xmp.Schema{s}}.Packet())
	for _, p := range s.Properties {
		if p.Value == "" {
			continue
		}
		element := "<" + provenance.Prefix + ":" + p.Name + ">"
		if !strings.Contains(packet, element) {
			t.Errorf("the packet does not write %s, which carries a value", element)
		}
		description := "<pdfaProperty:name>" + p.Name + "</pdfaProperty:name>"
		if !strings.Contains(packet, description) {
			t.Errorf("the packet writes %s without describing it", element)
		}
	}
}

// TestXMPSchema_ADescribedPropertyWithNoValueIsNotWritten pins the other half.
//
// Describing a property this document does not use is legal and is what lets
// the vocabulary be declared once. Writing an empty one is a claim that the
// field is known to be empty, which is different from making no claim.
func TestXMPSchema_ADescribedPropertyWithNoValueIsNotWritten(t *testing.T) {
	s := embedded().XMPSchema() // carries no binding or template: this is not a fill

	packet := string(xmp.Metadata{Extensions: []xmp.Schema{s}}.Packet())
	for _, name := range []string{provenance.PropertyBindingHash, provenance.PropertyTemplateHash,
		provenance.PropertyGeneratedAt} {

		if strings.Contains(packet, "<"+provenance.Prefix+":"+name+">") {
			t.Errorf("the packet writes %s, which this record does not carry", name)
		}
		if !strings.Contains(packet, "<pdfaProperty:name>"+name+"</pdfaProperty:name>") {
			t.Errorf("the packet does not describe %s; the vocabulary is declared whole", name)
		}
	}
}

// TestXMPSchema_TheHashDescribesTheRecordInTheFile is the property that makes
// the record checkable without consulting anything else.
//
// The embedded record and the embedded hash are produced from one canonical
// encoding. Produced separately, a marshalling difference between them would
// make the hash describe a record nobody has — which is worse than carrying no
// hash at all, because it reads as an integrity check and is not one.
func TestXMPSchema_TheHashDescribesTheRecordInTheFile(t *testing.T) {
	r := embedded()
	s := r.XMPSchema()

	var carried, hash string
	for _, p := range s.Properties {
		switch p.Name {
		case provenance.PropertyRecord:
			carried = p.Value
		case provenance.PropertyRecordHash:
			hash = p.Value
		}
	}
	if carried == "" || hash == "" {
		t.Fatalf("the schema carries record=%q hash=%q", carried, hash)
	}

	want, err := canon.Canonical(r)
	if err != nil {
		t.Fatalf("Canonical: %v", err)
	}
	if carried != string(want) {
		t.Errorf("the carried record is\n%s\nwant\n%s", carried, want)
	}
	if got := r.Hash(); got != hash {
		t.Errorf("the carried hash is %q, want %q", hash, got)
	}

	// And the carried record is the record: a reader parses it back rather
	// than being handed a summary of it.
	var back provenance.Record
	if err := json.Unmarshal([]byte(carried), &back); err != nil {
		t.Fatalf("the carried record does not parse: %v", err)
	}
	if len(back.Assets) != 1 || len(back.Fonts) != 1 || len(back.Sources) != 1 {
		t.Errorf("the carried record lost its repeated fields: %+v", back)
	}
}

// TestXMPSchema_ANilRecordDeclaresTheVocabularyAndWritesNothing keeps the
// degenerate case from producing a described schema with nothing under it, or a
// panic.
func TestXMPSchema_ANilRecordDeclaresTheVocabularyAndWritesNothing(t *testing.T) {
	s := (*provenance.Record)(nil).XMPSchema()

	if len(s.Properties) == 0 {
		t.Fatal("the vocabulary is empty")
	}
	for _, p := range s.Properties {
		if p.Value != "" {
			t.Errorf("%s carries %q for a nil record", p.Name, p.Value)
		}
	}

	// A schema nothing uses is not described either. A description with no
	// values is legal and pointless, and it costs bytes in every document that
	// never asked for the vocabulary.
	packet := string(xmp.Metadata{Extensions: []xmp.Schema{s}}.Packet())
	if strings.Contains(packet, "pdfaExtension:schemas") {
		t.Error("an unused schema was described")
	}
}

// TestXMPSchema_EveryPropertyIsWellFormed checks the parts a validator reads
// but a human eye slides over.
func TestXMPSchema_EveryPropertyIsWellFormed(t *testing.T) {
	s := embedded().XMPSchema()

	if s.Namespace == "" || s.Prefix == "" || s.Name == "" {
		t.Fatalf("the schema is %+v", s)
	}
	for _, p := range s.Properties {
		if p.Description == "" {
			t.Errorf("%s has no description; a reader who has never seen the namespace learns nothing", p.Name)
		}
		if !valid(xmp.AllValueTypes(), p.Type) {
			t.Errorf("%s has value type %q, which is not one XMP defines and Vellum describes no others",
				p.Name, p.Type)
		}
		if !valid(xmp.AllCategories(), p.Category) {
			t.Errorf("%s has category %q", p.Name, p.Category)
		}
		if p.Value != "" && !p.Type.Accepts(p.Value) {
			t.Errorf("%s declares type %s and carries %q, which is not that type.\n\n"+
				"This is the failure the oracle caught first: a Boolean written in Go's syntax "+
				"rather than XMP's produces a file that opens, reads correctly everywhere, and "+
				"fails ISO 19005-2 clause 6.6.2.3.1. Use xmp.Bool, xmp.Int or xmp.Date.",
				p.Name, p.Type, p.Value)
		}
	}
}

func valid[T comparable](all []T, want T) bool {
	for _, v := range all {
		if v == want {
			return true
		}
	}
	return false
}
