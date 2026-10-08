// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/updatecheck"
	"github.com/we-are-mono/verso/internal/widget"
)

// The package surface (ADR-011): shell-owned, because installing a package
// mutates the set of things the shell trusts. Installed and Upgradable filter
// the inventory; All pages through the cached index. Install searches in a
// drawer. A package is a group of files; what
// those files RUN lives on the Services page, which answers the other
// question. Package operations ride the helper's apk verbs; nothing here is a
// uci write, so none of it stages (ADR-010 boundary).

// handlePackagesPage renders the Installed inventory.
func (s *Server) handlePackagesPage(w http.ResponseWriter, r *http.Request) {
	s.renderPackages(w, r, "")
}

// handlePackagesAction shares the exact-name install, remove and upgrade path.
func (s *Server) handlePackagesAction(w http.ResponseWriter, r *http.Request) {
	s.handleDiscoverAction(w, r)
}

// renderPackages composes the inventory: the full installed set, the lens
// keeping 100+ rows one page. errMsg, when set, leads as a danger callout.
func (s *Server) renderPackages(w http.ResponseWriter, r *http.Request, errMsg string) {
	lang, t := s.localize(r)
	tr := translatorOrIdentity(t)
	installed, total, count := 0, 0, 0
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	// A search that arrives without a view searches everything: whoever links
	// here by name (DNS & DHCP naming a package to install) is after a package
	// that may not be installed yet.
	tab := r.URL.Query().Get("tab")
	all := tab == "all" || (tab == "" && q != "")
	if len(q) > 128 {
		http.Error(w, "bad query", http.StatusBadRequest)
		return
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 || offset > 100000 {
		http.Error(w, "bad offset", http.StatusBadRequest)
		return
	}
	children := []widget.Widget{}
	if errMsg != "" {
		children = append(children, &widget.Callout{Variant: "danger", Title: "Action failed", Body: errMsg})
	}
	// An update of every package is started here, so what became of it is said
	// here, once.
	if err := packageUpgrade.takeFailure(); err != nil {
		children = append(children, &widget.Callout{Variant: "danger", Compact: true, Body: fmt.Sprintf(tr("Package update failed: %v"), err)})
	}
	if feedRefresh.running() {
		children = append(children, refreshingInstead(
			"The list returns as soon as the refresh finishes — reload the page in a moment."))
	} else {
		var pkgs []openwrt.Package
		var pkgErr error
		if all {
			var page openwrt.PackagePage
			page, pkgErr = s.backend.PkgBrowse(r.Context(), s.sessionSID(r), q, offset)
			pkgs, installed, total, count = page.Packages, page.Installed, page.Total, page.Count
		} else {
			pkgs, pkgErr = s.backend.PkgInstalled(r.Context(), s.sessionSID(r))
			installed = len(pkgs)
		}
		if pkgErr != nil {
			children = append(children, &widget.Callout{Variant: "warning", Title: "Package list unavailable",
				Body: fmt.Sprintf(tr("The package database could not be read (%v)."), pkgErr)})
		}
		var table *widget.Table
		if all {
			table = packageListingTable(pkgs)
		} else {
			table = packagesTable(pkgs).(*widget.Table)
		}
		truth, _ := s.updateTruth()
		upgrades := map[string]string{}
		for _, p := range truth.Packages {
			upgrades[p.Name] = p.Available
		}
		for i := range table.Rows {
			row := &table.Rows[i]
			if version := upgrades[row.ID]; version != "" && (!all || pkgs[i].Installed) {
				row.Tags = []string{"upgradable"}
				// What it becomes, under what it is: the installed version
				// recedes and the available one is led by an arrow in the action's
				// colour — an update on offer is news, not a warning.
				row.Cells[1].Sub, row.Cells[1].Variant = version, "info"
				row.Cells[4].Actions = append([]widget.TableRowAct{{Icon: "upload", Title: "Update", Opens: true}}, row.Cells[4].Actions...)
			}
		}
		children = append(children, table)
	}

	truth, _ := s.updateTruth()
	busy := feedRefresh.running()
	// What the band narrows — the outcome, the rows, the index's pager — is
	// one block under it, a cell apart within.
	if all {
		children = append(children, packagesPager(q, offset, total, tr))
	}
	listing := &widget.Stack{Children: []widget.Widget{packagesBar(all, q, installed, len(truth.Packages), count), &widget.Stack{Children: children}}}
	var page strings.Builder
	if err := s.widgets.RenderWithToken(&page, listing, s.sessionCSRF(r), lang, t); err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	// While the index refreshes, the busy button says so and the note beside it
	// says nothing: the index has no age until the refresh ends.
	note := ""
	if !busy {
		checked, checkErr := s.backend.PkgStatus(r.Context(), s.sessionSID(r))
		note = packageIndexNote(checked, checkErr, tr)
	}
	// A listing read in place carries the index's age, so the heading's note
	// keeps up with it, and says when a refresh another operator started holds
	// the listing back.
	if r.Header.Get("X-Verso-Interaction") == "packages" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("X-Verso-Packages-Note", url.PathEscape(note))
		if busy {
			w.Header().Set("X-Verso-Packages", "refreshing")
		}
		_, _ = w.Write([]byte(page.String()))
		return
	}
	// Install is the page's forward act, so it sits on the heading line where
	// every listing keeps its primary, and opens the search panel in place. While
	// anything installed has a newer version, the act that updates them all
	// stands beside it: this is where the upgradable list is. Refresh index
	// leads the line, the index's age before it.
	acts := []widget.Widget{packagesRefreshAct(busy)}
	if n := len(truth.Packages); n > 0 || packageUpgrade.running() {
		acts = append(acts, packagesUpdateAct(n))
	}
	var lead strings.Builder
	for _, act := range acts {
		if err := s.widgets.RenderWithToken(&lead, act, s.sessionCSRF(r), lang, t); err != nil {
			http.Error(w, "render error", http.StatusInternalServerError)
			return
		}
	}
	install := &widget.ActionBar{Heading: true, OpensPanel: true, Action: &widget.TableAction{Label: "Install", Href: "/system/packages/discover"}}
	hdr := pageHeader{Heading: "Packages", Tone: "neutral", HeadingNote: &note,
		HeadingAct: template.HTML(lead.String()) + s.headingAct(r, install, lang, t)} //nolint:gosec // rendered by the shell's own widgets
	s.renderPage(w, r, http.StatusOK, hdr, "wide", s.sectionPages("System", r.URL.Path), template.HTML(page.String()))
}

