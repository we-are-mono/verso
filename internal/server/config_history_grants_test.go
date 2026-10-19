// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/history"
)

// firewallReader is an rpcd login that may read the firewall config and
// nothing else: no other config, no write, no ubus read.
func firewallReader(scope, object, function string) bool {
	return scope == "uci" && object == "firewall" && function == "read"
}

// reservedHost is an apply to the dhcp config: a reservation's address moved,
// by root from 10.9.8.7.
func reservedHost(at time.Time) history.Change {
	return history.Change{Time: at, Status: history.ChangeApplied, User: "root", Address: "10.9.8.7", OldRead: true, Items: []history.ChangeItem{{
		Page: "DHCP", Plugin: "dnsdhcp", Verb: "changed", SubjectKind: "dhcp", SubjectName: "labhost",
		Ops: []history.ChangeOp{{Config: "dhcp", Section: "labhost", Op: "set", Option: "ip", Value: text("192.168.77.88"), Old: json.RawMessage(`"192.168.77.11"`)}},
	}}}
}

// historyOfTwoConfigs is config history holding a firewall entry, a dhcp
// entry, one touching both, a starting point for firewall and wireless, and
// events for each kind of reading.
func historyOfTwoConfigs(t *testing.T) (*Server, *[]uciWrite, map[string]int64) {
	t.Helper()
	s, store, writes, _ := actsServer(t, nil)
	ctx := t.Context()
	now := time.Now()
	ids := map[string]int64{}
	firewall := turnedOff(now.Add(-3 * time.Minute))
	firewall.OldRead = true
	ids["firewall"] = record(t, store, firewall)
	ids["dhcp"] = record(t, store, reservedHost(now.Add(-2*time.Minute)))
	both := turnedOff(now.Add(-time.Minute))
	both.OldRead = true
	both.Items[0].Ops[0].Value, both.Items[0].Ops[0].Old = text("REJECT"), json.RawMessage(`"DROP"`)
	both.Items = append(both.Items, reservedHost(now).Items...)
	both.Items[1].Ops[0].Option, both.Items[1].Ops[0].Value, both.Items[1].Ops[0].Old = "name", text("lab-printer"), json.RawMessage(`"old-printer"`)
	ids["both"] = record(t, store, both)

	_ = store.StartConfig(ctx, "firewall", json.RawMessage(`{"cfg0792bd":{".type":"rule","name":"Allow-Ping","target":"ACCEPT"}}`), now.Add(-time.Hour))
	_ = store.SetConfigState(ctx, "firewall", json.RawMessage(`{"cfg0792bd":{".type":"rule","name":"Allow-Ping","target":"REJECT"}}`), now)
	_ = store.StartConfig(ctx, "wireless", json.RawMessage(`{"default_radio0":{".type":"wifi-iface","ssid":"LabSSID"}}`), now.Add(-time.Hour))
	_ = store.SetConfigState(ctx, "wireless", json.RawMessage(`{"default_radio0":{".type":"wifi-iface","ssid":"CurrentSSID"}}`), now)

	for _, e := range []history.Event{
		{Time: now.Add(-50 * time.Minute), Kind: "boot", Detail: `{"clean":true,"id":"b1"}`},
		{Time: now.Add(-40 * time.Minute), Kind: "wan.down", Subject: "eth-wan9"},
		{Time: now.Add(-2 * time.Minute), Kind: "changes.applied", Detail: fmt.Sprintf(`{"pages":["DHCP"],"change":%d}`, ids["dhcp"])},
		{Time: now.Add(-time.Minute), Kind: "changes.applied", Detail: fmt.Sprintf(`{"pages":["Firewall","DHCP"],"change":%d}`, ids["both"])},
		{Time: now.Add(-30 * time.Second), Kind: "changes.rolledback", Detail: `{"pages":["Wireless"]}`},
	} {
		if err := store.Record(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	return s, writes, ids
}

// as makes the server's rpcd answer probes as grant does.
func as(s *Server, grant func(scope, object, function string) bool, probes *[]string) {
	b := s.backend.(fakeBackend)
	b.grant, b.probes = grant, probes
	s.backend = b
}

// TestConfigHistoryShowsOnlyWhatTheSessionMayRead: history.db is the shell's
// own store, so each session reads of it only the configs rpcd lets it read
// (ADR-007): an entry wholly in another config is not there for it, its who
// and where with it; a mixed entry keeps only its readable rows and offers
// no act; the starting point holds only readable configs; and an event shows
// only under the grant its reading needs.
func TestConfigHistoryShowsOnlyWhatTheSessionMayRead(t *testing.T) {
	s, writes, ids := historyOfTwoConfigs(t)
	as(s, firewallReader, nil)
	hidden := []string{"192.168.77.11", "192.168.77.88", "10.9.8.7", "labhost", "lab-printer", "old-printer", "LabSSID", "CurrentSSID"}
	leaks := func(where, body string) {
		t.Helper()
		for _, value := range hidden {
			if strings.Contains(body, value) {
				t.Errorf("%s shows %q to a session that may not read its config", where, value)
			}
		}
	}

	_, page := getSignedIn(t, s, changesPath)
	leaks("the listing", page)
	for _, want := range []string{">Allow-Ping</span>", ">REJECT<", ">Starting point<", "10.0.0.12"} {
		if !strings.Contains(page, want) {
			t.Errorf("the listing misses %q, which the session may read", want)
		}
	}
	if strings.Contains(page, fmt.Sprintf("%s/%d", changesPath, ids["dhcp"])) {
		t.Error("an entry wholly in an unreadable config is listed")
	}

	rec := getPanel(t, s, fmt.Sprintf("%s/%d", changesPath, ids["dhcp"]))
	leaks("an unreadable entry's drawer", rec.Body.String())
	if rec.Code != http.StatusNotFound {
		t.Errorf("an unreadable entry's drawer = %d, want 404", rec.Code)
	}
	mixed := getPanel(t, s, fmt.Sprintf("%s/%d", changesPath, ids["both"])).Body.String()
	leaks("a mixed entry's drawer", mixed)
	if !strings.Contains(mixed, firewallTargetLine) {
		t.Error("a mixed entry's drawer misses its readable row")
	}
	if !strings.Contains(mixed, "Part of this is in settings this account may not read") || strings.Contains(mixed, `value="undo"`) {
		t.Error("a mixed entry's drawer offers an undo the session cannot see whole")
	}
	since := getPanel(t, s, fmt.Sprintf("%s/%d?tab=since", changesPath, ids["firewall"])).Body.String()
	leaks("since then", since)
	if strings.Contains(since, `value="return"`) {
		t.Error("a return across an unreadable entry is offered")
	}
	start := getPanel(t, s, startPath).Body.String()
	leaks("the starting point", start)
	if !strings.Contains(start, firewallTargetLine) || strings.Contains(start, `value="return"`) {
		t.Error("the starting point shows its readable config and offers no return across an unreadable one")
	}

	for _, post := range []struct{ path, act string }{
		{fmt.Sprintf("%s/%d", changesPath, ids["both"]), "undo"},
		{fmt.Sprintf("%s/%d", changesPath, ids["firewall"]), "return"},
		{startPath, "return"},
	} {
		rec := postPlugin(t, s, post.path, url.Values{"act": {post.act}})
		leaks(post.act+" refused", rec.Body.String())
		if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "Part of this is in settings this account may not read") {
			t.Errorf("%s %s = %d, want a refusal naming why", post.act, post.path, rec.Code)
		}
	}
	if len(*writes) != 0 {
		t.Errorf("a refused act staged %+v", *writes)
	}

	_, events := getSignedIn(t, s, eventsPath)
	for _, never := range []string{"eth-wan9", "Internet went down", "Router started", "Changes applied: DHCP", "Firewall, DHCP", "Changes rolled back: Wireless"} {
		if strings.Contains(events, never) {
			t.Errorf("the events page shows %q without the grant its reading needs", never)
		}
	}
	if !strings.Contains(events, "Changes applied: Firewall") {
		t.Error("the events page misses an apply whose entry the session may read")
	}
}

// TestConfigHistoryActsNeedTheWriteGrant: a session that may read an entry
// but not write its config is refused the act before anything is staged.
func TestConfigHistoryActsNeedTheWriteGrant(t *testing.T) {
	s, writes, ids := historyOfTwoConfigs(t)
	as(s, firewallReader, nil)
	rec := postPlugin(t, s, fmt.Sprintf("%s/%d", changesPath, ids["firewall"]), url.Values{"act": {"undo"}})
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "This account may not change these settings, so nothing was staged.") {
		t.Errorf("undo without the write grant = %d", rec.Code)
	}
	if len(*writes) != 0 {
		t.Errorf("a refused undo staged %+v", *writes)
	}
	as(s, func(scope, object, function string) bool {
		return scope == "uci" && object == "firewall"
	}, nil)
	if rec := postPlugin(t, s, fmt.Sprintf("%s/%d", changesPath, ids["firewall"]), url.Values{"act": {"undo"}}); rec.Code != http.StatusSeeOther {
		t.Errorf("undo with the grants = %d: %s", rec.Code, rec.Body)
	}
}

