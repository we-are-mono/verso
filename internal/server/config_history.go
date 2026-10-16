// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"sync"

	"github.com/we-are-mono/verso/internal/alerts"
	"github.com/we-are-mono/verso/internal/history"
	"github.com/we-are-mono/verso/internal/plugin"
	"github.com/we-are-mono/verso/internal/secrets"
)

// changeRecorder keeps config history (ADR-021): history.db's, or a test's.
type changeRecorder interface {
	RecordChange(ctx context.Context, c history.Change) (int64, error)
	MarkUndone(ctx context.Context, ids []int64, by int64) error
}

// secretOptions is every option config history keeps no value of: OpenWrt's
// own (internal/secrets), and each installed plugin's (pluginSecrets).
func (s *Server) secretOptions() map[string]bool {
	base := secrets.Base()
	names := make(map[string]bool, len(base))
	for _, name := range base {
		names[name] = true
	}
	for _, m := range s.manifestList() {
		for _, name := range pluginSecrets(m) {
			names[name] = true
		}
	}
	return names
}

// pluginSecrets is the secrets m's manifest names in a config its acl
// writes. A plugin says which options of the configs it writes hold a secret,
// never of one it only reads or does not touch: a secret's values leave all
// of config history.
func pluginSecrets(m plugin.Manifest) []string {
	own := declaredUCIConfigs(m)
	var names []string
	for _, name := range m.Secrets {
		if config, _, ok := strings.Cut(name, "."); ok && own[config] {
			names = append(names, name)
		}
	}
	return names
}

// stagedPlain is sid's stage read in the words config history keeps, the
// pages' own, untranslated.
func (s *Server) stagedPlain(ctx context.Context, sid string) stagedView {
	identity := func(s string) string { return s }
	return s.staged(ctx, sid, identity, func(string) func(string) string { return identity })
}

// stagedChangeOf is sid's stage, as view read it, as config history keeps it
// once applied: the Pending changes page's rows, under the page each was made
// on, each with the uci changes it stands for and what each replaced, and who
// applied it from where. It is read before the apply, while the configs still
// hold what the stage replaces.
func (s *Server) stagedChangeOf(ctx context.Context, sid string, view stagedView, user, address string) history.Change {
	secrets := s.secretOptions()
	change := history.Change{User: user, Address: address, Items: stagedItems(view, secrets)}
	var configs []string
	seen := map[string]bool{}
	for _, item := range change.Items {
		for _, op := range item.Ops {
			if !seen[op.Config] && op.Op != "file" {
				seen[op.Config] = true
				configs = append(configs, op.Config)
			}
		}
	}
	if len(configs) == 0 {
		return change
	}
	// What the configs held is the history's to keep, not the apply's to
	// wait on: a read that fails leaves the change without old values.
	committed, err := s.backend.UCICommitted(ctx, sid, configs)
	if err != nil {
		log.Printf("verso: what the apply replaces could not be read: %v", err)
		return change
	}
	for i := range change.Items {
		for j := range change.Items[i].Ops {
			op := &change.Items[i].Ops[j]
			op.Old = replaced(committed, *op, secrets)
		}
	}
	change.OldRead = true
	return change
}

// scrubHistory takes every secret the shell knows out of config history:
// what a shell kept before it kept no copy of one, and what was kept of a
// secret before the list or a plugin's manifest named it. It runs when the
// history opens and each time the plugins are read again, one scrub at a
// time, and writes only what holds a value still.
func (s *Server) scrubHistory(ctx context.Context) {
	if s.historyStore == nil {
		return
	}
	s.scrubbing.Lock()
	defer s.scrubbing.Unlock()
	secrets := s.secretOptions()
	n, err := s.historyStore.ScrubSecrets(ctx, secrets, func(op history.ChangeOp) string {
		return humanizeChange(op.Config, opTuple(op), secrets)
	})
	s.scrubbed(ctx, n, err)
}

