// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/we-are-mono/verso/internal/history"
	"github.com/we-are-mono/verso/internal/plugin"
)

// Config history's acts (ADR-021): Undo stages the reverse of one entry,
// Return to this point the reverse of every entry in force since, and Stage
// again an entry's own writes. Each only stages, through the broker a Save
// uses, and goes live from the review drawer's Apply.

// actPlan is what an act stages: its writes, each under the row it came from,
// and what it says about them. Rows are the options it returns, as they are
// now and as they will be. Secrets and Files are what it leaves as they are,
// having no value to write; Unknown is a value it needs and history never read.
type actPlan struct {
	Steps   []planStep
	Rows    []planRow
	Secrets []string
	Files   []string
	Unknown bool
}

// planStep is one write, staged as the plugin whose page it was made on.
type planStep struct {
	plugin, page string
	op           plugin.CommitOp
}

// planRow is one option an act returns: as it is now, and as it will be.
type planRow struct{ Option, Now, Then string }

// touch is one uci change of an entry, with the row it was made under.
type touch struct {
	change history.Change
	item   history.ChangeItem
	op     history.ChangeOp
}

// bySection is the entries' changes per section, in the order the sections
// were first touched; a change to a file is no section's.
func bySection(entries []history.Change, plan *actPlan) ([]string, map[string][]touch) {
	var order []string
	touches := map[string][]touch{}
	for _, c := range entries {
		for _, item := range c.Items {
			for _, op := range item.Ops {
				key := op.Config + "." + op.Section
				if op.Op == "file" {
					if !slices.Contains(plan.Files, key) {
						plan.Files = append(plan.Files, key)
					}
					continue
				}
				if _, seen := touches[key]; !seen {
					order = append(order, key)
				}
				touches[key] = append(touches[key], touch{c, item, op})
			}
		}
	}
	return order, touches
}

func (p *actPlan) step(who history.ChangeItem, op plugin.CommitOp) {
	p.Steps = append(p.Steps, planStep{plugin: who.Plugin, page: who.Page, op: op})
}

func (p *actPlan) secret(path string) {
	if !slices.Contains(p.Secrets, path) {
		p.Secrets = append(p.Secrets, path)
	}
}

// optionOp reports whether an op changes one option rather than a section.
func optionOp(op history.ChangeOp) bool {
	switch op.Op {
	case "set", "remove-option", "list-add", "list-del":
		return true
	}
	return false
}

// reverseOf is what returns the configs to how they were before the first of
// entries (oldest first): each option to what the first change to it
// replaced, each section made since removed, each removed since made again
// with what it held but its secrets, which were not kept. An option changed
// and changed back needs nothing.
func reverseOf(entries []history.Change) actPlan {
	var plan actPlan
	order, touches := bySection(entries, &plan)
	for _, key := range order {
		ts := touches[key]
		first := ts[0]
		config, section := first.op.Config, first.op.Section
		existedBefore := first.op.Op != "add-section"
		existsNow, removedAt := existedBefore, -1
		for i, t := range ts {
			switch t.op.Op {
			case "add-section":
				existsNow = true
			case "remove-section":
				existsNow = false
				if removedAt < 0 {
					removedAt = i
				}
			}
		}
		if !existedBefore {
			if existsNow {
				plan.step(first.item, plugin.CommitOp{Config: config, Section: section, Delete: true})
				plan.Rows = append(plan.Rows, planRow{key, first.op.Option, none})
			}
			continue
		}
		before := ts
		if removedAt >= 0 {
			before = ts[:removedAt]
		}
		options, then, now := optionStates(before, &plan)
		if removedAt >= 0 {
			removed := ts[removedAt]
			var held map[string]any
			if !removed.change.OldRead || json.Unmarshal(removed.op.Old, &held) != nil || held == nil {
				plan.Unknown = true
				continue
			}
			typ, _ := held[".type"].(string)
			values := map[string]any{}
			for k, v := range held {
				switch {
				case strings.HasPrefix(k, "."):
				case isFingerprint(v):
					plan.secret(key + "." + k)
				default:
					values[k] = uciValue(v)
				}
			}
			for _, option := range options {
				if then[option] == nil {
					delete(values, option)
				} else {
					values[option] = then[option]
				}
			}
			name := section
			if anonymous, _ := held[".anonymous"].(bool); anonymous {
				name = ""
			}
			if existsNow {
				plan.step(removed.item, plugin.CommitOp{Config: config, Section: section, Delete: true})
			}
			plan.step(removed.item, plugin.CommitOp{Config: config, Section: name, Type: typ, Values: values})
			plan.Rows = append(plan.Rows, planRow{key, none, typ})
			continue
		}
		values := map[string]any{}
		for _, option := range options {
			if equalValues(now[option], then[option]) {
				continue
			}
			values[option] = then[option]
			plan.Rows = append(plan.Rows, planRow{key + "." + option, rowValue(now[option]), rowValue(then[option])})
		}
		if len(values) > 0 {
			plan.step(first.item, plugin.CommitOp{Config: config, Section: section, Values: values})
		}
	}
	return plan
}

