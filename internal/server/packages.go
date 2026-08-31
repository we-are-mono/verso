// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/widget"
)

// The package surface (ADR-011): shell-owned, because installing a package
// mutates the set of things the shell trusts. Two faces behind one local mode
// switch: Installed — the inventory of what is on disk — and Available — the
// configured feeds, searched server-side. A package is a group of files; what
// those files RUN lives on the Services page, which answers the other
// question. Package operations ride the helper's apk verbs; nothing here is a
// uci write, so none of it stages (ADR-010 boundary).

// handlePackagesPage renders the Installed inventory.
func (s *Server) handlePackagesPage(w http.ResponseWriter, r *http.Request) {
	s.renderPackages(w, r, "")
}

// handlePackagesAction is the inventory's one act: removing a package (its
// drawer's Remove). Success flashes and redirects (PRG); the manifest set is
// rescanned in case a plugin package left (ADR-011 §7).
func (s *Server) handlePackagesAction(w http.ResponseWriter, r *http.Request) {
	_, t := s.localize(r)
	tr := translatorOrIdentity(t)
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
	if feedRefresh.running() {
		s.flash(r, "info", tr("The feeds are being refreshed — try again in a moment."))
		http.Redirect(w, r, "/system/packages", http.StatusSeeOther)
		return
	}
	if err := s.backend.PkgRemove(r.Context(), s.sessionSID(r), name); err != nil {
		s.renderPackages(w, r, fmt.Sprintf(tr("The device refused: %v."), err))
		return
	}
	s.rescanManifests()
	s.flash(r, "success", fmt.Sprintf(tr("%s removed."), name))
	http.Redirect(w, r, "/system/packages", http.StatusSeeOther)
}

// renderPackages composes the inventory: the full installed set, the lens
// keeping 100+ rows one page. errMsg, when set, leads as a danger callout.
func (s *Server) renderPackages(w http.ResponseWriter, r *http.Request, errMsg string) {
	children := []widget.Widget{}
	if errMsg != "" {
		children = append(children, &widget.Callout{Variant: "danger", Title: "Action failed", Body: errMsg})
	}
	if feedRefresh.running() {
		children = append(children, refreshingInstead(
			"The list returns as soon as the refresh finishes — reload the page in a moment."))
	} else {
		pkgs, pkgErr := s.backend.PkgInstalled(r.Context(), s.sessionSID(r))
		if pkgErr != nil {
			children = append(children, &widget.Callout{Variant: "warning", Title: "Package list unavailable",
				Body: fmt.Sprintf("The package database could not be read (%v).", pkgErr)})
		}
		children = append(children,
			&widget.Filter{Placeholder: "Filter — package, feed, version…"},
			packagesTable(pkgs),
		)
	}

	var body strings.Builder
	lang, t := s.localize(r)
	if err := s.widgets.RenderWithToken(&body, &widget.Stack{Children: children}, s.sessionCSRF(r), lang, t); err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	// Everything here is immediate (ADR-011 §8), so the staged-changes bar
	// appears only when other pages' edits are pending.
	s.renderPage(w, r, http.StatusOK, pageHeader{
		Heading:    "System",
		Subheading: "The software installed on this router — every package, from every feed.",
		Modes:      packageModes(r.URL.Path),
	}, "narrow", s.systemPages(r.URL.Path), false, template.HTML(body.String()))
}

func packageModes(active string) []pageTab {
	modes := []pageTab{
		{Label: "Installed", Href: "/system/packages"},
		{Label: "Available", Href: "/system/packages/discover"},
	}
	markActiveTab(modes, active)
	return modes
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
	if len(p.RequiredBy) > 0 {
		props = append(props, widget.Property{Label: "Required by", Value: strings.Join(p.RequiredBy, ", "), Mono: true})
	} else if !p.Removable {
		props = append(props, widget.Property{Label: "Removal", Value: "Protected system package"})
	}
	drawer := []widget.Widget{
		&widget.Callout{Variant: "neutral", Compact: true, Body: desc,
			Link: packageWebsite(p.Webpage)},
	}
	drawer = append(drawer,
		&widget.Properties{Style: "system", Items: props},
	)
	if p.Removable {
		drawer = append(drawer, &widget.Form{Submit: "Remove", Fields: []widget.Widget{
			&widget.Field{Kind: "hidden", Name: "package", Value: p.Name},
			&widget.Field{Kind: "hidden", Name: "_primary", Value: "remove"},
		}})
	}
	return widget.TableRow{ID: p.Name, Cells: []widget.TableCell{
		{Text: p.Name}, {Text: p.Version, Emphasis: true}, {Text: p.Feed},
	}, Drawer: &widget.RowDrawer{Title: p.Name, Children: drawer}}
}

