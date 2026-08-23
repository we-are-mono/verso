# SPDX-License-Identifier: GPL-2.0-only
# SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

# Verso shell — build/test tooling.
# Prototype build target is arm64 (project memo); override GOARCH to change.

BINARY   := verso
CMD      := ./cmd/verso
GOOS     ?= linux
GOARCH   ?= arm64
BUILDDIR := build

.PHONY: all build run dev test vet tidy clean

all: vet test build

build:
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
