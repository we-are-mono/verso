// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/we-are-mono/verso/internal/openwrt"
)

func TestSystemLogSourcesAndSeverities(t *testing.T) {
	for _, tc := range []struct {
		message  string
		source   string
		body     string
		priority int64
		severity string
	}{
		{"verso[1055706]: verso listening on :8080", "verso", "verso listening on :8080", 30, "info"},
		{"verso-plugin-interfaces[1056119: verso-plugin interfaces listening on /var/run/verso/interfaces.sock", "verso-plugin-interfaces", "verso-plugin interfaces listening on /var/run/verso/interfaces.sock", 30, "info"},
		{"verso-plugin-interfaces[1056119]: bind failed", "verso-plugin-interfaces", "bind failed", 27, "err"},
		{"procd: failed adding instance cgroup for verso: Operation not permitted", "procd", "failed adding instance cgroup for verso: Operation not permitted", 28, "warn"},
		{"plain message", "syslog", "plain message", 29, "notice"},
		{"unclosed[text: message", "syslog", "unclosed[text: message", 27, "err"},
	} {
		t.Run(tc.message, func(t *testing.T) {
			row := systemLogRow(openwrt.LogEntry{ID: 7, Source: 1, Priority: tc.priority, Time: 1700000001000, Msg: tc.message})
			if row.Source != tc.source || row.Message != tc.body || row.Severity != tc.severity || row.At != 1700000001 || row.ID != 7 {
				t.Errorf("parsed log entry: %+v", row)
			}
		})
	}
	row := systemLogRow(openwrt.LogEntry{Source: 0, Priority: 4, Msg: "eth0: link down"})
	if row.Source != "kernel" || row.Message != "eth0: link down" || row.Severity != "warn" {
		t.Errorf("kernel message was parsed as a process tag: %+v", row)
	}
}

func TestSystemLogStreamKeepsZeroIDAndParsesSyslog(t *testing.T) {
	s := newServer(t, fakeBackend{logEntries: []openwrt.LogEntry{
		{ID: 0, Source: 0, Priority: 4, Time: 1700000000000, Msg: "link down"},
		{ID: 1, Source: 1, Priority: 27, Time: 1700000001000, Msg: "dnsmasq[42]: <script>alert(1)</script>"},
	}})
	_, payload := openStream(t, s, systemLogSource)
	var frame struct {
		Rows []systemLogEvent `json:"rows"`
	}
	if err := json.Unmarshal([]byte(payload), &frame); err != nil {
		t.Fatal(err)
	}
	if len(frame.Rows) != 2 || frame.Rows[0].ID != 0 || frame.Rows[0].Source != "kernel" || frame.Rows[0].Severity != "warn" {
		t.Fatalf("kernel row: %+v", frame)
	}
	if row := frame.Rows[1]; row.Source != "dnsmasq" || row.Severity != "err" || row.Message != "<script>alert(1)</script>" {
		t.Fatalf("syslog row: %+v", row)
	}
	if strings.Contains(payload, "<script>") {
		t.Error("wire JSON must escape HTML characters")
	}
}

// TestSystemLogActsStandOnTheHeadingLine: the log has no band. The search and
// what acts on the log — the live control, the download, the settings — stand
// on the heading line as equals, the search first, and that group is the log's
// bar; the log runs straight under the heading, and a source that cannot be
// read is said on a notice line.
func TestSystemLogActsStandOnTheHeadingLine(t *testing.T) {
	whole := get(t, newServer(t, fakeBackend{access: true}), "/system/logs").Body.String()
	body := whole[strings.LastIndex(whole, "</style>"):]
	heading := strings.Index(body, `verso-page-heading">Logs`)
	bar, search := strings.Index(body, `data-verso-actionbar="log"`), strings.Index(body, "data-verso-listing-filter")
	live, download := strings.Index(body, "data-log-pause"), strings.Index(body, "data-log-download")
	settings, rows := strings.Index(body, `href="/system/logs/settings"`), strings.Index(body, "data-verso-console-rows")
	if heading < 0 || bar < 0 || search < 0 || live < 0 || download < 0 || settings < 0 || rows < 0 {
		t.Fatalf("logs page missing heading, search, acts or log:\n%s", body)
	}
	if !(heading < bar && bar < search && search < live && live < download && download < settings && settings < rows) {
		t.Errorf("search, Live, Download, Settings should stand in that order on the heading line, the log under them (h1 %d, bar %d, search %d, live %d, download %d, settings %d, log %d)", heading, bar, search, live, download, settings, rows)
	}
	// the acts wear the secondary dress every quiet button does: meta words on
	// the strong hairline, the hairline wash and ink under the pointer
	if acts := body[live:rows]; strings.Count(acts, "border-rule-strong bg-transparent text-meta transition-colors hover:border-sand-5 hover:bg-rule hover:text-ink") != 3 {
		t.Errorf("the log's three acts wear the secondary dress:\n%s", acts)
	}
	for _, want := range []string{
		"overflow-y-auto bg-quiet pt-3 pb-6 shadow-[inset_0_4px_4px_-4px_rgba(27,25,23,.14)]", // the log on the sand bar's ground, under the light bar, sunk under it
		`data-log-pause title="Pause"`, "<span data-log-pause-label>Connecting…</span></button>",
		"data-verso-wait", // the log's spinner, turning while lines arrive
		`<p data-log-health role="status" hidden`,
		`<div data-verso-masthead="light" class="mb-10 py-4">`, // the light bar, on the page's own ground
	} {
		if !strings.Contains(body, want) {
			t.Errorf("logs page missing %q", want)
		}
	}
	// the log is read whole: no cut, no source, and its firewall traffic is
	// the settings drawer's to turn on
	for _, never := range []string{"data-log-state", "data-verso-listing-cut", "data-verso-listing-select", "data-log-include-firewall", "px-10 py-4"} {
		if strings.Contains(body, never) {
			t.Errorf("logs page still carries %q", never)
		}
	}
}

