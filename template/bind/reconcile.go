package bind

// This file is E10-S4's pre-flight reconciliation: FR-F6 requires an error
// on an anchor present in the template but absent from the binding, and the
// reverse, unless explicitly marked optional. [Reconcile] answers that
// question structurally, over the binding's whole statement tree, before
// [Execute] runs anything — so a binding author learns about every mismatch
// at once, in one call, rather than one at a time as execution happens to
// trip over them (execBind's own VELLUM_BIND_ANCHOR_UNKNOWN check stays in
// place as a defensive fallback, but a caller that runs Reconcile first
// never reaches it for a mismatch this file already reports).
//
// Reconciliation is deliberately static rather than execution-driven:
// whether an if branch is actually taken, or how many times a repeat's body
// runs, depends on data Reconcile does not have. The question this file asks
// is narrower and answerable without any: does the binding's statement tree
// *mention* this anchor anywhere in its structure, regardless of what
// Skip/if/repeat/with branch it sits under.

import (
	"sort"

	verr "github.com/frankbardon/vellum/errors"
	"github.com/frankbardon/vellum/template/anchor"
)

// directionMissingInBinding names a problem where the template discovered an
// anchor the binding's statement tree never references, and the anchor's
// name is not listed in [Binding.OptionalAnchors].
const directionMissingInBinding = "missing_in_binding"

// directionMissingInTemplate names a problem where the binding's statement
// tree references an anchor the template's own [anchor.Inventory] does not
// contain, and the referencing [Bind] statement is not [Bind.Optional].
const directionMissingInTemplate = "missing_in_template"

// Reconcile checks inv (a template's discovered anchors) against b (a
// binding's statement tree) and returns nil when every anchor is accounted
// for, or a single [verr.CodedError] under VELLUM_BIND_ANCHOR_UNRECONCILED
// whose Details["problems"] lists every mismatch found — not only the
// first — each entry naming the anchor and which direction it failed:
// [directionMissingInBinding] or [directionMissingInTemplate].
//
// Two directions, per FR-F6:
//
//   - A [Bind] statement's Anchor names something inv does not discover, and
//     that Bind is not Optional: true. Reachable at any nesting depth,
//     regardless of Skip or which if/repeat/with branch it sits under — see
//     this file's own doc comment for why that has to be structural rather
//     than execution-driven.
//   - inv discovers an anchor no Bind statement anywhere in b's tree
//     references, and the anchor's Name is not listed in
//     b.OptionalAnchors.
//
// Problems are reported in a deterministic order — sorted by anchor name,
// then by direction — never map iteration order, per CLAUDE.md's
// determinism rules; the anchor-name sets built along the way use maps only
// for membership testing.
//
// b is assumed already [Binding.Validate]d: Reconcile does not re-check
// statement shape, only anchor presence.
func Reconcile(inv *anchor.Inventory, b *Binding) error {
	if inv == nil {
		return verr.NewCodedError(verr.VELLUM_INTERNAL_INVARIANT, "nil inventory")
	}
	if b == nil {
		return verr.NewCodedError(verr.VELLUM_BIND_INVALID, "nil binding")
	}

	templateAnchors := make(map[string]bool, len(inv.Anchors))
	for _, a := range inv.Anchors {
		templateAnchors[a.Name] = true
	}

	optionalAnchors := make(map[string]bool, len(b.OptionalAnchors))
	for _, name := range b.OptionalAnchors {
		optionalAnchors[name] = true
	}

	// referenced tracks, for every anchor name a Bind statement anywhere in
	// the tree names, whether it was ever referenced by a non-Optional
	// Bind. A name referenced by both an Optional and a non-Optional Bind
	// (two different statements naming the same anchor) counts as
	// referenced-and-required, since at least one statement in the tree
	// expects the template to actually have it.
	referenced := make(map[string]bool)
	requiredButMissing := make(map[string]bool)

	Walk(b.Statements, func(s *Statement) error {
		if s.Kind != StatementBind || s.Bind == nil || s.Bind.Anchor == "" {
			return nil
		}
		name := s.Bind.Anchor
		referenced[name] = true
		if !s.Bind.Optional && !templateAnchors[name] {
			requiredButMissing[name] = true
		}
		return nil
	})

	var problems []map[string]any

	names := make([]string, 0, len(requiredButMissing))
	for name := range requiredButMissing {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		problems = append(problems, map[string]any{
			"anchor":    name,
			"direction": directionMissingInTemplate,
		})
	}

	names = names[:0]
	for _, a := range inv.Anchors {
		if !referenced[a.Name] && !optionalAnchors[a.Name] {
			names = append(names, a.Name)
		}
	}
	sort.Strings(names)
	// Two anchors of different Kind sharing the same Name were already
	// rejected at discovery time (VELLUM_ANCHOR_DUPLICATE), so inv.Anchors
	// carries no duplicate names here — dedupe is not needed on this side.
	for _, name := range names {
		problems = append(problems, map[string]any{
			"anchor":    name,
			"direction": directionMissingInBinding,
		})
	}

	if len(problems) == 0 {
		return nil
	}

	sort.SliceStable(problems, func(i, j int) bool {
		ai, aj := problems[i]["anchor"].(string), problems[j]["anchor"].(string)
		if ai != aj {
			return ai < aj
		}
		return problems[i]["direction"].(string) < problems[j]["direction"].(string)
	})

	return verr.NewCodedErrorWithDetails(verr.VELLUM_BIND_ANCHOR_UNRECONCILED,
		"the template's discovered anchors and the binding's statements do not reconcile",
		map[string]any{"problems": problems, "problem_count": len(problems)})
}
