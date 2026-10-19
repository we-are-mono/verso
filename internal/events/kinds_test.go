// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package events

import (
	"slices"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/history"
	"github.com/we-are-mono/verso/internal/plugin"
	"github.com/we-are-mono/verso/internal/units"
)

func identity(s string) string { return s }

// acme is a plugin declaring a renewal that ends a failure, and the failure,
// which alerts.
func acme() plugin.Manifest {
	return plugin.Manifest{ID: "acme", Name: "Trusted certificate", Commands: &plugin.Registry{
		Commands: []plugin.RegisteredCommand{{Name: "state", Kind: "read", Version: 1}},
		Events: []plugin.EventDeclaration{
			{Name: "renewed", Sentence: "The certificate for %s was renewed.", Tone: "success", Read: "state", Notify: true, Ends: []string{"renew-failed"}},
			{Name: "renew-failed", Sentence: "The certificate for %s could not be renewed.", Tone: "danger", Read: "state", Notify: true, Alert: true},
			{Name: "quiet", Sentence: "Nothing to say.", Tone: "info", Read: "state"},
		},
	}}
}

// TestCoreKinds: each of the router's own kinds carries its grant, whether it
// may come from the spool, whether it is said, its alert kind and what it
// ends (ADR-028), so every reader of events reads them alike.
func TestCoreKinds(t *testing.T) {
	table := NewTable(nil)
	for _, want := range []struct {
		name      string
		grant     *Grant
		byChange  bool
		announced bool
		alert     string
		ends      []string
	}{
		{"wan.down", &InterfaceGrant, false, false, "wan", nil},
		{"wan.up", &InterfaceGrant, false, false, "wan", []string{"wan.down"}},
		{"boot", &SystemGrant, false, false, "boot", nil},
		{"changes.applied", nil, true, false, "", nil},
		{"changes.rolledback", nil, true, false, "rollback", nil},
		{"storage.unavailable", nil, false, false, "storage", nil},
		{"storage.available", nil, false, false, "storage", nil},
		{"certificate.replaced", &AccessCredentialsGrant, false, true, "", nil},
	} {
		k, ok := table.Kind(want.name)
		if !ok {
			t.Errorf("%s: not in the table", want.name)
			continue
		}
		if k.Plugin != "" || k.Said || k.Unknown {
			t.Errorf("%s: plugin %q, said %v, unknown %v; core kinds are core's and unsaid", want.name, k.Plugin, k.Said, k.Unknown)
		}
		if (k.Grant == nil) != (want.grant == nil) || (k.Grant != nil && *k.Grant != *want.grant) {
			t.Errorf("%s: grant %v, want %v", want.name, k.Grant, want.grant)
		}
		if k.ByChange != want.byChange || k.Announced != want.announced || k.Alert != want.alert || !slices.Equal(k.Ends, want.ends) {
			t.Errorf("%s: byChange %v announced %v alert %q ends %v", want.name, k.ByChange, k.Announced, k.Alert, k.Ends)
		}
	}
	if _, ok := table.Kind("wan.sideways"); ok {
		t.Error("a kind nobody records is not in the table")
	}
}

// TestCoreWords: the router's own events read as the Events page has always
// read them.
func TestCoreWords(t *testing.T) {
	table := NewTable(nil)
	for _, c := range []struct {
		event      history.Event
		what, tone string
	}{
		{history.Event{Kind: "wan.down"}, "Internet went down", "danger"},
		{history.Event{Kind: "wan.up", Detail: `{"after":480}`}, "Internet came back after " + units.Duration(480), "success"},
		{history.Event{Kind: "boot", Detail: `{"clean":true,"id":"x"}`}, "Router started", "neutral"},
		{history.Event{Kind: "boot", Detail: `{"clean":false,"id":"x"}`}, "Router started after losing power or crashing", "warning"},
		{history.Event{Kind: "storage.unavailable", Detail: `{"cause":"disk full"}`}, "History could not be saved: disk full", "warning"},
		{history.Event{Kind: "storage.available"}, "History is saved again", "success"},
		{history.Event{Kind: "changes.applied", Detail: `{"pages":["Firewall","DHCP"],"change":3}`}, "Changes applied: Firewall, DHCP", "neutral"},
		{history.Event{Kind: "changes.rolledback", Detail: `{"pages":["Firewall"],"change":4}`},
			"Changes rolled back: Firewall. The router could not be reached to confirm them.", "warning"},
		{history.Event{Kind: "certificate.replaced", Detail: `{"spool":"s","args":["router.example.com","2027-01-17"]}`},
			"The web interface’s certificate was replaced, for router.example.com until 2027-01-17.", "info"},
	} {
		what, tone := table.Of(c.event).Words(c.event, identity)
		if what != c.what || tone != c.tone {
			t.Errorf("%s: %q in %s, want %q in %s", c.event.Kind, what, tone, c.what, c.tone)
		}
	}
	translated := func(s string) string { return "«" + s + "»" }
	applied := history.Event{Kind: "changes.applied", Detail: `{"pages":["Firewall"]}`}
	if what, _ := table.Of(applied).Words(applied, translated); what != "«Changes applied: «Firewall»»" {
		t.Errorf("an apply's words and its pages are the reader's: %q", what)
	}
}

