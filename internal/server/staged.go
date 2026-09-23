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
	"sort"
	"strings"
	"sync"

	"github.com/we-are-mono/verso/internal/plugin"
)

// stageMemo holds one request's read of the stage. A page asks the stage twice
// — for the marks on the controls whose options wait, and for the chip — and
// both must read the same stage, once: a ubus round trip is the router's CPU,
// which is the budget a LAN page spends. The memo is installed where a page is
// answered, after anything that request stages has been written.
type stageMemo struct {
	once    sync.Once
	changes map[string][][]string
	err     error
}

type stageMemoKey struct{}

// withStageMemo gives a request's context one shared read of the stage.
func withStageMemo(r *http.Request) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), stageMemoKey{}, &stageMemo{}))
}

// stageChanges reads the pending changes, once per request where a memo is
// installed.
func (s *Server) stageChanges(ctx context.Context, sid string) (map[string][][]string, error) {
	memo, ok := ctx.Value(stageMemoKey{}).(*stageMemo)
	if !ok {
		return s.backend.UCIChanges(ctx, sid)
	}
	memo.once.Do(func() { memo.changes, memo.err = s.backend.UCIChanges(ctx, sid) })
	return memo.changes, memo.err
}

// waitingOptions is the set of options that wait on the stage, by their full
// address "config.section.option", for the configs the shell manages — what
// MarkStaged asks each control about. A change to a whole section (made or
// removed) names no option and marks no control; its row says so instead.
func (s *Server) waitingOptions(ctx context.Context, sid string) func(string) bool {
	waiting := map[string]bool{}
	if sid != "" {
		changes, err := s.stageChanges(ctx, sid)
		if err != nil {
			log.Printf("verso: uci changes unavailable: %v", err)
		}
		declared := s.declaredConfigsUnion()
		for config, list := range changes {
			if !declared[config] {
				continue
			}
			for _, ch := range coalesceChanges(list) {
				if section, option, ok := changedOption(ch); ok {
					waiting[config+"."+section+"."+option] = true
				}
			}
		}
	}
	return func(address string) bool { return waiting[address] }
}

// changedOption names the option a change tuple touches: a value set or
// removed, or one value added to or taken from a list. A change to a whole
// section names none.
func changedOption(ch []string) (section, option string, ok bool) {
	switch {
	case len(ch) == 4 && (ch[0] == "set" || ch[0] == "list-add" || ch[0] == "list-del"),
		len(ch) == 3 && ch[0] == "remove":
		return ch[1], ch[2], true
	}
	return "", "", false
}

// uciRollbackTimeout is the window rpcd holds an applied configuration before
// reverting it on the device, unless the browser confirms (ADR-010).
const uciRollbackTimeout = 30

// stagedView is what the staged-changes chip and its review drawer render
// (ADR-010): the pending uci changes across the configs plugins declare, read
// from UCI's own stage — the browser holds nothing.
type stagedView struct {
	Count  int
	Label  string        // "3 staged changes", worded here so the chip prints it raw
	Groups []stagedGroup // the drawer's rows, under the page each change belongs to
	// Rollback is the window the drawer promises, in seconds. It is the same
	// constant the apply actually arms rpcd with, carried into the sentence so
	// the promise on screen cannot drift from the one the device keeps.
	Rollback int
}

// stagedGroup is one page's run of changes in the drawer: a band naming the
// page and what its changes amount to, then the rows.
type stagedGroup struct {
	Page  string
	Tally string // "3 changes"
	Items []stagedItem
}

// stagedItem is one row of the drawer: what happened, as the closed word the
// row's mark is coloured by (Verb) and in the reader's language (Word); the
// owning plugin's sentence for it; and, opened, the uci lines it stands for.
// Mono marks a row that is a raw uci line — a change no plugin described.
type stagedItem struct {
	Verb  string // "added" | "changed" | "removed" | "reordered"
	Word  string
	Label string
	Mono  bool
	Lines []stagedLine
}

// stagedLine is one uci line under an opened row, led by the glyph of what
// happens to it: + for what is new, - for what goes, ~ for what changes.
type stagedLine struct {
	Op   string
	Text string
}

// stagedChange is one coalesced net change, flattened once so the count, the
// describe round-trip, and the drawer all speak of the same ordered set.
type stagedChange struct {
	config string
	tuple  []string
	raw    string
}

// stagedRow is a drawer row with the config it came from, which is what files
// it under a page.
type stagedRow struct {
	config string
	item   stagedItem
}

