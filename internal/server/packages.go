// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/widget"
)

// The package surface (ADR-011): shell-owned, because installing a package
// mutates the set of things the shell trusts. Two faces behind the top bar:
// Installed — the inventory of what is on disk — and Discover — the
// configured feeds, searched server-side. A package is a group of files; what
// those files RUN lives on the Services page, which answers the other
// question. Package operations ride the helper's apk verbs; nothing here is a
// uci write, so none of it stages (ADR-010 boundary).

// packagesTabs is the domain's top bar: two kinds of visit, Installed (what
// is here) and Discover (browse the feeds).
func packagesTabs(active string) []pageTab {
	return []pageTab{
		{Label: "Installed", Href: "/system/packages", Active: active == "installed"},
		{Label: "Discover", Href: "/system/packages/discover", Active: active == "discover"},
	}
}

// handlePackagesPage renders the Installed inventory.
func (s *Server) handlePackagesPage(w http.ResponseWriter, r *http.Request) {
	s.renderPackages(w, r, "")
}

// handlePackagesAction is the inventory's one act: removing a package (its
// drawer's Remove). Success flashes and redirects (PRG); the manifest set is
// rescanned in case a plugin package left (ADR-011 §7).
func (s *Server) handlePackagesAction(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	name := r.PostForm.Get("package")
	if name == "" {
		http.Error(w, "no package in form", http.StatusBadRequest)
		return
	}
	if !pkgNameOK(name) {
		http.Error(w, "bad package name", http.StatusBadRequest)
		return
	}
	if err := s.backend.PkgRemove(r.Context(), s.sessionSID(r), name); err != nil {
		s.renderPackages(w, r, fmt.Sprintf("The device refused: %v.", err))
		return
	}
	s.rescanManifests()
	s.flash(r, "success", name+" removed.")
	http.Redirect(w, r, "/system/packages", http.StatusSeeOther)
}

// renderPackages composes the inventory: the full installed set, the lens
// keeping 100+ rows one page. errMsg, when set, leads as a danger callout.
func (s *Server) renderPackages(w http.ResponseWriter, r *http.Request, errMsg string) {
	pkgs, pkgErr := s.backend.PkgInstalled(r.Context(), s.sessionSID(r))

	children := []widget.Widget{}
	if errMsg != "" {
		children = append(children, &widget.Callout{Variant: "danger", Title: "Action failed", Body: errMsg})
	}
	if pkgErr != nil {
		children = append(children, &widget.Callout{Variant: "warning", Title: "Package list unavailable",
			Body: fmt.Sprintf("The package database could not be read (%v).", pkgErr)})
	}
	children = append(children,
		&widget.Filter{Placeholder: "Filter — package, feed, version…"},
		packagesTable(pkgs),
	)

	var body strings.Builder
	if err := s.widgets.RenderWithToken(&body, &widget.Stack{Children: children}, s.sessionCSRF(r)); err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	// Everything here is immediate (ADR-011 §8), so the staged-changes bar
	// appears only when other pages' edits are pending.
	s.renderPage(w, r, http.StatusOK, pageHeader{
		Heading:    "Packages",
		Subheading: "The software installed on this router — every package, from every feed.",
	}, "wide", packagesTabs("installed"), false, template.HTML(body.String()))
}

// packagesTable is the inventory roster: name, version, feed — files on disk,
// no live state (that is the Services page's question). The drawer tells each
// package's story and offers the Remove.
func packagesTable(pkgs []openwrt.Package) widget.Widget {
	cols := []widget.TableColumn{
		{Label: "Package", Kind: "name"},
		{Label: "Version", Kind: "mono"},
		{Label: "Feed", Kind: "keyword"},
	}
	rows := make([]widget.TableRow, 0, len(pkgs))
	for _, p := range pkgs {
		rows = append(rows, packageRow(p))
	}
	return &widget.Table{Columns: cols, Rows: rows}
}

// packageRow keeps the row terse; the drawer is where the package tells its
// story — description, license, size, homepage — and offers the Remove.
func packageRow(p openwrt.Package) widget.TableRow {
	desc := p.Description
	if desc == "" {
		desc = "No description in the package."
	}
	// The website rides inline at the description's end — a block link would
	// crowd the Remove below.
	if p.Webpage != "" {
		desc += " [Project website](" + p.Webpage + ")"
	}
	props := []widget.Property{
		{Label: "Version", Value: p.Version, Mono: true},
		{Label: "Feed", Value: p.Feed, Mono: true},
	}
	if p.License != "" {
		props = append(props, widget.Property{Label: "License", Value: p.License, Mono: true})
	}
	if p.Size > 0 {
		props = append(props, widget.Property{Label: "Size", Value: humanSize(p.Size)})
	}
	return widget.TableRow{ID: p.Name, Cells: []widget.TableCell{
		{Text: p.Name}, {Text: p.Version}, {Text: p.Feed},
	}, Drawer: &widget.RowDrawer{Title: p.Name, Children: []widget.Widget{
		&widget.Text{Markdown: desc},
		&widget.Properties{Items: props},
		&widget.Form{Submit: "Remove", Fields: []widget.Widget{
			&widget.Field{Kind: "hidden", Name: "package", Value: p.Name},
			&widget.Field{Kind: "hidden", Name: "_primary", Value: "remove"},
		}},
	}}}
}