// packagesBar is the listing's control band. The cut is one dropdown:
// Installed and Upgradable narrow the inventory on screen, and All is another
// listing — the cached index, paged and searched by the router — so its
// option (or, from it, theirs) loads that listing instead of narrowing. The
// search narrows the inventory in place and asks the router of the index.
func packagesBar(all bool, q string, installed, upgradable, count int) *widget.ActionBar {
	bar := &widget.ActionBar{Filter: "Find a package", Finds: true, Query: q, Paged: all}
	if all {
		bar.Tabs = []widget.ActionTab{
			{Label: "Installed", Count: installed, Href: packagesPath},
			{Label: "Upgradable", Count: upgradable, Match: "upgradable", Href: packagesPath + "?tab=upgradable"},
			{Label: "All", Count: count, Match: "all", Active: true},
		}
		return bar
	}
	bar.Tabs = []widget.ActionTab{
		{Label: "Installed", Count: installed, Active: true},
		{Label: "Upgradable", Count: upgradable, Match: "upgradable"},
		{Label: "All", Match: "all", Href: packagesPath + "?tab=all", Uncounted: true},
	}
	return bar
}

// packagesPager walks the index thirty packages at a time: one line of prose
// saying how many matched, with the way to the pages either side.
func packagesPager(q string, offset, total int, tr func(string) string) widget.Widget {
	at := func(n int) string {
		return packagesPath + "?tab=all&q=" + url.QueryEscape(q) + "&offset=" + strconv.Itoa(n)
	}
	line := fmt.Sprintf(tr("%d matches"), total)
	if offset > 0 {
		line += " · [" + tr("Previous") + "](" + at(max(0, offset-30)) + ")"
	}
	if offset+30 < total {
		line += " · [" + tr("Next") + "](" + at(offset+30) + ")"
	}
	return &widget.Text{Markdown: line, Translated: true}
}

