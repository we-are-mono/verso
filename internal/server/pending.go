// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"slices"
	"strings"

	"github.com/we-are-mono/verso/internal/clock"
	"github.com/we-are-mono/verso/internal/history"
	"github.com/we-are-mono/verso/internal/i18n"
	"github.com/we-are-mono/verso/internal/widget"
)

// pendingPath is the Pending changes page: the stage read back by the page
// each change was made on, what applying it does, and its acts (ADR-010).
const pendingPath = "/pending"

// pendingStep is one of the When you apply card's steps, with its number.
type pendingStep struct {
	applyStep
	Number int
}

// handlePending renders the Pending changes page: "N pending changes" with
// Discard all and Apply on its heading line; the When you apply card, opened
// by the warning where a change may cut this browser off, then the apply's
// steps and the note; then a card per page the changes were made on. With
// nothing pending it says how the stage last ended and leads back to the page
// of its latest change.
func (s *Server) handlePending(w http.ResponseWriter, r *http.Request) {
	lang, t := s.localize(r)
	tr := translatorOrIdentity(t)
	ctx := r.Context()
	sid := s.sessionSID(r)
	csrf := s.sessionCSRF(r)
	view := s.staged(ctx, sid, tr, s.pluginTranslators(r))
	end, ended := s.stageEnds.of(sid)
	if view.Count == 0 {
		s.renderPendingEmpty(w, r, end, ended)
		return
	}
	s.withWas(ctx, sid, &view, tr)
	hdr := pageHeader{
		Heading: i18n.Count(tr, view.Count, "%d pending change", "%d pending changes (two)", "%d pending changes (few)", "%d pending changes"),
		Tone:    "neutral",
	}
	if record, ok := s.undos.announce(sid); ok {
		hdr.Arrival = s.pendingTemplate(lang, "verso-pending-putback", struct {
			Words, Token, CSRF string
			Restorable         bool
		}{record.words, record.token, csrf, record.restorable})
	}
	var discard strings.Builder
	if err := s.widgets.RenderWithToken(&discard, discardConfirm(), csrf, lang, t); err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	hdr.HeadingAct = s.pendingTemplate(lang, "verso-pending-acts", struct {
		CSRF, Apply string
		Discard     template.HTML
	}{csrf, stagedApplyLabel(view.Count, tr), template.HTML(discard.String())}) //nolint:gosec // the shell's own rendered widget

	var blocks widget.Widgets
	if r.URL.Query().Get("confirm") == "discard" {
		blocks = append(blocks, &widget.Form{Action: pendingPath + "/discard", NoSubmit: true, Fields: widget.Widgets{
			&widget.Callout{Variant: "danger", Title: "Discard all pending changes?", Body: discardConfirm().Message,
				Acts: []widget.Link{{Label: "Not now", Href: pendingPath}}},
			&widget.Button{Label: "Discard all", Name: "discard", Value: "1", Style: "danger"},
		}})
	}
	if ended && end.Kind == stageRolledBack {
		blocks = append(blocks, &widget.Callout{Variant: "danger",
			Title: fmt.Sprintf(tr("The last apply was put back at %s."), clock.Local(end.At).Format("15:04")),
			Body:  "This page could not check in with the router within the rollback window, so the router returned to its previous settings. Your changes are still here, as you left them."})
	}
	blocks = append(blocks, s.whenYouApply(r, sid, view, lang, tr))
	// Each row reads as config history will read it once applied, with what
	// it replaces: the stage as config history keeps it, in the view's order.
	kept := s.stagedChangeOf(ctx, sid, view, "", "")
	next := 0
	for _, g := range view.Groups {
		items := kept.Items[next:min(next+len(g.Items), len(kept.Items))]
		next += len(g.Items)
		blocks = append(blocks, pendingCard(g, items, kept.OldRead, tr))
	}
	var body strings.Builder
	if err := s.widgets.RenderWithToken(&body, &widget.Stack{Children: blocks}, csrf, lang, t); err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	s.renderPage(w, r, http.StatusOK, hdr, "", nil, template.HTML(body.String())) //nolint:gosec // rendered by the shell's own templates
}

