// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// Package plugin discovers out-of-process Verso plugins and carries widget
// schema between the shell and a plugin's unix socket. It is the mechanical half
// of the plugin contract (ADR-006); ADR-005 governs what a plugin may emit.
package plugin

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Manifest is a plugin's static self-description, discovered by the shell. It is
// the shell's only build-free knowledge of a plugin (ADR-006).
type Manifest struct {
	// SystemAccess contributes service settings to the shell-owned Access page.
	SystemAccess    string     `json:"system_access,omitempty"`
	ManifestVersion int        `json:"manifest_version"`
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Socket          string     `json:"socket"`
	SchemaVersion   int        `json:"schema_version"`
	Nav             []NavEntry `json:"nav"`
	// EntityTabs and EntityActs are what this plugin has to say about a subject
	// some other page lists — see EntityTab. Both are optional: most plugins own
	// pages and contribute to nobody's panel.
	EntityTabs []EntityTab `json:"entity_tabs,omitempty"`
	EntityActs []EntityAct `json:"entity_acts,omitempty"`
	ACL        ACL         `json:"acl"`
}

// ACL is a plugin's declared rpcd access surface — the Verso analog of LuCI's
// per-plugin acl.d (ADR-007). The two directions are gated differently, because
// rpcd already gates them differently:
//
//   - Write scopes are probed against session.access before the shell dispatches a
//     state-changing request, so an operator whose session lacks the grant is
//     refused by the shell and the plugin never sees the write. A plugin that
//     declares no write scopes cannot receive a state-changing request at all.
//   - Read scopes name what the shell pre-reads and hands the plugin: a "uci" scope
//     names a config, read as the plugin's snapshot; a "ubus" scope on object
//     "verso" names one of the privileged helper's read functions, from a closed set
//     the shell knows, read as live system state beside the snapshot. Neither is a
//     second gate: the operator's sid scopes the actual read, so an operator who may
//     not perform it simply gets nothing where the data would be. A plugin that
//     declares no read scopes receives neither channel.
type ACL struct {
	Read  []ACLScope `json:"read"`
	Write []ACLScope `json:"write"`
}

// ACLScope is one rpcd access triple, mirroring session.access{scope,object,
// function} one-to-one: may the session call object.function within scope? For a
// uci write the scope is "uci" and the object is the config name.
type ACLScope struct {
	Scope    string `json:"scope"`
	Object   string `json:"object"`
	Function string `json:"function"`
}

// NavEntry places one of a plugin's pages in the shell's navigation. A plugin
// may declare several, each auto-grouped under its section. Path is relative to
// the plugin's /plugins/<id>/ mount ("" is treated as "/").
//
// Icon names a Lucide glyph the shell owns (ADR-005): a plugin references an icon
// by name and never ships one. An entry naming none takes its section's glyph.
// Mode is the reading the entry belongs to (ADR-015): "basic", "advanced", or
// empty for both.
type NavEntry struct {
	Section string `json:"section"`
	Label   string `json:"label"`
	Path    string `json:"path"`
	Icon    string `json:"icon,omitempty"`
	Mode    string `json:"mode,omitempty"`
}

// EntityTab is a plugin's contribution to one kind of subject's panel: the
// firewall's say about a device, dnsdhcp's say about an interface. A subject
// several plugins each hold a piece of gets one panel, owned by the shell,
// carrying one tab per live contributor — plugins are separate processes and
// cannot compose each other, so only the shell can assemble that panel.
//
// The plugin declares which kind of subject it has something to say about and
// under which Slot. The shell owns everything else: the panel frame, the pinned
// facts, the order tabs appear in, the icon each slot wears in a listing's row,
// and which shortcut opens which tab. So a plugin cannot spend the design's
// vocabulary, and the row keeps one shape whatever is installed.
//
// Label is what the tab is called; the shell falls back to it when the plugin's
// answer names nothing. A slot no live plugin claims renders nothing at all —
// which is how a board with no QoS plugin simply has no limits tab and no limits
// icon, with no shell change and nothing for the reader to wonder about.
type EntityTab struct {
	Entity string `json:"entity"` // the kind of subject: "device", "interface", "zone"
	Slot   string `json:"slot"`   // which of that kind's slots this fills
	Label  string `json:"label"`
	// Summaries contributes configured subjects to the shell's roster through
	// GET /entity/<kind>/, including subjects no longer visible at runtime.
	Summaries bool `json:"summaries,omitempty"`
}

// EntityAct is a plugin's act on a subject that opens nothing — removing a
// reservation, say. It wears a slot's icon in the listing's row exactly as a tab
// does; the difference is only that following it does the thing rather than
// opening the panel.
//
// Path is a route into the plugin, with {id} standing for the subject's own
// identity (a MAC, a UCI section name). The shell substitutes it and applies the
// same URL policy every plugin link passes.
type EntityAct struct {
	Entity string `json:"entity"`
	Slot   string `json:"slot"`
	Path   string `json:"path"`
}

// validate enforces the required fields and a URL-safe id. A manifest that fails
// this is skipped at discovery and never mounted — the id in particular guards
// both the /plugins/<id>/ route and the socket namespace, so path characters in
// it are rejected outright.
func (m Manifest) validate() error {
	switch {
	case m.ID == "":
		return fmt.Errorf("manifest missing id")
	case strings.ContainsAny(m.ID, `/.\ `):
		return fmt.Errorf("id %q must be URL-safe (no / . \\ or space)", m.ID)
	case m.Name == "":
		return fmt.Errorf("plugin %q missing name", m.ID)
	case m.Socket == "":
		return fmt.Errorf("plugin %q missing socket", m.ID)
	case !filepath.IsAbs(m.Socket) || m.Socket != filepath.Clean(m.Socket):
		// The socket is dialed as-is; require a clean absolute path so a manifest
		// cannot aim the shell at a relative or traversal path (VS-09).
		return fmt.Errorf("plugin %q socket must be a clean absolute path", m.ID)
	case len(m.Nav) == 0 && len(m.EntityTabs) == 0 && len(m.EntityActs) == 0 && m.SystemAccess == "":
		return fmt.Errorf("plugin %q has no page or entity contributions", m.ID)
	}
	if m.SystemAccess != "" && (!strings.HasPrefix(m.SystemAccess, "/") || strings.HasPrefix(m.SystemAccess, "//") || strings.ContainsAny(m.SystemAccess, "?#\\") || m.SystemAccess != filepath.Clean(m.SystemAccess)) {
		return fmt.Errorf("plugin %q access path must be a clean local path", m.ID)
	}
	for i, n := range m.Nav {
		if n.Section == "" {
			return fmt.Errorf("plugin %q nav[%d] missing section", m.ID, i)
		}
		if n.Label == "" {
			return fmt.Errorf("plugin %q nav[%d] missing label", m.ID, i)
		}
	}
	for i, a := range m.ACL.Read {
		// A partial triple names no config to fetch — reject it rather than carry a
		// meaningless read scope.
		if a.Scope == "" || a.Object == "" || a.Function == "" {
			return fmt.Errorf("plugin %q acl.read[%d] needs scope, object and function", m.ID, i)
		}
	}
	for i, a := range m.ACL.Write {
		// A partial triple can't be probed against session.access, so it would
		// silently gate nothing — reject it rather than dispatch an ungated write.
		if a.Scope == "" || a.Object == "" || a.Function == "" {
			return fmt.Errorf("plugin %q acl.write[%d] needs scope, object and function", m.ID, i)
		}
	}
	return nil
}
