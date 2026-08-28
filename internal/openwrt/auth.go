// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package openwrt

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"strings"

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
	// loginFn / destroyFn seam the rpcd calls for tests; nil uses the real ubus.
	loginFn   func(username, password string) (sid string, err error)
	destroyFn func(sid string)
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
		if probe, err := a.call(username, decoyPassword(password)); err == nil && probe != "" {
			a.destroy(sid)
			a.destroy(probe)
			return "", ErrInvalidCredentials
		}
	}
	return sid, nil
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

// decoyPassword derives a password that is guaranteed to differ from the given
// one (and thus be wrong on any account that actually checks it) by appending a
// random token.
func decoyPassword(password string) string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return password + "\x00decoy-" + hex.EncodeToString(b[:])
}

// ShadowSecurity answers security questions from the shadow file. The shell is
// granted CAP_DAC_READ_SEARCH to read /etc/shadow (a read-only question); it does
// not write it — setting the password is a privileged rpcd call (Backend.
// SetPassword), since the shell holds no ambient write privilege (ADR-007).
type ShadowSecurity struct {
	path string // "" = /etc/shadow
}

// NewShadowSecurity reads the default /etc/shadow.
func NewShadowSecurity() *ShadowSecurity { return &ShadowSecurity{} }

// RootHasPassword reports whether root has a password set. It fails safe (assumes
// yes) on a read error or a missing root line, so a transient failure never nags
// the operator with a false "no password" warning.
func (s *ShadowSecurity) RootHasPassword() bool {
	path := s.path
	if path == "" {
		path = "/etc/shadow"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return true
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "root:") {
			fields := strings.SplitN(line, ":", 3)
			return len(fields) >= 2 && fields[1] != ""
		}
	}
	return true
}
