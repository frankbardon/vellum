package dettest_test

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/frankbardon/vellum/internal/dettest"
	"github.com/frankbardon/vellum/opc/zipdet"
)

// TestXLSX_StaysWithinThePresentationTablesBoundary checks the line CLAUDE.md
// draws for this format directly against the bytes: no formulas, no pivot
// tables, no macros. A workbook is Vellum's presentation-table target, not a
// spreadsheet-authoring one — a consumer wanting a live model builds it
// elsewhere and hands Vellum the numbers, not the other way around.
//
// A capability-matrix row cannot express this the way it expresses a block or
// a font-embedding mode, because there is no "formula" feature a caller could
// ask for and be refused: nothing in the block model can request one. The
// boundary is enforced here instead, over the actual package, so a future
// change that starts writing `<f>` elements or a `vbaProject.bin` part is
// caught as what it would be — a scope change nobody decided — rather than
// discovered by a consumer building on the promise that it would not happen.
func TestXLSX_StaysWithinThePresentationTablesBoundary(t *testing.T) {
	for _, c := range dettest.Cases() {
		if c.Ext != "xlsx" {
			continue
		}

		t.Run(c.Name, func(t *testing.T) {
			body, err := c.Bytes(zipdet.PinnedEpoch)
			if err != nil {
				t.Fatalf("emit: %v", err)
			}
			zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
			if err != nil {
				t.Fatalf("reading the package: %v", err)
			}

			for _, f := range zr.File {
				name := f.Name

				if name == "xl/vbaProject.bin" || strings.HasSuffix(name, "vbaProject.bin") {
					t.Errorf("the package carries %s: a macro project, which a presentation-table "+
						"target must never write", name)
				}
				if strings.HasPrefix(name, "xl/pivotTables/") || strings.HasPrefix(name, "xl/pivotCache/") {
					t.Errorf("the package carries %s: a pivot table part, which a presentation-table "+
						"target must never write", name)
				}

				if name == "[Content_Types].xml" {
					rc, err := f.Open()
					if err != nil {
						t.Fatalf("opening %s: %v", name, err)
					}
					raw, err := io.ReadAll(rc)
					rc.Close()
					if err != nil {
						t.Fatalf("reading %s: %v", name, err)
					}
					if strings.Contains(string(raw), "macroEnabled") {
						t.Errorf("[Content_Types].xml declares a macro-enabled content type")
					}
					continue
				}

				if !strings.HasPrefix(name, "xl/worksheets/") || !strings.HasSuffix(name, ".xml") {
					continue
				}
				rc, err := f.Open()
				if err != nil {
					t.Fatalf("opening %s: %v", name, err)
				}
				raw, err := io.ReadAll(rc)
				rc.Close()
				if err != nil {
					t.Fatalf("reading %s: %v", name, err)
				}
				if strings.Contains(string(raw), "<f>") || strings.Contains(string(raw), "<f ") {
					t.Errorf("%s carries a formula element (<f>); a presentation-table target "+
						"writes live values, never a computation a reader would have to evaluate", name)
				}
			}
		})
	}
}

// TestXLSX_TheContentTypeIsNeverMacroEnabled pins the boundary at its
// narrowest point: even with no macro part written, the workbook's own
// content type must name the ordinary, non-macro form. A reader that trusts
// the extension over the declared type would still refuse a mismatched file,
// and a macro-enabled content type is itself a claim about the document's
// capabilities that this library must never make.
func TestXLSX_TheContentTypeIsNeverMacroEnabled(t *testing.T) {
	found := false
	for _, c := range dettest.Cases() {
		if c.Ext != "xlsx" {
			continue
		}
		found = true

		body, err := c.Bytes(zipdet.PinnedEpoch)
		if err != nil {
			t.Fatalf("emit: %v", err)
		}
		zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
		if err != nil {
			t.Fatalf("reading the package: %v", err)
		}
		var ct []byte
		for _, f := range zr.File {
			if f.Name != "[Content_Types].xml" {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				t.Fatalf("opening [Content_Types].xml: %v", err)
			}
			ct, err = io.ReadAll(rc)
			rc.Close()
			if err != nil {
				t.Fatalf("reading [Content_Types].xml: %v", err)
			}
		}
		if !strings.Contains(string(ct), "spreadsheetml.sheet.main+xml") {
			t.Errorf("%s: [Content_Types].xml does not declare the ordinary workbook content type", c.Name)
		}
	}
	if !found {
		t.Fatal("no xlsx case is registered; this test would pass vacuously")
	}
}
