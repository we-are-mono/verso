// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/we-are-mono/verso/internal/plugin"
	"github.com/we-are-mono/verso/internal/widget"
)

// errNoSuchPlugin is the one failure fetchEntity can name on its own: the claim
// outlived the manifest it came from.
var errNoSuchPlugin = errors.New("verso: no such plugin")

// The entity panel: one subject, one panel, one tab per plugin that has
// something to say about it.
//
// A device is dnsdhcp's reservation, the firewall's verdict on its traffic, and
// a QoS plugin's shaping — three plugins, three separate processes, none of
// which can call another. So the panel is the shell's: it asks each live
// contributor for its tab and frames the answers together. That is the nav
// rail's rule one level down, and it is what lets a QoS plugin appear the day it
// is installed with no change anywhere else.
//
// The shell owns the vocabulary — which slots a kind of subject has, in what
// order, wearing which glyph. A plugin claims a slot and answers for it; it
// never names an icon or a position, so the row's shape is the design's and
// survives any combination of installed plugins.

// entityPath is where a panel is fetched from, and the prefix a plugin answers
// its own tab on. Both sides use the same word so the route reads the same from
// either end.
const entityPath = "/entity/"

// entitySlot is one place in a kind of subject's panel and in a listing's row.
// Icon and the order of the slice are the design's; a plugin supplies only what
// sits behind them.
type entitySlot struct {
	Name string
	Icon string
	// Act marks a slot whose icon does the thing rather than opening the panel
	// — a removal, a toggle. The panel never carries a tab for it.
	Act bool
	// Listing marks a slot that belongs to the listing rather than to any one of
	// its rows: the doorway that makes a new subject of this kind. It draws as
	// the action bar's one forward act, never as a row icon.
	Listing bool
}

// entitySlots is the closed vocabulary, by subject kind. A slot no plugin claims
// draws nothing; the row keeps the slot's place either way, so a listing does
// not change shape between boards.
var entitySlots = map[string][]entitySlot{
	"device": {
		{Name: "add", Icon: "plus", Act: true, Listing: true},
		{Name: "reserve", Icon: "pin"},
		{Name: "unreserve", Icon: "pin-off", Act: true},
		{Name: "shape", Icon: "sliders-horizontal"},
	},
}

// entityTab is one assembled tab: what it is called, whose it is, and the body
// the plugin answered with.
type entityTab struct {
	Slot     string
	Label    string
	PluginID string
	Active   bool
	Href     string
	Body     widget.Widget
	// CTA is the commit row's verb for this tab. It is the plugin's because
	// only the plugin knows what its tab saves; a tab that stages nothing
	// supplies none and the row is not drawn.
	CTA string
}

// entityClaim is one live plugin's claim on one slot.
type entityClaim struct {
	plugin.EntityTab
	Socket   string
	PluginID string
}

// entityContributors are the live plugins claiming a slot for one kind of
// subject, in the shell's slot order rather than in discovery order — so the
// tabs read the same on every board however the plugins were found.
func (s *Server) entityContributors(kind string) []entityClaim {
	var out []entityClaim
	for _, m := range s.manifestList() {
		if !s.probe(m.Socket) {
			continue
		}
		for _, t := range m.EntityTabs {
			if t.Entity == kind && t.Slot != "" {
				out = append(out, entityClaim{EntityTab: t, Socket: m.Socket, PluginID: m.ID})
			}
		}
	}
	order := slotOrder(kind)
	sort.SliceStable(out, func(a, b int) bool { return order[out[a].Slot] < order[out[b].Slot] })
	return out
}

// entityActs are the live direct acts on one subject of this kind, already
// addressed to that subject, in slot order.
func (s *Server) entityActs(kind, id string) map[string]string {
	acts := map[string]string{}
	for _, m := range s.manifestList() {
		if !s.probe(m.Socket) {
			continue
		}
		for _, a := range m.EntityActs {
			if a.Entity != kind || a.Slot == "" || a.Path == "" {
				continue
			}
			acts[a.Slot] = pluginHref(m.ID, strings.ReplaceAll(a.Path, "{id}", url.PathEscape(id)))
		}
	}
	return acts
}

