// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSDKFixturesDecode pins the Rust SDK's typed Widget mirror to this
// package's decoder (ADR-006 §9): the SDK's tests serialize one sample of every
// variant into its testdata/, and every fixture must decode here. A renamed
// field or a wrong tag on either side fails this test, not a user's page.
// The plugin sources are a dev-only tree (ADR-006 §10), so absent fixtures
// skip rather than fail.
func TestSDKFixturesDecode(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "..", "plugins", "verso-plugin-sdk", "testdata", "widget-*.json"))
	if err != nil || len(files) == 0 {
		t.Skip("SDK conformance fixtures not present — run `cargo test` in plugins/verso-plugin-sdk to generate them")
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		if _, err := Decode(data); err != nil {
			t.Errorf("SDK fixture %s does not decode: %v", filepath.Base(f), err)
		}
	}
}

// TestSDKActDecodes: the page's act travels in the envelope, beside the widget
// tree, so the SDK pins it on its own; the shell must read it whole, blank
// panel and its children included.
func TestSDKActDecodes(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "plugins", "verso-plugin-sdk", "testdata", "act-new-rule.json"))
	if err != nil {
		t.Skip("SDK act fixture not present — run `cargo test` in plugins/verso-plugin-sdk to generate it")
	}
	act, err := DecodeHeadingAct(data)
	if err != nil {
		t.Fatalf("SDK act fixture does not decode: %v", err)
	}
	if act.Label != "Add rule" || !act.OpensPanel || act.Drawer == nil || !act.Drawer.Open || len(act.Drawer.Children) != 1 {
		t.Errorf("SDK act lost a field crossing the wire: %+v", act)
	}
}

// TestSDKRateReachesTheShell: a rate the SDK hands over as a number must arrive
// as one, or the shell has nothing to write in the reader's language.
func TestSDKRateReachesTheShell(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "plugins", "verso-plugin-sdk", "testdata", "widget-properties.json"))
	if err != nil {
		t.Skip("SDK properties fixture not present — run `cargo test` in plugins/verso-plugin-sdk to generate it")
	}
	w, err := Decode(data)
	if err != nil {
		t.Fatalf("SDK properties fixture does not decode: %v", err)
	}
	p, ok := w.(*Properties)
	if !ok || len(p.Items) == 0 || p.Items[len(p.Items)-1].Rate == nil || *p.Items[len(p.Items)-1].Rate != 2.5e9 {
		t.Errorf("SDK rate lost crossing the wire: %+v", w)
	}
}

// TestSDKDurationReachesTheShell: a duration the SDK hands over as seconds must
// arrive as a number, on a fact and on a cell, or the shell has nothing to
// write as every duration is written.
func TestSDKDurationReachesTheShell(t *testing.T) {
	dir := filepath.Join("..", "..", "plugins", "verso-plugin-sdk", "testdata")
	props, err := os.ReadFile(filepath.Join(dir, "widget-properties.json"))
	if err != nil {
		t.Skip("SDK properties fixture not present — run `cargo test` in plugins/verso-plugin-sdk to generate it")
	}
	w, err := Decode(props)
	if err != nil {
		t.Fatalf("SDK properties fixture does not decode: %v", err)
	}
	found := false
	for _, item := range w.(*Properties).Items {
		found = found || (item.Duration != nil && *item.Duration == 4523580)
	}
	if !found {
		t.Errorf("SDK duration lost crossing the wire on a fact: %+v", w)
	}
	table, err := os.ReadFile(filepath.Join(dir, "widget-table.json"))
	if err != nil {
		t.Fatalf("read the table fixture: %v", err)
	}
	if w, err = Decode(table); err != nil {
		t.Fatalf("SDK table fixture does not decode: %v", err)
	}
	found = false
	for _, row := range w.(*Table).Rows {
		for _, cell := range row.Cells {
			found = found || (cell.Duration != nil && *cell.Duration == 8040)
		}
	}
	if !found {
		t.Errorf("SDK duration lost crossing the wire on a cell")
	}
}

// TestSDKReshapeReachesTheShell: a choice the SDK marks as reshaping its form
// must arrive marked, or changing it would ask for nothing.
func TestSDKReshapeReachesTheShell(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "plugins", "verso-plugin-sdk", "testdata", "widget-select-reshapes.json"))
	if err != nil {
		t.Skip("SDK reshape fixture not present — run `cargo test` in plugins/verso-plugin-sdk to generate it")
	}
	w, err := Decode(data)
	if err != nil {
		t.Fatalf("SDK reshape fixture does not decode: %v", err)
	}
	if f, ok := w.(*Field); !ok || !f.Reshapes || f.Kind != "select" {
		t.Errorf("SDK reshaping choice lost its mark crossing the wire: %+v", w)
	}
}