// TestLogSettingsHoldTheFirewallTraffic: whether the log folds in the
// firewall's traffic is a way of reading it, so it stands in the settings
// drawer over the log, above the settings and outside their form: it never
// stages.
func TestLogSettingsHoldTheFirewallTraffic(t *testing.T) {
	s := newServer(t, fakeBackend{access: true, uci: map[string]map[string]any{"system": {"main": map[string]any{".type": "system", "hostname": "router"}}}})
	body := getPanel(t, s, "/system/logs/settings").Body.String()
	toggle, form := strings.Index(body, "data-log-include-firewall"), strings.Index(body, "<form")
	if toggle < 0 || form < 0 || toggle > form || !strings.Contains(body, "Include firewall traffic") {
		t.Errorf("the firewall traffic switch should stand above the settings form:\n%s", body)
	}
}

// TestLogSettingsSayWhatTheySave: the settings form's button names what it
// saves, never a bare "Save".
func TestLogSettingsSayWhatTheySave(t *testing.T) {
	s := newServer(t, fakeBackend{access: true, uci: map[string]map[string]any{"system": {"main": map[string]any{".type": "system", "hostname": "router"}}}})
	body := get(t, s, "/system/logs/settings").Body.String()
	if !strings.Contains(body, ">Save log settings<") || strings.Contains(body, ">Save<") {
		t.Errorf("log settings must name what they save:\n%s", body)
	}
}

func TestLogSettingsValidateBeforeStaging(t *testing.T) {
	var writes []uciWrite
	s := newServer(t, fakeBackend{access: true, writes: &writes, uci: map[string]map[string]any{"system": {"main": map[string]any{".type": "system", "hostname": "router"}}}})
	form := url.Values{"log_ip": {"192.0.2.42"}, "log_port": {"514"}, "log_proto": {"tcp"}, "log_buffer_size": {"256"}, "log_file": {""}, "log_remote": {"1"}}
	form.Set("log_port", "70000")
	if res := postPlugin(t, s, "/system/logs/settings", form); res.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid port status=%d", res.Code)
	}
	if len(writes) != 0 {
		t.Fatal("invalid input must not stage any value")
	}
	form.Set("log_port", "514")
	if res := postPlugin(t, s, "/system/logs/settings", form); res.Code != http.StatusSeeOther {
		t.Fatalf("valid settings status=%d", res.Code)
	}
	if len(writes) != 1 || writes[0].config != "system" || writes[0].section != "main" || writes[0].values["log_buffer_size"] != "256" {
		t.Fatalf("staged settings=%+v", writes)
	}
	if _, ok := writes[0].values["hostname"]; ok {
		t.Error("log settings must not rewrite the hostname")
	}
	if writes[0].values["log_remote"] != "1" {
		t.Error("remote logging must follow its switch")
	}
	form.Set("log_remote", "0")
	postPlugin(t, s, "/system/logs/settings", form)
	if len(writes) != 2 || writes[1].values["log_remote"] != "0" {
		t.Fatal("saving a remote host must preserve disabled forwarding")
	}
	if !s.declaredConfigsUnion()["system"] {
		t.Error("log edits must appear in the shared stage")
	}
}

func TestServiceIconsChangeRuntimeOnly(t *testing.T) {
	var actions []string
	s := newServer(t, fakeBackend{access: true, rcInits: &actions})
	for _, verb := range []string{"start", "stop", "restart"} {
		res := postPlugin(t, s, "/system/services", url.Values{"_service_action": {verb + ":dnsmasq"}})
		if res.Code != http.StatusSeeOther {
			t.Fatalf("%s: status=%d", verb, res.Code)
		}
	}
	if !reflect.DeepEqual(actions, []string{"dnsmasq start", "dnsmasq stop", "dnsmasq restart"}) {
		t.Fatalf("actions=%v", actions)
	}
	for _, value := range []string{"stop:firewall", "restart:verso", "reload:dnsmasq", "stop:../network"} {
		postPlugin(t, s, "/system/services", url.Values{"_service_action": {value}})
	}
	if len(actions) != 3 {
		t.Fatalf("a refused service action reached rc: %v", actions)
	}
	// The firewall's rules reload, the one place a reload is offered.
	if res := postPlugin(t, s, "/system/services", url.Values{"_service_action": {"reload:firewall"}}); res.Code != http.StatusSeeOther {
		t.Fatalf("reload firewall: status=%d", res.Code)
	}
	if got := actions[len(actions)-1]; got != "firewall reload" {
		t.Fatalf("reloading the firewall's rules should reach rc as a reload, got %q", got)
	}
}

func TestRebootRefusesUnresolvedOrUnreadableStage(t *testing.T) {
	for _, b := range []fakeBackend{
		{changes: map[string][][]string{"system": {{"set", "main", "hostname", "new"}}}},
		{changesErr: errors.New("unreadable")},
	} {
		rebooted := false
		b.restart = func(context.Context, string) error { rebooted = true; return nil }
		s := newServer(t, b)
		res := postPlugin(t, s, "/system/maintenance/restart", url.Values{})
		if res.Code < 400 || rebooted {
			t.Fatalf("reboot must wait for resolved stage: status=%d rebooted=%v", res.Code, rebooted)
		}
	}
}
