// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/we-are-mono/verso/internal/plugin"
	"github.com/we-are-mono/verso/internal/widget"
)

func TestOpenedSlotsAreNamedUnderTheirOpener(t *testing.T) {
	page, err := widget.Decode([]byte(`{"type":"stack","children":[
		{"type":"properties","items":[{"label":"Issued to","value":"a"}],"slot":"certificate.source"},
		{"type":"stack","inline":true,"children":[{"type":"slot","id":"certificate.acts","kind":"act"}]},
		{"type":"slot","id":"appearance"},
		{"type":"slot","id":"acme.acts","kind":"act"},
		{"type":"slot","id":"odd","kind":"fact"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, s := range openedSlots("system", page) {
		got = append(got, s.Name+":"+s.Kind)
	}
	want := []string{"system.certificate.source:fact", "system.certificate.acts:act", "system.acme.acts:act"}
	if !slices.Equal(got, want) {
		t.Errorf("opened = %v, want %v (appearance is the shell's block, an odd kind is none)", got, want)
	}
}

func TestAContributionsOwnSlotsAreNotCollected(t *testing.T) {
	page := &widget.Stack{Children: []widget.Widget{
		&widget.Embed{HTML: `<div>{"type":"slot","id":"x","kind":"act"}</div>`},
	}}
	if got := openedSlots("system", page); len(got) != 0 {
		t.Errorf("opened = %v, want none from an embedded contribution", got)
	}
}

func placed(pluginID, id, position string, order int) slotPlacement {
	pos, err := plugin.ParsePosition(position)
	if err != nil {
		panic(err)
	}
	return slotPlacement{Plugin: pluginID, Contribution: plugin.Contribution{ID: id, Path: "/" + id, Position: pos, Order: order}}
}

func orderNames(entries []slotEntry) []string {
	var out []string
	for _, e := range entries {
		if e.Placement != nil {
			out = append(out, e.Placement.Name())
		} else {
			out = append(out, e.Anchor)
		}
	}
	return out
}

func TestOrderSlot(t *testing.T) {
	own := []string{"password", "sessions", "web", "luci"}
	for name, tc := range map[string]struct {
		placed []slotPlacement
		want   []string
	}{
		"before a shell section": {
			[]slotPlacement{placed("a", "x", "before:web", 100)},
			[]string{"password", "sessions", "a.x", "web", "luci"},
		},
		"first and last": {
			[]slotPlacement{placed("a", "end", "", 100), placed("a", "top", "first", 100)},
			[]string{"a.top", "password", "sessions", "web", "luci", "a.end"},
		},
		"order, then plugin id, then contribution id": {
			[]slotPlacement{
				placed("b", "x", "after:sessions", 100),
				placed("a", "y", "after:sessions", 100),
				placed("a", "x", "after:sessions", 100),
				placed("c", "x", "after:sessions", 10),
			},
			[]string{"password", "sessions", "c.x", "a.x", "a.y", "b.x", "web", "luci"},
		},
		"after another contribution": {
			[]slotPlacement{placed("b", "more", "after:a.status", 100), placed("a", "status", "before:web", 100)},
			[]string{"password", "sessions", "a.status", "b.more", "web", "luci"},
		},
		"a missing anchor goes last": {
			[]slotPlacement{placed("b", "more", "after:a.status", 100)},
			[]string{"password", "sessions", "web", "luci", "b.more"},
		},
		"a cycle goes last": {
			[]slotPlacement{placed("a", "x", "after:b.y", 100), placed("b", "y", "after:a.x", 100)},
			[]string{"password", "sessions", "web", "luci", "a.x", "b.y"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if got := orderNames(orderSlot(own, tc.placed)); !slices.Equal(got, tc.want) {
				t.Errorf("order = %v, want %v", got, tc.want)
			}
		})
	}
}

func manifestWith(id string, contributes ...plugin.Contribution) plugin.Manifest {
	return plugin.Manifest{ID: id, Name: id, Socket: "/s", SchemaVersion: 1, Contributes: contributes}
}

func contribution(id, slot, position string) plugin.Contribution {
	pos, _ := plugin.ParsePosition(position)
	return plugin.Contribution{ID: id, Slot: slot, Path: "/" + id, Position: pos, Order: plugin.DefaultOrder}
}

func TestIndexSlotsSaysWhatIsMissingOnce(t *testing.T) {
	index, problems := indexSlots([]plugin.Manifest{
		manifestWith("system"),
		manifestWith("acme",
			contribution("use", "system.certificate.acts", ""),
			contribution("gone", "shell.access.footer", ""),
			contribution("orphan", "absent.x", ""),
			contribution("late", "shell.access.blocks", "after:zz.status"),
			contribution("ok", "shell.access.blocks", "after:sessions"),
		),
	})
	if got := len(index["system.certificate.acts"]); got != 1 {
		t.Errorf("system.certificate.acts holds %d, want 1", got)
	}
	var said []string
	for _, p := range problems {
		said = append(said, p.Error())
	}
	text := strings.Join(said, "\n")
	for _, want := range []string{`"acme" contribution "gone"`, "shell.access.footer", `"orphan"`, "absent", `"late"`, "zz.status"} {
		if !strings.Contains(text, want) {
			t.Errorf("problems %q do not name %s", text, want)
		}
	}
	if len(problems) != 3 {
		t.Errorf("problems = %d, want 3 (an unknown shell slot, a missing opener, a missing anchor)", len(problems))
	}
}

func TestIndexSlotsSaysACycleOnce(t *testing.T) {
	_, problems := indexSlots([]plugin.Manifest{
		manifestWith("a", contribution("x", "shell.access.blocks", "after:b.y")),
		manifestWith("b", contribution("y", "shell.access.blocks", "after:a.x"), contribution("z", "shell.access.blocks", "after:b.y")),
	})
	if len(problems) != 2 {
		t.Fatalf("problems = %v, want one for each of the two in the cycle", problems)
	}
	for _, p := range problems {
		if !strings.Contains(p.Error(), "leads back to itself") {
			t.Errorf("problem %q does not say it is a cycle", p)
		}
	}
}

// The shell's slots and the SDK's catalogue are one list: a slot one side
// names and the other lacks, or one whose kind or anchors differ, fails here.
func TestShellSlotsMatchTheSDKCatalogue(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "plugins", "verso-plugin-sdk", "testdata", "slots.json"))
	if err != nil {
		t.Skip("SDK slots fixture not present — run `cargo test` in plugins/verso-plugin-sdk to generate it")
	}
	var sdk []shellSlot
	if err := json.Unmarshal(data, &sdk); err != nil {
		t.Fatal(err)
	}
	if !slices.EqualFunc(sdk, shellSlots, func(a, b shellSlot) bool {
		return a.Name == b.Name && a.Kind == b.Kind && slices.Equal(a.Anchors, b.Anchors)
	}) {
		t.Errorf("SDK catalogue %+v\nshell slots    %+v", sdk, shellSlots)
	}
}
