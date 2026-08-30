// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"net/http"

	"github.com/we-are-mono/verso/internal/plugin"
)

// systemPages is the shell-owned System frame. Ownership of the pages behind
// it is deliberately mixed: General is a bundled plugin, Access/Packages/
// Services are shell machinery, and Maintenance is filled in separately.
func systemPages(active string) []pageTab {
	items := []struct {
		key, label, href string
	}{
		{"general", "General", "/system/general"},
		{"access", "Access", "/system/access"},
		{"packages", "Packages", "/system/packages"},
		{"services", "Services", "/system/services"},
		{"maintenance", "Maintenance", "/system/maintenance"},
	}
	tabs := make([]pageTab, 0, len(items))
	for _, item := range items {
		tabs = append(tabs, pageTab{Label: item.label, Href: item.href, Active: item.key == active})
	}
	return tabs
}

func (s *Server) handleSystemRoot(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/system/general", http.StatusSeeOther)
}

// handleSystemGeneral gives the bundled plugin a stable core URL. The plugin
// still travels through the ordinary schema gateway and privileged broker; the
// only special treatment is that the shell supplies the System page frame.
func (s *Server) handleSystemGeneral(w http.ResponseWriter, r *http.Request) {
	m, ok := s.manifestByID("system")
	if !ok {
		m = plugin.Manifest{
			ID: "system", Name: "System", Socket: "/var/run/verso/system.sock",
			SchemaVersion: supportedSchemaVersion,
		}
	}
	hdr := pageHeader{Heading: "System"}
	width := "narrow"
	var ignored []pageTab
	body, status := s.pluginBodyAt(r, m, "", &hdr, &width, &ignored)
	hdr.Heading = "System"
	s.renderPage(w, r, status, hdr, width, systemPages("general"), !hdr.Immediate, body)
}
