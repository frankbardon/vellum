package bind_test

import (
	"bytes"
	"strings"
	"testing"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/template/bind"
)

func TestExecute_BindOnly(t *testing.T) {
	// Each marker sits in its own paragraph/run rather than sharing one: two
	// markers inside the same w:r run each conservatively claim that run's
	// whole span when spliced independently (defrag/splice replaces the
	// run a match touches, not just the matched substring), so two such
	// splices computed against the same pristine run necessarily overlap
	// once both are applied together — a real fragmentation case
	// template/defrag's own corpus covers, not something this test is
	// about.
	pkg := buildExecPkg(t, `<w:p><w:r><w:t>Dear {{customer_name}},</w:t></w:r></w:p>`+
		`<w:p><w:r><w:t>your total is {{amount}}.</w:t></w:r></w:p>`)

	stmts := []bind.Statement{
		{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "customer_name", Expr: "customer.name"}},
		{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "amount", Expr: "customer.amount", Format: `$#,##0.00`}},
	}
	data := bind.Scope{"customer": map[string]any{"name": "Acme & Co.", "amount": 1234.5}}

	out := runExec(t, pkg, stmts, data)

	if !bytes.Contains(out, []byte("Acme &amp; Co.")) {
		t.Errorf("customer_name not spliced in: %s", out)
	}
	if !bytes.Contains(out, []byte("$1,234.50")) {
		t.Errorf("amount not formatted as expected: %s", out)
	}
	if bytes.Contains(out, []byte("{{customer_name}}")) || bytes.Contains(out, []byte("{{amount}}")) {
		t.Errorf("raw marker text survived: %s", out)
	}
}

func TestExecute_IfThenBranch(t *testing.T) {
	pkg := buildExecPkg(t, `<w:p><w:r><w:t>{{note}}</w:t></w:r></w:p>`)
	stmts := []bind.Statement{
		{Kind: bind.StatementIf, If: &bind.If{
			When: "customer.vip",
			Then: []bind.Statement{{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "note", Expr: `"VIP customer."`}}},
			Else: []bind.Statement{{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "note", Expr: `"Standard customer."`}}},
		}},
	}

	out := runExec(t, pkg, stmts, bind.Scope{"customer": map[string]any{"vip": true}})
	if !bytes.Contains(out, []byte("VIP customer.")) {
		t.Errorf("then branch did not run: %s", out)
	}
	if bytes.Contains(out, []byte("Standard customer.")) {
		t.Errorf("else branch ran when it should not have: %s", out)
	}
}

func TestExecute_IfElseBranch(t *testing.T) {
	pkg := buildExecPkg(t, `<w:p><w:r><w:t>{{note}}</w:t></w:r></w:p>`)
	stmts := []bind.Statement{
		{Kind: bind.StatementIf, If: &bind.If{
			When: "customer.vip",
			Then: []bind.Statement{{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "note", Expr: `"VIP customer."`}}},
			Else: []bind.Statement{{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "note", Expr: `"Standard customer."`}}},
		}},
	}

	out := runExec(t, pkg, stmts, bind.Scope{"customer": map[string]any{"vip": false}})
	if !bytes.Contains(out, []byte("Standard customer.")) {
		t.Errorf("else branch did not run: %s", out)
	}
	if bytes.Contains(out, []byte("VIP customer.")) {
		t.Errorf("then branch ran when it should not have: %s", out)
	}
}

func TestExecute_WithNarrowsScope(t *testing.T) {
	pkg := buildExecPkg(t, `<w:p><w:r><w:t>{{city}}</w:t></w:r></w:p>`)
	stmts := []bind.Statement{
		{Kind: bind.StatementWith, With: &bind.With{
			As:    "addr",
			Value: "customer.address",
			Body: []bind.Statement{
				{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "city", Expr: "addr.city"}},
			},
		}},
	}
	data := bind.Scope{"customer": map[string]any{
		"address": map[string]any{"city": "Springfield"},
	}}

	out := runExec(t, pkg, stmts, data)
	if !bytes.Contains(out, []byte("Springfield")) {
		t.Errorf("with-narrowed scope was not visible to the nested bind: %s", out)
	}
}

func TestExecute_SkipOnBindContributesNothing(t *testing.T) {
	pkg := buildExecPkg(t, `<w:p><w:r><w:t>{{note}}</w:t></w:r></w:p>`)
	stmts := []bind.Statement{
		{Kind: bind.StatementBind, Skip: "true", Bind: &bind.Bind{Anchor: "note", Expr: `"should not appear"`}},
	}
	frame := discoverFrame(t, pkg)
	repls := bind.NewReplacementSet()
	if err := bind.Execute(stmts, nil, bind.NewFEELEvaluator(), frame, pkg, repls); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if names := repls.PartNames(); len(names) != 0 {
		t.Errorf("a skipped bind produced replacements: %v", names)
	}
}

