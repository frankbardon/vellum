BINARY_NAME=vellum
BUILD_DIR=bin
GO=go
VERSION?=dev
LDFLAGS=-s -w -X main.version=$(VERSION)
BUILD_FLAGS=-trimpath -ldflags="$(LDFLAGS)"

# CGO_ENABLED=0 is a contract, not a preference. Vellum must `go get` and build
# without a C toolchain on every platform, and nothing on the render path may
# depend on a system library. A future import of "C" fails the build here rather
# than silently reintroducing cgo.
export CGO_ENABLED=0

ifneq (,$(wildcard ./.env))
    include .env
    export
endif

.DEFAULT_GOAL := build

.PHONY: build clean test cover fmt vet lint bench docs docs-serve docs-clean

build:
	$(GO) build $(BUILD_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/vellum

clean:
	rm -rf $(BUILD_DIR) coverage.out

test:
	$(GO) test ./...

cover:
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

lint: vet
	$(GO) run honnef.co/go/tools/cmd/staticcheck@latest ./...

# Benchmarks are manual and deliberately not wired into `make test`. Pipe the
# output through benchstat against a saved baseline; this target asserts no
# threshold of its own, because a wall-clock assertion in CI is a flake.
bench:
	$(GO) test -bench=. -benchmem -run='^$$' -count=1 ./opc/... ./pdf/... ./doc/... ./sheet/... ./deck/...

docs:
	mdbook build docs

docs-serve:
	mdbook serve docs --open

docs-clean:
	rm -rf docs/book
