// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"bytes"
	"strings"
	"testing"
)

// TestModalSaysWhichStepItIsOn: a dialog that walks through steps says where
// it is, under its title — each step's packet filled when done, denim for the
// one in hand, hollow for what is still ahead — so a person dropping a file
// knows what happens after the drop.
func TestModalSaysWhichStepItIsOn(t *testing.T) {
	got := render(t, newRenderer(t), &Modal{Trigger: "Upload a custom image…", Title: "Upload a custom image",
		Steps: []string{"Choose", "Verify", "Install"}, Step: 1})
	for _, want := range []string{
		`<ol data-verso-steps aria-label="Steps"`,
		`<li data-verso-step data-state="done"`,
		`<li data-verso-step data-state="current" aria-current="step"`,
		`<li data-verso-step data-state="ahead"`,
		">Choose</span>", ">Verify</span>", ">Install</span>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("want %s in:\n%s", want, got)
		}
	}
	if plain := render(t, newRenderer(t), &Modal{Trigger: "Open", Title: "Plain"}); strings.Contains(plain, "data-verso-steps") {
		t.Errorf("a dialog without steps draws no step line:\n%s", plain)
	}
}

// TestFileFieldIsTheWholeDropArea: a file to drop is the dialog's whole
// subject, so its area fills the width in the app's own place-marking dashed
// hairline on sand, names what to drop and how else to choose, and says under
// it what fits. It is not a settings row with a 256px control column.
func TestFileFieldIsTheWholeDropArea(t *testing.T) {
	got := render(t, newRenderer(t), &Field{Name: "firmware_image", Kind: "file", Accept: ".bin", Required: true,
		Prompt: "Drop a sysupgrade image here", Help: "Built for Mono Gateway Development Kit · .bin, up to 128 MiB"})
	for _, want := range []string{
		`data-verso-drop`, "border-dashed border-rule-strong bg-quiet",
		`id="firmware_image" name="firmware_image" type="file" accept=".bin" required`,
		">Drop a sysupgrade image here<", "choose one from your computer",
		`<p id="firmware_image-help" class="text-sm leading-[1.45] text-pretty text-meta">Built for Mono Gateway Development Kit · .bin, up to 128 MiB</p>`,
		`aria-describedby="firmware_image-help"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("want %s in:\n%s", want, got)
		}
	}
	for _, gone := range []string{"sm:w-64", "border-dotted", "border-2"} {
		if strings.Contains(got, gone) {
			t.Errorf("the drop area is not a settings row's control (%s):\n%s", gone, got)
		}
	}
}

// TestModalContentsRenderAlone: a dialog answering its own upload replaces
// what it holds — title, steps and body — without its trigger or its frame.
func TestModalContentsRenderAlone(t *testing.T) {
	r := newRenderer(t)
	var out bytes.Buffer
	m := &Modal{Trigger: "Upload a custom image…", Title: "Upload a custom image", Steps: []string{"Choose", "Verify", "Install"}, Step: 2,
		Children: []Widget{&Callout{Variant: "success", Title: "Firmware verified"}}}
	if err := r.RenderModalContents(&out, m, "tok", "", nil); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{">Upload a custom image</h2>", `data-state="current" aria-current="step"`, "Firmware verified"} {
		if !strings.Contains(got, want) {
			t.Errorf("want %s in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Upload a custom image…") || strings.Contains(got, "x-teleport") {
		t.Errorf("the contents carry no trigger and no frame:\n%s", got)
	}
}
