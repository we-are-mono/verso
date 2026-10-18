// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"html/template"
	"log"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"

	"github.com/we-are-mono/verso/internal/plugin"
	"github.com/we-are-mono/verso/internal/widget"
)

// contributionQuery names, on a page's address, the contribution a form on
// it posts to: <plugin id>.<contribution id>.
const contributionQuery = "contribution"

// slotDepth is how deep slots are filled: a page's own, and those of a
// contribution standing in them. A contribution's answer standing in those
// opens nothing, which bounds one page's fan-out and makes a loop of
// plugins filling each other impossible.
const slotDepth = 2

// pluginCall is how one answer is asked of a plugin on a page's behalf
// (ADR-026): as the page itself (the zero value), or as a contribution to a
// slot on it, of the slot's kind.
type pluginCall struct {
	// depth is 0 for the page, 1 for a contribution to its slots, 2 for one
	// to theirs.
	depth int
	// slot and kind are the slot this answer fills, sent as X-Verso-Slot,
	// and what it must be.
	slot, kind string
	// page is the address of the page being drawn: where a contribution's
	// forms post, naming the contribution, and where a save sends the
	// operator back to.
	page string
	// name is the contribution's full id.
	name string
	// post is the request a form on the page sent, kept for the one
	// contribution it names while the page and every other contribution are
	// asked as on a visit.
	post *http.Request
	// outcome is shared by every call of one page: the answer of the
	// contribution the post went to.
	outcome *contributionOutcome
	// facts receives a fact contribution's rows, in its own plugin's words,
	// instead of a rendering.
	facts *[]widget.Property
	// card receives an Overview card contribution, header and body, in its
	// own plugin's words, for the Overview to draw as its own.
	card *widget.OverviewContribution
}

// contributionOutcome is what the contribution a page's form posted to
// answered: whether it staged or ran something, what it says, and its
// rendering for a preview.
type contributionOutcome struct {
	mu     sync.Mutex
	posted bool
	hdr    pageHeader
	body   template.HTML
	code   int
}

// redirects reports whether the post changed something the page shows, so
// the page is read again rather than drawn as the post's answer, and a
// reload never posts it twice.
func (o *contributionOutcome) redirects() bool {
	return o.posted && (o.hdr.CommandDone || (o.hdr.StagedCommit && o.code < http.StatusBadRequest))
}

// takesKind reports whether w is an answer a slot of kind takes. The kinds
// keep the closed vocabulary where it is drawn: a facts list takes rows, an
// act row an act, never a form.
func takesKind(kind string, w widget.Widget) bool {
	switch kind {
	case "block":
		// A block stands where a page's own would: it may be anything a page
		// holds.
		return w != nil
	case "act":
		link, ok := w.(*widget.Link)
		return ok && link.Icon != ""
	case "fact":
		_, ok := w.(*widget.Properties)
		return ok
	case "notice":
		_, ok := w.(*widget.Callout)
		return ok
	case "overview-card":
		// A card's body reads; it never asks, so it holds no form.
		form := false
		widget.Walk(w, func(n widget.Widget) {
			if _, ok := n.(*widget.Form); ok {
				form = true
			}
		})
		return !form
	}
	return false
}

// slotJob is one contribution to ask for one opened slot.
type slotJob struct {
	slot      openedSlot
	placement slotPlacement
	html      template.HTML
	rows      []widget.Property
	card      widget.OverviewContribution
}

// fillOpenedSlots fills each slot the opener's page opens with what plugins
// contribute to it, asking them all at once, each within the transport's
// timeout. Nothing is asked for a slot the page does not open.
func (s *Server) fillOpenedSlots(r *http.Request, opener string, page widget.Widget, call pluginCall) {
	if call.depth >= slotDepth {
		return
	}
	var jobs []*slotJob
	for _, slot := range openedSlots(opener, page) {
		for _, entry := range orderSlot(nil, s.slotFills(slot.Name)) {
			jobs = append(jobs, &slotJob{slot: slot, placement: *entry.Placement})
		}
	}
	s.askAll(r, jobs, call)
	byName := map[string][]*slotJob{}
	for _, job := range jobs {
		byName[job.slot.Name] = append(byName[job.slot.Name], job)
	}
	for _, slot := range openedSlots(opener, page) {
		var html strings.Builder
		for _, job := range byName[slot.Name] {
			if slot.Facts != nil {
				slot.Facts.Join(job.rows)
				continue
			}
			html.WriteString(string(job.html))
		}
		if slot.Slot != nil && html.Len() > 0 {
			slot.Slot.Fill(&widget.Embed{HTML: template.HTML(html.String())}) //nolint:gosec // rendered by the shell's own templates
		}
	}
}

