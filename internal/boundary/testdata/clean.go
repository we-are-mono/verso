// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// Nothing here names a plugin: OpenWrt's own names, the shell's reserved id,
// empty checks, a sort by label and two labels compared with each other.
package fixture

import (
	"sort"

	"github.com/we-are-mono/verso/internal/plugin"
)

const mount = "/plugins/"

func readsFirewall(config string) bool { return config == "firewall" }

func reserved(m plugin.Manifest) bool { return m.ID == plugin.ShellID }

func unnamed(m plugin.Manifest, e plugin.NavEntry) bool { return m.ID == "" || e.Label == "" }

func byLabel(entries []plugin.NavEntry) {
	sort.Slice(entries, func(a, b int) bool { return entries[a].Label < entries[b].Label })
}

func same(a, b plugin.NavEntry) bool { return a.Label == b.Label }

func href(pluginID string) string { return mount + pluginID + "/" }

var own = href(plugin.ShellID)