// whenYouApply is the card that says what an apply does: the warning where a
// change may cut this browser off, the apply's steps, and the note config
// history keeps with it (ADR-021), in the form the heading's Apply posts.
func (s *Server) whenYouApply(r *http.Request, sid string, view stagedView, lang string, tr func(string) string) widget.Widget {
	ctx := r.Context()
	var children widget.Widgets
	if warning := s.disconnectRisk(ctx, r, sid, view); warning != nil {
		body := fmt.Sprintf(tr("A pending change to %s may cut off this browser, which reaches the router at %s."), warning.Interface, warning.From)
		if warning.To != "" {
			body = fmt.Sprintf(tr("The %s address moves from %s to %s, and %s is the address this browser uses."), strings.ToUpper(warning.Interface), warning.From, warning.To, warning.From)
		}
		children = append(children, &widget.Callout{Variant: "warning", Title: "May disconnect you.", Body: body})
	}
	triggers, err := s.backend.ReloadTriggers(ctx, sid)
	if err != nil {
		triggers = nil
	}
	var steps []pendingStep
	for i, step := range applySteps(view.Count, stagedConfigs(view), triggers, tr) {
		steps = append(steps, pendingStep{applyStep: step, Number: i + 1})
	}
	children = append(children,
		&widget.Embed{HTML: s.pendingTemplate(lang, "verso-pending-steps", steps)},
		&widget.Form{ID: "verso-pending-apply", Action: "/uci/apply", NoSubmit: true, Fields: widget.Widgets{
			&widget.Field{Name: "note", Label: "Note", Help: "Optional. Config history keeps it with the apply.", Placeholder: view.Summary},
		}},
	)
	return &widget.Section{Title: "When you apply", Children: children}
}

// pendingCard is one page's changes: titled by the page's heading, its acts
// the way to the page and, for more than one row, undoing them all; a row
// per change reading as config history's rows do.
func pendingCard(g stagedGroup, kept []history.ChangeItem, oldRead bool, tr func(string) string) widget.Widget {
	var acts widget.Widgets
	if g.Href != "" {
		acts = append(acts, &widget.Link{Label: "Open page", Icon: "arrow-up-right", Href: g.Href, Style: "act"})
	}
	if len(g.Items) > 1 {
		undo := &widget.Form{Action: pendingPath + "/undo", NoSubmit: true, Style: "inline"}
		for _, item := range g.Items {
			undo.Fields = append(undo.Fields, &widget.Field{Name: "row", Kind: "hidden", Value: item.ID})
		}
		undo.Fields = append(undo.Fields, &widget.Button{Label: "Undo these", Icon: "undo-2", Name: "undo", Value: "1", Style: "act"})
		acts = append(acts, undo)
	}
	table := &widget.Table{Columns: []widget.TableColumn{
		{Label: "Changed", Kind: "phrase"},
		{Label: "Staged", Width: widget.MeasureShort},
		{Label: "Status", Kind: "status", Width: widget.MeasureWord},
		{Kind: "actions", Width: widget.MeasureShort},
	}}
	for i, item := range g.Items {
		var as history.ChangeItem
		if i < len(kept) {
			as = kept[i]
		}
		table.Rows = append(table.Rows, pendingRow(item, as, oldRead, g.Href, tr))
	}
	section := &widget.Section{Title: g.Heading, Children: widget.Widgets{table}}
	if len(acts) > 0 {
		section.Control = &widget.Stack{Inline: true, Children: acts}
	}
	return section
}

// pendingVariant is a row's status tone by what happened: a change waits in
// the caveat colour, an addition in green, a removal in red, a new order in
// the info tone.
var pendingVariant = map[string]string{"changed": "warning", "added": "success", "removed": "danger", "reordered": "info"}

// pendingWord is a row's status, the word for what happened.
var pendingWord = map[string]string{"changed": "Changed", "added": "Added", "removed": "Removed", "reordered": "Reordered"}

