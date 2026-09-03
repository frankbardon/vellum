package bind

import (
	"strconv"

	verr "github.com/frankbardon/vellum/errors"
)

// FormatVersion is the wire version of the binding document shape.
const FormatVersion = "1.0"

// StatementKind names a statement's type. The vocabulary is closed: four
// kinds, forming the thin control layer FEEL itself does not provide — FEEL
// is a pure expression language with no iteration-with-output and no
// conditional block, so repetition and branching are stated here instead,
// around FEEL expressions FEEL itself does evaluate.
type StatementKind string

const (
	// StatementBind is the leaf case: one anchor, one FEEL expression.
	StatementBind StatementKind = "bind"

	// StatementRepeat produces zero or more copies of its body, once per
	// item of a FEEL list expression.
	StatementRepeat StatementKind = "repeat"

	// StatementIf selects one of two statement lists by a FEEL boolean
	// expression.
	StatementIf StatementKind = "if"

	// StatementWith narrows the evaluation scope for its body by binding a
	// name to an evaluated FEEL expression, so nested statements can
	// reference it without repeating the whole path.
	StatementWith StatementKind = "with"
)

// allStatementKinds is the registry, hand-maintained and ordered.
var allStatementKinds = []StatementKind{StatementBind, StatementRepeat, StatementIf, StatementWith}

// AllStatementKinds returns a copy of the statement vocabulary, in
// declaration order.
func AllStatementKinds() []StatementKind {
	out := make([]StatementKind, len(allStatementKinds))
	copy(out, allStatementKinds)
	return out
}

// ValidStatementKind reports whether k is in the vocabulary.
func ValidStatementKind(k StatementKind) bool {
	for _, v := range allStatementKinds {
		if v == k {
			return true
		}
	}
	return false
}

// RepeatTarget says which of the two ways DOCX repetition realizes a
// [Repeat] statement splices into.
//
// A repeated anchor lands in the template one of two structurally different
// ways: inside a table row, where repetition means splicing N rows into the
// table the anchor's row belongs to, or inside a whole native content
// control block, where repetition means splicing N copies of the control's
// content. The two need different splice logic, and this field is what
// tells the execution layer (E10-S3) which one to run, rather than making it
// infer intent by inspecting where the body's anchors happen to sit in the
// document tree.
//
// This is a deliberate design choice, not the only one available: the
// execution layer *could* discover the target by walking the template for
// the anchor's structural position once the repeat actually runs. Declaring
// it here instead follows the project's own rule for everything else it
// calls "declared, not emergent" — the capability matrix states an outcome
// before a document exercises it rather than letting a render discover one,
// and a repeat's target is exactly that kind of fact: knowable the moment
// the binding is authored, and the wrong kind of thing to leave for a
// splicer to guess from document shape. It also means a binding is
// reviewable and diffable on its own, without the template open beside it —
// which is the whole reason this package exists apart from the template.
type RepeatTarget string

const (
	// RepeatTargetRow repeats a table row: the anchor's row is spliced N
	// times into the table it belongs to.
	RepeatTargetRow RepeatTarget = "row"

	// RepeatTargetBlock repeats a native content control block: the
	// control's content is spliced N times.
	RepeatTargetBlock RepeatTarget = "block"
)

// allRepeatTargets is the registry, hand-maintained and ordered.
var allRepeatTargets = []RepeatTarget{RepeatTargetRow, RepeatTargetBlock}

// AllRepeatTargets returns a copy of the repeat-target vocabulary, in
// declaration order.
func AllRepeatTargets() []RepeatTarget {
	out := make([]RepeatTarget, len(allRepeatTargets))
	copy(out, allRepeatTargets)
	return out
}

// ValidRepeatTarget reports whether t is in the vocabulary. The zero value
// is not valid: a repeat's target is not defaulted, because the two splice
// strategies produce different documents from the same data and guessing
// between them is exactly what "declared, not emergent" exists to prevent.
func ValidRepeatTarget(t RepeatTarget) bool {
	for _, v := range allRepeatTargets {
		if v == t {
			return true
		}
	}
	return false
}

// Binding is a complete binding document: an ordered list of top-level
// statements.
type Binding struct {
	// FormatVersion is the wire version this binding was authored against.
	FormatVersion string `json:"format_version"`

	// Statements are the top-level bindings, evaluated in order.
	Statements []Statement `json:"statements"`
}

