package opc

import (
	"strings"

	verr "github.com/frankbardon/vellum/errors"
)

// ContentTypesName is the archive entry holding the content-type declarations.
// It is not an OPC part — it has no part name of its own and is not addressable
// by a relationship — which is why it is a plain constant here rather than a
// [Part].
const ContentTypesName = "[Content_Types].xml"

// RootRelsName is the package-level relationships part.
const RootRelsName = "/_rels/.rels"

// ValidatePartName reports whether name is a well-formed OPC part name.
//
// Part names are absolute and forward-slashed: "/word/document.xml". The
// leading slash is what distinguishes a part name from the archive entry name
// it maps to, and keeping the two spellings distinct throughout is deliberate —
// conflating them is how a relationship target comes to resolve against the
// wrong base.
func ValidatePartName(name string) error {
	fail := func(reason string) error {
		return verr.NewCodedErrorWithDetails(verr.VELLUM_OPC_PART_NAME_INVALID,
			"part name is not a valid OPC part name",
			map[string]any{"part_name": name, "reason": reason})
	}

	switch {
	case name == "":
		return fail("empty")
	case !strings.HasPrefix(name, "/"):
		return fail("not absolute")
	case strings.HasSuffix(name, "/"):
		return fail("trailing slash")
	case strings.Contains(name, `\`):
		return fail("backslash separator")
	case strings.Contains(name, "\x00"):
		return fail("null byte")
	}

	for _, seg := range strings.Split(strings.TrimPrefix(name, "/"), "/") {
		switch seg {
		case "":
			return fail("empty path segment")
		case ".":
			return fail("current-directory segment")
		case "..":
			return fail("parent-directory segment")
		}
	}
	return nil
}

// entryName converts an OPC part name to its archive entry name by dropping
// the leading slash.
func entryName(partName string) string {
	return strings.TrimPrefix(partName, "/")
}

// partName converts an archive entry name to its OPC part name by adding the
// leading slash.
func partName(entry string) string {
	if strings.HasPrefix(entry, "/") {
		return entry
	}
	return "/" + entry
}

// extension returns the lower-cased extension of a part name, without the dot,
// or the empty string when it has none.
func extension(name string) string {
	i := strings.LastIndexByte(name, '.')
	if i < 0 {
		return ""
	}
	if strings.IndexByte(name[i:], '/') >= 0 {
		return ""
	}
	return strings.ToLower(name[i+1:])
}

// IsRelsPart reports whether name addresses a relationships part.
func IsRelsPart(name string) bool {
	return extension(name) == "rels" && strings.Contains(name, "/_rels/")
}

// RelsNameFor returns the name of the relationships part belonging to owner.
//
// The package-level relationships part is a special case: its owner is the
// package itself rather than a part, and it lives at /_rels/.rels.
func RelsNameFor(owner string) string {
	if owner == "" || owner == "/" {
		return RootRelsName
	}
	i := strings.LastIndexByte(owner, '/')
	dir, base := owner[:i], owner[i+1:]
	return dir + "/_rels/" + base + ".rels"
}

// ownerOfRels is the inverse of RelsNameFor. The second result reports whether
// name was a relationships part at all.
func ownerOfRels(name string) (string, bool) {
	if name == RootRelsName {
		return "/", true
	}
	if !IsRelsPart(name) {
		return "", false
	}
	i := strings.LastIndex(name, "/_rels/")
	dir := name[:i]
	base := strings.TrimSuffix(name[i+len("/_rels/"):], ".rels")
	if base == "" {
		return "", false
	}
	return dir + "/" + base, true
}
