// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/we-are-mono/verso/internal/history"
	"github.com/we-are-mono/verso/internal/plugin"
)

// Configs written straight to /etc/config (ADR-021): over SSH, by a script or
// by another tool. verso-rpcd spools each config's committed sections once its
// writes settle, every secret a fingerprint; the shell takes each reading and
// tells Verso's own writes from the rest, which are entries of their own, by
// no one.

// changesSpool is where verso-rpcd leaves the readings.
const changesSpool = "/var/run/verso/changes"

// spoolEvery is how often the spool is read.
const spoolEvery = 2 * time.Second

// ownWriteSlack is how long after an apply is recorded a write to its
// configs is still its own: the helper reads a config two seconds after its
// last write.
const ownWriteSlack = 10 * time.Second

// configReading is one config as verso-rpcd read it: when it was written,
// whether the helper wrote it itself, and its sections, a secret's value a
// {"secret": fingerprint}.
type configReading struct {
	Time     int64                     `json:"time"`
	Config   string                    `json:"config"`
	Own      bool                      `json:"own"`
	Sections map[string]map[string]any `json:"sections"`
}

// expectedWrites is when each config's writes are an apply's own: from the
// apply until a little after it was recorded, whether kept or rolled back.
type expectedWrites struct {
	mu sync.Mutex
	by map[string][2]time.Time
}

func (e *expectedWrites) expect(configs []string, from, to time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.by == nil {
		e.by = map[string][2]time.Time{}
	}
	for _, config := range configs {
		e.by[config] = [2]time.Time{from, to}
	}
}

// holds reports whether a write to config at at is an apply's own. at is
// in whole seconds, so the window opens a second early.
func (e *expectedWrites) holds(config string, at time.Time) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	span, ok := e.by[config]
	return ok && !at.Before(span[0].Add(-time.Second)) && !at.After(span[1])
}