// packagesRefreshAct reads the feeds' index anew. While it runs the button
// says so and stays pressed; the page's script keeps it in step after that
// (verso-packages.js).
func packagesRefreshAct(busy bool) widget.Widget {
	label := "Refresh index"
	if busy {
		label = "Refreshing index…"
	}
	return &widget.Form{Action: packagesPath + "/discover", Style: "inline", NoSubmit: true, Fields: []widget.Widget{
		&widget.Field{Kind: "hidden", Name: "return_to", Value: packagesPath},
		&widget.Button{Label: label, Style: "secondary", Name: "_action", Value: "refresh", Loading: busy},
	}}
}

// packagesTable is the inventory roster: name, version, feed — files on disk,
// no live state (that is the Services page's question). The drawer tells each
// package's story and offers the Remove.
func packagesTable(pkgs []openwrt.Package) widget.Widget {
	pkgs = append([]openwrt.Package(nil), pkgs...)
	for i := range pkgs {
		pkgs[i].Installed = true
	}
	return packageListingTable(pkgs)
}

func packageListingTable(pkgs []openwrt.Package) *widget.Table {
	cols := []widget.TableColumn{
		{Label: "Package", Kind: "name", Width: widget.MeasureLong},
		{Label: "Version", Kind: "mono", Width: widget.MeasureWord},
		{Label: "What it is", Kind: "comment"},
		{Label: "Size", Kind: "runtime", Width: widget.MeasureShort},
		{Kind: "actions", Width: widget.MeasureShort},
	}
	rows := make([]widget.TableRow, 0, len(pkgs))
	for _, p := range pkgs {
		rows = append(rows, packageRow(p))
	}
	return &widget.Table{Style: "flat", Columns: cols, Rows: rows}
}

// packageRow keeps the row terse; the drawer is where the package tells its
// story. A package that can go is removed from the row, never from the drawer:
// its trash act asks first, then posts the one pair (remove=<name>).
func packageRow(p openwrt.Package) widget.TableRow {
	size := "—"
	if p.Size > 0 {
		size = humanSize(p.Size)
	}
	acts := []widget.TableRowAct{{Icon: "lock", Title: "Required package"}}
	if !p.Installed {
		acts = []widget.TableRowAct{{Icon: "download", Title: "Install", Opens: true}}
	} else if p.Removable {
		acts = []widget.TableRowAct{{Icon: "trash-2", Title: "Remove", Name: "remove", Value: p.Name,
			ConfirmTitle: "Remove %s?",
			Confirm:      "Everything it installed is taken off the router, and whatever needs it may stop working."}}
	}
	// The row ships its drawer's address, not its drawer: a router holds
	// hundreds of packages, and rendering every drawer for the one anybody
	// opens cost the listing seconds.
	return widget.TableRow{ID: p.Name, Cells: []widget.TableCell{
		{Text: p.Name, Opens: true}, {Text: p.Version}, {Text: packageDescription(p)}, {Text: size}, {Actions: acts},
	}, Panel: packagePanelPath + "?name=" + url.QueryEscape(p.Name)}
}

func packageDescription(p openwrt.Package) string {
	if p.Description == "" {
		return "No description in the package."
	}
	return p.Description
}

