// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package theme

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/we-are-mono/verso/internal/palette"
)

// TestTheShippedThemesKeepEveryFloor loads the themes the verso package ships
// against the shipped palette's tokens: each loads, and each keeps every
// contrast floor, its shadows darkened when it is dark.
func TestTheShippedThemesKeepEveryFloor(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "server", "assets", "palette.css"))
	if err != nil {
		t.Fatal(err)
	}
	set, problems := Load(os.DirFS(filepath.Join("..", "..", "themes")), palette.TokenNames(string(src)))
	for _, p := range problems {
		t.Error(p)
	}
	for _, want := range []string{SandDark, "catppuccin-mocha"} {
		if _, ok := set[want]; !ok {
			t.Errorf("the shipped themes lack %s", want)
		}
	}
	base := sandLight(t)
	for _, th := range set {
		for _, short := range th.Contrast(base) {
			t.Error(short)
		}
	}
}