// TestStartingPointNoneOfWhichIsReadableIsNotThere: the listing lists the
// starting point only where the session reads one of its configs, and its
// drawer answers alike, as one no longer kept, its time with it.
func TestStartingPointNoneOfWhichIsReadableIsNotThere(t *testing.T) {
	s, _, _ := historyOfTwoConfigs(t)
	as(s, func(scope, object, function string) bool {
		return scope == "uci" && object == "dhcp" && function == "read"
	}, nil)
	if _, page := getSignedIn(t, s, changesPath); strings.Contains(page, ">Starting point<") {
		t.Fatal("a starting point in no readable config is listed")
	}
	rec := getPanel(t, s, startPath)
	if rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), "Starting point ·") {
		t.Errorf("its drawer = %d: %s, want 404 without its time", rec.Code, rec.Body)
	}
}

// TestConfigHistoryUncheckedSaysSo: where rpcd does not answer the grants,
// history is withheld as from a refusal, but the listings say the permissions
// could not be checked rather than that nothing happened, and a drawer says
// so rather than answering as one no longer kept.
func TestConfigHistoryUncheckedSaysSo(t *testing.T) {
	s, _, ids := historyOfTwoConfigs(t)
	b := s.backend.(fakeBackend)
	b.access, b.accessErr = true, errors.New("rpcd gone")
	s.backend = b
	for _, path := range []string{changesPath, eventsPath} {
		_, page := getSignedIn(t, s, path)
		if !strings.Contains(page, "couldn’t be checked") {
			t.Errorf("%s does not say the permissions could not be checked", path)
		}
		for _, wrong := range []string{"Nothing has been applied yet.", "Nothing has happened yet.", "10.9.8.7", "eth-wan9"} {
			if strings.Contains(page, wrong) {
				t.Errorf("%s with rpcd unanswered shows %q", path, wrong)
			}
		}
	}
	for _, path := range []string{fmt.Sprintf("%s/%d", changesPath, ids["firewall"]), startPath} {
		if rec := getPanel(t, s, path); rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "couldn’t be checked") {
			t.Errorf("%s with rpcd unanswered = %d: %s", path, rec.Code, rec.Body)
		}
	}
}

