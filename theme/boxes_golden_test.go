package theme_test

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/frankbardon/vellum/artifact"
	"github.com/frankbardon/vellum/descriptor"
	"github.com/frankbardon/vellum/theme"
)

var update = flag.Bool("update", false, "rewrite the golden files")

// boxRow is the golden's shape: EMU rather than the authored units, because a
// consumer keying render presets off this set cares what size the box actually
// is, not how the theme happened to spell it.
type boxRow struct {
	Format    string `json:"format"`
	Role      string `json:"role"`
	WidthEMU  int64  `json:"width_emu"`
	HeightEMU int64  `json:"height_emu"`
	Intrinsic bool   `json:"intrinsic_height"`
}

// TestBoxesGolden pins the built-in theme's complete answer to the layout
// query, for every format.
//
// This is a contract golden rather than a regression test. The set is what a
// host enumerates its render presets from — one artifact per (role, box) — so
// moving a box invalidates a cache the host has already filled. That is a
// legitimate thing to do and an illegitimate thing to do by accident, which is
// exactly what a golden with a hash trailer is for.
func TestBoxesGolden(t *testing.T) {
	th, err := theme.Builtin()
	if err != nil {
		t.Fatalf("Builtin: %v", err)
	}

	var rows []boxRow
	for _, format := range artifact.AllFormats() {
		for _, b := range th.Boxes(format) {
			w, err := b.Width.EMU()
			if err != nil {
				t.Fatalf("%s/%s width: %v", format, b.Role, err)
			}
			var h int64
			if !b.IntrinsicHeight() {
				if h, err = b.Height.EMU(); err != nil {
					t.Fatalf("%s/%s height: %v", format, b.Role, err)
				}
			}
			rows = append(rows, boxRow{
				Format:    string(format),
				Role:      string(b.Role),
				WidthEMU:  w,
				HeightEMU: h,
				Intrinsic: b.IntrinsicHeight(),
			})
		}
	}

	rendered, err := descriptor.RenderGolden(rows)
	if err != nil {
		t.Fatalf("RenderGolden: %v", err)
	}
	path := filepath.Join("testdata", "boxes.json")

	if *update {
		if err := os.WriteFile(path, rendered, 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden: %v (regenerate with: go test ./theme -update)", err)
	}
	if _, err := descriptor.SplitGolden(want); err != nil {
		t.Fatalf("the committed golden is damaged: %v", err)
	}
	if string(want) != string(rendered) {
		t.Errorf("the layout query's answer has changed.\n\nA host enumerates its render presets from this set, so a moved box "+
			"invalidates artifacts it has already rendered. If the change is intended, regenerate with:\n"+
			"  go test ./theme -update\n\nwant:\n%s\ngot:\n%s", want, rendered)
	}
}
