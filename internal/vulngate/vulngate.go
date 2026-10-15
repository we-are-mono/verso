// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// Package vulngate decides whether a release may ship given what the
// vulnerability scanners found. It reads govulncheck's JSON stream for the Go
// module and osv-scanner's JSON result for the Rust lockfiles and the vendored
// scripts, folds both into one list of findings, and sets them against the
// repository's list of triaged advisories. A finding the code reaches, or one
// a lockfile pins, fails the gate unless an entry that has not lapsed names it.
package vulngate

import (
	"fmt"
	"io"
	"slices"
	"strings"
	"text/tabwriter"
	"time"
)

// Reach is how close an advisory comes to the shipped code.
type Reach int

const (
	// Required: the vulnerable module is in the build list, but no package of
	// it with the flaw is imported.
	Required Reach = iota
	// Imported: a vulnerable package is imported, but govulncheck finds no call
	// path to the vulnerable symbols.
	Imported
	// Called: govulncheck finds a call path from Verso's code to a vulnerable
	// symbol.
	Called
	// Listed: a lockfile or the vendored-script manifest pins an affected
	// version. Nothing proves the flaw unreachable, so it counts as reached.
	Listed
)

func (r Reach) String() string {
	switch r {
	case Required:
		return "required, not imported"
	case Imported:
		return "imported, not called"
	case Called:
		return "called"
	case Listed:
		return "pinned"
	}
	return "unknown"
}

// Finding is one advisory against one package version.
type Finding struct {
	ID      string
	Aliases []string // the advisory's other identifiers: CVE, GHSA, RUSTSEC
	Package string
	Version string
	Fixed   string // the versions that fix it, comma separated; "" when none is published
	Where   string // the file that pins the version, relative to the repository
	Reach   Reach
}

// blocking reports whether the finding stops a release unless triaged.
func (f Finding) blocking() bool { return f.Reach == Called || f.Reach == Listed }

// names reports whether id is any of the finding's identifiers.
func (f Finding) names(id string) bool { return f.ID == id || slices.Contains(f.Aliases, id) }

// Merge folds the findings of several scans into one list: an advisory against
// the same package version, pinned in the same place, is one finding at the
// deepest reach any scan shows. govulncheck runs once per release target, and
// a call path can exist on one architecture alone.
func Merge(scans ...[]Finding) []Finding {
	type key struct{ id, pkg, version, where string }
	byKey := map[key]int{}
	var out []Finding
	for _, scan := range scans {
		for _, f := range scan {
			k := key{f.ID, f.Package, f.Version, f.Where}
			if i, seen := byKey[k]; seen {
				out[i].Reach = max(out[i].Reach, f.Reach)
				continue
			}
			byKey[k] = len(out)
			out = append(out, f)
		}
	}
	sortFindings(out)
	return out
}

// Verdict is the gate's decision and everything it rests on.
type Verdict struct {
	Untriaged []Finding // blocking findings no active entry names
	Triaged   []Finding // findings an active entry names
	Unreached []Finding // Go findings the code does not call, shown for the record
	Lapsed    []Ignore  // entries past their date; they no longer ignore anything
	Unused    []Ignore  // active entries that name no finding
}

// Failed reports whether the release must stop.
func (v Verdict) Failed() bool { return len(v.Untriaged) > 0 }

// Triage sets the findings against the ignore list as of now.
func Triage(findings []Finding, ignores []Ignore, now time.Time) Verdict {
	var v Verdict
	used := make([]bool, len(ignores))
	for _, f := range findings {
		triaged := false
		for i, ig := range ignores {
			if ig.Active(now) && f.names(ig.ID) {
				used[i], triaged = true, true
			}
		}
		switch {
		case triaged:
			v.Triaged = append(v.Triaged, f)
		case f.blocking():
			v.Untriaged = append(v.Untriaged, f)
		default:
			v.Unreached = append(v.Unreached, f)
		}
	}
	for i, ig := range ignores {
		switch {
		case !ig.Active(now):
			v.Lapsed = append(v.Lapsed, ig)
		case !used[i]:
			v.Unused = append(v.Unused, ig)
		}
	}
	return v
}

// Write renders the verdict for a person at a terminal or in a CI log.
func (v Verdict) Write(w io.Writer) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	section := func(title string, findings []Finding) {
		if len(findings) == 0 {
			return
		}
		fmt.Fprintf(tw, "\n%s (%d)\n", title, len(findings))
		fmt.Fprintln(tw, "ADVISORY\tALIASES\tPACKAGE\tVERSION\tFIXED IN\tPINNED BY\tREACH")
		for _, f := range findings {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", f.ID, orDash(strings.Join(f.Aliases, " ")),
				f.Package, f.Version, orNone(f.Fixed), f.Where, f.Reach)
		}
	}
	entries := func(title string, ignores []Ignore) {
		if len(ignores) == 0 {
			return
		}
		fmt.Fprintf(tw, "\n%s (%d)\n", title, len(ignores))
		for _, ig := range ignores {
			fmt.Fprintf(tw, "%s\tuntil %s\tline %d\t%s\n", ig.ID, ig.Until.Format(dateLayout), ig.Line, ig.Reason)
		}
	}
	section("Untriaged: these stop a release", v.Untriaged)
	section("Triaged in the ignore list", v.Triaged)
	section("Not reached by Verso's code", v.Unreached)
	entries("Lapsed ignore entries: re-triage them", v.Lapsed)
	entries("Ignore entries that match no finding: remove them", v.Unused)
	if v.Failed() {
		fmt.Fprintf(tw, "\nvuln: %d untriaged advisories\n", len(v.Untriaged))
	} else {
		fmt.Fprintln(tw, "\nvuln: no untriaged advisories")
	}
	return tw.Flush()
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}

// sortFindings orders findings by advisory, then by where they are pinned, so
// a report reads the same from run to run.
func sortFindings(fs []Finding) {
	slices.SortFunc(fs, func(a, b Finding) int {
		if c := strings.Compare(a.ID, b.ID); c != 0 {
			return c
		}
		if c := strings.Compare(a.Where, b.Where); c != 0 {
			return c
		}
		return strings.Compare(a.Package, b.Package)
	})
}
