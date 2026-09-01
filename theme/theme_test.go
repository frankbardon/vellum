package theme_test

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"strings"
	"testing"

	"github.com/frankbardon/vellum/artifact"
	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/spec"
	"github.com/frankbardon/vellum/theme"
)

func builtin(t *testing.T) *theme.Theme {
	t.Helper()
	th, err := theme.Builtin()
	if err != nil {
		t.Fatalf("Builtin: %v", err)
	}
	return th
}

// TestBuiltin_Validates is the first test of the theme reader, because the
// built-in theme goes through the same decode path a consumer's theme does.
func TestBuiltin_Validates(t *testing.T) {
	th := builtin(t)
	if th.ID != theme.BuiltinID {
		t.Errorf("ID = %q, want %q", th.ID, theme.BuiltinID)
	}
	if err := th.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

// TestBuiltin_IsACopy pins that a caller cannot reach through the built-in
// theme and change what every later render means.
func TestBuiltin_IsACopy(t *testing.T) {
	a := builtin(t)
	a.Colors[0].Value = "000000"
	a.Layouts[0].Boxes[0].Width = spec.Inches(99)

	b := builtin(t)
	if b.Colors[0].Value == "000000" {
		t.Error("mutating a returned theme changed the built-in one")
	}
	if b.Layouts[0].Boxes[0].Width.Value == 99 {
		t.Error("mutating a returned layout's boxes changed the built-in one")
	}
}

// TestBuiltin_CoversEveryDeclaredRole is the completeness claim: every role in
// every registry is declared, so a document cannot reach for one that is
// missing. Written by cardinality rather than by inspection, so adding a role
// fails here until the built-in theme declares it.
func TestBuiltin_CoversEveryDeclaredRole(t *testing.T) {
	th := builtin(t)

	for _, role := range theme.AllColorRoles() {
		if _, ok := th.LookupColor(role); !ok {
			t.Errorf("the built-in theme declares no colour for role %q", role)
		}
	}
	for _, role := range theme.AllFontRoles() {
		if _, ok := th.LookupFont(role); !ok {
			t.Errorf("the built-in theme declares no font for role %q", role)
		}
	}
}

// TestBoxes_AnswerableBeforeASpecExists is the property the layout query is
// for: a host can enumerate its render presets from a theme alone.
func TestBoxes_AnswerableBeforeASpecExists(t *testing.T) {
	th := builtin(t)

	for _, tc := range []struct {
		format artifact.Format
		want   []theme.BoxRole
	}{
		{artifact.FormatDOCX, []theme.BoxRole{theme.BoxAssetFull, theme.BoxAssetHalf, theme.BoxAssetQuarter, theme.BoxLogo}},
		{artifact.FormatPPTX, []theme.BoxRole{theme.BoxAssetFull, theme.BoxAssetHalf, theme.BoxAssetQuarter, theme.BoxLogo}},
		{artifact.FormatPDF, []theme.BoxRole{theme.BoxAssetFull, theme.BoxAssetHalf, theme.BoxAssetQuarter, theme.BoxLogo}},
		// xlsx declares none, and that is the honest answer rather than an
		// omission: the matrix rejects assets in a workbook, so a theme
		// offering an asset box for one would be promising a slot no render
		// can fill.
		{artifact.FormatXLSX, nil},
	} {
		got := th.Boxes(tc.format).Roles()
		if len(got) != len(tc.want) {
			t.Errorf("Boxes(%s) = %v, want %v", tc.format, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("Boxes(%s) = %v, want %v", tc.format, got, tc.want)
				break
			}
		}
	}
}

// TestBoxes_SortedAndStable pins the ordering guarantee. The result unions
// several layouts, which have no shared declaration order, so it is sorted —
// and a caller keying a render cache off it needs the same list every time.
func TestBoxes_SortedAndStable(t *testing.T) {
	th := builtin(t)
	first := th.Boxes(artifact.FormatPPTX)

	for range 50 {
		got := th.Boxes(artifact.FormatPPTX)
		if len(got) != len(first) {
			t.Fatalf("Boxes returned %d boxes, then %d", len(first), len(got))
		}
		for i := range got {
			if got[i] != first[i] {
				t.Fatalf("Boxes disagreed at index %d: %+v then %+v", i, first[i], got[i])
			}
		}
	}
	for i := 1; i < len(first); i++ {
		if first[i-1].Role >= first[i].Role {
			t.Errorf("boxes are not sorted by role: %q then %q", first[i-1].Role, first[i].Role)
		}
	}
}

// TestBoxes_FlowingFormatsTakeIntrinsicHeight is the distinction the Height
// field's zero value carries: a slide declares a rectangle, a page declares a
// column and lets the asset's aspect ratio decide the rest.
func TestBoxes_FlowingFormatsTakeIntrinsicHeight(t *testing.T) {
	th := builtin(t)

	docx, ok := th.Boxes(artifact.FormatDOCX).Lookup(theme.BoxAssetFull)
	if !ok {
		t.Fatal("docx declares no asset.full box")
	}
	if !docx.IntrinsicHeight() {
		t.Errorf("docx asset.full declares height %v; a flowing format has no basis to impose one", docx.Height)
	}

	pptx, ok := th.Boxes(artifact.FormatPPTX).Lookup(theme.BoxAssetFull)
	if !ok {
		t.Fatal("pptx declares no asset.full box")
	}
	if pptx.IntrinsicHeight() {
		t.Error("pptx asset.full has an intrinsic height; a slide is fixed geometry and must declare both dimensions")
	}
}

// TestBoxes_FullBoxMatchesTheContentColumn checks the arithmetic actually
// holds, rather than trusting the JSON to be self-consistent. A full-width box
// that is not the content width is the exact defect the layout query exists to
// prevent, expressed in the theme instead of at the call site.
func TestBoxes_FullBoxMatchesTheContentColumn(t *testing.T) {
	th := builtin(t)

	for _, format := range []artifact.Format{artifact.FormatDOCX, artifact.FormatPDF, artifact.FormatPPTX} {
		layout, err := th.LayoutFor(format, "")
		if err != nil {
			t.Fatalf("LayoutFor(%s): %v", format, err)
		}
		want, err := layout.Page.ContentWidth()
		if err != nil {
			t.Fatalf("ContentWidth(%s): %v", format, err)
		}
		box, err := layout.BoxFor(theme.BoxAssetFull)
		if err != nil {
			t.Fatalf("BoxFor(%s): %v", format, err)
		}
		got, err := box.Width.EMU()
		if err != nil {
			t.Fatalf("box width EMU(%s): %v", format, err)
		}
		// A tolerance of one point, because the theme is authored in round
		// millimetres and inches rather than in EMU.
		const pointEMU = spec.EMUPerInch / 72
		if diff := got - want; diff > pointEMU || diff < -pointEMU {
			t.Errorf("%s asset.full is %d EMU, content column is %d EMU (differ by %d)",
				format, got, want, diff)
		}
	}
}

func TestLayoutFor_UnknownIDIsAnError(t *testing.T) {
	th := builtin(t)

	_, err := th.LayoutFor(artifact.FormatDOCX, "no-such-layout")
	if !verr.HasCode(err, verr.VELLUM_THEME_LAYOUT_NOT_FOUND) {
		t.Fatalf("error = %v, want VELLUM_THEME_LAYOUT_NOT_FOUND", err)
	}

	var coded *verr.CodedError
	if !stderrors.As(err, &coded) {
		t.Fatal("error is not a CodedError")
	}
	// The available set is in the details so a caller can act without parsing
	// prose, and the format is there because a theme may legitimately carry a
	// layout for one format and not another.
	if coded.Details["format"] != string(artifact.FormatDOCX) {
		t.Errorf("details format = %v, want %q", coded.Details["format"], artifact.FormatDOCX)
	}
	if _, ok := coded.Details["available"]; !ok {
		t.Error("details carry no available set")
	}
}

func TestLayoutFor_EmptyIDSelectsTheDefault(t *testing.T) {
	th := builtin(t)

	l, err := th.LayoutFor(artifact.FormatPPTX, "")
	if err != nil {
		t.Fatalf("LayoutFor: %v", err)
	}
	if !l.Default {
		t.Errorf("empty id resolved to %q, which is not the default layout", l.ID)
	}
	// pptx carries a second, non-default layout, so this is a real choice
	// rather than the only candidate.
	if named, err := th.LayoutFor(artifact.FormatPPTX, "title"); err != nil {
		t.Fatalf("LayoutFor(title): %v", err)
	} else if named.ID == l.ID {
		t.Error("the named and default pptx layouts are the same; the test proves nothing")
	}
}

func TestBoxFor_UnknownRoleIsAnError(t *testing.T) {
	th := builtin(t)
	l, err := th.LayoutFor(artifact.FormatDOCX, "")
	if err != nil {
		t.Fatalf("LayoutFor: %v", err)
	}
	if _, err := l.BoxFor("asset.sixteenth"); !verr.HasCode(err, verr.VELLUM_THEME_BOX_NOT_FOUND) {
		t.Fatalf("error = %v, want VELLUM_THEME_BOX_NOT_FOUND", err)
	}
	if _, err := l.BoxFor(""); err != nil {
		t.Errorf("an empty role must select the default box, got %v", err)
	}
}

func TestDecode_RejectsUnknownFields(t *testing.T) {
	src := strings.Replace(string(theme.BuiltinJSON()), `"name": "Vellum Default",`,
		`"name": "Vellum Default", "brand_colour": "FF0000",`, 1)

	_, err := theme.Decode([]byte(src))
	if !verr.HasCode(err, verr.VELLUM_THEME_INVALID) {
		t.Fatalf("error = %v, want VELLUM_THEME_INVALID", err)
	}
	var coded *verr.CodedError
	if stderrors.As(err, &coded) && coded.Details["field"] != "brand_colour" {
		t.Errorf("details field = %v, want brand_colour", coded.Details["field"])
	}
}

func TestDecode_YAMLAndJSONAgree(t *testing.T) {
	asYAML := "format_version: \"1.0\"\nid: acme\n"
	// Only the round-trip property is under test, so both forms are the same
	// minimal document and both are expected to fail validation the same way.
	fromYAML, errYAML := theme.DecodeYAML([]byte(asYAML))
	fromJSON, errJSON := theme.Decode([]byte(`{"format_version":"1.0","id":"acme"}`))

	if (errYAML == nil) != (errJSON == nil) {
		t.Fatalf("YAML and JSON disagreed: %v vs %v", errYAML, errJSON)
	}
	if errYAML != nil && errYAML.Error() != errJSON.Error() {
		t.Errorf("YAML and JSON produced different errors:\n  %v\n  %v", errYAML, errJSON)
	}
	if fromYAML != nil && fromJSON != nil && fromYAML.ID != fromJSON.ID {
		t.Errorf("ids differ: %q vs %q", fromYAML.ID, fromJSON.ID)
	}
}

// TestDecode_YAMLBareScalarsAreYAML11 documents a trap rather than a Vellum
// behaviour, and pins it so nobody has to rediscover it.
//
// YAML 1.1 resolves bare y, n, yes, no, on and off to booleans, so a theme id
// written unquoted as `id: y` reaches the JSON decoder as `true` and fails as a
// type mismatch. This is the Norway problem, and it is inherited from routing
// YAML through JSON — which is the reason for that dependency, not an accident
// of it, since it is the only way to guarantee the two forms mean the same
// thing.
//
// Vellum does not paper over it. Quoting the scalar is the fix, and a loud
// failure is a better teacher than a silent coercion.
func TestDecode_YAMLBareScalarsAreYAML11(t *testing.T) {
	bare, errBare := theme.DecodeYAML([]byte("format_version: \"1.0\"\nid: y\n"))
	if errBare == nil {
		t.Fatalf("bare `id: y` decoded to %+v; YAML 1.1 makes it a boolean and it must fail", bare)
	}
	if !verr.HasCode(errBare, verr.VELLUM_THEME_INVALID) {
		t.Errorf("error = %v, want VELLUM_THEME_INVALID", errBare)
	}

	// Quoted, the same document reaches validation like any other.
	quoted, errQuoted := theme.DecodeYAML([]byte("format_version: \"1.0\"\nid: \"y\"\n"))
	if verr.HasCode(errQuoted, verr.VELLUM_THEME_INVALID) && quoted == nil {
		var coded *verr.CodedError
		if stderrors.As(errQuoted, &coded) && coded.Details["field"] != nil {
			t.Errorf("quoted id still failed as a decode fault: %v", errQuoted)
		}
	}
}

// validationCase mutates a copy of the built-in document and states the code
// the result must carry. Driving these off the shipped document rather than off
// hand-built structs means each case differs from a valid theme in exactly one
// way.
type validationCase struct {
	name string
	code verr.Code
	edit func(map[string]any)
}

func TestValidate_RejectsIncompleteThemes(t *testing.T) {
	cases := []validationCase{
		{"missing colour role", verr.VELLUM_THEME_INVALID, func(m map[string]any) {
			colors := m["colors"].([]any)
			m["colors"] = colors[1:]
		}},
		{"missing font role", verr.VELLUM_THEME_INVALID, func(m map[string]any) {
			fonts := m["fonts"].([]any)
			m["fonts"] = fonts[1:]
		}},
		{"lowercase colour value", verr.VELLUM_THEME_INVALID, func(m map[string]any) {
			m["colors"].([]any)[0].(map[string]any)["value"] = "ffffff"
		}},
		{"colour value with a hash", verr.VELLUM_THEME_INVALID, func(m map[string]any) {
			m["colors"].([]any)[0].(map[string]any)["value"] = "#FFFFFF"
		}},
		{"unknown colour role", verr.VELLUM_THEME_INVALID, func(m map[string]any) {
			m["colors"].([]any)[0].(map[string]any)["role"] = "sidebar"
		}},
		{"no default layout for a covered format", verr.VELLUM_THEME_INVALID, func(m map[string]any) {
			delete(m["layouts"].([]any)[0].(map[string]any), "default")
		}},
		{"two default layouts for one format", verr.VELLUM_THEME_INVALID, func(m map[string]any) {
			m["layouts"].([]any)[3].(map[string]any)["default"] = true
		}},
		{"margins leave no content area", verr.VELLUM_THEME_INVALID, func(m map[string]any) {
			page := m["layouts"].([]any)[0].(map[string]any)["page"].(map[string]any)
			page["margin_left"] = map[string]any{"value": 200.0, "unit": "mm"}
		}},
		{"a box with no width", verr.VELLUM_THEME_INVALID, func(m map[string]any) {
			boxes := m["layouts"].([]any)[0].(map[string]any)["boxes"].([]any)
			delete(boxes[0].(map[string]any), "width")
		}},
		{"a mark naming an undeclared colour role", verr.VELLUM_THEME_INVALID, func(m map[string]any) {
			m["marks"].([]any)[0].(map[string]any)["color"] = "sidebar"
		}},
		{"a font that is neither embeddable nor substituted", verr.VELLUM_FONT_NOT_EMBEDDABLE, func(m map[string]any) {
			delete(m["fonts"].([]any)[0].(map[string]any), "substitute")
		}},
		{"a font declared embeddable with no handle", verr.VELLUM_FONT_UNAVAILABLE, func(m map[string]any) {
			f := m["fonts"].([]any)[0].(map[string]any)
			f["embeddable"] = true
			delete(f, "substitute")
		}},
		{"an unsupported format version", verr.VELLUM_THEME_INVALID, func(m map[string]any) {
			m["format_version"] = "0.9"
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var m map[string]any
			if err := json.Unmarshal(theme.BuiltinJSON(), &m); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			tc.edit(m)
			edited, err := json.Marshal(m)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if _, err := theme.Decode(edited); !verr.HasCode(err, tc.code) {
				t.Fatalf("error = %v, want %s", err, tc.code)
			}
		})
	}
}

func TestProvider_InertDefaultServesTheBuiltinTheme(t *testing.T) {
	ctx := context.Background()

	// A nil provider is the wire-nothing case, and it must work rather than
	// panic — that is what makes the inert default true at the call site.
	got, err := theme.Resolve(ctx, nil, "")
	if err != nil {
		t.Fatalf("Resolve(nil): %v", err)
	}
	if got.ID != theme.BuiltinID {
		t.Errorf("ID = %q, want %q", got.ID, theme.BuiltinID)
	}
}

func TestProvider_UnknownIDIsNotSilentlyDefaulted(t *testing.T) {
	_, err := theme.Resolve(context.Background(), nil, "acme")
	if !verr.HasCode(err, verr.VELLUM_THEME_NOT_FOUND) {
		t.Fatalf("error = %v, want VELLUM_THEME_NOT_FOUND", err)
	}
}

func TestStaticProvider_ServesRegisteredAndBuiltin(t *testing.T) {
	custom := builtin(t)
	custom.ID = "acme"
	custom.Colors[0].Value = "101010"

	p, err := theme.NewStaticProvider(custom)
	if err != nil {
		t.Fatalf("NewStaticProvider: %v", err)
	}

	ctx := context.Background()
	got, err := theme.Resolve(ctx, p, "acme")
	if err != nil {
		t.Fatalf("Resolve(acme): %v", err)
	}
	if v, _ := got.LookupColor(theme.ColorBackground); v != "101010" {
		t.Errorf("background = %q, want 101010", v)
	}

	// The built-in theme stays reachable through a provider that did not
	// register it, so wiring a provider never costs a host the default.
	if base, err := theme.Resolve(ctx, p, ""); err != nil {
		t.Fatalf("Resolve(\"\"): %v", err)
	} else if base.ID != theme.BuiltinID {
		t.Errorf("empty id resolved to %q", base.ID)
	}
}

// TestStaticProvider_ValidatesAtRegistration pins where a broken theme fails.
// Registration is a long way, in time and in blame, from whichever render first
// reached for the missing field.
func TestStaticProvider_ValidatesAtRegistration(t *testing.T) {
	broken := builtin(t)
	broken.ID = "broken"
	broken.Colors = broken.Colors[1:]

	if _, err := theme.NewStaticProvider(broken); !verr.HasCode(err, verr.VELLUM_THEME_INVALID) {
		t.Fatalf("error = %v, want VELLUM_THEME_INVALID", err)
	}
}

func TestHeadingSize_ClampsBeyondTheScale(t *testing.T) {
	th := builtin(t)
	last := th.Type.Headings[len(th.Type.Headings)-1]

	// An outline deeper than the theme anticipated is a document that should
	// still render, so the deepest declared size is reused rather than failing.
	if got := th.Type.HeadingSize(99); got != last {
		t.Errorf("HeadingSize(99) = %v, want %v", got, last)
	}
	if got := th.Type.HeadingSize(1); got != th.Type.Headings[0] {
		t.Errorf("HeadingSize(1) = %v, want %v", got, th.Type.Headings[0])
	}
	if got := th.Type.HeadingSize(0); got != th.Type.Headings[0] {
		t.Errorf("HeadingSize(0) = %v, want %v", got, th.Type.Headings[0])
	}
}
