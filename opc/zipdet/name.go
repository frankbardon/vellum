package zipdet

import (
	"strings"

	verr "github.com/frankbardon/vellum/errors"
)

// validateEntryName rejects archive entry names that are unsafe or ambiguous.
//
// The rules are deliberately strict and the failures are refusals rather than
// sanitisations. A traversal segment in an entry name is an attack against any
// consumer that extracts a package to disk, and "clean it up and carry on" is
// how a sanitiser becomes a bypass — the caller learns nothing and the
// resulting archive claims to be something it is not.
func validateEntryName(name string) error {
	fail := func(reason string) error {
		return verr.NewCodedErrorWithDetails(verr.VELLUM_ZIP_ENTRY_NAME_INVALID,
			"archive entry name is not valid",
			map[string]any{"entry_name": name, "reason": reason})
	}

	switch {
	case name == "":
		return fail("empty")
	case strings.HasPrefix(name, "/"):
		return fail("absolute path")
	case strings.Contains(name, `\`):
		return fail("backslash separator")
	case strings.Contains(name, "\x00"):
		return fail("null byte")
	}

	// A Windows drive-letter prefix ("C:") is absolute on the platform that
	// matters most for OOXML consumers, even though it does not start with a
	// slash.
	if len(name) >= 2 && name[1] == ':' {
		return fail("drive-letter prefix")
	}

	for _, seg := range strings.Split(name, "/") {
		switch seg {
		case "":
			// An empty segment is an interior "//" or a trailing slash. A
			// trailing slash would mean a directory entry, which Vellum never
			// writes: OPC packages carry no directory entries, and accepting
			// one here would let a package declare a part that is not one.
			return fail("empty path segment")
		case ".":
			return fail("current-directory segment")
		case "..":
			return fail("parent-directory segment")
		}
	}
	return nil
}
