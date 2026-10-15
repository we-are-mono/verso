// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/we-are-mono/verso/internal/clock"
	"github.com/we-are-mono/verso/internal/history"
	"github.com/we-are-mono/verso/internal/widget"
)

// changesPath is System → Changes, config history's page (ADR-021); an
// entry's drawer is read from changesPath/{id}.
const changesPath = "/system/changes"

// changesListed is how many entries the page lists: every one history keeps.
const changesListed = 1000

// subjectIcons is the glyph each kind of subject draws with (plugin.SubjectKinds).
var subjectIcons = map[string]string{
	"rule":         "shield",
	"port-forward": "corner-down-right",
	"zone":         "zone",
	"crossing":     "arrow-right",
	"network":      "network",
	"route":        "route",
	"device":       "device",
	"wifi":         "wifi",
	"dhcp":         "server",
	"service":      "settings",
	"setting":      "sliders-horizontal",
}

// handleChanges lists every apply config history keeps, newest first, in
// lanes by day (ADR-021).
func (s *Server) handleChanges(w http.ResponseWriter, r *http.Request) {
	if s.historyStore == nil {
		http.NotFound(w, r)
		return
	}
	lang, t := s.localize(r)
	tr := translatorOrIdentity(t)
	changes, err := s.historyStore.Changes(r.Context(), changesListed)
	if err != nil {
		http.Error(w, "history unavailable", http.StatusInternalServerError)
		return
	}
	var startAt time.Time
	if start, err := s.historyStore.StartingPoint(r.Context()); err == nil && len(start.Configs) > 0 {
		startAt = start.Time
	}
	table := changesTable(changes, startAt, time.Now(), clientIP(r), tr)
	page := &widget.Stack{Children: []widget.Widget{table}}
	if h := s.historyStore.Health(); !h.OK {
		page.Children = []widget.Widget{&widget.Callout{Variant: "warning", Compact: true, Verbatim: h.Cause,
			Body: "History is not being saved. Changes applied now are kept in memory until the storage answers again."}, table}
	}
	var body strings.Builder
	if err := s.widgets.RenderWithToken(&body, page, s.sessionCSRF(r), lang, t); err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	s.renderPage(w, r, http.StatusOK, pageHeader{Heading: "Changes", Tone: "neutral"}, "wide",
		s.sectionPages("System", r.URL.Path), template.HTML(body.String())) //nolint:gosec // rendered by the shell's own templates
}

// handleChangePanel is one entry's drawer: who applied it from where, how it
// stands, and what it changed.
func (s *Server) handleChangePanel(w http.ResponseWriter, r *http.Request) {
	if s.historyStore == nil {
		http.NotFound(w, r)
		return
	}
	lang, t := s.localize(r)
	tr := translatorOrIdentity(t)
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if !panelRequest(r) && r.Method == http.MethodGet {
		http.Redirect(w, r, changesPath, http.StatusSeeOther)
		return
	}
	all, err := s.historyStore.Changes(r.Context(), changesListed)
	if err != nil {
		s.entityNotice(w, http.StatusInternalServerError, fmt.Sprintf(tr("The history could not be read (%v)."), err))
		return
	}
	i := slices.IndexFunc(all, func(c history.Change) bool { return c.ID == id })
	if i < 0 {
		s.entityNotice(w, http.StatusNotFound, tr("This change is no longer kept."))
		return
	}
	c, since, status := all[i], r.URL.Query().Get("tab") == "since", http.StatusOK
	var failed string
	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		act := r.PostForm.Get("act")
		// Staged, so not yet done: the page comes back with the chip saying
		// a change waits.
		if failed = s.changeAct(r.Context(), s.sessionSID(r), act, c, all, tr); failed == "" {
			if panelRequest(r) {
				w.Header().Set("HX-Redirect", changesPath)
				w.WriteHeader(http.StatusOK)
				return
			}
			http.Redirect(w, r, changesPath, http.StatusSeeOther)
			return
		}
		since, status = act == "return", http.StatusConflict
	}
	s.writeChangeDrawer(w, r, strconv.FormatInt(id, 10), changeDrawer(c, all, since, time.Now(), tr), failed, status, lang, t)
}

