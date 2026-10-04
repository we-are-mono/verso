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
