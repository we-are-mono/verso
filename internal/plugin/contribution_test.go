// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package plugin

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func contributor(id, contributes string) string {
	return `{"id":"` + id + `","name":"N","socket":"/run/verso/` + id + `.sock","schema_version":1,"contributes":` + contributes + `}`
}

func TestDiscoverReadsContributes(t *testing.T) {
	fsys := mapFS(map[string]string{
		"plugins/acme/manifest.json": contributor("acme", `[
			{"id":"source","slot":"system.certificate.source","path":"/slots/source"},
			{"id":"card","slot":"shell.overview.cards","path":"/slots/card","position":"after:system","order":20}
		]`),
	})
	got, problems := Discover(fsys, testGlob)
	if len(problems) != 0 || len(got) != 1 {
		t.Fatalf("got=%v problems=%v", got, problems)
	}
	c := got[0].Contributes
	if len(c) != 2 {
		t.Fatalf("contributes = %v", c)
	}
	if c[0].ID != "source" || c[0].Slot != "system.certificate.source" || c[0].Path != "/slots/source" {
		t.Errorf("first = %+v", c[0])
	}
	if c[0].Position != (Position{Where: Last}) || c[0].Order != DefaultOrder {
		t.Errorf("defaults = %+v", c[0])
	}
	if c[1].Position != (Position{Where: After, Anchor: "system"}) || c[1].Order != 20 {
		t.Errorf("second = %+v", c[1])
	}
	if c[1].Name("acme") != "acme.card" {
		t.Errorf("full id = %q", c[1].Name("acme"))
	}
}

func TestDiscoverLoadsAContributionOnlyPlugin(t *testing.T) {
	fsys := mapFS(map[string]string{
		"plugins/a/manifest.json": contributor("a", `[{"id":"x","slot":"shell.access.blocks","path":"/x"}]`),
	})
	got, problems := Discover(fsys, testGlob)
	if len(got) != 1 || len(problems) != 0 {
		t.Fatalf("got=%v problems=%v", got, problems)
	}
	if len(got[0].Nav) != 0 {
		t.Errorf("nav = %v, want none", got[0].Nav)
	}
}

func TestDiscoverSkipsABadContributionAndKeepsTheRest(t *testing.T) {
	for name, bad := range map[string]string{
		"no path":       `{"id":"b","slot":"shell.access.blocks"}`,
		"repeated id":   `{"id":"a","slot":"shell.access.blocks","path":"/b"}`,
		"uppercase id":  `{"id":"B","slot":"shell.access.blocks","path":"/b"}`,
		"dotted id":     `{"id":"b.c","slot":"shell.access.blocks","path":"/b"}`,
		"no slot":       `{"id":"b","path":"/b"}`,
		"unowned slot":  `{"id":"b","slot":"blocks","path":"/b"}`,
		"bad position":  `{"id":"b","slot":"shell.access.blocks","path":"/b","position":"middle"}`,
		"empty anchor":  `{"id":"b","slot":"shell.access.blocks","path":"/b","position":"after:"}`,
		"escaping path": `{"id":"b","slot":"shell.access.blocks","path":"/../b"}`,
		"query path":    `{"id":"b","slot":"shell.access.blocks","path":"/b?x=1"}`,
	} {
		t.Run(name, func(t *testing.T) {
			fsys := mapFS(map[string]string{
				"plugins/p/manifest.json": contributor("p", `[{"id":"a","slot":"shell.access.blocks","path":"/a"},`+bad+`]`),
			})
			got, problems := Discover(fsys, testGlob)
			if len(got) != 1 {
				t.Fatalf("manifest dropped: problems=%v", problems)
			}
			if ids := contributionIDs(got[0]); !slices.Equal(ids, []string{"a"}) {
				t.Errorf("contributions = %v, want [a]", ids)
			}
			if len(problems) != 1 || !strings.Contains(problems[0].Error(), "contributes[1]") {
				t.Errorf("problems = %v, want one naming contributes[1]", problems)
			}
		})
	}
}

func TestDiscoverRefusesTheShellID(t *testing.T) {
	fsys := mapFS(map[string]string{
		"plugins/shell/manifest.json": validManifest("shell"),
	})
	got, problems := Discover(fsys, testGlob)
	if len(got) != 0 || len(problems) != 1 || !strings.Contains(problems[0].Error(), "reserved") {
		t.Fatalf("got=%v problems=%v", got, problems)
	}
}

