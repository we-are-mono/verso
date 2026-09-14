// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// Package version carries the build's release string, stamped into the binary
// at link time. A build with no stamp — `go run`, `go test` — reports "dev";
// the `make dev` hot-reload loop stamps "<ver>-dev", so a working build states
// both its lineage and its nature, and a bare version means a packaged (apk)
// build.
package version

// Version is the release string. It is overwritten at link time via
//
//	-ldflags "-X github.com/we-are-mono/verso/internal/version.Version=<ver>"
//
// (see the Makefile). The default is what an un-stamped build reports.
var Version = "dev"
