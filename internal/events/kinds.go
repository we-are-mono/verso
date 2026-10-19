// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// Package events is what the router knows of each kind of event it records
// (ADR-019, ADR-028): the router's own kinds, and each installed plugin's
// declared events, read alike by the Events page, its grants, the alerts and
// the stream that says events to the operators looking.
package events

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/we-are-mono/verso/internal/history"
	"github.com/we-are-mono/verso/internal/plugin"
	"github.com/we-are-mono/verso/internal/units"
)

// Grant is the session.access question that lets a session see an event:
// rpcd's grant of the read the event's fact comes from (ADR-007, ADR-019).
type Grant struct{ Scope, Object, Function string }

// The grants of the router's own kinds.
var (
	// InterfaceGrant guards the internet's outages: the read they are seen by.
	InterfaceGrant = Grant{"ubus", "network.interface", "dump"}
	// SystemGrant guards the router starting.
	SystemGrant = Grant{"ubus", "system", "info"}
	// AccessCredentialsGrant guards the web certificate's replacement: the
	// read that shows the served certificate's facts.
	AccessCredentialsGrant = Grant{"ubus", "verso", "accessCredentials"}
)

// Words says one event of a kind in the reader's words: what happened, and
// its weight in the tone vocabulary. tr is the reader's translator, the
// event's plugin's for a plugin's kind.
type Words func(e history.Event, tr func(string) string) (what, tone string)

// Kind is one kind of event.
type Kind struct {
	Name string
	// Plugin is the plugin whose words these are, translated through its
	// catalog; empty for the router's own.
	Plugin string
	Words  Words
	// Grant lets a session see the event; nil, every session may.
	Grant *Grant
	// ByChange: the event shows under its config history entry instead.
	ByChange bool
	// Announced: root may announce it, and the shell takes it from the spool.
	Announced bool
	// Said as the top-right notification to the operators looking.
	Said bool
	// Alert is the alert kind it alerts under, empty when it does not.
	Alert string
	// Ends are the kinds this one closes, as the internet coming back closes
	// an outage.
	Ends []string
	// Unknown: no installed plugin declares it, so it shows only to a
	// session that may read the whole of config history.
	Unknown bool
	// declared is a plugin's kind's declaration, the words it is recorded with.
	declared *plugin.EventDeclaration
}

// Detail is what an announced event keeps beside its kind: the spool name it
// was taken under, and for a plugin's event its words as declared when it was
// recorded, so it still reads once the plugin is gone.
type Detail struct {
	Spool    string   `json:"spool"`
	Plugin   string   `json:"plugin,omitempty"`
	Sentence string   `json:"sentence,omitempty"`
	Args     []string `json:"args,omitempty"`
	Tone     string   `json:"tone,omitempty"`
	Read     string   `json:"read,omitempty"`
}

// CertificateReplaced is the web certificate's replacement, announced by the
// root install command and the helper's hand install (ADR-017 §6).
const CertificateReplaced = "certificate.replaced"

// certificateSentence is CertificateReplaced's words, its arguments the
// certificate's name and the day it expires.
const certificateSentence = "The web interface’s certificate was replaced, for %s until %s."

// Table is the kinds the router records: its own, and the declared events of
// the plugins installed when it was made.
type Table struct {
	kinds   map[string]Kind
	plugins []AlertPlugin
	// ended is the kinds another kind ends: the ones that open an incident.
	ended map[string]bool
}

// AlertPlugin is an installed plugin that declares an alerting event, an
// alert kind of its own by its id.
type AlertPlugin struct{ ID, Name string }

// NewTable is the router's kinds with each manifest's declared events.
func NewTable(manifests []plugin.Manifest) *Table {
	t := &Table{kinds: map[string]Kind{}}
	for _, k := range core() {
		t.kinds[k.Name] = k
	}
	for _, m := range manifests {
		if m.Commands == nil {
			continue
		}
		alerts := false
		for _, e := range m.Commands.Events {
			k := pluginKind(m.ID, e)
			t.kinds[k.Name] = k
			alerts = alerts || e.Alert
		}
		if alerts {
			t.plugins = append(t.plugins, AlertPlugin{ID: m.ID, Name: m.Name})
		}
	}
	t.ended = map[string]bool{}
	for _, k := range t.kinds {
		for _, end := range k.Ends {
			t.ended[end] = true
		}
	}
	return t
}

