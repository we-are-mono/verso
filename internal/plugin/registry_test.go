// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package plugin

import (
	"os"
	"strings"
	"testing"
)

// TestDiscoverReadsTheRegistryBesideTheManifest: commands.json rides with its
// manifest (ADR-027), so a rescan picks up an installed plugin's root commands
// with its pages.
func TestDiscoverReadsTheRegistryBesideTheManifest(t *testing.T) {
	fsys := mapFS(map[string]string{
		"plugins/vpn/manifest.json": validManifest("vpn"),
		"plugins/vpn/commands.json": `{"schema_version":1,"commands":[` +
			`{"name":"state","kind":"read","version":2,"exec":"/usr/bin/verso-plugin-vpn"},` +
			`{"name":"restart","kind":"act","exec":"/usr/bin/verso-plugin-vpn"}],` +
			`"certificates":{"state":["certificate"]}}`,
		"plugins/ddns/manifest.json": validManifest("ddns"),
	})
	got, problems := Discover(fsys, testGlob)
	if len(problems) != 0 {
		t.Fatalf("problems = %v", problems)
	}
	vpn, ddns := got[1], got[0]
	read, ok := vpn.Commands.Read("state")
	if !ok || read.Version != 2 {
		t.Errorf("vpn's state read = %+v, %v; want version 2", read, ok)
	}
	if _, ok := vpn.Commands.Read("restart"); ok {
		t.Error("an act is not a read")
	}
	if vpn.Commands.Certificates["state"][0] != "certificate" {
		t.Errorf("certificates = %v", vpn.Commands.Certificates)
	}
	if ddns.Commands != nil {
		t.Error("a plugin with no commands.json has no registry")
	}
	if _, ok := ddns.Commands.Read("state"); ok {
		t.Error("a nil registry has no reads")
	}
}

// TestDiscoverKeepsAPluginWhoseRegistryIsBroken: a broken registry costs the
// plugin its root commands, reported, and never its pages.
func TestDiscoverKeepsAPluginWhoseRegistryIsBroken(t *testing.T) {
	for name, body := range map[string]string{
		"unparsable":  `{`,
		"unversioned": `{"schema_version":1,"commands":[{"name":"state","kind":"read","exec":"/x"}]}`,
	} {
		fsys := mapFS(map[string]string{
			"plugins/vpn/manifest.json": validManifest("vpn"),
			"plugins/vpn/commands.json": body,
		})
		got, problems := Discover(fsys, testGlob)
		if len(got) != 1 || got[0].Commands != nil {
			t.Errorf("%s: manifests = %+v, want vpn without a registry", name, got)
		}
		if len(problems) != 1 || !strings.Contains(problems[0].Error(), "commands.json") {
			t.Errorf("%s: problems = %v, want one naming commands.json", name, problems)
		}
	}
}

// TestTheSDKRegistryFixtureDecodes: the shell reads the registry the SDK
// writes (plugins/verso-plugin-sdk/testdata/commands.json).
func TestTheSDKRegistryFixtureDecodes(t *testing.T) {
	data, err := os.ReadFile("../../plugins/verso-plugin-sdk/testdata/commands.json")
	if err != nil {
		t.Skip("SDK registry fixture not present — run `cargo test` in plugins/verso-plugin-sdk to generate it")
	}
	r, err := parseRegistry(data)
	if err != nil {
		t.Fatal(err)
	}
	if read, ok := r.Read("state"); !ok || read.Version != 1 {
		t.Errorf("state = %+v, %v", read, ok)
	}
	if r.Certificates["state"][0] != "certificate" {
		t.Errorf("certificates = %v", r.Certificates)
	}
}
