package theme

import (
	"context"
	"sort"

	verr "github.com/frankbardon/vellum/errors"
)

// Provider resolves a theme id to a theme document.
//
// It is a seam with an inert default, in the style of every other seam in this
// library: a host that wires nothing still gets a working library over the
// built-in theme, rather than a construction failure. Wiring one is how a
// consumer serves their own themes from their own storage, without Vellum
// learning anything about that storage.
type Provider interface {
	// Theme returns the document under an id. An empty id means the built-in
	// theme.
	//
	// An id the provider does not carry must be VELLUM_THEME_NOT_FOUND rather
	// than a substituted default. Serving a different theme than the one asked
	// for produces a document that is wrong in a way that looks right, which is
	// the worst kind of wrong a document library can produce.
	Theme(ctx context.Context, id string) (*Theme, error)
}

// BuiltinProvider serves the theme Vellum ships and nothing else.
//
// This is the inert default. Its zero value works, and it is what a host that
// wires no provider gets.
type BuiltinProvider struct{}

// Theme implements [Provider].
func (BuiltinProvider) Theme(_ context.Context, id string) (*Theme, error) {
	if id == "" || id == BuiltinID {
		return Builtin()
	}
	return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_THEME_NOT_FOUND,
		"no theme is registered under this id",
		map[string]any{"theme_id": id, "available": []string{BuiltinID}})
}

// StaticProvider serves a fixed set of themes, plus the built-in one.
//
// Useful for a host whose themes are known at construction, and for tests. A
// host loading themes from storage implements [Provider] directly instead;
// this type is a convenience, not the seam.
type StaticProvider struct {
	// themes is keyed by id. A map is safe here because nothing iterates it
	// on an output path — lookups are by key, and Available sorts what it
	// returns.
	themes map[string]*Theme
}

// NewStaticProvider registers the given themes, validating each.
//
// Validating at registration rather than at first use is deliberate: a broken
// theme should fail where it is wired, not on whichever render first reaches
// for it.
func NewStaticProvider(docs ...*Theme) (*StaticProvider, error) {
	p := &StaticProvider{themes: make(map[string]*Theme, len(docs))}
	for _, t := range docs {
		if err := t.Validate(); err != nil {
			return nil, err
		}
		if _, exists := p.themes[t.ID]; exists {
			return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_THEME_INVALID,
				"two themes claim the same id", map[string]any{"theme_id": t.ID})
		}
		p.themes[t.ID] = t.Clone()
	}
	return p, nil
}

// Theme implements [Provider].
func (p *StaticProvider) Theme(ctx context.Context, id string) (*Theme, error) {
	if id == "" {
		id = BuiltinID
	}
	if t, ok := p.themes[id]; ok {
		return t.Clone(), nil
	}
	if id == BuiltinID {
		return Builtin()
	}
	return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_THEME_NOT_FOUND,
		"no theme is registered under this id",
		map[string]any{"theme_id": id, "available": p.Available()})
}

// Available lists the registered ids, sorted bytewise.
//
// Sorted rather than in registration order: this reaches a consumer and an
// error's details, and both want the same answer twice.
func (p *StaticProvider) Available() []string {
	out := make([]string, 0, len(p.themes)+1)
	seenBuiltin := false
	for id := range p.themes {
		out = append(out, id)
		if id == BuiltinID {
			seenBuiltin = true
		}
	}
	if !seenBuiltin {
		out = append(out, BuiltinID)
	}
	sort.Strings(out)
	return out
}

// Resolve is the dispatch point every caller uses.
//
// A nil provider is the inert default rather than a panic, which is what makes
// "wire nothing and it still works" true at the call site rather than only in
// the documentation.
func Resolve(ctx context.Context, p Provider, id string) (*Theme, error) {
	if p == nil {
		return BuiltinProvider{}.Theme(ctx, id)
	}
	t, err := p.Theme(ctx, id)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_THEME_NOT_FOUND,
			"the provider returned no theme and no error",
			map[string]any{"theme_id": id})
	}
	// Validated here as well as at registration, because a Provider is a host
	// implementation and this is the boundary where a host's output becomes
	// Vellum's input. Trusting it would move a theme bug into whichever writer
	// first dereferenced the missing field.
	if err := t.Validate(); err != nil {
		return nil, err
	}
	return t, nil
}