// packageDrawer is one package in one flow, nothing to navigate: what it is,
// its project's site, its facts — the version stating what it becomes when an
// update is on offer (available) — the files it installed, which the drawer
// reads in as it opens (verso-packages.js), and last its one act, named for
// what it does to what: installing a package not yet here, or updating one
// that has a newer version. Removing is the row's act, not the drawer's.
func packageDrawer(p openwrt.Package, available string) *widget.RowDrawer {
	children := []widget.Widget{&widget.Text{Markdown: packageDescription(p)}}
	if website := packageWebsite(p.Webpage); website != nil {
		children = append(children, website)
	}
	props := []widget.Property{
		{Label: "Version", Value: p.Version, Next: available, Mono: true},
		{Label: "Feed", Value: p.Feed, Mono: true},
	}
	if p.License != "" {
		props = append(props, widget.Property{Label: "License", Value: p.License, Mono: true})
	}
	if p.Size > 0 {
		props = append(props, widget.Property{Label: "Size", Value: humanSize(p.Size), Verbatim: true})
	}
	if len(p.RequiredBy) > 0 {
		props = append(props, widget.Property{Label: "Required by", Value: strings.Join(p.RequiredBy, ", "), Mono: true})
	} else if p.Installed && !p.Removable {
		props = append(props, widget.Property{Label: "Removal", Value: "Protected system package"})
	}
	children = append(children, &widget.Properties{Style: "system", Items: props})
	if p.Installed {
		children = append(children, &widget.Link{Label: "Installed files", Href: "/system/packages/files?package=" + url.QueryEscape(p.Name)})
	}
	switch {
	case !p.Installed:
		children = append(children, packageActionForm(p.Name, "install", "Install "+p.Name))
	case available != "":
		children = append(children, packageActionForm(p.Name, "upgrade", "Update "+p.Name))
	}
	return &widget.RowDrawer{Title: p.Name, Verbatim: true, Children: children}
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
	if r.URL.Path == "/system/packages" {
		back = "/system/packages"
	}
	if q != "" {
		back += "?q=" + url.QueryEscape(q)
	}
	if r.PostForm.Get("return_to") == "/system/packages" {
		back = "/system/packages"
	}
	// A secondary button's _action (Refresh) outranks the form's primary. A
	// row's trash act posts its one pair, remove=<name>: removal is the row's
	// act, asked about on the row.
	verb := r.PostForm.Get("_action")
	if verb == "" {
		verb = r.PostForm.Get("_primary")
	}
	if verb == "" && r.PostForm.Get("remove") != "" {
		verb = "remove"
	}
	switch verb {
	case "search":
		if panelRequest(r) {
			s.renderDiscover(w, r, "")
			return
		}
		http.Redirect(w, r, back, http.StatusSeeOther)
	case "refresh":
		// The run outlives this response, so it carries a context of its own;
		// the helper client's deadline is what bounds the wait now.
		if !feedRefresh.start(func() error {
			if err := s.backend.PkgUpdate(context.Background(), sid); err != nil {
				return err
			}
			// A rebuilt index is exactly when what this router can install changes,
			// so the update truths are re-read here — never on a page render.
			return s.refreshPackageTruth(context.Background(), sid)
		}) {
			s.flash(r, "info", tr("The feeds are already being refreshed."))
		}
		if panelRequest(r) || r.Header.Get("X-Verso-Interaction") == "packages" {
			w.Header().Set("X-Verso-Packages", "refreshing")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.Redirect(w, r, back, http.StatusSeeOther)
	case "install", "remove", "upgrade":
		name := r.PostForm.Get("package")
		if name == "" {
			name = r.PostForm.Get("remove")
		}
		if !pkgNameOK(name) {
			http.Error(w, "bad package name", http.StatusBadRequest)
			return
		}
		if feedRefresh.running() {
			if panelRequest(r) {
				s.entityNotice(w, http.StatusConflict, tr("The feeds are being refreshed — try again in a moment."))
				return
			}
			s.flash(r, "info", tr("The feeds are being refreshed — try again in a moment."))
			http.Redirect(w, r, back, http.StatusSeeOther)
			return
		}
		var err error
		if verb == "install" {
			err = s.backend.PkgInstall(r.Context(), sid, name)
		} else if verb == "upgrade" {
			err = s.backend.PkgUpgradeOne(r.Context(), sid, name)
		} else {
			err = s.backend.PkgRemove(r.Context(), sid, name)
		}
		if err != nil {
			if panelRequest(r) {
				s.entityNotice(w, http.StatusBadGateway, fmt.Sprintf(tr("The device refused: %v."), err))
				return
			}
			if r.URL.Path == packagesPath {
				s.renderPackages(w, r, fmt.Sprintf(tr("The device refused: %v."), err))
			} else {
				s.renderDiscover(w, r, fmt.Sprintf(tr("The device refused: %v."), err))
			}
			return
		}
		// A plugin package just landed (or left): re-read the manifests so
		// its pages and nav rows exist without a shell restart.
		s.Rescan()
		if err := s.refreshPackageTruth(r.Context(), sid); err != nil {
			log.Printf("verso: package action completed; update status: %v", err)
		}
		if panelRequest(r) {
			message := tr("%s removed.")
			if verb == "install" {
				message = tr("%s installed.")
			}
			if verb == "upgrade" {
				message = tr("%s upgraded.")
			}
			w.Header().Set("HX-Reswap", "none")
			w.Header().Set("X-Verso-Packages", "changed")
			lang, _ := s.localize(r)
			_ = s.pageSet(lang).ExecuteTemplate(w, "verso-flash", widget.Flash{Variant: "success", Message: fmt.Sprintf(message, name)})
			// Installing/removing a plugin or catalog can change navigation. Send
			// those rows with the outcome, without rebuilding the surrounding page.
			pluginTr := s.pluginTranslators(r)
			pages := s.sectionPages("System", packagesPath)
			for i := range pages {
				pages[i].Label = localizeLabel(pages[i].PluginID, pages[i].Label, tr, pluginTr)
			}
			nav := s.buildSidebar(packagesPath, tr, pluginTr, pages)
			_ = s.pageSet(lang).ExecuteTemplate(w, "package-navigation.html.tmpl", nav)
			return
		}
		if verb == "upgrade" {
			s.flash(r, "success", fmt.Sprintf(tr("%s upgraded."), name))
		} else if verb == "install" {
			s.flash(r, "success", fmt.Sprintf(tr("%s installed."), name))
		} else {
			s.flash(r, "success", fmt.Sprintf(tr("%s removed."), name))
		}
		http.Redirect(w, r, back, http.StatusSeeOther)
	default:
		http.Error(w, "no package action in form", http.StatusBadRequest)
	}
}

// backgroundJob is one device-wide operation detached from the browser. Work that
// reaches the network takes longer than a person will hold a page open for, so the
// POST starts it and answers; each render then states what the job is doing.
// verso-rpcd's own mutexes remain the serializers that keep two such runs apart —
// this job's guard is what makes the page honest about a run already being under
// way, and what moves the helper call's wait off the request.
type backgroundJob struct {
	mu      sync.Mutex
	active  bool
	done    bool // the last run finished without error and has not been acknowledged
	failure error
}

// jobPhase is one job's lifecycle as a whole, for a surface that must stand on
// the outcome rather than glance at it and forget: never-started or acknowledged
// (idle), under way (running), finished clean (done), or failed. The firmware
// takeover reads this — where the other update surfaces only ask running() and
// takeFailure() — because it holds a person on the outcome until it is released.
type jobPhase int

const (
	jobIdle jobPhase = iota
	jobRunning
	jobDone
	jobFailed
)

// start runs work in the background unless a run is already under way, reporting
// whether this call owns the new one.
func (j *backgroundJob) start(refresh func() error) bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.active {
		return false
	}
	j.active, j.failure, j.done = true, nil, false
	go func() {
		err := refresh()
		j.mu.Lock()
		j.active, j.failure, j.done = false, err, err == nil
		j.mu.Unlock()
	}()
	return true
}