// Ended reports whether another kind ends kind, so that an alert of it opens
// an incident the other closes (ADR-020).
func (t *Table) Ended(kind string) bool { return t.ended[kind] }

// Kind is the kind named name, if the router records it.
func (t *Table) Kind(name string) (Kind, bool) {
	k, ok := t.kinds[name]
	return k, ok
}

// Of is e's kind: the table's, or, for a kind no installed plugin declares,
// one that reads the words e was recorded with.
func (t *Table) Of(e history.Event) Kind {
	if k, ok := t.kinds[e.Kind]; ok {
		return k
	}
	var d Detail
	_ = json.Unmarshal([]byte(e.Detail), &d)
	k := Kind{Name: e.Kind, Plugin: d.Plugin, Unknown: true, Words: func(history.Event, func(string) string) (string, string) {
		return e.Kind, "neutral"
	}}
	if d.Sentence != "" {
		k.Words = recordedWords
	}
	return k
}

// AlertPlugins is the installed plugins that are alert kinds, in the order
// their manifests were found.
func (t *Table) AlertPlugins() []AlertPlugin { return t.plugins }

// Record is the event history keeps of a, taken from the spool under the name
// spool, and its kind; false when a is of no kind root may announce. A
// plugin's kind the table lacks is looked for in that plugin's registry in
// pluginsDir, read again: an upgrade's new event may come before the rescan
// its package asks for.
func (t *Table) Record(a Announcement, spool, pluginsDir string) (history.Event, Kind, bool) {
	k, ok := t.kinds[a.Kind]
	if !ok {
		k, ok = declaredOnDisk(pluginsDir, a.Kind)
	}
	if !ok || !k.Announced {
		return history.Event{}, Kind{}, false
	}
	d := Detail{Spool: spool, Args: a.Args}
	if k.declared != nil {
		d.Plugin, d.Sentence, d.Tone, d.Read = k.Plugin, k.declared.Sentence, k.declared.Tone, k.declared.Read
	}
	detail, err := json.Marshal(d)
	if err != nil {
		return history.Event{}, Kind{}, false
	}
	return history.Event{Time: time.Unix(0, a.Time), Kind: a.Kind, Subject: a.Subject, Detail: string(detail)}, k, true
}

// declaredOnDisk is kind as its plugin's registry in pluginsDir declares it.
func declaredOnDisk(pluginsDir, kind string) (Kind, bool) {
	owner, name, ok := strings.Cut(kind, ".")
	if pluginsDir == "" || !ok || !ownerName.MatchString(owner) || slices.Contains(plugin.CoreNames, owner) {
		return Kind{}, false
	}
	data, err := os.ReadFile(filepath.Join(pluginsDir, owner, "commands.json"))
	if err != nil {
		return Kind{}, false
	}
	registry, _, err := plugin.ParseRegistry(data)
	if err != nil {
		return Kind{}, false
	}
	e, ok := registry.Event(name)
	if !ok {
		return Kind{}, false
	}
	return pluginKind(owner, e), true
}

// pluginKind is one declared event of plugin's as a kind of its own.
func pluginKind(id string, e plugin.EventDeclaration) Kind {
	k := Kind{
		Name: id + "." + e.Name, Plugin: id, Words: recordedWords,
		Grant: &Grant{"verso-plugin", id, e.Read}, Announced: true, Said: e.Notify, declared: &e,
	}
	if e.Alert {
		k.Alert = id
	}
	for _, end := range e.Ends {
		k.Ends = append(k.Ends, id+"."+end)
	}
	return k
}

