package bind

// Visit is called once for every statement reached by [Walk].
type Visit func(*Statement) error

// Walk visits every statement reachable from stmts, depth-first and
// pre-order: a statement before its own children, and for an if statement,
// its then branch before its else branch. Both are slices walked in order,
// so the traversal itself introduces no nondeterminism.
//
// This is the one general-purpose tree walk the package exposes, meant to be
// reused rather than re-implemented by whatever needs to visit every
// statement — a later execution layer (E10-S3) driving repeat, if and with,
// in particular.
func Walk(stmts []Statement, visit Visit) error {
	for i := range stmts {
		s := &stmts[i]
		if err := visit(s); err != nil {
			return err
		}
		switch s.Kind {
		case StatementRepeat:
			if s.Repeat != nil {
				if err := Walk(s.Repeat.Body, visit); err != nil {
					return err
				}
			}
		case StatementIf:
			if s.If != nil {
				if err := Walk(s.If.Then, visit); err != nil {
					return err
				}
				if err := Walk(s.If.Else, visit); err != nil {
					return err
				}
			}
		case StatementWith:
			if s.With != nil {
				if err := Walk(s.With.Body, visit); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// ExprVisit is called once for every FEEL expression string reached by
// [WalkExprs].
type ExprVisit func(expr string) error

// WalkExprs visits every FEEL expression string reachable from stmts: each
// statement's Skip modifier when set, then its own kind-specific
// expression — Bind.Expr, Repeat.Over, If.When or With.Value — before
// recursing into its children, in the same depth-first pre-order [Walk]
// uses. An empty string is never visited: a required field left empty is a
// [Binding.Validate] failure, not an expression to evaluate, and Skip is
// optional so its absence is ordinary.
//
// This exists so that a later FEEL-aware validator (the bind.Validate free
// function CLAUDE.md describes, landing in E10-S2) can reject a
// nondeterministic builtin — now() or today(), bound to time.Now() by
// pbinitiative/feel — by walking the whole tree once, in one place, without
// this package needing to know what FEEL is or what "nondeterministic"
// means. Every string this package lets an author write as a FEEL
// expression is reachable through here; a later story adding a new
// FEEL-bearing field to this model must extend this function in the same
// change; a nested string it fails to reach is a nondeterministic builtin it
// fails to catch.
func WalkExprs(stmts []Statement, visit ExprVisit) error {
	return Walk(stmts, func(s *Statement) error {
		if s.Skip != "" {
			if err := visit(s.Skip); err != nil {
				return err
			}
		}
		switch s.Kind {
		case StatementBind:
			if s.Bind != nil && s.Bind.Expr != "" {
				if err := visit(s.Bind.Expr); err != nil {
					return err
				}
			}
		case StatementRepeat:
			if s.Repeat != nil && s.Repeat.Over != "" {
				if err := visit(s.Repeat.Over); err != nil {
					return err
				}
			}
		case StatementIf:
			if s.If != nil && s.If.When != "" {
				if err := visit(s.If.When); err != nil {
					return err
				}
			}
		case StatementWith:
			if s.With != nil && s.With.Value != "" {
				if err := visit(s.With.Value); err != nil {
					return err
				}
			}
		}
		return nil
	})
}
