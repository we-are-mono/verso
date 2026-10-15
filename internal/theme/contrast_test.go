// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package theme

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/we-are-mono/verso/internal/palette"
)

// sandLight is the shipped palette, the ground every theme lies over.
func sandLight(t *testing.T) palette.Palette {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("..", "server", "assets", "palette.css"))
	if err != nil {
		t.Fatal(err)
	}
	p, err := palette.Parse(string(src))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSandLightKeepsItsOwnFloors(t *testing.T) {
	if got := (Theme{ID: SandLight, Scheme: Light}).Contrast(sandLight(t)); len(got) > 0 {
		t.Errorf("Sand light falls short: %v", got)
	}
}

func TestContrastNamesThePairThatFallsShort(t *testing.T) {
	// Meta at #8b857a stands at about 3.5 : 1 on Paper.
	got := (Theme{ID: "dim", Scheme: Light, Colors: map[string]string{"meta": "#8b857a"}}).Contrast(sandLight(t))
	if len(got) == 0 {
		t.Fatal("a meta at 3.5 : 1 passes")
	}
	joined := strings.Join(got, "\n")
	if !strings.Contains(joined, "meta on ground") || !strings.Contains(joined, "4.5") {
		t.Errorf("the report %q names neither the pair nor its floor", joined)
	}
}

func TestContrastReadsATranslucentWordOverItsGround(t *testing.T) {
	// Ink at 30% over Paper is a pale grey, whatever Ink is.
	got := (Theme{ID: "ghost", Scheme: Light, Colors: map[string]string{"ink": "#1b19174d"}}).Contrast(sandLight(t))
	if !strings.Contains(strings.Join(got, "\n"), "ink on ground") {
		t.Errorf("a translucent ink passes: %v", got)
	}
}

func TestADarkThemeThatKeepsLightShadowsIsReported(t *testing.T) {
	dark := map[string]string{"ground": "#1c1a17", "quiet": "#151412", "canvas": "#121110", "mid": "#2a2723",
		"ink": "#eeeae3", "body": "#bdb6aa", "meta": "#a49e92", "glyph": "#8f897e"}
	got := (Theme{ID: "night", Scheme: Dark, Colors: dark}).Contrast(sandLight(t))
	if !strings.Contains(strings.Join(got, "\n"), "shadow") {
		t.Errorf("a dark theme with Sand light's shadows is not reported: %v", got)
	}
	dark["shadow-dialog"] = "#00000080"
	for _, line := range (Theme{ID: "night", Scheme: Dark, Colors: dark}).Contrast(sandLight(t)) {
		if strings.Contains(line, "shadow") {
			t.Errorf("a dark theme that sets a shadow colour is still reported: %q", line)
		}
	}
}
