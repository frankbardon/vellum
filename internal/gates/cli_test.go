package gates

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/frankbardon/vellum/internal/cli"
)

// cliFlagsDocPath is the command index CLAUDE.md's Update Demand table
// requires a CLI leaf to be added to or removed from.
const cliFlagsDocPath = repoRoot + "/docs/src/cli/flags.md"

// commandIndexHeading is the section of cliFlagsDocPath that lists every
// verb, one table row each.
const commandIndexHeading = "## Command index"

// cliLeafRe pulls the verb name out of a command-index table row's first
// cell, which is always backtick-quoted and begins "vellum <name>" — e.g.
// "| `vellum compose [spec-file] --format <fmt> ...` | ..." yields "compose".
// The character class excludes the backtick as well as whitespace, so a
// no-argument command like "`vellum schema`" does not capture the closing
// backtick as part of the name.
var cliLeafRe = regexp.MustCompile("^\\|\\s*`vellum\\s+([^\\s`]+)")

// TestSkillsCoverAllCliLeaves asserts every *cli.Command internal/cli.New
// registers has a row in docs/src/cli/flags.md's command index, and that the
// index carries no row for a command that does not exist.
//
// Named with the TestSkillsCover reserved prefix, per CLAUDE.md's Update
// Demand table ("A CLI leaf (add/remove) | ... | TestSkillsCoverAllCliLeaves")
// and to sit alongside skills/skills_test.go's own TestSkillsCover* family by
// name. It lives in internal/gates rather than skills/ because what it checks
// is docs/src/cli/flags.md, not the skill pack: skills/skills_test.go's own
// gates all read skills.All(), and a test that reads a docs/ file instead
// belongs with this package's other CLAUDE.md-hygiene meta-tests, which
// already read CLAUDE.md and .github/workflows/ci.yml directly.
//
// This is the doc -> source direction: CLAUDE.md's own
// TestClaudeMdMentionsAllNonSkippableGates only checks that a gate named in
// source is documented, never that a CLI leaf named in source has its own
// promised documentation. Without this test, a leaf could be added or removed
// in internal/cli and the command index could simply never be told.
func TestSkillsCoverAllCliLeaves(t *testing.T) {
	live := liveCliLeaves(t)
	documented := documentedCliLeaves(t)

	for name := range live {
		if !documented[name] {
			t.Errorf("CLI leaf %q is registered in internal/cli.New but has no row in %s's command index.\n"+
				"Add one, per CLAUDE.md's Update Demand table.", name, cliFlagsDocPath)
		}
	}
	for name := range documented {
		if !live[name] {
			t.Errorf("%s's command index has a row for %q, which internal/cli.New does not register.\n"+
				"Either the command was removed and the row is stale, or the row names something "+
				"that was never a real leaf.", cliFlagsDocPath, name)
		}
	}
}

// liveCliLeaves returns the name of every top-level command internal/cli.New
// registers.
func liveCliLeaves(t *testing.T) map[string]bool {
	t.Helper()
	root := cli.New("gate-test")
	out := make(map[string]bool, len(root.Commands))
	for _, c := range root.Commands {
		if c.Name == "" {
			t.Fatalf("internal/cli.New registered a command with an empty Name")
		}
		out[c.Name] = true
	}
	if len(out) == 0 {
		t.Fatal("internal/cli.New registered no commands; this gate would pass vacuously")
	}
	return out
}

// documentedCliLeaves returns every command name the flags.md command index
// names, one per table row's leading "| `vellum <name>" cell.
//
// Restricted to lines that open a table row on purpose: the command index's
// own closing sentence ("Run `vellum <command> --help` for a command's exact
// flags...") also contains a backtick-quoted "vellum <something>" span, and
// without the row anchor that placeholder text is misread as a command named
// "<command>".
func documentedCliLeaves(t *testing.T) map[string]bool {
	t.Helper()
	raw, err := os.ReadFile(cliFlagsDocPath)
	if err != nil {
		t.Fatalf("reading %s: %v", cliFlagsDocPath, err)
	}
	section := sectionOf(t, string(raw), commandIndexHeading)

	out := make(map[string]bool)
	for _, line := range strings.Split(section, "\n") {
		m := cliLeafRe.FindStringSubmatch(strings.TrimSpace(line))
		if m != nil {
			out[m[1]] = true
		}
	}
	if len(out) == 0 {
		t.Fatalf("%s's %q section named no commands; the regex or the heading is wrong "+
			"and this gate would pass vacuously", cliFlagsDocPath, commandIndexHeading)
	}
	return out
}