// TestPluginKindsJoinTheTable: a plugin's declared events are kinds of its
// own, under its read's grant, said as declared, alerting under its id, and
// ending its own events.
func TestPluginKindsJoinTheTable(t *testing.T) {
	table := NewTable([]plugin.Manifest{acme()})
	renewed, ok := table.Kind("acme.renewed")
	if !ok || renewed.Plugin != "acme" || !renewed.Said || !renewed.Announced || renewed.Alert != "" || renewed.Unknown {
		t.Fatalf("acme.renewed = %+v, %v", renewed, ok)
	}
	if renewed.Grant == nil || *renewed.Grant != (Grant{"verso-plugin", "acme", "state"}) {
		t.Errorf("grant = %v, want verso-plugin acme state", renewed.Grant)
	}
	if !slices.Equal(renewed.Ends, []string{"acme.renew-failed"}) {
		t.Errorf("ends = %v", renewed.Ends)
	}
	if failed, _ := table.Kind("acme.renew-failed"); failed.Alert != "acme" {
		t.Errorf("an alerting event alerts under its plugin's id: %q", failed.Alert)
	}
	if quiet, _ := table.Kind("acme.quiet"); quiet.Said {
		t.Error("an event declared without notify is not said")
	}
	if got := table.AlertPlugins(); len(got) != 1 || got[0].ID != "acme" || got[0].Name != "Trusted certificate" {
		t.Errorf("alert plugins = %+v", got)
	}
	if NewTable([]plugin.Manifest{{ID: "ddns"}}).AlertPlugins() != nil {
		t.Error("a plugin with no alerting event is no alert kind")
	}
}

// TestPluginWords: a plugin's event reads its recorded sentence through the
// plugin's catalog, its arguments filled in as written, in its tone; with no
// translation, in English.
func TestPluginWords(t *testing.T) {
	table := NewTable([]plugin.Manifest{acme()})
	e := history.Event{Time: time.Unix(1, 0), Kind: "acme.renewed",
		Detail: `{"spool":"s","plugin":"acme","sentence":"The certificate for %s was renewed.","args":["router.example.com"],"tone":"success","read":"state"}`}
	sl := map[string]string{"The certificate for %s was renewed.": "Potrdilo za %s je obnovljeno."}
	what, tone := table.Of(e).Words(e, func(s string) string {
		if t, ok := sl[s]; ok {
			return t
		}
		return s
	})
	if what != "Potrdilo za router.example.com je obnovljeno." || tone != "success" {
		t.Errorf("%q in %s", what, tone)
	}
	if what, _ := table.Of(e).Words(e, identity); what != "The certificate for router.example.com was renewed." {
		t.Errorf("untranslated: %q", what)
	}
}

// TestARemovedPluginsEventKeepsItsWords: once its plugin is gone, an event
// still reads its English sentence, filled in, and is of no known kind, so
// only a reader of the whole history sees it.
func TestARemovedPluginsEventKeepsItsWords(t *testing.T) {
	table := NewTable(nil)
	e := history.Event{Kind: "acme.renewed",
		Detail: `{"spool":"s","plugin":"acme","sentence":"The certificate for %s was renewed.","args":["router.example.com"],"tone":"success","read":"state"}`}
	k := table.Of(e)
	if !k.Unknown || k.Plugin != "acme" {
		t.Errorf("kind = %+v, want unknown, acme's", k)
	}
	if what, tone := k.Words(e, identity); what != "The certificate for router.example.com was renewed." || tone != "success" {
		t.Errorf("%q in %s", what, tone)
	}
	odd := history.Event{Kind: "odd.thing"}
	if what, tone := table.Of(odd).Words(odd, identity); what != "odd.thing" || tone != "neutral" {
		t.Errorf("an event with no words reads its kind: %q in %s", what, tone)
	}
}

// TestFill: a sentence takes its arguments only where its count of %s is
// theirs; a translation that lost one falls back to the English, filled; a
// sentence that cannot be filled reads as written.
func TestFill(t *testing.T) {
	broken := func(string) string { return "Potrdilo je obnovljeno." }
	for _, c := range []struct {
		sentence string
		args     []string
		tr       func(string) string
		want     string
	}{
		{"No arguments.", nil, identity, "No arguments."},
		{"For %s until %s.", []string{"a", "b"}, identity, "For a until b."},
		{"For %s.", []string{"100%"}, identity, "For 100%."},
		{"The certificate for %s was renewed.", []string{"x"}, broken, "The certificate for x was renewed."},
		{"For %s and %s.", []string{"one"}, identity, "For %s and %s."},
		{"For %d.", []string{"one"}, identity, "For %d."},
	} {
		if got := Fill(c.sentence, c.args, c.tr); got != c.want {
			t.Errorf("Fill(%q, %v) = %q, want %q", c.sentence, c.args, got, c.want)
		}
	}
}
