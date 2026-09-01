package color_test

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/frankbardon/vellum/pdf/color"
)

// requiredTags are the tags an RGB matrix/TRC display profile must carry.
//
// A profile missing one is not a profile a validator will accept as an output
// intent's destination, which is the only thing this one is for.
var requiredTags = []string{"desc", "wtpt", "rXYZ", "gXYZ", "bXYZ", "rTRC", "gTRC", "bTRC", "cprt"}

// TestSRGBProfile_HeaderIsWellFormed reads the profile the way a validator
// does.
//
// The profile is built rather than shipped as a blob, which removes a
// redistributed binary and its licence and puts the bytes in reviewable source.
// It also means nothing outside this repository has ever checked them, so the
// structure is asserted here rather than assumed.
func TestSRGBProfile_HeaderIsWellFormed(t *testing.T) {
	p := color.SRGBProfile()

	if len(p) < 132 {
		t.Fatalf("the profile is %d bytes, too short for a header and a tag count", len(p))
	}
	if got := binary.BigEndian.Uint32(p); int(got) != len(p) {
		t.Errorf("the header declares %d bytes and the profile is %d", got, len(p))
	}
	if got := binary.BigEndian.Uint32(p[8:]); got>>16 != 0x0240 {
		t.Errorf("the version is %#08x, want 2.4", got)
	}
	if got := string(p[12:16]); got != "mntr" {
		t.Errorf("the device class is %q, want mntr", got)
	}
	if got := string(p[16:20]); got != "RGB " {
		t.Errorf("the colour space is %q, want RGB", got)
	}
	if got := string(p[20:24]); got != "XYZ " {
		t.Errorf("the connection space is %q, want XYZ", got)
	}
	if got := string(p[36:40]); got != "acsp" {
		t.Errorf("the file signature is %q, want acsp", got)
	}

	// The PCS illuminant is fixed at D50 by the specification, not chosen.
	wantD50 := []int32{0xF6D6, 0x10000, 0xD32D}
	for i, want := range wantD50 {
		if got := int32(binary.BigEndian.Uint32(p[68+i*4:])); got != want {
			t.Errorf("PCS illuminant component %d is %#x, want %#x", i, got, want)
		}
	}
}

// TestSRGBProfile_TagTableIsConsistent walks every tag the way a reader would.
func TestSRGBProfile_TagTableIsConsistent(t *testing.T) {
	p := color.SRGBProfile()

	count := int(binary.BigEndian.Uint32(p[128:]))
	if count != len(requiredTags) {
		t.Errorf("the profile declares %d tags, want %d", count, len(requiredTags))
	}
	if 132+count*12 > len(p) {
		t.Fatalf("the tag table runs past the end of the profile")
	}

	seen := map[string]bool{}
	var previous string
	for i := range count {
		rec := p[132+i*12:]
		sig := string(rec[:4])
		offset := int(binary.BigEndian.Uint32(rec[4:]))
		size := int(binary.BigEndian.Uint32(rec[8:]))

		if offset+size > len(p) {
			t.Errorf("tag %q runs past the end of the profile", sig)
			continue
		}
		if offset%4 != 0 {
			t.Errorf("tag %q starts at offset %d, which is not four-byte aligned", sig, offset)
		}
		if offset < 132+count*12 {
			t.Errorf("tag %q starts inside the tag table", sig)
		}
		// The specification says signatures should ascend, and a reader doing a
		// binary search over them depends on it.
		if previous != "" && sig <= previous {
			t.Errorf("tag %q follows %q, so the table is not in ascending order", sig, previous)
		}
		previous = sig
		seen[sig] = true
	}

	for _, want := range requiredTags {
		if !seen[want] {
			t.Errorf("the profile has no %q tag", want)
		}
	}
}

// TestSRGBProfile_TagTypesMatchTheirSignatures checks each element's type
// signature, which is what a reader dispatches on.
func TestSRGBProfile_TagTypesMatchTheirSignatures(t *testing.T) {
	p := color.SRGBProfile()
	count := int(binary.BigEndian.Uint32(p[128:]))

	wantType := map[string]string{
		"desc": "desc", "cprt": "text",
		"wtpt": "XYZ ", "rXYZ": "XYZ ", "gXYZ": "XYZ ", "bXYZ": "XYZ ",
		"rTRC": "curv", "gTRC": "curv", "bTRC": "curv",
	}

	for i := range count {
		rec := p[132+i*12:]
		sig := string(rec[:4])
		offset := int(binary.BigEndian.Uint32(rec[4:]))

		want, known := wantType[sig]
		if !known {
			t.Errorf("the profile carries an unexpected tag %q", sig)
			continue
		}
		if got := string(p[offset : offset+4]); got != want {
			t.Errorf("tag %q holds a %q element, want %q", sig, got, want)
		}
	}
}

// TestSRGBProfile_IsConstant pins that nothing in it varies.
//
// It is embedded in every conforming file Vellum writes, so a profile that
// varied would make every PDF differ from every other for a reason nothing in
// the document expresses.
func TestSRGBProfile_IsConstant(t *testing.T) {
	first := color.SRGBProfile()
	for range 25 {
		if !bytes.Equal(first, color.SRGBProfile()) {
			t.Fatal("two calls produced different profiles")
		}
	}
}

// TestSRGBProfile_IsNotAliased pins that a caller cannot corrupt the profile
// for everybody else.
func TestSRGBProfile_IsNotAliased(t *testing.T) {
	a := color.SRGBProfile()
	a[0] = 0xFF

	if b := color.SRGBProfile(); b[0] == 0xFF {
		t.Error("the profile is returned by reference; one caller writing to it changes every later one")
	}
}
