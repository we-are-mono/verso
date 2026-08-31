// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/we-are-mono/verso/internal/datatype"
	"github.com/we-are-mono/verso/internal/plugin"
	"github.com/we-are-mono/verso/internal/widget"
)

// supportedSchemaVersion is the widget-schema vocabulary this shell renders. A
// plugin declaring another version degrades to a notice (ADR-006 §6), never a
// crash.
const supportedSchemaVersion = 1

// handlePlugin is the schema gateway (ADR-006 §3): it fetches a widget schema
// from the plugin's socket and renders it through the shell's own chrome and
// tokens. It is NOT a reverse proxy — plugin bytes are data the shell renders,
// never markup streamed to the browser.
func (s *Server) handlePlugin(w http.ResponseWriter, r *http.Request) {
	m, ok := s.manifestByID(r.PathValue("id"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	hdr := pageHeader{Heading: m.Name}
	width := ""
	var pages []pageTab
	body, status := s.pluginBodyAt(r, m, r.PathValue("path"), &hdr, &width, &pages)
	// A plugin filing a page into System joins the shell's mixed-ownership
	// System frame. The manifest registration, not a shell route, supplies the
	// page and its label; stopped plugins disappear through the ordinary live
	// registration filter used by every other plugin page.
	if pluginNavSectionAt(m, r.PathValue("path")) == "System" {
		hdr.Heading = "System"
		pages = s.systemPages(r.URL.Path)
	}
	// Configuration pages keep the staging capsule at rest. A page made only of
	// immediate commands may omit the clean capsule; an existing stage still
	// follows the operator here as shared state.
	s.renderPage(w, r, status, hdr, width, pages, !hdr.Immediate, body)
}

// pluginBody returns the rendered page body for a plugin request, or a contained
// notice. Every plugin-side failure — unsupported schema version, transport
// error, undecodable schema, render error — resolves to a styled notice and a
// 200, so a third-party plugin can never 500 the shell or take it down
// (ADR-006 §7). The shell's own failures (e.g. the page template) still 500.
// pluginBody returns the rendered body and the HTTP status to send the browser.
// Notices (version mismatch, unavailable) are 200 — the shell is fine, it is
// just reporting. A plugin's own 422 (a validation failure) is propagated, so
// the HTTP semantics stay honest; everything else is 200.
func (s *Server) pluginBodyAt(r *http.Request, m plugin.Manifest, pluginPath string, hdr *pageHeader, width *string, pages *[]pageTab) (template.HTML, int) {
	// The request's language, negotiated once. This is a plugin render, so the
	// translator is the plugin's catalog overlaid on base (ADR-012 §5): t localizes
	// the widget tree at render, and tr localizes the envelope the shell copies out
	// (heading, subheading, subpage labels) plus the notices this gateway may return
	// — the plugin's own text from its catalog, shell-owned notices from base.
	lang, t := s.pluginLocalize(r, m.ID)
	tr := translatorOrIdentity(t)
	if m.SchemaVersion != supportedSchemaVersion {
		return s.notice(tr("Plugin needs a newer Verso"), fmt.Sprintf(
			tr("%s speaks schema version %d; this shell supports version %d."),
			m.Name, m.SchemaVersion, supportedSchemaVersion)), http.StatusOK
	}

	// The shell is the enforcement point for plugin writes (ADR-007): a
	// state-changing request is refused here unless the session's rpcd ACLs cover
	// the plugin's declared write scopes, so the plugin — which holds no session —
	// never sees a write the operator is not authorized for.
	if !safeMethod(r.Method) {
		switch allowed, err := s.authorizePluginWrite(r.Context(), m, s.sessionSID(r)); {
		case err != nil:
			log.Printf("verso: plugin %q permission check failed: %v", m.ID, err)
			return s.notice(tr("Permission check unavailable"),
				tr("Verso couldn’t verify your permissions just now. Try again in a moment.")), http.StatusServiceUnavailable
		case !allowed:
			return s.notice(tr("Not permitted"),
				fmt.Sprintf(tr("Your account isn’t permitted to change %s."), m.Name)), http.StatusForbidden
		}
	}

	// Parse a state-changing request's form up front, so a repeater structural op
	// can be read and, failing that, the form can be forwarded to the plugin.
	method := r.Method
	if !safeMethod(method) {
		if err := r.ParseForm(); err == nil {
			r.PostForm.Del("_csrf") // the shell's CSRF token is not the plugin's business
		}
		// A repeater's add/remove is the shell's to realize, not the plugin's
		// (ADR-005 §7): the shell performs the uci section add/delete through rpcd,
		// then re-renders the fresh state as a read. The plugin ships no add/remove
		// logic — it only declared the repeater.
		if r.PostForm.Get(widget.RepeaterOpField) != "" {
			if body, st, ok := s.realizeRepeater(r.Context(), m, s.sessionSID(r), r.PostForm, tr); !ok {
				return body, st
			}
			method = http.MethodGet // render current state; carry no form, run no commit
		}
	}

	req := plugin.Request{Method: method, Path: pluginPath, Query: r.URL.Query()}
	if !safeMethod(method) {
		req.Form = r.PostForm
	}
	// The shell brokers the plugin's reads (ADR-007): it reads each config the plugin
	// declared in acl.read with the operator's sid and hands the plugin a snapshot,
	// so a session-less plugin never touches /etc/config itself. Read after any
	// repeater op, so the re-render reflects the structural change.
	req.UCI = s.readSnapshot(r.Context(), m, s.sessionSID(r))

	env, err := s.transport.Fetch(r.Context(), m.Socket, req)
	if err != nil {
		log.Printf("verso: plugin %q unavailable: %v", m.ID, err)
		return s.unavailable(m, tr), http.StatusOK
	}

	wdg, err := widget.Decode(env.Widget)
	if err != nil {
		log.Printf("verso: plugin %q returned undecodable schema: %v", m.ID, err)
		return s.unavailable(m, tr), http.StatusOK
	}

	status := http.StatusOK
	if env.Status == http.StatusUnprocessableEntity {
		status = http.StatusUnprocessableEntity
	}

	// On a state-changing request the shell enforces the declared datatypes on the
	// returned schema (ADR-008): any failure annotates the widget in place, forces
	// 422, and blocks the write — merged with whatever the plugin already flagged.
	// Only a clean submission reaches brokerStage, which stages the plugin's
	// commit intent through rpcd (ADR-007, ADR-010) — nothing is live until the
	// capsule applies. A repeater op was downgraded to a render above, so it skips
	// this — its write already went through rpcd.
	if !safeMethod(method) {
		if validateSchema(wdg) {
			status = http.StatusUnprocessableEntity
		} else {
			if err := validateApplyActions(m, env.Apply); err != nil {
				log.Printf("verso: plugin %q returned an invalid apply action: %v", m.ID, err)
				return s.notice(tr("Not permitted"), fmt.Sprintf(
					tr("%s tried to perform an operation it did not declare."), m.Name)), http.StatusForbidden
			}
			if len(env.Commit) > 0 {
				if body, st, ok := s.brokerStage(r.Context(), m, s.sessionSID(r), env.Commit, tr); !ok {
					return body, st
				}
			}
			s.setPendingApply(s.sessionSID(r), env.Apply)
		}
	}

	var b strings.Builder
	if err := s.widgets.RenderWithToken(&b, wdg, s.sessionCSRF(r), lang, t); err != nil {
		log.Printf("verso: plugin %q render failed: %v", m.ID, err)
		return s.unavailable(m, tr), http.StatusOK
	}
	if env.Title != "" {
		hdr.Heading = env.Title
	}
	// Localize the envelope with the plugin's translator here, at the point the
	// shell copies it out of the schema. renderPage re-runs the base translator over
	// these, which is a no-op on an already-translated value (a translation is never
	// an English base key), so the plugin's text survives (ADR-012 §5).
	hdr.Heading = tr(hdr.Heading)
	hdr.Kicker = tr(env.Kicker)
	hdr.KickerStatus = tr(env.KickerStatus)
	hdr.Immediate = env.Immediate
	hdr.Live = env.Live
	hdr.Subheading = tr(env.Subheading)
	hdr.Banner = localizeBanner(env.Banner, tr)
	*width = env.Width
	// The subpage tabs carry the plugin id, so renderPage localizes their labels
	// from the plugin's catalog (ADR-012 §5) — no need to pre-translate here.
	*pages = subpageTabsAt(m, pluginPath, env.Pages)
	return template.HTML(b.String()), status
}

// subpageTabs builds the top bar (the third navigation tier) from a plugin's
// declared subpages. Paths are relative to the plugin's mount — the shell
// builds every href and marks the active tab from the request, so the bar can
// never point outside the plugin.
func subpageTabsAt(m plugin.Manifest, pluginPath string, declared []plugin.PageTab) []pageTab {
	if len(declared) == 0 {
		return nil
	}
	cur := strings.Trim(pluginPath, "/")
	tabs := make([]pageTab, 0, len(declared))
	for _, p := range declared {
		rel := strings.Trim(p.Path, "/")
		href := "/plugins/" + m.ID + "/"
		if rel != "" {
			href += rel
		}
		tabs = append(tabs, pageTab{Label: p.Label, Href: href, Active: cur == rel, PluginID: m.ID})
	}
	return tabs
}

// validateSchema walks the widget tree and enforces each field's and list item's
// declared datatype against its value (ADR-008), annotating any failure in place.
// It does not overwrite an error a plugin already set — a semantic message is more
// specific — and reports whether the tree carries any error after the walk, so the
// caller blocks the write and re-renders as 422.
func validateSchema(w widget.Widget) bool {
	switch n := w.(type) {
	case *widget.Card:
		found := false
		for _, c := range n.Children {
			found = validateSchema(c) || found
		}
		return found
	case *widget.Stack:
		found := false
		for _, c := range n.Children {
			found = validateSchema(c) || found
		}
		return found
	case *widget.Grid:
		found := false
		for _, c := range n.Children {
			found = validateSchema(c) || found
		}
		return found
	case *widget.Section:
		found := n.Control != nil && validateSchema(n.Control)
		for _, c := range n.Children {
			found = validateSchema(c) || found
		}
		return found
	case *widget.Form:
		found := n.Error != ""
		for _, f := range n.Fields {
			found = validateSchema(f) || found
		}
		return found
	case *widget.Field:
		if n.Error == "" && n.Datatype != "" {
			if err := datatype.Validate(n.Datatype, n.Value); err != nil {
				n.Error = err.Error()
			}
		}
		return n.Error != ""
	case *widget.List:
		if n.Datatype != "" {
			for i, item := range n.Items {
				key := strconv.Itoa(i)
				if _, has := n.Errors[key]; has {
					continue
				}
				if err := datatype.Validate(n.Datatype, item); err != nil {
					if n.Errors == nil {
						n.Errors = map[string]string{}
					}
					n.Errors[key] = err.Error()
				}
			}
		}
		return len(n.Errors) > 0
	case *widget.Repeater:
		found := false
		for _, it := range n.Items {
			found = validateSchema(it.Widget) || found
		}
		return found
	case *widget.Conditional:
		found := false
		for _, f := range n.Fields {
			found = validateSchema(f) || found
		}
		for _, f := range n.Otherwise {
			found = validateSchema(f) || found
		}
		return found
	default:
		return false
	}
}

// validateApplyActions bounds the privileged tail of a plugin transaction to
// named operations and to scopes the plugin declared in its manifest. Adding a
// helper method is not enough: a plugin must opt into its exact permission.
func validateApplyActions(m plugin.Manifest, actions []plugin.ApplyAction) error {
	if len(actions) > 1 {
		return fmt.Errorf("too many apply actions")
	}
	for _, action := range actions {
		var required plugin.ACLScope
		switch action.Name {
		case "set-system-time":
			required = plugin.ACLScope{Scope: "ubus", Object: "verso", Function: "setSystemTime"}
			if action.Args["datetime"] == "" || action.Args["timezone"] == "" {
				return fmt.Errorf("set-system-time needs datetime and timezone")
			}
		default:
			return fmt.Errorf("unknown action %q", action.Name)
		}
		declared := false
		for _, scope := range m.ACL.Write {
			if scope == required {
				declared = true
				break
			}
		}
		if !declared {
			return fmt.Errorf("action %q lacks declared scope", action.Name)
		}
	}
	return nil
}

// setPendingApply records the non-UCI apply tail a plugin POST prepared, keyed by
// the operator's session so one operator's tail can never fire under another's
// Save & Apply. Actions merge by name within the session (a re-save replaces its
// own), and an empty set is a no-op — an unrelated save on another page must not
// wipe a tail already armed for this session.
func (s *Server) setPendingApply(sid string, actions []plugin.ApplyAction) {
	if sid == "" || len(actions) == 0 {
		return
	}
	s.pendingApplyMu.Lock()
	defer s.pendingApplyMu.Unlock()
	if s.pendingApply == nil {
		s.pendingApply = make(map[string][]plugin.ApplyAction)
	}
	merged := s.pendingApply[sid]
	for _, action := range actions {
		replaced := false
		for i := range merged {
			if merged[i].Name == action.Name {
				merged[i] = action
				replaced = true
				break
			}
		}
		if !replaced {
			merged = append(merged, action)
		}
	}
	s.pendingApply[sid] = merged
}

// takePendingApply returns the session's apply tail and clears it in one step, so
// a drained (or failed) action can never linger to fire on a later, unrelated apply.
func (s *Server) takePendingApply(sid string) []plugin.ApplyAction {
	s.pendingApplyMu.Lock()
	defer s.pendingApplyMu.Unlock()
	actions := s.pendingApply[sid]
	delete(s.pendingApply, sid)
	return actions
}

func (s *Server) clearPendingApply(sid string) {
	s.pendingApplyMu.Lock()
	defer s.pendingApplyMu.Unlock()
	delete(s.pendingApply, sid)
}

type noticeData struct {
	Title, Message          string
	ActionLabel, ActionHref string // optional door out of the dead end
}

// notice renders a shell-owned message (not plugin content) through the shell's
// tokens. Fields flow through html/template, so an untrusted plugin name in the
// message is escaped.
func (s *Server) notice(title, message string) template.HTML {
	return s.noticeWith(noticeData{Title: title, Message: message})
}

func (s *Server) noticeWith(d noticeData) template.HTML {
	var b bytes.Buffer
	// The notice template carries no {{ t }} chrome of its own — its text arrives
	// already localized in d (wrapped at the call site) — so the English set renders
	// it correctly in any language.
	if err := s.pageSet("").ExecuteTemplate(&b, "notice.html.tmpl", d); err != nil {
		return template.HTML(template.HTMLEscapeString(d.Title + ": " + d.Message))
	}
	return template.HTML(b.String())
}

// unavailable is the honest dead-plugin page — and a door, not a wall: the
// commonest cause is the plugin being turned off, and the management surface
// (ADR-011) is where it turns back on.
func (s *Server) unavailable(m plugin.Manifest, tr func(string) string) template.HTML {
	return s.noticeWith(noticeData{
		Title: tr("Plugin unavailable"),
		Message: fmt.Sprintf(
			tr("%s isn’t responding right now — it may be turned off. The rest of Verso is unaffected."), m.Name),
		ActionLabel: tr("Open Packages"),
		ActionHref:  "/system/packages",
	})
}

// safeMethod reports whether the HTTP method is read-only, and so neither
// ACL-gated nor CSRF-checked. It mirrors the CSRF gate's notion of a safe method.
func safeMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead
}

// authorizePluginWrite reports whether the session may perform the writes the
// plugin declares. A plugin that declares no write scopes may not receive a
// state-changing request (fail closed — its declared write surface is empty);
// otherwise every declared scope must be granted by rpcd, and any single denial
// refuses the whole request. A transport error is returned, not swallowed, so the
// caller fails closed rather than dispatching an unauthorized write.
func (s *Server) authorizePluginWrite(ctx context.Context, m plugin.Manifest, sid string) (bool, error) {
	if len(m.ACL.Write) == 0 {
		return false, nil
	}
	for _, a := range m.ACL.Write {
		allowed, err := s.backend.Access(ctx, sid, a.Scope, a.Object, a.Function)
		if err != nil {
			return false, err
		}
		if !allowed {
			return false, nil
		}
	}
	return true, nil
}

// brokerStage performs, through rpcd and on the operator's behalf, the uci writes
// a plugin requested (ADR-007) — into UCI's stage, never committed here
// (ADR-010): the staged-changes capsule owns apply and discard. It refuses any
// op whose config the plugin did not declare in its manifest acl — a plugin
// cannot broker a write outside its declared surface — and rpcd re-checks the
// operator's sid on every call. On refusal or failure it returns a contained
// notice and a status with ok=false; on success ok is true and the caller
// renders the plugin's returned widget.
func (s *Server) brokerStage(ctx context.Context, m plugin.Manifest, sid string, ops []plugin.CommitOp, tr func(string) string) (template.HTML, int, bool) {
	declared := declaredUCIConfigs(m)
	for _, op := range ops {
		if op.Config == "" || !declared[op.Config] {
			log.Printf("verso: plugin %q tried to write undeclared uci config %q; refused", m.ID, op.Config)
			return s.notice(tr("Not permitted"), fmt.Sprintf(
				tr("%s tried to change settings it did not declare."), m.Name)), http.StatusForbidden, false
		}
		// An op with no section and a type creates the section first (through
		// rpcd, staged like the set): the "drawer first, row on save" flow —
		// a plugin never adds bare sections it then has to chase.
		section := op.Section
		if section == "" && op.Type != "" {
			created, err := s.backend.UCIAdd(ctx, sid, op.Config, op.Type)
			if err != nil {
				log.Printf("verso: plugin %q section create in uci %q failed: %v", m.ID, op.Config, err)
				return s.notice(tr("Save failed"),
					tr("The change couldn’t be saved just now. Try again in a moment.")), http.StatusServiceUnavailable, false
			}
			section = created
		}
		if err := s.backend.UCISet(ctx, sid, op.Config, section, op.Values); err != nil {
			log.Printf("verso: plugin %q write to uci %q failed: %v", m.ID, op.Config, err)
			return s.notice(tr("Save failed"),
				tr("The change couldn’t be saved just now. Try again in a moment.")), http.StatusServiceUnavailable, false
		}
	}
	return "", 0, true
}

// realizeRepeater performs a repeater's structural change on the operator's behalf
// (ADR-005 §7): a uci section add or delete through rpcd, staged like every other
// write (ADR-010). It refuses a config the plugin did not declare in its
// acl.write — the same surface brokerStage bounds — and rpcd re-checks the
// operator's sid. On success the caller re-renders the fresh state; a failure is
// a contained notice, never a crash.
func (s *Server) realizeRepeater(ctx context.Context, m plugin.Manifest, sid string, form url.Values, tr func(string) string) (template.HTML, int, bool) {
	config := form.Get(widget.RepeaterConfigField)
	if config == "" || !declaredUCIConfigs(m)[config] {
		log.Printf("verso: plugin %q repeater op on undeclared uci config %q; refused", m.ID, config)
		return s.notice(tr("Not permitted"), fmt.Sprintf(
			tr("%s tried to change settings it did not declare."), m.Name)), http.StatusForbidden, false
	}

	var err error
	switch op := form.Get(widget.RepeaterOpField); op {
	case widget.RepeaterOpAdd:
		secType := form.Get(widget.RepeaterTypeField)
		if secType == "" {
			return s.malformedRepeater(m, tr)
		}
		_, err = s.backend.UCIAdd(ctx, sid, config, secType)
	case widget.RepeaterOpRemove:
		section := form.Get(widget.RepeaterSectionField)
		if section == "" {
			return s.malformedRepeater(m, tr)
		}
		err = s.backend.UCIDelete(ctx, sid, config, section)
	default:
		log.Printf("verso: plugin %q unknown repeater op %q; refused", m.ID, op)
		return s.malformedRepeater(m, tr)
	}
	if err != nil {
		log.Printf("verso: plugin %q repeater op on uci %q failed: %v", m.ID, config, err)
		return s.notice(tr("Save failed"),
			tr("The change couldn’t be saved just now. Try again in a moment.")), http.StatusServiceUnavailable, false
	}
	return "", 0, true
}

// malformedRepeater is the contained response to a repeater affordance that posted
// without the fields the shell needs to act — a client-side problem, not the
// operator's, so it is a plain notice.
func (s *Server) malformedRepeater(m plugin.Manifest, tr func(string) string) (template.HTML, int, bool) {
	return s.notice(tr("Couldn’t apply that change"), fmt.Sprintf(
		tr("Verso couldn’t apply that change to %s."), m.Name)), http.StatusBadRequest, false
}

// declaredUCIConfigs is the set of uci configs a plugin declared it may write in
// its manifest acl (scope "uci"). It bounds what the shell will broker for it.
func declaredUCIConfigs(m plugin.Manifest) map[string]bool {
	out := make(map[string]bool)
	for _, a := range m.ACL.Write {
		if a.Scope == "uci" {
			out[a.Object] = true
		}
	}
	return out
}

// readSnapshot brokers the plugin's reads (ADR-007): it reads each uci config the
// plugin declared in acl.read with the operator's sid and returns them as the
// snapshot the shell injects into the plugin request. It never fails the request —
// reads are not gated by the shell (rpcd scopes them to the operator), and a read
// that errors contributes nothing, degrading to an empty page section rather than
// a 500. Returns nil when the plugin declares no reads, so no snapshot is sent.
func (s *Server) readSnapshot(ctx context.Context, m plugin.Manifest, sid string) plugin.UCI {
	configs := declaredUCIReadConfigs(m)
	if len(configs) == 0 {
		return nil
	}
	snapshot := make(plugin.UCI, len(configs))
	for _, config := range configs {
		values, err := s.backend.UCIConfig(ctx, sid, config)
		if err != nil {
			log.Printf("verso: plugin %q read of uci %q failed: %v", m.ID, config, err)
			continue
		}
		snapshot[config] = values
	}
	return snapshot
}

// declaredUCIReadConfigs is the ordered, de-duplicated set of uci configs a plugin
// declared it reads (manifest acl.read, scope "uci"). It bounds what the shell
// pre-reads and brokers as the plugin's snapshot.
func declaredUCIReadConfigs(m plugin.Manifest) []string {
	var out []string
	seen := make(map[string]bool)
	for _, a := range m.ACL.Read {
		if a.Scope == "uci" && !seen[a.Object] {
			seen[a.Object] = true
			out = append(out, a.Object)
		}
	}
	return out
}
