package dettest_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/frankbardon/vellum/internal/dettest"
	"github.com/frankbardon/vellum/opc/zipdet"
)

// TestGoldensNotHandEdited is the gate that keeps goldens usable as evidence.
//
// A golden that can be quietly adjusted to match the code proves nothing. The
// manifest pins each artifact's digest, and the manifest's own trailer pins
// the manifest, so editing either is detected. Regeneration is deliberate:
//
//	go test ./internal/dettest -update
func TestGoldensNotHandEdited(t *testing.T) {
	root := dettest.GoldenRoot
	cases := dettest.Cases()

	if *update {
		regenerate(t, root, cases)
		return
	}

	m, err := dettest.LoadManifest(root)
	if err != nil {
		t.Fatalf("%v", err)
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			want, err := dettest.ReadGolden(root, c, m)
			if err != nil {
				t.Fatalf("%v", err)
			}
			got, err := c.Bytes(zipdet.PinnedEpoch)
			if err != nil {
				t.Fatalf("emit: %v", err)
			}
			// The assertion is on raw bytes. The normalised description below
			// is for reading only; comparing normalised forms would excuse
			// exactly the differences this suite exists to catch.
			if !bytes.Equal(want, got) {
				t.Errorf("output does not match the committed golden.\n%s\nIf the change is intended, regenerate with:\n  go test ./internal/dettest -update",
					dettest.DescribeMismatch(want, got))
			}
		})
	}

	if orphans := dettest.OrphanGoldens(m, cases); len(orphans) > 0 {
		t.Errorf("the manifest lists goldens with no matching case: %v.\nA renamed case leaves a stale artifact behind claiming to be evidence; regenerate with -update", orphans)
	}
}

func regenerate(t *testing.T, root string, cases []dettest.Case) {
	t.Helper()

	m := &dettest.Manifest{Entries: map[string]dettest.ManifestEntry{}}
	for _, c := range cases {
		artifact, err := c.Bytes(zipdet.PinnedEpoch)
		if err != nil {
			t.Fatalf("emit %s: %v", c.Name, err)
		}
		if err := dettest.WriteGolden(root, c, artifact, m); err != nil {
			t.Fatalf("write golden %s: %v", c.Name, err)
		}
		t.Logf("wrote %s (%d bytes)", dettest.GoldenPath(root, c), len(artifact))
	}

	// Remove directories for cases that no longer exist, so -update leaves no
	// stale artifact behind.
	entries, err := os.ReadDir(root)
	if err == nil {
		live := make(map[string]bool, len(cases))
		for _, c := range cases {
			live[c.Name] = true
		}
		for _, e := range entries {
			if e.IsDir() && !live[e.Name()] {
				if err := os.RemoveAll(filepath.Join(root, e.Name())); err != nil {
					t.Fatalf("removing stale golden %s: %v", e.Name(), err)
				}
				t.Logf("removed stale golden directory %s", e.Name())
			}
		}
	}

	if err := dettest.SaveManifest(root, m); err != nil {
		t.Fatalf("save manifest: %v", err)
	}
	t.Log("goldens regenerated; review the diff before committing")
}

// TestGoldenManifestDetectsTampering proves the gate actually catches an edit
// rather than merely claiming to.
func TestGoldenManifestDetectsTampering(t *testing.T) {
	root := t.TempDir()
	c := dettest.Cases()[0]

	artifact, err := c.Bytes(zipdet.PinnedEpoch)
	if err != nil {
		t.Fatalf("emit: %v", err)
	}
	m := &dettest.Manifest{Entries: map[string]dettest.ManifestEntry{}}
	if err := dettest.WriteGolden(root, c, artifact, m); err != nil {
		t.Fatalf("write golden: %v", err)
	}
	if err := dettest.SaveManifest(root, m); err != nil {
		t.Fatalf("save manifest: %v", err)
	}

	t.Run("intact manifest loads", func(t *testing.T) {
		if _, err := dettest.LoadManifest(root); err != nil {
			t.Fatalf("a freshly written manifest failed to load: %v", err)
		}
	})

	t.Run("edited artifact is caught", func(t *testing.T) {
		path := dettest.GoldenPath(root, c)
		tampered := append(bytes.Clone(artifact), 0x00)
		if err := os.WriteFile(path, tampered, 0o644); err != nil {
			t.Fatalf("tamper: %v", err)
		}
		loaded, err := dettest.LoadManifest(root)
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		if _, err := dettest.ReadGolden(root, c, loaded); err == nil {
			t.Error("a hand-edited artifact was accepted")
		}
		if err := os.WriteFile(path, artifact, 0o644); err != nil {
			t.Fatalf("restore: %v", err)
		}
	})

	t.Run("edited manifest is caught", func(t *testing.T) {
		path := filepath.Join(root, "manifest.json")
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read manifest: %v", err)
		}
		edited := bytes.Replace(raw, []byte(`"bytes"`), []byte(`"BYTES"`), 1)
		if bytes.Equal(edited, raw) {
			t.Fatal("test fixture did not change the manifest")
		}
		if err := os.WriteFile(path, edited, 0o644); err != nil {
			t.Fatalf("tamper: %v", err)
		}
		if _, err := dettest.LoadManifest(root); err == nil {
			t.Error("a hand-edited manifest was accepted")
		}
	})
}

// TestDescribeMismatchIsReadable checks the failure display does the job it
// exists for: naming the part that differs rather than dumping bytes.
func TestDescribeMismatchIsReadable(t *testing.T) {
	c := dettest.Cases()[0]
	want, err := c.Bytes(zipdet.PinnedEpoch)
	if err != nil {
		t.Fatalf("emit: %v", err)
	}
	got, err := c.Bytes(zipdet.PinnedEpoch.AddDate(1, 0, 0))
	if err != nil {
		t.Fatalf("emit: %v", err)
	}

	desc := dettest.DescribeMismatch(want, got)
	if !bytes.Contains([]byte(desc), []byte("parts identical")) {
		t.Errorf("mismatch description does not report identical parts:\n%s", desc)
	}
	t.Logf("sample failure display:\n%s", desc)
}