// declaredConfigsUnion is the set of uci configs any installed plugin declares,
// plus the shell's own (ADR-013) — the surface the stage reports and discards.
// Changes staged outside it (a concurrent uci shell on an undeclared config) are
// not the shell's to manage.
func (s *Server) declaredConfigsUnion() map[string]bool {
	union := make(map[string]bool)
	for _, cfg := range shellConfigs {
		union[cfg] = true
	}
	for _, m := range s.manifestList() {
		for cfg := range declaredUCIConfigs(m) {
			union[cfg] = true
		}
	}
	return union
}

// staged reads the pending changes for the session. A read failure logs and
// yields an empty stage — the page renders; the chip simply stays hidden.
// pluginTr names each page in the reader's language from its plugin's own
// catalog (ADR-012).
func (s *Server) staged(ctx context.Context, sid string, tr func(string) string, pluginTr func(string) func(string) string) stagedView {
	v := stagedView{Rollback: uciRollbackTimeout, Label: stagedLabel(0, tr)}
	if sid == "" {
		return v
	}
	changes, err := s.stageChanges(ctx, sid)
	if err != nil {
		log.Printf("verso: uci changes unavailable: %v", err)
		return v
	}
	declared := s.declaredConfigsUnion()

	configs := make([]string, 0, len(changes))
	for cfg := range changes {
		if declared[cfg] {
			configs = append(configs, cfg)
		}
	}
	sort.Strings(configs)
	var flat []stagedChange
	for _, cfg := range configs {
		for _, ch := range coalesceChanges(changes[cfg]) {
			flat = append(flat, stagedChange{config: cfg, tuple: ch, raw: humanizeChange(cfg, ch)})
		}
	}
	if len(flat) == 0 {
		return v
	}

	// The count is the number of things a person did, not the number of uci
	// writes: each row is one change. A plugin folds all of one item's writes —
	// a rule's six fields — into a single described row, so the item counts
	// once; a settings page leaves each option on its own row, so each counts.
	// The rows gather under the page they belong to, in the order the pages
	// first appear.
	index := map[string]int{}
	for _, row := range s.describeStaged(ctx, sid, flat, tr) {
		page := s.pageOf(row.config, tr, pluginTr)
		i, ok := index[page]
		if !ok {
			i = len(v.Groups)
			index[page] = i
			v.Groups = append(v.Groups, stagedGroup{Page: page})
		}
		v.Groups[i].Items = append(v.Groups[i].Items, row.item)
	}
	for i := range v.Groups {
		n := len(v.Groups[i].Items)
		v.Groups[i].Tally = stagedTally(n, tr)
		v.Count += n
	}
	v.Label = stagedLabel(v.Count, tr)
	return v
}

// stagedLabel is what the chip and the drawer's title say the stage holds,
// localized at composition — the template prints it raw, and the client
// rewrites it from the same catalog (verso.js T), so the two speak one
// vocabulary.
func stagedLabel(n int, tr func(string) string) string {
	switch n {
	case 0:
		return tr("No staged changes")
	case 1:
		return tr("1 staged change")
	default:
		return fmt.Sprintf(tr("%d staged changes"), n)
	}
}

// stagedTally is what a page's band says its run amounts to.
func stagedTally(n int, tr func(string) string) string {
	if n == 1 {
		return tr("1 change")
	}
	return fmt.Sprintf(tr("%d changes"), n)
}

// pageOf names the page a change belongs to: the plugin that owns its config,
// by the name that plugin's catalog gives it, or System for the shell's own.
func (s *Server) pageOf(config string, tr func(string) string, pluginTr func(string) func(string) string) string {
	if m, ok := s.configOwner(config); ok {
		return pluginTr(m.ID)(m.Name)
	}
	return tr("System")
}