func TestExecute_SkipOnIfContributesNothing(t *testing.T) {
	pkg := buildExecPkg(t, `<w:p><w:r><w:t>{{note}}</w:t></w:r></w:p>`)
	stmts := []bind.Statement{
		{Kind: bind.StatementIf, Skip: "true", If: &bind.If{
			When: "true",
			Then: []bind.Statement{{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "note", Expr: `"should not appear"`}}},
		}},
	}
	frame := discoverFrame(t, pkg)
	repls := bind.NewReplacementSet()
	if err := bind.Execute(stmts, nil, bind.NewFEELEvaluator(), frame, pkg, repls); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if names := repls.PartNames(); len(names) != 0 {
		t.Errorf("a skipped if produced replacements: %v", names)
	}
}

func TestExecute_SkipOnRepeatContributesNothing(t *testing.T) {
	pkg := buildExecPkg(t, buildRowTable())
	stmts := []bind.Statement{
		{Kind: bind.StatementRepeat, Skip: "true", Repeat: &bind.Repeat{
			Over: "items", As: "item", Target: bind.RepeatTargetRow,
			Body: []bind.Statement{
				{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "item_name", Expr: "item.name"}},
				{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "item_qty", Expr: "item.qty"}},
				{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "item_status", Expr: `"n/a"`}},
			},
		}},
	}
	frame := discoverFrame(t, pkg)
	repls := bind.NewReplacementSet()
	// items evaluates to something that would fail if it were ever reached
	// (not a list), so if Skip does not stop traversal this fails loudly
	// rather than silently passing.
	data := bind.Scope{"items": "not-a-list"}
	if err := bind.Execute(stmts, data, bind.NewFEELEvaluator(), frame, pkg, repls); err != nil {
		t.Fatalf("Execute: %v (a skipped repeat must not even evaluate Over)", err)
	}
	if names := repls.PartNames(); len(names) != 0 {
		t.Errorf("a skipped repeat produced replacements: %v", names)
	}
}

func TestExecute_UnknownAnchorIsCodedError(t *testing.T) {
	pkg := buildExecPkg(t, `<w:p><w:r><w:t>no anchors here</w:t></w:r></w:p>`)
	stmts := []bind.Statement{
		{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "does_not_exist", Expr: `"x"`}},
	}
	frame := discoverFrame(t, pkg)
	repls := bind.NewReplacementSet()
	err := bind.Execute(stmts, nil, bind.NewFEELEvaluator(), frame, pkg, repls)
	if !verr.HasCode(err, verr.VELLUM_BIND_ANCHOR_UNKNOWN) {
		t.Fatalf("err = %v, want VELLUM_BIND_ANCHOR_UNKNOWN", err)
	}
}

func TestExecute_SecondPartNeverTouched(t *testing.T) {
	pkg := buildExecPkg(t, `<w:p><w:r><w:t>{{note}}</w:t></w:r></w:p>`)
	before, ok := pkg.Get(execSecondPart)
	if !ok {
		t.Fatal("fixture lost its own second part")
	}
	beforeBytes, err := before.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}

	stmts := []bind.Statement{
		{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "note", Expr: `"hello"`}},
	}
	frame := discoverFrame(t, pkg)
	repls := bind.NewReplacementSet()
	if err := bind.Execute(stmts, nil, bind.NewFEELEvaluator(), frame, pkg, repls); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if names := repls.PartNames(); len(names) != 1 || names[0] != execMainPart {
		t.Fatalf("touched parts = %v, want only %q", names, execMainPart)
	}

	after, ok := pkg.Get(execSecondPart)
	if !ok {
		t.Fatal("second part vanished from the package")
	}
	afterBytes, err := after.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	if !bytes.Equal(beforeBytes, afterBytes) {
		t.Errorf("the second part changed even though nothing targeted it:\nbefore: %s\nafter:  %s", beforeBytes, afterBytes)
	}
}

// TestExecute_StatementsOutOfDocumentOrderStillApply proves ReplacementSet.For
// sorts by Start rather than relying on the binding's own statement order to
// match the template's document order.
func TestExecute_StatementsOutOfDocumentOrderStillApply(t *testing.T) {
	pkg := buildExecPkg(t, `<w:p><w:r><w:t>{{first}}</w:t></w:r></w:p><w:p><w:r><w:t>{{second}}</w:t></w:r></w:p>`)
	// Binding declares "second" before "first" — the reverse of document
	// order.
	stmts := []bind.Statement{
		{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "second", Expr: `"S"`}},
		{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "first", Expr: `"F"`}},
	}
	out := runExec(t, pkg, stmts, nil)
	if bytes.Contains(out, []byte("{{first}}")) || bytes.Contains(out, []byte("{{second}}")) {
		t.Errorf("raw marker text survived: %s", out)
	}
	first := strings.Index(string(out), "F")
	second := strings.Index(string(out), "S")
	if first < 0 || second < 0 {
		t.Fatalf("both anchors' filled text must be present: %s", out)
	}
	if first > second {
		t.Errorf("first anchor's text (at %d) must precede second's (at %d) in the output, matching document order: %s", first, second, out)
	}
}
