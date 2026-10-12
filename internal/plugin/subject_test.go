// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package plugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestTheSDKNamesTheSameSubjectKinds pins the Rust SDK's SubjectKind to
// SubjectKinds: a kind a plugin can name and the shell drops, or one the shell
// draws and no plugin can name, fails here. The SDK's tests write the fixture;
// absent, it skips, as the widget fixtures do.
func TestTheSDKNamesTheSameSubjectKinds(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "plugins", "verso-plugin-sdk", "testdata", "subject-kinds.json"))
	if err != nil {
		t.Skip("SDK subject kinds fixture not present — run `cargo test` in plugins/verso-plugin-sdk to generate it")
	}
	var kinds []string
	if err := json.Unmarshal(data, &kinds); err != nil {
		t.Fatal(err)
	}
	sdk := map[string]bool{}
	for _, kind := range kinds {
		sdk[kind] = true
	}
	if !reflect.DeepEqual(sdk, SubjectKinds) {
		t.Errorf("the SDK's subject kinds\n%v\nthe shell's\n%v", sdk, SubjectKinds)
	}
}
