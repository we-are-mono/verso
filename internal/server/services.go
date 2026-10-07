// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/we-are-mono/verso/internal/openwrt"
	"github.com/we-are-mono/verso/internal/plugin"
	"github.com/we-are-mono/verso/internal/widget"
)

// The Services page: the processes this router runs — procd's service table,
// the userspace half of `ps`. A service is a running (or stoppable) thing; a
// package is files on disk — two natures, two pages (the Packages page owns
// the other). Rows expose immediate start, stop and restart actions; boot policy stays
// unchanged. Verso plugins add a liveness probe from their socket.
// Nothing here stages (ADR-010 boundary); a keep-list refuses severing the
// surface itself.

// pluginServicePrefix is the init-script naming convention (ADR-011 §3): the
// package verso-plugin-<id> installs /etc/init.d/verso-plugin-<id>.
const pluginServicePrefix = "verso-plugin-"

// rcActions is the closed set of lifecycle verbs the surface forwards to
// procd; anything else in a POST is rejected before touching the bus.
var rcActions = map[string]bool{"start": true, "stop": true, "restart": true, "reload": true, "enable": true, "disable": true}

// svcReapply names the subsystems (init scripts that apply configuration and
// exit, leaving nothing running) whose live state can drift from their config
// without the config changing, and the words for putting it back. firewall4's
// ruleset can be rewritten by an include script or by another program, and a
// reload rebuilds it from the config in one swap (fw4 reload; a restart
// flushes it first). Every other subsystem, `system` among them, re-runs on
// its own whenever its config changes, so it offers no act at all.
var svcReapply = map[string]string{"firewall": "Reload rules"}

// svcKeep is the set of services this surface refuses to stop or restart:
// severing them severs the surface itself (the shell, its privileged path,
// the bus). Everything else — network included — is the operator's call,
// exactly as it is over SSH.
var svcKeep = map[string]bool{"verso": true, "verso-rpcd": true, "rpcd": true, "ubus": true}

// svcMustStayEnabled covers kernel-state loaders whose stop action is unsafe as
// an everyday service control. firewall4 has no resident daemon: stopping it
// flushes the router's nftables, NAT and forwarding rules. It may be restarted,
// but the Services page never offers or accepts stop/disable.
var svcMustStayEnabled = map[string]bool{"firewall": true}

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
	Kind     openwrt.ServiceKind
	PIDs     []int
	Memory   int64
	Uptime   int64
	Order    *int
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
			st.Kind, st.PIDs, st.Memory, st.Uptime, st.Order = r.Kind, r.PIDs, r.MemoryBytes, r.Uptime, r.Order
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

// handleServicesAction dispatches a lifecycle POST. Row actions return runtime
// cells; legacy switch submissions and ordinary forms retain their response paths.
func (s *Server) handleServicesAction(w http.ResponseWriter, r *http.Request) {
	switchRequest := r.Header.Get("X-Verso-Interaction") == "switch"
	rowRequest := r.Header.Get("X-Verso-Interaction") == "act"
	_, t := s.localize(r)
	tr := translatorOrIdentity(t)
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
		msg := fmt.Sprintf(tr("%s keeps this page alive — manage it over SSH if you really mean it."), svc)
		if switchRequest || rowRequest {
			// A switch POST reads res.ok as success and reloads (verso.js), so a
			// refusal must be an error status, not a 200 re-render — parity with the
			// svcMustStayEnabled branch below.
			http.Error(w, msg, http.StatusConflict)
			return
		}
		s.renderServices(w, r, msg)
		return
	}
	if svcMustStayEnabled[svc] && (slices.Contains(actions, "stop") || slices.Contains(actions, "disable")) {
		msg := fmt.Sprintf(tr("%s must remain enabled; stopping it would remove the router's firewall and forwarding rules."), svc)
		if switchRequest || rowRequest {
			http.Error(w, msg, http.StatusConflict)
			return
		}
		s.renderServices(w, r, msg)
		return
	}
	for _, a := range actions {
		// A reload is offered only where putting the state back means something
		// (svcReapply), so it is accepted only there.
		if !rcActions[a] || (a == "reload" && svcReapply[svc] == "") {
			http.Error(w, "unknown action", http.StatusBadRequest)
			return
		}
	}
	for _, a := range actions {
		if err := s.backend.RCInit(r.Context(), s.sessionSID(r), svc, a); err != nil {
			if switchRequest || rowRequest {
				http.Error(w, fmt.Sprintf(tr("Could not %s %s: the device refused (%v)."), a, svc, err), http.StatusBadGateway)
				return
			}
			s.renderServices(w, r, fmt.Sprintf(tr("Could not %s %s: the device refused (%v)."), a, svc, err))
			return
		}
	}
	if rowRequest {
		s.renderServiceRuntime(w, r, svc)
		return
	}
	s.flash(r, "success", svc+" "+tr(lifecycleWord(actions))+".")
	if switchRequest {
		// A fetch-based switch already reloads the page after this response.
		// Returning 204 avoids following a 303 through one expensive service-list
		// render only to discard it and perform the same render again.
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(w, r, "/system/services", http.StatusSeeOther)
}