// writeChangeDrawer answers with one drawer of config history, open, a
// refusal of what was asked of it above its contents.
func (s *Server) writeChangeDrawer(w http.ResponseWriter, r *http.Request, id string, drawer *widget.RowDrawer, failed string, status int, lang string, t func(string) string) {
	if failed != "" {
		drawer.Children = append(widget.Widgets{&widget.Callout{Variant: "danger", Compact: true, Body: failed}}, drawer.Children...)
	}
	drawer.Open = true
	var body bytes.Buffer
	if _, err := s.widgets.RenderOpenPanelWithToken(&body, &widget.Table{Columns: []widget.TableColumn{{}}, Rows: []widget.TableRow{{ID: id, Drawer: drawer}}},
		s.sessionCSRF(r), lang, t, widget.Flash{}); err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body.Bytes())
}

// handleStartPanel is the starting point's drawer: what is set differently
// now than before the oldest entry kept, and Return to the starting point.
// Posted to, it stages that return.
func (s *Server) handleStartPanel(w http.ResponseWriter, r *http.Request) {
	if s.historyStore == nil {
		http.NotFound(w, r)
		return
	}
	lang, t := s.localize(r)
	tr := translatorOrIdentity(t)
	if !panelRequest(r) && r.Method == http.MethodGet {
		http.Redirect(w, r, changesPath, http.StatusSeeOther)
		return
	}
	start, now, err := s.readStartAndNow(r.Context())
	if err != nil {
		s.entityNotice(w, http.StatusInternalServerError, fmt.Sprintf(tr("The history could not be read (%v)."), err))
		return
	}
	plan := startPlan(start.sections, now)
	status, failed := http.StatusOK, ""
	if r.Method == http.MethodPost {
		all, err := s.historyStore.Changes(r.Context(), changesListed)
		switch {
		case err != nil:
			failed = fmt.Sprintf(tr("The history could not be read (%v)."), err)
		case r.FormValue("act") != "return":
			failed = tr("That is not something a change can do.")
		case len(plan.Steps) == 0:
			failed = tr("There is nothing to stage.")
		case !s.stagePlan(r.Context(), s.sessionSID(r), plan, tr):
			failed = tr("The changes could not be staged. Some may have been: review or discard them from the staged changes.")
		default:
			s.relations.set(s.sessionSID(r), stageRelation{relation: "return", reverses: inForceAfter(all, history.Change{})})
			if panelRequest(r) {
				w.Header().Set("HX-Redirect", changesPath)
				w.WriteHeader(http.StatusOK)
				return
			}
			http.Redirect(w, r, changesPath, http.StatusSeeOther)
			return
		}
		status = http.StatusConflict
	}
	drawer := &widget.RowDrawer{Title: fmt.Sprintf(tr("Starting point · %s"), changeTitle(start.at, time.Now(), tr)), Verbatim: true,
		Children: returnBody(plan, startPath, "Return to the starting point", "At the starting point", tr)}
	s.writeChangeDrawer(w, r, "start", drawer, failed, status, lang, t)
}

// startReading is the starting point read for a return.
type startReading struct {
	at       time.Time
	sections map[string]map[string]map[string]any
}

// readStartAndNow is the starting point, and every config as last read.
func (s *Server) readStartAndNow(ctx context.Context) (startReading, map[string]map[string]map[string]any, error) {
	start, err := s.historyStore.StartingPoint(ctx)
	if err != nil {
		return startReading{}, nil, err
	}
	states, err := s.historyStore.ConfigStates(ctx)
	if err != nil {
		return startReading{}, nil, err
	}
	decode := func(raw map[string]json.RawMessage) (map[string]map[string]map[string]any, error) {
		out := make(map[string]map[string]map[string]any, len(raw))
		for config, text := range raw {
			var sections map[string]map[string]any
			if err := json.Unmarshal(text, &sections); err != nil {
				return nil, err
			}
			out[config] = sections
		}
		return out, nil
	}
	then, err := decode(start.Configs)
	if err != nil {
		return startReading{}, nil, err
	}
	now, err := decode(states)
	return startReading{at: start.Time, sections: then}, now, err
}

