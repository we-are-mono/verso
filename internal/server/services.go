// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"fmt"
	"html/template"
	"net"
	"net/http"
	"regexp"
	"slices"
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
// human concept (on = enable+start, off = stop+disable), and Verso plugins add a
// liveness probe from their socket.
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
			st.Kind, st.PIDs, st.Memory, st.Uptime = r.Kind, r.PIDs, r.MemoryBytes, r.Uptime
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
	switchRequest := r.Header.Get("X-Verso-Interaction") == "switch"
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
		if switchRequest {
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
		if switchRequest {
			http.Error(w, msg, http.StatusConflict)
			return
		}
		s.renderServices(w, r, msg)
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
			if switchRequest {
				http.Error(w, fmt.Sprintf(tr("Could not %s %s: the device refused (%v)."), a, svc, err), http.StatusBadGateway)
				return
			}
			s.renderServices(w, r, fmt.Sprintf(tr("Could not %s %s: the device refused (%v)."), a, svc, err))
			return
		}
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

// renderServices composes the page: procd's whole table, using the same
// flush-edged table treatment as the rest of Verso; errMsg leads as a danger callout.
func (s *Server) renderServices(w http.ResponseWriter, r *http.Request, errMsg string) {
	sid := s.sessionSID(r)
	rc, rcErr := s.backend.RCList(r.Context(), sid)
	// APK reports the init scripts each package owns. This is the authoritative
	// join: service and package names are often different (verso-rpcd belongs to
	// verso, for example), so matching names would silently discard real owners.
	owners := map[string]string{}
	if pkgs, err := s.backend.PkgInstalled(r.Context(), sid); err == nil {
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
			Body: fmt.Sprintf("procd could not be read (%v).", rcErr)})
	} else {
		children = append(children, &widget.Callout{
			Variant: "warning",
			Title:   "Proceed with care",
			Body:    "Stopping or disabling system services can make OpenWrt unstable or inaccessible. Change only services you understand.",
		})
		children = append(children, servicesTable(s.pluginStates(rc), rc, owners))
	}

	var body strings.Builder
	lang, t := s.localize(r)
	if err := s.widgets.RenderWithToken(&body, &widget.Stack{Children: children}, s.sessionCSRF(r), lang, t); err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	// Immediate acts only (ADR-011 §8) — the staged-changes bar appears here
	// solely when other pages' edits are pending.
	s.renderPage(w, r, http.StatusOK, pageHeader{
		Heading:    "System",
		Subheading: "The processes this router runs — procd's service table, live.",
	}, "wide", s.systemPages(r.URL.Path), false, template.HTML(body.String()))
}

