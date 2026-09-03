package bind_test

// TestReconcile_* exercise E10-S4's pre-flight reconciliation between a
// template's discovered anchor.Inventory and a binding's statement tree —
// FR-F6's "error on anchors present in the template but absent from the
// binding, and the reverse, unless explicitly marked optional" — checked
// structurally, before any FEEL expression is evaluated.

import (
	"sort"
	"testing"

	"github.com/frankbardon/vellum/artifact"
	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/template/anchor"
	"github.com/frankbardon/vellum/template/bind"
)

// reconcileInventory builds a package from body and runs anchor.Discover
// against its main part — the inventory half of discoverFrame, for a test
// that reconciles against it rather than executing against it.
func reconcileInventory(t *testing.T, body string) *anchor.Inventory {
	t.Helper()
	pkg := buildExecPkg(t, body)
	inv, err := anchor.Discover(pkg, artifact.FormatDOCX, execMainPart)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	return inv
}

func problemDirections(t *testing.T, err error) map[string]string {
	t.Helper()
	ce, ok := err.(*verr.CodedError)
	if !ok {
		t.Fatalf("err is not a *verr.CodedError: %v (%T)", err, err)
	}
	raw, ok := ce.Details["problems"]
	if !ok {
		t.Fatalf("err has no \"problems\" detail: %+v", ce.Details)
	}
	problems, ok := raw.([]map[string]any)
	if !ok {
		t.Fatalf("\"problems\" detail is not []map[string]any: %#v", raw)
	}
	out := make(map[string]string, len(problems))
	for _, p := range problems {
		name, _ := p["anchor"].(string)
		dir, _ := p["direction"].(string)
		out[name] = dir
	}
	return out
}

func TestReconcile_ExactMatchIsNoError(t *testing.T) {
	inv := reconcileInventory(t, `<w:p><w:r><w:t>Dear {{customer_name}},</w:t></w:r></w:p>`+
		`<w:p><w:r><w:t>{{note}}</w:t></w:r></w:p>`)
	b := &bind.Binding{FormatVersion: bind.FormatVersion, Statements: []bind.Statement{
		{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "customer_name", Expr: "x"}},
		{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "note", Expr: "x"}},
	}}
	if err := bind.Reconcile(inv, b); err != nil {
		t.Fatalf("Reconcile: %v, want nil", err)
	}
}

func TestReconcile_TemplateAnchorNeverReferencedIsError(t *testing.T) {
	inv := reconcileInventory(t, `<w:p><w:r><w:t>Dear {{customer_name}},</w:t></w:r></w:p>`+
		`<w:p><w:r><w:t>{{note}}</w:t></w:r></w:p>`)
	b := &bind.Binding{FormatVersion: bind.FormatVersion, Statements: []bind.Statement{
		{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "customer_name", Expr: "x"}},
	}}
	err := bind.Reconcile(inv, b)
	if !verr.HasCode(err, verr.VELLUM_BIND_ANCHOR_UNRECONCILED) {
		t.Fatalf("err = %v, want VELLUM_BIND_ANCHOR_UNRECONCILED", err)
	}
	dirs := problemDirections(t, err)
	if dirs["note"] != "missing_in_binding" {
		t.Errorf("problems = %v, want note -> missing_in_binding", dirs)
	}
}

func TestReconcile_TemplateAnchorListedOptionalIsNoError(t *testing.T) {
	inv := reconcileInventory(t, `<w:p><w:r><w:t>Dear {{customer_name}},</w:t></w:r></w:p>`+
		`<w:p><w:r><w:t>{{note}}</w:t></w:r></w:p>`)
	b := &bind.Binding{
		FormatVersion:   bind.FormatVersion,
		Statements:      []bind.Statement{{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "customer_name", Expr: "x"}}},
		OptionalAnchors: []string{"note"},
	}
	if err := bind.Reconcile(inv, b); err != nil {
		t.Fatalf("Reconcile: %v, want nil (note is in OptionalAnchors)", err)
	}
}

func TestReconcile_BindingReferencesNonexistentAnchorIsError(t *testing.T) {
	inv := reconcileInventory(t, `<w:p><w:r><w:t>Dear {{customer_name}},</w:t></w:r></w:p>`)
	b := &bind.Binding{FormatVersion: bind.FormatVersion, Statements: []bind.Statement{
		{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "customer_name", Expr: "x"}},
		{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "does_not_exist", Expr: "x"}},
	}}
	err := bind.Reconcile(inv, b)
	if !verr.HasCode(err, verr.VELLUM_BIND_ANCHOR_UNRECONCILED) {
		t.Fatalf("err = %v, want VELLUM_BIND_ANCHOR_UNRECONCILED", err)
	}
	dirs := problemDirections(t, err)
	if dirs["does_not_exist"] != "missing_in_template" {
		t.Errorf("problems = %v, want does_not_exist -> missing_in_template", dirs)
	}
}

