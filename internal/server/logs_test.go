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