// changesTable is the listing: lanes by day, a row per entry, named from
// what it touched. An entry no longer in force (rolled back, undone) is
// marked over, as a switched-off rule is. viewer is the reader's address, so
// their own applies say so.
//
// The starting point closes the listing where history has one (startAt is
// its moment): the configs as they stood before the oldest entry kept.
func changesTable(changes []history.Change, startAt, now time.Time, viewer string, tr func(string) string) *widget.Table {
	table := &widget.Table{
		Columns: []widget.TableColumn{
			{Label: "When", Width: widget.MeasureShort},
			{Label: "Changed", Kind: "phrase"},
			{Label: "Notes", Kind: "comment", Width: widget.MeasureName},
			{Label: "By", Kind: "phrase", Width: widget.MeasureName},
			{Label: "Status", Kind: "status", Width: widget.MeasureWord},
			{Kind: "actions", Width: widget.MeasureShort},
		},
		EmptyText: "Nothing has been applied yet. Every apply is kept here, with what it replaced.",
	}
	when := make(map[int64]time.Time, len(changes))
	for _, c := range changes {
		when[c.ID] = c.Time
	}
	rows := make([]widget.TableRow, 0, len(changes)+1)
	times := make([]time.Time, 0, len(changes)+1)
	for _, c := range changes {
		rows, times = append(rows, changeRow(c, when, now, viewer, tr)), append(times, c.Time)
	}
	if !startAt.IsZero() {
		rows, times = append(rows, startRow(startAt, tr)), append(times, startAt)
	}
	day := ""
	for i, row := range rows {
		if d := changeDay(times[i], now, tr); d != day {
			day = d
			row.Group = &widget.TableGroup{Label: d, Verbatim: true}
		}
		table.Rows = append(table.Rows, row)
	}
	return table
}

// startPath is the starting point's drawer.
const startPath = changesPath + "/start"

// startRow is the starting point in the listing: kept, never undone, and
// opening on what is set differently now.
func startRow(at time.Time, tr func(string) string) widget.TableRow {
	return widget.TableRow{
		ID:    "start",
		Panel: startPath,
		Cells: []widget.TableCell{
			{Text: clock.Local(at).Format("15:04"), Verbatim: true},
			{Text: tr("Starting point"), Parts: []widget.TablePart{{Words: tr("Starting point")}}, Verbatim: true},
			{},
			{},
			{Text: "Kept"},
			{Actions: []widget.TableRowAct{{Icon: "info", Title: "Details", Opens: true}}},
		},
	}
}

// changeRow is one entry in the listing.
func changeRow(c history.Change, when map[int64]time.Time, now time.Time, viewer string, tr func(string) string) widget.TableRow {
	parts, plain := changePhrase(c, tr)
	// Notes is what the entry is to another, then the operator's own note.
	var notes []string
	if c.Relation == "return" && c.Related == 0 {
		notes = append(notes, tr("back to the starting point"))
	} else if related, ok := when[c.Related]; ok {
		switch c.Relation {
		case "undo":
			notes = append(notes, fmt.Sprintf(tr("undo of %s"), eventTime(related, now)))
		case "return":
			notes = append(notes, fmt.Sprintf(tr("back to %s"), eventTime(related, now)))
		case "again":
			notes = append(notes, fmt.Sprintf(tr("again, from %s"), eventTime(related, now)))
		}
	}
	if c.Note != "" {
		notes = append(notes, c.Note)
	}
	word, variant := changeStatus(c.Status)
	return widget.TableRow{
		ID:    strconv.FormatInt(c.ID, 10),
		Muted: c.Status != history.ChangeApplied,
		Panel: fmt.Sprintf("%s/%d", changesPath, c.ID),
		Cells: []widget.TableCell{
			{Text: clock.Local(c.Time).Format("15:04"), Verbatim: true},
			{Text: plain, Parts: parts, Verbatim: true},
			{Text: strings.Join(notes, " · ")},
			changeBy(c, viewer, tr),
			{Text: word, Variant: variant},
			{Actions: []widget.TableRowAct{{Icon: "info", Title: "Details", Opens: true}}},
		},
	}
}

// changeStatus is how an entry stands, in a word and the status mark's
// colour.
func changeStatus(status string) (string, string) {
	switch status {
	case history.ChangeApplied:
		return "Applied", "success"
	case history.ChangeRolledBack:
		return "Rolled back", "warning"
	}
	return "Undone", ""
}

