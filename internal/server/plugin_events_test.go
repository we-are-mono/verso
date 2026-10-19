// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/events"
	"github.com/we-are-mono/verso/internal/history"
	"github.com/we-are-mono/verso/internal/plugin"
	"github.com/we-are-mono/verso/internal/widget"
)

// acmeEvents is acme declaring its renewal, said, under its state read.
func acmeEvents() plugin.Manifest {
	m := demoManifest()
	m.ID, m.Name = "acme", "Trusted certificate"
	m.Commands = &plugin.Registry{
		Commands: []plugin.RegisteredCommand{{Name: "state", Kind: "read", Version: 1}},
		Events: []plugin.EventDeclaration{
			{Name: "renewed", Sentence: "The certificate for %s was renewed.", Tone: "success", Read: "state", Notify: true},
		},
	}
	return m
}

// coreRow is the events page's row of one of the router's own events.
func coreRow(e history.Event, now time.Time, tr func(string) string) widget.TableRow {
	return eventRow(events.NewTable(nil).Of(e), e, now, tr)
}

// renewedDetail is an acme renewal as the spool taker records it.
const renewedDetail = `{"spool":"s","plugin":"acme","sentence":"The certificate for %s was renewed.","args":["router.example.com"],"tone":"success","read":"state"}`

// eventsPageWith is the events page, as a session the backend grants as it
// says, of a router holding events, with manifests installed.
func eventsPageWith(t *testing.T, backend fakeBackend, manifests []plugin.Manifest, recorded ...history.Event) string {
	t.Helper()
	s := newServerWith(t, backend, &fakeTransport{}, manifests)
	store, err := history.Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	s.historyStore = store
	for _, e := range recorded {
		if err := store.Record(t.Context(), e); err != nil {
			t.Fatal(err)
		}
	}
	status, body := getSignedIn(t, s, eventsPath)
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	return body
}

// TestAPluginEventReadsThroughItsCatalog: a plugin's event is said in the
// plugin's words, translated through its catalog, in the tone it declared.
func TestAPluginEventReadsThroughItsCatalog(t *testing.T) {
	table := events.NewTable([]plugin.Manifest{acmeEvents()})
	e := history.Event{Time: time.Now(), Kind: "acme.renewed", Detail: renewedDetail}
	k := table.Of(e)
	shell := func(s string) string { return "shell: " + s }
	sl := func(s string) string {
		if s == "The certificate for %s was renewed." {
			return "Potrdilo za %s je obnovljeno."
		}
		return s
	}
	tr := kindTranslator(k, shell, func(id string) func(string) string {
		if id == "acme" {
			return sl
		}
		return shell
	})
	row := eventRow(k, e, time.Now(), tr)
	if got := row.Cells[2]; got.Text != "Potrdilo za router.example.com je obnovljeno." || got.Variant != "success" {
		t.Errorf("event = %+v", got)
	}
	core := events.NewTable(nil).Of(history.Event{Kind: "wan.down"})
	if kindTranslator(core, shell, nil)("x") != "shell: x" {
		t.Error("a core event reads in the shell's words")
	}
}

// TestAPluginsEventShowsUnderItsReadsGrant: the events page lists a plugin's
// event only to a session rpcd grants the read it declares.
func TestAPluginsEventShowsUnderItsReadsGrant(t *testing.T) {
	renewed := history.Event{Time: time.Now().Add(-time.Hour), Kind: "acme.renewed", Detail: renewedDetail}
	body := eventsPageWith(t, fakeBackend{access: true}, []plugin.Manifest{acmeEvents()}, renewed)
	if !strings.Contains(body, "The certificate for router.example.com was renewed.") {
		t.Errorf("a granted session does not see acme's renewal:\n%s", body)
	}
	body = eventsPageWith(t, fakeBackend{grant: grantAllBut("verso-plugin acme state")}, []plugin.Manifest{acmeEvents()}, renewed)
	if strings.Contains(body, "router.example.com") {
		t.Errorf("a session without acme's state read sees its renewal:\n%s", body)
	}
}

