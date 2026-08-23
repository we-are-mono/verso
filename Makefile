# SPDX-License-Identifier: GPL-2.0-only
# SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

# Verso shell — build/test tooling.
# Prototype build target is arm64 (project memo); override GOARCH to change.

BINARY   := verso
CMD      := ./cmd/verso
GOOS     ?= linux
GOARCH   ?= arm64
BUILDDIR := build

.PHONY: all build run test vet tidy clean

all: vet test build

build:
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -trimpath -o $(BUILDDIR)/$(BINARY) $(CMD)

run:
	go run $(CMD)

test:
	go test ./...

vet:
	go vet ./...

tidy:
	go mod tidy

clean:
	rm -rf $(BUILDDIR)
