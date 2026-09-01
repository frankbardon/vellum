// Package asset is the seam between Vellum and wherever a host keeps its
// pictures.
//
// Vellum owns nothing and fetches nothing. A specification references an asset
// by handle, never by bytes, and the host resolves it. That is what keeps the
// library ignorant of the host's storage — which is precisely what makes it
// reusable rather than one product's document writer — and it is also what
// makes the content hash cheap enough to compute before deciding whether to
// render at all.
//
// # The request carries the target format
//
// A resolver is asked for an asset *for a particular render*, because the
// target format constrains what can be embedded. PDF has no SVG mechanism at
// all. Vellum will not rasterise — it never renders — and will not ship an
// SVG-to-PDF translator, which would be a second renderer with its own text
// layout and font matching, free to drift from whatever produced the asset. So
// the format and the media types it accepts travel with the request, a host
// with several encodings can serve the right one, and a host with only the
// wrong one gets a loud error naming the set instead of a silent omission.
package asset

import (
	"context"
	"sort"
	"strings"

	"github.com/frankbardon/vellum/artifact"
	"github.com/frankbardon/vellum/canon"
	verr "github.com/frankbardon/vellum/errors"
)

// hashTag namespaces asset hashes. See [canon.BytesHash].
const hashTag = "vellum.asset"

// Media types Vellum knows how to reason about.
const (
	MediaPNG  = "image/png"
	MediaJPEG = "image/jpeg"
	MediaSVG  = "image/svg+xml"
)

// Request is what a resolver is asked for.
type Request struct {
	// Handle is the host's opaque identifier, verbatim from the specification.
	// Vellum does not interpret it, does not treat it as a path, and does not
	// derive a media type from it.
	Handle string

	// Format is the render this asset is for.
	Format artifact.Format

	// Accept is the media types this render can embed, most preferred first.
	//
	// A host holding several encodings of the same asset uses it to serve the
	// right one. A host holding one ignores it, and the mismatch is caught
	// after the fact by [CheckMedia] — so Accept is an optimisation for the
	// host, never the enforcement.
	Accept []string
}

// Asset is a resolved asset: the bytes, what they are, and how big.
type Asset struct {
	// Handle is the request's handle, echoed back.
	Handle string

	// MediaType is the IANA type. A resolver that knows it should say so;
	// one that does not leaves it empty and Vellum sniffs the bytes.
	MediaType string

	// Bytes is the content.
	Bytes []byte

	// Hash is the content hash, filled in by [Ingest]. A resolver need not set
	// it; one that does has it overwritten, because a hash Vellum did not
	// compute is a hash Vellum cannot stand behind.
	Hash string

	// WidthPx and HeightPx are the intrinsic pixel dimensions, zero when
	// unknown. They are what a box with an intrinsic height needs in order to
	// have one: the aspect ratio comes from the asset, and the asset is the
	// only place it can come from.
	WidthPx  float64
	HeightPx float64
}

// AspectRatio returns width divided by height, and whether it is known.
func (a *Asset) AspectRatio() (float64, bool) {
	if a == nil || a.WidthPx <= 0 || a.HeightPx <= 0 {
		return 0, false
	}
	return a.WidthPx / a.HeightPx, true
}

// Resolver turns a handle into bytes.
//
// A seam with an inert default: a host that wires nothing still gets a working
// library over inline assets, rather than a construction failure.
type Resolver interface {
	// Resolve returns the asset for a request.
	//
	// A handle the resolver does not carry must be VELLUM_ASSET_NOT_FOUND.
	// Returning a placeholder would put a picture in a document that nobody
	// asked for, which is worse than failing.
	Resolve(ctx context.Context, req Request) (*Asset, error)
}

// Hasher is an optional seam a [Resolver] may also implement.
//
// It lets a host answer "what is this asset's content hash" without moving the
// bytes. That is what makes artifact naming cheap enough to do *before*
// deciding whether to render: the name is an input, so a consumer can ask
// whether the artifact already exists and skip the render entirely — but only
// if asking is cheaper than rendering.
//
// A resolver that does not implement it is not an error and not a degradation.
// The assertion simply fails and Vellum hashes the bytes itself, which is the
// same answer by a slower route.
type Hasher interface {
	// AssetHash returns the content hash for a handle. The boolean reports
	// whether this resolver can answer for it at all; false means "ask me for
	// the bytes instead", not "no such asset".
	AssetHash(ctx context.Context, handle string) (string, bool, error)
}

