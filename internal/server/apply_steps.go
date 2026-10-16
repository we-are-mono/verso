// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"fmt"
	"slices"
	"strings"

	"github.com/we-are-mono/verso/internal/i18n"
	"github.com/we-are-mono/verso/internal/units"
)

// applyStep is one line of the Pending changes page's When you apply card:
// what an apply does, in the order it does it (ADR-010). Kind is the step's
// part in the apply ("save", "reload", "check"), which the page's script
// reports each step's state by; Title is the whole of a step its line
// shortens.
type applyStep struct {
	Kind  string
	Text  string
	Title string
}

// reloadsNamed is how many reloads a reload step names before it counts the
// rest, so the step keeps to one line.
const reloadsNamed = 4

// serviceWords is what the page calls the init scripts procd reloads: a
// script the operator knows by its purpose, by that. Two scripts that serve
// one purpose share its name and are said once. A script with no word here
// is named as procd names it.
var serviceWords = map[string]string{
	"network":         "Network",
	"dnsmasq":         "DNS & DHCP",
	"odhcpd":          "DNS & DHCP",
	"firewall":        "Firewall",
	"dropbear":        "SSH",
	"uhttpd":          "Web server",
	"system":          "System",
	"log":             "System log",
	"sysntpd":         "Time sync",
	"led":             "LEDs",
	"nlbwmon":         "Bandwidth monitor",
	"packet_steering": "Packet steering",
	"verso":           "Web interface",
	"ddns":            "Dynamic DNS",
}

// applySteps is the card's steps for n changes to configs, given procd's
// reload triggers: the router saves them; it reloads what the triggers name
// for those configs, together and in no order it promises, so as one step;
// it keeps them once the page checks in within the rollback window. A stage
// whose configs no trigger names has no reload step.
func applySteps(n int, configs []string, triggers map[string][]string, tr func(string) string) []applyStep {
	steps := []applyStep{{Kind: "save", Text: i18n.Count(tr, n, "Saves %d change.", "Saves %d changes. (two)", "Saves %d changes. (few)", "Saves %d changes.")}}
	if names := reloadNames(configs, triggers, tr); len(names) > 0 {
		shown := names
		if len(names) > reloadsNamed {
			more := len(names) - reloadsNamed
			shown = append(slices.Clone(names[:reloadsNamed]), i18n.Count(tr, more, "%d more", "%d more (two)", "%d more (few)", "%d more"))
		}
		step := applyStep{Kind: "reload", Text: fmt.Sprintf(tr("Reloads %s."), andList(shown, tr))}
		if len(names) > reloadsNamed {
			step.Title = andList(names, tr)
		}
		steps = append(steps, step)
	}
	return append(steps, applyStep{Kind: "check", Text: fmt.Sprintf(tr("Keeps them if this page checks in within %s."), units.Since(uciRollbackTimeout))})
}

// reloadNames is what the triggers reload for configs, each named once:
// Network first, the reload that drops every connection; then each config's
// own service before its others.
func reloadNames(configs []string, triggers map[string][]string, tr func(string) string) []string {
	var scripts []string
	add := func(script string) {
		if !slices.Contains(scripts, script) {
			scripts = append(scripts, script)
		}
	}
	for _, config := range configs {
		if slices.Contains(triggers[config], "network") {
			add("network")
		}
	}
	for _, config := range configs {
		if slices.Contains(triggers[config], config) {
			add(config)
		}
		for _, script := range triggers[config] {
			add(script)
		}
	}
	var names []string
	for _, script := range scripts {
		name := script
		if word, ok := serviceWords[script]; ok {
			name = tr(word)
		}
		if !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	return names
}

// andList joins names as a sentence does: "A, B and C".
func andList(names []string, tr func(string) string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	}
	return fmt.Sprintf(tr("%s and %s"), strings.Join(names[:len(names)-1], ", "), names[len(names)-1])
}
