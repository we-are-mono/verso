// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// Package version carries the build's release string, stamped into the binary
// at link time. A build with no stamp — `go run`, `go test`, the `make dev`
// hot-reload loop — reports "dev", which is itself a useful signal: the login
// page shows "dev" for a working build and a real version only for a packaged
// (apk) one.
package version

// Version is the release string. It is overwritten at link time via
//
//	-ldflags "-X github.com/we-are-mono/verso/internal/version.Version=<ver>"
//
// (see the Makefile). The default is what an un-stamped build reports.
var Version = "dev"
