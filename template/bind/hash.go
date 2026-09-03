package bind

import "github.com/frankbardon/vellum/canon"

// hashTag namespaces a binding hash. See [canon.CanonicalHash].
//
// Distinct from every other tag in the tree — "spec", "vellum.asset",
// "provenance" and so on — because two different content types whose
// canonical JSON happened to coincide must never collide under one hash: a
// binding and a specification that both serialised to {} would otherwise be
// indistinguishable to a caller keying a cache on the hash alone.
const hashTag = "bind"

// Hash returns the binding's canonical content hash.
//
// The guarantees mirror [spec.Spec.Hash] exactly, because a binding is
// reviewed and diffed the same way a specification is:
//
//   - the same logical binding produces the same hash across processes and
//     across Vellum versions;
//   - field order does not affect it, so JSON and YAML authoring agree;
//   - an omitted format_version and the current one hash alike;
//   - adding a new omitempty field to this model does not move the hash of a
//     binding that omits it.
func (b *Binding) Hash() string {
	if b == nil {
		return canon.CanonicalHash(hashTag, (*Binding)(nil))
	}
	return canon.CanonicalHash(hashTag, b.normalizedForHash())
}

// normalizedForHash returns a copy with defaults applied, on a clone so that
// asking for a hash never mutates the caller's binding.
func (b *Binding) normalizedForHash() *Binding {
	out := &Binding{FormatVersion: b.FormatVersion}
	if out.FormatVersion == "" {
		out.FormatVersion = FormatVersion
	}
	out.Statements = normalizeStatements(b.Statements)
	return out
}

// normalizeStatements collapses a nil slice and an empty one to the same
// representation, so an author who writes body: [] and one who omits body
// entirely produce the same hash.
func normalizeStatements(in []Statement) []Statement {
	if len(in) == 0 {
		return nil
	}
	out := make([]Statement, len(in))
	for i := range in {
		out[i] = normalizeStatement(&in[i])
	}
	return out
}

func normalizeStatement(in *Statement) Statement {
	out := Statement{Kind: in.Kind, Skip: in.Skip}
	switch in.Kind {
	case StatementBind:
		if in.Bind != nil {
			bv := *in.Bind
			out.Bind = &bv
		}
	case StatementRepeat:
		if in.Repeat != nil {
			r := Repeat{Over: in.Repeat.Over, As: in.Repeat.As, Target: in.Repeat.Target}
			r.Body = normalizeStatements(in.Repeat.Body)
			out.Repeat = &r
		}
	case StatementIf:
		if in.If != nil {
			iv := If{When: in.If.When}
			iv.Then = normalizeStatements(in.If.Then)
			iv.Else = normalizeStatements(in.If.Else)
			out.If = &iv
		}
	case StatementWith:
		if in.With != nil {
			w := With{As: in.With.As, Value: in.With.Value}
			w.Body = normalizeStatements(in.With.Body)
			out.With = &w
		}
	}
	return out
}