// humanSize renders bytes at package scale.
func humanSize(b int64) string {
	switch {
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MiB", float64(b)/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.0f KiB", float64(b)/(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

// discoverDefaultQuery is what Discover searches before the person types:
// the plugin naming convention (ADR-011 §3), so real plugins surface first
// and an empty feed says so honestly.
const discoverDefaultQuery = "verso-plugin"

// handleDiscoverPage renders the Discover face for ?q= (or the default query).
func (s *Server) handleDiscoverPage(w http.ResponseWriter, r *http.Request) {
	s.renderDiscover(w, r, "")
}

// handleDiscoverAction dispatches Discover's POSTs: refresh the feed index,
// run a search (PRG to ?q=), or install/remove one package — after which the
// manifest set is rescanned so a plugin package appears without a restart
// (ADR-011 §7).
func (s *Server) handleDiscoverAction(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	sid := s.sessionSID(r)
	q := strings.TrimSpace(r.PostForm.Get("q"))
	back := "/system/packages/discover"
	if q != "" {
		back += "?q=" + url.QueryEscape(q)
	}
	// A secondary button's _action (Refresh) outranks the form's primary.
	verb := r.PostForm.Get("_action")
	if verb == "" {
		verb = r.PostForm.Get("_primary")
	}
	switch verb {
	case "search":
		http.Redirect(w, r, back, http.StatusSeeOther)
	case "refresh":
		if err := s.backend.PkgUpdate(r.Context(), sid); err != nil {
			s.renderDiscover(w, r, fmt.Sprintf("Could not refresh the feeds: %v.", err))
			return
		}
		s.flash(r, "success", "Feeds refreshed.")
		http.Redirect(w, r, back, http.StatusSeeOther)
	case "install", "remove":
		name := r.PostForm.Get("package")
		if !pkgNameOK(name) {
			http.Error(w, "bad package name", http.StatusBadRequest)
			return
		}
		var err error
		if verb == "install" {
			err = s.backend.PkgInstall(r.Context(), sid, name)
		} else {
			err = s.backend.PkgRemove(r.Context(), sid, name)
		}
		if err != nil {
			s.renderDiscover(w, r, fmt.Sprintf("The device refused: %v.", err))
			return
		}
		// A plugin package just landed (or left): re-read the manifests so
		// its pages and nav rows exist without a shell restart.
		s.rescanManifests()
		if verb == "install" {
			s.flash(r, "success", name+" installed.")
		} else {
			s.flash(r, "success", name+" removed.")
		}
		http.Redirect(w, r, back, http.StatusSeeOther)
	default:
		http.Error(w, "no discover action in form", http.StatusBadRequest)
	}
}

// pkgNameRe mirrors the helper's package-name alphabet — refused here first
// so a bad name never even reaches the bus.
var pkgNameRe = regexp.MustCompile(`^[a-z0-9][a-zA-Z0-9._+-]{0,63}$`)

func pkgNameOK(name string) bool { return pkgNameRe.MatchString(name) }

// renderDiscover composes the Discover face: the freshness line with its
// Refresh, the search, and the results with install/remove drawers.
func (s *Server) renderDiscover(w http.ResponseWriter, r *http.Request, errMsg string) {
	sid := s.sessionSID(r)
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		q = r.PostForm.Get("q")
	}
	shown := q
	if shown == "" {
		shown = discoverDefaultQuery
	}

	children := []widget.Widget{}
	if errMsg != "" {
		children = append(children, &widget.Callout{Variant: "danger", Title: "Action failed", Body: errMsg})
	}

	// One toolbar: the search row carries Refresh as its secondary action (the
	// same form, so the query survives a refresh), and the index's age sits at
	// the row's right end as a quiet fact.
	checkedAt, statusErr := s.backend.PkgStatus(r.Context(), sid)
	children = append(children,
		&widget.Form{Style: "inline", Icon: "search", Submit: "Search",
			Note:    freshnessLine(checkedAt, statusErr),
			Actions: []widget.FormAction{{Label: "Refresh feeds", Action: "refresh", Icon: "refresh-cw"}},
			Fields: []widget.Widget{
				&widget.Field{Name: "q", Value: q},
				&widget.Field{Kind: "hidden", Name: "_primary", Value: "search"},
			}},
		&widget.Divider{Tight: true},
	)

	pkgs, total, err := s.backend.PkgSearch(r.Context(), sid, shown)
	switch {
	case err != nil:
		children = append(children, &widget.Callout{Variant: "warning", Title: "Search unavailable",
			Body: fmt.Sprintf("The package index could not be read (%v). Refresh the feeds and try again.", err)})
	case len(pkgs) == 0 && q == "":
		children = append(children, &widget.Empty{Icon: "search", Title: "No Verso plugins in your feeds yet",
			Body: "Search above for any package — plugins appear here as feeds publish them."})
	case len(pkgs) == 0:
		children = append(children, &widget.Empty{Icon: "search", Title: "Nothing matches “" + q + "”",
			Body: "Feeds are searched by package name."})
	default:
		if total > len(pkgs) {
			children = append(children, &widget.Badge{Variant: "info", Icon: "info", Size: "lg", Text: fmt.Sprintf(
				"Showing the first %d of %d matches — narrow the search to see the rest", len(pkgs), total)})
		}
		children = append(children, discoverTable(pkgs, q))
	}

	var body strings.Builder
	if err := s.widgets.RenderWithToken(&body, &widget.Stack{Children: children}, s.sessionCSRF(r)); err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	s.renderPage(w, r, http.StatusOK, pageHeader{
		Heading:    "Packages",
		Subheading: "Browse your configured feeds — your own and the official ones together.",
	}, "wide", packagesTabs("discover"), false, template.HTML(body.String()))
}

// freshnessLine is the honest age of the package index, beside the Refresh
// button — the time carries the emphasis.
func freshnessLine(checkedAt int64, err error) string {
	switch {
	case err != nil:
		return "Feed freshness unknown."
	case checkedAt == 0:
		return "Feeds have **never** been checked on this device."
	default:
		return "Feeds checked **" + humanAgo(time.Since(time.Unix(checkedAt, 0))) + "**."
	}
}

func humanAgo(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%d min ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%d h ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%d days ago", int(d.Hours()/24))
	}
}

// discoverTable lists the matches: name, what it does, version, feed origin,
// installed state — the drawer carries the description and the act.
func discoverTable(pkgs []openwrt.Package, q string) widget.Widget {
	cols := []widget.TableColumn{
		{Label: "Package", Kind: "name"},
		{Label: "Does", Kind: "comment"},
		{Label: "Version", Kind: "mono"},
		{Label: "Feed", Kind: "keyword"},
		{Label: "State", Kind: "pill"},
	}
	rows := make([]widget.TableRow, 0, len(pkgs))
	for _, p := range pkgs {
		state := widget.TableCell{}
		if p.Installed {
			state = widget.TableCell{Text: "installed", Variant: "success"}
		}
		rows = append(rows, widget.TableRow{ID: p.Name, Cells: []widget.TableCell{
			{Text: p.Name},
			{Text: p.Description},
			{Text: p.Version},
			{Text: p.Feed},
			state,
		}, Drawer: discoverDrawer(p, q)})
	}
	return &widget.Table{Columns: cols, Rows: rows}
}

// discoverDrawer is the act: what it is, where it is from, then Install — or
// Remove for what is already here. The active query rides along so acting on
// a result lands back on the same search — a shopping flow installs several.
func discoverDrawer(p openwrt.Package, q string) *widget.RowDrawer {
	verb, label := "install", "Install"
	if p.Installed {
		verb, label = "remove", "Remove"
	}
	desc := p.Description
	if desc == "" {
		desc = "No description in the feed."
	}
	return &widget.RowDrawer{Title: label + " — " + p.Name, Children: []widget.Widget{
		&widget.Text{Markdown: desc},
		&widget.Properties{Items: []widget.Property{
			{Label: "Package", Value: p.Name, Mono: true},
			{Label: "Version", Value: p.Version, Mono: true},
			{Label: "Feed", Value: p.Feed, Mono: true},
		}},
		&widget.Form{Submit: label, Fields: []widget.Widget{
			&widget.Field{Kind: "hidden", Name: "package", Value: p.Name},
			&widget.Field{Kind: "hidden", Name: "_primary", Value: verb},
			&widget.Field{Kind: "hidden", Name: "q", Value: q},
		}},
	}}
}
