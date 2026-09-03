# Installation

Vellum is a Go library first, with a CLI and an MCP server built on top of it.
All three ship from the same module.

## Requirements

- Go 1.26 or later (the module's own `go.mod` pins `go 1.26.1`).
- No C toolchain. `CGO_ENABLED=0` is exported by the project's `Makefile` as a
  build contract, not a preference — nothing in Vellum imports `"C"`, ever.
- No external binaries and no network access at runtime. The library never
  shells out and never dials anything.

## As a library

```sh
go get github.com/frankbardon/vellum
```

Import the root package for the facade, and the leaf packages you need
directly for their own public types:

```go
import (
    "github.com/frankbardon/vellum"
    "github.com/frankbardon/vellum/spec"
    "github.com/frankbardon/vellum/artifact"
)
```

`vellum` re-exports the common types a caller touches most often (`vellum.Spec`
is an alias for `spec.Spec`, and so on) so an embedder can write against one
import in the common case — see [`aliases.go`](../library/facade.md) — while
still being able to reach into `theme`, `asset`, `template` or any other leaf
package for deeper configuration.

## As a CLI

Build the `vellum` binary from a checkout:

```sh
git clone https://github.com/frankbardon/vellum
cd vellum
make build   # writes bin/vellum, version-stamped via -X main.version=$(VERSION)
```

or install it directly with `go install`:

```sh
go install github.com/frankbardon/vellum/cmd/vellum@latest
```

Run `vellum doctor` after installing to check the local environment: the
built-in theme and its fonts, every `VELLUM_*` environment variable's current
setting, the PDF/A sRGB ICC profile, and write permission on the current
directory. See [Commands and flags](../cli/flags.md) for the full command
index.

## As an MCP server

`vellum mcp` runs a Model Context Protocol server over stdio — no separate
install step beyond building or installing the binary above. See
[MCP Integration](../mcp/index.md).

## Verifying a build

```sh
make test    # go test ./...
make lint    # go vet + staticcheck
```

Both are required CI contexts on every pull request; running them locally
before committing catches most problems immediately rather than in review.
`make test-office` and `make test-pdfa` additionally run build-tagged
conformance gates against a locally installed LibreOffice and veraPDF — see
[Determinism](../internals/determinism.md) for what they check and why they
are optional locally but required in CI.