// changeBy is who applied an entry and from where: this browser's own
// applies say so; an edit written straight to the config has no one.
func changeBy(c history.Change, viewer string, tr func(string) string) widget.TableCell {
	if c.User == "" {
		return widget.TableCell{Text: tr("Edited directly"), Parts: []widget.TablePart{{Words: tr("Edited directly")}}, Verbatim: true}
	}
	from := widget.TableChip{Icon: "device", Label: c.Address}
	if c.Address != "" && c.Address == viewer {
		from = widget.TableChip{Icon: "laptop", Label: "this browser", LocalizeLabel: true, Title: c.Address}
	}
	parts := []widget.TablePart{{Value: c.User}}
	if c.Address != "" {
		parts = append(parts, widget.TablePart{Chip: &from})
	}
	return widget.TableCell{Text: c.User, Parts: parts, Verbatim: true}
}

// changeDay is the lane an entry falls in: today, yesterday, or its date.
func changeDay(at, now time.Time, tr func(string) string) string {
	at, now = clock.Local(at), clock.Local(now)
	day := func(t time.Time) time.Time { return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location()) }
	switch {
	case day(at).Equal(day(now)):
		return tr("Today")
	case day(at).Equal(day(now).AddDate(0, 0, -1)):
		return tr("Yesterday")
	case at.Year() == now.Year():
		return at.Format("Mon 2 Jan")
	}
	return at.Format("Mon 2 Jan 2006")
}

// subject is what a row of an entry is about: the kind its plugin named and
// the name, or no kind and the config and section it changed.
type subject struct{ kind, name string }

func itemSubject(item history.ChangeItem) subject {
	if item.SubjectKind != "" {
		return subject{item.SubjectKind, item.SubjectName}
	}
	if len(item.Ops) > 0 {
		return subject{"", item.Ops[0].Config + "." + item.Ops[0].Section}
	}
	return subject{}
}

func (s subject) chip() *widget.TableChip {
	return &widget.TableChip{Icon: subjectIcons[s.kind], Label: s.name}
}

// changePhrase names an entry from what it touched (ADR-021): one option of
// one subject reads as the edit, the value it held and holds; anything wider
// reads as its first two subjects, each with what happened to it, and how
// many more there are. plain is the same line read as words.
func changePhrase(c history.Change, tr func(string) string) ([]widget.TablePart, string) {
	return phrase(c, tr, true)
}

// changeSummary is a stage's rows read as subjects, what each did, in words:
// the review drawer's offer for a note, before any value it replaced is read.
func changeSummary(items []history.ChangeItem, tr func(string) string) string {
	_, plain := phrase(history.Change{Items: items}, tr, false)
	return plain
}

// phrase is changePhrase, reading one option of one subject as the edit only
// where edit is set.
func phrase(c history.Change, tr func(string) string, edit bool) ([]widget.TablePart, string) {
	type touched struct {
		subject
		verbs []string
		ops   []history.ChangeOp
	}
	var all []*touched
	for _, item := range c.Items {
		s := itemSubject(item)
		i := slices.IndexFunc(all, func(t *touched) bool { return t.subject == s })
		if i < 0 {
			i = len(all)
			all = append(all, &touched{subject: s})
		}
		all[i].verbs = append(all[i].verbs, item.Verb)
		all[i].ops = append(all[i].ops, item.Ops...)
	}
	var parts []widget.TablePart
	if edit && len(all) == 1 && len(all[0].ops) == 1 {
		if edited, ok := editParts(all[0].ops[0], tr); ok {
			parts = append([]widget.TablePart{{Chip: all[0].chip()}}, edited...)
		}
	}
	if parts == nil {
		shown := all[:min(2, len(all))]
		for i, t := range shown {
			word := tr(verbOf(t.verbs))
			if i == 0 && len(shown) > 1 {
				word += ","
			}
			parts = append(parts, widget.TablePart{Chip: t.chip()}, widget.TablePart{Words: word})
		}
		if more := len(all) - len(shown); more > 0 {
			parts = append(parts, widget.TablePart{Words: fmt.Sprintf(tr("and %d more"), more)})
		}
	}
	words := make([]string, len(parts))
	for i, p := range parts {
		switch {
		case p.Chip != nil:
			words[i] = p.Chip.Label
		case p.Value != "":
			words[i] = p.Value
		default:
			words[i] = p.Words
		}
	}
	return parts, strings.Join(words, " ")
}

