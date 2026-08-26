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

// fakePkgs records package-manager calls and serves canned search results.
type fakePkgs struct {
	updated            bool
	installed, removed []string
	found              []Package
	err                error
}

func (f *fakePkgs) Update() error { f.updated = true; return f.err }
func (f *fakePkgs) Search(string, int) ([]Package, int, error) {
	return f.found, len(f.found), f.err
}
func (f *fakePkgs) Installed() ([]Package, error) { return f.found, f.err }
func (f *fakePkgs) Install(name string) (string, error) {
	f.installed = append(f.installed, name)
	return "OK", f.err
}
func (f *fakePkgs) Remove(name string) (string, error) {
	f.removed = append(f.removed, name)
	return "OK", f.err
}
func (f *fakePkgs) CheckedAt() int64 { return 42 }

func run(h *Helper, stdin string, args ...string) (int, string) {
	var out strings.Builder
	code := h.Run(args, strings.NewReader(stdin), &out)
	return code, out.String()
}

func TestListAdvertisesMethods(t *testing.T) {
	h := New(fakeAuth{}, &fakePasswd{}, &fakePkgs{})
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
	h := New(fakeAuth{allow: map[string]bool{"good-sid": true}}, pw, &fakePkgs{})
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
	h := New(fakeAuth{allow: map[string]bool{"good-sid": true}}, pw, &fakePkgs{})
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
	h := New(fakeAuth{err: errors.New("ubus down")}, pw, &fakePkgs{})
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
	h := New(fakeAuth{allow: map[string]bool{"good-sid": true}}, pw, &fakePkgs{})
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
	h := New(fakeAuth{allow: map[string]bool{"good-sid": true}}, pw, &fakePkgs{})
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
	h := New(fakeAuth{allow: map[string]bool{"good-sid": true}}, pw, &fakePkgs{})
	code, _ := run(h, `{"ubus_rpc_session":"good-sid","username":"root","password":"x2345678"}`, "call", "setPassword")
	if code != statusUnknownError {
		t.Fatalf("exit = %d, want %d (unknown error)", code, statusUnknownError)
	}
}

func TestUnknownMethod(t *testing.T) {
	h := New(fakeAuth{allow: map[string]bool{"good-sid": true}}, &fakePasswd{}, &fakePkgs{})
	code, _ := run(h, `{"ubus_rpc_session":"good-sid"}`, "call", "reboot")
	if code != statusMethodNotFound {
		t.Fatalf("exit = %d, want %d (method not found)", code, statusMethodNotFound)
	}
}

func TestNoArgsIsInvalid(t *testing.T) {
	h := New(fakeAuth{}, &fakePasswd{}, &fakePkgs{})
	if code, _ := run(h, ""); code != statusInvalidArgument {
		t.Fatalf("exit = %d, want %d", code, statusInvalidArgument)
	}
}

// TestPkgInstallValidatesName: a name outside the exact package alphabet is
// refused before the package manager is touched — options, globs, and shell
// metacharacters are untypeable (ADR-011 §4).
func TestPkgInstallValidatesName(t *testing.T) {
	pm := &fakePkgs{}
	h := New(fakeAuth{allow: map[string]bool{"good-sid": true}}, &fakePasswd{}, pm)
	for _, bad := range []string{"", "-rf", "a b", "htop;reboot", "../etc", "htop*"} {
		code, _ := run(h, `{"ubus_rpc_session":"good-sid","package":"`+bad+`"}`, "call", "pkgInstall")
		if code != statusInvalidArgument {
			t.Errorf("package %q: exit = %d, want %d", bad, code, statusInvalidArgument)
		}
	}
	if len(pm.installed) != 0 {
		t.Fatalf("invalid names must not reach the manager: %v", pm.installed)
	}
}

// TestPkgInstallRuns: a valid gated call reaches the manager and reports OK.
func TestPkgInstallRuns(t *testing.T) {
	pm := &fakePkgs{}
	h := New(fakeAuth{allow: map[string]bool{"good-sid": true}}, &fakePasswd{}, pm)
	code, out := run(h, `{"ubus_rpc_session":"good-sid","package":"htop"}`, "call", "pkgInstall")
	if code != statusOK || len(pm.installed) != 1 || pm.installed[0] != "htop" {
		t.Fatalf("exit=%d installed=%v out=%s", code, pm.installed, out)
	}
}

// TestPkgRemoveKeepsBase: the base set that keeps the device (and this
// surface) alive is not removable.
func TestPkgRemoveKeepsBase(t *testing.T) {
	pm := &fakePkgs{}
	h := New(fakeAuth{allow: map[string]bool{"good-sid": true}}, &fakePasswd{}, pm)
	code, _ := run(h, `{"ubus_rpc_session":"good-sid","package":"busybox"}`, "call", "pkgRemove")
	if code != statusInvalidArgument || len(pm.removed) != 0 {
		t.Fatalf("exit=%d removed=%v — busybox must be refused", code, pm.removed)
	}
}

// TestPkgSearchShape: a search returns the packages array and the total.
func TestPkgSearchShape(t *testing.T) {
	pm := &fakePkgs{found: []Package{{Name: "htop", Version: "3.5.1-r1", Feed: "packages", Installed: false}}}
	h := New(fakeAuth{allow: map[string]bool{"good-sid": true}}, &fakePasswd{}, pm)
	code, out := run(h, `{"ubus_rpc_session":"good-sid","query":"htop"}`, "call", "pkgSearch")
	if code != statusOK {
		t.Fatalf("exit = %d, want ok", code)
	}
	for _, want := range []string{`"htop"`, `"total":1`, `"packages"`} {
		if !strings.Contains(out, want) {
			t.Errorf("search output missing %s: %s", want, out)
		}
	}
}