func packageWebsite(href string) *widget.Link {
	if href == "" {
		return nil
	}
	return &widget.Link{Label: "Project website", Href: href, NewTab: true}
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

// handleDiscoverPage renders the Available face. It remains intentionally
// empty until the person submits a package name.
func (s *Server) handleDiscoverPage(w http.ResponseWriter, r *http.Request) {
	s.renderDiscover(w, r, "")
}

// handleDiscoverAction dispatches Available's POSTs: refresh the feed index,
// run a search (PRG to ?q=), or install/remove one package — after which the
// manifest set is rescanned so a plugin package appears without a restart
// (ADR-011 §7).
func (s *Server) handleDiscoverAction(w http.ResponseWriter, r *http.Request) {
	_, t := s.localize(r)
	tr := translatorOrIdentity(t)
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
		// The run outlives this response, so it carries a context of its own;
		// the helper client's deadline is what bounds the wait now.
		if !feedRefresh.start(func() error { return s.backend.PkgUpdate(context.Background(), sid) }) {
			s.flash(r, "info", tr("The feeds are already being refreshed."))
		}
		http.Redirect(w, r, back, http.StatusSeeOther)
	case "install", "remove":
		name := r.PostForm.Get("package")
		if !pkgNameOK(name) {
			http.Error(w, "bad package name", http.StatusBadRequest)
			return
		}
		if feedRefresh.running() {
			s.flash(r, "info", tr("The feeds are being refreshed — try again in a moment."))
			http.Redirect(w, r, back, http.StatusSeeOther)
			return
		}
		var err error
		if verb == "install" {
			err = s.backend.PkgInstall(r.Context(), sid, name)
		} else {
			err = s.backend.PkgRemove(r.Context(), sid, name)
		}
		if err != nil {
			s.renderDiscover(w, r, fmt.Sprintf(tr("The device refused: %v."), err))
			return
		}
		// A plugin package just landed (or left): re-read the manifests so
		// its pages and nav rows exist without a shell restart.
		s.rescanManifests()
		if verb == "install" {
			s.flash(r, "success", fmt.Sprintf(tr("%s installed."), name))
		} else {
			s.flash(r, "success", fmt.Sprintf(tr("%s removed."), name))
		}
		http.Redirect(w, r, back, http.StatusSeeOther)
	default:
		http.Error(w, "no package action in form", http.StatusBadRequest)
	}
}

// feedRefreshJob is the feed refresh, detached from the browser. Reaching every
// configured repository takes longer than a person will hold a page open for, so
// the POST starts the work and answers; each render then states what the job is
// doing. verso-rpcd's package mutex remains the serializer that keeps two apk
// runs apart — this job's own guard is what makes the page honest about a refresh
// already being under way, and what moves the helper call's wait off the request.
type feedRefreshJob struct {
	mu      sync.Mutex
	active  bool
	failure error
}

// start refreshes in the background unless a run is already under way, reporting
// whether this call owns the new one.
func (j *feedRefreshJob) start(refresh func() error) bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.active {
		return false
	}
	j.active = true
	j.failure = nil
	go func() {
		err := refresh()
		j.mu.Lock()
		j.active, j.failure = false, err
		j.mu.Unlock()
	}()
	return true
}

// running reports whether a refresh is under way — and so whether apk is holding
// verso-rpcd's package guard. Every other package verb queues behind that guard,
// so while this is true the surface asks the helper for nothing and states what
// the device is doing instead: a page that answers at once and says the listing
// is a moment away beats a page that is right in a minute.
func (j *feedRefreshJob) running() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.active
}

// takeFailure reports how the last finished run failed and forgets it, so a
// failure nobody was waiting for is still stated once, on the next visit.
func (j *feedRefreshJob) takeFailure() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	failure := j.failure
	j.failure = nil
	return failure
}

// feedRefresh is that job. The package index is one file set the whole device
// shares, so the refresh is one act device-wide, not one per operator.
var feedRefresh feedRefreshJob

// refreshingInstead stands where a listing would be while the refresh owns apk.
func refreshingInstead(body string) widget.Widget {
	return &widget.Empty{Icon: "refresh-cw", Title: "Refreshing the package feeds", Body: body}
}

// pkgNameRe mirrors the helper's package-name alphabet — refused here first
// so a bad name never even reaches the bus.
var pkgNameRe = regexp.MustCompile(`^[a-z0-9][a-zA-Z0-9._+-]{0,63}$`)

func pkgNameOK(name string) bool { return pkgNameRe.MatchString(name) }

