// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package secrets

import (
	"regexp"
	"slices"
	"testing"
)

// TestEveryNameIsAConfigOption: config.option, each once, as uci names them.
func TestEveryNameIsAConfigOption(t *testing.T) {
	names := Base()
	if len(names) == 0 {
		t.Fatal("the list is empty")
	}
	name := regexp.MustCompile(`^[a-z0-9_-]+\.[A-Za-z0-9_]+$`)
	for i, n := range names {
		if !name.MatchString(n) {
			t.Errorf("%q is not config.option", n)
		}
		if slices.Contains(names[i+1:], n) {
			t.Errorf("%q is listed twice", n)
		}
	}
	if !slices.Contains(names, "wireless.key") {
		t.Error("Wi-Fi's key is missing")
	}
}