// askAll asks every job's plugin at once and returns when all have
// answered, each within the transport's timeout.
func (s *Server) askAll(r *http.Request, jobs []*slotJob, call pluginCall) {
	var wg sync.WaitGroup
	for _, job := range jobs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.askContribution(r, job, call)
		}()
	}
	wg.Wait()
}

// shellSlotBlocks is a shell page's blocks slot drawn in order: its own
// blocks by anchor (an anchor with no block draws nothing) and each
// contribution to the slot where its position puts it, all asked at once.
func (s *Server) shellSlotBlocks(r *http.Request, call pluginCall, name string, own map[string]widget.Widget) []widget.Widget {
	slot, _ := shellSlotNamed(name)
	entries := orderSlot(slot.Anchors, s.slotFills(name))
	jobs := map[int]*slotJob{}
	var asked []*slotJob
	for i, e := range entries {
		if e.Placement != nil {
			job := &slotJob{slot: openedSlot{Name: name, Kind: slot.Kind}, placement: *e.Placement}
			jobs[i], asked = job, append(asked, job)
		}
	}
	s.askAll(r, asked, call)
	var out []widget.Widget
	for i, e := range entries {
		switch {
		case e.Placement == nil && own[e.Anchor] != nil:
			out = append(out, own[e.Anchor])
		case e.Placement != nil && jobs[i].html != "":
			out = append(out, &widget.Embed{HTML: jobs[i].html})
		}
	}
	return out
}

// askContribution asks one plugin for its contribution to a slot on the
// page and keeps what it answered: its rendering, or a fact's rows.
func (s *Server) askContribution(r *http.Request, job *slotJob, call pluginCall) {
	m, ok := s.manifestByID(job.placement.Plugin)
	if !ok {
		return
	}
	name := job.placement.Name()
	request := r.Clone(r.Context())
	request.Method, request.Body, request.Form, request.PostForm = http.MethodGet, nil, nil, nil
	if call.post != nil && call.post.URL.Query().Get(contributionQuery) == name {
		request = call.post.Clone(call.post.Context())
	}
	var rows []widget.Property
	sub := pluginCall{
		depth: call.depth + 1, slot: job.slot.Name, kind: job.slot.Kind,
		page: call.page, name: name, post: call.post, outcome: call.outcome,
	}
	var card widget.OverviewContribution
	switch job.slot.Kind {
	case "fact":
		sub.facts = &rows
	case "overview-card":
		sub.card = &card
	}
	var hdr pageHeader
	var width string
	var pages []pageTab
	body, code := s.pluginBodyFor(request, m, job.placement.Contribution.Path, &hdr, &width, &pages, sub)
	if request.Method == http.MethodPost && call.outcome != nil {
		call.outcome.mu.Lock()
		call.outcome.posted, call.outcome.hdr, call.outcome.body, call.outcome.code = true, hdr, body, code
		call.outcome.mu.Unlock()
	}
	switch {
	case code == http.StatusNoContent:
		return
	case hdr.Unreachable && !s.probe(m.Endpoint()):
		// The plugin is not running: it leaves no trace on the page.
		return
	case hdr.Unreachable && job.slot.Kind != "block" && job.slot.Kind != "notice":
		// No notice fits an act row, a facts list or a card's place; the
		// failure is the log's.
		return
	}
	job.html, job.rows, job.card = body, rows, card
}