func TestReconcile_BindOptionalTrueOnNonexistentAnchorIsNoError(t *testing.T) {
	inv := reconcileInventory(t, `<w:p><w:r><w:t>Dear {{customer_name}},</w:t></w:r></w:p>`)
	b := &bind.Binding{FormatVersion: bind.FormatVersion, Statements: []bind.Statement{
		{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "customer_name", Expr: "x"}},
		{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "does_not_exist", Expr: "x", Optional: true}},
	}}
	if err := bind.Reconcile(inv, b); err != nil {
		t.Fatalf("Reconcile: %v, want nil (does_not_exist's Bind is Optional: true)", err)
	}
}

// TestReconcile_MultipleMismatchesReportedTogether proves Reconcile reports
// every mismatch found in one pass, not only the first: a template anchor
// the binding never references, and a binding anchor the template does not
// have, present simultaneously.
func TestReconcile_MultipleMismatchesReportedTogether(t *testing.T) {
	inv := reconcileInventory(t, `<w:p><w:r><w:t>Dear {{customer_name}},</w:t></w:r></w:p>`+
		`<w:p><w:r><w:t>{{unbound_anchor}}</w:t></w:r></w:p>`)
	b := &bind.Binding{FormatVersion: bind.FormatVersion, Statements: []bind.Statement{
		{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "customer_name", Expr: "x"}},
		{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "phantom_anchor", Expr: "x"}},
	}}
	err := bind.Reconcile(inv, b)
	if !verr.HasCode(err, verr.VELLUM_BIND_ANCHOR_UNRECONCILED) {
		t.Fatalf("err = %v, want VELLUM_BIND_ANCHOR_UNRECONCILED", err)
	}
	dirs := problemDirections(t, err)
	if dirs["unbound_anchor"] != "missing_in_binding" {
		t.Errorf("problems = %v, want unbound_anchor -> missing_in_binding", dirs)
	}
	if dirs["phantom_anchor"] != "missing_in_template" {
		t.Errorf("problems = %v, want phantom_anchor -> missing_in_template", dirs)
	}
	if len(dirs) != 2 {
		t.Errorf("problems = %v, want exactly 2 entries", dirs)
	}

	ce := err.(*verr.CodedError)
	if n, _ := ce.Details["problem_count"].(int); n != 2 {
		t.Errorf("problem_count = %v, want 2", ce.Details["problem_count"])
	}
}

// TestReconcile_NestedStructureReachesFullDepth proves the structural
// collection reaches an anchor buried inside nested if/with/repeat, matching
// collectAnchorNames' own reach (repeat.go), and that a repeat's own loop
// variable body is still reconciled even though whether it actually runs at
// all depends on runtime data Reconcile never evaluates.
func TestReconcile_NestedStructureReachesFullDepth(t *testing.T) {
	inv := reconcileInventory(t, `<w:p><w:r><w:t>{{deep_anchor}}</w:t></w:r></w:p>`)
	b := &bind.Binding{FormatVersion: bind.FormatVersion, Statements: []bind.Statement{
		{Kind: bind.StatementIf, If: &bind.If{
			When: "true",
			Then: []bind.Statement{
				{Kind: bind.StatementWith, With: &bind.With{
					As:    "scope",
					Value: "x",
					Body: []bind.Statement{
						{Kind: bind.StatementRepeat, Repeat: &bind.Repeat{
							Over: "items", As: "item", Target: bind.RepeatTargetRow,
							Body: []bind.Statement{
								{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "deep_anchor", Expr: "item"}},
							},
						}},
					},
				}},
			},
		}},
	}}
	if err := bind.Reconcile(inv, b); err != nil {
		t.Fatalf("Reconcile: %v, want nil (deep_anchor is referenced, just deeply nested)", err)
	}

	// The reverse direction: a nonexistent anchor buried at the same depth
	// is still caught.
	b2 := &bind.Binding{FormatVersion: bind.FormatVersion, Statements: []bind.Statement{
		{Kind: bind.StatementIf, If: &bind.If{
			When: "true",
			Then: []bind.Statement{
				{Kind: bind.StatementWith, With: &bind.With{
					As:    "scope",
					Value: "x",
					Body: []bind.Statement{
						{Kind: bind.StatementRepeat, Repeat: &bind.Repeat{
							Over: "items", As: "item", Target: bind.RepeatTargetRow,
							Body: []bind.Statement{
								{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "deep_anchor", Expr: "item"}},
								{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "deep_phantom", Expr: "item"}},
							},
						}},
					},
				}},
			},
		}},
	}}
	err := bind.Reconcile(inv, b2)
	if !verr.HasCode(err, verr.VELLUM_BIND_ANCHOR_UNRECONCILED) {
		t.Fatalf("err = %v, want VELLUM_BIND_ANCHOR_UNRECONCILED", err)
	}
	dirs := problemDirections(t, err)
	if dirs["deep_phantom"] != "missing_in_template" {
		t.Errorf("problems = %v, want deep_phantom -> missing_in_template", dirs)
	}
}

