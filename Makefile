# SPDX-License-Identifier: GPL-2.0-only
# SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

# Verso shell — build/test tooling.
# Prototype build target is arm64 (project memo); override GOARCH to change.

BINARY   := verso
CMD      := ./cmd/verso
GOOS     ?= linux
BUILDDIR := build
CARGO    ?= $(if $(wildcard $(HOME)/.cargo/bin/cargo),$(HOME)/.cargo/bin/cargo,cargo)
RPCD_MANIFEST := verso-rpcd/Cargo.toml

# `make build` cross-compiles every architecture in ARCHES; each maps to a Go
# GOARCH and the matching Rust musl target triple below. Override to build one:
#   make build ARCHES=arm64      (or just `make build-arm64`)
ARCHES ?= amd64 arm64
rust_target_amd64 := x86_64-unknown-linux-musl
rust_target_arm64 := aarch64-unknown-linux-musl

# Strip the symbol table (-s) and DWARF debug info (-w): a shipped runtime binary
# needs neither, and dropping them cuts ~25-30% off its size.
LDFLAGS  := -s -w

# Tailwind v4 standalone CLI (no Node); runs on the build host, pinned + cached.
TAILWIND         := $(BUILDDIR)/tools/tailwindcss
TAILWIND_VERSION := v4.3.3
CSS_IN           := internal/server/assets/input.css
CSS_OUT          := internal/server/assets/verso.css

# golangci-lint, pinned + cached on the build host (a dev tool, not a runtime dep).
GOLANGCI         := $(BUILDDIR)/tools/golangci-lint
GOLANGCI_VERSION := v1.64.8

# build-<arch> is intentionally NOT phony: make skips pattern rules for phony
# targets, and no file of that name is ever produced, so the rule fires each run.
.PHONY: all build run dev css test lint hooks tidy rpcd clean

all: lint test build

$(TAILWIND):
	@mkdir -p $(dir $@)
	curl -fsSL https://github.com/tailwindlabs/tailwindcss/releases/download/$(TAILWIND_VERSION)/tailwindcss-linux-x64 -o $@
	@chmod +x $@

# css compiles the embedded stylesheet. The generated CSS_OUT is committed so a
# bare `go build`/`go test` stays self-contained; regenerate it here on change.
css: $(TAILWIND)
	$(TAILWIND) -i $(CSS_IN) -o $(CSS_OUT) --minify

# build cross-compiles verso (Go) and verso-rpcd (Rust) for every arch in ARCHES.
# Outputs are arch-suffixed: build/verso-<arch>, build/verso-rpcd-<arch>. The Rust
# toolchain + musl targets are provisioned by verso-rpcd/rust-toolchain.toml, and
# rust-lld (bundled with rustc) is the cross-linker — no host cross-gcc needed
# (.cargo/config.toml). A fresh checkout builds with only Go, rustup, and make.
build: css $(addprefix build-,$(ARCHES))

# build-<arch>: one architecture's pair of binaries. Runnable on its own, e.g.
# `make build-arm64` for just the device target.
build-%: css
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$* go build -trimpath -ldflags "$(LDFLAGS)" -o $(BUILDDIR)/$(BINARY)-$* $(CMD)
	$(CARGO) build --locked --release --manifest-path $(RPCD_MANIFEST) --target $(rust_target_$*)
	cp verso-rpcd/target/$(rust_target_$*)/release/verso-rpcd $(BUILDDIR)/verso-rpcd-$*

run:
	go run $(CMD)

# dev: hot-reload loop — rebuild + swap the binary into the running container on
# every source change. Requires the container up (it will start one if needed).
dev:
	./scripts/dev.sh

test:
	go test ./...
	$(CARGO) test --locked --manifest-path $(RPCD_MANIFEST)

# lint replaces plain `go vet` (govet is one of the linters it runs). Sensible
# defaults: no custom config, golangci-lint's default linter set.
$(GOLANGCI):
	@mkdir -p $(dir $@)
	curl -fsSL https://github.com/golangci/golangci-lint/releases/download/$(GOLANGCI_VERSION)/golangci-lint-$(GOLANGCI_VERSION:v%=%)-linux-amd64.tar.gz \
		| tar -xz -C $(dir $@) --strip-components=1 golangci-lint-$(GOLANGCI_VERSION:v%=%)-linux-amd64/golangci-lint
	@chmod +x $@

lint: $(GOLANGCI)
	$(GOLANGCI) run ./...
	$(CARGO) clippy --locked --manifest-path $(RPCD_MANIFEST) --all-targets -- -D warnings

# hooks points git at the tracked pre-commit hook so commits are gated on lint.
hooks:
	git config core.hooksPath scripts/hooks

tidy:
	go mod tidy

# Native helper build for the Docker/dev loop (host arch, no cross-target).
rpcd:
	$(CARGO) build --locked --release --manifest-path $(RPCD_MANIFEST)

clean:
	rm -rf $(BUILDDIR)