// TestARemovedPluginsEventShowsToWholeHistoryReaders: once its plugin is gone
// an event keeps its row, in English, for a session that may read the whole
// of config history, as an event of an unknown kind does.
func TestARemovedPluginsEventShowsToWholeHistoryReaders(t *testing.T) {
	s, _, _ := historyOfTwoConfigs(t)
	renewed := history.Event{Time: time.Now().Add(-time.Hour), Kind: "acme.renewed", Detail: renewedDetail}
	if err := s.historyStore.Record(t.Context(), renewed); err != nil {
		t.Fatal(err)
	}
	as(s, nil, nil)
	if _, body := getSignedIn(t, s, eventsPath); !strings.Contains(body, "The certificate for router.example.com was renewed.") {
		t.Errorf("a whole-history reader does not see the removed plugin's event:\n%s", body)
	}
	as(s, func(scope, object, function string) bool { return scope != "uci" || object != "firewall" }, nil)
	if _, body := getSignedIn(t, s, eventsPath); strings.Contains(body, "router.example.com") {
		t.Errorf("a session that may not read all of config history sees the removed plugin's event:\n%s", body)
	}
}

// TestTheCertificatesReplacementShowsUnderItsFacts: the web certificate's
// replacement shows only to a session that may read the certificate's facts.
func TestTheCertificatesReplacementShowsUnderItsFacts(t *testing.T) {
	replaced := history.Event{Time: time.Now().Add(-time.Hour), Kind: events.CertificateReplaced,
		Detail: `{"spool":"s","args":["router.example.com","2027-01-17"]}`}
	if body := eventsPageWith(t, fakeBackend{access: true}, nil, replaced); !strings.Contains(body, "certificate was replaced, for router.example.com until 2027-01-17.") {
		t.Errorf("the replacement is not listed:\n%s", body)
	}
	if body := eventsPageWith(t, fakeBackend{grant: grantAllBut("ubus verso accessCredentials")}, nil, replaced); strings.Contains(body, "2027-01-17") {
		t.Errorf("a session without the certificate's facts sees its replacement:\n%s", body)
	}
}

// TestARescanReadsThePluginsEventsAgain: a plugin installed while the shell
// runs brings its events with it, as it brings its pages.
func TestARescanReadsThePluginsEventsAgain(t *testing.T) {
	s := newServer(t, fakeBackend{access: true})
	if _, ok := s.eventKinds().Kind("acme.renewed"); ok {
		t.Fatal("an event no installed plugin declares is a kind")
	}
	s.SetRescan(func() []plugin.Manifest { return []plugin.Manifest{acmeEvents()} })
	s.Rescan()
	if k, ok := s.eventKinds().Kind("acme.renewed"); !ok || k.Plugin != "acme" {
		t.Errorf("after a rescan, acme.renewed = %+v, %v", k, ok)
	}
}

// TestEveryCoreEventGrantIsAShellProbe: the router's own kinds name grants the
// shell asks with its own privilege, so each is one of shellProbes, which root
// is held to.
func TestEveryCoreEventGrantIsAShellProbe(t *testing.T) {
	for _, name := range []string{"wan.down", "wan.up", "boot", "changes.applied", "changes.rolledback",
		"storage.unavailable", "storage.available", events.CertificateReplaced} {
		k, ok := events.NewTable(nil).Kind(name)
		if !ok {
			t.Fatalf("%s is not a kind", name)
		}
		probe, known := kindProbe(k)
		if !known {
			t.Errorf("%s: its grant %v is no probe the shell asks", name, k.Grant)
			continue
		}
		if probe != nil && !slices.Contains(shellProbes, *probe) {
			t.Errorf("%s: probe %v is not in shellProbes", name, *probe)
		}
	}
}
