// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/we-are-mono/verso/internal/alerts"
	"github.com/we-are-mono/verso/internal/history"
	"github.com/we-are-mono/verso/internal/plugin"
)

// The secrets config history keeps no value of (ADR-021) reach it through
// copies as well as through the option changed: a section removed whole, the
// raw line an undescribed row is kept as. Neither copy keeps the value.

const wifiKey = "PSK-NEVER-KEPT-4711"

// wifiServer is a signed-in server whose plugin writes wireless, with config
// history in a history.db at path, what the shell stages recorded, and the
// rollback window's timer in the test's hand.
func wifiServer(t *testing.T, backend fakeBackend) (*Server, *history.Store, string, *[]uciWrite, *func()) {
	t.Helper()
	m := demoACLManifest()
	m.ACL.Write = append(m.ACL.Write, plugin.ACLScope{Scope: "uci", Object: "wireless", Function: "write"})
	writes, confirms := []uciWrite{}, 0
	backend.writes, backend.confirms = &writes, &confirms
	s := newServerWith(t, backend, &fakeTransport{}, []plugin.Manifest{m})
	path := filepath.Join(t.TempDir(), "history.db")
	store, err := history.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	s.historyStore, s.changeLog = store, store
	var window func()
	s.afterRollback = func(_ time.Duration, f func()) { window = f }
	return s, store, path, &writes, &window
}

// heldIn is every row of history.db's config history that holds needle, as
// table: text.
func heldIn(t *testing.T, path, needle string) []string {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var found []string
	for table, columns := range map[string]string{
		"changes":      "note",
		"change_items": "words",
		"change_ops":   "coalesce(value, '') || ' ' || coalesce(old, '')",
		"config_state": "sections",
		"config_start": "sections",
	} {
		rows, err := db.Query(`SELECT ` + columns + ` FROM ` + table)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var text string
			if err := rows.Scan(&text); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(text, needle) {
				found = append(found, table+": "+text)
			}
		}
		_ = rows.Close()
	}
	return found
}

// TestARemovedSectionKeepsNoSecret: a Wi-Fi network removed is kept whole but
// for its key, which history.db holds as not kept and the drawer shows so;
// undoing the removal makes the network again without a key, and says so.
func TestARemovedSectionKeepsNoSecret(t *testing.T) {
	s, store, path, writes, window := wifiServer(t, fakeBackend{
		changes: map[string][][]string{"wireless": {{"remove", "guestnet"}}},
		committed: map[string]map[string]any{"wireless": {
			"guestnet": map[string]any{".type": "wifi-iface", ".name": "guestnet", ".anonymous": false,
				"ssid": "Guest", "encryption": "psk2", "key": wifiKey},
		}},
	})
	applyConfirmed(t, s, window)

	if found := heldIn(t, path, wifiKey); len(found) > 0 {
		t.Fatalf("history.db holds the key:\n%s", strings.Join(found, "\n"))
	}
	all, err := store.Changes(t.Context(), 10)
	if err != nil || len(all) != 1 {
		t.Fatalf("changes = %+v, %v", all, err)
	}
	id := all[0].ID
	var held map[string]any
	if err := json.Unmarshal(all[0].Items[0].Ops[0].Old, &held); err != nil {
		t.Fatal(err)
	}
	if held["ssid"] != "Guest" || !reflect.DeepEqual(held["key"], map[string]any{"secret": true}) {
		t.Fatalf("kept %v, want the section with its key not kept", held)
	}

	body := getPanel(t, s, fmt.Sprintf("%s/%d", changesPath, id)).Body.String()
	if strings.Contains(body, wifiKey) || !strings.Contains(body, "wireless.guestnet.key") || !strings.Contains(body, "not kept") {
		t.Fatalf("drawer:\n%s", body)
	}

	if rec := act(t, s, id, "undo"); rec.Code != http.StatusSeeOther {
		t.Fatalf("undo = %d: %s", rec.Code, rec.Body)
	}
	for _, w := range *writes {
		if _, has := w.values["key"]; has || strings.Contains(fmt.Sprint(w.values), wifiKey) {
			t.Fatalf("undo staged %+v, want the network without its key", *writes)
		}
	}
	if len(*writes) == 0 || (*writes)[len(*writes)-1].values["ssid"] != "Guest" {
		t.Fatalf("undo staged %+v, want the network made again", *writes)
	}
}

