// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSDKFixturesDecode pins the Rust SDK's typed Widget mirror to this
// package's decoder (ADR-006 §9): the SDK's tests serialize one sample of every
// variant into its testdata/, and every fixture must decode here. A renamed
// field or a wrong tag on either side fails this test, not a user's page.
// The plugin sources are a dev-only tree (ADR-006 §10), so absent fixtures
// skip rather than fail.
func TestSDKFixturesDecode(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "..", "plugins", "verso-plugin-sdk", "testdata", "widget-*.json"))
	if err != nil || len(files) == 0 {
		t.Skip("SDK conformance fixtures not present — run `cargo test` in plugins/verso-plugin-sdk to generate them")
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		if _, err := Decode(data); err != nil {
			t.Errorf("SDK fixture %s does not decode: %v", filepath.Base(f), err)
		}
	}
}
