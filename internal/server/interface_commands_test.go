// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/plugin"
	"github.com/we-are-mono/verso/internal/widget"
)

func TestNamedCreationPreflightsWholeBatch(t *testing.T) {
	var writes []uciWrite
	var adds []string
	b := fakeBackend{writes: &writes, adds: &adds, uci: map[string]map[string]any{"network": {"lan": map[string]any{".type": "interface"}}}}
	s := newServerWith(t, b, nil, nil)
	m := plugin.Manifest{ID: "interfaces", ACL: plugin.ACL{Write: []plugin.ACLScope{{Scope: "uci", Object: "network", Function: "write"}}}}
	tr := func(s string) string { return s }
	update := plugin.CommitOp{Config: "network", Section: "lan", Values: map[string]any{"auto": "0"}}
	duplicate := plugin.CommitOp{Config: "network", Section: "lan", Type: "interface", Values: map[string]any{"proto": "dhcp"}}
	if _, _, ok := s.brokerStage(context.Background(), m, authorAt(m, "/"), "sid", []plugin.CommitOp{update, duplicate}, tr); ok || len(writes) > 0 || len(adds) > 0 {
		t.Fatal("partial write before duplicate creation was rejected")
	}
	duplicate.Section = "guest"
	if _, _, ok := s.brokerStage(context.Background(), m, authorAt(m, "/"), "sid", []plugin.CommitOp{duplicate}, tr); !ok {
		t.Fatal("named create failed")
	}
	if len(adds) != 1 || adds[0] != "network interface guest" || len(writes) != 1 || writes[0].section != "guest" {
		t.Fatalf("wrong named create: %v %v", adds, writes)
	}
	if !restructures([]plugin.CommitOp{duplicate}) {
		t.Fatal("named creation did not refresh listing")
	}
}

type interfaceControlBackend struct {
	fakeBackend
	calls []string
}

func (b *interfaceControlBackend) NetworkSetUp(_ context.Context, sid, name string, up bool) error {
	b.calls = append(b.calls, fmt.Sprintf("%s %s %t", sid, name, up))
	return nil
}

func TestInterfacePowerCommandsRequireTheirOwnGrant(t *testing.T) {
	b := &interfaceControlBackend{}
	s := newServerWith(t, b, nil, nil)
	m := plugin.Manifest{ID: "interfaces", ACL: plugin.ACL{Write: []plugin.ACLScope{{Scope: "ubus", Object: "network.interface", Function: "down"}}}}
	command := plugin.ApplyAction{Name: "interface-down", Args: map[string]string{"interface": "wan"}}
	if err := s.runPluginCommands(context.Background(), m, "operator", []plugin.ApplyAction{command}); err != nil {
		t.Fatal(err)
	}
	if len(b.calls) != 1 || b.calls[0] != "operator wan false" {
		t.Fatalf("wrong runtime action: %v", b.calls)
	}
	for _, name := range []string{"", "loopback", "wan.other", "../wan", "wan;reboot"} {
		command.Args["interface"] = name
		if err := s.runPluginCommands(context.Background(), m, "operator", []plugin.ApplyAction{command}); err == nil {
			t.Fatalf("accepted target %q", name)
		}
	}
	command.Name, command.Args["interface"] = "interface-up", "wan"
	if err := s.runPluginCommands(context.Background(), m, "operator", []plugin.ApplyAction{command}); err == nil {
		t.Fatal("down grant authorized up")
	}
	if len(b.calls) != 1 {
		t.Fatal("an invalid command reached the router")
	}
}

func TestInventoryRefreshPreservesIdleTimeoutAndFlash(t *testing.T) {
	clock := &fakeClock{t: time.Unix(1_000_000, 0)}
	s := &Server{sessions: newSessionsClock(clock.now)}
	token := s.sessions.CreateWithMetadata("sid", "root", "", "")
	s.sessions.SetFlash(token, "success", "Saved.")
	r := httptest.NewRequest(http.MethodGet, "/plugins/interfaces/", nil)
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	r.Header.Set("X-Verso-Refresh", "1")
	clock.advance(sessionIdleTimeout - time.Minute)
	if _, ok := s.currentSession(r); !ok {
		t.Fatal("live session rejected")
	}
	if _, message := s.takeFlash(r); message != "" {
		t.Fatal("refresh consumed the foreground notice")
	}
	if _, message := s.sessions.TakeFlash(token); message != "Saved." {
		t.Fatal("notice was lost")
	}
	clock.advance(2 * time.Minute)
	if _, ok := s.currentSession(r); ok {
		t.Fatal("background refresh kept an idle session alive")
	}
}
func TestConditionalFieldsValidateOnlyActiveBranch(t *testing.T) {
	inactive := &widget.Field{Name: "ipaddr", Datatype: "ipaddr", Value: "unfinished"}
	active := &widget.Field{Name: "hostname", Datatype: "hostname", Value: "router"}
	form := &widget.Form{Fields: []widget.Widget{&widget.When{Name: "proto", Value: "static", Active: false, Children: []widget.Widget{inactive}}, active}}
	if validateSchema(form) || inactive.Error != "" {
		t.Fatal("hidden disabled field rejected submission")
	}
	form.Fields[0].(*widget.When).Active = true
	if !validateSchema(form) || inactive.Error == "" {
		t.Fatal("active field escaped validation")
	}
}