// TestConfigHistoryRootSeesItAll: root's '*' reads every config, verso-admin's
// uci '*' among its groups (TestRootIsGrantedEveryShellProbe), so history
// reads as it always has — every entry with who and where, the starting point
// whole, every event — each config asked of rpcd once a page.
func TestConfigHistoryRootSeesItAll(t *testing.T) {
	s, _, ids := historyOfTwoConfigs(t)
	var probes []string
	as(s, nil, &probes)
	_, page := getSignedIn(t, s, changesPath)
	for _, want := range []string{"10.9.8.7", ">labhost</span>", "192.168.77.88", ">Allow-Ping</span>", ">Starting point<",
		fmt.Sprintf("%s/%d", changesPath, ids["dhcp"])} {
		if !strings.Contains(page, want) {
			t.Errorf("root's listing misses %q", want)
		}
	}
	for _, config := range []string{"uci firewall read", "uci dhcp read"} {
		if n := strings.Count(strings.Join(probes, "\n"), config); n != 1 {
			t.Errorf("%s asked %d times for one page, want once: %q", config, n, probes)
		}
	}
	mixed := getPanel(t, s, fmt.Sprintf("%s/%d", changesPath, ids["both"])).Body.String()
	for _, want := range []string{firewallTargetLine, chipsLine("dhcp.labhost", "name"), ">old-printer<", ">lab-printer<", `value="undo"`} {
		if !strings.Contains(mixed, want) {
			t.Errorf("root's drawer misses %q", want)
		}
	}
	if strings.Contains(mixed, "may not read") {
		t.Error("root is told it may not read")
	}
	start := getPanel(t, s, startPath).Body.String()
	for _, want := range []string{">LabSSID<", ">CurrentSSID<", `value="return"`} {
		if !strings.Contains(start, want) {
			t.Errorf("root's starting point misses %q", want)
		}
	}
	_, events := getSignedIn(t, s, eventsPath)
	for _, want := range []string{"eth-wan9", "Internet went down", "Router started", "Changes applied: DHCP", "Changes applied: Firewall, DHCP", "Changes rolled back: Wireless"} {
		if !strings.Contains(events, want) {
			t.Errorf("root's events miss %q", want)
		}
	}
}