// scrubbed takes the outcome of a scrub run with ctx: n rows written, and
// err. A rewrite that fails leaves what the rows held in history.db's free
// pages, which the operator is told of by the storage alert, once until a
// scrub makes it, and the Changes page while it lasts (unrewritten); a scrub
// that makes the rewrite, or owes none, ends it. A scrub cut short, as ctx
// ended (a rescan's deadline, the history stopping), failed at nothing: what
// it leaves undone the next scrub does, the start's at the latest, which has
// no deadline, so it is only logged.
func (s *Server) scrubbed(ctx context.Context, n int, err error) {
	if n > 0 {
		log.Printf("verso: config history: %d rows scrubbed of secrets", n)
	}
	switch {
	case err != nil && ctx.Err() != nil:
		log.Printf("verso: config history: scrub cut short, left to the next: %v", err)
	case errors.Is(err, history.ErrNotRewritten):
		log.Printf("verso: config history: history.db keeps what its scrubbed rows held until it is rewritten, which every scrub tries: %v", err)
		if s.unrewritten.fail(err) && s.alerter != nil {
			s.alerter.Urgent(context.Background(), alerts.KindStorage, "config history could not be rewritten",
				err.Error()+". Secrets taken out of it stay in the file's free space until it is, which is tried each time the router starts and each time a plugin is installed or removed.")
		}
	case err != nil:
		log.Printf("verso: %v", err)
	default:
		s.unrewritten.clear()
	}
}

// unrewritten is why history.db still holds what a scrub took out of its
// rows: the rewrite they owe failed, and no scrub has made it since. Empty
// while the file holds nothing scrubbed.
type unrewritten struct {
	mu    sync.Mutex
	cause string
}

// fail takes a rewrite that failed with err, and answers whether the file
// held nothing scrubbed until then.
func (u *unrewritten) fail(err error) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	first := u.cause == ""
	u.cause = err.Error()
	return first
}

// clear takes a rewrite made, or none owed.
func (u *unrewritten) clear() {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.cause = ""
}

// read is why the file holds what was scrubbed; "" while it holds nothing
// scrubbed.
func (u *unrewritten) read() string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.cause
}

// coalesced runs a job in the background, one run at a time. A run asked for
// while one runs is made once more after it, however many were asked for
// meanwhile, so each ask is served by a run that starts after it. Once
// closed, it cuts the run in progress short and makes none after.
type coalesced struct {
	mu     sync.Mutex
	ended  chan struct{}      // closed once the runs end; nil while none runs
	cancel context.CancelFunc // cuts the runs in progress short; nil while none runs
	again  bool
	closed bool
}

// run makes job run in the background, now or after the run in progress,
// with a context that ends as c closes; once c is closed, it runs nothing.
func (c *coalesced) run(job func(context.Context)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	if c.ended != nil {
		c.again = true
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	ended := make(chan struct{})
	c.ended, c.cancel = ended, cancel
	go func() {
		defer close(ended)
		defer cancel()
		for {
			job(ctx)
			c.mu.Lock()
			again := c.again && !c.closed
			c.again = false
			if !again {
				c.ended, c.cancel = nil, nil
			}
			c.mu.Unlock()
			if !again {
				return
			}
		}
	}()
}

// close cuts the run in progress short, drops one asked for meanwhile, and
// makes run run nothing from then on. wait returns once the run has ended.
func (c *coalesced) close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed, c.again = true, false
	if c.cancel != nil {
		c.cancel()
	}
}

// wait returns once no run is in progress.
func (c *coalesced) wait() {
	c.mu.Lock()
	ended := c.ended
	c.mu.Unlock()
	if ended != nil {
		<-ended
	}
}

// stopScrubs cuts a rescan's scrub short, and starts none after, as the
// history stops: procd gives the shell seconds to stop, and a scrub may take
// minutes. What it leaves undone the next start's scrub does: rows it did not
// commit, and the rewrite it owes, kept in history.db (rewrite_owed).
func (s *Server) stopScrubs() {
	s.scrubs.close()
	s.scrubs.wait()
}

// opTuple is a secret's change as `uci changes` lists it, its value masked:
// what a verbatim row's raw line is made from. It is masked here, not by the
// names the shell knows, as a secret's change may name an option no list
// names any more, or one only the helper's reading of the manifests named.
func opTuple(op history.ChangeOp) []string {
	switch op.Op {
	case "remove-option":
		return []string{"remove", op.Section, op.Option}
	case "set", "list-add", "list-del":
		return []string{op.Op, op.Section, op.Option, maskedValue}
	}
	return []string{op.Op, op.Section}
}

// stagedItems is the review drawer's rows as config history keeps them,
// under the page each was made on, each with the uci changes it stands for.
func stagedItems(view stagedView, secrets map[string]bool) []history.ChangeItem {
	var items []history.ChangeItem
	for _, g := range view.Groups {
		for _, item := range g.Items {
			items = append(items, historyItem(g.Page, item, secrets))
		}
	}
	return items
}

