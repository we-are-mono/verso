// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/we-are-mono/verso/internal/plugin"
	"github.com/we-are-mono/verso/internal/widget"
)

// TestAPluginRequestNamesTheInstalledPlugins: every plugin request carries the
// ids of the installed plugins, in order, its own among them.
func TestAPluginRequestNamesTheInstalledPlugins(t *testing.T) {
	tr := &fakeTransport{env: &plugin.Envelope{SchemaVersion: 1, Title: "Demo", Widget: json.RawMessage(`{"type":"card"}`)}}
	demo, other := demoManifest(), demoManifest()
	other.ID, other.Name = "acme", "Trusted certificate"
	s := newServerWith(t, fakeBackend{}, tr, []plugin.Manifest{demo, other})
	if rec := get(t, s, "/plugins/demo/"); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := strings.Join(tr.lastReq.Plugins, ","); got != "acme,demo" {
		t.Errorf("installed plugins = %q, want acme,demo", got)
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

func (b *fileStagingBackend) StageConfigFile(_ context.Context, _, path, _, _ string, configs []string) error {
	b.staged = append(b.staged, path+" "+strings.Join(configs, ","))
	return nil
}

// fileWriter is a plugin allowed to stage files and to write one config.
func fileWriter(config string) plugin.Manifest {
	return plugin.Manifest{ID: config, ACL: plugin.ACL{Write: []plugin.ACLScope{
		{Scope: "uci", Object: config, Function: "write"},
		{Scope: "ubus", Object: "verso", Function: "stageConfigFile"},
	}}}
}

// TestAFileIsStagedWithTheConfigsItsPluginWrites: a file is staged only for a
// plugin that declares file staging, and reaches the helper with the configs
// that plugin may write, so the helper — which holds the family table — stages
// it only when its family's config is one of them (ADR-027).
func TestAFileIsStagedWithTheConfigsItsPluginWrites(t *testing.T) {
	b := &fileStagingBackend{}
	s := newServerWith(t, b, nil, nil)
	stage := func(m plugin.Manifest, path string) error {
		return s.runPluginCommands(context.Background(), m, "operator", []plugin.ApplyAction{{
			Name: "config-file-stage",
			Args: map[string]string{"path": path, "expected": "v", "content": "x"},
		}})
	}
	both := fileWriter("dhcp")
	both.ACL.Write = append(both.ACL.Write, plugin.ACLScope{Scope: "uci", Object: "network", Function: "write"})
	if err := stage(both, "/etc/dnsmasq.d/10-local.conf"); err != nil {
		t.Fatal(err)
	}
	if err := stage(fileWriter("firewall"), "/etc/nftables.d/10-custom.nft"); err != nil {
		t.Fatal(err)
	}
	reader := plugin.Manifest{ID: "reader", ACL: plugin.ACL{Write: []plugin.ACLScope{{Scope: "uci", Object: "dhcp", Function: "write"}}}}
	if stage(reader, "/etc/dnsmasq.d/10-local.conf") == nil {
		t.Error("a plugin that declares no file staging staged a file")
	}
	stager := plugin.Manifest{ID: "stager", ACL: plugin.ACL{Write: []plugin.ACLScope{{Scope: "ubus", Object: "verso", Function: "stageConfigFile"}}}}
	if stage(stager, "/etc/dnsmasq.d/10-local.conf") == nil {
		t.Error("a plugin that writes no config staged a file")
	}
	want := []string{"/etc/dnsmasq.d/10-local.conf dhcp,network", "/etc/nftables.d/10-custom.nft firewall"}
	if strings.Join(b.staged, "|") != strings.Join(want, "|") {
		t.Errorf("staged %v, want %v", b.staged, want)
	}
}

// TestAPluginReadsOnlyItsOwnFamiliesFiles: the file listing a plugin declares
// is narrowed to the families whose config the plugin reads.
func TestAPluginReadsOnlyItsOwnFamiliesFiles(t *testing.T) {
	files := json.RawMessage(`{"files":[` +
		`{"path":"/etc/nftables.d/10-a.nft","family":"fw4","content":"","version":"v"},` +
		`{"path":"/etc/dnsmasq.conf","family":"dnsmasq","content":"","version":"v"}],` +
		`"families":[{"name":"fw4","config":"firewall","plugin":"firewall"},{"name":"dnsmasq","config":"dhcp","plugin":"dnsdhcp"}],` +
		`"active":false}`)
	tr := &fakeTransport{env: &plugin.Envelope{SchemaVersion: 1, Title: "Firewall", Widget: json.RawMessage(`{"type":"card"}`)}}
	m := demoManifest()
	m.ACL = plugin.ACL{Read: []plugin.ACLScope{
		{Scope: "uci", Object: "firewall", Function: "read"},
		{Scope: "ubus", Object: "verso", Function: "configFiles"},
	}}
	s := newServerWith(t, configFilesBackend{files: files}, tr, []plugin.Manifest{m})
	if rec := get(t, s, "/plugins/demo/"); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	got := string(tr.lastReq.Ubus["configFiles"])
	if !strings.Contains(got, "/etc/nftables.d/10-a.nft") || strings.Contains(got, "dnsmasq") {
		t.Errorf("the firewall's file read = %s", got)
	}
}

type configFilesBackend struct {
	fakeBackend
	files json.RawMessage
}

func (b configFilesBackend) ConfigFiles(context.Context, string) (json.RawMessage, error) {
	return b.files, nil
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
