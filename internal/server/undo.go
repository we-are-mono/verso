// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"slices"
	"sort"
	"strings"
	"sync"
)

// Undo takes one row, or one page's rows, out of a stage that holds others
// (ADR-010). rpcd lets go of a config's stage only whole, so an undo lets go
// of each config the rows touch and stages the rest of it again, through the
// same rpcd calls a save makes: nothing reaches into rpcd's own files. A
// config's staged files are let go of only when a file is what is undone.

// replayOp is one staged change as an undo takes it out of the stage or puts
// it back: its config, its coalesced uci change, and who staged it, where
// that is on record, at which of its addresses.
type replayOp struct {
	config  string
	tuple   []string
	author  stageAuthor
	address string // where the record was kept; empty where none was
}

// errRowsGone is an undo of rows the stage no longer holds: another tab
// applied, discarded or undid them first.
var errRowsGone = errors.New("the changes are no longer pending")

// undoRecord is what a session's last undo took out of its stage: what it
// says it took, and the token its put-back form carries. It is offered back
// once, on the page the undo leads to, and kept until the next undo, an
// apply or a discard.
type undoRecord struct {
	ops        []replayOp
	words      string
	token      string
	restorable bool // it holds a change the shell can write again
	announced  bool // the page has offered it back
}

// undoRecords is each session's last undo.
type undoRecords struct {
	mu sync.Mutex
	by map[string]undoRecord
}

func (u *undoRecords) set(sid string, record undoRecord) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.by == nil {
		u.by = map[string]undoRecord{}
	}
	u.by[sid] = record
}

// announce is sid's last undo the first time a page asks to offer it back;
// after that it is not offered again.
func (u *undoRecords) announce(sid string) (undoRecord, bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	record, ok := u.by[sid]
	if !ok || record.announced {
		return undoRecord{}, false
	}
	record.announced = true
	u.by[sid] = record
	return record, true
}

// take hands over sid's last undo for its put-back, once, when token is its.
func (u *undoRecords) take(sid, token string) (undoRecord, bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	record, ok := u.by[sid]
	if !ok || token == "" || record.token != token {
		return undoRecord{}, false
	}
	delete(u.by, sid)
	return record, true
}

// forget drops sid's last undo: its stage was applied or discarded.
func (u *undoRecords) forget(sid string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	delete(u.by, sid)
}

// undoRows takes the rows named ids out of sid's stage, leaving every other
// change staged as it was, and answers what it took. A replay that fails
// part way stages the config again as it was before the undo.
func (s *Server) undoRows(ctx context.Context, sid string, ids []string) ([]replayOp, error) {
	identity := func(s string) string { return s }
	view := s.staged(ctx, sid, identity, func(string) func(string) string { return identity })
	named := map[string]bool{}
	for _, id := range ids {
		named[id] = true
	}
	undone := map[string]bool{} // changeKey → taken
	var configs []string
	for _, g := range view.Groups {
		for _, item := range g.Items {
			if !named[item.ID] {
				continue
			}
			for _, c := range item.changes {
				undone[changeKey(c.config, c.tuple)] = true
				if !slices.Contains(configs, c.config) {
					configs = append(configs, c.config)
				}
			}
		}
	}
	if len(configs) == 0 {
		return nil, errRowsGone
	}
	sort.Strings(configs)
	journal, err := s.backend.UCIChanges(ctx, sid)
	if err != nil {
		return nil, err
	}
	var taken []replayOp
	for _, config := range configs {
		var all, keep, take []replayOp
		uci, files := false, false
		for _, ch := range coalesceChanges(journal[config]) {
			op := s.recorded(sid, config, ch)
			all = append(all, op)
			if !undone[changeKey(config, ch)] {
				keep = append(keep, op)
				continue
			}
			take = append(take, op)
			if ch[0] == "file" {
				files = true
			} else {
				uci = true
			}
		}
		if files {
			if err := s.backend.UnstageFiles(ctx, sid, config); err != nil {
				return taken, err
			}
		}
		if uci {
			staged := s.stagedOrder(ctx, sid, config, all)
			var order []string
			if reorders(keep) {
				order = staged
			}
			if err := s.backend.UCIUnstage(ctx, sid, config); err != nil {
				return taken, err
			}
			if err := s.replay(ctx, sid, config, keep, order); err != nil {
				s.restage(ctx, sid, config, all, staged)
				return taken, fmt.Errorf("undo in %s: %w", config, err)
			}
		}
		for _, op := range take {
			if op.address != "" {
				s.authors.drop(sid, op.address)
			}
		}
		taken = append(taken, take...)
	}
	return taken, nil
}

// putBack stages again what an undo took out, as it was and with the time it
// was first staged. A staged file is not put back: its contents are the
// helper's, not the shell's to write again.
func (s *Server) putBack(ctx context.Context, sid string, ops []replayOp) error {
	byConfig := map[string][]replayOp{}
	var configs []string
	for _, op := range ops {
		if op.tuple[0] == "file" {
			continue
		}
		if _, seen := byConfig[op.config]; !seen {
			configs = append(configs, op.config)
		}
		byConfig[op.config] = append(byConfig[op.config], op)
	}
	for _, config := range configs {
		if err := s.replay(ctx, sid, config, byConfig[config], nil); err != nil {
			return fmt.Errorf("put back in %s: %w", config, err)
		}
	}
	return nil
}

// restage stages a config again as it was, after a replay failed: what it
// cannot write again is logged, since the stage is already short of it.
func (s *Server) restage(ctx context.Context, sid, config string, all []replayOp, order []string) {
	if err := s.backend.UCIUnstage(ctx, sid, config); err != nil {
		log.Printf("verso: undo could not let go of %s to restore it: %v", config, err)
		return
	}
	if err := s.replay(ctx, sid, config, all, order); err != nil {
		log.Printf("verso: undo could not restore %s's stage: %v", config, err)
	}
}

