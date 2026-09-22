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
