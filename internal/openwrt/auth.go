// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package openwrt

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/we-are-mono/verso/internal/ubus"
)

// ErrInvalidCredentials is returned when rpcd accepts the call but issues no
// session — i.e. the username/password were rejected.
var ErrInvalidCredentials = errors.New("openwrt: invalid credentials")

// RPCDAuthenticator authenticates against rpcd's session.login over the ubus
// socket, returning the session id. An empty password succeeds only when the
// account has none — rpcd, not Verso, enforces that (OpenWrt's fresh-boot
// behavior). The sid is held server-side; it never reaches the browser.
type RPCDAuthenticator struct {
	socket string // "" = default ubus socket
}

// NewRPCDAuthenticator returns an authenticator using the default ubus socket.
func NewRPCDAuthenticator() *RPCDAuthenticator { return &RPCDAuthenticator{} }

// Login returns the rpcd session id (sid) for valid credentials.
func (a *RPCDAuthenticator) Login(_ context.Context, username, password string) (string, error) {
	c, err := ubus.Dial(a.socket)
	if err != nil {
		return "", err
	}
	defer c.Close()

	id, err := c.Lookup("session")
	if err != nil {
		return "", err
	}
	res, err := c.InvokeArgs(id, "login", map[string]string{
		"username": username,
		"password": password,
	})
	if err != nil {
		return "", err
	}
	sid, ok := res["ubus_rpc_session"].(string)
	if !ok || sid == "" {
		return "", ErrInvalidCredentials
	}
	return sid, nil
}

// ShadowSecurity answers security questions from the shadow file. Verso runs as
// root, so it can read it.
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