// servicesTable is procd's table, one flush-edged row per service: name and
// lifecycle type, APK-reported package, live state, compact process runtime,
// restart, and the enabled switch. No drawers — every fact and immediate action
// has its own column. The switch represents boot policy; runtime state remains a
// separate fact and is only claimed where procd can establish it.
func servicesTable(states []pluginState, rc map[string]openwrt.RCState, owners map[string]string) widget.Widget {
	cols := []widget.TableColumn{
		{Label: "Service", Kind: "name"},
		{Label: "Package", Kind: "mono"},
		{Label: "State", Kind: "pill"},
		{Label: "Runtime", Kind: "runtime"},
		{Label: "Restart", Kind: "action"},
		{Label: "Enabled", Kind: "toggle"},
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
	sort.Slice(rows, func(a, b int) bool { return rows[a].ID < rows[b].ID })
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

// svcSwitch is the boot-policy cell; keep-listed services and one-shot tasks
// get none. It intentionally reflects Enabled rather than Running: a daemon can
// be enabled but crashed, while an action-based subsystem may have no PID at all.
func svcSwitch(name string, st openwrt.RCState) widget.TableCell {
	if svcMustStayEnabled[name] {
		return widget.TableCell{Icon: "lock", Button: "Firewall must remain enabled"}
	}
	if svcKeep[name] || serviceKind(st.Kind) == openwrt.ServiceTask {
		return widget.TableCell{}
	}
	return widget.TableCell{On: st.Enabled, Name: "svc:" + name}
}

// svcRestart is the immediate restart action. Startup tasks do not have a
// persistent process to restart, and keep-listed services cannot be safely
// interrupted from the surface they sustain.
func svcRestart(name string, kind openwrt.ServiceKind) widget.TableCell {
	if svcKeep[name] || serviceKind(kind) == openwrt.ServiceTask {
		return widget.TableCell{}
	}
	return widget.TableCell{
		Text:   name,
		Name:   "service",
		Action: "restart",
		Button: "Restart " + name,
		Icon:   "refresh-cw",
	}
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
		// A subsystem such as firewall4 can apply persistent kernel state and
		// exit. A false generic `running` result therefore proves nothing; only
		// surface an active state when the init script positively reports it.
		if st.Running {
			pill = widget.TableCell{Text: "active", Variant: "success"}
		}
	}
	return widget.TableRow{ID: name, Cells: []widget.TableCell{
		serviceNameCell(name, kind),
		{Text: packageOf(name, owners), Emphasis: true},
		pill,
		serviceRuntimeCell(st.PIDs, st.MemoryBytes),
		svcRestart(name, kind),
		svcSwitch(name, st),
	}}
}

// pluginServiceRow is a Verso plugin's service: procd's truth sharpened by
// the socket probe.
func pluginServiceRow(st pluginState, owners map[string]string) widget.TableRow {
	m := st.Manifest
	kind := serviceKind(st.Kind)
	restart := svcRestart(st.Service, kind)
	if !st.Known {
		restart = widget.TableCell{}
	}
	pill := widget.TableCell{Muted: true}
	switch {
	case !st.Known:
		pill = widget.TableCell{Text: "not managed", Variant: "warning"}
	case kind == openwrt.ServiceTask:
		pill = widget.TableCell{Text: "disabled", Variant: "neutral"}
		if st.Enabled {
			pill = widget.TableCell{Text: "runs at boot", Variant: "info"}
		}
	case kind == openwrt.ServiceSubsystem:
		if st.Running {
			pill = widget.TableCell{Text: "active", Variant: "success"}
		}
	case st.Running && st.Alive:
		pill = runningStateCell(st.Uptime)
	case st.Running:
		pill = widget.TableCell{Text: "not responding", Variant: "danger"}
	default:
		pill = widget.TableCell{Text: "stopped", Variant: "neutral"}
	}
	return widget.TableRow{ID: st.Service, Cells: []widget.TableCell{
		serviceNameCell(st.Service, kind),
		{Text: packageOf(st.Service, owners), Emphasis: true},
		pill,
		serviceRuntimeCell(st.PIDs, st.Memory),
		restart,
		svcPluginSwitch(m.ID, st, kind),
	}}
}

func svcPluginSwitch(id string, st pluginState, kind openwrt.ServiceKind) widget.TableCell {
	if svcKeep[st.Service] || kind == openwrt.ServiceTask || !st.Known {
		return widget.TableCell{}
	}
	return widget.TableCell{On: st.Enabled, Name: "on:" + id}
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
	text := "running"
	if uptime > 0 {
		text += " for " + compactServiceUptime(uptime)
	}
	return widget.TableCell{Text: text, Variant: "success"}
}

func serviceRuntimeCell(pids []int, memory int64) widget.TableCell {
	if len(pids) == 0 {
		return widget.TableCell{Text: "—", Muted: true}
	}
	parts := make([]string, 0, 2)
	if len(pids) == 1 {
		parts = append(parts, fmt.Sprintf("PID %d", pids[0]))
	} else {
		parts = append(parts, fmt.Sprintf("%d processes", len(pids)))
	}
	if memory > 0 {
		parts = append(parts, humanSize(memory))
	}
	return widget.TableCell{Text: strings.Join(parts, " · ")}
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