// renderDiscover composes the Available face: the freshness line with its
// Refresh, the search, and the results with install/remove drawers.
func (s *Server) renderDiscover(w http.ResponseWriter, r *http.Request, errMsg string) {
	sid := s.sessionSID(r)
	lang, t := s.localize(r)
	tr := translatorOrIdentity(t)
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		q = r.PostForm.Get("q")
	}
	children := []widget.Widget{}
	if errMsg != "" {
		children = append(children, &widget.Callout{Variant: "danger", Title: "Action failed", Body: errMsg})
	}
	refreshing := feedRefresh.running()
	if !refreshing {
		if err := feedRefresh.takeFailure(); err != nil {
			children = append(children, &widget.Callout{Variant: "danger", Title: "Feed refresh failed",
				Body: fmt.Sprintf(tr("The package feeds could not be refreshed (%v)."), err)})
		}
	}
	// One toolbar: the search row carries Refresh as its secondary action (the
	// same form, so the query survives a refresh), and the index's age sits at
	// the row's right end as a quiet fact. A refresh under way takes the button's
	// place with what it is doing — the act cannot be started twice, so offering
	// it again would be an offer the device would decline.
	toolbar := &widget.Form{Style: "search", Icon: "search", Submit: "Search",
		Actions: []widget.FormAction{{Label: "Refresh feeds", Action: "refresh", Icon: "refresh-cw"}},
		Fields: []widget.Widget{
			&widget.Field{Name: "q", Value: q, Placeholder: "Package name", Autofocus: true},
			&widget.Field{Kind: "hidden", Name: "_primary", Value: "search"},
		}}
	if refreshing {
		toolbar.Actions = nil
		toolbar.Note = "Refreshing the feeds… **reload to see the result**."
	} else {
		checkedAt, statusErr := s.backend.PkgStatus(r.Context(), sid)
		toolbar.Note = freshnessLine(checkedAt, statusErr)
	}
	children = append(children, toolbar, &widget.Divider{Tight: true})

	if refreshing {
		children = append(children, refreshingInstead(
			"Results come from the feed index this refresh is rebuilding — reload the page in a moment to search it."))
	} else if q == "" {
		children = append(children, &widget.Empty{
			Icon:  "search",
			Title: "Search available packages",
			Body:  "Enter a package name, then select Search. Matching packages will appear here.",
		})
	} else if pkgs, total, err := s.backend.PkgSearch(r.Context(), sid, q); err != nil {
		children = append(children, &widget.Callout{Variant: "warning", Title: "Search unavailable",
			Body: fmt.Sprintf("The package index could not be read (%v). Refresh the feeds and try again.", err)})
	} else if len(pkgs) == 0 {
		children = append(children, &widget.Empty{
			Icon:  "search",
			Title: "No packages found",
			Body:  fmt.Sprintf(tr("No available packages match “%s”. Check the spelling or refresh the package feeds."), q),
		})
	} else {
		if total > len(pkgs) {
			children = append(children, &widget.Badge{Variant: "info", Icon: "info", Size: "lg", Text: fmt.Sprintf(
				"Showing the first %d of %d matches — narrow the search to see the rest", len(pkgs), total)})
		}
		children = append(children, discoverTable(pkgs, q))
	}

	var body strings.Builder
	if err := s.widgets.RenderWithToken(&body, &widget.Stack{Children: children}, s.sessionCSRF(r), lang, t); err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	s.renderPage(w, r, http.StatusOK, pageHeader{
		Heading:    "System",
		Subheading: "Search the packages available from your configured feeds.",
		Modes:      packageModes(r.URL.Path),
	}, "narrow", s.systemPages(r.URL.Path), false, template.HTML(body.String()))
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

// discoverTable lists Available matches: name, what it does, version, feed origin,
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
			{Text: p.Version, Emphasis: true},
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
		if !p.Removable {
			label = "Installed"
		}
	}
	desc := p.Description
	if desc == "" {
		desc = "No description in the feed."
	}
	props := []widget.Property{
		{Label: "Package", Value: p.Name, Mono: true},
		{Label: "Version", Value: p.Version, Mono: true},
		{Label: "Feed", Value: p.Feed, Mono: true},
	}
	if len(p.RequiredBy) > 0 {
		props = append(props, widget.Property{Label: "Required by", Value: strings.Join(p.RequiredBy, ", "), Mono: true})
	} else if p.Installed && !p.Removable {
		props = append(props, widget.Property{Label: "Removal", Value: "Protected system package"})
	}
	children := []widget.Widget{
		&widget.Text{Markdown: desc},
		&widget.Properties{Items: props},
	}
	if !p.Installed || p.Removable {
		children = append(children, &widget.Form{Submit: label, Fields: []widget.Widget{
			&widget.Field{Kind: "hidden", Name: "package", Value: p.Name},
			&widget.Field{Kind: "hidden", Name: "_primary", Value: verb},
			&widget.Field{Kind: "hidden", Name: "q", Value: q},
		}})
	}
	return &widget.RowDrawer{Title: label + " — " + p.Name, Children: children}
}
