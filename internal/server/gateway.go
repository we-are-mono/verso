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
	m, ok := s.pluginByID[r.PathValue("id")]
	if !ok {
		http.NotFound(w, r)
		return
	}
	heading := m.Name
	width := ""
	body, status := s.pluginBody(r, m, &heading, &width)
	s.renderPage(w, r, status, heading, width, body)
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
func (s *Server) pluginBody(r *http.Request, m plugin.Manifest, heading, width *string) (template.HTML, int) {
	if m.SchemaVersion != supportedSchemaVersion {
		return s.notice("Plugin needs a newer Verso", fmt.Sprintf(
			"%s speaks schema version %d; this shell supports version %d.",
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
			return s.notice("Permission check unavailable",
				"Verso couldn’t verify your permissions just now. Try again in a moment."), http.StatusServiceUnavailable
		case !allowed:
			return s.notice("Not permitted",
				fmt.Sprintf("Your account isn’t permitted to change %s.", m.Name)), http.StatusForbidden
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
			if body, st, ok := s.realizeRepeater(r.Context(), m, s.sessionSID(r), r.PostForm); !ok {
				return body, st
			}
			method = http.MethodGet // render current state; carry no form, run no commit
		}
	}

	req := plugin.Request{Method: method, Path: r.PathValue("path"), Query: r.URL.Query()}
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
		return s.unavailable(m), http.StatusOK
	}

	wdg, err := widget.Decode(env.Widget)
	if err != nil {
		log.Printf("verso: plugin %q returned undecodable schema: %v", m.ID, err)
		return s.unavailable(m), http.StatusOK
	}

	status := http.StatusOK
	if env.Status == http.StatusUnprocessableEntity {
		status = http.StatusUnprocessableEntity
	}

	// On a state-changing request the shell enforces the declared datatypes on the
	// returned schema (ADR-008): any failure annotates the widget in place, forces
	// 422, and blocks the write — merged with whatever the plugin already flagged.
	// Only a clean submission reaches brokerCommit, which executes the plugin's
	// commit intent through rpcd (ADR-007). A repeater op was downgraded to a render
	// above, so it skips this — its write already went through rpcd.
	if !safeMethod(method) {
		if validateSchema(wdg) {
			status = http.StatusUnprocessableEntity
		} else if len(env.Commit) > 0 {
			if body, st, ok := s.brokerCommit(r.Context(), m, s.sessionSID(r), env.Commit); !ok {
				return body, st
			}
		}
	}

	var b strings.Builder
	if err := s.widgets.RenderWithToken(&b, wdg, s.sessionCSRF(r)); err != nil {
		log.Printf("verso: plugin %q render failed: %v", m.ID, err)
		return s.unavailable(m), http.StatusOK
	}
	if env.Title != "" {
		*heading = env.Title
	}
	*width = env.Width
	return template.HTML(b.String()), status
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
		return found
	default:
		return false
	}
}

type noticeData struct{ Title, Message string }

// notice renders a shell-owned message (not plugin content) through the shell's
// tokens. Fields flow through html/template, so an untrusted plugin name in the
// message is escaped.
func (s *Server) notice(title, message string) template.HTML {
	var b bytes.Buffer
	if err := s.page.ExecuteTemplate(&b, "notice.html.tmpl", noticeData{title, message}); err != nil {
		return template.HTML(template.HTMLEscapeString(title + ": " + message))
	}
	return template.HTML(b.String())
}

