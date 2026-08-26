// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"fmt"
	"html/template"
	"net"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/plugin"
	"github.com/we-are-mono/verso/internal/widget"
)

// The Services page: the processes this router runs — procd's service table,
// the userspace half of `ps`. A service is a running (or stoppable) thing; a
// package is files on disk — two natures, two pages (the Packages page owns
// the other). Every row drives the procd lifecycle: the switch is the one
// human concept (on = enable+start, off = stop+disable), Restart lives in the
// drawer, and Verso plugins add their liveness probe and declared powers.
// Nothing here stages (ADR-010 boundary); a keep-list refuses severing the
// surface itself.

// pluginServicePrefix is the init-script naming convention (ADR-011 §3): the
// package verso-plugin-<id> installs /etc/init.d/verso-plugin-<id>.
const pluginServicePrefix = "verso-plugin-"

// rcActions is the closed set of lifecycle verbs the surface forwards to
// procd; anything else in a POST is rejected before touching the bus.
var rcActions = map[string]bool{"start": true, "stop": true, "restart": true, "enable": true, "disable": true}

// svcKeep is the set of services this surface refuses to stop or restart:
// severing them severs the surface itself (the shell, its privileged path,
// the bus). Everything else — network included — is the operator's call,
// exactly as it is over SSH.
var svcKeep = map[string]bool{"verso": true, "rpcd": true, "ubus": true}

// svcNameRe is the shape of an init-script name — the only thing that reaches
// procd's rc object.
var svcNameRe = regexp.MustCompile(`^[a-z0-9][a-zA-Z0-9._-]{0,63}$`)

func svcNameOK(name string) bool { return svcNameRe.MatchString(name) }

// socketProbeTimeout bounds the liveness dial; a plugin that cannot accept a
// connection this fast is not answering pages either.
const socketProbeTimeout = 400 * time.Millisecond