func TestDiscoverRefusesAPluginWhoseContributionsAllFail(t *testing.T) {
	fsys := mapFS(map[string]string{
		"plugins/p/manifest.json": contributor("p", `[{"id":"a","slot":"shell.access.blocks"}]`),
	})
	got, problems := Discover(fsys, testGlob)
	if len(got) != 0 || len(problems) != 2 {
		t.Fatalf("got=%v problems=%v, want the entry and the empty plugin", got, problems)
	}
}

func TestSystemAccessReadsAsAContribution(t *testing.T) {
	fsys := mapFS(map[string]string{
		"plugins/old/manifest.json": `{"id":"old","name":"N","socket":"/s","schema_version":1,"system_access":"/access"}`,
	})
	got, problems := Discover(fsys, testGlob)
	if len(got) != 1 || len(problems) != 0 {
		t.Fatalf("got=%v problems=%v", got, problems)
	}
	want := Contribution{ID: "access", Slot: AccessBlocks, Path: "/access", Position: Position{Where: Before, Anchor: "web"}, Order: DefaultOrder}
	if len(got[0].Contributes) != 1 || got[0].Contributes[0] != want {
		t.Errorf("contributes = %+v, want %+v", got[0].Contributes, want)
	}
}

func TestSystemAccessGivesWayToAnAccessContribution(t *testing.T) {
	fsys := mapFS(map[string]string{
		"plugins/new/manifest.json": `{"id":"new","name":"N","socket":"/s","schema_version":1,"system_access":"/access",
			"contributes":[{"id":"part","slot":"shell.access.blocks","path":"/part","position":"first"}]}`,
	})
	got, _ := Discover(fsys, testGlob)
	if len(got) != 1 {
		t.Fatal("manifest dropped")
	}
	if ids := contributionIDs(got[0]); !slices.Equal(ids, []string{"part"}) {
		t.Errorf("contributions = %v, want [part] alone", ids)
	}
}

func TestParsePosition(t *testing.T) {
	for in, want := range map[string]Position{
		"":                  {Where: Last},
		"last":              {Where: Last},
		"first":             {Where: First},
		"before:web":        {Where: Before, Anchor: "web"},
		"after:acme.status": {Where: After, Anchor: "acme.status"},
	} {
		got, err := ParsePosition(in)
		if err != nil || got != want {
			t.Errorf("ParsePosition(%q) = %+v, %v; want %+v", in, got, err, want)
		}
	}
	for _, in := range []string{"middle", "before:", "after", "after:Web", "after:a.b.c", "first:x"} {
		if _, err := ParsePosition(in); err == nil {
			t.Errorf("ParsePosition(%q) accepted", in)
		}
	}
}

func TestSlotOwner(t *testing.T) {
	for name, owner := range map[string]string{
		"shell.access.blocks":       "shell",
		"system.certificate.source": "system",
		"verso-acme.x":              "verso-acme",
	} {
		if got, ok := SlotOwner(name); !ok || got != owner {
			t.Errorf("SlotOwner(%q) = %q, %v; want %q", name, got, ok, owner)
		}
	}
	for _, name := range []string{"", "blocks", ".x", "a.", "a..b", "a.B"} {
		if _, ok := SlotOwner(name); ok {
			t.Errorf("SlotOwner(%q) accepted", name)
		}
	}
}

// The author's guide opens on a manifest; it must be one the shell loads,
// contributions and all.
func TestTheGuidesManifestLoads(t *testing.T) {
	guide, err := os.ReadFile(filepath.Join("..", "..", "docs", "plugins.md"))
	if err != nil {
		t.Fatal(err)
	}
	_, after, _ := strings.Cut(string(guide), "## The manifest")
	_, block, _ := strings.Cut(after, "```json\n")
	example, _, ok := strings.Cut(block, "```")
	if !ok {
		t.Fatal("the guide's manifest example is not found")
	}
	got, problems := Discover(mapFS(map[string]string{"plugins/ntp/manifest.json": example}), testGlob)
	if len(got) != 1 || len(problems) != 0 || len(got[0].Contributes) != 1 {
		t.Fatalf("got=%+v problems=%v", got, problems)
	}
}

func contributionIDs(m Manifest) []string {
	var ids []string
	for _, c := range m.Contributes {
		ids = append(ids, c.ID)
	}
	return ids
}
