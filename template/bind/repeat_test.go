package bind_test

import (
	"bytes"
	"testing"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/template/bind"
)

func rowRepeatBody() []bind.Statement {
	return []bind.Statement{
		{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "item_name", Expr: "item.name"}},
		{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "item_qty", Expr: "item.qty"}},
		{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "item_status", Expr: `"n/a"`}},
	}
}

func threeItems() []any {
	return []any{
		map[string]any{"name": "Widget", "qty": 3.0},
		map[string]any{"name": "Gadget", "qty": 5.0},
		map[string]any{"name": "Sprocket", "qty": 9.0},
	}
}

// TestExecute_RepeatRowTarget: a 3-item repeat over a table's templated
// row, verified by re-Walking the result and checking each row's own text.
func TestExecute_RepeatRowTarget(t *testing.T) {
	pkg := buildExecPkg(t, buildRowTable())
	stmts := []bind.Statement{
		{Kind: bind.StatementRepeat, Repeat: &bind.Repeat{
			Over: "items", As: "item", Target: bind.RepeatTargetRow,
			Body: rowRepeatBody(),
		}},
	}
	data := bind.Scope{"items": threeItems()}

	out := runExec(t, pkg, stmts, data)

	rows := rowTexts(t, out)
	// header + 3 filled rows
	if len(rows) != 4 {
		t.Fatalf("got %d <w:tr> rows, want 4 (1 header + 3 items): %q", len(rows), rows)
	}
	if rows[0] != "NameQtyStatus" {
		t.Errorf("header row = %q, want unchanged", rows[0])
	}
	wantRows := []string{"Widget3n/a", "Gadget5n/a", "Sprocket9n/a"}
	for i, want := range wantRows {
		if rows[i+1] != want {
			t.Errorf("row %d = %q, want %q", i+1, rows[i+1], want)
		}
	}
	if bytes.Contains(out, []byte("{{item_name}}")) {
		t.Errorf("templated row's raw marker text survived: %s", out)
	}
}

// TestExecute_RepeatBlockTarget mirrors the row case for a native content
// control: the whole control's own content is spliced N times.
func TestExecute_RepeatBlockTarget(t *testing.T) {
	pkg := buildExecPkg(t, `<w:sdt><w:sdtPr><w:id w:val="1"/></w:sdtPr>`+
		`<w:sdtContent><w:p><w:r><w:t>{{item_name}}</w:t></w:r></w:p></w:sdtContent></w:sdt>`)
	stmts := []bind.Statement{
		{Kind: bind.StatementRepeat, Repeat: &bind.Repeat{
			Over: "items", As: "item", Target: bind.RepeatTargetBlock,
			Body: []bind.Statement{
				{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "item_name", Expr: "item"}},
			},
		}},
	}
	data := bind.Scope{"items": []any{"Alpha", "Beta", "Gamma"}}

	out := runExec(t, pkg, stmts, data)

	for _, want := range []string{"Alpha", "Beta", "Gamma"} {
		if !bytes.Contains(out, []byte(want)) {
			t.Errorf("output missing %q: %s", want, out)
		}
	}
	if n := bytes.Count(out, []byte("<w:sdt>")); n != 3 {
		t.Errorf("got %d <w:sdt> copies, want 3: %s", n, out)
	}
	if bytes.Contains(out, []byte("{{item_name}}")) {
		t.Errorf("raw marker text survived: %s", out)
	}
}

// TestExecute_RepeatZeroItems is the documented empty-list behaviour: the
// container's own span (here, the templated <w:tr>) is deleted entirely,
// leaving only the header row. CT_Tbl's own content model allows a w:tbl
// with zero w:tr, so this is well-formed WordprocessingML even when — as
// here — it is the table's only data row.
func TestExecute_RepeatZeroItems(t *testing.T) {
	pkg := buildExecPkg(t, buildRowTable())
	stmts := []bind.Statement{
		{Kind: bind.StatementRepeat, Repeat: &bind.Repeat{
			Over: "items", As: "item", Target: bind.RepeatTargetRow,
			Body: rowRepeatBody(),
		}},
	}
	data := bind.Scope{"items": []any{}}

	out := runExec(t, pkg, stmts, data)

	rows := rowTexts(t, out)
	if len(rows) != 1 {
		t.Fatalf("got %d <w:tr> rows, want 1 (header only): %q", len(rows), rows)
	}
	if rows[0] != "NameQtyStatus" {
		t.Errorf("header row = %q, want unchanged", rows[0])
	}
	if !bytes.Contains(out, []byte("<w:tbl>")) {
		t.Error("the table element itself should survive; only its templated row should vanish")
	}
}