// recordedWords reads a plugin's event by the sentence and tone it was
// recorded with, translated, its arguments filled in.
func recordedWords(e history.Event, tr func(string) string) (string, string) {
	var d Detail
	_ = json.Unmarshal([]byte(e.Detail), &d)
	tone := d.Tone
	if tone == "" {
		tone = "neutral"
	}
	return Fill(d.Sentence, d.Args, tr), tone
}

// core is the router's own kinds.
func core() []Kind {
	fixed := func(what, tone string) Words {
		return func(_ history.Event, tr func(string) string) (string, string) { return tr(what), tone }
	}
	return []Kind{
		{Name: "wan.down", Grant: &InterfaceGrant, Alert: "wan", Words: fixed("Internet went down", "danger")},
		{Name: "wan.up", Grant: &InterfaceGrant, Alert: "wan", Ends: []string{"wan.down"},
			Words: func(e history.Event, tr func(string) string) (string, string) {
				var d struct{ After int64 }
				_ = json.Unmarshal([]byte(e.Detail), &d)
				return fmt.Sprintf(tr("Internet came back after %s"), units.Duration(d.After)), "success"
			}},
		{Name: "boot", Grant: &SystemGrant, Alert: "boot",
			Words: func(e history.Event, tr func(string) string) (string, string) {
				var d struct{ Clean bool }
				_ = json.Unmarshal([]byte(e.Detail), &d)
				if !d.Clean {
					return tr("Router started after losing power or crashing"), "warning"
				}
				return tr("Router started"), "neutral"
			}},
		{Name: "storage.unavailable", Alert: "storage",
			Words: func(e history.Event, tr func(string) string) (string, string) {
				var d struct{ Cause string }
				_ = json.Unmarshal([]byte(e.Detail), &d)
				return fmt.Sprintf(tr("History could not be saved: %s"), d.Cause), "warning"
			}},
		{Name: "storage.available", Alert: "storage", Words: fixed("History is saved again", "success")},
		{Name: "changes.applied", ByChange: true,
			Words: func(e history.Event, tr func(string) string) (string, string) {
				return fmt.Sprintf(tr("Changes applied: %s"), ChangedPages(e.Detail, tr)), "neutral"
			}},
		{Name: "changes.rolledback", ByChange: true, Alert: "rollback",
			Words: func(e history.Event, tr func(string) string) (string, string) {
				return fmt.Sprintf(tr("Changes rolled back: %s. The router could not be reached to confirm them."), ChangedPages(e.Detail, tr)), "warning"
			}},
		{Name: CertificateReplaced, Grant: &AccessCredentialsGrant, Announced: true,
			Words: func(e history.Event, tr func(string) string) (string, string) {
				var d Detail
				_ = json.Unmarshal([]byte(e.Detail), &d)
				return Fill(certificateSentence, d.Args, tr), "info"
			}},
	}
}

// ChangedPages names the pages an apply's changes were made on, in the
// reader's language.
func ChangedPages(detail string, tr func(string) string) string {
	var d struct{ Pages []string }
	_ = json.Unmarshal([]byte(detail), &d)
	names := make([]string, len(d.Pages))
	for i, page := range d.Pages {
		names[i] = tr(page)
	}
	return strings.Join(names, ", ")
}

// Fill is sentence in the reader's words with args in place of its %s, each
// as written: translated where the translation keeps one %s for each
// argument, else the English filled in, else the English as written. The
// arguments are never translated, and a % among them is text.
func Fill(sentence string, args []string, tr func(string) string) string {
	if len(args) == 0 {
		return tr(sentence)
	}
	fill := func(s string) (string, bool) {
		if strings.Count(s, "%s") != len(args) || strings.Count(s, "%") != len(args) {
			return s, false
		}
		values := make([]any, len(args))
		for i, a := range args {
			values[i] = a
		}
		return fmt.Sprintf(s, values...), true
	}
	if filled, ok := fill(tr(sentence)); ok {
		return filled
	}
	if filled, ok := fill(sentence); ok {
		return filled
	}
	return sentence
}
