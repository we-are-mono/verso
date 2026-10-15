// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package vulngate

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// goVulnDB is the Go vulnerability database. A scan against any other source,
// a local mirror included, could be stale or trimmed, so it proves nothing.
const goVulnDB = "https://vuln.go.dev"

// govulncheckMessage is one object of govulncheck's -format json stream. Each
// carries one of the fields; the gate reads the scan's config (proof the file
// is a symbol-level govulncheck result against the Go vulnerability database),
// the advisories, and the findings.
type govulncheckMessage struct {
	Config *struct {
		ScannerName string `json:"scanner_name"`
		DB          string `json:"db"`
		ScanLevel   string `json:"scan_level"`
	} `json:"config"`
	OSV *struct {
		ID      string   `json:"id"`
		Aliases []string `json:"aliases"`
	} `json:"osv"`
	Finding *struct {
		OSV          string  `json:"osv"`
		FixedVersion *string `json:"fixed_version"`
		Trace        []struct {
			Module   string `json:"module"`
			Version  string `json:"version"`
			Package  string `json:"package"`
			Function string `json:"function"`
		} `json:"trace"`
	} `json:"finding"`
}

// ReadGovulncheck reads a govulncheck -format json stream into one finding per
// advisory, at the deepest reach any of its traces shows. govulncheck emits a
// finding per trace and level; the first frame of each names the vulnerable
// module and, when reached, its package and function. Only a symbol-level scan
// follows calls, so a module- or package-level stream, which can never report
// a called advisory, is refused.
func ReadGovulncheck(r io.Reader) ([]Finding, error) {
	dec := json.NewDecoder(r)
	configErr := errors.New("govulncheck output: no scan config; not a govulncheck -format json result")
	aliases := map[string][]string{}
	byID := map[string]*Finding{}
	for {
		var m govulncheckMessage
		err := dec.Decode(&m)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("govulncheck output: %w", err)
		}
		switch {
		case m.Config != nil:
			configErr = checkConfig(m.Config.ScannerName, m.Config.DB, m.Config.ScanLevel)
		case m.OSV != nil:
			aliases[m.OSV.ID] = m.OSV.Aliases
		case m.Finding != nil && len(m.Finding.Trace) > 0:
			frame := m.Finding.Trace[0]
			reach := Required
			switch {
			case frame.Function != "":
				reach = Called
			case frame.Package != "":
				reach = Imported
			}
			f, ok := byID[m.Finding.OSV]
			if !ok {
				f = &Finding{ID: m.Finding.OSV, Package: frame.Module, Version: frame.Version, Where: goPinnedBy(frame.Module), Reach: reach}
				if m.Finding.FixedVersion != nil {
					f.Fixed = *m.Finding.FixedVersion
				}
				byID[m.Finding.OSV] = f
			}
			f.Reach = max(f.Reach, reach)
		}
	}
	if configErr != nil {
		return nil, configErr
	}
	out := make([]Finding, 0, len(byID))
	for id, f := range byID {
		f.Aliases = aliases[id]
		out = append(out, *f)
	}
	sortFindings(out)
	return out, nil
}

// checkConfig refuses a scan config the gate cannot rest on: another scanner,
// another database, or a scan level that does not follow calls.
func checkConfig(scanner, db, level string) error {
	switch {
	case scanner != "govulncheck":
		return fmt.Errorf("govulncheck output: scanner %q is not govulncheck", scanner)
	case db != goVulnDB:
		return fmt.Errorf("govulncheck output: scanned against %q, not %s", db, goVulnDB)
	case level != "symbol":
		return fmt.Errorf("govulncheck output: scan level %q does not follow calls; run with -scan symbol", level)
	}
	return nil
}

// goPinnedBy names what sets a Go module's version: go.mod for a dependency,
// the toolchain that builds the binary for the standard library.
func goPinnedBy(module string) string {
	if module == "stdlib" {
		return "Go toolchain"
	}
	return "go.mod"
}