// running reports whether a run is under way — and so, for the package jobs,
// whether apk is holding verso-rpcd's package guard. Every other package verb
// queues behind that guard, so while this is true the surface asks the helper for
// nothing and states what the device is doing instead: a page that answers at once
// and says the listing is a moment away beats a page that is right in a minute.
func (j *backgroundJob) running() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.active
}

// takeFailure reports how the last finished run failed and forgets it, so a
// failure nobody was waiting for is still stated once, on the next visit.
func (j *backgroundJob) takeFailure() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	failure := j.failure
	j.failure = nil
	return failure
}

// phase reports the run's lifecycle as one value, read under the same lock that
// finishes it — so "running" and "failed" (or "done") can never be seen apart:
// the goroutine flips active off and records its outcome in one critical section.
func (j *backgroundJob) phase() jobPhase {
	j.mu.Lock()
	defer j.mu.Unlock()
	switch {
	case j.active:
		return jobRunning
	case j.failure != nil:
		return jobFailed
	case j.done:
		return jobDone
	default:
		return jobIdle
	}
}

// peekFailure reports how the last finished run failed without forgetting it —
// the non-consuming read a server-authoritative surface needs, so a reload keeps
// landing on the failure until the person is the one who releases it.
func (j *backgroundJob) peekFailure() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.failure
}