// TestSDKTargetsReachTheShell: where a form's or section's options live, as
// the SDK serializes it, is what the shell marks staged controls by — it must
// survive the trip.
func TestSDKTargetsReachTheShell(t *testing.T) {
	dir := filepath.Join("..", "..", "plugins", "verso-plugin-sdk", "testdata")
	for file, want := range map[string]string{"widget-form.json": "system.cfg01e48a", "widget-section.json": "firewall.cfg0a1b2c"} {
		data, err := os.ReadFile(filepath.Join(dir, file))
		if err != nil {
			t.Skipf("SDK fixture %s not present: %v", file, err)
		}
		w, err := Decode(data)
		if err != nil {
			t.Fatalf("decode %s: %v", file, err)
		}
		holder, ok := w.(targetHolder)
		if !ok || holder.defaultTarget() != want {
			t.Errorf("%s: target lost in decode, want %q", file, want)
		}
	}
}

// TestSDKPhraseReachesTheShell: a phrase's parts arrive each as the one it
// is, in order.
func TestSDKPhraseReachesTheShell(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "plugins", "verso-plugin-sdk", "testdata", "widget-table-phrase.json"))
	if err != nil {
		t.Skip("SDK phrase fixture not present — run `cargo test` in plugins/verso-plugin-sdk to generate it")
	}
	w, err := Decode(data)
	if err != nil {
		t.Fatalf("SDK phrase fixture does not decode: %v", err)
	}
	parts := w.(*Table).Rows[0].Cells[0].Parts
	if len(parts) != 3 || parts[0].Chip == nil || parts[0].Chip.Label != "Allow-Ping" || parts[1].Value != "ACCEPT" || parts[2].Words != "→" {
		t.Errorf("SDK phrase lost a part crossing the wire: %+v", parts)
	}
}

// TestSDKMeterMarksReachTheShell: a meter cell's warn limit and its no-track
// state arrive as the SDK built them, and a share that sends neither arrives
// bare.
func TestSDKMeterMarksReachTheShell(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "plugins", "verso-plugin-sdk", "testdata", "widget-table-meter-marks.json"))
	if err != nil {
		t.Skip("SDK meter marks fixture not present — run `cargo test` in plugins/verso-plugin-sdk to generate it")
	}
	w, err := Decode(data)
	if err != nil {
		t.Fatalf("SDK meter marks fixture does not decode: %v", err)
	}
	rows := w.(*Table).Rows
	if c := rows[0].Cells[0]; c.WarnMark != 89 || c.Fill != 50 || c.NoTrack {
		t.Errorf("SDK headroom lost its marks crossing the wire: %+v", c)
	}
	if c := rows[1].Cells[0]; !c.NoTrack || c.Text != "71.0 °C" {
		t.Errorf("SDK reading with no ceiling lost its state crossing the wire: %+v", c)
	}
	if c := rows[2].Cells[0]; c.WarnMark != 0 || c.NoTrack || c.Fill != 61 {
		t.Errorf("SDK share gained marks crossing the wire: %+v", c)
	}
}

// TestSDKFactHeadsReachTheShell: a fact sheet's head arrives as the SDK
// built it, a group's title and chip or two columns' names, and so does the
// sentence that stands where its rows would.
func TestSDKFactHeadsReachTheShell(t *testing.T) {
	for file, want := range map[string]PropertiesHead{
		"widget-properties-headed.json":  {Label: "IPv4", Chip: "dhcp"},
		"widget-properties-columns.json": {Label: "Part", Value: "Installed"},
	} {
		data, err := os.ReadFile(filepath.Join("..", "..", "plugins", "verso-plugin-sdk", "testdata", file))
		if err != nil {
			t.Skip("SDK fact head fixtures not present — run `cargo test` in plugins/verso-plugin-sdk to generate them")
		}
		w, err := Decode(data)
		if err != nil {
			t.Fatalf("%s does not decode: %v", file, err)
		}
		p, ok := w.(*Properties)
		if !ok || p.Head == nil || *p.Head != want || p.Empty == "" {
			t.Errorf("%s: head or empty sentence lost crossing the wire: %+v", file, w)
		}
	}
}