// slotOrder maps a kind's slot names to their designed position, so a tab strip
// and a row's icons read the same order on every board.
func slotOrder(kind string) map[string]int {
	order := map[string]int{}
	for i, slot := range entitySlots[kind] {
		order[slot.Name] = i
	}
	return order
}

// EntityRowActs is what a listing's row shows for one subject: one icon per slot
// the design gives this kind, in order, each either opening the panel (a claimed
// tab), doing its thing (a claimed act), or standing empty. An unclaimed slot
// still holds its place — the column must not change width because a plugin is
// missing. Slots omitted from titles do not apply to this subject and are hidden.
func (s *Server) EntityRowActs(kind, id, subject string, titles map[string]string) []widget.TableRowAct {
	tabs := map[string]bool{}
	for _, t := range s.entityContributors(kind) {
		tabs[t.Slot] = true
	}
	acts := s.entityActs(kind, id)
	out := make([]widget.TableRowAct, 0, len(entitySlots[kind]))
	for _, slot := range entitySlots[kind] {
		if slot.Listing {
			continue // the listing's own doorway, not this row's
		}
		title, available := titles[slot.Name]
		if !available {
			continue
		}
		act := widget.TableRowAct{Icon: slot.Icon, Title: title}
		switch {
		case slot.Act && acts[slot.Name] != "":
			act.Href = acts[slot.Name]
		case !slot.Act && tabs[slot.Name]:
			act.Opens, act.Tab = true, slot.Name
		}
		// A slot nothing claims keeps neither Href nor Opens: the icon holds its
		// place and does nothing, so the column's width is the listing's and not
		// the install's.
		out = append(out, act)
	}
	return out
}

// EntityListingAct is where a listing's own forward act leads — the doorway that
// makes a new subject of this kind. It comes from whichever plugin claims the
// kind's listing slot, so the shell never names a plugin: with none installed
// there is nothing to make, and the bar simply carries no act.
func (s *Server) EntityListingAct(kind string) string {
	acts := s.entityActs(kind, "")
	for _, slot := range entitySlots[kind] {
		if slot.Listing {
			return acts[slot.Name]
		}
	}
	return ""
}

// fetchEntity asks one plugin for its tab on one subject. The plugin answers on
// a reserved path under its own mount, so it routes the request like any other
// — the shell brokers its declared reads the same way, and the plugin needs no
// session of its own (ADR-007).
func (s *Server) fetchEntity(ctx context.Context, r *http.Request, claim entityClaim, kind, id string) (*plugin.Envelope, error) {
	m, ok := s.manifestByID(claim.PluginID)
	if !ok {
		return nil, errNoSuchPlugin
	}
	req := plugin.Request{
		Method: http.MethodGet,
		Path:   entityPath + kind + "/" + id,
		Query:  map[string][]string{},
		UCI:    s.readSnapshot(ctx, m, s.sessionSID(r)),
		Ubus:   s.readUbus(ctx, m, s.sessionSID(r)),
	}
	return s.transport.Fetch(ctx, claim.Socket, req)
}

