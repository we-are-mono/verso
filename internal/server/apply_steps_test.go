// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"reflect"
	"testing"

	"github.com/we-are-mono/verso/internal/plugin"
)

// baseNames names services in the shell's words alone, no plugin installed.
func baseNames(tr func(string) string) func(string) string {
	return serviceNamer(nil, tr, nil)
}

func stepTexts(steps []applyStep) []string {
	var out []string
	for _, s := range steps {
		out = append(out, s.Text)
	}
	return out
}

// TestApplyStepsSayWhatTheRouterDoes: the card numbers what an apply does,
// in the order it does it, one line each: it saves the changes, it reloads
// what procd's triggers name for their configs, and it keeps them once the
// page checks in within the rollback window.
func TestApplyStepsSayWhatTheRouterDoes(t *testing.T) {
	identity := func(s string) string { return s }
	triggers := map[string][]string{"dhcp": {"dnsmasq"}, "firewall": {"firewall"}}
	steps := applySteps(3, []string{"dhcp", "firewall"}, triggers, identity, baseNames(identity))
	want := []string{
		"Saves 3 changes.",
		"Reloads DNS & DHCP and Firewall.",
		"Keeps them if this page checks in within 30 s.",
	}
	if got := stepTexts(steps); !reflect.DeepEqual(got, want) {
		t.Errorf("steps = %q, want %q", got, want)
	}
	if steps[0].Kind != "save" || steps[1].Kind != "reload" || steps[2].Kind != "check" {
		t.Errorf("kinds = %s %s %s, want save reload check", steps[0].Kind, steps[1].Kind, steps[2].Kind)
	}
}

// TestApplyStepsLeadWithNetwork: Network, the reload that drops every
// connection, leads the sentence; two scripts that serve one purpose are
// named once; a script the shell has no word for is named as procd names it.
func TestApplyStepsLeadWithNetwork(t *testing.T) {
	identity := func(s string) string { return s }
	triggers := map[string][]string{
		"dhcp":    {"dnsmasq", "odhcpd"},
		"network": {"network", "odhcpd"},
		"vpnd":    {"tunneld"},
	}
	steps := applySteps(1, []string{"dhcp", "network", "vpnd"}, triggers, identity, baseNames(identity))
	if got := steps[0].Text; got != "Saves 1 change." {
		t.Errorf("save step = %q, want the count's own form", got)
	}
	if got := steps[1].Text; got != "Reloads Network, DNS & DHCP and tunneld." {
		t.Errorf("reload step = %q", got)
	}
}

// TestApplyStepsWithoutAReload: a stage whose configs no trigger names has no
// reload step, and the check-in follows the save.
func TestApplyStepsWithoutAReload(t *testing.T) {
	identity := func(s string) string { return s }
	steps := applySteps(2, []string{"verso_extra"}, nil, identity, baseNames(identity))
	if got := stepTexts(steps); len(got) != 2 || got[0] != "Saves 2 changes." || steps[1].Kind != "check" {
		t.Errorf("steps = %q, want the save and the check-in alone", got)
	}
}

// TestApplyStepsNameAtMostFour: a config's own service leads its others; a
// long run of reloads names four and counts the rest, so the step keeps to
// one line, and the whole list rides as its title.
func TestApplyStepsNameAtMostFour(t *testing.T) {
	identity := func(s string) string { return s }
	triggers := map[string][]string{"system": {"dnsmasq", "led", "log", "nlbwmon", "sysntpd", "system"}}
	steps := applySteps(1, []string{"system"}, triggers, identity, baseNames(identity))
	if got := steps[1].Text; got != "Reloads System, DNS & DHCP, LEDs, System log and 2 more." {
		t.Errorf("reload step = %q", got)
	}
	if got := steps[1].Title; got != "System, DNS & DHCP, LEDs, System log, Bandwidth monitor and Time sync" {
		t.Errorf("reload title = %q", got)
	}
}

// TestTheReloadStepNamesEachServiceInItsOwnersWords: a base OpenWrt service,
// or the shell's own, in the shell's word; a service a plugin brings in that
// plugin's word and its catalog's language, installed or not running; any
// other as procd names it. A plugin's word never replaces the shell's, and
// two services with one word are said once.
func TestTheReloadStepNamesEachServiceInItsOwnersWords(t *testing.T) {
	identity := func(s string) string { return s }
	ddns := plugin.Manifest{ID: "ddns", Services: map[string]string{"ddns": "Dynamic DNS", "ddns-mon": "Dynamic DNS"}}
	vpn := plugin.Manifest{ID: "vpn", Services: map[string]string{"openvpn": "OpenVPN"}}
	renamer := plugin.Manifest{ID: "a-renamer", Services: map[string]string{"firewall": "Packet filter"}}
	slovenian := func(pluginID string) func(string) string {
		return func(s string) string {
			if pluginID == "ddns" && s == "Dynamic DNS" {
				return "Dinamični DNS"
			}
			return s
		}
	}
	english := func(string) func(string) string { return identity }
	for _, c := range []struct {
		name      string
		manifests []plugin.Manifest
		pluginTr  func(string) func(string) string
		config    string
		scripts   []string
		want      string
	}{
		{"ddns", []plugin.Manifest{ddns}, english, "ddns", []string{"ddns"}, "Reloads Dynamic DNS."},
		{"openvpn", []plugin.Manifest{vpn}, english, "openvpn", []string{"openvpn"}, "Reloads OpenVPN."},
		{"a plugin that declares no words", []plugin.Manifest{{ID: "vpn"}}, english, "openvpn", []string{"openvpn"}, "Reloads openvpn."},
		{"a plugin that renames a base service", []plugin.Manifest{renamer}, english, "firewall", []string{"firewall"}, "Reloads Firewall."},
		{"in the plugin's language", []plugin.Manifest{ddns}, slovenian, "ddns", []string{"ddns"}, "Reloads Dinamični DNS."},
		{"two services with one word", []plugin.Manifest{ddns}, english, "ddns", []string{"ddns", "ddns-mon"}, "Reloads Dynamic DNS."},
	} {
		triggers := map[string][]string{c.config: c.scripts}
		steps := applySteps(1, []string{c.config}, triggers, identity, serviceNamer(c.manifests, identity, c.pluginTr))
		if got := steps[1].Text; got != c.want {
			t.Errorf("%s: reload step = %q, want %q", c.name, got, c.want)
		}
	}
}

// TestAStoppedPluginsServiceKeepsItsWords: the namer reads the installed
// manifests, not the running plugins.
func TestAStoppedPluginsServiceKeepsItsWords(t *testing.T) {
	ddns := plugin.Manifest{ID: "ddns", Socket: "/ddns.sock", Services: map[string]string{"ddns": "Dynamic DNS"}}
	s := navServer(ddns)
	s.probe = func(plugin.Endpoint) bool { return false }
	identity := func(s string) string { return s }
	name := serviceNamer(s.manifestsByID(), identity, func(string) func(string) string { return identity })
	if got := name("ddns"); got != "Dynamic DNS" {
		t.Errorf("a stopped plugin's service = %q, want Dynamic DNS", got)
	}
}
