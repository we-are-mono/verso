// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// Package openwrt reads live state from the OpenWrt backend through rpcd's
// ACL-gated ubus objects, carrying the caller's session id (ADR-007).
package openwrt

import (
	"context"
	"errors"
	"fmt"

	"github.com/we-are-mono/verso/internal/ubus"
)

// ErrAccessDenied is returned when rpcd's ACLs refuse the session the operation.
var ErrAccessDenied = errors.New("openwrt: access denied by rpcd ACL")

// Backend reads live state from the OpenWrt system on behalf of a session. It is
// the injected seam (ADR-003): callers depend on this interface, tests supply a
// fake. Every method takes the rpcd session id (sid) so rpcd — not Verso —
// authorizes the operation (ADR-007).
type Backend interface {
	SystemInfo(ctx context.Context, sid string) (SystemInfo, error)
	Hostname(ctx context.Context, sid string) (string, error)
}

// SystemInfo is the subset of `ubus call system info` that Verso renders.
type SystemInfo struct {
	Uptime int64
	Load   [3]int64
	Memory Memory
}

// Memory holds byte counts reported by system info.
type Memory struct {
	Total     int64
	Free      int64
	Available int64
}

// The side-effecting operations sit behind func seams so the backend is
// unit-testable with fakes; the real implementations dial the ubus socket and
// go through rpcd (verified live, like the ubus client itself).
type (
	hostnameFn   func(ctx context.Context, sid string) (string, error)
	systemInfoFn func(ctx context.Context, sid string) (map[string]any, error)
)

// NativeBackend reads OpenWrt state over the ubus socket, presenting the session
// sid to rpcd's ACL-gated objects. Verso holds no ambient privilege over config:
// rpcd authorizes and executes, so a restricted operator is limited to what
// their ACLs grant (ADR-007).
type NativeBackend struct {
	hostname   hostnameFn
	systemInfo systemInfoFn
}

// NewNativeBackend returns a backend using the default ubus socket.
func NewNativeBackend() *NativeBackend {
	return &NativeBackend{
		hostname:   dialHostname(""),
		systemInfo: dialSystemInfo(""),
	}
}

// Hostname reads the configured hostname through rpcd's ACL-gated `uci` object.
func (b *NativeBackend) Hostname(ctx context.Context, sid string) (string, error) {
	return b.hostname(ctx, sid)
}

// SystemInfo fetches and maps live system info, gated by an rpcd ACL check.
func (b *NativeBackend) SystemInfo(ctx context.Context, sid string) (SystemInfo, error) {
	m, err := b.systemInfo(ctx, sid)
	if err != nil {
		return SystemInfo{}, err
	}
	return parseSystemInfo(m), nil
}

// dialHostname reads system.@system[0].hostname via rpcd's `uci get`, carrying
// the sid so rpcd applies the session's ACLs.
func dialHostname(socket string) hostnameFn {
	return func(_ context.Context, sid string) (string, error) {
		c, err := ubus.Dial(socket)
		if err != nil {
			return "", err
		}
		defer c.Close()

		id, err := c.Lookup("uci")
		if err != nil {
			return "", err
		}
		res, err := c.InvokeArgs(id, "get", map[string]string{
			"ubus_rpc_session": sid,
			"config":           "system",
			"section":          "@system[0]",
			"option":           "hostname",
		})
		if err != nil {
			return "", err
		}
		val, _ := res["value"].(string)
		if val == "" {
			return "", fmt.Errorf("openwrt: hostname not set in uci system config")
		}
		return val, nil
	}
}

// dialSystemInfo probes rpcd `session.access` for the sid, then invokes
// `system info` directly. system is procd's object (rpcd does not proxy it), so
// the ACL is enforced by the pre-check — the pattern for any non-rpcd ubus call.
func dialSystemInfo(socket string) systemInfoFn {
	return func(_ context.Context, sid string) (map[string]any, error) {
		c, err := ubus.Dial(socket)
		if err != nil {
			return nil, err
		}
		defer c.Close()

		if !access(c, sid, "system", "info") {
			return nil, ErrAccessDenied
		}
		id, err := c.Lookup("system")
		if err != nil {
			return nil, err
		}
		return c.Invoke(id, "info")
	}
}

// access asks rpcd whether the session may call object.function on the ubus
// scope. It is Verso's replacement for the ACL check the HTTP→ubus bridge does.
func access(c *ubus.Client, sid, object, function string) bool {
	id, err := c.Lookup("session")
	if err != nil {
		return false
	}
	res, err := c.InvokeArgs(id, "access", map[string]string{
		"ubus_rpc_session": sid,
		"scope":            "ubus",
		"object":           object,
		"function":         function,
	})
	if err != nil {
		return false
	}
	// ubus encodes booleans as the blobmsg INT8/BOOL type, which the client
	// decodes to int64 (1/0), not a Go bool.
	switch v := res["access"].(type) {
	case bool:
		return v
	case int64:
		return v != 0
	}
	return false
}

// parseSystemInfo maps the generic ubus result table onto SystemInfo.
func parseSystemInfo(m map[string]any) SystemInfo {
	var si SystemInfo
	si.Uptime = asInt64(m["uptime"])
	if load, ok := m["load"].([]any); ok {
		for i := 0; i < len(si.Load) && i < len(load); i++ {
			si.Load[i] = asInt64(load[i])
		}
	}
	if mem, ok := m["memory"].(map[string]any); ok {
		si.Memory.Total = asInt64(mem["total"])
		si.Memory.Free = asInt64(mem["free"])
		si.Memory.Available = asInt64(mem["available"])
	}
	return si
}

func asInt64(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case float64:
		return int64(n)
	}
	return 0
}