// Ingest resolves, types, measures and hashes an asset.
//
// This is the only path into the library for asset bytes, so every asset that
// reaches a writer has been through the same four steps in the same order —
// rather than each writer doing its own subset of them slightly differently.
func Ingest(ctx context.Context, r Resolver, req Request, opts Options) (*Asset, error) {
	if r == nil {
		r = Inline{}
	}
	maxBytes := opts.MaxBytes
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}

	a, err := r.Resolve(ctx, req)
	if err != nil {
		// A coded error from the resolver is passed through: a host that took
		// the trouble to say VELLUM_ASSET_NOT_FOUND should not have it
		// re-wrapped as something vaguer.
		if _, ok := verr.CodeOf(err); ok {
			return nil, err
		}
		return nil, verr.WrapCodedErrorWithDetails(err, verr.VELLUM_ASSET_RESOLVE_FAILED,
			"the asset resolver returned an error",
			map[string]any{"handle": req.Handle, "format": string(req.Format)})
	}
	if a == nil {
		return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_ASSET_NOT_FOUND,
			"the resolver returned no asset and no error",
			map[string]any{"handle": req.Handle})
	}
	if int64(len(a.Bytes)) > maxBytes {
		return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_ASSET_TOO_LARGE,
			"the resolved asset exceeds the configured size bound",
			map[string]any{"handle": req.Handle,
				"size_bytes": len(a.Bytes), "limit_bytes": maxBytes})
	}

	a.Handle = req.Handle
	sniffed, sniffOK := SniffMedia(a.Bytes)
	switch {
	case a.MediaType == "" && !sniffOK:
		return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_ASSET_MEDIA_UNKNOWN,
			"the asset's media type was not declared and its bytes match no known signature",
			map[string]any{"handle": truncateHandle(req.Handle), "known": KnownMedia()})

	case a.MediaType == "":
		a.MediaType = sniffed

	default:
		a.MediaType = NormaliseMedia(a.MediaType)
		// A declaration is checked against the bytes rather than believed.
		// Writing bytes into a package under a content type they contradict
		// makes a reader that checks — Word and Excel both do — refuse the
		// whole document, and the failure then reads as "this file is corrupt"
		// several layers from the mistake.
		//
		// Only a contradiction fails. Bytes that match no signature Vellum
		// knows are not evidence against the declaration, because Vellum's
		// signature set is small and the host's knowledge of its own storage is
		// better than a sniffer's.
		if sniffOK && sniffed != a.MediaType {
			return nil, verr.NewCodedErrorWithDetails(verr.VELLUM_ASSET_MEDIA_MISMATCH,
				"the asset's declared media type contradicts its own bytes",
				map[string]any{
					"handle":              truncateHandle(req.Handle),
					"declared_media_type": a.MediaType,
					"sniffed_media_type":  sniffed,
				})
		}
	}

	// Measured after typing, because the probe is chosen by media type rather
	// than by guessing at the bytes twice.
	if a.WidthPx == 0 && a.HeightPx == 0 {
		w, h, ok := Measure(a.MediaType, a.Bytes)
		if ok {
			a.WidthPx, a.HeightPx = w, h
		}
	}

	// Hashed last and unconditionally, overwriting whatever the resolver set.
	// A hash Vellum did not compute is a hash Vellum cannot stand behind, and
	// this one names the artifact.
	a.Hash = canon.BytesHash(hashTag, a.Bytes)
	return a, nil
}

// Options configures ingestion.
type Options struct {
	// MaxBytes bounds one resolved asset. Zero selects [DefaultMaxBytes].
	MaxBytes int64
}

// DefaultMaxBytes is the per-asset bound when none is configured.
//
// Generous, because legitimate assets genuinely are large and a bound that
// refuses real input gets disabled — which is worse than no bound at all.
const DefaultMaxBytes int64 = 64 << 20 // 64 MiB

// HashFor returns an asset's content hash, using the [Hasher] seam when the
// resolver offers it and falling back to reading the bytes when it does not.
//
// The fallback is the point. The optional seam makes the cheap path available
// without making it required, so a host that has not implemented it gets a
// correct answer slowly rather than an error.
func HashFor(ctx context.Context, r Resolver, req Request, opts Options) (string, error) {
	if r == nil {
		r = Inline{}
	}
	if h, ok := r.(Hasher); ok {
		hash, answered, err := h.AssetHash(ctx, req.Handle)
		if err != nil {
			if _, coded := verr.CodeOf(err); coded {
				return "", err
			}
			return "", verr.WrapCodedErrorWithDetails(err, verr.VELLUM_ASSET_RESOLVE_FAILED,
				"the asset resolver's hasher returned an error",
				map[string]any{"handle": req.Handle})
		}
		if answered {
			return hash, nil
		}
	}
	a, err := Ingest(ctx, r, req, opts)
	if err != nil {
		return "", err
	}
	return a.Hash, nil
}

// CheckMedia reports whether a media type is in an accepted set.
//
// The enforcement of the format-aware media policy, kept here rather than in
// the capability package so that this package stays ignorant of the matrix and
// the matrix stays the single declaration of what each format accepts. The
// caller supplies the set; this decides.
func CheckMedia(handle, mediaType string, format artifact.Format, accepted []string) error {
	normalised := NormaliseMedia(mediaType)
	for _, m := range accepted {
		if NormaliseMedia(m) == normalised {
			return nil
		}
	}
	sorted := append([]string(nil), accepted...)
	sort.Strings(sorted)
	return verr.NewCodedErrorWithDetails(verr.VELLUM_ASSET_MEDIA_UNSUPPORTED,
		"the target format cannot embed an asset of this media type",
		map[string]any{
			"handle":     handle,
			"media_type": normalised,
			"format":     string(format),
			"accepted":   sorted,
		})
}

// NormaliseMedia lowercases a media type and drops any parameters, so
// "IMAGE/SVG+XML; charset=utf-8" compares equal to "image/svg+xml".
func NormaliseMedia(m string) string {
	if i := strings.IndexByte(m, ';'); i >= 0 {
		m = m[:i]
	}
	return strings.ToLower(strings.TrimSpace(m))
}

// KnownMedia returns the media types Vellum can sniff, sorted.
func KnownMedia() []string {
	out := []string{MediaJPEG, MediaPNG, MediaSVG}
	sort.Strings(out)
	return out
}