// describeStaged turns the flattened net changes into the drawer's rows: each
// owning plugin describes its own changes in plain words, and anything left
// undescribed — a shell-owned config, a plugin with no describe hook, a describe
// call that failed — keeps its raw uci line. Order follows the flattened set, and
// a plain sentence lands where the first change it covers would have.
func (s *Server) describeStaged(ctx context.Context, sid string, flat []stagedChange, tr func(string) string) []stagedRow {
	// Group each change's position by the plugin that owns its config. The shell's
	// own configs, and any config no installed plugin writes, own no describer.
	byPlugin := map[string][]int{}
	sockets := map[string]string{}
	manifests := map[string]plugin.Manifest{}
	for i, c := range flat {
		m, ok := s.configOwner(c.config)
		if !ok {
			continue
		}
		byPlugin[m.ID] = append(byPlugin[m.ID], i)
		sockets[m.ID] = m.Socket
		manifests[m.ID] = m
	}

	// covered maps a flattened index to the sentence that accounts for it, and the
	// full ordered run of indices that sentence folds together.
	type run struct {
		plain   string
		indices []int
	}
	covered := map[int]*run{}
	for id, positions := range byPlugin {
		m := manifests[id]
		reqChanges := make([]plugin.DescribeChange, len(positions))
		for j, pos := range positions {
			reqChanges[j] = describeChange(flat[pos])
		}
		snapshot := s.readSnapshot(ctx, m, sid)
		descs, err := s.transport.Describe(ctx, sockets[id], reqChanges, snapshot)
		if err != nil {
			log.Printf("verso: plugin %q describe failed: %v", id, err)
			continue // leave this plugin's changes as raw lines
		}
		for _, d := range descs {
			if d.Plain == "" || len(d.Covers) == 0 {
				continue
			}
			r := &run{plain: d.Plain}
			for _, local := range d.Covers {
				if local < 0 || local >= len(positions) {
					continue // a plugin cannot cover a change the shell never sent
				}
				global := positions[local]
				if covered[global] != nil {
					continue // a change belongs to the first sentence that claimed it
				}
				covered[global] = r
				r.indices = append(r.indices, global)
			}
			if len(r.indices) == 0 {
				continue
			}
			sort.Ints(r.indices)
		}
	}

	var rows []stagedRow
	emitted := map[*run]bool{}
	for i, c := range flat {
		if r := covered[i]; r != nil {
			if emitted[r] {
				continue // its sentence already rendered, at its first covered change
			}
			emitted[r] = true
			changes := make([]stagedChange, 0, len(r.indices))
			for _, g := range r.indices {
				changes = append(changes, flat[g])
			}
			rows = append(rows, stagedRow{config: c.config, item: stagedItemOf(r.plain, false, changes, tr)})
			continue
		}
		rows = append(rows, stagedRow{config: c.config, item: stagedItemOf(c.raw, true, []stagedChange{c}, tr)})
	}
	return rows
}

// stagedItemOf builds a row from the changes it stands for. The verb is what
// the run amounts to: a section going is "removed", a section arriving is
// "added", an order rewritten is "reordered", anything else is "changed". Each
// line then wears the glyph of its own op — except that every line of a thing
// added or removed is new or going along with it.
func stagedItemOf(label string, mono bool, changes []stagedChange, tr func(string) string) stagedItem {
	verb := "changed"
	ops := make([]string, len(changes))
	for i, c := range changes {
		ops[i] = describeChange(c).Op
	}
	switch {
	case hasOp(ops, "remove-section"):
		verb = "removed"
	case hasOp(ops, "add-section"):
		verb = "added"
	case hasOp(ops, "reorder"):
		verb = "reordered"
	}
	item := stagedItem{Verb: verb, Word: tr(verb), Label: label, Mono: mono}
	for i, c := range changes {
		op := "~"
		switch {
		case verb == "added", ops[i] == "add-section", ops[i] == "list-add":
			op = "+"
		case verb == "removed", ops[i] == "remove-section", ops[i] == "remove-option", ops[i] == "list-del":
			op = "-"
		}
		if ops[i] == "file" && len(c.tuple) == 3 {
			item.Lines = append(item.Lines, stagedLine{Op: op, Text: c.tuple[2]})
			continue
		}
		item.Lines = append(item.Lines, stagedLine{Op: op, Text: c.raw})
	}
	return item
}

func hasOp(ops []string, op string) bool {
	for _, o := range ops {
		if o == op {
			return true
		}
	}
	return false
}

// describeChange flattens a coalesced uci change tuple into the named-field shape
// a plugin's describe hook reads, normalizing the op so the plugin never has to
// reason about a tuple's length (which alone tells "set option to empty" from
// "new section of a type"). The normalized ops are a small closed vocabulary:
//
//	set            Option set to Value on Section
//	add-section    new Section, Option is its uci type
//	remove-option  Option cleared from Section
//	remove-section Section removed whole
//	list-add       Value added to the list Option on Section
//	list-del       Value removed from the list Option on Section
func describeChange(c stagedChange) plugin.DescribeChange {
	dc := plugin.DescribeChange{Config: c.config}
	ch := c.tuple
	if len(ch) == 0 {
		return dc
	}
	op := ch[0]
	if len(ch) > 1 {
		dc.Section = ch[1]
	}
	switch {
	case op == "file" && len(ch) == 3:
		dc.Op, dc.Value = "file", ch[2]
	case op == "set" && len(ch) == 4:
		dc.Op, dc.Option, dc.Value = "set", ch[2], ch[3]
	case (op == "set" || op == "add") && len(ch) == 3:
		dc.Op, dc.Option = "add-section", ch[2] // Option carries the new section's type
	case op == "remove" && len(ch) == 3:
		dc.Op, dc.Option = "remove-option", ch[2]
	case op == "remove":
		dc.Op = "remove-section"
	case op == "list-add" && len(ch) == 4:
		dc.Op, dc.Option, dc.Value = "list-add", ch[2], ch[3]
	case op == "list-del" && len(ch) == 4:
		dc.Op, dc.Option, dc.Value = "list-del", ch[2], ch[3]
	default:
		dc.Op = op
		if len(ch) > 2 {
			dc.Option = ch[2]
		}
		if len(ch) > 3 {
			dc.Value = ch[3]
		}
	}
	return dc
}

