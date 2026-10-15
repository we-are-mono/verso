// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package vulngate

import (
	"strings"
	"testing"
)

// osvScannerResult is osv-scanner's --format json output in miniature: a
// lockfile with one package under two grouped advisories, a manifest with a
// clean package, and the fields the gate does not read.
const osvScannerResult = `{
  "results": [
    {
      "source": {"path": "/src/verso/verso-rpcd/Cargo.lock", "type": "lockfile"},
      "packages": [
        {
          "package": {"name": "smallvec", "version": "1.6.0", "ecosystem": "crates.io"},
          "vulnerabilities": [
            {"id": "RUSTSEC-2021-0003", "aliases": ["CVE-2021-25900", "GHSA-43w2-9j62-hq99"],
             "affected": [
               {"package": {"ecosystem": "crates.io", "name": "smallvec"},
                "ranges": [{"type": "SEMVER", "events": [{"introduced": "0.6.3"}, {"fixed": "0.6.14"}, {"introduced": "1.0.0"}, {"fixed": "1.6.1"}]}]},
               {"package": {"ecosystem": "crates.io", "name": "other"},
                "ranges": [{"type": "SEMVER", "events": [{"introduced": "0"}, {"fixed": "9.9.9"}]}]}
             ]},
            {"id": "GHSA-43w2-9j62-hq99", "aliases": ["CVE-2021-25900"],
             "affected": [
               {"package": {"ecosystem": "crates.io", "name": "smallvec"},
                "ranges": [{"type": "SEMVER", "events": [{"introduced": "1.0.0"}, {"fixed": "1.6.1"}]}]}
             ]}
          ],
          "groups": [{"ids": ["RUSTSEC-2021-0003", "GHSA-43w2-9j62-hq99"], "aliases": ["CVE-2021-25900", "GHSA-43w2-9j62-hq99", "RUSTSEC-2021-0003"], "max_severity": "9.8"}]
        }
      ]
    },
    {
      "source": {"path": "/src/verso/internal/server/assets/vendored-js.json", "type": "lockfile"},
      "packages": [
        {"package": {"name": "htmx.org", "version": "2.0.4", "ecosystem": "npm"}, "vulnerabilities": [], "groups": []}
      ]
    }
  ],
  "experimental_config": {"licenses": {"summary": false}}
}`

// TestReadOSVScannerFindsOneFindingPerAdvisoryGroup: osv-scanner groups the
// records of one flaw; the gate reports the group once, at the lockfile that
// pins the package, with the versions that fix it for that package.
func TestReadOSVScannerFindsOneFindingPerAdvisoryGroup(t *testing.T) {
	got, err := ReadOSVScanner(strings.NewReader(osvScannerResult), "/src/verso", scannedFiles)
	if err != nil {
		t.Fatalf("ReadOSVScanner: %v", err)
	}
	want := []Finding{{
		ID:      "RUSTSEC-2021-0003",
		Aliases: []string{"CVE-2021-25900", "GHSA-43w2-9j62-hq99"},
		Package: "smallvec", Version: "1.6.0", Fixed: "0.6.14, 1.6.1",
		Where: "verso-rpcd/Cargo.lock", Reach: Listed,
	}}
	assertFindings(t, got, want)
}

// TestReadOSVScannerRejectsAnEmptyResult: a scan that found no packages at all
// scanned nothing, which is not the same as a clean scan.
func TestReadOSVScannerRejectsAnEmptyResult(t *testing.T) {
	for name, body := range map[string]string{
		"empty":       "",
		"no results":  `{"results": []}`,
		"not json":    "Scanned 5 files",
		"no packages": `{"results": [{"source": {"path": "/x/Cargo.lock"}, "packages": []}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ReadOSVScanner(strings.NewReader(body), "/x", nil); err == nil {
				t.Fatalf("%s read as a clean scan", name)
			}
		})
	}
}

// TestReadOSVScannerRejectsAResultMissingAnExpectedFile: osv-scanner leaves a
// lockfile that lists no package out of its result, so a file the scan was
// asked to read and does not report was not checked, and the scan is refused
// naming it.
func TestReadOSVScannerRejectsAResultMissingAnExpectedFile(t *testing.T) {
	expected := append([]string{"plugins/verso-plugin-system/Cargo.lock"}, scannedFiles...)
	_, err := ReadOSVScanner(strings.NewReader(osvScannerResult), "/src/verso", expected)
	if err == nil {
		t.Fatal("a result without an expected lockfile read as a clean scan")
	}
	if !strings.Contains(err.Error(), "plugins/verso-plugin-system/Cargo.lock") {
		t.Errorf("error %q does not name the missing file", err)
	}
}

// TestReadOSVScannerMatchesExpectedFilesByCleanPath: the expected list is
// relative to the repository, however it is spelled.
func TestReadOSVScannerMatchesExpectedFilesByCleanPath(t *testing.T) {
	expected := []string{"./verso-rpcd/Cargo.lock", "internal/server/assets//vendored-js.json"}
	if _, err := ReadOSVScanner(strings.NewReader(osvScannerResult), "/src/verso", expected); err != nil {
		t.Fatalf("ReadOSVScanner: %v", err)
	}
}

// scannedFiles is what the fixture's scan was asked to read.
var scannedFiles = []string{"verso-rpcd/Cargo.lock", "internal/server/assets/vendored-js.json"}