// probeSocket reports whether the plugin's unix socket accepts a connection
// right now — the same reachability its pages depend on.
func probeSocket(path string) bool {
	conn, err := net.DialTimeout("unix", path, socketProbeTimeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// pluginState is one installed plugin's assembled truth: the manifest identity
// plus procd's view and the socket probe.
type pluginState struct {
	Manifest plugin.Manifest
	Service  string // init script name, by the verso-plugin-<id> convention
	Known    bool   // procd's rc table lists the service
	Enabled  bool   // starts at boot
	Running  bool   // procd's live view
	Alive    bool   // the socket accepted a connection just now
}

// pluginStates assembles the plugin rows from the discovered manifests and
// the rc snapshot it is handed. An empty snapshot degrades to Known=false
// rows rather than an empty page — the manifests are still the truth about
// what is installed.
func (s *Server) pluginStates(rc map[string]openwrt.RCState) []pluginState {
	manifests := s.manifestList()
	states := make([]pluginState, 0, len(manifests))
	for _, m := range manifests {
		svc := pluginServicePrefix + m.ID
		st := pluginState{Manifest: m, Service: svc}
		if r, ok := rc[svc]; ok {
			st.Known, st.Enabled, st.Running = true, r.Enabled, r.Running
		}
		if st.Running {
			st.Alive = s.probe(m.Socket)
		}
		states = append(states, st)
	}
	sort.Slice(states, func(a, b int) bool { return states[a].Manifest.ID < states[b].Manifest.ID })
	return states
}

// handleServicesPage renders the service table.
func (s *Server) handleServicesPage(w http.ResponseWriter, r *http.Request) {
	s.renderServices(w, r, "")
}

// handleServicesAction dispatches a lifecycle POST: the on/off switch
// ("svc:<name>" or a plugin's "on:<id>") expanding to both procd facts, or a
// drawer's Restart. Success flashes the completed act and redirects (PRG).
func (s *Server) handleServicesAction(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	svc, actions := serviceActionsOf(r, s.manifestByID)
	if svc == "" {
		http.Error(w, "no action in form", http.StatusBadRequest)
		return
	}
	if !svcNameOK(svc) {
		http.Error(w, "bad service name", http.StatusBadRequest)
		return
	}
	if svcKeep[svc] {
		s.renderServices(w, r, fmt.Sprintf(
			"%s keeps this page alive — manage it over SSH if you really mean it.", svc))
		return
	}
	for _, a := range actions {
		if !rcActions[a] {
			http.Error(w, "unknown action", http.StatusBadRequest)
			return
		}
	}
	for _, a := range actions {
		if err := s.backend.RCInit(r.Context(), s.sessionSID(r), svc, a); err != nil {
			s.renderServices(w, r, fmt.Sprintf("Could not %s %s: the device refused (%v).", a, svc, err))
			return
		}
	}
	s.flash(r, "success", svc+" "+lifecycleWord(actions)+".")
	http.Redirect(w, r, "/system/services", http.StatusSeeOther)
}

// serviceActionsOf extracts (service, rc actions) from the POSTed form. The
// on/off switch posts under "on:<plugin id>" (resolved through the manifest)
// or "svc:<service>" for plain services, expanding to both procd facts; the
// drawer's action form posts plugin=<id> or service=<name> with _action
// ("" = the primary button, restart).
func serviceActionsOf(r *http.Request, byID func(string) (plugin.Manifest, bool)) (string, []string) {
	for name, vals := range r.PostForm {
		if len(vals) == 0 {
			continue
		}
		var svc string
		if id, found := strings.CutPrefix(name, "on:"); found {
			m, ok := byID(id)
			if !ok {
				continue
			}
			svc = pluginServicePrefix + m.ID
		} else if v, found := strings.CutPrefix(name, "svc:"); found {
			svc = v
		} else {
			continue
		}
		if vals[0] == "on" {
			return svc, []string{"enable", "start"}
		}
		return svc, []string{"stop", "disable"}
	}
	action := r.PostForm.Get("_action")
	if action == "" {
		action = r.PostForm.Get("_primary")
	}
	if id := r.PostForm.Get("plugin"); id != "" {
		m, ok := byID(id)
		if !ok {
			return "", nil
		}
		return pluginServicePrefix + m.ID, []string{action}
	}
	if svc := r.PostForm.Get("service"); svc != "" {
		return svc, []string{action}
	}
	return "", nil
}

// lifecycleWord names the completed act in the flash: the switch pairs read
// as their one human concept, single verbs as their past tense.
func lifecycleWord(actions []string) string {
	if len(actions) == 2 {
		if actions[0] == "enable" {
			return "turned on"
		}
		return "turned off"
	}
	switch actions[0] {
	case "restart":
		return "restarted"
	case "start":
		return "started"
	case "stop":
		return "stopped"
	default:
		return actions[0] + "ed"
	}
}

// renderServices composes the page: procd's whole table, lined like the
// process list it corresponds to; errMsg leads as a danger callout.
func (s *Server) renderServices(w http.ResponseWriter, r *http.Request, errMsg string) {
	sid := s.sessionSID(r)
	rc, rcErr := s.backend.RCList(r.Context(), sid)
	// The providing package, by exact name match; mismatches (busybox
	// applets, firewall4's "firewall") honestly read as none.
	pkgNames := map[string]bool{}
	if pkgs, err := s.backend.PkgInstalled(r.Context(), sid); err == nil {
		for _, p := range pkgs {
			pkgNames[p.Name] = true
		}
	}

	children := []widget.Widget{}
	if errMsg != "" {
		children = append(children, &widget.Callout{Variant: "danger", Title: "Action failed", Body: errMsg})
	}
	if rcErr != nil {
		children = append(children, &widget.Callout{Variant: "warning", Title: "Service table unavailable",
			Body: fmt.Sprintf("procd could not be read (%v).", rcErr)})
	} else {
		children = append(children, servicesTable(s.pluginStates(rc), rc, pkgNames))
	}

	var body strings.Builder
	if err := s.widgets.RenderWithToken(&body, &widget.Stack{Children: children}, s.sessionCSRF(r)); err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	// Immediate acts only (ADR-011 §8) — the staged-changes bar appears here
	// solely when other pages' edits are pending.
	s.renderPage(w, r, http.StatusOK, pageHeader{
		Heading:    "Services",
		Subheading: "The processes this router runs — procd's service table, live.",
	}, "", nil, false, template.HTML(body.String()))
}

// servicesTable is procd's table, one lined row per service: name, the
// providing package (exact name match; none reads as a dash), live state,
// boot as a checkmark, and the on/off switch. No drawers — every fact is a
// column, the switch is the act, and off→on covers what restart did.
func servicesTable(states []pluginState, rc map[string]openwrt.RCState, pkgNames map[string]bool) widget.Widget {
	cols := []widget.TableColumn{
		{Label: "Service", Kind: "name"},
		{Label: "Package", Kind: "mono"},
		{Label: "State", Kind: "pill"},
		{Label: "Starts at boot", Kind: "check"},
		{Kind: "toggle"},
	}
	byService := make(map[string]pluginState, len(states))
	for _, st := range states {
		byService[st.Service] = st
	}
	rows := make([]widget.TableRow, 0, len(rc))
	for name, st := range rc {
		if ps, ok := byService[name]; ok {
			rows = append(rows, pluginServiceRow(ps, pkgNames))
			continue
		}
		rows = append(rows, serviceRow(name, st, pkgNames))
	}
	// A dev-deployed plugin procd cannot see still shows, honestly unmanaged.
	for _, ps := range states {
		if _, ok := rc[ps.Service]; !ok {
			rows = append(rows, pluginServiceRow(ps, pkgNames))
		}
	}
	sort.Slice(rows, func(a, b int) bool { return rows[a].ID < rows[b].ID })
	return &widget.Table{Style: "lined", Columns: cols, Rows: rows}
}

// packageOf is the Package column: the exact-name provider or a dash.
func packageOf(service string, pkgNames map[string]bool) string {
	if pkgNames[service] {
		return service
	}
	return "—"
}

// svcSwitch is the on/off cell; keep-listed services get none.
func svcSwitch(name string, running bool) widget.TableCell {
	if svcKeep[name] {
		return widget.TableCell{}
	}
	return widget.TableCell{On: running, Name: "svc:" + name}
}

// serviceRow is one plain procd service.
func serviceRow(name string, st openwrt.RCState, pkgNames map[string]bool) widget.TableRow {
	pill := widget.TableCell{Text: "stopped", Variant: "neutral"}
	if st.Running {
		pill = widget.TableCell{Text: "running", Variant: "success"}
	}
	return widget.TableRow{ID: name, Cells: []widget.TableCell{
		{Text: name},
		{Text: packageOf(name, pkgNames)},
		pill,
		{On: st.Enabled},
		svcSwitch(name, st.Running),
	}}
}

// pluginServiceRow is a Verso plugin's service: procd's truth sharpened by
// the socket probe.
func pluginServiceRow(st pluginState, pkgNames map[string]bool) widget.TableRow {
	m := st.Manifest
	pill := widget.TableCell{Text: "stopped", Variant: "neutral"}
	switch {
	case !st.Known:
		pill = widget.TableCell{Text: "not managed", Variant: "warning"}
	case st.Running && st.Alive:
		pill = widget.TableCell{Text: "running", Variant: "success"}
	case st.Running:
		pill = widget.TableCell{Text: "not responding", Variant: "danger"}
	}
	return widget.TableRow{ID: st.Service, Cells: []widget.TableCell{
		{Text: st.Service},
		{Text: packageOf(st.Service, pkgNames)},
		pill,
		{On: st.Enabled},
		{On: st.Running, Name: "on:" + m.ID},
	}}
}
