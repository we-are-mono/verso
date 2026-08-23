// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import "strings"

// navSection is one collapsible group in the sidebar; navLink is one entry. The
// nav is built from discovered plugin manifests plus the built-in Status group,
// so a plugin appears in the chrome without any shell change (ADR-006 §2).
type navSection struct {
	Title string
	Links []navLink
	Open  bool
}

type navLink struct {
	Label  string
	Href   string
	Active bool
}

// buildNav assembles the sidebar for the current path. The built-in Status group
// is always first; plugins are grouped under their manifest's nav.section in
// discovery order (already id-sorted), so the sidebar is deterministic. The
// section containing the active link is expanded.
func (s *Server) buildNav(active string) []navSection {
	sections := []navSection{{
		Title: "Status",
		Open:  true,
		Links: []navLink{{Label: "Overview", Href: "/"}},
	}}
	index := map[string]int{"Status": 0}

	for _, m := range s.manifests {
		for _, entry := range m.Nav {
			i, ok := index[entry.Section]
			if !ok {
				i = len(sections)
				index[entry.Section] = i
				sections = append(sections, navSection{Title: entry.Section})
			}
			sections[i].Links = append(sections[i].Links, navLink{
				Label: entry.Label,
				Href:  pluginHref(m.ID, entry.Path),
			})
		}
	}

	for si := range sections {
		for li := range sections[si].Links {
			if isActive(active, sections[si].Links[li].Href) {
				sections[si].Links[li].Active = true
				sections[si].Open = true
			}
		}
	}
	return sections
}

// pluginHref is the shell-side URL for a plugin page: the /plugins/<id>/ mount
// joined with the manifest's (mount-relative) nav path.
func pluginHref(id, navPath string) string {
	return "/plugins/" + id + "/" + strings.TrimPrefix(navPath, "/")
}

func isActive(current, href string) bool {
	if href == "/" {
		return current == "/"
	}
	return current == href || strings.HasPrefix(current, href)
}
