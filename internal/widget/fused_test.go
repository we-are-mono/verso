// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// synflood is the firewall's rate and burst: two values that are one answer to
// "how many connections may start", each with the unit it counts in.
func synflood() *Grid {
	return &Grid{Style: "form", Columns: 2, Label: "Connection rate", Children: []Widget{
		&Field{Name: "synflood_rate", Label: "Connections per second", Key: "synflood_rate", Value: "25", Unit: "/s"},
		&Field{Name: "synflood_burst", Label: "Burst", Key: "synflood_burst", Value: "50", Unit: "packets"},
	}}
}

// TestRelatedValuesAreOneControl: values entered together are one setting —
// one row, one label covering every part, one chip naming every option — in
// one box split by hairlines, each part still its own input under its own
// name, posting under its own option.
func TestRelatedValuesAreOneControl(t *testing.T) {
	got := render(t, newRenderer(t), synflood())
	if strings.Contains(got, "verso-form-grid") {
		t.Errorf("related values are one control, not two columns:\n%s", got)
	}
	for want, n := range map[string]int{
		"verso-field-row":         1, // one row
		`class="verso-box"`:       1, // one box
		`class="verso-box-part"`:  2, // one part per value
		"verso-field-label ":      1, // one label line
		"<label":                  0, // the label names the group, not one input
		"data-verso-change-field": 2, // each part is its own change
	} {
		if got := strings.Count(got, want); got != n {
			t.Errorf("want %d of %q, got %d", n, want, got)
		}
	}
	for _, want := range []string{
		`<span id="synflood_rate-group-label" class="text-sm font-medium text-ink">Connection rate</span>`,
		`>synflood_rate · synflood_burst</span>`, // one chip, every option
		`role="group" aria-labelledby="synflood_rate-group-label"`,
		`id="synflood_rate" name="synflood_rate" value="25" type="text" aria-label="Connections per second"`,
		`data-verso-change-name="synflood_burst" data-verso-change-label="Burst" data-verso-change-kind="text" data-verso-writes="synflood_burst"`,
		`<span id="synflood_burst-unit" class="verso-box-unit">packets</span>`,
		`aria-describedby="synflood_burst-unit"`, // the unit is read with the value
	} {
		if !strings.Contains(got, want) {
			t.Errorf("fused control missing %q:\n%s", want, got)
		}
	}
}

// TestAFusedLabelFallsBackToItsParts: a group the plugin did not name is
// named by its parts, so the row is never unlabelled.
func TestAFusedLabelFallsBackToItsParts(t *testing.T) {
	g := synflood()
	g.Label = ""
	got := render(t, newRenderer(t), g)
	if !strings.Contains(got, `>Connections per second · Burst</span>`) {
		t.Errorf("an unnamed group is named by its parts:\n%s", got)
	}
}