// TestAnUndescribedSecretKeepsNoValueInItsWords: a listed secret no name
// rule masks, changed where no plugin describes the row, is kept as its raw
// line with the value masked.
func TestAnUndescribedSecretKeepsNoValueInItsWords(t *testing.T) {
	const radius = "RADIUS-NEVER-KEPT-0042"
	s, store, path, _, window := wifiServer(t, fakeBackend{
		changes: map[string][][]string{"wireless": {{"set", "corp", "auth_secret", radius}}},
		committed: map[string]map[string]any{"wireless": {
			"corp": map[string]any{".type": "wifi-iface", "auth_secret": "RADIUS-BEFORE"},
		}},
	})
	applyConfirmed(t, s, window)

	for _, needle := range []string{radius, "RADIUS-BEFORE"} {
		if found := heldIn(t, path, needle); len(found) > 0 {
			t.Fatalf("history.db holds %s:\n%s", needle, strings.Join(found, "\n"))
		}
	}
	all, _ := store.Changes(t.Context(), 10)
	item := all[0].Items[0]
	if !item.Verbatim || item.Words != "wireless: corp.auth_secret = ••••••••" || !item.Ops[0].Secret {
		t.Fatalf("kept %+v", item)
	}
}

// TestTheReviewMasksEverySecretItKnows: the review drawer's raw line and the
// note it offers mask a listed secret, as well as what a name says is one.
func TestTheReviewMasksEverySecretItKnows(t *testing.T) {
	const radius = "RADIUS-NEVER-SHOWN-0042"
	s, _, _, _, _ := wifiServer(t, fakeBackend{
		changes: map[string][][]string{"wireless": {{"set", "corp", "auth_secret", radius}}},
	})
	view := s.staged(t.Context(), "test-sid", func(s string) string { return s }, func(string) func(string) string { return func(s string) string { return s } })
	shown := []string{view.Summary}
	for _, g := range view.Groups {
		for _, item := range g.Items {
			shown = append(shown, item.Label)
			for _, line := range item.Lines {
				shown = append(shown, line.Text)
			}
		}
	}
	if len(shown) < 3 || strings.Contains(strings.Join(shown, "\n"), radius) {
		t.Fatalf("the review shows %q", shown)
	}
}

// TestTheRawLineMasksListedSecretsAndNamedOnes: a secret the list names is
// masked whatever its name, a list's value too, and a name that says
// password or key is masked even where no list names it.
func TestTheRawLineMasksListedSecretsAndNamedOnes(t *testing.T) {
	secrets := map[string]bool{"wireless.auth_secret": true, "vpn.tls_auth": true}
	cases := []struct {
		config string
		ch     []string
		want   string
	}{
		{"wireless", []string{"set", "corp", "auth_secret", "x"}, "wireless: corp.auth_secret = ••••••••"},
		{"vpn", []string{"list-add", "office", "tls_auth", "x"}, "vpn: office.tls_auth += ••••••••"},
		{"vpn", []string{"list-del", "office", "tls_auth", "x"}, "vpn: office.tls_auth -= ••••••••"},
		{"network", []string{"set", "wan", "password", "x"}, "network: wan.password = ••••••••"},
		{"network", []string{"set", "wan", "username", "x"}, "network: wan.username = x"},
		{"vpn", []string{"remove", "office", "tls_auth"}, "vpn: remove office.tls_auth"},
	}
	for _, c := range cases {
		if got := humanizeChange(c.config, c.ch, secrets); got != c.want {
			t.Errorf("humanizeChange(%v) = %q, want %q", c.ch, got, c.want)
		}
	}
}

