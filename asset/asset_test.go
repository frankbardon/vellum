package asset_test

import (
	"context"
	"encoding/base64"
	stderrors "errors"
	"fmt"
	"strings"
	"testing"

	"github.com/frankbardon/vellum/artifact"
	"github.com/frankbardon/vellum/asset"
	"github.com/frankbardon/vellum/capability"
	verr "github.com/frankbardon/vellum/errors"
)

// onePixelPNG is a real 1x1 PNG, so the sniffer and the probe are tested
// against a file a reader would accept rather than against a header this test
// invented.
var onePixelPNG = mustDecode(`iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==`)

// tinyJPEG is a minimal baseline JPEG, 8x8.
var tinyJPEG = mustDecode(`/9j/4AAQSkZJRgABAQEAYABgAAD/2wBDAAgGBgcGBQgHBwcJCQgKDBQNDAsLDBkSEw8UHRofHh0aHBwgJC4nICIsIxwcKDcpLDAxNDQ0Hyc5PTgyPC4zNDL/wAALCAAIAAgBAREA/8QAHwAAAQUBAQEBAQEAAAAAAAAAAAECAwQFBgcICQoL/8QAtRAAAgEDAwIEAwUFBAQAAAF9AQIDAAQRBRIhMUEGE1FhByJxFDKBkaEII0KxwRVS0fAkM2JyggkKFhcYGRolJicoKSo0NTY3ODk6Q0RFRkdISUpTVFVWV1hZWmNkZWZnaGlqc3R1dnd4eXqDhIWGh4iJipKTlJWWl5iZmqKjpKWmp6ipqrKztLW2t7i5usLDxMXGx8jJytLT1NXW19jZ2uHi4+Tl5ufo6erx8vP09fb3+Pn6/9oACAEBAAA/APn+iiigD//Z`)