// verbOf is what happened to a subject over all its rows: a thing removed
// was removed, a thing made was added, whatever else was done to it.
func verbOf(verbs []string) string {
	switch {
	case slices.Contains(verbs, "removed"):
		return "removed"
	case slices.Contains(verbs, "added"):
		return "added"
	case len(verbs) > 0 && !slices.ContainsFunc(verbs, func(v string) bool { return v != "reordered" }):
		return "reordered"
	}
	return "changed"
}

// editParts reads one change of one option as its key's chip and the value
// it held and holds; a list's as the value it gained or lost; a secret's as
// changed. A change to a whole section is no edit.
func editParts(op history.ChangeOp, tr func(string) string) ([]widget.TablePart, bool) {
	key := widget.TablePart{Chip: &widget.TableChip{Label: op.Option}}
	switch op.Op {
	case "set", "remove-option", "list-add", "list-del":
	default:
		return nil, false
	}
	if op.Secret {
		return []widget.TablePart{key, {Words: tr("changed")}}, true
	}
	value := func(v *string, ok bool) widget.TablePart {
		switch {
		case !ok || v == nil:
			return widget.TablePart{Words: tr("not set")}
		case *v == "":
			return widget.TablePart{Value: "''"}
		}
		return widget.TablePart{Value: *v}
	}
	arrow := widget.TablePart{Words: "→"}
	switch op.Op {
	case "list-add":
		return []widget.TablePart{key, {Words: tr("added")}, value(op.Value, true)}, true
	case "list-del":
		return []widget.TablePart{key, {Words: tr("removed")}, value(op.Value, true)}, true
	case "remove-option":
		old, ok := oldValue(op.Old)
		return []widget.TablePart{key, value(&old, ok), arrow, {Words: tr("not set")}}, true
	}
	old, ok := oldValue(op.Old)
	return []widget.TablePart{key, value(&old, ok), arrow, value(op.Value, true)}, true
}

// oldValue is what an option held, as one line: a value, or a list's values
// apart. A section, or nothing, is no option's value.
func oldValue(old json.RawMessage) (string, bool) {
	var one string
	if json.Unmarshal(old, &one) == nil {
		return one, true
	}
	var list []string
	if json.Unmarshal(old, &list) == nil {
		return strings.Join(list, " "), true
	}
	return "", false
}

// changeDrawer is an entry's drawer, on one of its two tabs: what it changed,
// or what has changed since.
func changeDrawer(c history.Change, all []history.Change, since bool, now time.Time, tr func(string) string) *widget.RowDrawer {
	path := fmt.Sprintf("%s/%d", changesPath, c.ID)
	d := &widget.RowDrawer{
		Title: changeTitle(c.Time, now, tr), Verbatim: true,
		Tabs: []widget.DrawerTab{
			{Label: "What changed", Href: path, Active: !since},
			{Label: "Since then", Href: path + "?tab=since", Active: since},
		},
	}
	if since {
		d.Children = sinceThenTab(c, all, path, tr)
	} else {
		d.Children = whatChangedTab(c, all, path, now, tr)
	}
	return d
}