// Statement is a tagged union of the four control-layer kinds. Exactly one
// arm is non-nil and Kind names it.
//
// A tagged struct rather than a Go interface, for the same reason
// [spec.Block] is one: the model must round-trip through strict JSON
// decoding, and an interface does not do that without a custom codec that
// would then be one more thing able to drift from the JSON shape.
type Statement struct {
	// Kind names which arm carries this statement's content.
	Kind StatementKind `json:"kind"`

	// Bind is set when Kind is StatementBind.
	Bind *Bind `json:"bind,omitempty"`

	// Repeat is set when Kind is StatementRepeat.
	Repeat *Repeat `json:"repeat,omitempty"`

	// If is set when Kind is StatementIf.
	If *If `json:"if,omitempty"`

	// With is set when Kind is StatementWith.
	With *With `json:"with,omitempty"`

	// Skip is a FEEL boolean expression modifier available on every
	// statement kind, not a statement kind of its own.
	//
	// When set, it is evaluated first; if it evaluates true the whole
	// statement is treated as absent — not an error, not rendered, nothing
	// spliced, and (for a bind statement) not a candidate for
	// VELLUM_ANCHOR_* reconciliation failure the way a genuinely unbound
	// required anchor would be. This is what lets an anchor be legitimately
	// conditionally optional at binding-authoring time, distinct from
	// Bind.Optional: Optional says "this anchor may not exist in the
	// template at all"; Skip says "this anchor exists, but this data does
	// not call for filling it on this run". Modelling it as a field on every
	// statement rather than as a fifth statement kind avoids forcing every
	// leaf and every branch to be wrapped in a synthetic "skip" node just to
	// become conditional, and keeps a skipped repeat or a skipped if from
	// needing special-cased children.
	Skip string `json:"skip,omitempty"`
}

// Bind is the leaf statement: one anchor, one FEEL expression.
type Bind struct {
	// Anchor is the binding key, matching an [anchor.Anchor.Name] — the
	// content control's w:tag value for a native anchor, or the text
	// between {{ and }} for a marker.
	Anchor string `json:"anchor"`

	// Expr is a FEEL expression, evaluated against the caller-supplied data
	// and the current evaluation scope. Opaque at this layer: this package
	// does not parse or validate FEEL syntax, only stores and makes it
	// reachable for a later layer that does.
	Expr string `json:"expr"`

	// Format is an xlsx number-format code applied to the evaluated value
	// before it becomes text, the same vocabulary and field shape
	// [spec.Cell.Format] uses, so there is no second dialect to learn.
	Format string `json:"format,omitempty"`

	// Optional marks that this anchor may legitimately be absent from the
	// template without that being an error. Reconciling anchor presence
	// against a binding is E10-S4's job; this field is carried faithfully
	// now so that story has something to read.
	Optional bool `json:"optional,omitempty"`
}

// Repeat produces zero or more copies of Body, once per item of a FEEL list
// expression.
type Repeat struct {
	// Over is a FEEL expression expected to evaluate to a list.
	Over string `json:"over"`

	// As names the loop variable each iteration's evaluation scope is bound
	// to.
	As string `json:"as"`

	// Target says which of the two DOCX repetition mechanisms this repeat
	// realizes as. See [RepeatTarget] for why this is declared here rather
	// than inferred from the anchor's position in the template.
	Target RepeatTarget `json:"target"`

	// Body is the nested statements evaluated once per item.
	Body []Statement `json:"body"`
}

// If selects Then or Else by a FEEL boolean expression.
type If struct {
	// When is a FEEL boolean expression.
	When string `json:"when"`

	// Then is evaluated when When is true.
	Then []Statement `json:"then"`

	// Else is evaluated when When is false. Omitting it means "do nothing"
	// on the false branch, not an error.
	Else []Statement `json:"else,omitempty"`
}

// With narrows the evaluation scope for Body by binding As to the evaluated
// Value.
type With struct {
	// As names the scope variable Body's statements can reference.
	As string `json:"as"`

	// Value is a FEEL expression, evaluated once and bound to As for
	// everything nested inside Body.
	Value string `json:"value"`

	// Body is the nested statements evaluated in the narrowed scope.
	Body []Statement `json:"body"`
}

// Validate reports structural problems with the binding.
//
// It checks shape only — that the binding carries at least one statement,
// that every statement's kind is in the vocabulary and that the arm matching
// its kind is present and its own required fields are non-empty, recursively
// through every nested body. It does not parse or evaluate any FEEL
// expression: a syntactically invalid expr or a nondeterministic builtin
// both pass here and are caught by the FEEL-aware validator layered on top,
// once one exists.
func (b *Binding) Validate() error {
	if b == nil {
		return verr.NewCodedError(verr.VELLUM_BIND_INVALID, "binding is nil")
	}
	if len(b.Statements) == 0 {
		return verr.NewCodedError(verr.VELLUM_BIND_INVALID, "binding has no statements")
	}
	return validateStatements(b.Statements, nil)
}

func validateStatements(stmts []Statement, path []string) error {
	for i := range stmts {
		p := appendPath(path, indexSegment(i))
		if err := stmts[i].validate(p); err != nil {
			return err
		}
	}
	return nil
}

