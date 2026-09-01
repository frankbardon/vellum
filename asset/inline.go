package asset

import (
	"context"
	"encoding/base64"
	"net/url"
	"sort"
	"strings"

	"github.com/frankbardon/vellum/canon"
	verr "github.com/frankbardon/vellum/errors"
)

// dataScheme is the prefix of a handle carrying its own bytes.
const dataScheme = "data:"

// Inline is the inert default resolver: it serves assets that carry their own
// bytes, and nothing else.
//
// A handle of the form data:image/png;base64,... is resolved from the handle
// itself. That is what makes "wire nothing and it still works" true rather than
// aspirational — a caller can compose a complete document with a picture in it
// without implementing anything, and without Vellum having acquired an opinion
// about storage.
//
// The zero value works. It reaches nothing: no filesystem, no network, no
// process. A handle that is not a data URI is VELLUM_ASSET_NOT_FOUND, which is
// the honest answer, because this resolver genuinely does not have it.
type Inline struct{}

// Resolve implements [Resolver].
func (Inline) Resolve(_ context.Context, req Request) (*Asset, error) {
	if !strings.HasPrefix(req.Handle, dataScheme) {
		return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_ASSET_NOT_FOUND,
			"the default resolver serves only inline data URIs; wire an asset.Resolver to serve stored assets",
			map[string]any{"handle": truncateHandle(req.Handle), "format": string(req.Format)})
	}
	mediaType, raw, err := decodeDataURI(req.Handle)
	if err != nil {
		return nil, err
	}
	return &Asset{Handle: req.Handle, MediaType: mediaType, Bytes: raw}, nil
}

// AssetHash implements [Hasher].
//
// An inline asset's bytes are in the handle, so the cheap path and the
// expensive path are the same path. Implementing the seam anyway keeps the
// default resolver a complete worked example of one.
func (Inline) AssetHash(_ context.Context, handle string) (string, bool, error) {
	if !strings.HasPrefix(handle, dataScheme) {
		return "", false, nil
	}
	_, raw, err := decodeDataURI(handle)
	if err != nil {
		return "", false, err
	}
	return canon.BytesHash(hashTag, raw), true, nil
}

// decodeDataURI splits a data URI into its media type and its bytes.
//
// Both the base64 and the percent-encoded forms are accepted, because both are
// what a caller will actually have: base64 from a tool that produced one, and
// percent-encoded from an SVG somebody pasted.
func decodeDataURI(handle string) (string, []byte, error) {
	rest := handle[len(dataScheme):]
	comma := strings.IndexByte(rest, ',')
	if comma < 0 {
		return "", nil, verr.NewCodedErrorWithDetails(verr.VELLUM_ASSET_NOT_FOUND,
			"the handle looks like a data URI but has no comma separating its metadata from its content",
			map[string]any{"handle": truncateHandle(handle)})
	}
	meta, payload := rest[:comma], rest[comma+1:]

	isBase64 := false
	mediaType := ""
	for i, part := range strings.Split(meta, ";") {
		switch {
		case part == "base64":
			isBase64 = true
		case i == 0 && part != "":
			mediaType = NormaliseMedia(part)
		}
	}

	var raw []byte
	if isBase64 {
		decoded, err := base64.StdEncoding.DecodeString(payload)
		if err != nil {
			return "", nil, verr.WrapCodedErrorWithDetails(err, verr.VELLUM_ASSET_NOT_FOUND,
				"the data URI's base64 payload does not decode",
				map[string]any{"handle": truncateHandle(handle)})
		}
		raw = decoded
	} else {
		unescaped, err := url.PathUnescape(payload)
		if err != nil {
			return "", nil, verr.WrapCodedErrorWithDetails(err, verr.VELLUM_ASSET_NOT_FOUND,
				"the data URI's percent-encoded payload does not decode",
				map[string]any{"handle": truncateHandle(handle)})
		}
		raw = []byte(unescaped)
	}
	// The declared media type is returned as-is and left empty when absent, so
	// ingestion sniffs rather than trusting a data URI's own claim about
	// itself when it made none.
	return mediaType, raw, nil
}

// truncateHandle bounds a handle in an error's details.
//
// A data URI can be megabytes long, and an error carrying the whole payload
// would make a diagnostic bigger than the thing it diagnoses — and would put
// the asset's bytes into any log the envelope reaches.
func truncateHandle(h string) string {
	const limit = 96
	if len(h) <= limit {
		return h
	}
	return h[:limit] + "…"
}

// Map is a resolver over an in-memory set of handles.
//
// For a host whose assets are known at construction, and for tests. A host
// reading from storage implements [Resolver] directly; this is a convenience,
// not the seam.
type Map struct {
	// entries is keyed by handle. A map is safe here because nothing ranges it
	// on an output path — lookups are by key, and Handles sorts what it
	// returns.
	entries map[string]Asset
}

// NewMap registers assets by handle.
func NewMap(entries map[string]Asset) *Map {
	m := &Map{entries: make(map[string]Asset, len(entries))}
	for handle, a := range entries {
		a.Handle = handle
		m.entries[handle] = a
	}
	return m
}

// Resolve implements [Resolver], falling through to [Inline] for data URIs so
// that registering a map never costs a caller the inline default.
func (m *Map) Resolve(ctx context.Context, req Request) (*Asset, error) {
	if a, ok := m.entries[req.Handle]; ok {
		out := a
		out.Bytes = append([]byte(nil), a.Bytes...)
		return &out, nil
	}
	if strings.HasPrefix(req.Handle, dataScheme) {
		return Inline{}.Resolve(ctx, req)
	}
	return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_ASSET_NOT_FOUND,
		"no asset is registered under this handle",
		map[string]any{"handle": truncateHandle(req.Handle),
			"format": string(req.Format), "available": m.Handles()})
}

// Handles lists the registered handles, sorted bytewise.
func (m *Map) Handles() []string {
	out := make([]string, 0, len(m.entries))
	for h := range m.entries {
		out = append(out, h)
	}
	sort.Strings(out)
	return out
}