// A lifecycle action changes runtime cells only. Identity and APK ownership are
// already on screen; re-reading the package database cannot improve this result.
func (s *Server) renderServiceRuntime(w http.ResponseWriter, r *http.Request, name string) {
	lang, t := s.localize(r)
	tr := translatorOrIdentity(t)
	rc, err := s.backend.RCList(r.Context(), s.sessionSID(r))
	if err != nil {
		http.Error(w, fmt.Sprintf(tr("The action completed, but service status could not be read (%v)."), err), http.StatusBadGateway)
		return
	}
	state, ok := rc[name]
	if !ok {
		http.Error(w, tr("The action completed, but the service is no longer listed."), http.StatusNotFound)
		return
	}
	snapshot := map[string]openwrt.RCState{name: state}
	var plugins []pluginState
	if strings.HasPrefix(name, pluginServicePrefix) {
		for _, ps := range s.pluginStates(snapshot) {
			if ps.Service == name {
				plugins = append(plugins, ps)
			}
		}
	}
	table := servicesTable(plugins, snapshot, nil)
	var body strings.Builder
	if err := s.widgets.RenderWithToken(&body, table, s.sessionCSRF(r), lang, t); err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprintf(w, `<div data-verso-row-patch="3,4,5,6">%s</div>`, body.String())
}

