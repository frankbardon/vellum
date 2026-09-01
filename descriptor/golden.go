package descriptor

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// GoldenHashPrefix marks the trailer line pinning a golden's content.
//
// Goldens are evidence. Evidence that can be quietly adjusted to match new
// output is not evidence, so the trailer makes a hand edit detectable and the
// regeneration path is an explicit -update run.
const GoldenHashPrefix = "// golden-hash: "

// RenderGolden serialises v as indented JSON with a hash trailer.
//
// Indented rather than compact because these files are read by people in
// review, and a one-line diff on a 40 KB single line is not a diff anybody can
// read. encoding/json sorts object keys, so the output is deterministic
// regardless.
func RenderGolden(v any) ([]byte, error) {
	var body []byte
	var err error

	if raw, ok := v.(json.RawMessage); ok {
		var buf bytes.Buffer
		if err := json.Indent(&buf, raw, "", "  "); err != nil {
			return nil, err
		}
		body = buf.Bytes()
	} else {
		body, err = json.MarshalIndent(v, "", "  ")
		if err != nil {
			return nil, err
		}
	}
	body = append(body, '\n')

	sum := sha256.Sum256(body)
	return append(body, []byte(GoldenHashPrefix+hex.EncodeToString(sum[:])+"\n")...), nil
}

// SplitGolden separates a golden's body from its recorded hash and verifies
// them against each other.
func SplitGolden(raw []byte) ([]byte, error) {
	s := string(raw)
	i := strings.LastIndex(s, GoldenHashPrefix)
	if i < 0 {
		return nil, fmt.Errorf("the golden has no %q trailer; regenerate it with -update rather than editing it",
			strings.TrimSpace(GoldenHashPrefix))
	}
	body := []byte(s[:i])
	recorded := strings.TrimSpace(s[i+len(GoldenHashPrefix):])

	sum := sha256.Sum256(body)
	if got := hex.EncodeToString(sum[:]); got != recorded {
		return nil, fmt.Errorf("the golden has been hand-edited: recorded hash %s, actual %s. Goldens are regenerated with -update, never edited",
			recorded, got)
	}
	return body, nil
}