// TestExecute_RepeatWithNestedIf varies its own output per item via an if
// inside the repeat's body.
func TestExecute_RepeatWithNestedIf(t *testing.T) {
	pkg := buildExecPkg(t, buildRowTable())
	stmts := []bind.Statement{
		{Kind: bind.StatementRepeat, Repeat: &bind.Repeat{
			Over: "items", As: "item", Target: bind.RepeatTargetRow,
			Body: []bind.Statement{
				{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "item_name", Expr: "item.name"}},
				{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "item_qty", Expr: "item.qty"}},
				{Kind: bind.StatementIf, If: &bind.If{
					When: "item.qty > 5",
					Then: []bind.Statement{{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "item_status", Expr: `"HIGH"`}}},
					Else: []bind.Statement{{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "item_status", Expr: `"LOW"`}}},
				}},
			},
		}},
	}
	data := bind.Scope{"items": threeItems()} // qty: 3, 5, 9

	out := runExec(t, pkg, stmts, data)

	rows := rowTexts(t, out)
	if len(rows) != 4 {
		t.Fatalf("got %d rows, want 4: %q", len(rows), rows)
	}
	wantRows := []string{"Widget3LOW", "Gadget5LOW", "Sprocket9HIGH"}
	for i, want := range wantRows {
		if rows[i+1] != want {
			t.Errorf("row %d = %q, want %q (qty > 5 must take the then branch, per item)", i+1, rows[i+1], want)
		}
	}
}

// TestExecute_RepeatBodyAnchorNotFound checks that resolving a repeat body's
// own anchors surfaces the same coded error a top-level Bind would.
func TestExecute_RepeatBodyAnchorNotFound(t *testing.T) {
	pkg := buildExecPkg(t, buildRowTable())
	stmts := []bind.Statement{
		{Kind: bind.StatementRepeat, Repeat: &bind.Repeat{
			Over: "items", As: "item", Target: bind.RepeatTargetRow,
			Body: []bind.Statement{
				{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "does_not_exist", Expr: "item.name"}},
			},
		}},
	}
	frame := discoverFrame(t, pkg)
	repls := bind.NewReplacementSet()
	err := bind.Execute(stmts, bind.Scope{"items": threeItems()}, bind.NewFEELEvaluator(), frame, pkg, repls)
	if !verr.HasCode(err, verr.VELLUM_BIND_ANCHOR_UNKNOWN) {
		t.Fatalf("err = %v, want VELLUM_BIND_ANCHOR_UNKNOWN", err)
	}
}

// TestExecute_RepeatContainerMismatchIsCodedError covers a repeat whose
// body's anchors are scattered across more than one row, so there is no
// single <w:tr> to reconcile them to.
func TestExecute_RepeatContainerMismatchIsCodedError(t *testing.T) {
	pkg := buildExecPkg(t,
		`<w:tbl><w:tblPr/><w:tblGrid><w:gridCol/><w:gridCol/></w:tblGrid>`+
			`<w:tr><w:tc><w:p><w:r><w:t>{{item_name}}</w:t></w:r></w:p></w:tc></w:tr>`+
			`<w:tr><w:tc><w:p><w:r><w:t>{{item_qty}}</w:t></w:r></w:p></w:tc></w:tr>`+
			`</w:tbl>`)
	stmts := []bind.Statement{
		{Kind: bind.StatementRepeat, Repeat: &bind.Repeat{
			Over: "items", As: "item", Target: bind.RepeatTargetRow,
			Body: []bind.Statement{
				{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "item_name", Expr: "item.name"}},
				{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "item_qty", Expr: "item.qty"}},
			},
		}},
	}
	frame := discoverFrame(t, pkg)
	repls := bind.NewReplacementSet()
	err := bind.Execute(stmts, bind.Scope{"items": threeItems()}, bind.NewFEELEvaluator(), frame, pkg, repls)
	if !verr.HasCode(err, verr.VELLUM_TEMPLATE_REPEAT_CONTAINER_INVALID) {
		t.Fatalf("err = %v, want VELLUM_TEMPLATE_REPEAT_CONTAINER_INVALID", err)
	}
}

// TestExecute_RepeatContainerMismatchNoRowsAtAll covers the same failure
// mode when the template carries no <w:tr> element at all.
func TestExecute_RepeatContainerMismatchNoRowsAtAll(t *testing.T) {
	pkg := buildExecPkg(t, `<w:p><w:r><w:t>{{item_name}}</w:t></w:r></w:p>`)
	stmts := []bind.Statement{
		{Kind: bind.StatementRepeat, Repeat: &bind.Repeat{
			Over: "items", As: "item", Target: bind.RepeatTargetRow,
			Body: []bind.Statement{
				{Kind: bind.StatementBind, Bind: &bind.Bind{Anchor: "item_name", Expr: "item"}},
			},
		}},
	}
	frame := discoverFrame(t, pkg)
	repls := bind.NewReplacementSet()
	err := bind.Execute(stmts, bind.Scope{"items": []any{"x"}}, bind.NewFEELEvaluator(), frame, pkg, repls)
	if !verr.HasCode(err, verr.VELLUM_TEMPLATE_REPEAT_CONTAINER_INVALID) {
		t.Fatalf("err = %v, want VELLUM_TEMPLATE_REPEAT_CONTAINER_INVALID", err)
	}
}
