// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
package server

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/we-are-mono/verso/internal/plugin"
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
	} {
		if err := stage(refused.m, refused.path); err == nil {
			t.Errorf("%s staged %s", refused.m.ID, refused.path)
		}
	}
	if len(b.staged) != 3 {
		t.Errorf("only the allowed files reach the router: %v", b.staged)
	}
}
