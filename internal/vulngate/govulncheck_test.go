// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package vulngate

import (
	"strings"
	"testing"
)

// govulncheckStream is govulncheck's -format json output in miniature: a
// stream of objects, one finding per trace, the advisories it cites, and
// messages the gate has no use for.
const govulncheckStream = `{"config": {"scanner_name": "govulncheck", "db": "https://vuln.go.dev", "go_version": "go1.27.1", "scan_level": "symbol", "scan_mode": "source"}}
{"SBOM": {"go_version": "go1.27.1", "modules": [{"path": "github.com/we-are-mono/verso"}]}}
{"progress": {"message": "Scanning your code..."}}
{"osv": {"id": "GO-2026-5320", "aliases": ["CVE-2026-5160", "GHSA-c97m-vxhj-p7j6"], "summary": "XSS"}}
{"osv": {"id": "GO-2026-6604", "aliases": ["CVE-2026-0001"]}}
{"osv": {"id": "GO-2026-5932"}}
{"finding": {"osv": "GO-2026-5320", "fixed_version": "v1.7.17", "trace": [{"module": "github.com/yuin/goldmark", "version": "v1.7.8"}]}}
{"finding": {"osv": "GO-2026-5320", "fixed_version": "v1.7.17", "trace": [{"module": "github.com/yuin/goldmark", "version": "v1.7.8", "package": "github.com/yuin/goldmark/renderer/html"}]}}
{"finding": {"osv": "GO-2026-5320", "fixed_version": "v1.7.17", "trace": [{"module": "github.com/yuin/goldmark", "version": "v1.7.8", "package": "github.com/yuin/goldmark/renderer/html", "function": "renderLink", "receiver": "*Renderer"}, {"module": "github.com/we-are-mono/verso", "package": "github.com/we-are-mono/verso/internal/widget", "function": "Transform"}]}}
{"finding": {"osv": "GO-2026-5320", "fixed_version": "v1.7.17", "trace": [{"module": "github.com/yuin/goldmark", "version": "v1.7.8", "package": "github.com/yuin/goldmark/renderer/html", "function": "renderImage", "receiver": "*Renderer"}]}}
{"finding": {"osv": "GO-2026-6604", "fixed_version": "v1.27.2", "trace": [{"module": "stdlib", "version": "v1.27.1"}]}}
{"finding": {"osv": "GO-2026-6604", "fixed_version": "v1.27.2", "trace": [{"module": "stdlib", "version": "v1.27.1", "package": "net/http"}]}}
{"finding": {"osv": "GO-2026-5932", "fixed_version": null, "trace": [{"module": "golang.org/x/crypto", "version": "v0.46.0"}]}}
`

// TestReadGovulncheckFoldsTracesIntoOneFindingPerAdvisory: every trace of an
// advisory is one finding at its deepest reach, carrying the advisory's
// aliases and the version that fixes it.
func TestReadGovulncheckFoldsTracesIntoOneFindingPerAdvisory(t *testing.T) {
	got, err := ReadGovulncheck(strings.NewReader(govulncheckStream))
	if err != nil {
		t.Fatalf("ReadGovulncheck: %v", err)
	}
	want := []Finding{
		{ID: "GO-2026-5320", Aliases: []string{"CVE-2026-5160", "GHSA-c97m-vxhj-p7j6"}, Package: "github.com/yuin/goldmark", Version: "v1.7.8", Fixed: "v1.7.17", Where: "go.mod", Reach: Called},
		{ID: "GO-2026-5932", Package: "golang.org/x/crypto", Version: "v0.46.0", Where: "go.mod", Reach: Required},
		{ID: "GO-2026-6604", Aliases: []string{"CVE-2026-0001"}, Package: "stdlib", Version: "v1.27.1", Fixed: "v1.27.2", Where: "Go toolchain", Reach: Imported},
	}
	assertFindings(t, got, want)
}

