package cli

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/frankbardon/vellum/asset"
	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/pdf/color"
	"github.com/frankbardon/vellum/theme"
	"github.com/urfave/cli/v3"
)

// DoctorCheck is one diagnostic doctor ran: a name, whether it passed, and a
// one-line human-readable detail. It is the unit both the --json envelope's
// data and the human table are built from, so the two presentations can never
// name a different set of checks or disagree about which passed.
type DoctorCheck struct {
	// Name identifies the check, dot-separated and stable across releases —
	// a consumer scripting against --json can key off it.
	Name string `json:"name"`

	// OK reports whether the check passed.
	OK bool `json:"ok"`

	// Detail is one line of human-readable context: what was found, or what
	// is wrong.
	Detail string `json:"detail"`
}

// DoctorReport is every check doctor ran, plus the overall verdict.
type DoctorReport struct {
	// OK is the AND of every check's own OK. A single failing check makes
	// the whole report not OK, but every check still runs and is reported —
	// doctor never stops at the first failure, so one broken theme does not
	// hide an unwritable output directory behind it.
	OK bool `json:"ok"`

	// Checks is every diagnostic doctor ran, in a fixed declaration order
	// (never a map range, per CLAUDE.md's determinism conventions), so the
	// same environment produces the same ordering of rows on every run.
	Checks []DoctorCheck `json:"checks"`
}

// failedNames returns the Name of every check that did not pass, in the
// order Checks carries them.
func (r *DoctorReport) failedNames() []string {
	if r == nil {
		return nil
	}
	var out []string
	for _, c := range r.Checks {
		if !c.OK {
			out = append(out, c.Name)
		}
	}
	return out
}

// Table renders the report as rows of strings for [printTable]: a header
// row, then one row per check in [DoctorReport.Checks]'s own order. A nil
// receiver still returns the header row so a caller need not nil-check
// before ranging the result — the same convention
// [template.InspectReport.AnchorsTable] establishes.
func (r *DoctorReport) Table() [][]string {
	rows := [][]string{{"Check", "Status", "Detail"}}
	if r == nil {
		return rows
	}
	for _, c := range r.Checks {
		status := "ok"
		if !c.OK {
			status = "FAIL"
		}
		rows = append(rows, []string{c.Name, status, c.Detail})
	}
	return rows
}

func newDoctorCommand() *cli.Command {
	return &cli.Command{
		Name:  "doctor",
		Usage: "check the local environment: the built-in theme, its fonts, the theme/asset directory env vars, the ICC profile and write permissions",
		Description: "Runs every diagnostic Vellum can usefully perform against the local\n" +
			"environment without shelling out or reaching the network — Vellum runs no\n" +
			"subprocess and does no I/O over the network, so there is no converter\n" +
			"binary to probe for. Every check runs regardless of whether an earlier one\n" +
			"failed, and every one is reported; --json's data.ok is the AND of all of\n" +
			"them, for a script to gate on.",
		Flags: []cli.Flag{
			jsonFlag,
			&cli.StringFlag{Name: "dir", Usage: "directory to check for write permission; defaults to the current working directory"},
		},
		Action: runDoctor,
	}
}

func runDoctor(ctx context.Context, cmd *cli.Command) error {
	asJSON := cmd.Bool("json")

	report := runDoctorChecks(cmd.String("dir"))

	var opErr error
	if !report.OK {
		opErr = verr.NewCodedErrorWithDetails(verr.VELLUM_CLI_DOCTOR_FAILED,
			"one or more doctor checks reported a problem",
			map[string]any{"failed": report.failedNames()})
	}

	if asJSON {
		if werr := writeEnvelopeWithError(cmd.Writer, report, opErr); werr != nil {
			return failErr(werr)
		}
		if opErr != nil {
			return failErr(opErr)
		}
		return nil
	}

	printTable(cmd.Writer, report.Table())
	if opErr != nil {
		printHumanError(cmd.ErrWriter, opErr)
		return failErr(opErr)
	}
	return nil
}

