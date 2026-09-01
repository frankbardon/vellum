package dettest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// GoldenRoot is the directory holding committed expected artifacts.
const GoldenRoot = "testdata/goldens"

// manifestName is the index of every golden and its digest.
const manifestName = "manifest.json"

// goldenHashPrefix marks the trailer line that pins the manifest's own
// content. Its purpose is narrow and worth stating: it makes hand-editing a
// golden detectable. Goldens are evidence, and evidence that can be quietly
// adjusted to match the code is not evidence.
const goldenHashPrefix = "// golden-hash: "

// ManifestEntry records one committed artifact.
type ManifestEntry struct {
	// File is the artifact path, relative to GoldenRoot.
	File string `json:"file"`

	// SHA256 is the artifact's digest. Binary artifacts cannot carry an
	// in-band trailer, so their integrity is pinned here and this file's own
	// integrity is pinned by its trailer.
	SHA256 string `json:"sha256"`

	// Bytes is the artifact size, carried for human readers scanning a diff.
	Bytes int `json:"bytes"`
}

// Manifest is the golden index, keyed by case name.
type Manifest struct {
	Entries map[string]ManifestEntry `json:"entries"`
}

// LoadManifest reads the golden manifest and verifies its trailer.
func LoadManifest(root string) (*Manifest, error) {
	raw, err := os.ReadFile(filepath.Join(root, manifestName))
	if err != nil {
		return nil, err
	}
	body, recorded, err := splitTrailer(raw)
	if err != nil {
		return nil, err
	}
	if got := digestHex(body); got != recorded {
		return nil, fmt.Errorf(
			"%s has been hand-edited: recorded hash %s, actual %s. Goldens are regenerated with -update, never edited",
			manifestName, recorded, got)
	}

	var m Manifest
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", manifestName, err)
	}
	if m.Entries == nil {
		m.Entries = map[string]ManifestEntry{}
	}
	return &m, nil
}

// SaveManifest writes the manifest with a fresh trailer.
func SaveManifest(root string, m *Manifest) error {
	// Marshal with sorted keys — encoding/json sorts map keys already, which
	// is the one place its determinism can be relied on.
	body, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	out := append(body, []byte(goldenHashPrefix+digestHex(body)+"\n")...)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, manifestName), out, 0o644)
}

// splitTrailer separates the manifest body from its recorded hash.
func splitTrailer(raw []byte) (body []byte, recorded string, err error) {
	s := string(raw)
	i := strings.LastIndex(s, goldenHashPrefix)
	if i < 0 {
		return nil, "", fmt.Errorf("%s has no %q trailer; regenerate it with -update", manifestName, strings.TrimSpace(goldenHashPrefix))
	}
	recorded = strings.TrimSpace(s[i+len(goldenHashPrefix):])
	return []byte(s[:i]), recorded, nil
}

func digestHex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// GoldenPath is the artifact path for a case.
func GoldenPath(root string, c Case) string {
	return filepath.Join(root, c.Name, "expected."+c.Ext)
}

// WriteGolden writes an artifact and updates the manifest entry for it.
func WriteGolden(root string, c Case, artifact []byte, m *Manifest) error {
	path := GoldenPath(root, c)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, artifact, 0o644); err != nil {
		return err
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return err
	}
	m.Entries[c.Name] = ManifestEntry{
		File:   filepath.ToSlash(rel),
		SHA256: digestHex(artifact),
		Bytes:  len(artifact),
	}
	return nil
}

// ReadGolden loads the committed artifact for a case and checks it against the
// manifest, so a golden edited in place is caught before it is compared.
func ReadGolden(root string, c Case, m *Manifest) ([]byte, error) {
	entry, ok := m.Entries[c.Name]
	if !ok {
		return nil, fmt.Errorf("case %q has no golden; run the suite with -update to create one", c.Name)
	}
	raw, err := os.ReadFile(filepath.Join(root, entry.File))
	if err != nil {
		return nil, err
	}
	if got := digestHex(raw); got != entry.SHA256 {
		return nil, fmt.Errorf(
			"golden for %q does not match its manifest digest (recorded %s, actual %s); it has been hand-edited",
			c.Name, entry.SHA256, got)
	}
	return raw, nil
}

// OrphanGoldens returns manifest entries with no corresponding case, so a
// renamed case does not leave a stale artifact behind claiming to be evidence.
func OrphanGoldens(m *Manifest, cases []Case) []string {
	live := make(map[string]bool, len(cases))
	for _, c := range cases {
		live[c.Name] = true
	}
	var out []string
	for name := range m.Entries {
		if !live[name] {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}