// acknowledge releases a finished run's outcome — a failure or a clean done — so
// the surface that watched it steps aside and the ordinary page returns. A run
// still under way is left untouched: there is nothing yet to acknowledge.
func (j *backgroundJob) acknowledge() {
	j.mu.Lock()
	defer j.mu.Unlock()
	if !j.active {
		j.failure, j.done = nil, false
	}
}

// feedRefresh is the feed refresh: reaching every configured repository is slow,
// and the package index it rebuilds is one file set the whole device shares, so
// the refresh is one act device-wide, not one per operator.
var feedRefresh backgroundJob

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
	toolbar := &widget.Form{Action: "/system/packages/discover", Style: "search", Submit: "Search",
		Actions: []widget.FormAction{{Label: "Refresh index", Action: "refresh"}},
		Fields: []widget.Widget{
			&widget.Field{Name: "q", Value: q, Placeholder: "Package name", Autofocus: true},
			&widget.Field{Kind: "hidden", Name: "_primary", Value: "search"},
		}}
	if panelRequest(r) {
		toolbar.Frame, toolbar.Panel = widget.FramePanel, "/system/packages/discover"
		toolbar.Actions = nil
	}
	if refreshing {
		toolbar.Actions = nil
		toolbar.Note = "Refreshing the feeds… **reload to see the result**."
	} else {
		checkedAt, statusErr := s.backend.PkgStatus(r.Context(), sid)
		toolbar.Note = packageIndexNote(checkedAt, statusErr, tr)
		toolbar.NoteVerbatim = true
	}
	children = append(children, toolbar, &widget.Divider{Tight: true})

	if refreshing {
		children = append(children, refreshingInstead(
			"Results come from the feed index this refresh is rebuilding — reload the page in a moment to search it."))
	} else if q == "" {
		// Before a search the listing says what goes in it, in its one row.
		table := discoverTable(nil, q).(*widget.Table)
		table.EmptyText = "Enter a package name, then select Search. Matching packages will appear here."
		children = append(children, table)
	} else if pkgs, total, err := s.backend.PkgSearch(r.Context(), sid, q); err != nil {
		children = append(children, &widget.Callout{Variant: "warning", Title: "Search unavailable",
			Body: fmt.Sprintf(tr("The package index could not be read (%v). Refresh the feeds and try again."), err)})
	} else if len(pkgs) == 0 {
		// The row is plain text, so the query reads back exactly as typed.
		table := discoverTable(nil, q).(*widget.Table)
		table.EmptyText = fmt.Sprintf(tr("No available packages match “%s”. Check the spelling or refresh the package feeds."), q)
		children = append(children, table)
	} else {
		if total > len(pkgs) {
			children = append(children, &widget.Badge{Variant: "info", Icon: "info", Size: "lg", Text: fmt.Sprintf(
				tr("Showing the first %d of %d matches — narrow the search to see the rest"), len(pkgs), total)})
		}
		table := discoverTable(pkgs, q).(*widget.Table)
		truth, _ := s.updateTruth()
		for i := range table.Rows {
			if !pkgs[i].Installed {
				continue
			}
			for _, upgrade := range truth.Packages {
				if upgrade.Name == table.Rows[i].ID {
					table.Rows[i].Drawer = packageDrawer(pkgs[i], upgrade.Available)
					break
				}
			}
		}
		children = append(children, table)
	}

	var body strings.Builder
	if err := s.widgets.RenderWithToken(&body, &widget.Stack{Children: children}, s.sessionCSRF(r), lang, t); err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	if panelRequest(r) {
		data := struct {
			Title string
			Body  template.HTML
		}{tr("Install packages"), template.HTML(body.String())}
		if err := s.pageSet(lang).ExecuteTemplate(w, "system-panel.html.tmpl", data); err != nil {
			http.Error(w, "render error", http.StatusInternalServerError)
		}
		return
	}
	s.renderPage(w, r, http.StatusOK, pageHeader{Heading: "Install packages", Tone: "neutral"}, "wide", s.sectionPages("System", r.URL.Path), template.HTML(body.String()))
}

