// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// TestATextFileIsReadInTheBrowser: a plugin's file field ("text" style) posts
// what the file says, not the file: the picker itself is never submitted, the
// browser reads the chosen file into an ordinary field, and its name rides
// beside it. A file already read comes back named, so the form can be drawn
// again around it.
func TestATextFileIsReadInTheBrowser(t *testing.T) {
	got := render(t, newRenderer(t), &Field{
		Name: "profile", Label: "Profile", Kind: "file", Style: "text", Accept: ".ovpn,.conf",
		Prompt: "Drop the profile here", Value: "client\n", Chosen: "proton.ovpn", Reshapes: true,
	})
	for _, want := range []string{
		`type="file"`, `data-verso-file-read`, `accept=".ovpn,.conf"`,
		`<textarea name="profile" hidden data-verso-file-text>client`,
		`name="profile_name" value="proton.ovpn"`,
		`data-verso-reshape`,
		">proton.ovpn<",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("text file field missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, `type="file" name=`) || strings.Contains(got, `name="profile" type="file"`) {
		t.Errorf("the picker must not post the file itself:\n%s", got)
	}
}