// changeKey names one coalesced change of a config.
func changeKey(config string, tuple []string) string {
	return config + "\x00" + strings.Join(tuple, "\x00")
}

// recorded is a change with who staged it, by the option it touches, else
// its section, as authorOf finds it; with no record it carries none.
func (s *Server) recorded(sid, config string, ch []string) replayOp {
	op := replayOp{config: config, tuple: ch}
	var addresses []string
	if section, option, ok := changedOption(ch); ok {
		addresses = append(addresses, optionAddress(config, section, option))
	}
	if len(ch) >= 2 && ch[0] != "file" {
		addresses = append(addresses, sectionAddress(config, ch[1]))
	}
	for _, address := range addresses {
		if who, ok := s.authors.of(sid, address); ok {
			op.author, op.address = who, address
			break
		}
	}
	return op
}

// stagedOrder is the config's sections in the order the stage holds them,
// read before the stage is let go of, where one of ops reorders them; nil
// where none does.
func (s *Server) stagedOrder(ctx context.Context, sid, config string, ops []replayOp) []string {
	if !reorders(ops) {
		return nil
	}
	sections, err := s.backend.UCIConfig(ctx, sid, config)
	if err != nil {
		log.Printf("verso: undo could not read %s's staged order: %v", config, err)
		return nil
	}
	return sectionOrder(sections)
}

// reorders is whether one of ops is a section reorder.
func reorders(ops []replayOp) bool {
	return slices.ContainsFunc(ops, func(op replayOp) bool { return op.tuple[0] == "order" })
}

// replay stages ops into config through the calls a save makes, in order: a
// section made is made again under the name rpcd gives it now, and every
// later change to it follows the name; a list is written whole, as rpcd
// journals a list (its option removed, each value added); order, where
// given, is the section order to stage last. Each change's record is noted
// again, at its section's new name, with the time it carried.
func (s *Server) replay(ctx context.Context, sid, config string, ops []replayOp, order []string) error {
	names := map[string]string{}
	name := func(section string) string {
		if n, ok := names[section]; ok {
			return n
		}
		return section
	}
	listed := map[string]bool{}
	for _, op := range ops {
		if d := describeChange(stagedChange{config: config, tuple: op.tuple}); d.Op == "list-add" || d.Op == "list-del" {
			listed[d.Section+"\x00"+d.Option] = true
		}
	}
	type list struct {
		section, option string
		values          []string
	}
	var lists []*list
	byKey := map[string]*list{}
	var committed map[string]map[string]any
	for _, op := range ops {
		d := describeChange(stagedChange{config: config, tuple: op.tuple})
		key := d.Section + "\x00" + d.Option
		var err error
		switch d.Op {
		case "add-section":
			given := ""
			if !anonymousSection.MatchString(d.Section) {
				given = d.Section
			}
			var made string
			if made, err = s.backend.UCIAdd(ctx, sid, config, d.Option, given); err == nil {
				names[d.Section] = made
			}
		case "set":
			err = s.backend.UCISet(ctx, sid, config, name(d.Section), map[string]any{d.Option: d.Value})
		case "remove-option":
			if listed[key] {
				// The list written whole after it removes the option first.
				l := &list{section: d.Section, option: d.Option}
				byKey[key] = l
				lists = append(lists, l)
				break
			}
			err = s.backend.UCIDelete(ctx, sid, config, name(d.Section), d.Option)
		case "remove-section":
			err = s.backend.UCIDelete(ctx, sid, config, name(d.Section), "")
		case "list-add", "list-del":
			l := byKey[key]
			if l == nil {
				if committed == nil {
					if committed, err = s.backend.UCICommitted(ctx, sid, []string{config}); err != nil {
						return err
					}
				}
				l = &list{section: d.Section, option: d.Option, values: committedList(committed[config], d.Section, d.Option)}
				byKey[key] = l
				lists = append(lists, l)
			}
			if d.Op == "list-add" {
				l.values = append(l.values, d.Value)
			} else {
				l.values = slices.DeleteFunc(l.values, func(v string) bool { return v == d.Value })
			}
		case "order", "file":
			// The order is staged last, whole; a staged file is the helper's.
		default:
			err = fmt.Errorf("a change the undo cannot write again: %v", op.tuple)
		}
		if err != nil {
			return err
		}
		if op.address != "" {
			s.authors.note(sid, renamed(op.address, config, d.Section, name(d.Section)), op.author)
		}
	}
	for _, l := range lists {
		var err error
		if len(l.values) == 0 {
			err = s.backend.UCIDelete(ctx, sid, config, name(l.section), l.option)
		} else {
			err = s.backend.UCISet(ctx, sid, config, name(l.section), map[string]any{l.option: l.values})
		}
		if err != nil {
			return err
		}
	}
	if len(order) > 0 {
		mapped := make([]string, len(order))
		for i, section := range order {
			mapped[i] = name(section)
		}
		return s.backend.UCIOrder(ctx, sid, config, mapped)
	}
	return nil
}

// renamed is a record's address with its section under its new name.
func renamed(address, config, section, now string) string {
	if section == now {
		return address
	}
	return config + "." + now + strings.TrimPrefix(address, config+"."+section)
}

// committedList is a committed option as a list: its values, a single value
// as one, nothing where it is not set.
func committedList(sections map[string]any, section, option string) []string {
	values, _ := sections[section].(map[string]any)
	switch v := values[option].(type) {
	case string:
		return []string{v}
	case []any:
		var out []string
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return slices.Clone(v)
	}
	return nil
}
