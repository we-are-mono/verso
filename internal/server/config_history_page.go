// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

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
	table := changesTable(changes, time.Now(), clientIP(r), tr)
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
	if !panelRequest(r) {
		http.Redirect(w, r, changesPath, http.StatusSeeOther)
		return
	}
	c, ok, err := s.historyStore.Change(r.Context(), id)
	switch {
	case err != nil:
		s.entityNotice(w, http.StatusInternalServerError, fmt.Sprintf(tr("The history could not be read (%v)."), err))
		return
	case !ok:
		s.entityNotice(w, http.StatusNotFound, tr("This change is no longer kept."))
		return
	}
	row := widget.TableRow{ID: strconv.FormatInt(id, 10), Drawer: changeDrawer(c, time.Now(), tr)}
	row.Drawer.Open = true
	var body bytes.Buffer
	if _, err := s.widgets.RenderOpenPanelWithToken(&body, &widget.Table{Columns: []widget.TableColumn{{}}, Rows: []widget.TableRow{row}},
		s.sessionCSRF(r), lang, t, widget.Flash{}); err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(body.Bytes())
}

// changesTable is the listing: lanes by day, a row per entry, named from
// what it touched. An entry no longer in force (rolled back, undone) is
// marked over, as a switched-off rule is. viewer is the reader's address, so
// their own applies say so.
func changesTable(changes []history.Change, now time.Time, viewer string, tr func(string) string) *widget.Table {
	table := &widget.Table{
		Columns: []widget.TableColumn{
			{Label: "When", Width: widget.MeasureShort},
			{Label: "Changed", Kind: "phrase"},
			{Label: "By", Kind: "phrase", Width: widget.MeasureName},
			{Label: "Status", Kind: "status", Width: widget.MeasureWord},
			{Kind: "actions", Width: widget.MeasureShort},
		},
		EmptyText: "Nothing has been applied yet. Every apply is kept here, with what it replaced.",
	}
	when := make(map[int64]time.Time, len(changes))
	perDay := map[string]int{}
	for _, c := range changes {
		when[c.ID] = c.Time
		perDay[changeDay(c.Time, now, tr)]++
	}
	day := ""
	for _, c := range changes {
		row := changeRow(c, when, now, viewer, tr)
		if d := changeDay(c.Time, now, tr); d != day {
			day = d
			tally := tr("1 change")
			if n := perDay[d]; n != 1 {
				tally = fmt.Sprintf(tr("%d changes"), n)
			}
			row.Group = &widget.TableGroup{Label: d, Tally: tally, Verbatim: true}
		}
		table.Rows = append(table.Rows, row)
	}
	return table
}