// runDoctorChecks runs every diagnostic and assembles the report. dir is the
// --dir flag's value; empty selects the current working directory for the
// write-permission check.
func runDoctorChecks(dir string) *DoctorReport {
	var checks []DoctorCheck

	th, themeCheck := checkBuiltinTheme()
	checks = append(checks, themeCheck)
	checks = append(checks, checkThemeFonts(th)...)

	checks = append(checks, checkDirEnv("VELLUM_THEME_DIR",
		"unset means the built-in theme only — no directory-backed theme.Provider reads this path today"))
	checks = append(checks, checkDirEnv("VELLUM_ASSET_DIR",
		"unset means inline assets only — no directory-backed asset.Resolver reads this path today"))

	checks = append(checks, checkMaxAssetBytesEnv())
	checks = append(checks, checkSourceDateEpochEnv())

	checks = append(checks, checkICCProfile())

	checks = append(checks, checkWritePermissions(dir))

	ok := true
	for _, c := range checks {
		if !c.OK {
			ok = false
			break
		}
	}
	return &DoctorReport{OK: ok, Checks: checks}
}

// checkBuiltinTheme validates the built-in theme via [theme.Builtin], which
// decodes and validates it internally. The returned *theme.Theme is nil when
// the check failed, so [checkThemeFonts] has nothing further to report.
func checkBuiltinTheme() (*theme.Theme, DoctorCheck) {
	th, err := theme.Builtin()
	if err != nil {
		return nil, DoctorCheck{Name: "theme.builtin", OK: false, Detail: err.Error()}
	}
	return th, DoctorCheck{
		Name: "theme.builtin",
		OK:   true,
		Detail: fmt.Sprintf("theme %q validated: %d font(s), %d colour(s), %d layout(s)",
			th.ID, len(th.Fonts), len(th.Colors), len(th.Layouts)),
	}
}

// checkThemeFonts reports one check per font the built-in theme declares:
// its family, role, embed mode, and whether Embeddable holds whenever Embed
// is not the zero value. That combination — a non-embeddable font
// nonetheless declaring an embed mode — is exactly what
// [theme.Theme.Validate] rejects as VELLUM_THEME_INVALID; doctor surfaces
// the same check as a diagnostic rather than inventing a new one. Because
// [theme.Builtin] already validates before returning, this can only ever
// report a passing built-in theme — the check exists so a theme swapped in
// later (once a directory-backed [theme.Provider] exists) is covered by the
// same diagnostic without doctor needing to change.
//
// th's Fonts is ranged directly rather than through a lookup, which is safe
// here: it is a slice in the theme document's own declared order, never a
// map, so this stays deterministic.
func checkThemeFonts(th *theme.Theme) []DoctorCheck {
	if th == nil {
		return nil
	}
	out := make([]DoctorCheck, 0, len(th.Fonts))
	for _, f := range th.Fonts {
		embed := string(f.Embed)
		if embed == "" {
			embed = "auto"
		}
		ok := f.Embeddable || f.Embed == theme.EmbedAuto
		detail := fmt.Sprintf("family=%q role=%s embed=%s embeddable=%t", f.Family, f.Role, embed, f.Embeddable)
		if !ok {
			detail += " — a non-embeddable font must not declare an embed mode (VELLUM_THEME_INVALID)"
		}
		out = append(out, DoctorCheck{Name: "theme.font." + string(f.Role), OK: ok, Detail: detail})
	}
	return out
}

// checkDirEnv reports whether the directory named by the environment
// variable name looks usable: it exists, is a directory, and is readable.
//
// It deliberately stops there. No directory-backed [theme.Provider] or
// [asset.Resolver] exists anywhere in this codebase today that would
// actually read this path — this check reports on the directory a future
// consumer might point one at, the same way a diagnostic tool reports on an
// optional dependency without implementing the feature that would consume
// it.
func checkDirEnv(name, unsetDetail string) DoctorCheck {
	checkName := "env." + strings.ToLower(name)
	val := os.Getenv(name)
	if val == "" {
		return DoctorCheck{Name: checkName, OK: true, Detail: unsetDetail}
	}
	info, err := os.Stat(val)
	if err != nil {
		return DoctorCheck{Name: checkName, OK: false, Detail: fmt.Sprintf("%s=%q: %v", name, val, err)}
	}
	if !info.IsDir() {
		return DoctorCheck{Name: checkName, OK: false, Detail: fmt.Sprintf("%s=%q is not a directory", name, val)}
	}
	if _, err := os.ReadDir(val); err != nil {
		return DoctorCheck{Name: checkName, OK: false, Detail: fmt.Sprintf("%s=%q is not readable: %v", name, val, err)}
	}
	return DoctorCheck{Name: checkName, OK: true, Detail: fmt.Sprintf("%s=%q exists and is readable", name, val)}
}

