// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// refusalBand is how a refusal reads under the box it refuses: a band on
// crimson's own wash, the crimson square on its first 20px line, then the
// words — the same band under a field, a key's box, or a list's box. Half a
// cell inside at top and bottom keeps it whole cells of the notebook's grid.
const refusalBand = `class="flex items-start gap-2 rounded-xs bg-crimson-soft px-3 py-2.5 text-sm leading-5 text-crimson-deep"><span aria-hidden="true" class="mt-1.75 size-1.5 shrink-0 rounded-[1px] bg-crimson"></span><span data-verso-error-text>`

// TestEveryRefusalUnderABoxIsOneBand: a password refused, a key the router
// cannot read, a server that is not a host — each refusal under its box is
// the same band, so a wrong value reads the same wherever it was typed.
func TestEveryRefusalUnderABoxIsOneBand(t *testing.T) {
	r := newRenderer(t)
	for name, w := range map[string]Widget{
		"field":          &Field{Name: "password", Label: "New password", Kind: "password", Error: "Use at least 8 characters."},
		"collection add": &Collection{Add: &CollectionAdd{Label: "Add a key", Name: "authorized_key", Submit: "Add key", Error: "Paste one complete SSH public key."}},
		"list box":       &List{Name: "server", Label: "Servers", Items: []string{"not a host"}, Errors: map[string]string{"0": "Enter a hostname."}},
	} {
		got := render(t, r, w)
		if !strings.Contains(got, refusalBand) {
			t.Errorf("%s: the refusal is not the band:\n%s", name, got)
		}
	}
}
