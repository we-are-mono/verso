// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package i18n

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"testing"
)

// spacedUnit is a figure, an ordinary space and a unit symbol: what the
// Narrow Space Rule (DESIGN.md) writes with U+202F instead, so "32 KiB" reads
// as one word and never breaks at a line's end. Units spelled as words
// ("5 minutes") are prose and take an ordinary space.
var spacedUnit = regexp.MustCompile(`[0-9] (KiB|MiB|GiB|TiB|kB|MB|GB|B|MHz|GHz|kHz|dBm|dB|%|°C|ms|s|min|h|d|W|V|A|Mb/s|Gb/s|kb/s)([^\p{L}0-9]|$)`)

// TestShippedCatalogsKeepTheNarrowSpace: every catalog Verso and the plugins
// in this checkout ship holds its figures to the rule, in the English source
// string (the key, which is what the code writes) and in its translation.
func TestShippedCatalogsKeepTheNarrowSpace(t *testing.T) {
	var files []string
	for _, pattern := range []string{"../../i18n/*/*.json", "../../plugins/*/i18n/*.json"} {
		found, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, found...)
	}
	if len(files) == 0 {
		t.Fatal("no catalogs found")
	}
	for _, name := range files {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		var catalog map[string]string
		if err := json.Unmarshal(raw, &catalog); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		keys := make([]string, 0, len(catalog))
		for key := range catalog {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			for _, text := range []string{key, catalog[key]} {
				if spacedUnit.MatchString(text) {
					t.Errorf("%s: %q spaces a unit with an ordinary space, not U+202F", filepath.ToSlash(name), text)
				}
			}
		}
	}
}