// historyItem is one row of the stage as config history keeps it, under the
// page it was made on.
func historyItem(page string, item stagedItem, secrets map[string]bool) history.ChangeItem {
	kept := history.ChangeItem{Page: page, Plugin: item.plugin, Verb: item.Verb, Words: item.Label, Verbatim: item.Mono,
		SubjectKind: item.subject.Kind, SubjectName: item.subject.Name}
	for _, c := range item.changes {
		kept.Ops = append(kept.Ops, changeOp(c, secrets))
	}
	return kept
}

// applyNoteLimit is how long a note on an apply may be, in characters: a
// line, not a document.
const applyNoteLimit = 200

// applyNote is the review drawer's note as config history keeps it: one
// line, trimmed, and no longer than applyNoteLimit.
func applyNote(typed string) string {
	note := strings.Join(strings.Fields(typed), " ")
	if runes := []rune(note); len(runes) > applyNoteLimit {
		note = string(runes[:applyNoteLimit])
	}
	return note
}

// subjectOf is what a described row is about, as config history keeps it: the
// subject its plugin named, called by the section of its first change where
// the plugin gave no name. A kind outside the vocabulary is no subject, and
// the row is named by its config and section, as an undescribed row is.
func subjectOf(named *plugin.Subject, first stagedChange) plugin.Subject {
	if named == nil || !plugin.SubjectKinds[named.Kind] {
		return plugin.Subject{}
	}
	subject := *named
	if subject.Name == "" {
		subject.Name = describeChange(first).Section
	}
	return subject
}

// changeOp is one staged uci change as config history keeps it. A value is
// kept for the ops that carry one; a section's type rides as its option, as
// the describe hook has it.
func changeOp(c stagedChange, secrets map[string]bool) history.ChangeOp {
	d := describeChange(c)
	op := history.ChangeOp{Config: d.Config, Section: d.Section, Op: d.Op, Option: d.Option}
	switch d.Op {
	case "set", "list-add", "list-del", "file":
		value := d.Value
		op.Value = &value
	}
	switch d.Op {
	case "set", "list-add", "list-del", "remove-option":
		op.Secret = secrets[d.Config+"."+d.Option]
	}
	return op
}

// replaced is what op's target held committed, as JSON: the option's value
// for a change to an option, the whole section for a section removed, each
// option secrets names (config.option) in it not kept. A new section, a file,
// a secret, and anything that was not there replaced nothing.
func replaced(committed map[string]map[string]any, op history.ChangeOp, secrets map[string]bool) json.RawMessage {
	section, ok := committed[op.Config][op.Section].(map[string]any)
	if !ok || op.Secret {
		return nil
	}
	var old any
	switch op.Op {
	case "set", "list-add", "list-del", "remove-option":
		value, present := section[op.Option]
		if !present {
			return nil
		}
		old = value
	case "remove-section":
		old, _ = history.WithoutSecrets(op.Config, section, secrets)
	default:
		return nil
	}
	encoded, err := json.Marshal(old)
	if err != nil {
		return nil
	}
	return encoded
}

// changePages is the pages a change was made on, in the order its rows have
// them.
func changePages(c history.Change) []string {
	var pages []string
	seen := map[string]bool{}
	for _, item := range c.Items {
		if !seen[item.Page] {
			seen[item.Page] = true
			pages = append(pages, item.Page)
		}
	}
	return pages
}

// changesKeptDefault is how many entries config history keeps unless the
// operator says otherwise (verso.history.changes).
const changesKeptDefault = 1000

// SetChangesKept sets how many entries config history keeps; fewer than one
// keeps the default.
func (s *Server) SetChangesKept(n int) {
	if n < 1 {
		n = changesKeptDefault
	}
	s.changesKept = n
}

// foldHistory keeps config history to its limit, the oldest entries folded
// into the starting point.
func (s *Server) foldHistory(ctx context.Context) {
	if s.historyStore == nil {
		return
	}
	kept := s.changesKept
	if kept < 1 {
		kept = changesKeptDefault
	}
	if err := s.historyStore.FoldChanges(ctx, kept); err != nil {
		log.Printf("verso: config history not folded: %v", err)
	}
}