func (s *Statement) validate(path []string) error {
	where := map[string]any{"path": pathString(path), "kind": string(s.Kind)}

	if !ValidStatementKind(s.Kind) {
		return verr.NewCodedErrorWithDetails(verr.VELLUM_BIND_STATEMENT_KIND_UNKNOWN,
			"statement declares a kind that is not in the vocabulary", where)
	}

	switch s.Kind {
	case StatementBind:
		if s.Bind == nil {
			return missingArm(where, "bind")
		}
		if s.Bind.Anchor == "" {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_BIND_INVALID,
				"bind statement has no anchor", where)
		}
		if s.Bind.Expr == "" {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_BIND_INVALID,
				"bind statement has no expression", where)
		}

	case StatementRepeat:
		if s.Repeat == nil {
			return missingArm(where, "repeat")
		}
		r := s.Repeat
		if r.Over == "" {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_BIND_INVALID,
				"repeat statement has no over expression", where)
		}
		if r.As == "" {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_BIND_INVALID,
				"repeat statement has no loop variable name", where)
		}
		if !ValidRepeatTarget(r.Target) {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_BIND_REPEAT_TARGET_UNKNOWN,
				"repeat statement declares a target that is not \"row\" or \"block\"",
				withList(where, "known", repeatTargetStrings()))
		}
		if err := validateStatements(r.Body, appendPath(path, "repeat", "body")); err != nil {
			return err
		}

	case StatementIf:
		if s.If == nil {
			return missingArm(where, "if")
		}
		iv := s.If
		if iv.When == "" {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_BIND_INVALID,
				"if statement has no when expression", where)
		}
		if err := validateStatements(iv.Then, appendPath(path, "if", "then")); err != nil {
			return err
		}
		if err := validateStatements(iv.Else, appendPath(path, "if", "else")); err != nil {
			return err
		}

	case StatementWith:
		if s.With == nil {
			return missingArm(where, "with")
		}
		w := s.With
		if w.As == "" {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_BIND_INVALID,
				"with statement has no scope variable name", where)
		}
		if w.Value == "" {
			return verr.NewCodedErrorWithDetails(verr.VELLUM_BIND_INVALID,
				"with statement has no value expression", where)
		}
		if err := validateStatements(w.Body, appendPath(path, "with", "body")); err != nil {
			return err
		}
	}

	return s.checkNoStrayArms(where)
}

// checkNoStrayArms rejects a statement carrying content for a kind other
// than its own, the same construction mistake [spec.Block] guards against
// and for the same reason: honouring whichever arm the discriminator names
// would hide the mistake and silently drop the other content.
func (s *Statement) checkNoStrayArms(where map[string]any) error {
	present := make([]string, 0, 3)
	add := func(name string, set, isOwn bool) {
		if set && !isOwn {
			present = append(present, name)
		}
	}
	add("bind", s.Bind != nil, s.Kind == StatementBind)
	add("repeat", s.Repeat != nil, s.Kind == StatementRepeat)
	add("if", s.If != nil, s.Kind == StatementIf)
	add("with", s.With != nil, s.Kind == StatementWith)

	if len(present) == 0 {
		return nil
	}
	detail := copyDetails(where)
	detail["stray_arms"] = present
	return verr.NewCodedErrorWithDetails(verr.VELLUM_BIND_INVALID,
		"statement carries content for a kind other than its own", detail)
}

func missingArm(where map[string]any, arm string) error {
	detail := copyDetails(where)
	detail["missing_arm"] = arm
	return verr.NewCodedErrorWithDetails(verr.VELLUM_BIND_INVALID,
		"statement declares a kind but carries no content for it", detail)
}

func copyDetails(in map[string]any) map[string]any {
	out := make(map[string]any, len(in)+1)
	for k, v := range in {
		out[k] = v
	}
	return out
}

func withList(where map[string]any, key string, values []string) map[string]any {
	out := copyDetails(where)
	out[key] = values
	return out
}

func repeatTargetStrings() []string {
	all := AllRepeatTargets()
	out := make([]string, len(all))
	for i, t := range all {
		out[i] = string(t)
	}
	return out
}

// appendPath returns path with segs appended, always copying so that two
// branches built from the same prefix (if's then and else, in particular)
// never alias one another's backing array.
func appendPath(path []string, segs ...string) []string {
	out := make([]string, 0, len(path)+len(segs))
	out = append(out, path...)
	out = append(out, segs...)
	return out
}

func pathString(path []string) string {
	if len(path) == 0 {
		return "/"
	}
	out := ""
	for i, seg := range path {
		if i > 0 {
			out += "/"
		}
		out += seg
	}
	return out
}

func indexSegment(i int) string {
	return strconv.Itoa(i)
}
