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

.PHONY: build clean test test-office warn-office cover fmt vet lint bench docs docs-serve docs-clean

build:
	$(GO) build $(BUILD_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/vellum

clean:
	rm -rf $(BUILD_DIR) coverage.out

test:
	$(GO) test ./...
	@$(MAKE) --no-print-directory warn-office

# SOFFICE resolves the LibreOffice binary the same way internal/oovalidate does,
# so the Makefile's warning and the test's skip cannot disagree about whether
# the tool is present.
SOFFICE := $(or $(VELLUM_SOFFICE),$(shell command -v soffice 2>/dev/null),$(shell command -v libreoffice 2>/dev/null),$(wildcard /Applications/LibreOffice.app/Contents/MacOS/soffice),$(wildcard /usr/bin/libreoffice))

# warn-office prints where `make test` stops short.
#
# The warning is here rather than left to the test's own t.Skip because `go
# test` prints nothing for a package that passed, so a skip is invisible in a
# non-verbose run — which is every run. An optional gate nobody is told about
# is an optional gate nobody provisions.
warn-office:
ifeq ($(strip $(SOFFICE)),)
	@printf '\n\033[33mWARNING\033[0m  no LibreOffice found: the office-reader checks did not run.\n'
	@printf '         Nothing in `make test` establishes that the .docx/.xlsx/.pptx artifacts open.\n'
	@printf '         Install LibreOffice, or set VELLUM_SOFFICE, then: make test-office\n\n'
else
	@printf '\nnote: LibreOffice found at %s — run the office-reader checks with: make test-office\n\n' '$(SOFFICE)'
endif

# test-office runs the checks that need a real office reader.
#
# Separate from `make test` because it needs an installation the module does not
# declare and cannot vendor, and because it costs seconds per case where the
# rest of the suite costs milliseconds. It is a gate on artifacts, not on code:
# see internal/oovalidate for what a pass does and does not establish.
test-office:
ifeq ($(strip $(SOFFICE)),)
	@printf '\n\033[31mERROR\033[0m  no LibreOffice found.\n'
	@printf '       Install it, or point VELLUM_SOFFICE at the soffice binary.\n\n'
	@exit 1
else
	@printf 'using LibreOffice at %s\n' '$(SOFFICE)'
	VELLUM_SOFFICE='$(SOFFICE)' VELLUM_REQUIRE_OPTIONAL_GATES=1 $(GO) test -tags soffice -v -run 'TestOfficeReader' ./internal/dettest/
endif

cover:
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...
	$(GO) vet -tags soffice ./...

lint: vet
	$(GO) run honnef.co/go/tools/cmd/staticcheck@latest ./...
	$(GO) run honnef.co/go/tools/cmd/staticcheck@latest -tags soffice ./...

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