// entityTabs asks every live contributor for its tab on one subject. A plugin
// that fails or answers nothing contributes no tab: a panel is worth opening for
// the tabs that did answer, and a tab that cannot render is worse than absent.
func (s *Server) entityTabs(ctx context.Context, r *http.Request, kind, id, active string, posted *entityPost, shell []entityTab) []entityTab {
	// A tab's prose is its plugin's, so it localizes from that plugin's own
	// catalog and never from the shell's base (ADR-012 §5) — the label, the
	// commit verb, and the sentence about what applying it costs alike. The
	// shell's own tabs lead, what the subject is before what a plugin does with
	// it, and are named from the shell's catalog the same way.
	pluginTr := s.pluginTranslators(r)
	out := append([]entityTab(nil), shell...)
	for i := range out {
		out[i].Label = pluginTr(out[i].PluginID)(out[i].Label)
	}
	for _, claim := range s.entityContributors(kind) {
		// The tab that was just submitted already has its answer — what the
		// plugin said about the submission, errors and all. Asking it again with
		// a fresh read would render the values on disk over the ones the operator
		// typed, which is the one thing a refusal must not do.
		env, err := posted.answer(claim.Slot), error(nil)
		if env == nil {
			env, err = s.fetchEntity(ctx, r, claim, kind, id)
		}
		if err != nil || env == nil {
			continue
		}
		body, err := widget.Decode(env.Widget)
		if err != nil {
			continue
		}
		tr := pluginTr(claim.PluginID)
		label := env.Title
		if label == "" {
			label = claim.Label
		}
		out = append(out, entityTab{
			Slot: claim.Slot, Label: tr(label), PluginID: claim.PluginID, Body: body,
			Href: entityPath + kind + "/" + url.PathEscape(id) + "?tab=" + url.QueryEscape(claim.Slot),
			CTA:  tr(env.CTA),
		})
	}
	// The tab in force is the one asked for, else the first the design orders.
	for i := range out {
		if out[i].Slot == active {
			out[i].Active = true
			return out
		}
	}
	if len(out) > 0 {
		out[0].Active = true
	}
	return out
}

// entityPanelData is the panel's render model.
type entityPanelData struct {
	JoinsCode bool
	Kind      string
	ID        string
	Title     string
	Tabs      []entityTabView
	Body      template.HTML
	CTA       string
	// Post is where the commit row's verb submits: the tab in force, on the
	// panel's own route. The row is the panel's frame rather than part of the
	// plugin's form, so it carries the route itself.
	Post string
	CSRF string
}

// entityTabView is one tab in the strip.
type entityTabView struct {
	Label  string
	Href   string
	Active bool
}

