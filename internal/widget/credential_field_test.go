// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// TestSignInFieldsTakeTheCredentialMeasure: a username and its password are a
// short pair, not a line of prose: in the answer column they are the column's
// width, and where a row stacks they keep the credential measure rather than
// spanning the row, on any page that asks for a sign-in.
func TestSignInFieldsTakeTheCredentialMeasure(t *testing.T) {
	r := newRenderer(t)
	for _, f := range []*Field{
		{Name: "username", Label: "Username", Key: "username"},
		{Name: "password", Label: "Password", Kind: "password", Key: "password"},
	} {
		if got := render(t, r, f); !strings.Contains(got, `data-verso-control-measure="secret"`) {
			t.Errorf("%s does not take the credential measure:\n%s", f.Name, got)
		}
	}
	if got := render(t, r, &Field{Name: "name", Label: "Name"}); strings.Contains(got, `data-verso-control-measure="secret"`) {
		t.Errorf("a name is not a credential:\n%s", got)
	}
}
