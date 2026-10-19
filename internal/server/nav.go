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
func sectionLess(a, b string) bool { return orderedLess(coreSectionOrder, a, b) }

// orderedLess orders names by their place in order; names order does not hold
// follow all of them, by name.
func orderedLess(order []string, a, b string) bool {
	ra, rb := slices.Index(order, a), slices.Index(order, b)
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
	// PendingActive is the Pending changes page being the one open: the
	// stage's row, first in the rail and in no group, carries the marker.
	PendingActive bool
}

// navRow is one destination in the rail. The row you are in opens its subpages
// beneath it, so the rail carries the whole path rather than handing the last
// step to a bar somewhere else on the page.
type navRow struct {
	Label  string
	Href   string
	Icon   string
	Active bool
	// Children are the row's subpages, already localized. A Group carries them
	// on every page (Network's and System's, or a plugin's manifest list) and
	// opens in place; Open is the group holding the page you are on. A plugin
	// that says only "pages": true has them on its own page alone, from the
	// envelope, and stays a link.
	Children    []pageTab
	Group, Open bool
	// Opens says the row leads into subpages at all, known on every page, so a
	// closed row can say it opens.
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

// The shell's own places in the rail (ADR-009 §2). Overview stands first, at
// 0, before anything a plugin declares; Devices and the Network and System
// rows stand at numbers plugin authors are told, so a plugin's row can stand
// before, between or after them.
const (
	devicesPlace = 10
	networkPlace = 20
	systemPlace  = 90
)

// place is where one rail row or collapsed section page stands among its
// siblings: its declared order, nil for none, and for ties the section it is
// filed under, its English label and the plugin that filed it.
type place struct {
	order    *int
	section  string
	label    string
	pluginID string
}

// fixed is one of the shell's own places.
func fixed(order int) *int { return &order }

// placeLess orders siblings by their declared order, every placed one before
// every unplaced one; unplaced ones by the section they are filed under
// (sectionLess); then by English label and plugin id. The labels are still
// English here, so the rail reads the same in every language, and the same
// whatever order the plugins were found in.
func placeLess(a, b place) bool {
	switch {
	case (a.order != nil) != (b.order != nil):
		return a.order != nil
	case a.order != nil && *a.order != *b.order:
		return *a.order < *b.order
	case a.order == nil && a.section != b.section:
		return sectionLess(a.section, b.section)
	case a.label != b.label:
		return a.label < b.label
	}
	return a.pluginID < b.pluginID
}

// sectionIcon is the glyph a section's rows wear when their entry names none.
// The shell chooses no glyph by a plugin's label: a plugin names its own.
func sectionIcon(section string) string {
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
// destinations, Overview first and the rest where each declares its place
// (placeLess), the active one carrying the subpages it opens. Labels are
// localized at the display edge (ADR-012): shell-owned labels from base (tr),
// each plugin's from that plugin's catalog (pluginTr(id)), which is why the
// ordering happens while they are still English.
func (s *Server) buildSidebar(active string, tr func(string) string, pluginTr func(pluginID string) func(string) string, pages []pageTab) navModel {
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
		place place
		row   navRow
	}
	// A row says nothing of how things are — the homepage does — so Devices
	// is its name alone.
	places := []filed{{
		place: place{order: fixed(devicesPlace), section: "Status", label: "Devices"},
		row:   navRow{Label: "Devices", Href: devicesPath, Icon: "phone", Active: isActive(active, devicesPath)},
	}}
	var hrefs []string // every live registration, rail row or not
	for _, m := range s.manifestList() {
		if !s.probe(m.Endpoint()) {
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
				icon = sectionIcon(entry.Section)
			}
			row := navRow{Label: entry.Label, Href: href, Icon: icon, PluginID: m.ID, Opens: entry.Pages.Opens, Group: entry.Pages.List != nil}
			for _, p := range entry.Pages.List {
				row.Children = append(row.Children, pageTab{Label: p.Label, Href: pluginHref(m.ID, p.Path), PluginID: m.ID})
			}
			places = append(places, filed{place{order: entry.Order, section: entry.Section, label: entry.Label, pluginID: m.ID}, row})
		}
	}
	here := s.sectionAt(active)
	for _, c := range collapsedSections {
		pages := s.sectionPages(c.Section, active)
		if len(pages) == 0 {
			continue
		}
		row := navRow{Label: c.Section, Href: pages[0].Href, Icon: sectionIcon(c.Section), Active: here == c.Section, Opens: true, Group: true, Children: pages}
		// What System has waiting for you is what it has to install.
		if c.Section == "System" {
			if truth, ok := s.updateTruth(); ok {
				if row.Detail, row.Hint = waitingToInstall(truth, tr); row.Detail != "" {
					row.Dot, row.Variant = true, "info"
				}
			}
		}
		places = append(places, filed{place{order: fixed(c.Place), section: c.Section, label: c.Section}, row})
	}
	sort.SliceStable(places, func(a, b int) bool { return placeLess(places[a].place, places[b].place) })

	// Overview is place 0: nothing a plugin declares stands before it.
	rows := []navRow{{Label: "Overview", Href: "/", Icon: "house", Active: isActive(active, "/")}}
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

	for i := range rows {
		rows[i].Label = localizeLabel(rows[i].PluginID, rows[i].Label, tr, pluginTr)
		for j, p := range rows[i].Children {
			rows[i].Children[j].Label = localizeLabel(p.PluginID, p.Label, tr, pluginTr)
		}
	}
	// The row you are in takes the page's own subpages, already localized and
	// with the one you are on marked; a plugin without a manifest list has
	// them here alone.
	for i := range rows {
		if rows[i].Active {
			if len(pages) > 0 {
				rows[i].Children = pages
			}
			rows[i].Open = rows[i].Group
			break
		}
	}
	return navModel{Rows: rows, PendingActive: active == pendingPath}
}

// pluginHref is the shell-side URL for a plugin page: the /plugins/<id>/ mount
// joined with the manifest's (mount-relative) nav path.
func pluginHref(pluginID, navPath string) string {
	return "/plugins/" + pluginID + "/" + strings.TrimPrefix(navPath, "/")
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
