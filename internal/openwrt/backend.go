// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// Package openwrt reads live state from the OpenWrt backend (ubus/uci).
package openwrt

import (
	"context"
	"fmt"

	uci "github.com/digineo/go-uci"

	"github.com/we-are-mono/verso/internal/ubus"
)

// Backend reads live state from the OpenWrt system. It is the injected seam
// (ADR-003): callers depend on this interface, and tests supply a fake.
type Backend interface {
	SystemInfo(ctx context.Context) (SystemInfo, error)
	Hostname(ctx context.Context) (string, error)
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

// ubusSystemInfo fetches the raw `system info` result table over ubus. It is
// the side-effecting seam for SystemInfo: production dials the socket, tests
// return a canned table.
type ubusSystemInfo func(ctx context.Context) (map[string]any, error)

// NativeBackend reads OpenWrt state natively, with no subprocess: uci config
// straight from /etc/config via go-uci, and system info over the ubus socket
// via the pure-Go ubus client.
//
// SECURITY: verso runs as root and reaches ubus/uci with no ACLs (kickoff
// scope). Deliberate for the spike; a real deployment needs privilege gating.
type NativeBackend struct {
	uciDir     string
	systemInfo ubusSystemInfo
}

// NewNativeBackend returns a backend reading /etc/config and the default ubus
// socket.
func NewNativeBackend() *NativeBackend {
	return &NativeBackend{
		uciDir:     "/etc/config",
		systemInfo: dialSystemInfo(""),
	}
}

// dialSystemInfo returns a ubusSystemInfo that dials the socket per call, looks
// up the `system` object and invokes `info`.
func dialSystemInfo(socket string) ubusSystemInfo {
	return func(_ context.Context) (map[string]any, error) {
		c, err := ubus.Dial(socket)
		if err != nil {
			return nil, err
		}
		defer c.Close()

		id, err := c.Lookup("system")
		if err != nil {
			return nil, err
		}
		return c.Invoke(id, "info")
	}
}

// Hostname reads the configured hostname from the uci system config. It opens a
// fresh tree per call so it reflects external writes — e.g. the hostname plugin
// committing a change — rather than a value cached at startup.
func (b *NativeBackend) Hostname(_ context.Context) (string, error) {
	vals, ok := uci.NewTree(b.uciDir).Get("system", "@system[0]", "hostname")
	if !ok || len(vals) == 0 {
		return "", fmt.Errorf("openwrt: hostname not set in uci system config")
	}
	return vals[0], nil
}

// SystemInfo fetches and maps live system info from ubus.
func (b *NativeBackend) SystemInfo(ctx context.Context) (SystemInfo, error) {
	m, err := b.systemInfo(ctx)
	if err != nil {
		return SystemInfo{}, err
	}
	return parseSystemInfo(m), nil
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
