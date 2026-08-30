// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package openwrt

import (
	"context"
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

func TestVerifyDestroysProbeSession(t *testing.T) {
	var destroyed string
	a := &RPCDAuthenticator{
		loginFn: func(_, password string) (string, error) {
			if password == "correct" {
				return "VERIFY-SID", nil
			}
			return "", nil
		},
		destroyFn: func(sid string) { destroyed = sid },
	}
	if err := a.Verify(context.Background(), "root", "correct"); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if destroyed != "VERIFY-SID" {
		t.Errorf("verification sid was not destroyed: %q", destroyed)
	}
}

// Root-password detection now lives in verso-rpcd (it reads /etc/shadow as
// root); its cases are covered by shadow_root_has_password's Rust unit test.
