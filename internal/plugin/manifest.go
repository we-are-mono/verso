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
	ManifestVersion int        `json:"manifest_version"`
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Socket          string     `json:"socket"`
	SchemaVersion   int        `json:"schema_version"`
	Nav             []NavEntry `json:"nav"`
}

// NavEntry places one of a plugin's pages in the shell's navigation. A plugin
// may declare several, each auto-grouped under its section. Path is relative to
// the plugin's /plugins/<id>/ mount ("" is treated as "/").
type NavEntry struct {
	Section string `json:"section"`
	Label   string `json:"label"`
	Path    string `json:"path"`
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
	case len(m.Nav) == 0:
		return fmt.Errorf("plugin %q has no nav entries", m.ID)
	}
	for i, n := range m.Nav {
		if n.Section == "" {
			return fmt.Errorf("plugin %q nav[%d] missing section", m.ID, i)
		}
		if n.Label == "" {
			return fmt.Errorf("plugin %q nav[%d] missing label", m.ID, i)
		}
	}
	return nil
}
