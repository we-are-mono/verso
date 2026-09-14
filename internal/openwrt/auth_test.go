// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package openwrt

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/we-are-mono/verso/internal/ubus"
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

// TestRenewReportsLiveness: Renew reports a session gone ONLY when rpcd answers
// NOT_FOUND; a live session and a transport blip both leave it alive, so the
// renewer never evicts an operator over a momentary rpcd stumble (ADR-007 §7).
func TestRenewReportsLiveness(t *testing.T) {
	cases := []struct {
		name  string
		err   error
		alive bool
	}{
		{"live session", nil, true},
		{"session gone", &ubus.StatusError{Code: ubus.StatusNotFound, Phase: ubus.PhaseInvoke, Call: "access"}, false},
		{"gone, wrapped", fmt.Errorf("openwrt: %w", &ubus.StatusError{Code: ubus.StatusNotFound}), false},
		{"other ubus status", &ubus.StatusError{Code: ubus.StatusNotFound + 1}, true},
		{"transport error", errors.New("dial: connection refused"), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got string
			a := &RPCDAuthenticator{accessFn: func(sid string) error { got = sid; return tc.err }}
			if alive := a.Renew(context.Background(), "SID-1"); alive != tc.alive {
				t.Errorf("Renew alive = %v, want %v", alive, tc.alive)
			}
			if got != "SID-1" {
				t.Errorf("Renew probed sid %q, want SID-1", got)
			}
		})
	}
}

// TestDestroyTearsDownSession: Destroy hands the sid to the teardown seam.
func TestDestroyTearsDownSession(t *testing.T) {
	var destroyed string
	a := &RPCDAuthenticator{destroyFn: func(sid string) { destroyed = sid }}
	a.Destroy(context.Background(), "SID-2")
	if destroyed != "SID-2" {
		t.Errorf("Destroy tore down %q, want SID-2", destroyed)
	}
}

// Root-password detection now lives in verso-rpcd (it reads /etc/shadow as
// root); its cases are covered by shadow_root_has_password's Rust unit test.
