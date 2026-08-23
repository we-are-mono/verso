# SPDX-License-Identifier: GPL-2.0-only
# SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

# Verso shell — build/test tooling.
# Prototype build target is arm64 (project memo); override GOARCH to change.

BINARY   := verso
CMD      := ./cmd/verso
GOOS     ?= linux
GOARCH   ?= arm64
BUILDDIR := build

# Tailwind v4 standalone CLI (no Node); runs on the build host, pinned + cached.
TAILWIND         := $(BUILDDIR)/tools/tailwindcss
TAILWIND_VERSION := v4.3.3
CSS_IN           := internal/server/assets/input.css
CSS_OUT          := internal/server/assets/verso.css

# golangci-lint, pinned + cached on the build host (a dev tool, not a runtime dep).
GOLANGCI         := $(BUILDDIR)/tools/golangci-lint
GOLANGCI_VERSION := v1.64.8

.PHONY: all build run dev css test lint hooks tidy clean

all: lint test build

$(TAILWIND):
	@mkdir -p $(dir $@)
	curl -fsSL https://github.com/tailwindlabs/tailwindcss/releases/download/$(TAILWIND_VERSION)/tailwindcss-linux-x64 -o $@
	@chmod +x $@

# css compiles the embedded stylesheet. The generated CSS_OUT is committed so a
# bare `go build`/`go test` stays self-contained; regenerate it here on change.
css: $(TAILWIND)
	$(TAILWIND) -i $(CSS_IN) -o $(CSS_OUT) --minify

build: css
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -trimpath -o $(BUILDDIR)/$(BINARY) $(CMD)

run:
	go run $(CMD)

# dev: hot-reload loop — rebuild + swap the binary into the running container on
# every source change. Requires the container up (it will start one if needed).
dev:
	./scripts/dev.sh

test:
	go test ./...

# lint replaces plain `go vet` (govet is one of the linters it runs). Sensible
# defaults: no custom config, golangci-lint's default linter set.
$(GOLANGCI):
	@mkdir -p $(dir $@)
	curl -fsSL https://github.com/golangci/golangci-lint/releases/download/$(GOLANGCI_VERSION)/golangci-lint-$(GOLANGCI_VERSION:v%=%)-linux-amd64.tar.gz \
		| tar -xz -C $(dir $@) --strip-components=1 golangci-lint-$(GOLANGCI_VERSION:v%=%)-linux-amd64/golangci-lint
	@chmod +x $@

lint: $(GOLANGCI)
	$(GOLANGCI) run ./...

# hooks points git at the tracked pre-commit hook so commits are gated on lint.
hooks:
	git config core.hooksPath scripts/hooks

tidy:
	go mod tidy

clean:
	rm -rf $(BUILDDIR)