// pendingRow is one change: what it is, as config history says a change;
// how long ago it was staged; what happened; and its acts — to its place on
// its page, its undo, and the config lines it stands for.
func pendingRow(item stagedItem, kept history.ChangeItem, oldRead bool, href string, tr func(string) string) widget.TableRow {
	parts, plain := changePhrase(history.Change{Items: []history.ChangeItem{kept}, OldRead: oldRead}, tr)
	if item.Setting != nil {
		parts, plain = settingParts(item)
	}
	var acts []widget.TableRowAct
	if href != "" {
		goTo := href
		if item.Anchor != "" {
			goTo += "#" + item.Anchor
		}
		acts = append(acts, widget.TableRowAct{Icon: "arrow-up-right", Title: "Go to it on its page", Href: goTo})
	}
	acts = append(acts,
		widget.TableRowAct{Icon: "undo-2", Title: "Undo this change", Name: "row", Value: item.ID, Post: pendingPath + "/undo"},
		widget.TableRowAct{Icon: "info", Title: "Show the config lines", Opens: true},
	)
	var lines []string
	for _, l := range item.Lines {
		lines = append(lines, l.Op+" "+l.Text)
	}
	return widget.TableRow{
		ID: item.ID,
		Drawer: &widget.RowDrawer{Title: "Config lines", Children: widget.Widgets{
			&widget.Code{Value: strings.Join(lines, "\n"), Copy: true},
		}},
		Cells: []widget.TableCell{
			{Text: plain, Parts: parts, Verbatim: true},
			{Text: item.Age, Verbatim: true, Muted: true},
			{Text: tr(pendingWord[item.Verb]), Variant: pendingVariant[item.Verb], Verbatim: true},
			{Actions: acts},
		},
	}
}

// settingParts is a change its plugin described as one of its settings, read
// the way its settings row does: the setting's name, the option's key chip,
// what it held, and what it now holds — the machine's in mono, words in sans.
func settingParts(item stagedItem) ([]widget.TablePart, string) {
	said := func(text string, verbatim bool) widget.TablePart {
		if verbatim {
			return widget.TablePart{Value: text}
		}
		return widget.TablePart{Words: text}
	}
	parts := []widget.TablePart{{Words: item.Setting.Label}}
	if item.Key != "" {
		parts = append(parts, widget.TablePart{Chip: &widget.TableChip{Label: item.Key}})
	}
	if item.Was != nil {
		parts = append(parts, said(item.Was.Text, item.Was.Verbatim), widget.TablePart{Words: "→"})
	}
	parts = append(parts, said(item.Setting.Value, item.Setting.Verbatim))
	return parts, item.Setting.Label + " " + item.Setting.Value
}

// renderPendingEmpty is the page with nothing pending: how the stage last
// ended, and the way back to the page of its latest change where one is on
// record.
func (s *Server) renderPendingEmpty(w http.ResponseWriter, r *http.Request, end stageEnd, ended bool) {
	lang, t := s.localize(r)
	tr := translatorOrIdentity(t)
	heading := "Nothing pending"
	text := tr("Every save waits here until you apply it. Nothing is waiting, so the router runs what its pages show.")
	if ended && end.Kind == stageApplied {
		heading = "Changes applied"
		text = fmt.Sprintf(tr("The router kept %s at %s. What it reloaded can take a few more seconds to settle."),
			i18n.Count(tr, end.Count, "%d change", "%d changes (two)", "%d changes (few)", "%d changes"), clock.Local(end.At).Format("15:04"))
	}
	children := widget.Widgets{&widget.Text{Markdown: text, Translated: true}}
	if ended && end.Back.Href != "" {
		name := end.Back.Heading
		if name == "" {
			name = end.Back.Page
		}
		if end.Back.Plugin != "" && !end.Back.Shell {
			name = s.pluginTranslators(r)(end.Back.Plugin)(name)
		} else {
			name = tr(name)
		}
		children = append(children, &widget.Link{Label: fmt.Sprintf(tr("Back to %s"), name), Icon: "arrow-left", Href: end.Back.Href, Style: "secondary"})
	}
	var body strings.Builder
	if err := s.widgets.RenderWithToken(&body, &widget.Section{Children: children}, s.sessionCSRF(r), lang, t); err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	s.renderPage(w, r, http.StatusOK, pageHeader{Heading: heading, Tone: "neutral"}, "", nil, template.HTML(body.String())) //nolint:gosec // rendered by the shell's own templates
}