// configOwner returns the installed plugin that declares write access to config,
// the one asked to describe that config's changes. Ownership is a write grant; a
// config no plugin writes (or the shell's own) has no describer.
func (s *Server) configOwner(config string) (plugin.Manifest, bool) {
	for _, m := range s.manifestList() {
		if declaredUCIConfigs(m)[config] {
			return m, true
		}
	}
	return plugin.Manifest{}, false
}

// humanizeChange renders one uci change tuple ([op, section, option?, value?])
// as a plain line. Mechanical on purpose (ADR-010): config, section, option,
// value — no per-plugin interpretation.
func humanizeChange(config string, ch []string) string {
	if len(ch) < 2 {
		return config + ": " + strings.Join(ch, " ")
	}
	op, section := ch[0], ch[1]
	switch {
	case op == "file" && len(ch) == 3:
		return section
	case op == "set" && len(ch) == 4:
		return fmt.Sprintf("%s: %s.%s = %s", config, section, ch[2], ch[3])
	case op == "set" && len(ch) == 3:
		return fmt.Sprintf("%s: new %s section %s", config, ch[2], section)
	case op == "add" && len(ch) == 3:
		return fmt.Sprintf("%s: new %s section %s", config, ch[2], section)
	case op == "remove" && len(ch) == 3:
		return fmt.Sprintf("%s: remove %s.%s", config, section, ch[2])
	case op == "remove":
		return fmt.Sprintf("%s: remove %s", config, section)
	case op == "list-add" && len(ch) == 4:
		return fmt.Sprintf("%s: %s.%s += %s", config, section, ch[2], ch[3])
	case op == "list-del" && len(ch) == 4:
		return fmt.Sprintf("%s: %s.%s -= %s", config, section, ch[2], ch[3])
	default:
		return fmt.Sprintf("%s: %s %s", config, op, strings.Join(ch[1:], " "))
	}
}

// coalesceChanges reduces one config's raw change journal to its net effect per
// target. rpcd records every write as its own entry — not the net delta — so a
// value written N times (an inline toggle flipped N times) piles up as N entries.
// Replaying keeps only what actually differs from the committed config: the last
// write to a target wins, and a target this session first added and then removed
// cancels out — a toggle flipped back to its committed state coalesces to nothing
// instead of accumulating. A lone removal (deleting an option or section that was
// committed) stands, since that is a real change; a shape we do not model is left
// as its own uncoalesced entry so nothing is silently dropped.
func coalesceChanges(list [][]string) [][]string {
	type state struct {
		order        int
		firstPresent bool // the first op left the target present (a set) rather than absent (a removal)
		present      bool // the final op's effect
		last         []string
	}
	seen := map[string]*state{}
	order := 0
	for _, ch := range list {
		key, present, ok := changeTarget(ch)
		if !ok {
			key = fmt.Sprintf("\x00unmodeled\x00%d", order)
			present = true
		}
		st := seen[key]
		if st == nil {
			st = &state{order: order, firstPresent: present}
			seen[key] = st
			order++
		}
		st.present = present
		st.last = ch
	}
	ordered := make([]*state, 0, len(seen))
	for _, st := range seen {
		ordered = append(ordered, st)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].order < ordered[j].order })
	out := make([][]string, 0, len(ordered))
	for _, st := range ordered {
		if !st.present && st.firstPresent {
			continue // added then removed within this session — net nothing
		}
		out = append(out, st.last)
	}
	return out
}