// changeRow is one entry in the listing.
func changeRow(c history.Change, when map[int64]time.Time, now time.Time, viewer string, tr func(string) string) widget.TableRow {
	parts, plain := changePhrase(c, tr)
	changed := widget.TableCell{Text: plain, Parts: parts, Verbatim: true}
	if related, ok := when[c.Related]; ok {
		switch c.Relation {
		case "undo":
			changed.Detail = fmt.Sprintf(tr("undo of %s"), eventTime(related, now))
		case "return":
			changed.Detail = fmt.Sprintf(tr("back to %s"), eventTime(related, now))
		case "again":
			changed.Detail = fmt.Sprintf(tr("again, from %s"), eventTime(related, now))
		}
	}
	word, variant := changeStatus(c.Status)
	return widget.TableRow{
		ID:    strconv.FormatInt(c.ID, 10),
		Muted: c.Status != history.ChangeApplied,
		Panel: fmt.Sprintf("%s/%d", changesPath, c.ID),
		Cells: []widget.TableCell{
			{Text: clock.Local(c.Time).Format("15:04"), Verbatim: true},
			changed,
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
	if len(all) == 1 && len(all[0].ops) == 1 {
		if edit, ok := editParts(all[0].ops[0], tr); ok {
			parts = append([]widget.TablePart{{Chip: all[0].chip()}}, edit...)
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

// changeDrawer is an entry's drawer: its facts, then its rows by page, each
// opening on the lines it wrote.
func changeDrawer(c history.Change, now time.Time, tr func(string) string) *widget.RowDrawer {
	var facts []widget.Property
	if c.Note != "" {
		facts = append(facts, widget.Property{Label: "Note", Value: c.Note, Verbatim: true})
	}
	switch c.Status {
	case history.ChangeApplied:
		facts = append(facts, widget.Property{Label: "Status", Value: "Applied"})
	case history.ChangeRolledBack:
		facts = append(facts, widget.Property{Label: "Status", Value: fmt.Sprintf(tr("Rolled back. Nobody confirmed it within %d seconds, so the router put it back."), uciRollbackTimeout), Verbatim: true})
	default:
		facts = append(facts, widget.Property{Label: "Status", Value: "Undone"})
	}
	if c.User == "" {
		facts = append(facts, widget.Property{Label: "By", Value: "Not known. Written straight to the config: over SSH, by a script or by another tool."})
	} else {
		facts = append(facts, widget.Property{Label: "By", Value: c.User, Mono: true})
		if c.Address != "" {
			facts = append(facts, widget.Property{Label: "From", Value: c.Address, Mono: true})
		}
	}
	return &widget.RowDrawer{
		Title: changeTitle(c.Time, now, tr), Verbatim: true,
		Children: widget.Widgets{&widget.Properties{Style: "system", Items: facts}, changeItemsTable(c, tr)},
	}
}

// changeTitle is when an entry was made, as its drawer is headed.
func changeTitle(at, now time.Time, tr func(string) string) string {
	return changeDay(at, now, tr) + ", " + clock.Local(at).Format("15:04")
}

// changeItemsTable is an entry's rows as the review drawer read them, in
// lanes by page; each opens on the lines it wrote.
func changeItemsTable(c history.Change, tr func(string) string) *widget.Table {
	table := &widget.Table{Columns: []widget.TableColumn{
		{Label: "Change", Kind: "phrase"},
		{Label: "What happened", Kind: "status", Width: widget.MeasureWord},
	}}
	perPage := map[string]int{}
	for _, item := range c.Items {
		perPage[item.Page]++
	}
	page := ""
	for i, item := range c.Items {
		words := widget.TablePart{Words: item.Words}
		if item.Verbatim {
			words = widget.TablePart{Value: item.Words}
		}
		verb, variant := verbStatus(item.Verb)
		row := widget.TableRow{
			ID:       strconv.Itoa(i),
			Cells:    []widget.TableCell{{Text: item.Words, Parts: []widget.TablePart{words}, Verbatim: true}, {Text: verb, Variant: variant}},
			Expanded: widget.Widgets{&widget.Code{Value: changeLines(item.Ops, tr)}},
		}
		if item.Page != page {
			page = item.Page
			tally := tr("1 change")
			if n := perPage[page]; n != 1 {
				tally = fmt.Sprintf(tr("%d changes"), n)
			}
			row.Group = &widget.TableGroup{Label: tr(page), Tally: tally, Verbatim: true}
		}
		table.Rows = append(table.Rows, row)
	}
	return table
}

// verbStatus is a row's verb in a word and the mark's colour the review
// drawer gives it.
func verbStatus(verb string) (string, string) {
	switch verb {
	case "added":
		return "Added", "success"
	case "removed":
		return "Removed", "danger"
	case "reordered":
		return "Reordered", "info"
	}
	return "Changed", "warning"
}

// changeLines is what a row's uci changes wrote, a line each as `uci show`
// spells it, led by + for what is new, - for what went and ~ for a secret
// whose values are not kept. A value replaced stands above its successor; a
// section removed is listed with what it held.
func changeLines(ops []history.ChangeOp, tr func(string) string) string {
	var lines []string
	add := func(glyph, text string) { lines = append(lines, glyph+" "+text) }
	for _, op := range ops {
		path := op.Config + "." + op.Section
		option := path + "." + op.Option
		if op.Secret {
			add("~", option+" ("+tr("changed, not kept")+")")
			continue
		}
		value := ""
		if op.Value != nil {
			value = *op.Value
		}
		switch op.Op {
		case "add-section":
			add("+", path+"="+op.Option)
		case "set":
			if old, ok := oldValue(op.Old); ok {
				add("-", option+"="+uciQuote(old))
			}
			add("+", option+"="+uciQuote(value))
		case "list-add":
			add("+", option+"+="+uciQuote(value))
		case "list-del":
			add("-", option+"-="+uciQuote(value))
		case "remove-option":
			if old, ok := oldValue(op.Old); ok {
				add("-", option+"="+uciQuote(old))
			} else {
				add("-", option)
			}
		case "remove-section":
			lines = append(lines, removedSection(path, op.Old)...)
		default:
			add("~", path)
		}
	}
	return strings.Join(lines, "\n")
}

// removedSection is a removed section's lines: its type, then each option it
// held, in order of name.
func removedSection(path string, old json.RawMessage) []string {
	var held map[string]any
	if json.Unmarshal(old, &held) != nil || held == nil {
		return []string{"- " + path}
	}
	typ, _ := held[".type"].(string)
	lines := []string{"- " + path + "=" + typ}
	keys := make([]string, 0, len(held))
	for k := range held {
		if !strings.HasPrefix(k, ".") {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		switch v := held[k].(type) {
		case string:
			lines = append(lines, "- "+path+"."+k+"="+uciQuote(v))
		case []any:
			quoted := make([]string, len(v))
			for i, item := range v {
				quoted[i] = uciQuote(fmt.Sprint(item))
			}
			lines = append(lines, "- "+path+"."+k+"="+strings.Join(quoted, " "))
		}
	}
	return lines
}

// uciQuote is a value as `uci show` quotes it.
func uciQuote(v string) string {
	return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'"
}
