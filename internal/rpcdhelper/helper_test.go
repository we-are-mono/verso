// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package rpcdhelper

import (
	"errors"
	"strings"
	"testing"
)

// fakeAuth grants sids in its allow set; a non-empty err makes every probe fail
// (which must deny, fail-closed).
type fakeAuth struct {
	allow map[string]bool
	err   error
}

func (f fakeAuth) Access(sid, _ string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.allow[sid], nil
}

// fakePasswd records the last SetPassword call and can force an error.
type fakePasswd struct {
	called           bool
	gotUser, gotPass string
	err              error
}

func (f *fakePasswd) SetPassword(username, password string) error {
	f.called, f.gotUser, f.gotPass = true, username, password
	return f.err
}

func run(h *Helper, stdin string, args ...string) (int, string) {
	var out strings.Builder
	code := h.Run(args, strings.NewReader(stdin), &out)
	return code, out.String()
}

func TestListAdvertisesMethods(t *testing.T) {
	h := New(fakeAuth{}, &fakePasswd{})
	code, out := run(h, "", "list")
	if code != statusOK {
		t.Fatalf("list exit = %d, want 0", code)
	}
	if !strings.Contains(out, `"setPassword"`) ||
		!strings.Contains(out, `"username"`) || !strings.Contains(out, `"password"`) {
		t.Errorf("list output missing setPassword schema: %s", out)
	}
}

func TestSetPasswordAuthorized(t *testing.T) {
	pw := &fakePasswd{}
	h := New(fakeAuth{allow: map[string]bool{"good-sid": true}}, pw)
	code, _ := run(h, `{"ubus_rpc_session":"good-sid","username":"root","password":"correct-horse"}`, "call", "setPassword")
	if code != statusOK {
		t.Fatalf("exit = %d, want 0", code)
	}
	if !pw.called || pw.gotUser != "root" || pw.gotPass != "correct-horse" {
		t.Errorf("SetPassword got (%q,%q) called=%v, want (root,correct-horse) true", pw.gotUser, pw.gotPass, pw.called)
	}
}

// The self-gate is the security boundary: an unauthorized session must be
// refused with PERMISSION_DENIED and the action must not run.
func TestSetPasswordUnauthorizedDenied(t *testing.T) {
	pw := &fakePasswd{}
	h := New(fakeAuth{allow: map[string]bool{"good-sid": true}}, pw)
	code, _ := run(h, `{"ubus_rpc_session":"bad-sid","username":"root","password":"correct-horse"}`, "call", "setPassword")
	if code != statusPermissionDenied {
		t.Fatalf("exit = %d, want %d (permission denied)", code, statusPermissionDenied)
	}
	if pw.called {
		t.Error("SetPassword ran for an unauthorized session")
	}
}

// A probe error fails closed — the password is not changed.
func TestSetPasswordProbeErrorDenied(t *testing.T) {
	pw := &fakePasswd{}
	h := New(fakeAuth{err: errors.New("ubus down")}, pw)
	code, _ := run(h, `{"ubus_rpc_session":"any","username":"root","password":"x2345678"}`, "call", "setPassword")
	if code != statusPermissionDenied {
		t.Fatalf("exit = %d, want permission denied on probe error", code)
	}
	if pw.called {
		t.Error("SetPassword ran despite a failed authorization probe")
	}
}

// No stdin (hence no sid) is denied, not acted on — an argument-less call cannot
// slip past the gate.
func TestCallWithoutArgsDenied(t *testing.T) {
	pw := &fakePasswd{}
	h := New(fakeAuth{allow: map[string]bool{"good-sid": true}}, pw)
	code, _ := run(h, "", "call", "setPassword")
	if code != statusPermissionDenied {
		t.Fatalf("exit = %d, want permission denied", code)
	}
	if pw.called {
		t.Error("SetPassword ran with no arguments")
	}
}

func TestSetPasswordMissingFields(t *testing.T) {
	pw := &fakePasswd{}
	h := New(fakeAuth{allow: map[string]bool{"good-sid": true}}, pw)
	code, _ := run(h, `{"ubus_rpc_session":"good-sid","username":"root"}`, "call", "setPassword")
	if code != statusInvalidArgument {
		t.Fatalf("exit = %d, want %d (invalid argument)", code, statusInvalidArgument)
	}
	if pw.called {
		t.Error("SetPassword ran with a missing password")
	}
}

func TestPasswdFailureIsUnknownError(t *testing.T) {
	pw := &fakePasswd{err: errors.New("passwd blew up")}
	h := New(fakeAuth{allow: map[string]bool{"good-sid": true}}, pw)
	code, _ := run(h, `{"ubus_rpc_session":"good-sid","username":"root","password":"x2345678"}`, "call", "setPassword")
	if code != statusUnknownError {
		t.Fatalf("exit = %d, want %d (unknown error)", code, statusUnknownError)
	}
}

func TestUnknownMethod(t *testing.T) {
	h := New(fakeAuth{allow: map[string]bool{"good-sid": true}}, &fakePasswd{})
	code, _ := run(h, `{"ubus_rpc_session":"good-sid"}`, "call", "reboot")
	if code != statusMethodNotFound {
		t.Fatalf("exit = %d, want %d (method not found)", code, statusMethodNotFound)
	}
}

func TestNoArgsIsInvalid(t *testing.T) {
	h := New(fakeAuth{}, &fakePasswd{})
	if code, _ := run(h, ""); code != statusInvalidArgument {
		t.Fatalf("exit = %d, want %d", code, statusInvalidArgument)
	}
}