// pendingTemplate executes one of the page's own parts.
func (s *Server) pendingTemplate(lang, name string, data any) template.HTML {
	var buf bytes.Buffer
	if err := s.pageSet(lang).ExecuteTemplate(&buf, name, data); err != nil {
		log.Printf("verso: pending page part %s: %v", name, err)
		return ""
	}
	return template.HTML(buf.String()) //nolint:gosec // the shell's own template
}

// stagedConfigs is every config the view's changes touch, sorted.
func stagedConfigs(view stagedView) []string {
	var configs []string
	for _, g := range view.Groups {
		for _, item := range g.Items {
			for _, c := range item.changes {
				if !slices.Contains(configs, c.config) {
					configs = append(configs, c.config)
				}
			}
		}
	}
	slices.Sort(configs)
	return configs
}

// handlePendingDiscard lets the whole stage go, from the page's Discard all,
// and leads back to the page.
func (s *Server) handlePendingDiscard(w http.ResponseWriter, r *http.Request) {
	_, t := s.localize(r)
	tr := translatorOrIdentity(t)
	if n, err := s.discardStage(r.Context(), s.sessionSID(r)); err != nil {
		log.Printf("verso: discard failed: %v", err)
		s.flash(r, "danger", tr("The router did not let the changes go. Try again."))
	} else if n > 0 {
		s.flash(r, "info", i18n.Count(tr, n, "%d change discarded.", "%d changes discarded. (two)", "%d changes discarded. (few)", "%d changes discarded."))
	}
	http.Redirect(w, r, pendingPath, http.StatusSeeOther)
}

// handleUCIReview is the review drawer's former address, kept for a bookmark:
// it leads to the Pending changes page.
func (s *Server) handleUCIReview(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, pendingPath, http.StatusPermanentRedirect)
}

// handlePendingUndo takes the posted rows out of the stage, a row's undo or a
// card's Undo these, and leads back to the page, which offers them back.
func (s *Server) handlePendingUndo(w http.ResponseWriter, r *http.Request) {
	_, t := s.localize(r)
	tr := translatorOrIdentity(t)
	sid := s.sessionSID(r)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	ids := r.Form["row"]
	words := s.rowWordsOf(r, sid, ids)
	ops, err := s.undoRows(r.Context(), sid, ids)
	switch {
	case errors.Is(err, errRowsGone):
		s.flash(r, "warning", tr("Those changes were no longer pending."))
	case err != nil:
		log.Printf("verso: undo failed: %v", err)
		s.flash(r, "danger", tr("The router did not take the undo, so the changes stay as they were."))
	default:
		record := undoRecord{ops: ops, words: words, token: randomToken()}
		record.restorable = slices.ContainsFunc(ops, func(op replayOp) bool { return op.tuple[0] != "file" })
		s.undos.set(sid, record)
	}
	http.Redirect(w, r, pendingPath, http.StatusSeeOther)
}

// handlePendingRestore stages again what the session's last undo took out,
// when the form carries that undo's token, and leads back to the page.
func (s *Server) handlePendingRestore(w http.ResponseWriter, r *http.Request) {
	_, t := s.localize(r)
	tr := translatorOrIdentity(t)
	sid := s.sessionSID(r)
	if record, ok := s.undos.take(sid, r.FormValue("undo")); ok {
		if err := s.putBack(r.Context(), sid, record.ops); err != nil {
			log.Printf("verso: put back failed: %v", err)
			s.flash(r, "danger", tr("The router did not take it back. Make the change again on its page."))
		}
	}
	http.Redirect(w, r, pendingPath, http.StatusSeeOther)
}

// rowWordsOf is what an undo of ids says it took, in the reader's language:
// the row's own words for one, a count for several.
func (s *Server) rowWordsOf(r *http.Request, sid string, ids []string) string {
	_, t := s.localize(r)
	tr := translatorOrIdentity(t)
	view := s.staged(r.Context(), sid, tr, s.pluginTranslators(r))
	var found []string
	for _, g := range view.Groups {
		for _, item := range g.Items {
			if slices.Contains(ids, item.ID) {
				found = append(found, rowWords(item))
			}
		}
	}
	if len(found) == 1 {
		return fmt.Sprintf(tr("Undone: %s."), found[0])
	}
	return i18n.Count(tr, len(found), "Undone: %d change.", "Undone: %d changes. (two)", "Undone: %d changes. (few)", "Undone: %d changes.")
}