func packageIndexNote(checkedAt int64, err error, tr func(string) string) string {
	switch {
	case err != nil:
		return tr("Index freshness unknown")
	case checkedAt == 0:
		return tr("Index has not been refreshed")
	default:
		return fmt.Sprintf(tr("Index refreshed %s"), localizedAgo(time.Since(time.Unix(checkedAt, 0)), tr))
	}
}

// discoverTable lists Available matches at the drawer's width: name, version,
// installed state, and Details (the shell's act for a name that opens) — the
// drawer carries what it does, where it is from, and the act.
func discoverTable(pkgs []openwrt.Package, q string) widget.Widget {
	cols := []widget.TableColumn{
		{Label: "Package", Kind: "name"},
		{Label: "Version", Kind: "mono", Width: widget.MeasureWord},
		{Label: "State", Kind: "pill", Width: widget.MeasureShort},
		{Kind: "actions", Width: widget.MeasureShort},
	}
	rows := make([]widget.TableRow, 0, len(pkgs))
	for _, p := range pkgs {
		state := widget.TableCell{}
		if p.Installed {
			state = widget.TableCell{Text: "installed", Variant: "success"}
		}
		rows = append(rows, widget.TableRow{ID: p.Name, Cells: []widget.TableCell{
			{Text: p.Name, Opens: true},
			{Text: p.Version, Emphasis: true},
			state,
			{},
		}, Drawer: discoverDrawer(p, q)})
	}
	return &widget.Table{Style: "flat", Columns: cols, Rows: rows}
}

// discoverDrawer is the act: what it is, where it is from, then Install — or
// Remove for what is already here. The active query rides along so acting on
// a result lands back on the same search — a shopping flow installs several.
func discoverDrawer(p openwrt.Package, q string) *widget.RowDrawer {
	return packageDrawer(p, "")
}

func packageActionForm(name, verb, label string) *widget.Form {
	return &widget.Form{Action: "/system/packages", Frame: widget.FramePanel, Panel: "/system/packages", Submit: label, Fields: []widget.Widget{
		&widget.Field{Kind: "hidden", Name: "package", Value: name},
		&widget.Field{Kind: "hidden", Name: "_primary", Value: verb},
	}}
}

// packagePanelPath is where one package's panel is read from.
const packagePanelPath = "/system/packages/package"