// serviceActionsOf extracts (service, rc actions) from the POSTed form. The
// on/off switch posts under "on:<plugin id>" (resolved through the manifest)
// or "svc:<service>" for plain services, expanding to both procd facts; the
// drawer's action form posts plugin=<id> or service=<name> with _action
// ("" = the primary button, restart).
func serviceActionsOf(r *http.Request, byID func(string) (plugin.Manifest, bool)) (string, []string) {
	if verb, name, ok := strings.Cut(r.PostForm.Get("_service_action"), ":"); ok {
		return name, []string{verb}
	}
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

// installedPackagesForOwnership reads the installed set for the service→package
// join, and answers empty while a feed refresh owns apk.
func (s *Server) installedPackagesForOwnership(ctx context.Context, sid string) ([]openwrt.Package, error) {
	if feedRefresh.running() {
		return nil, nil
	}
	return s.backend.PkgInstalled(ctx, sid)
}

// renderServices composes the page: procd's whole table, using the same
// flush-edged table treatment as the rest of Verso; errMsg leads as a danger callout.
func (s *Server) renderServices(w http.ResponseWriter, r *http.Request, errMsg string) {
	lang, t := s.localize(r)
	tr := translatorOrIdentity(t)
	sid := s.sessionSID(r)
	rc, rcErr := s.backend.RCList(r.Context(), sid)
	// APK reports the init scripts each package owns. This is the authoritative
	// join: service and package names are often different (verso-rpcd belongs to
	// verso, for example), so matching names would silently discard real owners.
	// The read waits on the helper's package guard, which a running feed refresh
	// holds for as long as apk takes — and ownership is an enrichment, not the
	// page, so it is left out rather than made worth waiting for.
	owners := map[string]string{}
	if pkgs, err := s.installedPackagesForOwnership(r.Context(), sid); err == nil {
		for _, p := range pkgs {
			for _, service := range p.Services {
				owners[service] = p.Name
			}
		}
		// A successful helper round-trip is stronger evidence of its live state
		// than rc.list on systems where the daemon was launched outside procd.
		if st, ok := rc["verso-rpcd"]; ok {
			st.Running = true
			rc["verso-rpcd"] = st
		}
	}

	children := []widget.Widget{}
	if errMsg != "" {
		children = append(children, &widget.Callout{Variant: "danger", Title: "Action failed", Body: errMsg})
	}
	if rcErr != nil {
		children = append(children, &widget.Callout{Variant: "warning", Title: "Service table unavailable",
			Body: fmt.Sprintf(tr("procd could not be read (%v)."), rcErr)})
	} else {
		table := servicesTable(s.pluginStates(rc), rc, owners).(*widget.Table)
		for i := range table.Rows {
			row := &table.Rows[i]
			// The service identity stays verbatim; its lifecycle classification
			// is a label, even though generic table chips normally hold data.
			for j := range row.Cells[1].Chips {
				row.Cells[1].Chips[j].Label = tr(row.Cells[1].Chips[j].Label)
			}
		}
		// A still listing is scrolled and found in with the browser's own
		// find, so it stands under the heading with no control band.
		children = append(children, table)
	}

	var body strings.Builder
	if err := s.widgets.RenderWithToken(&body, &widget.Stack{Children: children}, s.sessionCSRF(r), lang, t); err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	// Immediate acts only (ADR-011 §8) — the staged-changes bar appears here
	// solely when other pages' edits are pending.
	s.renderPage(w, r, http.StatusOK, pageHeader{
		Heading: "Services", Tone: "neutral",
	}, "wide", s.sectionPages("System", r.URL.Path), template.HTML(body.String()))
}

// servicesTable is procd's table, one flush-edged row per service: name and
// lifecycle type, APK-reported package, live state, compact process runtime,
// restart, and start/stop. No drawers — every fact and immediate action has its
// own column. Runtime state is only claimed where procd can establish it.
func servicesTable(states []pluginState, rc map[string]openwrt.RCState, owners map[string]string) widget.Widget {
	cols := []widget.TableColumn{
		{Label: "Order", Kind: "mono", Width: widget.MeasureCount},
		{Label: "Service", Kind: "reference", Width: widget.MeasureLong},
		{Label: "Package", Kind: "mono", Width: widget.MeasureName},
		{Label: "PID", Kind: "mono", Width: widget.MeasureShort},
		{Label: "Memory", Kind: "runtime", Width: widget.MeasureShort},
		{Label: "State", Kind: "status"},
		{Kind: "actions", Width: widget.MeasureShort},
	}
	byService := make(map[string]pluginState, len(states))
	for _, st := range states {
		byService[st.Service] = st
	}
	rows := make([]widget.TableRow, 0, len(rc))
	for name, st := range rc {
		if ps, ok := byService[name]; ok {
			rows = append(rows, pluginServiceRow(ps, owners))
			continue
		}
		rows = append(rows, serviceRow(name, st, owners))
	}
	// A dev-deployed plugin procd cannot see still shows, honestly unmanaged.
	for _, ps := range states {
		if _, ok := rc[ps.Service]; !ok {
			rows = append(rows, pluginServiceRow(ps, owners))
		}
	}
	sort.Slice(rows, func(a, b int) bool {
		x, y := rc[rows[a].ID].Order, rc[rows[b].ID].Order
		if x != nil && y != nil && *x != *y {
			return *x < *y
		}
		if (x == nil) != (y == nil) {
			return x != nil
		}
		return rows[a].ID < rows[b].ID
	})
	return &widget.Table{Style: "flat", Columns: cols, Rows: rows}
}

// packageOf is the Package column: APK file ownership first, then the two
// development-deploy conventions whose files are copied rather than installed.
func packageOf(service string, owners map[string]string) string {
	if owner := owners[service]; owner != "" {
		return owner
	}
	if service == "verso" || service == "verso-rpcd" {
		return "verso"
	}
	if strings.HasPrefix(service, pluginServicePrefix) {
		return service
	}
	return "—"
}

// serviceRow is one plain procd service.
func serviceRow(name string, st openwrt.RCState, owners map[string]string) widget.TableRow {
	kind := serviceKind(st.Kind)
	pill := widget.TableCell{Muted: true}
	switch kind {
	case openwrt.ServiceTask:
		pill.Text = "disabled"
		pill.Variant = "neutral"
		if st.Enabled {
			pill = widget.TableCell{Text: "runs at boot", Variant: "info"}
		}
	case openwrt.ServiceDaemon:
		pill = widget.TableCell{Text: "stopped", Variant: "neutral"}
		if st.Running {
			pill = runningStateCell(st.Uptime)
		}
	case openwrt.ServiceSubsystem:
		pill = subsystemStateCell(st.Running, st.Enabled)
	}
	return serviceListingRow(name, st, pill, owners, true)
}

// subsystemStateCell is a subsystem's state. A subsystem such as firewall4
// applies persistent kernel state and exits, so a false generic `running`
// proves nothing: it is active only where the init script positively says so,
// and otherwise what it did, applied at boot, or that it does not run.
func subsystemStateCell(running, enabled bool) widget.TableCell {
	switch {
	case running:
		return widget.TableCell{Text: "active", Variant: "success"}
	case enabled:
		return widget.TableCell{Text: "applied at boot", Variant: "info"}
	default:
		return widget.TableCell{Text: "disabled", Variant: "neutral"}
	}
}

// pluginServiceRow is a Verso plugin's service: procd's truth sharpened by
// the socket probe.
func pluginServiceRow(st pluginState, owners map[string]string) widget.TableRow {
	kind := serviceKind(st.Kind)
	var pill widget.TableCell
	switch {
	case !st.Known:
		pill = widget.TableCell{Text: "not managed", Variant: "warning"}
	case kind == openwrt.ServiceTask:
		pill = widget.TableCell{Text: "disabled", Variant: "neutral"}
		if st.Enabled {
			pill = widget.TableCell{Text: "runs at boot", Variant: "info"}
		}
	case kind == openwrt.ServiceSubsystem:
		pill = subsystemStateCell(st.Running, st.Enabled)
	case st.Running && st.Alive:
		pill = runningStateCell(st.Uptime)
	case st.Running:
		pill = widget.TableCell{Text: "not responding", Variant: "danger"}
	default:
		pill = widget.TableCell{Text: "stopped", Variant: "neutral"}
	}
	return serviceListingRow(st.Service, openwrt.RCState{Kind: kind, Enabled: st.Enabled, Running: st.Running, PIDs: st.PIDs, MemoryBytes: st.Memory, Uptime: st.Uptime, Order: st.Order}, pill, owners, st.Known)
}

func serviceKind(kind openwrt.ServiceKind) openwrt.ServiceKind {
	if kind == "" {
		return openwrt.ServiceDaemon
	}
	return kind
}

func serviceNameCell(name string, kind openwrt.ServiceKind) widget.TableCell {
	icon := "server"
	if kind == openwrt.ServiceSubsystem {
		icon = "settings"
	} else if kind == openwrt.ServiceTask {
		icon = "clock"
	}
	return widget.TableCell{Text: name, Chips: []widget.TableChip{{Icon: icon, Label: string(kind)}}}
}

func runningStateCell(uptime int64) widget.TableCell {
	cell := widget.TableCell{Text: "Running", Variant: "success"}
	if uptime > 0 {
		cell.Sub = compactServiceUptime(uptime)
	}
	return cell
}

func serviceListingRow(name string, st openwrt.RCState, state widget.TableCell, owners map[string]string, managed bool) widget.TableRow {
	order, pid, memory := "—", "—", "—"
	if st.Order != nil {
		order = fmt.Sprint(*st.Order)
	}
	if len(st.PIDs) == 1 {
		pid = fmt.Sprint(st.PIDs[0])
	} else if len(st.PIDs) > 1 {
		pids := make([]string, len(st.PIDs))
		for i, value := range st.PIDs {
			pids[i] = strconv.Itoa(value)
		}
		pid = strings.Join(pids, ", ")
	}
	if st.MemoryBytes > 0 {
		memory = humanSize(st.MemoryBytes)
	}
	kind := serviceKind(st.Kind)
	acts := []widget.TableRowAct{}
	if managed && kind == openwrt.ServiceSubsystem {
		// Nothing runs, so nothing starts or stops: only a subsystem whose
		// state can drift is offered putting it back.
		if label, ok := svcReapply[name]; ok {
			acts = append(acts, widget.TableRowAct{Icon: "refresh-cw", Title: label, Name: "_service_action", Value: "reload:" + name})
		}
	} else if managed && kind != openwrt.ServiceTask {
		if !svcKeep[name] {
			acts = append(acts, widget.TableRowAct{Icon: "refresh-cw", Title: "Restart", Name: "_service_action", Value: "restart:" + name})
		}
		if svcKeep[name] || svcMustStayEnabled[name] {
			acts = append(acts, widget.TableRowAct{Icon: "lock", Title: "Cannot be stopped from here"})
		} else {
			verb, label := "start", "Start"
			if st.Running {
				verb, label = "stop", "Stop"
			}
			acts = append(acts, widget.TableRowAct{Icon: "power", Title: label, Name: "_service_action", Value: verb + ":" + name})
		}
	}
	if state.Text == "stopped" {
		state.Text = "Stopped"
		state.Variant = ""
	}
	if state.Text == "" {
		state.Text = "—"
	}
	return widget.TableRow{ID: name, Cells: []widget.TableCell{
		{Text: order}, serviceNameCell(name, kind), {Text: packageOf(name, owners)},
		{Text: pid}, {Text: memory}, state, {Actions: acts},
	}}
}

func compactServiceUptime(seconds int64) string {
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	minutes := seconds / 60
	if minutes < 60 {
		return fmt.Sprintf("%dm %ds", minutes, seconds%60)
	}
	hours := minutes / 60
	if hours < 24 {
		return fmt.Sprintf("%dh %dm", hours, minutes%60)
	}
	return fmt.Sprintf("%dd %dh", hours/24, hours%24)
}