// TestUndoOfARemovedSectionLeavesItsSecretsUnset: a secret kept as not kept,
// or as an edit made outside the review kept it, its fingerprint, has no
// value to write back.
func TestUndoOfARemovedSectionLeavesItsSecretsUnset(t *testing.T) {
	plan := reverseOf([]history.Change{entry(1,
		history.ChangeOp{Config: "wireless", Section: "guestnet", Op: "remove-section",
			Old: old(`{".type":"wifi-iface",".name":"guestnet",".anonymous":false,"ssid":"Guest","key":{"secret":true}}`)},
		history.ChangeOp{Config: "network", Section: "wg0", Op: "remove-section",
			Old: old(`{".type":"interface",".name":"wg0",".anonymous":false,"proto":"wireguard","private_key":{"secret":"0f1e2d3c4b5a69788796a5b4c3d2e1f0"}}`)},
	)})
	want := []plugin.CommitOp{
		{Config: "wireless", Section: "guestnet", Type: "wifi-iface", Values: map[string]any{"ssid": "Guest"}},
		{Config: "network", Section: "wg0", Type: "interface", Values: map[string]any{"proto": "wireguard"}},
	}
	if got := stepsOf(plan); !reflect.DeepEqual(got, want) {
		t.Fatalf("steps\n%#v\nwant\n%#v", got, want)
	}
	if !reflect.DeepEqual(plan.Secrets, []string{"wireless.guestnet.key", "network.wg0.private_key"}) {
		t.Errorf("secrets = %v", plan.Secrets)
	}
}

// TestHistoryIsScrubbedOfTheSecretsTheShellKnows: a history written before
// it kept no copy, or before a plugin named its secret, is scrubbed of every
// secret the shell knows when it starts and when a plugin arrives.
func TestHistoryIsScrubbedOfTheSecretsTheShellKnows(t *testing.T) {
	s, store, path, _, _ := wifiServer(t, fakeBackend{})
	record(t, store, history.Change{Time: time.Unix(60, 0), Status: history.ChangeApplied, OldRead: true, Items: []history.ChangeItem{
		{Page: "Wi-Fi", Verb: "removed", Words: "wireless: remove guestnet", Verbatim: true, Ops: []history.ChangeOp{
			{Config: "wireless", Section: "guestnet", Op: "remove-section", Old: old(`{".type":"wifi-iface","ssid":"Guest","key":"` + wifiKey + `"}`)},
		}},
		{Page: "VPN", Verb: "changed", Words: "vpn: office.auth_pass = PASS-BEFORE-ITS-PLUGIN", Verbatim: true, Ops: []history.ChangeOp{
			{Config: "vpn", Section: "office", Op: "set", Option: "auth_pass", Value: text("PASS-BEFORE-ITS-PLUGIN")},
		}},
	}})

	s.scrubHistory(t.Context())
	if found := heldIn(t, path, wifiKey); len(found) > 0 {
		t.Fatalf("history.db holds the key:\n%s", strings.Join(found, "\n"))
	}
	if found := heldIn(t, path, "PASS-BEFORE-ITS-PLUGIN"); len(found) == 0 {
		t.Fatal("an option no one names a secret was scrubbed")
	}

	m := demoACLManifest()
	m.ACL.Write = append(m.ACL.Write, plugin.ACLScope{Scope: "uci", Object: "vpn", Function: "write"})
	m.Secrets = []string{"vpn.auth_pass"}
	s.SetRescan(func() []plugin.Manifest { return []plugin.Manifest{m} })
	s.Rescan()
	if found := heldIn(t, path, "PASS-BEFORE-ITS-PLUGIN"); len(found) > 0 {
		t.Fatalf("history.db holds the plugin's secret:\n%s", strings.Join(found, "\n"))
	}
	all, _ := store.Changes(t.Context(), 10)
	if words := all[0].Items[1].Words; words != "vpn: office.auth_pass = ••••••••" {
		t.Errorf("words = %q", words)
	}
}

