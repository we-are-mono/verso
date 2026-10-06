// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/we-are-mono/verso/internal/plugin"
	"github.com/we-are-mono/verso/internal/widget"
)

type ruleFilesBackend struct {
	fakeBackend
	files json.RawMessage
}

func (b ruleFilesBackend) FirewallFiles(context.Context, string) (json.RawMessage, error) {
	return b.files, nil
}

// TestFirewallRuleFilesAreBrokeredToTheirPlugin: the rule files fw4 reads are
// a helper read a plugin declares, handed down verbatim beside its snapshot.
func TestFirewallRuleFilesAreBrokeredToTheirPlugin(t *testing.T) {
	files := json.RawMessage(`{"files":[{"path":"/etc/nftables.d/10-a.nft","family":"fw4","content":"","version":"v"}],"fw4":true}`)
	tr := &fakeTransport{env: &plugin.Envelope{SchemaVersion: 1, Title: "Firewall", Widget: json.RawMessage(`{"type":"card"}`)}}
	m := demoManifest()
	m.ACL = plugin.ACL{Read: []plugin.ACLScope{{Scope: "ubus", Object: "verso", Function: "firewallFiles"}}}
	s := newServerWith(t, ruleFilesBackend{files: files}, tr, []plugin.Manifest{m})
	if rec := get(t, s, "/plugins/demo/"); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := tr.lastReq.Ubus["firewallFiles"]; string(got) != string(files) {
		t.Errorf("brokered read = %s, want the helper's JSON verbatim", got)
	}
}

// TestARefusedSwitchRefusesTheSave: a switch the plugin refused refuses the
// whole submission as a refused field does, with no form-wide sentence needed
// to say so.
func TestARefusedSwitchRefusesTheSave(t *testing.T) {
	refused := &widget.Switch{Name: "dnssec", Label: "Verify answers are signed", Error: "Needs dnsmasq-full."}
	if !validateSchema(&widget.Form{Fields: []widget.Widget{refused}}) {
		t.Error("a refused switch must refuse the save")
	}
	if validateSchema(&widget.Form{Fields: []widget.Widget{&widget.Switch{Name: "dnssec", Label: "Verify answers are signed"}}}) {
		t.Error("a switch nobody refused refuses nothing")
	}
}

type fileStagingBackend struct {
	fakeBackend
	staged []string
}

func (b *fileStagingBackend) StageConfigFile(_ context.Context, _, path, _, _ string) error {
	b.staged = append(b.staged, path)
	return nil
}

// fileWriter is a plugin allowed to stage files and to write one config.
func fileWriter(config string) plugin.Manifest {
	return plugin.Manifest{ID: config, ACL: plugin.ACL{Write: []plugin.ACLScope{
		{Scope: "uci", Object: config, Function: "write"},
		{Scope: "ubus", Object: "verso", Function: "stageConfigFile"},
	}}}
}

// TestAFileIsStagedOnlyByItsDaemonsWriter: a dnsmasq file belongs to whoever
// may write the dhcp config and an nftables rule file to whoever may write the
// firewall's, so neither plugin can stage the other's files — and a path in
// no family is staged by nobody.
func TestAFileIsStagedOnlyByItsDaemonsWriter(t *testing.T) {
	b := &fileStagingBackend{}
	s := newServerWith(t, b, nil, nil)
	stage := func(m plugin.Manifest, path string) error {
		return s.runPluginCommands(context.Background(), m, "operator", []plugin.ApplyAction{{
			Name: "config-file-stage",
			Args: map[string]string{"path": path, "expected": "v", "content": "x"},
		}})
	}
	for _, ok := range []struct {
		m    plugin.Manifest
		path string
	}{
		{fileWriter("dhcp"), "/etc/dnsmasq.d/10-local.conf"},
		{fileWriter("dhcp"), "/etc/dnsmasq.conf"},
		{fileWriter("firewall"), "/etc/nftables.d/10-custom.nft"},
		{fileWriter("openvpn"), "/etc/openvpn/proton.ovpn"},
	} {
		if err := stage(ok.m, ok.path); err != nil {
			t.Errorf("%s staging %s: %v", ok.m.ID, ok.path, err)
		}
	}
	for _, refused := range []struct {
		m    plugin.Manifest
		path string
	}{
		{fileWriter("firewall"), "/etc/dnsmasq.d/10-local.conf"},
		{fileWriter("dhcp"), "/etc/nftables.d/10-custom.nft"},
		{fileWriter("firewall"), "/etc/firewall.user"},
		{fileWriter("dhcp"), "/etc/passwd"},
		{fileWriter("firewall"), "/etc/openvpn/proton.ovpn"},
	} {
		if err := stage(refused.m, refused.path); err == nil {
			t.Errorf("%s staged %s", refused.m.ID, refused.path)
		}
	}
	if len(b.staged) != 4 {
		t.Errorf("only the allowed files reach the router: %v", b.staged)
	}
}

// TestAFileStagesBesideTheConfigThatNamesIt: importing a profile writes the
// file and the section that names it in one save, and both wait on the stage
// together. Only a staged file joins staged changes; an act that happens at
// once still stands alone.
func TestAFileStagesBesideTheConfigThatNamesIt(t *testing.T) {
	calls := []uciWrite{}
	b := &fileStagingBackend{fakeBackend: fakeBackend{access: true, writes: &calls}}
	m := fileWriter("openvpn")
	m.ID, m.Socket, m.SchemaVersion = "demo", "/run/demo.sock", supportedSchemaVersion
	m.Nav = []plugin.NavEntry{{Section: "VPN", Label: "VPN", Path: "/"}}
	commit := []plugin.CommitOp{{Config: "openvpn", Section: "proton", Values: map[string]any{"config": "/etc/openvpn/proton.ovpn"}}}
	env := &plugin.Envelope{SchemaVersion: 1, Title: "VPN", Status: http.StatusOK,
		Widget: json.RawMessage(`{"type":"text","markdown":"vpn"}`), Commit: commit,
		Commands: []plugin.ApplyAction{{Name: "config-file-stage", Args: map[string]string{"path": "/etc/openvpn/proton.ovpn", "expected": "v", "content": "client\n"}}}}
	s := newServerWith(t, b, &fakeTransport{env: env}, []plugin.Manifest{m})
	if rec := postPlugin(t, s, "/plugins/demo/", url.Values{"x": {"1"}}); rec.Code >= 400 {
		t.Fatalf("a file and its section answered %d", rec.Code)
	}
	if len(b.staged) != 1 || len(calls) != 1 {
		t.Errorf("staged files %v and writes %v, want one of each", b.staged, calls)
	}

	m.ACL.Write = append(m.ACL.Write, plugin.ACLScope{Scope: "ubus", Object: "network.interface", Function: "restart"})
	env.Commands = []plugin.ApplyAction{{Name: "interface-restart", Args: map[string]string{"interface": "lan"}}}
	s = newServerWith(t, b, &fakeTransport{env: env}, []plugin.Manifest{m})
	if rec := postPlugin(t, s, "/plugins/demo/", url.Values{"x": {"1"}}); rec.Code != http.StatusForbidden {
		t.Errorf("an immediate act beside staged changes answered %d, want refused", rec.Code)
	}
}