// whatChangedTab is what its row cannot hold, its note, then every uci
// change it made, and the act that takes it back or puts it back: Undo for
// one in force, Stage again for one rolled back or undone.
func whatChangedTab(c history.Change, all []history.Change, path string, now time.Time, tr func(string) string) widget.Widgets {
	var children widget.Widgets
	if c.Note != "" {
		children = append(children, &widget.Text{Markdown: markdownLiteral(c.Note), Translated: true})
	}
	var notes []string
	var act *widget.Form
	count := len(c.Items)
	if c.Status == history.ChangeApplied {
		plan := reverseOf([]history.Change{c})
		switch {
		case !c.OldRead || plan.Unknown:
			notes = append(notes, tr("This change was recorded before what it replaced was kept, so it cannot be undone."))
		case len(plan.Steps) > 0:
			label := tr("Undo this change")
			if count > 1 {
				label = fmt.Sprintf(tr("Undo these %d changes"), count)
			}
			act = actForm(path, "undo", label)
		}
		if later := touchedAgain(all, c); len(later) > 0 {
			times := make([]string, len(later))
			for i, l := range later {
				times[i] = eventTime(l.Time, now)
			}
			notes = append(notes, fmt.Sprintf(tr("Changed again at %s. Undoing writes over that."), strings.Join(times, ", ")))
		}
		notes = append(notes, planNotes(plan, tr)...)
	} else {
		plan := forwardOf(c)
		if plan.Unknown {
			notes = append(notes, tr("A list it changed was recorded without what it held before, so it stays as it is now."))
		}
		notes = append(notes, planNotes(plan, tr)...)
		if len(plan.Steps) > 0 {
			label := tr("Stage this change again")
			if count > 1 {
				label = fmt.Sprintf(tr("Stage these %d changes again"), count)
			}
			act = actForm(path, "again", label)
		}
	}
	for _, note := range notes {
		children = append(children, &widget.Callout{Variant: "warning", Compact: true, Body: note})
	}
	children = append(children, changeOpsTable(c, tr))
	if act != nil {
		children = append(children, act)
	}
	return children
}

// sinceThenTab is what is set differently now than just after this entry,
// net of anything since undone or rolled back, and Return to this point,
// which stages the opposite of all of it.
func sinceThenTab(c history.Change, all []history.Change, path string, tr func(string) string) widget.Widgets {
	return returnBody(reverseOf(sinceThen(all, c)), path, "Return to this point", "At this point", tr)
}

// returnBody is what a return would stage, an option a row as it is now and
// as it was then, and the act that stages it, labelled. Nothing to return
// says the router is as it was.
func returnBody(plan actPlan, path, label, then string, tr func(string) string) widget.Widgets {
	if len(plan.Rows) == 0 && !plan.Unknown && len(plan.Secrets) == 0 && len(plan.Files) == 0 {
		return widget.Widgets{&widget.Text{Markdown: "Nothing has changed since. This is how the router is now."}}
	}
	var children widget.Widgets
	notes := planNotes(plan, tr)
	if plan.Unknown {
		notes = append([]string{tr("A change since then was recorded before what it replaced was kept, so the router cannot be returned to this point.")}, notes...)
	}
	for _, note := range notes {
		children = append(children, &widget.Callout{Variant: "warning", Compact: true, Body: note})
	}
	table := &widget.Table{Columns: []widget.TableColumn{
		{Label: "Option", Kind: "mono"},
		{Label: "Now", Kind: "mono", Width: widget.MeasureName},
		{Label: then, Kind: "mono", Width: widget.MeasureName},
		{Label: "Change", Kind: "status", Width: widget.MeasureWord},
	}}
	for _, row := range plan.Rows {
		verb, variant := "Changed", "warning"
		switch {
		case row.Then == none:
			verb, variant = "Removed", "danger"
		case row.Now == none:
			verb, variant = "Added", "success"
		}
		table.Rows = append(table.Rows, widget.TableRow{Cells: []widget.TableCell{{Text: row.Option}, {Text: row.Now}, {Text: row.Then}, {Text: verb, Variant: variant}}})
	}
	children = append(children, table)
	if !plan.Unknown && len(plan.Steps) > 0 {
		children = append(children, actForm(path, "return", label))
	}
	return children
}

// planNotes is what an act leaves as it is, in words.
func planNotes(plan actPlan, tr func(string) string) []string {
	var notes []string
	if len(plan.Secrets) > 0 {
		notes = append(notes, tr("A password or key it changed stays as it is now: its values are never kept."))
	}
	if len(plan.Files) > 0 {
		notes = append(notes, tr("Option files it changed stay as they are now."))
	}
	return notes
}

// actForm is one act on an entry, posted back to its drawer.
func actForm(path, act, label string) *widget.Form {
	return &widget.Form{Action: path, Frame: widget.FramePanel, Panel: path, Submit: label, Fields: widget.Widgets{
		&widget.Field{Kind: "hidden", Name: "act", Value: act},
	}}
}

