// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// TestRevealPasswordShowsTheKeyOnRequest: a shared secret — a Wi-Fi key that
// gets read out to a guest — is in the field, masked, with an eye beside it
// that shows it. A sign-in password never is: only the reveal style reflects
// its value.
func TestRevealPasswordShowsTheKeyOnRequest(t *testing.T) {
	r := newRenderer(t)
	got := render(t, r, &Field{Name: "key", Label: "Password", Kind: "password", Style: "reveal", Value: "correct-horse"})
	for _, want := range []string{
		`x-data="reveal"`,
		`:type="inputType"`, // a getter: the CSP build takes no expressions
		`type="password"`,   // masked until asked
		`value="correct-horse"`,
		`@click="toggle"`,
		`:aria-pressed="pressed"`,
		`aria-label="Show password"`,
		"size-9", // the 36px square beside the field
	} {
		if !strings.Contains(got, want) {
			t.Errorf("reveal field missing %q:\n%s", want, got)
		}
	}

	plain := render(t, r, &Field{Name: "password", Label: "Password", Kind: "password", Value: "hunter2"})
	if strings.Contains(plain, "hunter2") || strings.Contains(plain, `x-data="reveal"`) {
		t.Errorf("a plain password field must never reflect its value:\n%s", plain)
	}
}
