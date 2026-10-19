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
	r, dropped, err := ParseRegistry(data)
	if err != nil || len(dropped) != 0 {
		t.Fatal(err, dropped)
	}
	if read, ok := r.Read("state"); !ok || read.Version != 1 {
		t.Errorf("state = %+v, %v", read, ok)
	}
	if r.Certificates["state"][0] != "certificate" {
		t.Errorf("certificates = %v", r.Certificates)
	}
}

// registryWithEvents is acme's state read with events declared beside it.
func registryWithEvents(events string) string {
	return `{"schema_version":1,"commands":[` +
		`{"name":"state","kind":"read","version":1,"exec":"/usr/bin/verso-plugin-acme"}],` +
		`"events":[` + events + `]}`
}

// TestRegistryDeclaresEvents: each event a plugin announces is declared beside
// its commands (ADR-028): its sentence, tone, the read whose grant shows it,
// whether it is said, whether it alerts, and the events it ends.
func TestRegistryDeclaresEvents(t *testing.T) {
	fsys := mapFS(map[string]string{
		"plugins/acme/manifest.json": validManifest("acme"),
		"plugins/acme/commands.json": registryWithEvents(
			`{"name":"renewed","sentence":"The certificate for %s was renewed.","tone":"success","read":"state","notify":true,"ends":["renew-failed"]},` +
				`{"name":"renew-failed","sentence":"The certificate for %s could not be renewed.","tone":"danger","read":"state","alert":true}`),
	})
	got, problems := Discover(fsys, testGlob)
	if len(problems) != 0 {
		t.Fatalf("problems = %v", problems)
	}
	renewed, ok := got[0].Commands.Event("renewed")
	want := EventDeclaration{Name: "renewed", Sentence: "The certificate for %s was renewed.", Tone: "success",
		Read: "state", Notify: true, Ends: []string{"renew-failed"}}
	if !ok || renewed.Name != want.Name || renewed.Sentence != want.Sentence || renewed.Tone != want.Tone ||
		renewed.Read != want.Read || !renewed.Notify || renewed.Alert || len(renewed.Ends) != 1 || renewed.Ends[0] != "renew-failed" {
		t.Errorf("renewed = %+v, %v; want %+v", renewed, ok, want)
	}
	failed, ok := got[0].Commands.Event("renew-failed")
	if !ok || failed.Notify || !failed.Alert || len(failed.Ends) != 0 {
		t.Errorf("renew-failed = %+v, %v; want quiet (notify absent), alerting, ending nothing", failed, ok)
	}
	if _, ok := got[0].Commands.Event("issued"); ok {
		t.Error("an undeclared event is not declared")
	}
	var none *Registry
	if _, ok := none.Event("renewed"); ok {
		t.Error("a nil registry declares nothing")
	}
}

// TestRegistryDropsABrokenEventAlone: a declaration that breaks a rule is
// dropped with its reason, and the plugin keeps its other declarations and
// its commands.
func TestRegistryDropsABrokenEventAlone(t *testing.T) {
	good := `{"name":"renewed","sentence":"The certificate for %s was renewed.","tone":"success","read":"state"}`
	for name, tc := range map[string]struct{ bad, reason string }{
		"a name with capitals":    {`{"name":"Renewed","sentence":"x","tone":"success","read":"state"}`, "name"},
		"a name too long":         {`{"name":"` + strings.Repeat("a", 65) + `","sentence":"x","tone":"success","read":"state"}`, "name"},
		"an unknown tone":         {`{"name":"odd","sentence":"x","tone":"crimson","read":"state"}`, "tone"},
		"no such read":            {`{"name":"odd","sentence":"x","tone":"info","read":"status"}`, "read"},
		"an end not declared":     {`{"name":"odd","sentence":"x","tone":"info","read":"state","ends":["expired"]}`, "ends"},
		"an end of itself":        {`{"name":"odd","sentence":"x","tone":"info","read":"state","ends":["odd"]}`, "ends"},
		"no sentence":             {`{"name":"odd","sentence":"","tone":"info","read":"state"}`, "sentence"},
		"a sentence of two lines": {`{"name":"odd","sentence":"one\ntwo","tone":"info","read":"state"}`, "sentence"},
		"a name declared twice":   {`{"name":"renewed","sentence":"again","tone":"info","read":"state"}`, "twice"},
	} {
		fsys := mapFS(map[string]string{
			"plugins/acme/manifest.json": validManifest("acme"),
			"plugins/acme/commands.json": registryWithEvents(good + "," + tc.bad),
		})
		got, problems := Discover(fsys, testGlob)
		if len(got) != 1 || got[0].Commands == nil {
			t.Fatalf("%s: the plugin or its registry was lost: %+v", name, got)
		}
		if len(problems) != 1 || !strings.Contains(problems[0].Error(), "commands.json") || !strings.Contains(problems[0].Error(), tc.reason) {
			t.Errorf("%s: problems = %v, want one naming commands.json and %q", name, problems, tc.reason)
		}
		if e, ok := got[0].Commands.Event("renewed"); !ok || e.Sentence != "The certificate for %s was renewed." {
			t.Errorf("%s: the good declaration = %+v, %v", name, e, ok)
		}
		if len(got[0].Commands.Events) != 1 {
			t.Errorf("%s: events = %+v, want the good one alone", name, got[0].Commands.Events)
		}
		if _, ok := got[0].Commands.Read("state"); !ok {
			t.Errorf("%s: the commands were lost", name)
		}
	}
}

// TestRegistryFromBeforeEventsDeclaresNone: a registry with no events parses
// as it always has.
func TestRegistryFromBeforeEventsDeclaresNone(t *testing.T) {
	r, dropped, err := ParseRegistry([]byte(`{"schema_version":1,"commands":[{"name":"state","kind":"read","version":1,"exec":"/x"}]}`))
	if err != nil || len(dropped) != 0 {
		t.Fatalf("err = %v, dropped = %v", err, dropped)
	}
	if len(r.Events) != 0 {
		t.Errorf("events = %+v", r.Events)
	}
	if _, ok := r.Read("state"); !ok {
		t.Error("the read was lost")
	}
}