// optionStates follows each option the touches change: what it held before
// the first of them (then) and what it holds after the last (now), in the
// order first touched. A secret has no value to follow; an option whose first
// change was recorded without what it replaced makes the plan unknown.
func optionStates(ts []touch, plan *actPlan) (order []string, then, now map[string]any) {
	then, now = map[string]any{}, map[string]any{}
	skip := map[string]bool{}
	for _, t := range ts {
		if !optionOp(t.op) {
			continue
		}
		option := t.op.Option
		if t.op.Secret {
			plan.secret(t.op.Config + "." + t.op.Section + "." + option)
			skip[option] = true
		}
		if skip[option] {
			continue
		}
		if _, seen := then[option]; !seen {
			before, ok := oldOf(t)
			if !ok {
				plan.Unknown = true
				skip[option] = true
				continue
			}
			order = append(order, option)
			then[option], now[option] = before, before
		}
		now[option] = applied(now[option], t.op)
	}
	return order, then, now
}

// oldOf is what an option held before a change, as uci would write it: a
// string, a list, or nil for nothing. Without its entry's old values read it
// is not known.
func oldOf(t touch) (any, bool) {
	if !t.change.OldRead {
		return nil, false
	}
	if t.op.Old == nil {
		return nil, true
	}
	var v any
	if json.Unmarshal(t.op.Old, &v) != nil {
		return nil, false
	}
	switch v.(type) {
	case string, []any:
		return uciValue(v), true
	}
	return nil, false
}

// uciValue is a value read from JSON as uci writes it: a string, or a list of
// them.
func uciValue(v any) any {
	if list, ok := v.([]any); ok {
		out := make([]string, len(list))
		for i, item := range list {
			out[i] = fmt.Sprint(item)
		}
		return out
	}
	return fmt.Sprint(v)
}

// applied is an option's value after op.
func applied(value any, op history.ChangeOp) any {
	v := ""
	if op.Value != nil {
		v = *op.Value
	}
	list, _ := value.([]string)
	switch op.Op {
	case "set":
		return v
	case "remove-option":
		return nil
	case "list-add":
		return append(slices.Clone(list), v)
	case "list-del":
		return slices.DeleteFunc(slices.Clone(list), func(item string) bool { return item == v })
	}
	return value
}

func equalValues(a, b any) bool {
	if al, ok := a.([]string); ok {
		bl, ok := b.([]string)
		return ok && slices.Equal(al, bl)
	}
	return a == b
}

// none is an option that holds nothing, as a row shows it.
const none = "—"

// rowValue is a value as a row shows it: nothing as a dash, empty as uci's
// ”, a list's values apart.
func rowValue(v any) string {
	switch v := v.(type) {
	case nil:
		return none
	case []string:
		return strings.Join(v, " ")
	case string:
		if v == "" {
			return "''"
		}
		return v
	}
	return fmt.Sprint(v)
}

// anonymousSection is how uci names a section that has no name of its own.
var anonymousSection = regexp.MustCompile(`^cfg[0-9a-f]{6}$`)

// forwardOf is what writes an entry's changes again: each option as it left
// it, each section it made made again, each it removed removed. A list is
// written whole, from what it held before.
func forwardOf(c history.Change) actPlan {
	var plan actPlan
	order, touches := bySection([]history.Change{c}, &plan)
	for _, key := range order {
		ts := touches[key]
		first := ts[0]
		config, section := first.op.Config, first.op.Section
		made, removed, typ := false, false, ""
		for _, t := range ts {
			switch t.op.Op {
			case "add-section":
				made, removed, typ = true, false, t.op.Option
			case "remove-section":
				removed = true
			}
		}
		if removed {
			if !made {
				plan.step(first.item, plugin.CommitOp{Config: config, Section: section, Delete: true})
			}
			continue
		}
		values := map[string]any{}
		started := map[string]bool{}
		for _, t := range ts {
			if !optionOp(t.op) {
				continue
			}
			option := t.op.Option
			if t.op.Secret {
				plan.secret(key + "." + option)
				continue
			}
			if !started[option] {
				started[option] = true
				if !made && (t.op.Op == "list-add" || t.op.Op == "list-del") {
					before, ok := oldOf(t)
					if !ok {
						plan.Unknown = true
						continue
					}
					values[option] = before
				}
			}
			values[option] = applied(values[option], t.op)
		}
		if made {
			name := section
			if anonymousSection.MatchString(section) {
				name = ""
			}
			for option, v := range values {
				if v == nil {
					delete(values, option)
				}
			}
			plan.step(first.item, plugin.CommitOp{Config: config, Section: name, Type: typ, Values: values})
			continue
		}
		if len(values) > 0 {
			plan.step(first.item, plugin.CommitOp{Config: config, Section: section, Values: values})
		}
	}
	return plan
}

