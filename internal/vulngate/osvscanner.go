// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package vulngate

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"
)

// osvScan is the part of osv-scanner's --format json result the gate reads:
// each scanned file, its packages, and the advisories against each, grouped by
// osv-scanner into one group per flaw.
type osvScan struct {
	Results []struct {
		Source struct {
			Path string `json:"path"`
		} `json:"source"`
		Packages []struct {
			Package struct {
				Name      string `json:"name"`
				Version   string `json:"version"`
				Ecosystem string `json:"ecosystem"`
			} `json:"package"`
			Vulnerabilities []osvRecord `json:"vulnerabilities"`
			Groups          []struct {
				IDs     []string `json:"ids"`
				Aliases []string `json:"aliases"`
			} `json:"groups"`
		} `json:"packages"`
	} `json:"results"`
}

// osvRecord is one OSV advisory, as far as the fixing versions go.
type osvRecord struct {
	ID       string `json:"id"`
	Affected []struct {
		Package struct {
			Ecosystem string `json:"ecosystem"`
			Name      string `json:"name"`
		} `json:"package"`
		Ranges []struct {
			Events []struct {
				Fixed string `json:"fixed"`
			} `json:"events"`
		} `json:"ranges"`
	} `json:"affected"`
}

// ReadOSVScanner reads osv-scanner's --format json result into one finding per
// advisory group and scanned file. root is the repository the scan ran in;
// file paths are reported relative to it. expected names, relative to root,
// every file the scan was asked to read. A result that scanned no package is
// refused: it proves nothing. So is one that leaves out an expected file,
// which osv-scanner does when a lockfile lists no package.
func ReadOSVScanner(r io.Reader, root string, expected []string) ([]Finding, error) {
	var res osvScan
	if err := json.NewDecoder(r).Decode(&res); err != nil {
		return nil, fmt.Errorf("osv-scanner output: %w", err)
	}
	if len(res.Results) == 0 {
		return nil, errors.New("osv-scanner output: no file was scanned")
	}
	var out []Finding
	scanned := map[string]bool{}
	for _, src := range res.Results {
		where := src.Source.Path
		if rel, err := filepath.Rel(root, where); err == nil && !strings.HasPrefix(rel, "..") {
			where = rel
		}
		scanned[where] = true
		if len(src.Packages) == 0 {
			return nil, fmt.Errorf("osv-scanner output: %s lists no packages", where)
		}
		for _, p := range src.Packages {
			for _, g := range p.Groups {
				if len(g.IDs) == 0 {
					continue
				}
				id := g.IDs[0]
				var also []string
				for _, a := range slices.Concat(g.IDs, g.Aliases) {
					if a != id && !slices.Contains(also, a) {
						also = append(also, a)
					}
				}
				slices.Sort(also)
				out = append(out, Finding{
					ID:      id,
					Aliases: also,
					Package: p.Package.Name,
					Version: p.Package.Version,
					Fixed:   strings.Join(fixedVersions(p.Vulnerabilities, g.IDs, p.Package.Ecosystem, p.Package.Name), ", "),
					Where:   where,
					Reach:   Listed,
				})
			}
		}
	}
	for _, want := range expected {
		if want = filepath.Clean(want); !scanned[want] {
			return nil, fmt.Errorf("osv-scanner output: %s was not scanned; a lockfile that lists no package is left out", want)
		}
	}
	sortFindings(out)
	return out, nil
}

// fixedVersions collects, across the group's records, the versions that fix
// the flaw for this package, in the order the records give them; other
// packages a record covers are left out.
func fixedVersions(records []osvRecord, ids []string, ecosystem, name string) []string {
	var fixed []string
	for _, rec := range records {
		if !slices.Contains(ids, rec.ID) {
			continue
		}
		for _, a := range rec.Affected {
			if a.Package.Name != name || !strings.EqualFold(a.Package.Ecosystem, ecosystem) {
				continue
			}
			for _, rg := range a.Ranges {
				for _, ev := range rg.Events {
					if ev.Fixed != "" && !slices.Contains(fixed, ev.Fixed) {
						fixed = append(fixed, ev.Fixed)
					}
				}
			}
		}
	}
	return fixed
}
