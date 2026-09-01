// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package openwrt

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"

	"github.com/we-are-mono/verso/internal/ubus"
)

// ErrInvalidCredentials is returned when rpcd accepts the call but issues no
// session — i.e. the username/password were rejected.
var ErrInvalidCredentials = errors.New("openwrt: invalid credentials")

// RPCDAuthenticator authenticates against rpcd's session.login over the ubus
// socket, returning the session id. The sid is held server-side; it never
// reaches the browser.
//
// It does NOT blindly trust rpcd. When an account has no password, rpcd's
// `$p$root` login grants a session for ANY password — so a passwordless router
// would accept "anything" as a login. To close that, a non-empty password that
// succeeds is re-checked with a deliberately-wrong decoy: if the decoy ALSO
// yields a session, the account is not actually authenticating, so the login is
// rejected. Only a genuinely empty password (the true credential of a
// passwordless device) passes — and the shell nudges the operator to set one.
// This is fail-closed: it never depends on reading /etc/shadow, which the shell
// may not be able to do.
type RPCDAuthenticator struct {
	socket string // "" = default ubus socket
	// loginFn / destroyFn / accessFn seam the rpcd calls for tests; nil uses the
	// real ubus.
	loginFn   func(username, password string) (sid string, err error)
	destroyFn func(sid string)
	accessFn  func(sid string) error
}

// NewRPCDAuthenticator returns an authenticator using the default ubus socket.
func NewRPCDAuthenticator() *RPCDAuthenticator { return &RPCDAuthenticator{} }

// Login returns the rpcd session id (sid) for valid credentials, rejecting a
// permissive/passwordless account that would accept an arbitrary password.
func (a *RPCDAuthenticator) Login(_ context.Context, username, password string) (string, error) {
	sid, err := a.call(username, password)
	if err != nil {
		return "", err
	}
	if sid == "" {
		return "", ErrInvalidCredentials
	}
	// A non-empty password must be genuinely checked: if a wrong decoy also
	// authenticates, the account grants any password (passwordless/permissive),
	// so this login is not trustworthy.
	if password != "" {
		if probe, err := a.call(username, decoyPassword()); err == nil && probe != "" {
			a.destroy(sid)
			a.destroy(probe)
			return "", ErrInvalidCredentials
		}
	}
	return sid, nil
}

// Verify checks a credential without retaining the short-lived rpcd session it
// creates. The Access page uses this before changing an existing password.
func (a *RPCDAuthenticator) Verify(ctx context.Context, username, password string) error {
	sid, err := a.Login(ctx, username, password)
	if sid != "" {
		a.destroy(sid)
	}
	return err
}

// Renew touches the rpcd session behind sid so rpcd resets its inactivity timer
// (rpcd renews a session merely by looking it up for a session.access call,
// before it evaluates any ACL). It reports whether the session still exists:
// false only when rpcd positively answers that the sid is unknown
// (UBUS_STATUS_NOT_FOUND). A transport failure leaves it true, so a momentary
// rpcd blip never evicts a live operator — the caller retries next tick. This is
// what keeps the sid alive for the Verso session's lifetime (ADR-007 §7).
func (a *RPCDAuthenticator) Renew(_ context.Context, sid string) bool {
	err := a.access(sid)
	var se *ubus.StatusError
	if errors.As(err, &se) && se.Code == ubus.StatusNotFound {
		return false
	}
	return true
}

// Destroy tears down the rpcd session behind sid, called when the Verso session
// that owned it ends — idle expiry, absolute cap, or logout (ADR-007 §7). Best
// effort: an undestroyed session would expire on rpcd's own clock regardless.
func (a *RPCDAuthenticator) Destroy(_ context.Context, sid string) {
	a.destroy(sid)
}

// access runs rpcd's session.access for sid — through the test seam when set,
// else the real ubus socket. The scope/object/function triple is immaterial: the
// call exists to make rpcd touch (renew) the session and to report whether the
// session is still there, not to read the ACL result.
func (a *RPCDAuthenticator) access(sid string) error {
	if a.accessFn != nil {
		return a.accessFn(sid)
	}
	c, err := ubus.Dial(a.socket)
	if err != nil {
		return err
	}
	defer c.Close()
	id, err := c.Lookup("session")
	if err != nil {
		return err
	}
	_, err = c.InvokeArgs(id, "access", map[string]string{
		"ubus_rpc_session": sid,
		"scope":            "ubus",
		"object":           "session",
		"function":         "access",
	})
	return err
}

// call runs rpcd's session.login, returning the sid ("" when rejected) — through
// the test seam when set, else the real ubus socket.
func (a *RPCDAuthenticator) call(username, password string) (string, error) {
	if a.loginFn != nil {
		return a.loginFn(username, password)
	}
	c, err := ubus.Dial(a.socket)
	if err != nil {
		return "", err
	}
	defer c.Close()
	id, err := c.Lookup("session")
	if err != nil {
		return "", err
	}
	res, err := c.InvokeArgs(id, "login", map[string]string{"username": username, "password": password})
	if err != nil {
		return "", err
	}
	sid, _ := res["ubus_rpc_session"].(string)
	return sid, nil
}

// destroy tears down a session id — best effort (an orphaned probe session would
// otherwise linger until its timeout).
func (a *RPCDAuthenticator) destroy(sid string) {
	if a.destroyFn != nil {
		a.destroyFn(sid)
		return
	}
	c, err := ubus.Dial(a.socket)
	if err != nil {
		return
	}
	defer c.Close()
	id, err := c.Lookup("session")
	if err != nil {
		return
	}
	_, _ = c.InvokeArgs(id, "destroy", map[string]string{"ubus_rpc_session": sid})
}

// decoyPassword returns a random wrong password to probe whether an account
// accepts anything. It is NOT derived from the real password: a suffix approach
// once used a NUL separator, which rpcd/crypt truncated at — making the decoy
// collapse back onto the real password and falsely flag every real login as
// permissive. A fresh random string can't collide with a real credential and
// carries no NUL to be truncated at.
func decoyPassword() string {
	var b [24]byte
	_, _ = rand.Read(b[:])
	return "verso-login-probe-" + hex.EncodeToString(b[:])
}

// Root's password status is answered by verso-rpcd (Backend.RootHasPassword):
// the shell is unprivileged and cannot read /etc/shadow, so the root helper reads
// it and returns only the boolean — no shadow-read capability on the shell.
