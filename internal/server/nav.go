// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"slices"
	"sort"
	"strings"

	"github.com/we-are-mono/verso/internal/plugin"
)

// coreSectionOrder is the shell-owned core taxonomy (ADR-009 §2): these sections
// render first, in exactly this order, ahead of any plugin-introduced section.
// Status is served by the shell itself; Network, Security and System are backed
// by bundled first-party plugins. Membership here is a nav-ordering guarantee, not
// a mechanism — every configuring section is still an ADR-006 plugin.
var coreSectionOrder = []string{"Status", "Network", "Security", "System"}

// sectionLess orders the sections destinations are filed under: core sections
// first in canonical order, plugin-introduced sections after all of them, by
// title (ADR-009 §5) — deterministic regardless of plugin discovery order.
func sectionLess(a, b string) bool {
	ra, rb := slices.Index(coreSectionOrder, a), slices.Index(coreSectionOrder, b)
	if (ra >= 0) != (rb >= 0) {
		return ra >= 0
	}
	if ra >= 0 {
		return ra < rb
	}
	return a < b
}

// railTransition names a rail part for the browser's page-change transition
// (the stylesheet's ::view-transition rules): kind, then where the part leads,
// folded to an ident. The name comes from the destination, never the position,
// so a place carries one name on every page and the browser can carry it from
// the page you leave to the page you open.
func railTransition(kind, href string) string {
	var b strings.Builder
	b.WriteString("verso-nav-" + kind)
	gap := true
	for _, r := range strings.ToLower(href) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if gap {
				b.WriteByte('-')
			}
			b.WriteRune(r)
			gap = false
			continue
		}
		gap = true
	}
	if b.Len() == len("verso-nav-"+kind) {
		b.WriteString("-root")
	}
	return b.String()
}

// Transition is the row's page-change name: the row stays the same place
// whichever page draws it.
func (r navRow) Transition() string { return railTransition("row", r.Href) }

// BranchTransition names the subpages' branch the row opens. It is the row's
// own, so leaving one branch for another's closes the one and opens the other
// rather than carrying the first across.
func (r navRow) BranchTransition() string { return railTransition("branch", r.Href) }

// ChevronTransition names the row's chevron on its own, so a page change turns
// it from pointing the way the row opens to pointing down over its branch.
func (r navRow) ChevronTransition() string { return railTransition("chevron", r.Href) }

// Transition is a subpage's page-change name, kept apart from the rows' since
// a row may lead where its first subpage does.
func (p pageTab) Transition() string { return railTransition("sub", p.Href) }

// navModel is the rail: one flat list of destinations, with subpages under
// the active row.
type navModel struct {
	Rows []navRow
}

// navRow is one destination in the rail. The row you are in opens its subpages
// beneath it, so the rail carries the whole path rather than handing the last
// step to a bar somewhere else on the page.
type navRow struct {
	Label  string
	Href   string
	Icon   string
	Active bool
	// Children are the active row's subpages, already localized. Only the active
	// row has them: a plugin declares its pages in the envelope it answers with,
	// so the shell knows them for the page in hand and for no other.
	Children []pageTab
	// Opens says the row leads into subpages at all, known on every page, so a
	// closed row can say it opens: System's are the shell's own, and a plugin's
	// destination declares it in its manifest.
	Opens bool
	// Detail rides hard right: what waits for you there, in a word or two
	// ("15 updates"), never a reading of how things are. Dot leads it with the
	// rail's square, and Variant tints both (the tone vocabulary; update news
	// is "info"). Without a Detail, Dot marks a state beside the label. Hint is
	// the whole sentence behind the words, on the row as its title.
	Detail  string
	Dot     bool
	Variant string
	Hint    string
	// PluginID names the plugin that authored this row's label ("" for a
	// shell-owned row), so the label is localized from that plugin's catalog
	// rather than the shell's base (ADR-012 §5).
	PluginID string
}

// railOrder is the rail's designed order. Matching on the
// English label is what lets a plugin land in its designed place without the
// shell knowing anything about that plugin: the labels are still English at
// this point, localized only at the display edge. A destination it does not
// name follows these, in the order of the section it is filed under
// (sectionLess), so a new plugin appears in the rail with no shell change.
var railOrder = []string{
	"Overview", "Devices", "Traffic", "Journal", "Network", "Wireless", "VPN",
	"Firewall", "Routing", "Storage", "System",
}

// navIcon chooses a shell-owned glyph before labels are localized. Designed
// destinations have their own identity; other entries inherit their section's
// glyph unless the plugin names one in its manifest.
func navIcon(label, section string) string {
	switch label {
	case "Overview":
		return "house"
	case "Devices":
		return "phone"
	case "Traffic":
		return "activity"
	case "Journal":
		return "menu"
	case "Wireless":
		return "wifi"
	case "Firewall":
		return "zone"
	case "Routing":
		return "route"
	case "VPN":
		return "lock"
	case "Storage":
		return "hard-drive"
	case "System":
		return "settings"
	}
	switch section {
	case "Status":
		return "activity"
	case "Network":
		return "network"
	case "Security":
		return "zone"
	case "System":
		return "settings"
	default:
		return "menu"
	}
}