// watchSpool takes the readings verso-rpcd spools until ctx ends.
func (s *Server) watchSpool(ctx context.Context, store *history.Store, dir string) {
	tick := time.NewTicker(spoolEvery)
	defer tick.Stop()
	for {
		s.takeSpool(ctx, store, dir)
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// applyPending reports whether an apply's rollback window is open: until it
// closes, a reading may be its write or its undoing.
func (s *Server) applyPending() bool {
	s.applies.mu.Lock()
	defer s.applies.mu.Unlock()
	return len(s.applies.by) > 0
}

// takeSpool takes every reading waiting in dir, oldest first. One that
// cannot be kept stays for the next pass.
func (s *Server) takeSpool(ctx context.Context, store *history.Store, dir string) {
	if s.applyPending() {
		return
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var names []string
	for _, f := range files {
		if name := f.Name(); !strings.HasPrefix(name, ".") && strings.HasSuffix(name, ".json") {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var reading configReading
		if err := json.Unmarshal(data, &reading); err != nil || reading.Config == "" {
			log.Printf("verso: config reading %s unreadable: %v", name, err)
			_ = os.Remove(path)
			continue
		}
		if err := s.takeReading(ctx, store, reading); err != nil {
			log.Printf("verso: config reading of %s not kept: %v", reading.Config, err)
			continue
		}
		_ = os.Remove(path)
	}
}

// takeReading keeps one config as it was read. The first reading of it is
// where its history starts; Verso's own write, the helper's or an apply's,
// is kept as it is; any other difference from the last reading is an entry.
func (s *Server) takeReading(ctx context.Context, store *history.Store, reading configReading) error {
	if !s.declaredConfigsUnion()[reading.Config] {
		return nil
	}
	sections, err := json.Marshal(reading.Sections)
	if err != nil {
		return err
	}
	at := time.Unix(reading.Time, 0)
	last, known, err := store.ConfigState(ctx, reading.Config)
	if err != nil {
		return err
	}
	keep := func() error { return store.SetConfigState(ctx, reading.Config, sections, at) }
	if !known || reading.Own || s.expected.holds(reading.Config, at) {
		return keep()
	}
	var before map[string]map[string]any
	if err := json.Unmarshal(last, &before); err != nil {
		return keep()
	}
	tuples, secret := diffConfig(before, reading.Sections)
	if len(tuples) == 0 {
		return keep()
	}
	if s.changeLog != nil {
		if _, err := s.changeLog.RecordChange(ctx, s.directChange(ctx, reading, before, tuples, secret)); err != nil {
			return err
		}
	}
	return keep()
}

// directChange is an edit written straight to a config as config history
// keeps it: by no one, at the time it was written, each row as the plugin
// that writes the config describes it, with what each change replaced.
func (s *Server) directChange(ctx context.Context, reading configReading, before map[string]map[string]any, tuples [][]string, secret map[string]bool) history.Change {
	config := reading.Config
	flat := make([]stagedChange, len(tuples))
	for i, tuple := range tuples {
		flat[i] = stagedChange{config: config, tuple: tuple, raw: humanizeChange(config, tuple)}
	}
	identity := func(s string) string { return s }
	groups, _ := s.describedGroups(ctx, directSource{s, config, reading.Sections}, flat, identity, func(string) func(string) string { return identity })
	secrets := map[string]bool{}
	for option := range secret {
		secrets[config+"."+option] = true
	}
	change := history.Change{Time: time.Unix(reading.Time, 0), Status: history.ChangeApplied, OldRead: true,
		Items: stagedItems(stagedView{Groups: groups}, secrets)}
	held := map[string]any{}
	for name, section := range before {
		held[name] = section
	}
	committed := map[string]map[string]any{config: held}
	for i := range change.Items {
		for j := range change.Items[i].Ops {
			op := &change.Items[i].Ops[j]
			op.Old = replaced(committed, *op)
		}
	}
	return change
}

// directSource is a config written straight to /etc/config: no one staged
// it, so each change is the page of the plugin that writes the config, which
// reads it as it was read.
type directSource struct {
	s        *Server
	config   string
	sections map[string]map[string]any
}

func (d directSource) authorOf(c stagedChange) (plugin.Manifest, stageAuthor, bool) {
	m, ok := d.s.configOwner(c.config)
	return m, stageAuthor{Plugin: m.ID, Page: m.Name}, ok
}

// snapshot is the config as it was read, a secret empty, as a plugin never
// sees one.
func (d directSource) snapshot(context.Context, plugin.Manifest) plugin.UCI {
	sections := make(map[string]any, len(d.sections))
	for name, section := range d.sections {
		options := make(map[string]any, len(section))
		for option, value := range section {
			if _, isSecret := value.(map[string]any); isSecret {
				value = ""
			}
			options[option] = value
		}
		sections[name] = options
	}
	return plugin.UCI{d.config: sections}
}

// diffConfig is what changed between two readings of a config, as `uci
// changes` would list it had it been staged: options set, removed, and a
// list's values added and removed, in each section in the order the config
// holds them; a section made with every option it holds; a section removed.
// secret names the options whose value is a fingerprint, set with no value.
func diffConfig(before, after map[string]map[string]any) ([][]string, map[string]bool) {
	var tuples [][]string
	secret := map[string]bool{}
	for _, name := range sectionsInOrder(after) {
		now := after[name]
		then, existed := before[name]
		typ, _ := now[".type"].(string)
		if thenType, _ := then[".type"].(string); existed && thenType != typ {
			tuples = append(tuples, []string{"remove", name})
			existed = false
		}
		if !existed {
			tuples = append(tuples, []string{"add", name, typ})
			then = nil
		}
		for _, option := range optionNames(then, now) {
			was, had := then[option]
			is, has := now[option]
			_, wasSecret := was.(map[string]any)
			_, isSecret := is.(map[string]any)
			switch {
			case wasSecret || isSecret:
				if !had || !has || !reflectEqual(was, is) {
					secret[option] = true
					if has {
						tuples = append(tuples, []string{"set", name, option, ""})
					} else {
						tuples = append(tuples, []string{"remove", name, option})
					}
				}
			case !has:
				tuples = append(tuples, []string{"remove", name, option})
			default:
				tuples = append(tuples, optionChange(name, option, was, had, is)...)
			}
		}
	}
	for _, name := range sectionsInOrder(before) {
		if _, kept := after[name]; !kept {
			tuples = append(tuples, []string{"remove", name})
		}
	}
	return tuples, secret
}

// optionChange is one option's change to is, from was where it had one: a
// value set, or a list's values added and removed.
func optionChange(section, option string, was any, had bool, is any) [][]string {
	list, isList := is.([]any)
	if !isList {
		if had && was == is {
			return nil
		}
		return [][]string{{"set", section, option, stringOf(is)}}
	}
	var tuples [][]string
	old, wasList := was.([]any)
	if had && !wasList {
		tuples = append(tuples, []string{"remove", section, option})
	}
	for _, v := range list {
		if !slices.Contains(old, v) {
			tuples = append(tuples, []string{"list-add", section, option, stringOf(v)})
		}
	}
	for _, v := range old {
		if !slices.Contains(list, v) {
			tuples = append(tuples, []string{"list-del", section, option, stringOf(v)})
		}
	}
	return tuples
}

// sectionsInOrder is a reading's sections in the order the config holds
// them.
func sectionsInOrder(sections map[string]map[string]any) []string {
	names := make([]string, 0, len(sections))
	for name := range sections {
		names = append(names, name)
	}
	index := func(name string) float64 {
		switch i := sections[name][".index"].(type) {
		case float64:
			return i
		case int:
			return float64(i)
		}
		return 0
	}
	sort.SliceStable(names, func(a, b int) bool {
		if index(names[a]) != index(names[b]) {
			return index(names[a]) < index(names[b])
		}
		return names[a] < names[b]
	})
	return names
}

// optionNames is every option either reading of a section holds, by name.
func optionNames(then, now map[string]any) []string {
	var names []string
	for _, section := range []map[string]any{then, now} {
		for option := range section {
			if !strings.HasPrefix(option, ".") && !slices.Contains(names, option) {
				names = append(names, option)
			}
		}
	}
	sort.Strings(names)
	return names
}

func stringOf(v any) string {
	s, _ := v.(string)
	return s
}

func reflectEqual(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

// changedConfigs is every config a change wrote, once each.
func changedConfigs(c history.Change) []string {
	var configs []string
	for _, item := range c.Items {
		for _, op := range item.Ops {
			if !slices.Contains(configs, op.Config) {
				configs = append(configs, op.Config)
			}
		}
	}
	return configs
}