// contributionPost is the page's own request and its call. A form that
// posts to a contribution on the page leaves the page and every other
// contribution asked as on a visit; the post is kept for the one it names.
func contributionPost(r *http.Request) (*http.Request, pluginCall) {
	call := pluginCall{page: r.URL.Path}
	if safeMethod(r.Method) || r.URL.Query().Get(contributionQuery) == "" {
		return r, call
	}
	call.post, call.outcome = r, &contributionOutcome{}
	page := r.Clone(r.Context())
	page.Method, page.Body, page.Form, page.PostForm = http.MethodGet, nil, nil, nil
	return page, call
}

// answerContributionPost answers a post to a contribution when its answer
// is the whole response: a live preview's reading alone, since the page
// around the box is on screen, or, when the post staged or ran something,
// the page read again with its outcome said once. It reports whether it
// answered.
func (s *Server) answerContributionPost(w http.ResponseWriter, r *http.Request, o *contributionOutcome) bool {
	if o == nil || !o.posted {
		return false
	}
	if previewRequest(r) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(o.code)
		_, _ = w.Write([]byte(o.body))
		return true
	}
	if o.redirects() {
		s.flashNotice(r, o.hdr.Notice)
		http.Redirect(w, r, r.URL.Path, http.StatusSeeOther)
		return true
	}
	return false
}

// drawerQuery names, on a page's address, the drawer the page opens on load:
// one an act on the page leads to.
const drawerQuery = "drawer"

// drawerLanding is the address of the page a drawer opens over, asking it
// to open the drawer at its own address.
func drawerLanding(page, drawer string) string {
	u, err := url.Parse(widget.SafeHref(page))
	if err != nil || u.Path == "" {
		return "/"
	}
	q := u.Query()
	q.Set(drawerQuery, drawer)
	u.RawQuery = q.Encode()
	return u.String()
}

// emptyPage is a page whose body draws nothing: an empty stack.
func emptyPage(w widget.Widget) bool {
	stack, ok := w.(*widget.Stack)
	return ok && len(stack.Children) == 0
}

// postBack is the address a contribution's forms post to: the page being
// drawn, naming the contribution.
func (c pluginCall) postBack() string {
	u := url.URL{Path: c.page, RawQuery: url.Values{contributionQuery: {c.name}}.Encode()}
	return u.String()
}

// answerFate is what becomes of a contribution's answer.
type answerFate int

const (
	// answerDrawn: the answer is rendered where its slot stands.
	answerDrawn answerFate = iota
	// answerLifted: a fact's rows were taken out to join the host's list.
	answerLifted
	// answerRefused: the answer is not what its slot takes.
	answerRefused
)

// inspectContribution checks a contribution's answer against its slot's
// kind, and lifts a fact's rows out in its own plugin's words.
func inspectContribution(m plugin.Manifest, call pluginCall, w widget.Widget, card *plugin.OverviewCard, t func(string) string) answerFate {
	if !takesKind(call.kind, w) || (call.kind == "overview-card" && (card == nil || card.Title == "")) {
		log.Printf("verso: compatibility: plugin %q contribution %q answered what %s (%s) does not take; it is not drawn", m.ID, call.name, call.slot, call.kind)
		return answerRefused
	}
	switch {
	case call.facts != nil:
		facts := w.(*widget.Properties)
		widget.Translate(facts, t)
		*call.facts = facts.Items
		return answerLifted
	case call.card != nil:
		widget.Translate(w, t)
		tr := translatorOrIdentity(t)
		*call.card = widget.OverviewContribution{
			ID: call.name, Title: tr(card.Title), Icon: card.Icon,
			Tone: overviewTone(card.Tone), Status: tr(card.Status), Summary: tr(card.Summary), Cause: tr(card.Cause),
			Href: cardDoor(m.ID, card.Opens), Body: w,
		}
		return answerLifted
	}
	return answerDrawn
}

// overviewTone is a card's tone from the shell's vocabulary, neutral for
// any other word.
func overviewTone(tone string) string {
	switch tone {
	case "success", "warning", "danger":
		return tone
	}
	return "neutral"
}

// cardDoor is where a plugin card's door leads, a path under its plugin's
// mount, or nowhere for none or for one that steps out of it.
func cardDoor(id, opens string) string {
	if opens == "" || !strings.HasPrefix(opens, "/") || strings.ContainsAny(opens, "\\") || path.Clean(opens) != opens {
		return ""
	}
	return pluginHref(id, opens)
}