// TestASecretNoOneNamesStillReadsAsMasked: a secret's change whose option no
// list names any more, its plugin gone, is worded with its value masked, not
// as though it were set to nothing.
func TestASecretNoOneNamesStillReadsAsMasked(t *testing.T) {
	s, store, path, _, _ := wifiServer(t, fakeBackend{})
	record(t, store, history.Change{Time: time.Unix(60, 0), Status: history.ChangeApplied, OldRead: true, Items: []history.ChangeItem{
		{Page: "VPN", Verb: "changed", Words: "vpn: office.auth_pass = PASS-OF-A-GONE-PLUGIN", Verbatim: true, Ops: []history.ChangeOp{
			{Config: "vpn", Section: "office", Op: "set", Option: "auth_pass", Secret: true},
		}},
		{Page: "VPN", Verb: "changed", Words: "vpn: office.tls += TLS-OF-A-GONE-PLUGIN", Verbatim: true, Ops: []history.ChangeOp{
			{Config: "vpn", Section: "office", Op: "list-add", Option: "tls", Secret: true},
		}},
		{Page: "VPN", Verb: "changed", Words: "vpn: remove office.tls", Verbatim: true, Ops: []history.ChangeOp{
			{Config: "vpn", Section: "office", Op: "remove-option", Option: "tls", Secret: true},
		}},
	}})

	s.scrubHistory(t.Context())
	if found := heldIn(t, path, "OF-A-GONE-PLUGIN"); len(found) > 0 {
		t.Fatalf("history.db holds the secret:\n%s", strings.Join(found, "\n"))
	}
	all, _ := store.Changes(t.Context(), 10)
	var words []string
	for _, item := range all[0].Items {
		words = append(words, item.Words)
	}
	want := []string{"vpn: office.auth_pass = ••••••••", "vpn: office.tls += ••••••••", "vpn: remove office.tls"}
	if !reflect.DeepEqual(words, want) {
		t.Errorf("words = %q, want %q", words, want)
	}
}

// TestOpeningTheHistoryScrubsIt: the history the shell opens as it starts is
// scrubbed of every secret it knows before anything reads it.
func TestOpeningTheHistoryScrubsIt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.db")
	written, err := history.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	record(t, written, history.Change{Time: time.Unix(60, 0), Status: history.ChangeApplied, OldRead: true, Items: []history.ChangeItem{
		{Page: "Wi-Fi", Verb: "removed", Words: "wireless: remove guestnet", Verbatim: true, Ops: []history.ChangeOp{
			{Config: "wireless", Section: "guestnet", Op: "remove-section", Old: old(`{".type":"wifi-iface","ssid":"Guest","key":"` + wifiKey + `"}`)},
		}},
	}})
	if err := written.Close(); err != nil {
		t.Fatal(err)
	}

	s := newServerWith(t, fakeBackend{}, &fakeTransport{}, nil)
	store, onDisk := s.openHistory(dir, alerts.Config{})
	if store == nil || !onDisk {
		t.Fatalf("history not opened on disk: %v, %v", store, onDisk)
	}
	t.Cleanup(func() { _ = store.Close() })
	if found := heldIn(t, path, wifiKey); len(found) > 0 {
		t.Fatalf("history.db holds the key:\n%s", strings.Join(found, "\n"))
	}
}

