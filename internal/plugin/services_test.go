// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package plugin

import (
	"maps"
	"strings"
	"testing"
)

func servicer(id, services string) string {
	return `{"id":"` + id + `","name":"N","socket":"/run/verso/` + id + `.sock","schema_version":1,` +
		`"nav":[{"section":"Network","label":"N","path":"/"}],"services":` + services + `}`
}

// TestAPluginNamesTheServicesItBrings: services are the plugin's words for
// the init scripts it brings, in English its catalog translates.
func TestAPluginNamesTheServicesItBrings(t *testing.T) {
	fsys := mapFS(map[string]string{"plugins/ddns/manifest.json": servicer("ddns", `{"ddns": "Dynamic DNS"}`)})
	got, problems := Discover(fsys, testGlob)
	if len(problems) != 0 || len(got) != 1 {
		t.Fatalf("got=%v problems=%v", got, problems)
	}
	if want := map[string]string{"ddns": "Dynamic DNS"}; !maps.Equal(got[0].Services, want) {
		t.Errorf("services = %v, want %v", got[0].Services, want)
	}
}

// TestAMalformedServiceWordIsDroppedAlone: an entry whose key is no
// init-script name, or whose words are not text, is logged and dropped; the
// plugin's other words, and the plugin, load.
func TestAMalformedServiceWordIsDroppedAlone(t *testing.T) {
	fsys := mapFS(map[string]string{"plugins/ddns/manifest.json": servicer("ddns", `{"ddns": 1, "ddns-mon": "DDNS monitor", "../x": "Escape", "empty": ""}`)})
	got, problems := Discover(fsys, testGlob)
	if len(got) != 1 {
		t.Fatalf("the plugin did not load: %v", problems)
	}
	if want := map[string]string{"ddns-mon": "DDNS monitor"}; !maps.Equal(got[0].Services, want) {
		t.Errorf("services = %v, want %v", got[0].Services, want)
	}
	said := ""
	for _, p := range problems {
		said += p.Error() + "\n"
	}
	for _, want := range []string{`"ddns"`, `"../x"`, `"empty"`} {
		if !strings.Contains(said, want) {
			t.Errorf("problems do not name %s: %s", want, said)
		}
	}
}

// TestServicesThatAreNoObjectAreDroppedWhole: the plugin loads with no
// service words, and its pages stand.
func TestServicesThatAreNoObjectAreDroppedWhole(t *testing.T) {
	fsys := mapFS(map[string]string{"plugins/ddns/manifest.json": servicer("ddns", `["ddns"]`)})
	got, problems := Discover(fsys, testGlob)
	if len(got) != 1 || len(got[0].Nav) != 1 {
		t.Fatalf("the plugin did not load whole: got=%v problems=%v", got, problems)
	}
	if len(got[0].Services) != 0 || len(problems) != 1 || !strings.Contains(problems[0].Error(), "services") {
		t.Errorf("services = %v, problems = %v", got[0].Services, problems)
	}
}

// TestAServiceNameIsAnInitScriptName: the shape procd's rc object takes.
func TestAServiceNameIsAnInitScriptName(t *testing.T) {
	for name, want := range map[string]bool{"ddns": true, "verso-plugin-ddns": true, "gpio_switch": true, "a.b": true, "": false, "../x": false, "-x": false, "X y": false} {
		if got := ServiceName(name); got != want {
			t.Errorf("ServiceName(%q) = %v, want %v", name, got, want)
		}
	}
}
