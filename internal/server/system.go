// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"net/http"
	"strings"

	"github.com/we-are-mono/verso/internal/plugin"
	"github.com/we-are-mono/verso/internal/widget"
)

// systemPages assembles the mixed-ownership System frame for one reader's mode.
// Plugin-owned pages come from live manifest registrations; the shell contributes
// only the pages it actually owns. A stopped plugin therefore withdraws its page
// without a System-specific condition or a dead, hardcoded destination.
//
// Services is the advanced reading of this domain (ADR-015): procd's live table of
// what runs is machinery, not a task. Its tab is absent in basic mode — the URL
// still answers, because mode is never authority (ADR-015 §6).
func (s *Server) systemPages(active, mode string) []pageTab {
	tabs := make([]pageTab, 0, 6)
	for _, m := range s.manifestList() {
		if !s.probe(m.Socket) {
			continue
		}
		for _, entry := range m.Nav {
			if entry.Section != "System" || !modeShows(entry.Mode, mode) {
				continue
			}
			tabs = append(tabs, pageTab{
				Label:    entry.Label,
				Href:     pluginHref(m.ID, entry.Path),
				PluginID: m.ID,
			})
		}
	}
	for _, item := range []struct{ label, href, mode string }{
		{"Hardware", "/system/hardware", ""},
		{"Access", "/system/access", ""},
		{"Packages", "/system/packages", ""},
		{"Services", "/system/services", widget.ModeAdvanced},
		{"Maintenance", "/system/maintenance", ""},
	} {
		if !modeShows(item.mode, mode) {
			continue
		}
		tabs = append(tabs, pageTab{Label: item.label, Href: item.href})
	}
	markActiveTab(tabs, active)
	return tabs
}

func (s *Server) handleSystemRoot(w http.ResponseWriter, r *http.Request) {
	pages := s.systemPages("", readerMode(r))
	// Access is always present, so this can only be empty if the shell's own
	// System frame is broken. Keep the fallback defensive and local.
	destination := "/system/access"
	if len(pages) > 0 {
		destination = pages[0].Href
	}
	http.Redirect(w, r, destination, http.StatusSeeOther)
}

// pluginNavSectionAt resolves the section owning a plugin path. The most
// specific registered path wins, matching the active-link rule in nav.go.
func pluginNavSectionAt(m plugin.Manifest, pluginPath string) string {
	current := strings.Trim(pluginPath, "/")
	section, best := "", -1
	for _, entry := range m.Nav {
		path := strings.Trim(entry.Path, "/")
		matches := path == "" || current == path || strings.HasPrefix(current, path+"/")
		if matches && len(path) > best {
			section, best = entry.Section, len(path)
		}
	}
	return section
}

func markActiveTab(tabs []pageTab, active string) {
	best, bestLen := -1, -1
	for i := range tabs {
		if isActive(active, tabs[i].Href) && len(tabs[i].Href) > bestLen {
			best, bestLen = i, len(tabs[i].Href)
		}
	}
	if best >= 0 {
		tabs[best].Active = true
	}
}