// handlePackagePanel answers one package's own panel — the drawer each row on
// Packages fetches as it opens, Install and all — and the same panel for a page
// that offers the package where the need for it is (DNS & DHCP's encryption
// and blocklist), so it installs without leaving that page. Asked for as a
// page, the package is found on Packages instead.
func (s *Server) handlePackagePanel(w http.ResponseWriter, r *http.Request) {
	lang, t := s.localize(r)
	tr := translatorOrIdentity(t)
	name := r.URL.Query().Get("name")
	if !pkgNameOK(name) {
		http.Error(w, "bad package name", http.StatusBadRequest)
		return
	}
	if !panelRequest(r) {
		http.Redirect(w, r, "/system/packages?tab=all&q="+url.QueryEscape(name), http.StatusSeeOther)
		return
	}
	if feedRefresh.running() {
		s.entityNotice(w, http.StatusConflict, tr("The feeds are being refreshed — try again in a moment."))
		return
	}
	p, ok, err := s.backend.PkgInfo(r.Context(), s.sessionSID(r), name)
	if err != nil {
		s.entityNotice(w, http.StatusBadGateway, fmt.Sprintf(tr("The package index could not be read (%v). Refresh the feeds and try again."), err))
		return
	}
	if !ok {
		s.entityNotice(w, http.StatusNotFound, fmt.Sprintf(tr("%s is not in the package feeds. Refresh the index on Packages and try again."), name))
		return
	}
	// An installed package with a newer copy in the feeds offers it, as its
	// row on Packages says.
	available := ""
	if truth, known := s.updateTruth(); known && p.Installed {
		if i := slices.IndexFunc(truth.Packages, func(u openwrt.PackageUpgrade) bool { return u.Name == name }); i >= 0 {
			available = truth.Packages[i].Available
		}
	}
	row := packageRow(p)
	row.Drawer = packageDrawer(p, available)
	row.Drawer.Open = true
	var body bytes.Buffer
	if _, err := s.widgets.RenderOpenPanelWithToken(&body, &widget.Table{Columns: []widget.TableColumn{{}}, Rows: []widget.TableRow{row}},
		s.sessionCSRF(r), lang, t, widget.Flash{}); err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(body.Bytes())
}

func (s *Server) handlePackageFiles(w http.ResponseWriter, r *http.Request) {
	_, t := s.localize(r)
	tr := translatorOrIdentity(t)
	name := r.URL.Query().Get("package")
	if !pkgNameOK(name) {
		http.Error(w, "bad package name", http.StatusBadRequest)
		return
	}
	if feedRefresh.running() {
		s.entityNotice(w, http.StatusConflict, tr("The feeds are being refreshed — try again in a moment."))
		return
	}
	files, err := s.backend.PkgFiles(r.Context(), s.sessionSID(r), name)
	if err != nil {
		s.entityNotice(w, http.StatusBadGateway, fmt.Sprintf(tr("The device refused: %v."), err))
		return
	}
	lang, _ := s.localize(r)
	data := struct {
		Name  string
		Files []string
	}{name, files}
	_ = s.pageSet(lang).ExecuteTemplate(w, "package-files.html.tmpl", data)
}

func localizedAgo(d time.Duration, tr func(string) string) string {
	if d < time.Minute {
		return tr("just now")
	}
	if d < time.Hour {
		return fmt.Sprintf(tr("%d min ago"), int(d.Minutes()))
	}
	if d < 48*time.Hour {
		return fmt.Sprintf(tr("%d h ago"), int(d.Hours()))
	}
	return fmt.Sprintf(tr("%d days ago"), int(d.Hours()/24))
}

func (s *Server) handlePackageStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_, t := s.localize(r)
	tr := translatorOrIdentity(t)
	feedRefresh.mu.Lock()
	busy, failure := feedRefresh.active, feedRefresh.failure
	feedRefresh.mu.Unlock()
	message := ""
	if failure != nil {
		message = fmt.Sprintf(tr("The package feeds could not be refreshed (%v)."), failure)
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"refreshing": busy, "checking": updateChecks.running(), "error": message})
}

// Package operations only change the local package lane. Keep the last firmware
// answer and its timestamp; there is no reason to contact the firmware server.
func (s *Server) refreshPackageTruth(ctx context.Context, sid string) error {
	packages, err := s.backend.PkgUpgradable(ctx, sid)
	if err != nil {
		return err
	}
	truth, _ := s.updateTruth()
	truth.Packages = packages
	if truth.CheckedAt.IsZero() {
		truth.CheckedAt = time.Now()
	}
	return updatecheck.Write(s.stateDir, truth)
}