// TestReadGovulncheckRejectsATruncatedStream: a scan that died half-way must
// not read as a clean one.
func TestReadGovulncheckRejectsATruncatedStream(t *testing.T) {
	cut := govulncheckStream[:len(govulncheckStream)/2]
	if _, err := ReadGovulncheck(strings.NewReader(cut)); err == nil {
		t.Fatal("a truncated stream read without error")
	}
}

// TestReadGovulncheckRejectsAStreamWithoutConfig: an empty or foreign file is
// not a govulncheck result, however clean it looks.
func TestReadGovulncheckRejectsAStreamWithoutConfig(t *testing.T) {
	if _, err := ReadGovulncheck(strings.NewReader("")); err == nil {
		t.Fatal("an empty stream read as a clean scan")
	}
}

// TestReadGovulncheckRejectsAScanThatCannotProveACall: only a symbol-level
// scan of the Go vulnerability database can report a called advisory; a
// module- or package-level scan, or one against another database, reads
// clean by construction and is refused.
func TestReadGovulncheckRejectsAScanThatCannotProveACall(t *testing.T) {
	symbol := `"scan_level": "symbol"`
	official := `"db": "https://vuln.go.dev"`
	for name, swap := range map[string][2]string{
		"module level":   {symbol, `"scan_level": "module"`},
		"package level":  {symbol, `"scan_level": "package"`},
		"no scan level":  {symbol + ", ", ""},
		"local database": {official, `"db": "file:///tmp/vulndb"`},
		"plain http":     {official, `"db": "http://vuln.go.dev"`},
		"no database":    {official + ", ", ""},
	} {
		t.Run(name, func(t *testing.T) {
			stream := strings.Replace(govulncheckStream, swap[0], swap[1], 1)
			if stream == govulncheckStream {
				t.Fatalf("fixture has no %q to replace", swap[0])
			}
			if _, err := ReadGovulncheck(strings.NewReader(stream)); err == nil {
				t.Fatalf("a %s scan was accepted", name)
			}
		})
	}
}

// TestMergeFoldsOneAdvisoryAcrossScans: govulncheck runs once per release
// target, and a call can exist on one architecture alone, so the same
// advisory from several scans is one finding at the deepest reach any shows.
func TestMergeFoldsOneAdvisoryAcrossScans(t *testing.T) {
	arm64 := []Finding{
		{ID: "GO-2026-0001", Package: "stdlib", Version: "v1.27.1", Where: "Go toolchain", Reach: Imported},
		{ID: "GO-2026-0002", Package: "golang.org/x/sys", Version: "v0.1.0", Where: "go.mod", Reach: Called},
	}
	amd64 := []Finding{
		{ID: "GO-2026-0001", Package: "stdlib", Version: "v1.27.1", Where: "Go toolchain", Reach: Called},
		{ID: "GO-2026-0003", Package: "golang.org/x/net", Version: "v0.2.0", Where: "go.mod", Reach: Required},
	}
	want := []Finding{
		{ID: "GO-2026-0001", Package: "stdlib", Version: "v1.27.1", Where: "Go toolchain", Reach: Called},
		{ID: "GO-2026-0002", Package: "golang.org/x/sys", Version: "v0.1.0", Where: "go.mod", Reach: Called},
		{ID: "GO-2026-0003", Package: "golang.org/x/net", Version: "v0.2.0", Where: "go.mod", Reach: Required},
	}
	assertFindings(t, Merge(arm64, amd64), want)
}

func assertFindings(t *testing.T, got, want []Finding) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d findings, want %d:\n%+v", len(got), len(want), got)
	}
	for i := range want {
		if !equalFinding(got[i], want[i]) {
			t.Errorf("finding %d =\n  %+v\nwant\n  %+v", i, got[i], want[i])
		}
	}
}

func equalFinding(a, b Finding) bool {
	if a.ID != b.ID || a.Package != b.Package || a.Version != b.Version ||
		a.Fixed != b.Fixed || a.Where != b.Where || a.Reach != b.Reach || len(a.Aliases) != len(b.Aliases) {
		return false
	}
	for i := range a.Aliases {
		if a.Aliases[i] != b.Aliases[i] {
			return false
		}
	}
	return true
}
