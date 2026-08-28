// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package openwrt

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestLoginRejectsPermissiveAccount: a passwordless/permissive account (rpcd
// hands a session to ANY password) must not authenticate an arbitrary password —
// only a genuinely blank one passes.
func TestLoginRejectsPermissiveAccount(t *testing.T) {
	destroyed := 0
	a := &RPCDAuthenticator{
		loginFn:   func(_, _ string) (string, error) { return "SID", nil }, // grants anything
		destroyFn: func(string) { destroyed++ },
	}
	if sid, err := a.Login(context.Background(), "root", "anything"); err == nil || sid != "" {
		t.Errorf("a password on a permissive account must be rejected: sid=%q err=%v", sid, err)
	}
	if destroyed != 2 {
		t.Errorf("the real and decoy sessions should both be destroyed, got %d", destroyed)
	}
	if sid, err := a.Login(context.Background(), "root", ""); err != nil || sid != "SID" {
		t.Errorf("a blank password (the passwordless device's real credential) should pass: sid=%q err=%v", sid, err)
	}
}

// TestLoginStrictAccount: an account that actually checks the password lets the
// correct one through (the wrong decoy is rejected) and blocks a wrong one.
func TestLoginStrictAccount(t *testing.T) {
	a := &RPCDAuthenticator{
		loginFn: func(_, password string) (string, error) {
			if password == "correct" {
				return "SID", nil
			}
			return "", nil // rpcd issued no session
		},
		destroyFn: func(string) {},
	}
	if sid, err := a.Login(context.Background(), "root", "correct"); err != nil || sid != "SID" {
		t.Errorf("correct password should authenticate: sid=%q err=%v", sid, err)
	}
	if _, err := a.Login(context.Background(), "root", "wrong"); err == nil {
		t.Error("wrong password should be rejected")
	}
}

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