// TestAHistoryKeptBeforeAnOptionWasListedIsScrubbedOfIt: Wi-Fi's RADIUS,
// 802.11r and WPS secrets, its raw hostapd lines, and the alert addresses,
// under each name uci knows them by, were kept in plaintext by a shell whose
// list left them out: in a change's values and its raw words, and in each
// config as last read and as it stood at the starting point. The scrub the
// shell runs as it opens the history takes every one out, and keeps what is
// no secret.
func TestAHistoryKeptBeforeAnOptionWasListedIsScrubbedOfIt(t *testing.T) {
	listed := map[string][]string{
		"wireless": {"dae_secret", "radius_das_secret", "auth_server_shared_secret", "acct_server_shared_secret",
			"r0kh", "r1kh", "ap_pin", "wps_pin", "multi_ap_backhaul_key", "priv_key2_pwd", "private_key2_passwd",
			"hostapd_bss_options"},
		"verso": {"url", "heartbeat"},
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "history.db")
	written, err := history.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	reading := map[string]map[string]any{}
	var items []history.ChangeItem
	for config, options := range listed {
		section := map[string]any{".type": "kept", "label": "NO-SECRET"}
		for _, option := range options {
			value := "PLAINTEXT-" + option
			op, words, was := "set", " = ", `"PLAINTEXT-BEFORE-`+option+`"`
			section[option] = value
			if option == "r0kh" || option == "r1kh" || option == "hostapd_bss_options" {
				// A key holder is a list entry, its key the entry's last
				// field; a raw hostapd line one, its value after the =.
				op, words, was = "list-add", " += ", `["PLAINTEXT-BEFORE-`+option+`"]`
				section[option] = []string{value}
			}
			items = append(items, history.ChangeItem{Page: "P", Verb: "changed", Verbatim: true,
				Words: config + ": s." + option + words + value,
				Ops: []history.ChangeOp{{Config: config, Section: "s", Op: op, Option: option,
					Value: text(value), Old: old(was)}}})
		}
		reading[config] = map[string]any{"s": section}
	}
	record(t, written, history.Change{Time: time.Unix(60, 0), Status: history.ChangeApplied, OldRead: true, Items: items})
	for config, sections := range reading {
		held, _ := json.Marshal(sections)
		if err := errors.Join(written.SetConfigState(t.Context(), config, held, time.Unix(60, 0)),
			written.StartConfig(t.Context(), config, held, time.Unix(0, 0))); err != nil {
			t.Fatal(err)
		}
	}
	if err := written.Close(); err != nil {
		t.Fatal(err)
	}

	s := newServerWith(t, fakeBackend{}, &fakeTransport{}, nil)
	store, onDisk := s.openHistory(dir, alerts.Config{})
	if store == nil || !onDisk {
		t.Fatalf("history not opened on disk: %v, %v", store, onDisk)
	}
	t.Cleanup(func() { _ = store.Close() })
	if found := heldIn(t, path, "PLAINTEXT"); len(found) > 0 {
		t.Fatalf("history.db holds a listed secret:\n%s", strings.Join(found, "\n"))
	}
	if found := heldIn(t, path, "NO-SECRET"); len(found) != 2*len(listed) {
		t.Errorf("what is no secret is kept in %d readings, want %d", len(found), 2*len(listed))
	}
}

// TestAPluginNamesSecretsOnlyInTheConfigsItWrites: a secret a manifest names
// in a config its acl does not write with scope uci, one it only reads
// included, is not taken for one, as a scrub erases a secret's values from
// all of the history.
func TestAPluginNamesSecretsOnlyInTheConfigsItWrites(t *testing.T) {
	m := demoACLManifest()
	m.ACL.Read = []plugin.ACLScope{{Scope: "uci", Object: "vpn", Function: "read"}, {Scope: "ubus", Object: "network", Function: "read"}}
	m.ACL.Write = append(m.ACL.Write, plugin.ACLScope{Scope: "ubus", Object: "dhcp", Function: "write"})
	m.Secrets = []string{"system.notes", "vpn.auth_pass", "network.ipaddr", "dhcp.key_file"}
	s := newServerWith(t, fakeBackend{}, &fakeTransport{}, []plugin.Manifest{m})
	secrets := s.secretOptions()
	if !secrets["system.notes"] {
		t.Error("system.notes, in a config the plugin writes, is no secret")
	}
	for _, other := range []string{"vpn.auth_pass", "network.ipaddr", "dhcp.key_file"} {
		if secrets[other] {
			t.Errorf("%s, in a config the plugin does not write, is a secret", other)
		}
	}
	if !secrets["network.password"] {
		t.Error("the shared list's secrets are gone")
	}
}

// TestAPluginsSecretsAreReadAsTheHelperReadsThem: verso-rpcd fingerprints
// the secrets the shell scrubs only if both read a manifest alike, so both
// read the cases in testdata/plugin-secrets.json, its own test the helper's.
func TestAPluginsSecretsAreReadAsTheHelperReadsThem(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "plugin-secrets.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Case     string          `json:"case"`
		Manifest json.RawMessage `json:"manifest"`
		Secrets  []string        `json:"secrets"`
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		t.Run(c.Case, func(t *testing.T) {
			manifests, problems := plugin.Discover(fstest.MapFS{"p/manifest.json": {Data: c.Manifest}}, "*/manifest.json")
			if len(manifests) != 1 || len(problems) > 0 {
				t.Fatalf("the manifest is not taken: %v", problems)
			}
			got := pluginSecrets(manifests[0])
			slices.Sort(got)
			if !slices.Equal(got, c.Secrets) {
				t.Errorf("secrets = %q, want %q", got, c.Secrets)
			}
		})
	}
}
