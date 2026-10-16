// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"net"
	"net/http"
	"slices"
	"strings"

	"github.com/we-are-mono/verso/internal/openwrt"
)

// disconnectWarning is a pending change to the interface this browser
// reaches the router through: applying it may cut the browser off, and the
// router then puts everything back when the page cannot check in (ADR-010).
// From is the address the browser uses; To is the address the change sets,
// empty where it sets none (a protocol, the interface removed).
type disconnectWarning struct {
	Interface string
	From, To  string
}

// disconnectOf is the pending change in view that touches the interface
// holding local, the address the browser connected to: any option of that
// interface's network section set or removed, or the section removed. Only
// the interface is weighed, not the firewall or the listeners on it.
func disconnectOf(view stagedView, local string, ifaces []openwrt.NetIface) *disconnectWarning {
	if local == "" {
		return nil
	}
	name := ""
	for _, iface := range ifaces {
		if slices.Contains(iface.Addresses, local) {
			name = iface.Name
			break
		}
	}
	if name == "" {
		return nil
	}
	var warning *disconnectWarning
	for _, g := range view.Groups {
		for _, item := range g.Items {
			for _, c := range item.changes {
				d := describeChange(c)
				if d.Config != "network" || d.Section != name {
					continue
				}
				if warning == nil {
					warning = &disconnectWarning{Interface: name, From: local}
				}
				if d.Op == "set" && d.Option == "ipaddr" {
					address, _, _ := strings.Cut(d.Value, "/")
					warning.To = address
				}
			}
		}
	}
	return warning
}

// localAddress is the address the browser connected to, empty where the
// server cannot tell.
func localAddress(r *http.Request) string {
	addr, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr)
	if !ok {
		return ""
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return ""
	}
	host, _, _ = strings.Cut(host, "%")
	return host
}

// disconnectRisk weighs the stage against the interface the request came in
// on. A read of the interfaces that fails flags nothing: the warning is a
// courtesy, never a gate (warn, don't block).
func (s *Server) disconnectRisk(ctx context.Context, r *http.Request, sid string, view stagedView) *disconnectWarning {
	local := localAddress(r)
	if local == "" || view.Count == 0 {
		return nil
	}
	ifaces, err := s.backend.NetworkInterfaces(ctx, sid)
	if err != nil {
		return nil
	}
	return disconnectOf(view, local, ifaces)
}