// TestAFusedLabelExplainsItsParts: a fused row's description is the group's
// help, then each part's own help led by the part's name, so nothing a plugin
// wrote about a part goes missing, and the box is read with it.
func TestAFusedLabelExplainsItsParts(t *testing.T) {
	g := synflood()
	g.Children[0].(*Field).Help = "New connections allowed each second."
	g.Children[1].(*Field).Help = "Extra connections allowed at once."
	got := render(t, newRenderer(t), g)
	for _, want := range []string{
		`<p id="synflood_rate-group-desc" data-verso-field-desc class="text-sm leading-6 text-pretty text-body"><span>Connections per second: New connections allowed each second. Burst: Extra connections allowed at once.</span></p>`,
		`role="group" aria-labelledby="synflood_rate-group-label" aria-describedby="synflood_rate-group-desc"`,
		`synflood_rate · synflood_burst</span>`, // the chip names both options
	} {
		if !strings.Contains(got, want) {
			t.Errorf("fused description missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "-tip") || strings.Contains(got, "data-verso-tip") {
		t.Errorf("a fused row raises no tooltip:\n%s", got)
	}

	// A part that is the only one saying anything speaks for the row: its
	// name in front would only repeat the label over it.
	g.Children[1].(*Field).Help = ""
	got = render(t, newRenderer(t), g)
	if !strings.Contains(got, `data-verso-field-desc class="text-sm leading-6 text-pretty text-body"><span>New connections allowed each second.</span></p>`) {
		t.Errorf("a lone part's help is the row's description as it stands:\n%s", got)
	}
	g.Children[1].(*Field).Help = "Extra connections allowed at once."

	g.Help = "How fast new connections may arrive."
	got = render(t, newRenderer(t), g)
	if !strings.Contains(got, "><span>How fast new connections may arrive. Connections per second: New connections allowed each second. Burst: Extra connections allowed at once.</span></p>") {
		t.Errorf("the group's own help leads its description:\n%s", got)
	}
}

// TestDecodeAFusedGroup: the group's words travel on the grid and translate
// with the plugin's catalog, as every label does.
func TestDecodeAFusedGroup(t *testing.T) {
	w, err := Decode([]byte(`{"type":"grid","style":"form","columns":2,"label":"Connection rate","help":"How fast.",
		"children":[{"type":"field","name":"a","label":"A"},{"type":"field","name":"b","label":"B"}]}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	g := w.(*Grid)
	if g.Label != "Connection rate" || g.Help != "How fast." {
		t.Fatalf("grid words not decoded: %+v", g)
	}
	translateSchema(g, func(s string) string { return "«" + s + "»" })
	if g.Label != "«Connection rate»" || g.Help != "«How fast.»" {
		t.Errorf("grid words not translated: %q %q", g.Label, g.Help)
	}
}

// TestAFusedControlWearsOneMark: the group is one row, so a waiting change in
// any part marks the row once.
func TestAFusedControlWearsOneMark(t *testing.T) {
	g := synflood()
	g.Children[1].(*Field).Staged = true
	got := render(t, newRenderer(t), g)
	if n := strings.Count(got, "data-verso-staged-row"); n != 1 {
		t.Errorf("want one mark on the row, got %d:\n%s", n, got)
	}
}

// TestAFusedControlNamesTheRefusedPart: the band under the box says which part
// was refused, and only that part is invalid and pointed at its band.
func TestAFusedControlNamesTheRefusedPart(t *testing.T) {
	g := synflood()
	g.Children[1].(*Field).Error = "Enter a whole number."
	got := render(t, newRenderer(t), g)
	for _, want := range []string{
		`<span id="synflood_burst-error" data-verso-error`,
		`<span data-verso-error-text>Burst: Enter a whole number.</span>`,
		`aria-describedby="synflood_burst-error synflood_burst-unit" aria-invalid="true"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("refused part missing %q:\n%s", want, got)
		}
	}
	if n := strings.Count(got, `aria-invalid="true"`); n != 1 {
		t.Errorf("only the refused part is invalid, got %d:\n%s", n, got)
	}
}

// TestOnlyTypedValuesFuse: a choice, a list or a secret with its own reveal
// is not a value typed into a bare box, so a group holding one keeps its
// columns.
func TestOnlyTypedValuesFuse(t *testing.T) {
	for name, other := range map[string]Widget{
		"reveal": &Field{Name: "b", Label: "B", Kind: "password", Style: "reveal"},
		"list":   &List{Name: "b", Label: "B"},
	} {
		got := render(t, newRenderer(t), &Grid{Style: "form", Columns: 2, Children: []Widget{&Field{Name: "a", Label: "A"}, other}})
		if strings.Contains(got, "verso-box") || strings.Contains(got, "verso-form-grid") || strings.Count(got, `class="verso-field-row`) != 2 {
			t.Errorf("%s: a set holding one stands as a row per field, never fused, never columns:\n%s", name, got)
		}
	}
}

// TestAValueAndItsUnitFuse: a value and a dropdown of the units it is counted
// in are one value (form-fields): one row, the value's name as the question,
// one chip naming both options, and one box holding the value and the unit
// dropdown, which stands in the answer column.
func TestAValueAndItsUnitFuse(t *testing.T) {
	units := []Option{{Value: "seconds", Label: "seconds"}, {Value: "minutes", Label: "minutes"}, {Value: "hours", Label: "hours"}, {Value: "days", Label: "days"}}
	got := render(t, newRenderer(t), &Grid{Style: "form", Columns: 2, Children: []Widget{
		&Field{Name: "check_interval", Label: "Check every", Key: "check_interval", Value: "10"},
		&Field{Name: "check_unit", Label: "Unit", Kind: "select", Key: "check_unit", Value: "minutes", Options: units},
	}})
	for want, n := range map[string]int{
		`class="verso-field-row`: 1, `class="verso-box"`: 1, `class="verso-box-part"`: 2,
		`>Check every</span>`: 1, `>check_interval · check_unit</span>`: 1,
	} {
		if c := strings.Count(got, want); c != n {
			t.Errorf("want %d of %q, got %d:\n%s", n, want, c, got)
		}
	}
	if !strings.Contains(got, `<select id="check_unit" name="check_unit" aria-label="Unit"`) || !strings.Contains(got, "verso-box-select") {
		t.Errorf("the unit dropdown is not a part of the box:\n%s", got)
	}
	if strings.Contains(got, "verso-field-wide") || strings.Contains(got, ">Check every · Unit<") {
		t.Errorf("a value and its unit stand in the answer column under the value's own name:\n%s", got)
	}
}

// TestAFusedSetNamesItsParts: in a labelled set fused into one box each part
// shows its name inside its own trailing edge, where a unit stands, since the
// row's name is the set's (form-fields); a part with a unit shows the unit.
func TestAFusedSetNamesItsParts(t *testing.T) {
	got := render(t, newRenderer(t), &Grid{Style: "form", Columns: 3, Label: "Port and ranking", Children: []Widget{
		&Field{Name: "port", Label: "Port", Key: "port", Value: "5060"},
		&Field{Name: "priority", Label: "Priority", Key: "priority", Value: "10"},
		&Field{Name: "weight", Label: "Weight", Key: "weight", Value: "5"},
	}})
	for _, name := range []string{"Port", "Priority", "Weight"} {
		if !strings.Contains(got, `<span aria-hidden="true" class="verso-box-unit">`+name+`</span>`) {
			t.Errorf("the %s part does not name itself inside the box:\n%s", name, got)
		}
	}
	if !strings.Contains(got, `>port · priority · weight</span>`) {
		t.Errorf("the chip does not name every option:\n%s", got)
	}
	if burst := render(t, newRenderer(t), synflood()); strings.Contains(burst, `class="verso-box-unit">Burst<`) {
		t.Errorf("a part with a unit shows its unit, not its name:\n%s", burst)
	}
}

// TestAUnitIsNotANumber: a unit is words after the value, and a suffix such
// as a file's extension is one too — the box keeps the value's own measure
// and asks for no number pad.
func TestAUnitIsNotANumber(t *testing.T) {
	got := render(t, newRenderer(t), &Field{Name: "filename", Label: "File name", Value: "10-local", Unit: ".conf"})
	for _, want := range []string{`class="verso-box"`, `data-verso-measure="full"`, `<span id="filename-unit" class="verso-box-unit">.conf</span>`} {
		if !strings.Contains(got, want) {
			t.Errorf("suffix field missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, `inputmode="numeric"`) {
		t.Errorf("a suffix does not make a number:\n%s", got)
	}
}

// TestAUnitMakesANumberBox: a number counted in a unit sits in a box of one
// part, its unit words inside the frame — the box as wide as the value and
// the unit, however long the unit's word.
func TestAUnitMakesANumberBox(t *testing.T) {
	got := render(t, newRenderer(t), &Field{Name: "rate", Label: "Rate", Key: "synflood_rate", Value: "25", Unit: "Mbit/s"})
	for _, want := range []string{
		`class="verso-box"`, `class="verso-box-part" data-verso-measure="number"`,
		`inputmode="numeric"`, `<span id="rate-unit" class="verso-box-unit">Mbit/s</span>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("unit field missing %q:\n%s", want, got)
		}
	}
	for _, gone := range []string{"pr-20", `role="group"`} {
		if strings.Contains(got, gone) {
			t.Errorf("unit field must not carry %q:\n%s", gone, got)
		}
	}
	// The row tracks the change; the part does not track it a second time.
	if n := strings.Count(got, "data-verso-change-field"); n != 1 {
		t.Errorf("one change per value, got %d:\n%s", n, got)
	}
}
