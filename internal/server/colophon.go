// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"os"
	"strings"
	"sync"
)

// Version is the build's release string, stamped into the binary at link time:
//
//	-ldflags "-X github.com/we-are-mono/verso/internal/server.Version=<ver>"
//
// (see the Makefile). A build with no stamp — `go run`, `go test` — reports
// "dev"; the `make dev` hot-reload loop stamps "<ver>-dev", so a working build
// states both its lineage and its nature, and a bare version means a packaged
// (apk) build.
var Version = "dev"

// colophon is the last line of every page: what runs this router, the release
// with its revision and target, and the Verso build drawing it. Report is the
// same facts, and the board and kernel besides, set down the way a bug report
// asks for them, which is what the line's copy puts on the clipboard.
type colophon struct {
	Release  string // "OpenWrt 25.12.4"
	Revision string // "r32933-4ccb782af7"
	Target   string // "layerscape/armv8_64b"
	Verso    string // the running build, as Version states it
	Report   string
}

// pageColophon reads the colophon's facts once per process: the release, the
// kernel and the board change only with a sysupgrade, which restarts Verso on
// the image it installs.
var pageColophon = sync.OnceValue(func() colophon {
	release, _ := os.ReadFile("/etc/openwrt_release")
	kernel, _ := os.ReadFile("/proc/sys/kernel/osrelease")
	return newColophon(release, kernel, board(), Version)
})

// newColophon composes the colophon from /etc/openwrt_release, the kernel's
// release string and the board. A fact the router does not state is left out
// of both the line and the report; a release file that names nothing falls
// back to a plain "OpenWrt", as the sign-in page does.
func newColophon(release, kernel []byte, b boardFacts, verso string) colophon {
	keys := releaseKeys(release)
	c := colophon{
		Release:  releaseName(release),
		Revision: keys["DISTRIB_REVISION"],
		Target:   keys["DISTRIB_TARGET"],
		Verso:    verso,
	}
	if c.Release == "" {
		c.Release = "OpenWrt"
	}
	// A description that stood in for the release already carries the revision.
	if strings.Contains(c.Release, c.Revision) {
		c.Revision = ""
	}
	c.Report = strings.Join(nonEmpty(
		strings.TrimSpace(c.Release+" "+c.Revision),
		labelled("Target", c.Target),
		labelled("Board", boardLine(b)),
		labelled("Kernel", strings.TrimSpace(string(kernel))),
		labelled("Verso", verso),
	), "\n")
	return c
}

// boardLine names the board by the id OpenWrt selects its image with, and the
// name its maker gives it beside it.
func boardLine(b boardFacts) string {
	name := strings.TrimSpace(b.Maker + " " + b.Model)
	switch {
	case b.Name == "":
		return name
	case name == "":
		return b.Name
	}
	return b.Name + " (" + name + ")"
}

// labelled is one report line, "Label: value", or nothing when the value is.
func labelled(label, value string) string {
	if value == "" {
		return ""
	}
	return label + ": " + value
}

func nonEmpty(lines ...string) []string {
	out := lines[:0]
	for _, line := range lines {
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}
