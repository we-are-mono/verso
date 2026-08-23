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

.PHONY: all build run dev css test vet tidy clean

all: vet test build

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

vet:
	go vet ./...

tidy:
	go mod tidy

clean:
	rm -rf $(BUILDDIR)
