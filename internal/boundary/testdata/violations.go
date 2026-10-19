// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// Each line marked "violation" is one the guard must report, and nothing
// else here is.
package fixture

import (
	"slices"

	"github.com/we-are-mono/verso/internal/plugin"
)

var firewallZones = "/plugins/firewall/zones" // violation

func isVPN(m plugin.Manifest) bool {
	return m.ID == "vpn" // violation
}

type row struct {
	PluginID string
	Label    string
}

func glyph(r row) string {
	switch r.PluginID {
	case "vpn": // violation
		return "lock"
	}
	return ""
}

type kind struct{ Plugin string }

func isACME(k kind) bool {
	return k.Plugin != "acme" // violation
}

func pluginHref(pluginID, path string) string { return "/plugins/" + pluginID + path }

var zones = pluginHref("firewall", "/zones") // violation

func isVPNPage(e plugin.NavEntry) bool {
	return e.Label == "VPN" // violation
}

var places = map[string]int{"VPN": 40}

func place(e plugin.NavEntry) int {
	return places[e.Label] // violation
}

var designed = []string{"Interfaces", "DHCP"}

func designedAt(e plugin.NavEntry) int {
	return slices.Index(designed, e.Label) // violation
}

func shown(pluginID string) bool {
	return pluginID == "wireless" // violation
}