// sinceThen is what has been done since x and is in force now, oldest first:
// every later apply that holds, less an undo of something after x, or a
// return to x or later, whose work cancels within the span.
func sinceThen(all []history.Change, x history.Change) []history.Change {
	var out []history.Change
	for i := len(all) - 1; i >= 0; i-- {
		c := all[i]
		if c.ID <= x.ID || c.Status != history.ChangeApplied {
			continue
		}
		if (c.Relation == "undo" && c.Related > x.ID) || (c.Relation == "return" && c.Related >= x.ID) {
			continue
		}
		out = append(out, c)
	}
	return out
}

// inForceAfter is every apply after x still in force, oldest first: what a
// return to x undoes.
func inForceAfter(all []history.Change, x history.Change) []int64 {
	var ids []int64
	for i := len(all) - 1; i >= 0; i-- {
		if c := all[i]; c.ID > x.ID && c.Status == history.ChangeApplied {
			ids = append(ids, c.ID)
		}
	}
	return ids
}

// touchedAgain is every apply after c, in force, that changed an option or
// section c changed: undoing c writes over them.
func touchedAgain(all []history.Change, c history.Change) []history.Change {
	options, sections := map[string]bool{}, map[string]bool{}
	for _, item := range c.Items {
		for _, op := range item.Ops {
			if optionOp(op) {
				options[op.Config+"."+op.Section+"."+op.Option] = true
			} else {
				sections[op.Config+"."+op.Section] = true
			}
		}
	}
	var out []history.Change
	for i := len(all) - 1; i >= 0; i-- {
		later := all[i]
		if later.ID <= c.ID || later.Status != history.ChangeApplied {
			continue
		}
		hit := false
		for _, item := range later.Items {
			for _, op := range item.Ops {
				section := op.Config + "." + op.Section
				hit = hit || sections[section] || options[section+"."+op.Option] || (!optionOp(op) && hasPrefixKey(options, section+"."))
			}
		}
		if hit {
			out = append(out, later)
		}
	}
	return out
}

// stageRelation is what the next apply of a session's stage is to history:
// the entry it undoes, returns to or stages again, and the entries it undoes.
type stageRelation struct {
	relation string
	related  int64
	reverses []int64
}

// stageRelations holds each session's stage relation until it is applied or
// discarded.
type stageRelations struct {
	mu sync.Mutex
	by map[string]stageRelation
}

func (r *stageRelations) set(sid string, rel stageRelation) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.by == nil {
		r.by = map[string]stageRelation{}
	}
	r.by[sid] = rel
}

// take hands over sid's relation for the apply it describes, once.
func (r *stageRelations) take(sid string) (stageRelation, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rel, ok := r.by[sid]
	delete(r.by, sid)
	return rel, ok
}

// forget drops sid's relation with the stage it described.
func (r *stageRelations) forget(sid string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.by, sid)
}

// stagePlan stages a plan's writes through the broker, each as the plugin
// whose page it was made on; one whose plugin is gone, as the plugin that
// writes its config now, and one of the shell's own configs as the shell.
func (s *Server) stagePlan(ctx context.Context, sid string, plan actPlan, tr func(string) string) bool {
	for _, step := range plan.Steps {
		m, ok := s.manifestByID(step.plugin)
		if !ok || !declaredUCIConfigs(m)[step.op.Config] {
			if m, ok = s.configOwner(step.op.Config); !ok {
				if !slices.Contains(shellConfigs, step.op.Config) {
					return false
				}
				m = shellManifest()
			}
		}
		// A write no row made (a return to the starting point) is the
		// writer's own page.
		page := step.page
		if page == "" {
			page = m.Name
		}
		who := stageAuthor{Plugin: m.ID, Page: page}
		if _, _, staged := s.brokerStage(ctx, m, who, sid, []plugin.CommitOp{step.op}, tr); !staged {
			return false
		}
	}
	return true
}

// shellManifest is the shell as a writer of its own configs.
func shellManifest() plugin.Manifest {
	m := plugin.Manifest{Name: "Verso"}
	for _, config := range shellConfigs {
		m.ACL.Write = append(m.ACL.Write, plugin.ACLScope{Scope: "uci", Object: config, Function: "write"})
	}
	return m
}

