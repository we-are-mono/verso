// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package vulngate

import (
	"strings"
	"testing"
	"time"
)

var today = time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)

// TestTriageFailsOnACalledOrListedFinding: a reachable Go advisory and an
// advisory against a pinned crate or script both stop the gate.
func TestTriageFailsOnACalledOrListedFinding(t *testing.T) {
	called := Finding{ID: "GO-2026-0001", Reach: Called}
	listed := Finding{ID: "RUSTSEC-2026-0002", Reach: Listed}
	v := Triage([]Finding{called, listed}, nil, today)
	if !v.Failed() {
		t.Fatal("gate passed with untriaged findings")
	}
	assertIDs(t, "untriaged", v.Untriaged, "GO-2026-0001", "RUSTSEC-2026-0002")
}

// TestTriagePassesUnreachedGoFindings: govulncheck proved the vulnerable code
// is not called, so the finding is shown but does not stop the gate.
func TestTriagePassesUnreachedGoFindings(t *testing.T) {
	v := Triage([]Finding{
		{ID: "GO-2026-0003", Reach: Imported},
		{ID: "GO-2026-0004", Reach: Required},
	}, nil, today)
	if v.Failed() {
		t.Fatalf("gate failed on unreached findings: %+v", v.Untriaged)
	}
	assertIDs(t, "unreached", v.Unreached, "GO-2026-0003", "GO-2026-0004")
}

// TestTriageIgnoresByIDOrAlias: an entry names the advisory by any of its
// identifiers, so a CVE triaged once covers the Go, RustSec and GHSA records.
func TestTriageIgnoresByIDOrAlias(t *testing.T) {
	findings := []Finding{
		{ID: "GO-2026-0001", Reach: Called},
		{ID: "RUSTSEC-2026-0002", Aliases: []string{"CVE-2026-9999"}, Reach: Listed},
	}
	ignores := []Ignore{
		{ID: "GO-2026-0001", Until: day(2026, 12, 31), Reason: "r"},
		{ID: "CVE-2026-9999", Until: day(2026, 12, 31), Reason: "r"},
	}
	v := Triage(findings, ignores, today)
	if v.Failed() {
		t.Fatalf("gate failed on triaged findings: %+v", v.Untriaged)
	}
	assertIDs(t, "triaged", v.Triaged, "GO-2026-0001", "RUSTSEC-2026-0002")
	if len(v.Unused) != 0 {
		t.Errorf("entries that matched reported unused: %+v", v.Unused)
	}
}

// TestTriageLapsedEntryNoLongerIgnores: past its date an entry stops matching,
// so the finding fails the gate again and the entry is named as lapsed.
func TestTriageLapsedEntryNoLongerIgnores(t *testing.T) {
	v := Triage(
		[]Finding{{ID: "GO-2026-0001", Reach: Called}},
		[]Ignore{{ID: "GO-2026-0001", Until: day(2026, 10, 14), Reason: "r"}},
		today,
	)
	if !v.Failed() {
		t.Fatal("a lapsed entry still ignored its finding")
	}
	if len(v.Lapsed) != 1 || v.Lapsed[0].ID != "GO-2026-0001" {
		t.Errorf("lapsed = %+v, want the one entry", v.Lapsed)
	}
}

// TestTriageReportsUnusedEntries: an entry that matches nothing is named, so
// the list sheds advisories a bump has fixed.
func TestTriageReportsUnusedEntries(t *testing.T) {
	v := Triage(nil, []Ignore{{ID: "GO-2026-0001", Until: day(2026, 12, 31), Reason: "r"}}, today)
	if v.Failed() {
		t.Fatal("an unused entry failed the gate")
	}
	if len(v.Unused) != 1 {
		t.Errorf("unused = %+v, want the one entry", v.Unused)
	}
}

// TestVerdictWriteNamesEveryFinding: the report carries what a reader needs
// to act: the advisory, the package and version, the fix, and where it is
// pinned.
func TestVerdictWriteNamesEveryFinding(t *testing.T) {
	v := Triage([]Finding{
		{ID: "GO-2026-5320", Aliases: []string{"CVE-2026-5160"}, Package: "github.com/yuin/goldmark", Version: "v1.7.8", Fixed: "v1.7.17", Where: "go.mod", Reach: Called},
		{ID: "GO-2026-5932", Package: "golang.org/x/crypto", Version: "v0.46.0", Where: "go.mod", Reach: Required},
	}, []Ignore{{ID: "GO-2026-0042", Until: day(2026, 10, 1), Reason: "r"}}, today)
	var b strings.Builder
	if err := v.Write(&b); err != nil {
		t.Fatalf("Write: %v", err)
	}
	out := b.String()
	for _, want := range []string{
		"GO-2026-5320", "CVE-2026-5160", "github.com/yuin/goldmark", "v1.7.8", "v1.7.17", "go.mod", "called",
		"GO-2026-5932", "not imported", "none",
		"GO-2026-0042", "2026-10-01",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report lacks %q:\n%s", want, out)
		}
	}
}

func assertIDs(t *testing.T, what string, got []Finding, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %+v, want %v", what, got, want)
	}
	for i := range want {
		if got[i].ID != want[i] {
			t.Errorf("%s[%d] = %s, want %s", what, i, got[i].ID, want[i])
		}
	}
}
