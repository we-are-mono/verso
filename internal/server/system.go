// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"net/http"
)

// systemPages assembles the mixed-ownership System frame.
// Plugin-owned pages come from live manifest registrations; the shell contributes
// only the pages it actually owns. A stopped plugin therefore withdraws its page
// without a System-specific condition or a dead, hardcoded destination.
func (s *Server) systemPages(active string) []pageTab {
	tabs := make([]pageTab, 0, 6)
	for _, m := range s.manifestList() {
		if !s.probe(m.Socket) {
			continue
		}
		for _, entry := range m.Nav {
			if entry.Section != "System" {
				continue
			}
			tabs = append(tabs, pageTab{
				Label:    entry.Label,
				Href:     pluginHref(m.ID, entry.Path),
				PluginID: m.ID,
			})
		}
	}
	for _, item := range []struct{ label, href string }{
		{"Hardware", "/system/hardware"},
		{"Access", "/system/access"},
		{"Packages", "/system/packages"},
		{"Services", "/system/services"},
		{"Maintenance", "/system/maintenance"},
		{"Logs", "/system/logs"},
	} {
		tabs = append(tabs, pageTab{Label: item.label, Href: item.href})
	}
	hrefs := make([]string, len(tabs))
	for i, tab := range tabs {
		hrefs[i] = tab.Href
	}
	if i := bestHref(active, hrefs); i >= 0 {
		tabs[i].Active = true
	}
	return tabs
}

func (s *Server) handleSystemRoot(w http.ResponseWriter, r *http.Request) {
	pages := s.systemPages("")
	// Access is always present, so this can only be empty if the shell's own
	// System frame is broken. Keep the fallback defensive and local.
	destination := "/system/access"
	if len(pages) > 0 {
		destination = pages[0].Href
	}
	http.Redirect(w, r, destination, http.StatusSeeOther)
}
