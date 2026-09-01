package theme

import (
	_ "embed"
	"sync"

	"github.com/frankbardon/vellum/spec"
)

// builtinJSON is the theme Vellum ships, embedded rather than constructed in
// Go.
//
// It is a document because every theme is a document — including this one. A
// built-in expressed as Go literals would be a second way of saying what a
// theme is, free to drift from the way every other theme is said, and it would
// exercise none of the decoding path a consumer's theme goes through. Shipping
// it as JSON means the default theme is the first test of the theme reader.
//
//go:embed builtin.json
var builtinJSON []byte

var (
	builtinOnce sync.Once
	builtin     *Theme
	builtinErr  error
)

// Builtin returns the theme Vellum ships.
//
// The returned value is a fresh copy on every call: a caller that mutated a
// shared built-in would change what every later render means, and the failure
// would surface a long way from the mutation.
func Builtin() (*Theme, error) {
	builtinOnce.Do(func() {
		builtin, builtinErr = Decode(builtinJSON)
	})
	if builtinErr != nil {
		return nil, builtinErr
	}
	return builtin.Clone(), nil
}

// BuiltinJSON returns the built-in theme's source document.
//
// Exposed so a consumer authoring their own theme can start from a document
// that is known to validate, rather than from prose about one.
func BuiltinJSON() []byte { return append([]byte(nil), builtinJSON...) }

// Clone returns a deep copy.
func (t *Theme) Clone() *Theme {
	if t == nil {
		return nil
	}
	out := *t
	out.Fonts = append([]Font(nil), t.Fonts...)
	out.Colors = append([]Color(nil), t.Colors...)
	out.Marks = append([]MarkStyle(nil), t.Marks...)
	out.Type.Headings = append([]spec.Length(nil), t.Type.Headings...)
	out.Layouts = make([]Layout, len(t.Layouts))
	for i := range t.Layouts {
		out.Layouts[i] = t.Layouts[i]
		out.Layouts[i].Boxes = append([]Box(nil), t.Layouts[i].Boxes...)
	}
	return &out
}
