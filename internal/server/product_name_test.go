// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// productNamed are the messages that may say "Verso": each names the software
// itself as a thing with a version, never as something acting or a place
// someone is.
var productNamed = map[string]bool{
	"Verso":                      true,
	"Verso version":              true,
	"Plugin needs a newer Verso": true,
}

// TestNoMessageNamesTheProduct: Verso is the interface, not an actor — a
// message says what happened ("That change couldn’t be applied"), never what
// Verso did, and a place is the router, not Verso. Every message someone reads
// is a key in the catalogs (the i18n audit keeps them complete), so reading
// those reads them all, the plugins' included.
func TestNoMessageNamesTheProduct(t *testing.T) {
	var catalogs []string
	for _, pattern := range []string{"../../i18n/*/*.json", "../../plugins/*/i18n/*.json"} {
		found, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		catalogs = append(catalogs, found...)
	}
	if len(catalogs) == 0 {
		t.Fatal("no catalogs found")
	}
	for _, path := range catalogs {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var messages map[string]string
		if err := json.Unmarshal(raw, &messages); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		for key := range messages {
			if strings.Contains(key, "Verso") && !productNamed[key] {
				t.Errorf("%s: %q names Verso", path, key)
			}
		}
	}
}
