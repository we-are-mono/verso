// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package openwrt

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRootHasPassword(t *testing.T) {
	cases := []struct {
		name   string
		shadow string
		want   bool
	}{
		{"no password (fresh boot)", "root::0:99999:7:::\n", false},
		{"hashed password", "root:$1$abc$xyz0:0:99999:7:::\n", true},
		{"root not first line", "daemon:*:0:::\nroot:$6$h$h:0:::\n", true},
		{"no root line fails safe", "daemon:*:0:::\nnobody:*:0:::\n", true},
	}
	for _, c := range cases {
		p := filepath.Join(t.TempDir(), "shadow")
		if err := os.WriteFile(p, []byte(c.shadow), 0o600); err != nil {
			t.Fatalf("write shadow: %v", err)
		}
		if got := (&ShadowSecurity{path: p}).RootHasPassword(); got != c.want {
			t.Errorf("%s: RootHasPassword = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestRootHasPasswordMissingFileFailsSafe(t *testing.T) {
	s := &ShadowSecurity{path: filepath.Join(t.TempDir(), "does-not-exist")}
	if !s.RootHasPassword() {
		t.Error("a missing shadow file should fail safe to true (no false 'no password' nag)")
	}
}
