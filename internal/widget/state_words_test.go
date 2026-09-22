// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// TestPortsSayTheirState: a port's link state is drawn as LED colour, so each
// port also carries it in words. Both words are present and CSS keeps the one
// that matches the port's live class, so a stream that relinks the port changes
// what is read without touching the markup.
func TestPortsSayTheirState(t *testing.T) {
	got := render(t, newRenderer(t), &Ports{Items: []PortItem{
		{Kind: "rj45", Label: "Network 1", Linked: true, Iface: "eth0"},
	}})
	for _, want := range []string{
		`role="group" aria-label="Network 1"`,
		`<span class="verso-port-state-on sr-only">Connected</span>`,
		`<span class="verso-port-state-off sr-only">Not connected</span>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("port missing %q:\n%s", want, got)
		}
	}
}

// TestPanelFacesSwitchWithoutExpressions: the Front/Rear control runs under the
// CSP build of Alpine, which evaluates no expressions in bindings, so it binds
// to a component's properties. The face shown is filled in the body ink, as
// every selected segment is, never the action colour.
func TestPanelFacesSwitchWithoutExpressions(t *testing.T) {
	got := render(t, newRenderer(t), &Ports{Back: `<svg></svg>`, Front: `<svg></svg>`})
	for _, want := range []string{
		`x-data="panelFaces"`, `@click="showRear"`, `@click="showFront"`,
		`:class="rearClass"`, `:aria-pressed="rearPressed"`, `x-show="rear"`, `x-show="front"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("panel faces missing %q:\n%s", want, got)
		}
	}
	for _, gone := range []string{"face ===", "bg-denim text-white"} {
		if strings.Contains(got, gone) {
			t.Errorf("panel faces must not carry %q:\n%s", gone, got)
		}
	}
}

// TestFetchedPanelsSayTheyAreLoading: a drawer whose contents are one request
// away shows only the waiting mark, which is decoration; the frame also says
// "Loading…" in words until what it fetched replaces it.
func TestFetchedPanelsSayTheyAreLoading(t *testing.T) {
	got := render(t, newRenderer(t), &Table{
		Columns: []TableColumn{{Label: "Name", Kind: "name"}},
		Rows: []TableRow{
			{ID: "a", Panel: "/plugins/x/a", Cells: []TableCell{{Text: "a"}}},
			{ID: "b", Entity: &EntityRef{Kind: "device", ID: "b"}, Cells: []TableCell{{Text: "b"}}},
		},
	})
	if n := strings.Count(got, `<span class="sr-only">Loading…</span>`); n != 2 {
		t.Errorf("both fetched frames must say they are loading, found %d:\n%s", n, got)
	}
}

// TestCountsAlignOnTheirDigits: a number column that abbreviates some values
// (2.7k) keeps its digits in one column: the magnitude letter sits in a slot
// of its own at the cell's end, and a value with no letter keeps the slot,
// empty, so its last digit lines up with the others'. A column that never
// abbreviates takes no slot at all.
func TestCountsAlignOnTheirDigits(t *testing.T) {
	const slot = `<span class="inline-block w-[1ch] text-left">`
	got := render(t, newRenderer(t), &Table{
		Columns: []TableColumn{{Label: "Name", Kind: "name"}, {Label: "Hits", Kind: "num"}},
		Rows: []TableRow{
			{Cells: []TableCell{{Text: "a"}, {Text: "321"}}},
			{Cells: []TableCell{{Text: "b"}, {Text: "2.7k"}}},
			{Cells: []TableCell{{Text: "c"}, {Text: "1.2M"}}},
		},
	})
	for _, want := range []string{"321" + slot + "</span>", "2.7" + slot + "k</span>", "1.2" + slot + "M</span>"} {
		if !strings.Contains(got, want) {
			t.Errorf("number column missing %q:\n%s", want, got)
		}
	}
	plain := render(t, newRenderer(t), &Table{
		Columns: []TableColumn{{Label: "Name", Kind: "name"}, {Label: "Hits", Kind: "num"}},
		Rows:    []TableRow{{Cells: []TableCell{{Text: "a"}, {Text: "24"}}}},
	})
	if strings.Contains(plain, slot) {
		t.Errorf("a column that never abbreviates takes no slot:\n%s", plain)
	}
}

// TestStateMarksAreSquares: every mark that carries state is the one mark the
// app has — a 6px square with a 1px radius, the packet a router handles.
// Circles are for a switch's knob and the interface tree, never for state.
func TestStateMarksAreSquares(t *testing.T) {
	for name, w := range map[string]Widget{
		"badge": &Badge{Variant: "success", Text: "Up", Dot: true},
		"stat":  &Stat{Label: "Uptime", Value: "3 d", Sub: "since boot", Dot: true, Variant: "success"},
		"meter": &Meter{Compact: true, Label: "CPU", Value: "40", Fill: 40, Tone: "success"},
	} {
		// (a meter's track is a pill, which is a track, not a mark)
		got := render(t, newRenderer(t), w)
		if !strings.Contains(got, `<span class="size-1.5 shrink-0 rounded-[1px]`) || strings.Contains(got, "size-2 shrink-0 rounded-full") || strings.Contains(got, "size-1.5 rounded-full") {
			t.Errorf("%s: a state mark is a 6px square, not round:\n%s", name, got)
		}
	}
}

// TestTableStatesHaveWords: a yes/no cell is a check or nothing, and a missing
// value is a dash; each says so in words to a screen reader.
func TestTableStatesHaveWords(t *testing.T) {
	got := render(t, newRenderer(t), &Table{
		Columns: []TableColumn{{Label: "Name", Kind: "name"}, {Label: "Static", Kind: "check"}, {Label: "Address", Kind: "addr"}},
		Rows: []TableRow{
			{Cells: []TableCell{{Text: "a"}, {On: true}, {Text: "x"}}},
			{Cells: []TableCell{{Text: "b"}, {On: false}, {}}},
		},
	})
	for _, want := range []string{`<span class="sr-only">Yes</span>`, `<span class="sr-only">No</span>`, `<span aria-hidden="true">—</span><span class="sr-only">None</span>`} {
		if !strings.Contains(got, want) {
			t.Errorf("table missing %q:\n%s", want, got)
		}
	}
}
