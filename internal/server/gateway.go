// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/we-are-mono/verso/internal/datatype"
	"github.com/we-are-mono/verso/internal/openwrt"
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
	// The panel is closing on its own outcome: the frame is told to swap
	// nothing, and what it is handed instead is the outcome, to say where the
	// panel was.
	if hdr.PanelDone {
		w.Header().Set("HX-Reswap", "none")
	}
	// One panel's contents, asked for by a frame that is already on screen.
	// Nothing around it has changed, so nothing around it is sent. A live
	// preview answers the same way and for the same reason.
	if hdr.PanelOnly || hdr.PreviewOnly || hdr.PanelDone {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, string(body))
		return
	}
	// A panel's own submission that the panel did not survive — a delete, or a
	// rule made, which change which rows the listing has — is answered with the
	// page, and the frame has nothing to hold: a page swapped into a panel would
	// be the listing nested inside its own drawer. The answer is where the page
	// went, so the frame is told to go there, and the outcome waits on that page
	// as its flash — exactly what a native submit would have landed on. A
	// contained failure (a stage refused or failed) keeps its status and its
	// notice instead: the frame shows that where the person is, rather than
	// sending them away from what they typed.
	if panelRequest(r) && !safeMethod(r.Method) && (status < http.StatusBadRequest || status == http.StatusUnprocessableEntity) {
		if hdr.Notice != nil && hdr.Notice.Text != "" {
			s.flash(r, hdr.Notice.Level, hdr.Notice.Text)
		}
		destination := r.URL.Path
		if hdr.Back != nil && hdr.Back.Href != "" && status < http.StatusBadRequest {
			destination = hdr.Back.Href
		}
		w.Header().Set("HX-Redirect", destination)
		w.WriteHeader(http.StatusOK)
		return
	}
	// A record editor's submit that staged its one change goes back to the listing
	// it came from: the person sees the new row land there, in the pending set,
	// and applies from the review drawer. Back is the editor signal and the
	// return address; a listing's
	// inline toggle stages too, but sets no Back, so it is synced in place instead.
	// The outcome is already composed and localized (stagedOutcome); the
	// destination render's own translator leaves a sentence that is not a base
	// key alone.
	if !safeMethod(r.Method) && (hdr.StagedCommit || hdr.CommandDone) && hdr.Back != nil && hdr.Back.Href != "" {
		s.flash(r, hdr.Notice.Level, hdr.Notice.Text)
		http.Redirect(w, r, hdr.Back.Href, http.StatusSeeOther)
		return
	}
	// A plugin filing a page into System joins the shell's mixed-ownership
	// System frame. The manifest registration, not a shell route, supplies the
	// page and its label; stopped plugins disappear through the ordinary live
	// registration filter used by every other plugin page.
	if pluginNavSectionAt(m, r.PathValue("path")) == "System" {
		if hdr.Tone != "neutral" {
			hdr.Heading = "System"
		}
		pages = s.systemPages(r.URL.Path, readerMode(r))
	}
	s.renderPage(w, r, status, hdr, width, pages, body)
}

