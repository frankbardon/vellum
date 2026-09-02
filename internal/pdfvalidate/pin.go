package pdfvalidate

// The validator pins.
//
// This file carries no build tag on purpose. They are read by a hygiene gate
// that runs in the ordinary suite: a digest named both here and in a CI
// workflow is a digest that will eventually disagree with itself, and putting
// these behind the verapdf tag would mean the gate could only run on a machine
// where the validator already does.

// EnvVeraPDF names the environment variable holding an explicit veraPDF path,
// or a digest-pinned container reference prefixed with [ContainerPrefix].
const EnvVeraPDF = "VELLUM_VERAPDF"

// ContainerPrefix marks a [EnvVeraPDF] value as a container reference rather
// than a path.
//
// Explicit rather than inferred. A reference and an absolute path are not
// reliably distinguishable — "/Applications/veraPDF/verapdf" and
// "verapdf/cli:latest" differ only by a leading slash — and guessing wrong
// produces "not found" for a value that was perfectly well formed.
const ContainerPrefix = "container:"

// PinnedImage is the container the corpus's conformance verdicts were
// established against.
//
// By digest, never by tag. A tag is a name someone else may repoint, so a gate
// running a tagged image reports a verdict from whatever the registry held at
// the moment it ran — and a corpus that passed yesterday and fails today would
// have changed nothing. Bumping this is a deliberate act: the digest moves in
// one commit, with whatever the new validator now says about the corpus.
//
// verapdf/cli:v1.28.1, resolved 2026-09-01.
const PinnedImage = "docker.io/verapdf/cli@sha256:" +
	"24e4e1b22cc2805ab2fdd615efb1bdd08e7b67139b53d75221ba7037105bcb23"

// PinnedVersion is the release [PinnedImage] carries, and the one a local
// installation must also be, checked against the version the validator states
// in its own report.
//
// It is not redundant with the digest. The digest pins what CI runs; this pins
// what a developer's own installation is, and the two have to agree or a local
// pass means nothing about the gate. Both move together.
const PinnedVersion = "1.28.1"