func mustDecode(s string) []byte {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

const tinySVG = `<?xml version="1.0"?>
<!-- a chart -->
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 640 360"><rect width="640" height="360"/></svg>`

func dataURI(mediaType string, b []byte) string {
	return "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(b)
}

func TestSniffMedia(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want string
		ok   bool
	}{
		{"png", onePixelPNG, asset.MediaPNG, true},
		{"jpeg", tinyJPEG, asset.MediaJPEG, true},
		{"svg with a prologue", []byte(tinySVG), asset.MediaSVG, true},
		{"svg with a BOM", append([]byte{0xEF, 0xBB, 0xBF}, tinySVG...), asset.MediaSVG, true},
		{"bare svg root", []byte(`<svg width="10" height="10"/>`), asset.MediaSVG, true},
		{"empty", nil, "", false},
		{"plain text", []byte("hello"), "", false},
		// XML that merely mentions SVG must not match: a sniffer that keyed on
		// the string rather than the root element would type a rels part as a
		// picture.
		{"xml mentioning svg", []byte(`<?xml version="1.0"?><notes>about svg files</notes>`), "", false},
		{"truncated png signature", onePixelPNG[:4], "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := asset.SniffMedia(tc.in)
			if ok != tc.ok || got != tc.want {
				t.Errorf("SniffMedia = (%q, %v), want (%q, %v)", got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestMeasure(t *testing.T) {
	cases := []struct {
		name  string
		media string
		in    []byte
		wantW float64
		wantH float64
		ok    bool
	}{
		{"png", asset.MediaPNG, onePixelPNG, 1, 1, true},
		{"jpeg", asset.MediaJPEG, tinyJPEG, 8, 8, true},
		{"svg from viewBox", asset.MediaSVG, []byte(tinySVG), 640, 360, true},
		{"svg from width and height", asset.MediaSVG,
			[]byte(`<svg width="800" height="600" viewBox="0 0 4 3"/>`), 800, 600, true},
		{"svg with units", asset.MediaSVG,
			[]byte(`<svg width="10cm" height="5cm"/>`), 10, 5, true},
		// A percentage of an unstated container is not a size, so the viewBox
		// is the only answer available and its absence means no answer.
		{"svg with percentage width", asset.MediaSVG,
			[]byte(`<svg width="100%" height="100%"/>`), 0, 0, false},
		{"svg with neither", asset.MediaSVG, []byte(`<svg/>`), 0, 0, false},
		{"unknown media", "application/pdf", onePixelPNG, 0, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w, h, ok := asset.Measure(tc.media, tc.in)
			if ok != tc.ok || w != tc.wantW || h != tc.wantH {
				t.Errorf("Measure = (%v, %v, %v), want (%v, %v, %v)", w, h, ok, tc.wantW, tc.wantH, tc.ok)
			}
		})
	}
}

// TestMeasure_SVGViewBoxOnlyIsTheMotivatingCase states why the layout query
// exists, as a test rather than as a comment. A viewBox-only asset has a ratio
// and no size, so nothing downstream can decide how big to draw it — the host
// must be told the target box instead.
func TestMeasure_SVGViewBoxOnlyIsTheMotivatingCase(t *testing.T) {
	w, h, ok := asset.Measure(asset.MediaSVG, []byte(tinySVG))
	if !ok {
		t.Fatal("a viewBox-only SVG must still yield a ratio")
	}
	if got := w / h; got < 1.77 || got > 1.78 {
		t.Errorf("aspect ratio = %v, want 16:9", got)
	}
}

func TestIngest_InlineDefaultNeedsNoWiring(t *testing.T) {
	// A nil resolver is the wire-nothing case, and it must produce a complete
	// asset rather than a construction failure.
	got, err := asset.Ingest(context.Background(), nil,
		asset.Request{Handle: dataURI(asset.MediaPNG, onePixelPNG), Format: artifact.FormatDOCX},
		asset.Options{})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if got.MediaType != asset.MediaPNG {
		t.Errorf("MediaType = %q, want %q", got.MediaType, asset.MediaPNG)
	}
	if got.WidthPx != 1 || got.HeightPx != 1 {
		t.Errorf("dimensions = %vx%v, want 1x1", got.WidthPx, got.HeightPx)
	}
	if len(got.Hash) != 32 {
		t.Errorf("Hash = %q, want 32 hex characters", got.Hash)
	}
}

func TestIngest_SniffsWhenTheResolverDeclaresNothing(t *testing.T) {
	// A data URI with no media type: the bytes are the only evidence.
	handle := "data:;base64," + base64.StdEncoding.EncodeToString(onePixelPNG)
	got, err := asset.Ingest(context.Background(), nil,
		asset.Request{Handle: handle, Format: artifact.FormatDOCX}, asset.Options{})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if got.MediaType != asset.MediaPNG {
		t.Errorf("MediaType = %q, want %q", got.MediaType, asset.MediaPNG)
	}
}

func TestIngest_UnknownMediaIsAnError(t *testing.T) {
	handle := "data:;base64," + base64.StdEncoding.EncodeToString([]byte("not a picture"))
	_, err := asset.Ingest(context.Background(), nil,
		asset.Request{Handle: handle, Format: artifact.FormatDOCX}, asset.Options{})
	if !verr.HasCode(err, verr.VELLUM_ASSET_MEDIA_UNKNOWN) {
		t.Fatalf("error = %v, want VELLUM_ASSET_MEDIA_UNKNOWN", err)
	}
}

// TestIngest_RejectsADeclarationTheBytesContradict is the guard against the
// defect class that is invisible until a strict reader meets it: a part written
// under a content type it does not match. Word and Excel both check, and both
// refuse the whole document rather than the one part.
func TestIngest_RejectsADeclarationTheBytesContradict(t *testing.T) {
	m := asset.NewMap(map[string]asset.Asset{
		"mislabelled": {MediaType: asset.MediaPNG, Bytes: tinyJPEG},
	})
	_, err := asset.Ingest(context.Background(), m,
		asset.Request{Handle: "mislabelled", Format: artifact.FormatDOCX}, asset.Options{})

	if !verr.HasCode(err, verr.VELLUM_ASSET_MEDIA_MISMATCH) {
		t.Fatalf("error = %v, want VELLUM_ASSET_MEDIA_MISMATCH", err)
	}

	var coded *verr.CodedError
	if !stderrors.As(err, &coded) {
		t.Fatal("error is not a CodedError")
	}
	// Both types are in the details, because "they disagree" is not actionable
	// without knowing which is which.
	if coded.Details["declared_media_type"] != asset.MediaPNG {
		t.Errorf("declared_media_type = %v, want %q", coded.Details["declared_media_type"], asset.MediaPNG)
	}
	if coded.Details["sniffed_media_type"] != asset.MediaJPEG {
		t.Errorf("sniffed_media_type = %v, want %q", coded.Details["sniffed_media_type"], asset.MediaJPEG)
	}
}

// TestIngest_TrustsADeclarationTheSnifferCannotJudge pins the other half of
// that rule. Vellum's signature set is small, and bytes it does not recognise
// are not evidence against a host that knows its own storage.
func TestIngest_TrustsADeclarationTheSnifferCannotJudge(t *testing.T) {
	m := asset.NewMap(map[string]asset.Asset{
		"exotic": {MediaType: "image/avif", Bytes: []byte("bytes vellum cannot sniff")},
	})
	got, err := asset.Ingest(context.Background(), m,
		asset.Request{Handle: "exotic", Format: artifact.FormatDOCX}, asset.Options{})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if got.MediaType != "image/avif" {
		t.Errorf("MediaType = %q, want image/avif", got.MediaType)
	}
	// Whether that type may then be embedded is the matrix's question, asked
	// separately by CheckMedia. Ingestion types an asset; it does not police
	// what a format accepts.
	if err := asset.CheckMedia("exotic", got.MediaType, artifact.FormatDOCX,
		capability.AcceptedMedia(artifact.FormatDOCX)); !verr.HasCode(err, verr.VELLUM_ASSET_MEDIA_UNSUPPORTED) {
		t.Errorf("CheckMedia = %v, want VELLUM_ASSET_MEDIA_UNSUPPORTED", err)
	}
}

// TestIngest_OverwritesAResolverSuppliedHash pins that the hash naming the
// artifact is always one Vellum computed.
func TestIngest_OverwritesAResolverSuppliedHash(t *testing.T) {
	m := asset.NewMap(map[string]asset.Asset{
		"chart": {MediaType: asset.MediaPNG, Bytes: onePixelPNG, Hash: "deadbeef"},
	})
	got, err := asset.Ingest(context.Background(), m,
		asset.Request{Handle: "chart", Format: artifact.FormatDOCX}, asset.Options{})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if got.Hash == "deadbeef" {
		t.Error("a resolver-supplied hash survived ingestion; a hash Vellum did not compute is one it cannot stand behind")
	}
}

func TestIngest_HashIsStableAndContentAddressed(t *testing.T) {
	ctx := context.Background()
	req := asset.Request{Handle: dataURI(asset.MediaPNG, onePixelPNG), Format: artifact.FormatDOCX}

	first, err := asset.Ingest(ctx, nil, req, asset.Options{})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	for range 100 {
		again, err := asset.Ingest(ctx, nil, req, asset.Options{})
		if err != nil {
			t.Fatalf("Ingest: %v", err)
		}
		if again.Hash != first.Hash {
			t.Fatalf("hash is not stable: %q then %q", first.Hash, again.Hash)
		}
	}

	// Different bytes, different hash — including when they arrive under the
	// same handle, because the hash names content and not identity.
	other, err := asset.Ingest(ctx, asset.NewMap(map[string]asset.Asset{
		"x": {MediaType: asset.MediaJPEG, Bytes: tinyJPEG},
	}), asset.Request{Handle: "x", Format: artifact.FormatDOCX}, asset.Options{})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if other.Hash == first.Hash {
		t.Error("two different assets share a hash")
	}
}

func TestIngest_EnforcesTheSizeBound(t *testing.T) {
	big := make([]byte, 4096)
	copy(big, onePixelPNG)
	m := asset.NewMap(map[string]asset.Asset{"big": {MediaType: asset.MediaPNG, Bytes: big}})

	_, err := asset.Ingest(context.Background(), m,
		asset.Request{Handle: "big", Format: artifact.FormatDOCX},
		asset.Options{MaxBytes: 1024})
	if !verr.HasCode(err, verr.VELLUM_ASSET_TOO_LARGE) {
		t.Fatalf("error = %v, want VELLUM_ASSET_TOO_LARGE", err)
	}
}

func TestIngest_MissingHandleIsNotFound(t *testing.T) {
	_, err := asset.Ingest(context.Background(), nil,
		asset.Request{Handle: "s3://bucket/chart.png", Format: artifact.FormatDOCX}, asset.Options{})
	if !verr.HasCode(err, verr.VELLUM_ASSET_NOT_FOUND) {
		t.Fatalf("error = %v, want VELLUM_ASSET_NOT_FOUND", err)
	}
}

// failingResolver returns a plain error, as a host's resolver would.
type failingResolver struct{ err error }

func (f failingResolver) Resolve(context.Context, asset.Request) (*asset.Asset, error) {
	return nil, f.err
}

func TestIngest_WrapsAHostError(t *testing.T) {
	sentinel := stderrors.New("s3: connection reset")
	_, err := asset.Ingest(context.Background(), failingResolver{sentinel},
		asset.Request{Handle: "chart", Format: artifact.FormatPDF}, asset.Options{})

	if !verr.HasCode(err, verr.VELLUM_ASSET_RESOLVE_FAILED) {
		t.Fatalf("error = %v, want VELLUM_ASSET_RESOLVE_FAILED", err)
	}
	// The host's own error must be reachable, so a host can unwrap to its own
	// type — but it is never serialised into the envelope, because a host's
	// error prose can carry paths and credentials.
	if !stderrors.Is(err, sentinel) {
		t.Error("the host's error is not reachable through Unwrap")
	}
}

// TestIngest_PassesThroughACodedHostError pins that a host taking the trouble
// to say VELLUM_ASSET_NOT_FOUND does not have it re-wrapped as something vaguer.
func TestIngest_PassesThroughACodedHostError(t *testing.T) {
	coded := verr.NewCodedError(verr.VELLUM_ASSET_NOT_FOUND, "gone")
	_, err := asset.Ingest(context.Background(), failingResolver{coded},
		asset.Request{Handle: "chart", Format: artifact.FormatPDF}, asset.Options{})

	if !verr.HasCode(err, verr.VELLUM_ASSET_NOT_FOUND) {
		t.Fatalf("error = %v, want VELLUM_ASSET_NOT_FOUND", err)
	}
}

// countingResolver records how often bytes were actually moved, so the Hasher
// seam's whole reason for existing is observable.
type countingResolver struct {
	inner  asset.Resolver
	hash   string
	calls  *int
	answer bool
}

func (c countingResolver) Resolve(ctx context.Context, req asset.Request) (*asset.Asset, error) {
	*c.calls++
	return c.inner.Resolve(ctx, req)
}

func (c countingResolver) AssetHash(context.Context, string) (string, bool, error) {
	if !c.answer {
		return "", false, nil
	}
	return c.hash, true, nil
}

func TestHashFor_UsesTheHasherSeamWithoutMovingBytes(t *testing.T) {
	calls := 0
	r := countingResolver{
		inner:  asset.NewMap(map[string]asset.Asset{"chart": {MediaType: asset.MediaPNG, Bytes: onePixelPNG}}),
		hash:   "0123456789abcdef0123456789abcdef",
		calls:  &calls,
		answer: true,
	}

	got, err := asset.HashFor(context.Background(), r,
		asset.Request{Handle: "chart", Format: artifact.FormatDOCX}, asset.Options{})
	if err != nil {
		t.Fatalf("HashFor: %v", err)
	}
	if got != r.hash {
		t.Errorf("hash = %q, want %q", got, r.hash)
	}
	if calls != 0 {
		t.Errorf("the resolver moved bytes %d times; the Hasher seam exists so it does not have to", calls)
	}
}

// TestHashFor_FallsBackWhenTheSeamDeclines is the property that makes the seam
// optional rather than required: a resolver that cannot answer gets a correct
// answer slowly, never an error.
func TestHashFor_FallsBackWhenTheSeamDeclines(t *testing.T) {
	calls := 0
	r := countingResolver{
		inner:  asset.NewMap(map[string]asset.Asset{"chart": {MediaType: asset.MediaPNG, Bytes: onePixelPNG}}),
		calls:  &calls,
		answer: false,
	}

	got, err := asset.HashFor(context.Background(), r,
		asset.Request{Handle: "chart", Format: artifact.FormatDOCX}, asset.Options{})
	if err != nil {
		t.Fatalf("HashFor: %v", err)
	}
	if len(got) != 32 {
		t.Errorf("hash = %q, want 32 hex characters", got)
	}
	if calls != 1 {
		t.Errorf("resolver called %d times, want 1", calls)
	}
}

// TestHashFor_AgreesWithIngest pins that the two routes to a hash produce the
// same hash. If they diverged, a consumer's dedupe would depend on which one
// happened to run.
func TestHashFor_AgreesWithIngest(t *testing.T) {
	ctx := context.Background()
	req := asset.Request{Handle: dataURI(asset.MediaPNG, onePixelPNG), Format: artifact.FormatDOCX}

	viaSeam, err := asset.HashFor(ctx, asset.Inline{}, req, asset.Options{})
	if err != nil {
		t.Fatalf("HashFor: %v", err)
	}
	ingested, err := asset.Ingest(ctx, asset.Inline{}, req, asset.Options{})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if viaSeam != ingested.Hash {
		t.Errorf("the Hasher seam and Ingest disagree: %q vs %q", viaSeam, ingested.Hash)
	}
}

// TestCheckMedia_PDFRejectsSVG is the decision the whole format-aware request
// exists for, checked against the live matrix rather than against a list this
// test wrote down.
func TestCheckMedia_PDFRejectsSVG(t *testing.T) {
	err := asset.CheckMedia("chart", asset.MediaSVG, artifact.FormatPDF,
		capability.AcceptedMedia(artifact.FormatPDF))

	if !verr.HasCode(err, verr.VELLUM_ASSET_MEDIA_UNSUPPORTED) {
		t.Fatalf("error = %v, want VELLUM_ASSET_MEDIA_UNSUPPORTED", err)
	}

	var coded *verr.CodedError
	if !stderrors.As(err, &coded) {
		t.Fatal("error is not a CodedError")
	}
	// The accepted set is in the details, because "supply something else" is
	// not actionable without knowing what else.
	accepted, ok := coded.Details["accepted"].([]string)
	if !ok || len(accepted) == 0 {
		t.Fatalf("details accepted = %v, want a non-empty set", coded.Details["accepted"])
	}
	for _, m := range accepted {
		if m == asset.MediaSVG {
			t.Error("the accepted set names SVG, which is what was just rejected")
		}
	}
}

func TestCheckMedia_AcceptsWhatEachFormatDeclares(t *testing.T) {
	for _, format := range artifact.AllFormats() {
		accepted := capability.AcceptedMedia(format)
		for _, m := range accepted {
			if err := asset.CheckMedia("h", m, format, accepted); err != nil {
				t.Errorf("%s declares %q accepted but CheckMedia refused it: %v", format, m, err)
			}
		}
	}
}

// TestCheckMedia_NormalisesParametersAndCase pins that a resolver returning a
// charset parameter or an uppercase type is not refused for spelling.
func TestCheckMedia_NormalisesParametersAndCase(t *testing.T) {
	accepted := capability.AcceptedMedia(artifact.FormatDOCX)
	for _, m := range []string{"IMAGE/PNG", "image/png; charset=binary", "  image/png  "} {
		if err := asset.CheckMedia("h", m, artifact.FormatDOCX, accepted); err != nil {
			t.Errorf("CheckMedia(%q) = %v, want accepted", m, err)
		}
	}
}

// TestErrorDetails_DoNotCarryTheWholePayload pins that a data URI's bytes stay
// out of an error, and therefore out of any log the envelope reaches.
func TestErrorDetails_DoNotCarryTheWholePayload(t *testing.T) {
	// A JPEG declared as a PNG, so the mismatch check fires and the error
	// carries the handle — which here is the entire asset, inline.
	handle := dataURI(asset.MediaPNG, tinyJPEG)
	if len(handle) < 256 {
		t.Fatalf("the fixture handle is only %d bytes; the test proves nothing", len(handle))
	}

	_, err := asset.Ingest(context.Background(), nil,
		asset.Request{Handle: handle, Format: artifact.FormatDOCX}, asset.Options{})
	if !verr.HasCode(err, verr.VELLUM_ASSET_MEDIA_MISMATCH) {
		t.Fatalf("error = %v, want VELLUM_ASSET_MEDIA_MISMATCH", err)
	}
	if n := len(fmt.Sprint(err)); n > 512 {
		t.Errorf("the error is %d bytes; a diagnostic must not be larger than what it diagnoses, "+
			"and an inline asset's bytes must not reach a log through it", n)
	}

	var coded *verr.CodedError
	if !stderrors.As(err, &coded) {
		t.Fatal("error is not a CodedError")
	}
	if got := coded.Details["handle"].(string); !strings.HasSuffix(got, "…") {
		t.Errorf("details handle = %q, want a truncated handle", got)
	}
}