// panelRequest reports whether this visit is a frame asking for its contents
// rather than a browser asking for a page — a reading swapped into an open
// panel, or the panel's own form submitted from inside it. htmx marks its own
// requests; a plain visit or a native submit to the same address is a page and
// is answered as one, which is what keeps the panel linkable and what a browser
// with no script still gets.
func panelRequest(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

// panelFlash is a plugin's outcome as a panel carries it — the notice the page
// would have shown in its flash slot. A render with no notice carries nothing.
func panelFlash(n *plugin.Notice) widget.Flash {
	if n == nil {
		return widget.Flash{}
	}
	return widget.Flash{Variant: n.Level, Message: n.Text}
}

// restructures reports whether a stage changed which sections a config holds —
// one made, one removed — rather than the values of sections it already had. A
// listing drawn from that config gains or loses a row, which the page has to be
// drawn again to show.
func restructures(ops []plugin.CommitOp) bool {
	for _, op := range ops {
		if op.Delete || op.Type != "" {
			return true
		}
	}
	return false
}

// stagedOutcome is what a submission that changed the stage says about itself:
// the plugin's half, what happened to what, and the shell's half, what that
// means in the staged model. The model is the shell's, so no plugin has to know
// about applying; a plugin that said nothing gets the plain word.
func stagedOutcome(n *plugin.Notice, tr func(string) string) *plugin.Notice {
	level, text := "success", tr("Saved.")
	if n != nil && n.Text != "" {
		level, text = n.Level, n.Text
	}
	return &plugin.Notice{Level: level, Text: text + " " + tr("Nothing is live until you apply.")}
}

// previewRequest reports whether a submission is asking what the form on screen
// would write, rather than asking to write it. The shell marks it — the watcher
// in verso-forms.js sends the header — so a plugin cannot decide for itself that
// a write is "only a preview"; nothing on this path is staged, applied, or
// remembered, whatever the plugin returns.
func previewRequest(r *http.Request) bool {
	return !safeMethod(r.Method) && r.Header.Get("X-Verso-Interaction") == "preview"
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
		// A structural change to the plugin's own uci sections is the shell's to
		// realize, not the plugin's (ADR-005 §7): the shell performs the add,
		// delete, or reorder through rpcd, then re-renders the fresh state as a
		// read. The plugin ships none of that logic — it only declared the
		// repeater, or the listing whose rows drag.
		var realize structuralOp
		switch {
		case r.PostForm.Get(widget.RepeaterOpField) != "":
			realize = s.realizeRepeater
		case r.PostForm.Get(widget.ReorderConfigField) != "":
			realize = s.realizeReorder
		}
		if realize != nil {
			if body, st, ok := realize(r.Context(), m, s.sessionSID(r), r.PostForm, tr); !ok {
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
	req.Ubus = s.readUbus(r.Context(), m, s.sessionSID(r))

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

	// A composed access form returns through its shell host. The plugin still
	// owns the schema, validation, ACL and commit intents.
	if r.URL.Path == "/system/access" && m.SystemAccess == pluginPath {
		widget.Walk(wdg, func(n widget.Widget) {
			switch n := n.(type) {
			case *widget.Form:
				n.Action = "/system/access?plugin=" + m.ID
			case *widget.Collection:
				n.Action = "/system/access?plugin=" + m.ID
			}
		})
	}

	if env.Back != nil {
		widget.Walk(wdg, func(n widget.Widget) {
			if form, ok := n.(*widget.Form); ok && (form.Style == "page" || form.Style == "settings") {
				form.CancelHref = widget.SafeHref(env.Back.Href)
			}
		})
	}

	// The raw gauge (ADR-005 §5): raw is instrumented because its usage is the
	// demand signal for the next widget. Dev sessions log it; production pays
	// nothing (s.devCSS is set only under scripts/dev.sh).
	if s.devCSS != "" {
		rawCount := 0
		var unwired []string
		widget.Walk(wdg, func(n widget.Widget) {
			switch n := n.(type) {
			case *widget.Raw:
				rawCount++
			case *widget.Table:
				// A live source the shell does not serve renders a still
				// listing — silently, in production, because a table that
				// works minus its stream beats a page that fails. In dev the
				// silence is the bug, so it is named.
				if n.Stream != nil && !widget.StreamSourceKnown(n.Stream.Source) {
					unwired = append(unwired, n.Stream.Source)
				}
			}
		})
		if rawCount > 0 {
			log.Printf("verso: dev: plugin %q page %q carries %d raw widget(s) — check whether an existing widget or the envelope notice fits (ADR-005 §5)", m.ID, pluginPath, rawCount)
		}
		for _, source := range unwired {
			log.Printf("verso: dev: plugin %q page %q declares stream source %q, which the shell does not serve — the listing renders still", m.ID, pluginPath, source)
		}
		// Only the first live preview can be kept current: the watcher sends one
		// form and the answer is one block, so a second would either go stale or
		// be overwritten by its neighbour's.
		if n := widget.LivePreviewCount(wdg); n > 1 {
			log.Printf("verso: dev: plugin %q page %q declares %d live previews; only the first is kept current", m.ID, pluginPath, n)
		}
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
	// drawer applies. A repeater op was downgraded to a render above, so it skips
	// this — its write already went through rpcd.
	// A preview is a question, not a submission: the plugin was handed the values
	// on screen and asked what it would write from them, and the answer goes back
	// as the one block that says so. Nothing here stages, applies, or is
	// remembered — the operator is still typing, and half a port number is not a
	// change anybody asked to make.
	if previewRequest(r) {
		hdr.PreviewOnly = true
		var preview strings.Builder
		found, err := s.widgets.RenderLivePreviewWithToken(&preview, wdg, s.sessionCSRF(r), lang, t)
		if err != nil {
			log.Printf("verso: plugin %q preview render failed: %v", m.ID, err)
		}
		// A tree with no live preview has nothing to answer with. It leaves as
		// an empty body rather than as the page, because the answer to "what
		// would this write" is the only thing that was asked for — and because
		// falling through would stage a submission the operator never made.
		if !found || err != nil {
			return "", http.StatusNoContent
		}
		return template.HTML(preview.String()), http.StatusOK //nolint:gosec // rendered by the shell's own templates
	}

	if !safeMethod(method) {
		if validateSchema(wdg) || status == http.StatusUnprocessableEntity {
			status = http.StatusUnprocessableEntity
		} else {
			if err := validateApplyActions(m, env.Apply); err != nil {
				log.Printf("verso: plugin %q returned an invalid apply action: %v", m.ID, err)
				return s.notice(tr("Not permitted"), fmt.Sprintf(
					tr("%s tried to perform an operation it did not declare."), m.Name)), http.StatusForbidden
			}
			if len(env.Commands) > 0 {
				if len(env.Commit) > 0 || len(env.Apply) > 0 {
					return s.notice(tr("Not permitted"), tr("Commands cannot be combined with staged changes.")), http.StatusForbidden
				}
				if err := s.runPluginCommands(r.Context(), m, s.sessionSID(r), env.Commands); err != nil {
					log.Printf("verso: plugin %q command failed: %v", m.ID, err)
					message := "The router could not complete the action. Try again."
					var validation commandValidationError
					if errors.As(err, &validation) {
						message = validation.Error()
					}
					// Said once, in the page's notice: the act failed, not any
					// one form on the page, and a page of several forms would
					// otherwise repeat it in each.
					status = http.StatusUnprocessableEntity
					env.Notice = &plugin.Notice{Level: "danger", Text: message}
				} else {
					hdr.CommandDone = true
					if env.Commands[0].Name == "config-file-stage" {
						hdr.StagedCommit = true
						hdr.StagedStructure = true
					}
				}
			}

			if len(env.Commit) > 0 {
				if body, st, ok := s.brokerStage(r.Context(), m, s.sessionSID(r), env.Commit, tr); !ok {
					return body, st
				}
				hdr.StagedCommit = true
				hdr.StagedStructure = restructures(env.Commit)
			}
			s.setPendingApply(s.sessionSID(r), env.Apply)
		}
	}

	// Everything above judged what the plugin declared; everything below renders
	// what this reader sees. The mode filter (ADR-015) runs on that boundary: the
	// datatype gate and the brokered write are never softened by a reading, and the
	// page-wide lens is then weighed against the content that survives.
	mode := readerMode(r)
	wdg = widget.FilterMode(wdg, mode)

	// The page-wide lens is the shell's call, not each plugin's constant: a
	// plugin declares the filter it would like and the shell keeps it only on a
	// page with enough to sift (ADR-005 §5 — the vocabulary decides what a widget
	// is worth). Below the threshold the widget is removed rather than hidden, so
	// the page carries no dead dock and no "/" shortcut into nothing.
	if widget.FilterableCount(wdg) <= widget.FilterThreshold {
		wdg = widget.StripFilters(wdg)
	}

	// A page form carries its own submit: pressing it stages what the form
	// holds, and the stage is applied from the review drawer (ADR-010). The
	// shell guarantees the button — a page form the plugin left label-less has
	// nothing else to submit it — so a missing label is defaulted here.
	ensureEditorSubmit(wdg, tr("Save changes"))

	// What a submission that changed the stage says about itself is composed
	// here, once, for every way it can be answered: the page's flash slot, the
	// listing an editor returns to, and the outcome a closing panel hands back.
	hdr.Notice = localizeNotice(env.Notice, tr)
	if hdr.StagedCommit {
		hdr.Notice = stagedOutcome(hdr.Notice, tr)
	}

	// A request for one panel is not a request for a page: the frame is already
	// on screen and only what it holds is being replaced. The plugin was asked
	// nothing different — it answers an address naming an open panel with that
	// panel open, exactly as it does for a full visit, and it answers the panel's
	// own submission the same way, staged above like any other — so this only
	// takes the panel out of the answer and leaves the rest unsent.
	//
	// A submission that changed a row's values is done with the panel: the panel
	// closes, and the answer is the outcome alone, said where the panel was. One
	// the plugin refused keeps its 422 and comes back as the panel with the
	// offending controls marked, so the frame swaps the refusal in rather than
	// showing nothing; one the plugin answered with the panel and no write — a
	// computed round trip — comes back as that panel. A submission that changed
	// which rows there are, a section made or removed, is not answered here at
	// all: the listing behind the panel has to be drawn again to show it, and a
	// request that names no open panel is a page in any case — a stale address,
	// a direct visit, a delete.
	if panelRequest(r) && !hdr.StagedStructure {
		if hdr.StagedCommit {
			var outcome strings.Builder
			if err := s.pageSet(lang).ExecuteTemplate(&outcome, "verso-flash", panelFlash(hdr.Notice)); err != nil {
				log.Printf("verso: plugin %q outcome render failed: %v", m.ID, err)
			} else {
				hdr.PanelDone = true
				return template.HTML(outcome.String()), http.StatusOK //nolint:gosec // rendered by the shell's own templates
			}
		}
		var panel strings.Builder
		switch found, err := s.widgets.RenderOpenPanelWithToken(&panel, wdg, s.sessionCSRF(r), lang, t, panelFlash(hdr.Notice)); {
		case err != nil:
			log.Printf("verso: plugin %q panel render failed: %v", m.ID, err)
		case found:
			hdr.PanelOnly = true
			return template.HTML(panel.String()), status //nolint:gosec // rendered by the shell's own templates
		}
	}

	// A bar with nothing to narrow is no toolbar: its act goes to the heading
	// line. Lifted only here, for a whole page — a panel request above still
	// finds a blank object's panel on the bar where the plugin put it.
	hdr.HeadingAct = s.headingAct(r, widget.TakeHeadingAct(wdg), lang, t)
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
	hdr.Tone = env.Tone
	hdr.Subheading = tr(env.Subheading)
	hdr.Action = localizeAction(env.Action, tr)
	hdr.Back = localizeBack(env.Back, tr)
	hdr.Banner = localizeBanner(env.Banner, tr)
	*width = env.Width
	// The subpage tabs carry the plugin id, so renderPage localizes their labels
	// from the plugin's catalog (ADR-012 §5) — no need to pre-translate here.
	*pages = subpageTabsAt(m, pluginPath, env.Pages, mode)
	return template.HTML(b.String()), status
}

// subpageTabs builds the top bar (the third navigation tier) from a plugin's
// declared subpages. Paths are relative to the plugin's mount — the shell
// builds every href and marks the active tab from the request, so the bar can
// never point outside the plugin. A tab declaring the other reading is absent
// (ADR-015 §5), filtered by the same rule as a manifest nav entry; its URL still
// answers, so the operator standing on it keeps their page.
func subpageTabsAt(m plugin.Manifest, pluginPath string, declared []plugin.PageTab, mode string) []pageTab {
	if len(declared) == 0 {
		return nil
	}
	cur := strings.Trim(pluginPath, "/")
	tabs := make([]pageTab, 0, len(declared))
	for _, p := range declared {
		if !modeShows(p.Mode, mode) {
			continue
		}
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
// caller blocks the write and re-renders as 422. widget.Walk covers every
// container — a field inside a modal, a conditions item, or a table row's
// drawer is enforced the same as one directly in a form.
func validateSchema(w widget.Widget) bool {
	found := false
	widget.WalkActive(w, func(n widget.Widget) {
		switch n := n.(type) {
		case *widget.Form:
			if n.Error != "" {
				found = true
			}
		case *widget.Collection:
			// a refused addition blocks the write as a form's error does
			if n.Add != nil && n.Add.Error != "" {
				found = true
			}
		case *widget.Field:
			if n.Error == "" && n.Datatype != "" {
				if err := datatype.Validate(n.Datatype, n.Value); err != nil {
					n.Error = err.Error()
				}
			}
			if n.Error != "" {
				found = true
			}
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
			if len(n.Errors) > 0 {
				found = true
			}
		}
	})
	return found
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
// the apply. Actions merge by name within the session (a re-save replaces its
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
// (ADR-010): the review drawer owns apply and discard. It refuses any
// op whose config the plugin did not declare in its manifest acl — a plugin
// cannot broker a write outside its declared surface — and rpcd re-checks the
// operator's sid on every call. On refusal or failure it returns a contained
// notice and a status with ok=false; on success ok is true and the caller
// renders the plugin's returned widget.
func (s *Server) brokerStage(ctx context.Context, m plugin.Manifest, sid string, ops []plugin.CommitOp, tr func(string) string) (template.HTML, int, bool) {
	declared := declaredUCIConfigs(m)
	// Preflight the entire batch before staging its first operation. In
	// particular, a named create must never overwrite an existing interface.
	created := map[string]bool{}
	for _, op := range ops {
		if op.Config == "" || !declared[op.Config] {
			return s.notice(tr("Not permitted"), fmt.Sprintf(tr("%s tried to change settings it did not declare."), m.Name)), http.StatusForbidden, false
		}
		if (op.Section == "" && op.Type == "") || (op.Delete && (op.Section == "" || op.Type != "" || len(op.Values) > 0)) {
			return s.malformedOperation(m, tr)
		}
		if op.Type != "" && op.Section != "" {
			key := op.Config + "." + op.Section
			sections, err := s.backend.UCIConfig(ctx, sid, op.Config)
			if err != nil {
				return s.stageFailed(tr)
			}
			if _, exists := sections[op.Section]; exists || created[key] {
				return s.stageFailed(tr)
			}
			created[key] = true
		}
	}
	for _, op := range ops {
		// A delete is the whole section and nothing else: naming a type or values
		// beside it describes two operations at once, which the shell will not
		// guess at.
		if op.Delete {
			if op.Section == "" || op.Type != "" || len(op.Values) > 0 {
				log.Printf("verso: plugin %q sent a malformed delete for uci %q; refused", m.ID, op.Config)
				return s.malformedOperation(m, tr)
			}
			if err := s.backend.UCIDelete(ctx, sid, op.Config, op.Section, ""); err != nil {
				log.Printf("verso: plugin %q delete in uci %q failed: %v", m.ID, op.Config, err)
				return s.stageFailed(tr)
			}
			continue
		}
		// An op with no section and a type creates the section first (through
		// rpcd, staged like the set): the "drawer first, row on save" flow —
		// a plugin never adds bare sections it then has to chase.
		section, added := op.Section, false
		if op.Type != "" {
			// A typed operation creates a section. Named sections are required for
			// objects referenced by other configs (netifd interfaces, for example).
			created, err := s.backend.UCIAdd(ctx, sid, op.Config, op.Type, section)
			if err != nil {
				log.Printf("verso: plugin %q section create in uci %q failed: %v", m.ID, op.Config, err)
				return s.stageFailed(tr)
			}
			section, added = created, true
		}
		// A null value clears its option. uci.set has no way to say "unset", so
		// the nulls leave as option-level deletes and only the remaining values
		// are written. It is how an editor drops a setting it owns: writing the
		// option empty would leave a value behind, and an empty value is rarely
		// what "no longer set" means to the service reading it.
		values, cleared := splitClears(op.Values)
		absent := 0
		for _, option := range cleared {
			// An option that was never set is already in the state the clear asks
			// for. rpcd says so with NOT_FOUND, which the backend names
			// ErrOptionNotFound; an editor that owns a set of options states all
			// of them on every save, so a stock-shaped section hits this on the
			// ones nobody ever wrote. Any other failure still stops the stage.
			if err := s.backend.UCIDelete(ctx, sid, op.Config, section, option); err != nil {
				if errors.Is(err, openwrt.ErrOptionNotFound) {
					absent++
					continue
				}
				log.Printf("verso: plugin %q clear of uci %q option %q failed: %v", m.ID, op.Config, option, err)
				return s.stageFailed(tr)
			}
		}
		if len(values) == 0 && len(cleared) > 0 {
			// Every option this op named was already absent and it set none: not
			// one call carried a change, so nothing in the op has proved the
			// section it addressed still exists. A stale id — a section deleted
			// in another tab, a plugin working from an old snapshot — would
			// otherwise stage nothing and be reported as saved. One read of that
			// config settles it, and only on this path: a save that set a value,
			// or genuinely cleared one, has proved it already and pays nothing.
			if absent == len(cleared) && !added && !s.sectionPresent(ctx, sid, op.Config, section) {
				log.Printf("verso: plugin %q addressed uci %q section %q, which is not there; nothing was staged", m.ID, op.Config, section)
				return s.stageFailed(tr)
			}
			continue
		}
		if err := s.backend.UCISet(ctx, sid, op.Config, section, values); err != nil {
			log.Printf("verso: plugin %q write to uci %q failed: %v", m.ID, op.Config, err)
			return s.stageFailed(tr)
		}
	}
	return "", 0, true
}

// sectionPresent reports whether a config still holds the named section. A read
// that fails answers no: the caller reaches here only where nothing was written,
// and a section nobody could confirm is not one to report as saved.
func (s *Server) sectionPresent(ctx context.Context, sid, config, section string) bool {
	sections, err := s.backend.UCIConfig(ctx, sid, config)
	if err != nil {
		log.Printf("verso: uci %q could not be read to confirm section %q: %v", config, section, err)
		return false
	}
	_, ok := sections[section]
	return ok
}

// splitClears separates the options a commit sets from the options it clears —
// the entries whose value is JSON null. The cleared names come back sorted, so
// one submission stages the same sequence of operations every time.
func splitClears(values map[string]any) (map[string]any, []string) {
	set := make(map[string]any, len(values))
	var cleared []string
	for option, value := range values {
		if value == nil {
			cleared = append(cleared, option)
			continue
		}
		set[option] = value
	}
	sort.Strings(cleared)
	return set, cleared
}

// stageFailed is the contained response to rpcd refusing or failing a staged
// operation: the operator sees a plain "try again", never a stack trace.
func (s *Server) stageFailed(tr func(string) string) (template.HTML, int, bool) {
	return s.notice(tr("Save failed"),
		tr("The change couldn’t be saved just now. Try again in a moment.")), http.StatusServiceUnavailable, false
}

// malformedOperation is the contained response to an operation the shell cannot
// act on — a commit op that describes two things at once, a repeater affordance
// that posted without the fields to act on. A plugin-side problem, not the
// operator's, so it is a plain notice.
func (s *Server) malformedOperation(m plugin.Manifest, tr func(string) string) (template.HTML, int, bool) {
	return s.notice(tr("Couldn’t apply that change"), fmt.Sprintf(
		tr("Verso couldn’t apply that change to %s."), m.Name)), http.StatusBadRequest, false
}

// structuralOp is one shell-realized change to a plugin's uci sections — the
// repeater's add and remove, a reorderable listing's new order. Each is bounded
// by the plugin's declared write scope, performs its write through rpcd on the
// operator's behalf, and answers a contained notice with ok=false when it
// refuses or fails.
type structuralOp func(context.Context, plugin.Manifest, string, url.Values, func(string) string) (template.HTML, int, bool)

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
			return s.malformedOperation(m, tr)
		}
		_, err = s.backend.UCIAdd(ctx, sid, config, secType, "")
	case widget.RepeaterOpRemove:
		section := form.Get(widget.RepeaterSectionField)
		if section == "" {
			return s.malformedOperation(m, tr)
		}
		err = s.backend.UCIDelete(ctx, sid, config, section, "")
	default:
		log.Printf("verso: plugin %q unknown repeater op %q; refused", m.ID, op)
		return s.malformedOperation(m, tr)
	}
	if err != nil {
		log.Printf("verso: plugin %q repeater op on uci %q failed: %v", m.ID, config, err)
		return s.stageFailed(tr)
	}
	return "", 0, true
}

// realizeReorder performs a dragged listing's structural change on the operator's
// behalf: rpcd's `uci order` on the declared config, staged like every other write
// (ADR-010), so the review drawer owns the apply. It is bounded exactly as
// realizeRepeater is — the config must be one the plugin declared in acl.write,
// and rpcd re-checks the operator's sid — and every posted id must name a section
// the config really holds, so a stale page cannot order a listing into a shape the
// device does not have.
func (s *Server) realizeReorder(ctx context.Context, m plugin.Manifest, sid string, form url.Values, tr func(string) string) (template.HTML, int, bool) {
	config := form.Get(widget.ReorderConfigField)
	if !declaredUCIConfigs(m)[config] {
		log.Printf("verso: plugin %q reorder of undeclared uci config %q; refused", m.ID, config)
		return s.notice(tr("Not permitted"), fmt.Sprintf(
			tr("%s tried to change settings it did not declare."), m.Name)), http.StatusForbidden, false
	}
	moved := form[widget.ReorderIDField]
	if len(moved) == 0 {
		return s.malformedOperation(m, tr)
	}
	sections, err := s.backend.UCIConfig(ctx, sid, config)
	if err != nil {
		log.Printf("verso: plugin %q read of uci %q for a reorder failed: %v", m.ID, config, err)
		return s.stageFailed(tr)
	}
	current := sectionOrder(sections)
	order, err := reorderSections(current, moved)
	if err != nil {
		log.Printf("verso: plugin %q sent an unusable order for uci %q: %v", m.ID, config, err)
		return s.malformedOperation(m, tr)
	}
	// A drag the operator abandoned announces the sequence it started from, so an
	// order that moves nothing stages nothing: the re-render below is the whole
	// answer, and the stage stays as clean as it was.
	if slices.Equal(current, order) {
		return "", 0, true
	}
	if err := s.backend.UCIOrder(ctx, sid, config, order); err != nil {
		log.Printf("verso: plugin %q reorder of uci %q failed: %v", m.ID, config, err)
		return s.stageFailed(tr)
	}
	return "", 0, true
}

// sectionOrder is a uci snapshot's sections in the sequence the file holds them.
// rpcd hands a config back as a map, so only each section's `.index` states where
// it sits; equal indices fall back to the name, so one snapshot always yields one
// order.
func sectionOrder(sections map[string]any) []string {
	names := make([]string, 0, len(sections))
	for name := range sections {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		a, b := sectionIndex(sections[names[i]]), sectionIndex(sections[names[j]])
		if a == b {
			return names[i] < names[j]
		}
		return a < b
	})
	return names
}

// sectionIndex reads a section's position out of its `.index` meta. rpcd's
// blobmsg carries it as an integer and a JSON-decoded snapshot as a float; a
// section carrying neither sorts first, where the name breaks the tie.
func sectionIndex(section any) float64 {
	values, _ := section.(map[string]any)
	switch index := values[".index"].(type) {
	case int64:
		return float64(index)
	case float64:
		return index
	}
	return 0
}

// reorderSections rebuilds a config's whole section sequence from the ids the
// browser posted. The posted ids refill exactly the slots they already occupy, so
// a section the listing never showed — another type interleaved among them —
// keeps its place. A grouped listing posts its rows group-major, so the first drag
// on one can normalize more of the sequence than the visible move touched; that is
// faithful where the groups are themselves derived from the config, which is the
// only shape a grouped reorderable listing has (a firewall chain evaluates its own
// rules, in config order, and nothing else).
func reorderSections(current, moved []string) ([]string, error) {
	held := make(map[string]bool, len(current))
	for _, name := range current {
		held[name] = true
	}
	slots := make(map[string]bool, len(moved))
	for _, id := range moved {
		switch {
		case !held[id]:
			return nil, fmt.Errorf("no section %q in this config", id)
		case slots[id]:
			return nil, fmt.Errorf("section %q posted twice", id)
		}
		slots[id] = true
	}
	out := make([]string, 0, len(current))
	next := 0
	for _, name := range current {
		if slots[name] {
			out = append(out, moved[next])
			next++
			continue
		}
		out = append(out, name)
	}
	return out, nil
}

// declaredUCIConfigs is the set of uci configs a plugin declared it may write in
// its manifest acl (scope "uci"). It bounds what the shell will broker for it.
// ensureEditorSubmit gives every label-less page form in the tree a default submit
// label, so a page form always has its own button — nothing else submits one. A
// plugin that set a label — the object's own verb, "Add rule" — keeps it; only an
// empty one is defaulted.
func ensureEditorSubmit(w widget.Widget, label string) {
	widget.Walk(w, func(n widget.Widget) {
		if f, ok := n.(*widget.Form); ok && f.Style == "page" && f.Submit == "" && !f.NoSubmit && !f.AutoSubmit && !f.ConfirmDriven() {
			f.Submit = label
		}
	})
}

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

// readUbus brokers the plugin's non-uci reads (ADR-007): live system state a
// de-privileged plugin cannot reach, read through the privileged helper with the
// operator's sid and handed down beside the uci snapshot. It shares the snapshot's
// posture — a read the shell cannot complete contributes nothing rather than
// failing the page, and a plugin declaring none receives no header at all.
func (s *Server) readUbus(ctx context.Context, m plugin.Manifest, sid string) plugin.Ubus {
	var out plugin.Ubus
	for _, function := range declaredUbusReadFunctions(m) {
		result, brokered, err := s.brokeredUbusRead(ctx, sid, function)
		if !brokered {
			continue
		}
		if err != nil {
			log.Printf("verso: plugin %q read of ubus verso.%s failed: %v", m.ID, function, err)
			continue
		}
		if out == nil {
			out = make(plugin.Ubus)
		}
		out[function] = result
	}
	return out
}

// brokeredUbusRead bounds the reads the shell will perform on a plugin's behalf to
// a closed set of named helper functions, the way validateApplyActions bounds the
// privileged tail of a write. Declaring a scope is not enough: the shell must also
// know the function, so a manifest cannot name its way to an arbitrary helper verb.
// It reports whether the function is one the shell brokers at all.
func (s *Server) brokeredUbusRead(ctx context.Context, sid, function string) (json.RawMessage, bool, error) {
	switch function {
	case "dhcpState":
		if backend, ok := s.backend.(interface {
			DHCPState(context.Context, string) (json.RawMessage, error)
		}); ok {
			result, err := backend.DHCPState(ctx, sid)
			return result, true, err
		}
		return nil, true, fmt.Errorf("DHCP state unavailable")
	case "dnsState":
		if backend, ok := s.backend.(interface {
			DNSState(context.Context, string) (json.RawMessage, error)
		}); ok {
			result, err := backend.DNSState(ctx, sid)
			return result, true, err
		}
		return nil, true, fmt.Errorf("DNS state unavailable")
	case "accessCredentials":
		result, err := s.readAccessCredentials(ctx, sid)
		return result, true, err
	case "firewallCounters":
		result, err := s.backend.FirewallCounters(ctx, sid)
		return result, true, err
	case "dhcpLeases":
		result, err := s.dhcpLeases(ctx, sid)
		return result, true, err
	case "networkState":
		if backend, ok := s.backend.(interface {
			NetworkState(context.Context, string) (json.RawMessage, error)
		}); ok {
			result, err := backend.NetworkState(ctx, sid)
			if err == nil {
				result = s.networkTopology(ctx, result)
			}
			return result, true, err
		}
		return nil, true, fmt.Errorf("network state unavailable")
	case "wirelessState":
		if backend, ok := s.backend.(interface {
			WirelessState(context.Context, string) (json.RawMessage, error)
		}); ok {
			result, err := backend.WirelessState(ctx, sid)
			return result, true, err
		}
		return nil, true, fmt.Errorf("wireless state unavailable")
	}
	return nil, false, nil
}

// declaredUbusReadFunctions is the ordered, de-duplicated set of Verso helper
// functions a plugin declared it reads (manifest acl.read, scope "ubus", object
// "verso").
func declaredUbusReadFunctions(m plugin.Manifest) []string {
	var out []string
	seen := make(map[string]bool)
	for _, a := range m.ACL.Read {
		if a.Scope == "ubus" && a.Object == "verso" && !seen[a.Function] {
			seen[a.Function] = true
			out = append(out, a.Function)
		}
	}
	return out
}