// checkMaxAssetBytesEnv reports whether VELLUM_MAX_ASSET_BYTES, when set,
// parses as a positive int64 — the type [asset.Options.MaxBytes] itself
// carries. Nothing in this codebase reads this variable from the
// environment today; this check is syntactic only, the same honest limit
// [checkSourceDateEpochEnv] states for its own variable.
func checkMaxAssetBytesEnv() DoctorCheck {
	const name = "VELLUM_MAX_ASSET_BYTES"
	checkName := "env." + strings.ToLower(name)
	val := os.Getenv(name)
	if val == "" {
		return DoctorCheck{Name: checkName, OK: true,
			Detail: fmt.Sprintf("not set; default is %d bytes (asset.DefaultMaxBytes)", asset.DefaultMaxBytes)}
	}
	n, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return DoctorCheck{Name: checkName, OK: false, Detail: fmt.Sprintf("%s=%q does not parse as an integer: %v", name, val, err)}
	}
	if n <= 0 {
		return DoctorCheck{Name: checkName, OK: false, Detail: fmt.Sprintf("%s=%q must be positive", name, val)}
	}
	return DoctorCheck{Name: checkName, OK: true, Detail: fmt.Sprintf("%s=%d bytes", name, n)}
}

// checkSourceDateEpochEnv reports whether VELLUM_SOURCE_DATE_EPOCH, when
// set, is a well-formed RFC 3339 timestamp — the form CLAUDE.md's "Build /
// Env" documents.
//
// No code path in this codebase reads this variable from the environment
// today: [vellum.Options.SourceDateEpoch] is set by the embedder in Go, not
// parsed from this variable by anything under internal/cli or vellum.go.
// This check is therefore syntactic well-formedness only, and says so in its
// own detail, rather than claiming to confirm the epoch that would actually
// be used — inventing that wiring is a separate, larger change outside this
// story's scope.
func checkSourceDateEpochEnv() DoctorCheck {
	const name = "VELLUM_SOURCE_DATE_EPOCH"
	checkName := "env." + strings.ToLower(name)
	val := os.Getenv(name)
	if val == "" {
		return DoctorCheck{Name: checkName, OK: true, Detail: "not set; the pinned 1980 epoch is used"}
	}
	if _, err := time.Parse(time.RFC3339, val); err != nil {
		return DoctorCheck{Name: checkName, OK: false, Detail: fmt.Sprintf("%s=%q is not a well-formed RFC3339 timestamp: %v", name, val, err)}
	}
	return DoctorCheck{Name: checkName, OK: true,
		Detail: fmt.Sprintf("%s=%q is a well-formed RFC3339 timestamp (no code path reads this variable from the environment today)", name, val)}
}

// checkICCProfile confirms [color.SRGBProfile] returns non-empty bytes
// without panicking — the whole of what can go wrong with a profile that is
// built from constants rather than read from a file. The recover turns an
// unexpected panic into a failed check rather than a doctor invocation that
// crashes the process it was asked to diagnose.
func checkICCProfile() (check DoctorCheck) {
	const checkName = "icc.srgb_profile"
	defer func() {
		if r := recover(); r != nil {
			check = DoctorCheck{Name: checkName, OK: false, Detail: fmt.Sprintf("panicked: %v", r)}
		}
	}()
	profile := color.SRGBProfile()
	if len(profile) == 0 {
		return DoctorCheck{Name: checkName, OK: false, Detail: "SRGBProfile() returned no bytes"}
	}
	return DoctorCheck{Name: checkName, OK: true, Detail: fmt.Sprintf("%d bytes", len(profile))}
}

// checkWritePermissions reports whether dir — or, when empty, the current
// working directory — is actually writable, by creating and immediately
// removing a temporary file there. A mode-bit check is not enough:
// permission bits lie under enough real filesystem and ACL configurations
// that an actual write attempt is the only check worth trusting.
func checkWritePermissions(dir string) DoctorCheck {
	const checkName = "write.permissions"
	target := dir
	if target == "" {
		wd, err := os.Getwd()
		if err != nil {
			return DoctorCheck{Name: checkName, OK: false, Detail: fmt.Sprintf("could not determine the working directory: %v", err)}
		}
		target = wd
	}
	f, err := os.CreateTemp(target, ".vellum-doctor-*")
	if err != nil {
		return DoctorCheck{Name: checkName, OK: false, Detail: fmt.Sprintf("%s is not writable: %v", target, err)}
	}
	name := f.Name()
	f.Close()
	if err := os.Remove(name); err != nil {
		return DoctorCheck{Name: checkName, OK: false, Detail: fmt.Sprintf("%s: created a temp file but could not remove it: %v", target, err)}
	}
	return DoctorCheck{Name: checkName, OK: true, Detail: fmt.Sprintf("%s is writable", target)}
}