// handleEntity renders one subject's panel. It answers the drawer's own fetch,
// so the body arrives when a panel is opened rather than riding along with every
// row of a listing — a page of thirty devices would otherwise ask every
// contributing plugin thirty times before anyone clicked anything.
func (s *Server) handleEntity(w http.ResponseWriter, r *http.Request) {
	kind, id, ok := entityRef(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	lang, t := s.localize(r)
	tr := translatorOrIdentity(t)
	data, err := s.entityPanel(r, kind, id, r.URL.Query().Get("tab"), lang, tr, nil)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	var buf bytes.Buffer
	if err := s.pageSet(lang).ExecuteTemplate(&buf, "entity-panel.html.tmpl", data); err != nil {
		log.Printf("verso: entity panel: %v", err)
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(buf.Bytes())
}

// entityRef splits "/entity/<kind>/<id>" into its two halves.
func entityRef(path string) (kind, id string, ok bool) {
	rest, found := strings.CutPrefix(path, entityPath)
	if !found {
		return "", "", false
	}
	kind, raw, found := strings.Cut(rest, "/")
	if !found || kind == "" || raw == "" {
		return "", "", false
	}
	id, err := url.PathUnescape(raw)
	if err != nil || id == "" {
		return "", "", false
	}
	if _, known := entitySlots[kind]; !known {
		return "", "", false
	}
	return kind, id, true
}

// entityPanel assembles one subject's panel: the facts the shell knows, and one
// tab per plugin that answered for it.
func (s *Server) entityPanel(r *http.Request, kind, id, active, lang string, tr func(string) string, posted *entityPost) (entityPanelData, error) {
	data := entityPanelData{Kind: kind, ID: id, CSRF: s.sessionCSRF(r)}
	var shell []entityTab
	switch {
	case id == entityNew:
		// A subject that does not exist yet has no facts to pin: the panel is
		// the tabs that could bring it into being, and nothing else.
		data.Title = newSubjectTitle(kind, tr)
	case posted != nil && posted.Title != "":
		// Removing the last limit can remove an offline device from the roster.
		// Keep the subject of this successful submission visible until close.
		data.Title = posted.Title
	case kind == "device":
		d, ok := s.deviceByMAC(r, id)
		if !ok {
			return data, errNoSuchEntity
		}
		data.Title = d.Name
		shell = s.deviceTabs(r, d)

	default:
		return data, errNoSuchEntity
	}

	tabs := s.entityTabs(r.Context(), r, kind, id, active, posted, shell)
	// A tab is a form like a page's: each control whose option waits on the
	// stage is marked on every opening, as it is on the plugin's own page.
	waits := s.waitingOptions(r.Context(), s.sessionSID(r))
	for _, tab := range tabs {
		data.Tabs = append(data.Tabs, entityTabView{Label: tab.Label, Href: tab.Href, Active: tab.Active})
		if !tab.Active {
			continue
		}
		// A tab's form posts back into the panel rather than navigating away, so
		// the drawer stays open over the listing that opened it. Which route it
		// posts to is the shell's to know — the plugin answered on its own mount
		// and has never heard of /entity.
		widget.PostIntoFrame(tab.Body, widget.FrameEntity, tab.Href)
		widget.MarkStaged(tab.Body, waits)
		// The tab's widgets are the plugin's prose too, so they translate from
		// that plugin's catalog exactly as its page would.
		var body strings.Builder
		if err := s.widgets.RenderWithToken(&body, tab.Body, s.sessionCSRF(r), lang, s.pluginTranslators(r)(tab.PluginID)); err != nil {
			continue
		}
		data.Body, data.CTA, data.Post = template.HTML(body.String()), tab.CTA, tab.Href
		data.JoinsCode = widget.EndsWithCode(tab.Body)
	}
	return data, nil
}

// entityNew is the id a panel wears while its subject does not exist yet:
// making one is editing one that is not there, so it is the same panel.
const entityNew = "new"

// newSubjectTitle names the panel for a subject about to be made.
func newSubjectTitle(kind string, tr func(string) string) string {
	if kind == "device" {
		return tr("Reserve an address")
	}
	return tr("New")
}

// errNoSuchEntity is a panel asked for a subject this board does not have.
var errNoSuchEntity = errors.New("verso: no such entity")

// entityPost is one tab's answer to a submission, carried into the panel render
// so the tab that was saved shows what its plugin said — a success, or a refusal
// with the submitted values still in the controls — while its neighbours are
// read fresh. A render that answers no submission carries none.
type entityPost struct {
	Slot  string
	Env   *plugin.Envelope
	Title string
}

// answer is the submitted envelope for one slot, or nil for every other slot and
// for a render that answers no submission at all.
func (p *entityPost) answer(slot string) *plugin.Envelope {
	if p == nil || p.Slot != slot {
		return nil
	}
	return p.Env
}

// entityContributor is the live plugin answering for one slot of a kind, or the
// first the design orders when the caller named none — the same tab a panel
// opens on, so a submission with no tab lands where the operator was.
func (s *Server) entityContributor(kind, slot string) (entityClaim, bool) {
	claims := s.entityContributors(kind)
	if len(claims) == 0 {
		return entityClaim{}, false
	}
	for _, claim := range claims {
		if claim.Slot == slot {
			return claim, true
		}
	}
	if slot == "" {
		return claims[0], true
	}
	return entityClaim{}, false
}

// handleEntitySave answers a panel tab's submission. The tab belongs to one
// plugin, so this is that plugin's write and passes the same gate any other
// does (ADR-007): the shell checks the operator's grants against the plugin's
// declared write scopes, forwards the form, and brokers the uci ops the plugin
// asked for into the shared stage (ADR-010) — the review drawer still owns apply.
//
// It answers with the panel, not a redirect: the drawer is open over the listing
// that opened it, and the submission swapped in where the form was.
func (s *Server) handleEntitySave(w http.ResponseWriter, r *http.Request) {
	kind, id, ok := entityRef(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	claim, ok := s.entityContributor(kind, r.URL.Query().Get("tab"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	m, ok := s.manifestByID(claim.PluginID)
	if !ok {
		http.NotFound(w, r)
		return
	}
	lang, t := s.localize(r)
	tr := translatorOrIdentity(t)
	ptr := s.pluginTranslators(r)(claim.PluginID)

	switch allowed, err := s.authorizePluginWrite(r.Context(), m, s.sessionSID(r)); {
	case err != nil:
		log.Printf("verso: plugin %q permission check failed: %v", m.ID, err)
		s.entityNotice(w, http.StatusServiceUnavailable, ptr("Your permissions couldn’t be checked just now. Try again in a moment."))
		return
	case !allowed:
		s.entityNotice(w, http.StatusForbidden, fmt.Sprintf(ptr("Your account isn’t permitted to change %s."), m.Name))
		return
	}
	if err := r.ParseForm(); err != nil {
		s.entityNotice(w, http.StatusBadRequest, ptr("That form couldn’t be read, so nothing was saved."))
		return
	}
	r.PostForm.Del("_csrf") // the shell's CSRF token is not the plugin's business
	subjectTitle := ""
	if kind == "device" && id != entityNew {
		if device, found := s.deviceByMAC(r, id); found {
			subjectTitle = device.Name
		}
	}

	sid := s.sessionSID(r)
	env, err := s.transport.Fetch(r.Context(), claim.Socket, plugin.Request{
		Method: http.MethodPost,
		Path:   entityPath + kind + "/" + id,
		Query:  r.URL.Query(),
		Form:   r.PostForm,
		UCI:    s.readSnapshot(r.Context(), m, sid),
		Ubus:   s.readUbus(r.Context(), m, sid),
	})
	if err != nil || env == nil {
		log.Printf("verso: plugin %q unavailable: %v", m.ID, err)
		s.entityNotice(w, http.StatusOK, fmt.Sprintf(ptr("%s isn’t answering just now, so nothing was saved."), m.Name))
		return
	}
	// A plugin that refused its own submission asks for no writes; one that
	// accepted states them, and the shell performs them on the operator's behalf.
	if env.Status != http.StatusUnprocessableEntity && len(env.Commit) > 0 {
		// A panel's tab is the plugin's part of whichever page opened the
		// panel; its changes are filed under the plugin's own page.
		if body, status, ok := s.brokerStage(r.Context(), m, authorAt(m, "/"), sid, env.Commit, ptr); !ok {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(status)
			_, _ = w.Write([]byte(body))
			return
		}
	}
	data, err := s.entityPanel(r, kind, id, claim.Slot, lang, tr, &entityPost{Slot: claim.Slot, Env: env, Title: subjectTitle})
	if err != nil {
		http.NotFound(w, r)
		return
	}
	var buf bytes.Buffer
	if err := s.pageSet(lang).ExecuteTemplate(&buf, "entity-panel.html.tmpl", data); err != nil {
		log.Printf("verso: entity panel: %v", err)
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if env.Status == http.StatusUnprocessableEntity {
		// The refusal is the plugin's; the status is what says nothing was
		// written, so the stage and the browser both read it as a failed save.
		w.WriteHeader(http.StatusUnprocessableEntity)
	} else if len(env.Commit) > 0 {
		// The editor stays open; refresh roster labels and filter counts when
		// it closes, including a policy-only device whose last limit was removed.
		w.Header().Set("HX-Trigger", "verso-entity-saved")
	}
	_, _ = w.Write(buf.Bytes())
}

// entityNotice answers a submission the panel could not carry out with the
// sentence saying so, in the panel's own frame — the drawer stays open and says
// what happened rather than closing on a blank swap.
func (s *Server) entityNotice(w http.ResponseWriter, status int, text string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `<div class="px-10 py-6"><p class="text-sm leading-snug text-crimson-deep">%s</p></div>`, template.HTMLEscapeString(text))
}