func (s *Server) unavailable(m plugin.Manifest) template.HTML {
	return s.notice("Plugin unavailable", fmt.Sprintf(
		"%s isn’t responding right now. The rest of Verso is unaffected.", m.Name))
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

// brokerCommit performs, through rpcd and on the operator's behalf, the uci writes
// a plugin requested (ADR-007). It refuses any op whose config the plugin
// did not declare in its manifest acl — a plugin cannot broker a write outside its
// declared surface — and rpcd re-checks the operator's sid on every call. On
// refusal or failure it returns a contained notice and a status with ok=false; on
// success ok is true and the caller renders the plugin's returned widget.
func (s *Server) brokerCommit(ctx context.Context, m plugin.Manifest, sid string, ops []plugin.CommitOp) (template.HTML, int, bool) {
	declared := declaredUCIConfigs(m)
	dirty := make(map[string]bool)
	for _, op := range ops {
		if op.Config == "" || !declared[op.Config] {
			log.Printf("verso: plugin %q tried to write undeclared uci config %q; refused", m.ID, op.Config)
			return s.notice("Not permitted", fmt.Sprintf(
				"%s tried to change settings it did not declare.", m.Name)), http.StatusForbidden, false
		}
		if err := s.backend.UCISet(ctx, sid, op.Config, op.Section, op.Values); err != nil {
			log.Printf("verso: plugin %q write to uci %q failed: %v", m.ID, op.Config, err)
			return s.notice("Save failed",
				"The change couldn’t be saved just now. Try again in a moment."), http.StatusServiceUnavailable, false
		}
		dirty[op.Config] = true
	}
	for cfg := range dirty {
		if err := s.backend.UCICommit(ctx, sid, cfg); err != nil {
			log.Printf("verso: plugin %q commit of uci %q failed: %v", m.ID, cfg, err)
			return s.notice("Save failed",
				"The change was written but couldn’t be committed. Try again in a moment."), http.StatusServiceUnavailable, false
		}
	}
	return "", 0, true
}

// realizeRepeater performs a repeater's structural change on the operator's behalf
// (ADR-005 §7): a uci section add or delete through rpcd, then a commit. It refuses
// a config the plugin did not declare in its acl.write — the same surface
// brokerCommit bounds — and rpcd re-checks the operator's sid. On success the caller
// re-renders the fresh state; a failure is a contained notice, never a crash.
func (s *Server) realizeRepeater(ctx context.Context, m plugin.Manifest, sid string, form url.Values) (template.HTML, int, bool) {
	config := form.Get(widget.RepeaterConfigField)
	if config == "" || !declaredUCIConfigs(m)[config] {
		log.Printf("verso: plugin %q repeater op on undeclared uci config %q; refused", m.ID, config)
		return s.notice("Not permitted", fmt.Sprintf(
			"%s tried to change settings it did not declare.", m.Name)), http.StatusForbidden, false
	}

	var err error
	switch op := form.Get(widget.RepeaterOpField); op {
	case widget.RepeaterOpAdd:
		secType := form.Get(widget.RepeaterTypeField)
		if secType == "" {
			return s.malformedRepeater(m)
		}
		_, err = s.backend.UCIAdd(ctx, sid, config, secType)
	case widget.RepeaterOpRemove:
		section := form.Get(widget.RepeaterSectionField)
		if section == "" {
			return s.malformedRepeater(m)
		}
		err = s.backend.UCIDelete(ctx, sid, config, section)
	default:
		log.Printf("verso: plugin %q unknown repeater op %q; refused", m.ID, op)
		return s.malformedRepeater(m)
	}
	if err != nil {
		log.Printf("verso: plugin %q repeater op on uci %q failed: %v", m.ID, config, err)
		return s.notice("Save failed",
			"The change couldn’t be saved just now. Try again in a moment."), http.StatusServiceUnavailable, false
	}

	if err := s.backend.UCICommit(ctx, sid, config); err != nil {
		log.Printf("verso: plugin %q repeater commit of uci %q failed: %v", m.ID, config, err)
		return s.notice("Save failed",
			"The change was written but couldn’t be committed. Try again in a moment."), http.StatusServiceUnavailable, false
	}
	return "", 0, true
}

// malformedRepeater is the contained response to a repeater affordance that posted
// without the fields the shell needs to act — a client-side problem, not the
// operator's, so it is a plain notice.
func (s *Server) malformedRepeater(m plugin.Manifest) (template.HTML, int, bool) {
	return s.notice("Couldn’t apply that change", fmt.Sprintf(
		"Verso couldn’t apply that change to %s.", m.Name)), http.StatusBadRequest, false
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