// changeTitle is when an entry was made, as its drawer is headed.
func changeTitle(at, now time.Time, tr func(string) string) string {
	return changeDay(at, now, tr) + ", " + clock.Local(at).Format("15:04")
}

// changeOpsTable is every uci change an entry made, a row each: the option,
// what it held and what it holds. Each run of the review drawer's rows is a
// lane, named by its page and, where its plugin described it, its words.
func changeOpsTable(c history.Change, tr func(string) string) *widget.Table {
	table := &widget.Table{Columns: []widget.TableColumn{
		{Label: "Option", Kind: "mono"},
		{Label: "Before", Kind: "mono", Width: widget.MeasureName},
		{Label: "After", Kind: "mono", Width: widget.MeasureName},
		{Label: "Change", Kind: "status", Width: widget.MeasureWord},
	}}
	for _, item := range c.Items {
		var rows []widget.TableRow
		for _, op := range item.Ops {
			rows = append(rows, opRows(op, tr)...)
		}
		if len(rows) == 0 {
			continue
		}
		group := &widget.TableGroup{Label: tr(item.Page), Verbatim: true}
		if !item.Verbatim {
			group.Tally = item.Words
		}
		rows[0].Group = group
		table.Rows = append(table.Rows, rows...)
	}
	return table
}

// opRows is one uci change as the drawer's rows: the option, what it held
// and holds, and what happened to it. A section removed is listed with each
// option it held; a secret's values are not kept, in a change to it or in a
// section removed.
func opRows(op history.ChangeOp, tr func(string) string) []widget.TableRow {
	const none = "—"
	path := op.Config + "." + op.Section
	option := path + "." + op.Option
	row := func(name, before, after, verb, variant string) widget.TableRow {
		return widget.TableRow{Cells: []widget.TableCell{{Text: name}, {Text: before}, {Text: after}, {Text: verb, Variant: variant}}}
	}
	shown := func(v string) string {
		if v == "" {
			return "''"
		}
		return v
	}
	held := func() string {
		if old, ok := oldValue(op.Old); ok {
			return shown(old)
		}
		return none
	}
	value := none
	if op.Value != nil {
		value = shown(*op.Value)
	}
	if op.Secret {
		notKept := tr("not kept")
		return []widget.TableRow{row(option, notKept, notKept, "Changed", "warning")}
	}
	switch op.Op {
	case "add-section":
		return []widget.TableRow{row(path, none, op.Option, "Added", "success")}
	case "set":
		if before := held(); before != none {
			return []widget.TableRow{row(option, before, value, "Changed", "warning")}
		}
		return []widget.TableRow{row(option, none, value, "Added", "success")}
	case "list-add":
		return []widget.TableRow{row(option, none, value, "Added", "success")}
	case "list-del":
		return []widget.TableRow{row(option, value, none, "Removed", "danger")}
	case "remove-option":
		return []widget.TableRow{row(option, held(), none, "Removed", "danger")}
	case "remove-section":
		var section map[string]any
		_ = json.Unmarshal(op.Old, &section)
		typ, _ := section[".type"].(string)
		if typ == "" {
			typ = none
		}
		rows := []widget.TableRow{row(path, typ, none, "Removed", "danger")}
		keys := make([]string, 0, len(section))
		for k := range section {
			if !strings.HasPrefix(k, ".") {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		for _, k := range keys {
			if isFingerprint(section[k]) {
				rows = append(rows, row(path+"."+k, tr("not kept"), none, "Removed", "danger"))
				continue
			}
			v := fmt.Sprint(section[k])
			if list, ok := section[k].([]any); ok {
				words := make([]string, len(list))
				for i, item := range list {
					words[i] = fmt.Sprint(item)
				}
				v = strings.Join(words, " ")
			}
			rows = append(rows, row(path+"."+k, shown(v), none, "Removed", "danger"))
		}
		return rows
	}
	return []widget.TableRow{row(path, none, none, "Changed", "warning")}
}

// markdownLiteral is words a person typed, to stand in Markdown as they
// were typed: every ASCII punctuation mark escaped, so none of it is read as
// formatting.
func markdownLiteral(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r < 128 && (unicode.IsPunct(r) || unicode.IsSymbol(r)) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}