// hasPrefixKey reports whether any key of set starts with prefix.
func hasPrefixKey(set map[string]bool, prefix string) bool {
	for key := range set {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

// changeAct stages act on c (undo, return or again) and remembers what the
// stage is to history, or says why it could not. A session rpcd will not let
// write a config the act stages into is refused before any of it is staged.
func (s *Server) changeAct(ctx context.Context, g *sessionGrants, act string, c history.Change, all []history.Change, tr func(string) string) string {
	var plan actPlan
	var rel stageRelation
	switch act {
	case "undo":
		if c.Status != history.ChangeApplied {
			return tr("Only a change in force can be undone.")
		}
		plan, rel = reverseOf([]history.Change{c}), stageRelation{relation: "undo", related: c.ID, reverses: []int64{c.ID}}
		if !c.OldRead {
			plan.Unknown = true
		}
	case "return":
		plan, rel = reverseOf(sinceThen(all, c)), stageRelation{relation: "return", related: c.ID, reverses: inForceAfter(all, c)}
	case "again":
		if c.Status == history.ChangeApplied {
			return tr("This change is in force already.")
		}
		plan, rel = forwardOf(c), stageRelation{relation: "again", related: c.ID}
		plan.Unknown = false
	default:
		return tr("That is not something a change can do.")
	}
	switch {
	case plan.Unknown:
		return tr("What it replaced was never recorded, so it cannot be put back.")
	case len(plan.Steps) == 0:
		return tr("There is nothing to stage.")
	case !mayStage(ctx, g, plan):
		return tr(g.refusal(notWritable))
	case !s.stagePlan(ctx, g.sid, plan, tr):
		return tr("The changes could not be staged. Some may have been: review or discard them from the staged changes.")
	}
	s.relations.set(g.sid, rel)
	return ""
}

// startPlan is what returns each config from how it reads now to how it
// stood at the starting point: each option back to what it held, a section
// made since removed, one removed since made again as it was. A secret, kept
// only as its fingerprint, stays as it is.
func startPlan(start, now map[string]map[string]map[string]any) actPlan {
	var plan actPlan
	configs := make([]string, 0, len(start))
	for config := range start {
		configs = append(configs, config)
	}
	slices.Sort(configs)
	nobody := history.ChangeItem{}
	for _, config := range configs {
		then, current := start[config], now[config]
		if current == nil {
			continue
		}
		for _, name := range sectionsInOrder(then) {
			key := config + "." + name
			was := then[name]
			is, exists := current[name]
			typ, _ := was[".type"].(string)
			if nowType, _ := is[".type"].(string); !exists || nowType != typ {
				if exists {
					plan.step(nobody, plugin.CommitOp{Config: config, Section: name, Delete: true})
				}
				values := map[string]any{}
				for option, v := range was {
					switch {
					case strings.HasPrefix(option, "."):
					case isFingerprint(v):
						plan.secret(key + "." + option)
					default:
						values[option] = uciValue(v)
					}
				}
				section := name
				if anonymous, _ := was[".anonymous"].(bool); anonymous {
					section = ""
				}
				plan.step(nobody, plugin.CommitOp{Config: config, Section: section, Type: typ, Values: values})
				plan.Rows = append(plan.Rows, planRow{key, none, typ})
				continue
			}
			values := map[string]any{}
			for _, option := range optionNames(was, is) {
				a, had := was[option]
				b, has := is[option]
				if isFingerprint(a) || isFingerprint(b) {
					if !reflectEqual(a, b) {
						plan.secret(key + "." + option)
					}
					continue
				}
				if had == has && reflectEqual(a, b) {
					continue
				}
				thenValue, nowValue := presentValue(a, had), presentValue(b, has)
				values[option] = thenValue
				plan.Rows = append(plan.Rows, planRow{key + "." + option, rowValue(nowValue), rowValue(thenValue)})
			}
			if len(values) > 0 {
				plan.step(nobody, plugin.CommitOp{Config: config, Section: name, Values: values})
			}
		}
		for _, name := range sectionsInOrder(current) {
			if _, kept := then[name]; !kept {
				typ, _ := current[name][".type"].(string)
				plan.step(nobody, plugin.CommitOp{Config: config, Section: name, Delete: true})
				plan.Rows = append(plan.Rows, planRow{config + "." + name, typ, none})
			}
		}
	}
	return plan
}

// isFingerprint reports whether a value stands for a secret's: its
// fingerprint, as verso-rpcd spools one, or the secret not kept, as a section
// removed keeps it (history.NotKept).
func isFingerprint(v any) bool {
	_, ok := v.(map[string]any)
	return ok
}

// presentValue is a value as uci writes it, or nil where there is none.
func presentValue(v any, present bool) any {
	if !present {
		return nil
	}
	return uciValue(v)
}