// changeTarget returns a coalescing key for a change tuple and whether the op
// leaves that target present (a set) or absent (a removal). ok is false for a
// shape we do not model, which is then never coalesced with anything.
func changeTarget(ch []string) (key string, present, ok bool) {
	if len(ch) < 2 {
		return "", false, false
	}
	op, section := ch[0], ch[1]
	const sep = "\x00"
	switch {
	case op == "set" && len(ch) == 4: // section.option = value
		return section + sep + ch[2], true, true
	case (op == "set" || op == "add") && len(ch) == 3: // a new section of a type
		return section, true, true
	case op == "remove" && len(ch) == 3: // delete section.option
		return section + sep + ch[2], false, true
	case op == "remove" && len(ch) == 2: // delete the whole section
		return section, false, true
	case op == "list-add" && len(ch) == 4: // one value added to a list option
		return section + sep + ch[2] + sep + ch[3], true, true
	case op == "list-del" && len(ch) == 4: // one value removed from a list option
		return section + sep + ch[2] + sep + ch[3], false, true
	default:
		return "", false, false
	}
}

// handleUCIReview renders the review drawer's contents from the stage: the
// panel alone for the frame that asked for it, and the same contents on a page
// of their own for a browser with no script, which follows the chip's link.
func (s *Server) handleUCIReview(w http.ResponseWriter, r *http.Request) {
	lang, t := s.localize(r)
	tr := translatorOrIdentity(t)
	view := s.staged(r.Context(), s.sessionSID(r), tr, s.pluginTranslators(r))
	var buf bytes.Buffer
	if err := s.pageSet(lang).ExecuteTemplate(&buf, "verso-staged-review", view); err != nil {
		log.Printf("verso: staged review render failed: %v", err)
		http.Error(w, "review unavailable", http.StatusInternalServerError)
		return
	}
	if panelRequest(r) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(buf.Bytes())
		return
	}
	// On a page of its own the contents keep the scope their close controls
	// speak to, so a browser with script still finds nothing to trip over.
	body := `<span x-data="modal" class="contents">` + buf.String() + `</span>`
	s.renderPage(w, r, http.StatusOK, pageHeader{Heading: "Staged changes"}, "narrow", nil, template.HTML(body)) //nolint:gosec // the shell's own rendered template
}

// handleUCIApply commits every dirty config through rpcd's `uci apply` with the
// device-side rollback armed (ADR-010). The drawer then confirms from the
// browser; no confirm within the window and the router reverts itself.
func (s *Server) handleUCIApply(w http.ResponseWriter, r *http.Request) {
	sid := s.sessionSID(r)
	if err := s.backend.UCIApply(r.Context(), sid, uciRollbackTimeout); err != nil {
		log.Printf("verso: uci apply failed: %v", err)
		http.Error(w, "apply failed", http.StatusBadGateway)
		return
	}
	// The staged UCI is applied; drain and clear this session's paired non-UCI
	// tail atomically, so a failed action cannot linger to fire on an unrelated
	// later apply — this session's or any other's.
	for _, action := range s.takePendingApply(sid) {
		if err := s.executeApplyAction(r.Context(), sid, action); err != nil {
			log.Printf("verso: post-apply action %q failed: %v", action.Name, err)
			http.Error(w, "apply action failed", http.StatusBadGateway)
			return
		}
	}
	writeOK(w)
}

func (s *Server) executeApplyAction(ctx context.Context, sid string, action plugin.ApplyAction) error {
	switch action.Name {
	case "set-system-time":
		return s.backend.SetSystemTime(ctx, sid, action.Args["datetime"], action.Args["timezone"])
	default:
		return fmt.Errorf("unsupported apply action %q", action.Name)
	}
}

// handleUCIConfirm disarms the pending rollback, keeping the applied
// configuration. The drawer polls this inside the rollback window; an error
// here means "not confirmed yet" to the poller, which retries.
func (s *Server) handleUCIConfirm(w http.ResponseWriter, r *http.Request) {
	if err := s.backend.UCIConfirm(r.Context(), s.sessionSID(r)); err != nil {
		log.Printf("verso: uci confirm failed: %v", err)
		http.Error(w, "confirm failed", http.StatusBadGateway)
		return
	}
	writeOK(w)
}

// handleUCIDiscard reverts every declared config with pending changes (ADR-010:
// the stage is one unit).
func (s *Server) handleUCIDiscard(w http.ResponseWriter, r *http.Request) {
	sid := s.sessionSID(r)
	changes, err := s.backend.UCIChanges(r.Context(), sid)
	if err != nil {
		log.Printf("verso: uci changes unavailable: %v", err)
		http.Error(w, "discard failed", http.StatusBadGateway)
		return
	}
	declared := s.declaredConfigsUnion()
	for cfg := range changes {
		if !declared[cfg] {
			continue
		}
		if err := s.backend.UCIRevert(r.Context(), sid, cfg); err != nil {
			log.Printf("verso: uci revert of %q failed: %v", cfg, err)
			http.Error(w, "discard failed", http.StatusBadGateway)
			return
		}
	}
	s.clearPendingApply(sid)
	writeOK(w)
}

func writeOK(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}