// buildSidebar assembles the rail for the current path: one flat list of
// destinations in the designed order, the active one carrying the subpages it
// opens. Labels are localized at the display edge (ADR-012): shell-owned labels
// from base (tr), each plugin's from that plugin's catalog (pluginTr(id)), which
// is why the ordering above happens while they are still English.
func (s *Server) buildSidebar(active string, tr func(string) string, pluginTr func(id string) func(string) string, pages []pageTab) navModel {
	// Every section the manifests register becomes rows — no titles between
	// them, because the rail is a list of places and not a taxonomy. Status is
	// served by Overview; Network and System each collapse to one row whose
	// subpages are the pages filed there (sectionPages).
	//
	// Only plugins whose socket answers contribute rows: a row that leads to
	// "unavailable" is a dead door, and an installed-but-off plugin is the
	// management page's business (ADR-011). Direct URLs still answer — with the
	// notice and its way back on — so nothing is unreachable, just unlisted.
	type filed struct {
		section string
		row     navRow
	}
	var places []filed
	var hrefs []string // every live registration, rail row or not
	for _, m := range s.manifestList() {
		if !s.probe(m.Socket) {
			continue
		}
		for _, entry := range m.Nav {
			href := pluginHref(m.ID, entry.Path)
			hrefs = append(hrefs, href)
			if _, folded := collapsed(entry.Section); folded || entry.Section == "Status" {
				continue
			}
			icon := entry.Icon
			if icon == "" {
				icon = navIcon(entry.Label, entry.Section)
			}
			places = append(places, filed{entry.Section, navRow{Label: entry.Label, Href: href, Icon: icon, PluginID: m.ID, Opens: entry.Pages}})
		}
	}
	here := s.sectionAt(active)
	for _, c := range collapsedSections {
		pages := s.sectionPages(c.Section, active)
		if len(pages) == 0 {
			continue
		}
		row := navRow{Label: c.Section, Href: pages[0].Href, Icon: navIcon(c.Section, c.Section), Active: here == c.Section, Opens: true}
		// What System has waiting for you is what it has to install.
		if c.Section == "System" {
			if truth, ok := s.updateTruth(); ok {
				if row.Detail, row.Hint = waitingToInstall(truth, tr); row.Detail != "" {
					row.Dot, row.Variant = true, "info"
				}
			}
		}
		places = append(places, filed{c.Section, row})
	}
	// Stable, so a section's rows keep discovery (id-sorted) order.
	sort.SliceStable(places, func(a, b int) bool { return sectionLess(places[a].section, places[b].section) })

	// A row says nothing of how things are — the homepage does — so Devices
	// is its name alone.
	rows := []navRow{
		{Label: "Overview", Href: "/", Icon: navIcon("Overview", "Status"), Active: isActive(active, "/")},
		{Label: "Devices", Href: devicesPath, Icon: navIcon("Devices", "Status"), Active: isActive(active, devicesPath)},
	}
	// Only the most specific registration lights its row; one filed under a
	// collapsed section or Status lights none here (the section's row is lit by
	// sectionAt).
	lit := ""
	if i := bestHref(active, hrefs); i >= 0 {
		lit = hrefs[i]
	}
	for _, p := range places {
		if lit != "" && p.row.PluginID != "" && p.row.Href == lit {
			p.row.Active, lit = true, ""
		}
		rows = append(rows, p.row)
	}

	// The designed order, with anything the canvas does not name kept in
	// section order. Stable, so discovery order still decides between two rows
	// the canvas is silent about.
	sort.SliceStable(rows, func(a, b int) bool {
		ra, rb := slices.Index(railOrder, rows[a].Label), slices.Index(railOrder, rows[b].Label)
		if (ra >= 0) != (rb >= 0) {
			return ra >= 0
		}
		return ra >= 0 && ra < rb
	})

	// The one open row carries the subpages, and it is the only row that can:
	// the shell learns a plugin's pages from the envelope it has just rendered.
	for i := range rows {
		if rows[i].Active {
			rows[i].Children = pages
			break
		}
	}
	for i := range rows {
		if rows[i].PluginID == "" {
			rows[i].Label = tr(rows[i].Label)
		} else {
			rows[i].Label = pluginTr(rows[i].PluginID)(rows[i].Label)
		}
	}
	return navModel{Rows: rows}
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

// bestHref returns the index of the most specific href the current path sits
// under, or -1. A plugin's root (…/, its index subpage) prefixes every sibling
// subpage, so on a deeper page several match; the longest is the destination,
// and of equal ones the first.
func bestHref(current string, hrefs []string) int {
	best := -1
	for i, h := range hrefs {
		if isActive(current, h) && (best < 0 || len(h) > len(hrefs[best])) {
			best = i
		}
	}
	return best
}

// navEntryAt returns the manifest's nav entry a plugin path sits under: the most
// specific registered path that holds it, whole segments only.
func navEntryAt(m plugin.Manifest, pluginPath string) (plugin.NavEntry, bool) {
	current := strings.Trim(pluginPath, "/")
	var found plugin.NavEntry
	best := -1
	for _, entry := range m.Nav {
		path := strings.Trim(entry.Path, "/")
		matches := path == "" || current == path || strings.HasPrefix(current, path+"/")
		if matches && len(path) > best {
			found, best = entry, len(path)
		}
	}
	return found, best >= 0
}

// navLabelHref resolves a particular destination without confusing it with
// another plugin in the same section. Missing destinations stay unlinked.
func (s *Server) navLabelHref(label string) string {
	for _, manifest := range s.manifestList() {
		for _, entry := range manifest.Nav {
			if entry.Label == label && s.probe(manifest.Socket) {
				return pluginHref(manifest.ID, entry.Path)
			}
		}
	}
	return ""
}