// TestReconcile_SkipDoesNotExemptAStatementFromReconciliation proves
// reconciliation is structural, not execution-driven: a Skip modifier that
// would prevent a statement from running at execution time does not prevent
// Reconcile from still counting its anchor reference, because whether Skip
// evaluates true depends on data Reconcile does not have.
func TestReconcile_SkipDoesNotExemptAStatementFromReconciliation(t *testing.T) {
	inv := reconcileInventory(t, `<w:p><w:r><w:t>no anchors here</w:t></w:r></w:p>`)
	b := &bind.Binding{FormatVersion: bind.FormatVersion, Statements: []bind.Statement{
		{Kind: bind.StatementBind, Skip: "true", Bind: &bind.Bind{Anchor: "phantom", Expr: "x"}},
	}}
	err := bind.Reconcile(inv, b)
	if !verr.HasCode(err, verr.VELLUM_BIND_ANCHOR_UNRECONCILED) {
		t.Fatalf("err = %v, want VELLUM_BIND_ANCHOR_UNRECONCILED (Skip does not exempt structural reconciliation)", err)
	}
}

// TestReconcile_DeterministicProblemOrder proves the reported problem order
// does not depend on map iteration: two independent runs against the same
// inputs produce the same ordered list.
func TestReconcile_DeterministicProblemOrder(t *testing.T) {
	inv := reconcileInventory(t, `<w:p><w:r><w:t>{{zeta}}{{alpha}}{{mid}}</w:t></w:r></w:p>`)
	b := &bind.Binding{FormatVersion: bind.FormatVersion, Statements: []bind.Statement{
		{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "phantom_z", Expr: "x"}},
		{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "phantom_a", Expr: "x"}},
	}}

	var firstOrder []string
	for i := 0; i < 5; i++ {
		err := bind.Reconcile(inv, b)
		ce := err.(*verr.CodedError)
		problems := ce.Details["problems"].([]map[string]any)
		order := make([]string, len(problems))
		for j, p := range problems {
			order[j] = p["anchor"].(string) + ":" + p["direction"].(string)
		}
		if i == 0 {
			firstOrder = order
			// Also confirm the order is actually sorted by anchor name, so
			// this is not merely "consistent" but "documented and sorted".
			sorted := append([]string(nil), order...)
			sort.Strings(sorted)
			// anchor-name sort with ties broken by direction is what
			// Reconcile documents; since none of these anchor names
			// collide, a plain string sort of "anchor:direction" agrees.
			for k := range sorted {
				if sorted[k] != order[k] {
					t.Errorf("order = %v, want sorted by anchor name: %v", order, sorted)
					break
				}
			}
			continue
		}
		if len(order) != len(firstOrder) {
			t.Fatalf("run %d: order = %v, want same length as first run %v", i, order, firstOrder)
		}
		for j := range order {
			if order[j] != firstOrder[j] {
				t.Errorf("run %d: order = %v, want %v (same every run)", i, order, firstOrder)
				break
			}
		}
	}
}

func TestReconcile_NilInventoryIsInternalInvariant(t *testing.T) {
	b := &bind.Binding{FormatVersion: bind.FormatVersion, Statements: []bind.Statement{
		{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "x", Expr: "x"}},
	}}
	err := bind.Reconcile(nil, b)
	if !verr.HasCode(err, verr.VELLUM_INTERNAL_INVARIANT) {
		t.Fatalf("err = %v, want VELLUM_INTERNAL_INVARIANT", err)
	}
}

func TestReconcile_NilBindingIsBindInvalid(t *testing.T) {
	inv := reconcileInventory(t, `<w:p><w:r><w:t>none</w:t></w:r></w:p>`)
	err := bind.Reconcile(inv, nil)
	if !verr.HasCode(err, verr.VELLUM_BIND_INVALID) {
		t.Fatalf("err = %v, want VELLUM_BIND_INVALID", err)
	}
}
