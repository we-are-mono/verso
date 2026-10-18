// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package openwrt

import (
	"context"
	"path"
	"slices"
	"strings"
)

// ReloadTriggers names, for each config, the init scripts procd reloads when
// a change to it is applied, from procd's own trigger table (ADR-010: procd
// owns that mapping, so the shell reads it rather than keeping a copy).
func (b *NativeBackend) ReloadTriggers(_ context.Context, sid string) (map[string][]string, error) {
	raw, err := callChecked(b.ubusSocket, sid, "service", "list", map[string]any{"verbose": true})
	if err != nil {
		return nil, err
	}
	return reloadTriggers(raw), nil
}

// reloadTriggers reads procd's service table into config → init scripts:
// each config.change trigger whose condition names a package, for each init
// script its action runs. A trigger of any other shape names nothing, so a
// table this does not understand lists no reload rather than a wrong one.
// Each config's scripts come sorted and once.
func reloadTriggers(table map[string]any) map[string][]string {
	out := map[string][]string{}
	for _, service := range table {
		detail, _ := service.(map[string]any)
		triggers, _ := detail["triggers"].([]any)
		for _, t := range triggers {
			trigger, _ := t.([]any)
			if len(trigger) < 2 || trigger[0] != "config.change" {
				continue
			}
			rule, _ := trigger[1].([]any)
			if len(rule) < 3 || rule[0] != "if" {
				continue
			}
			scripts := initScripts(rule[2:], nil)
			for _, pkg := range packagesNamed(rule[1], nil) {
				for _, script := range scripts {
					if !slices.Contains(out[pkg], script) {
						out[pkg] = append(out[pkg], script)
					}
				}
			}
		}
	}
	for config := range out {
		slices.Sort(out[config])
	}
	return out
}

// packagesNamed is every package a condition tests for: ["eq", "package", x]
// wherever it stands in it ("and", "or").
func packagesNamed(condition any, found []string) []string {
	list, ok := condition.([]any)
	if !ok {
		return found
	}
	if len(list) == 3 && list[0] == "eq" && list[1] == "package" {
		if pkg, ok := list[2].(string); ok && pkg != "" {
			return append(found, pkg)
		}
		return found
	}
	for _, part := range list {
		found = packagesNamed(part, found)
	}
	return found
}

// initScripts is every init script an action runs: ["run_script",
// "/etc/init.d/x", ...] wherever it stands in it. A script outside
// /etc/init.d (a ubus call) reloads no service.
func initScripts(action any, found []string) []string {
	list, ok := action.([]any)
	if !ok {
		return found
	}
	if len(list) >= 2 && list[0] == "run_script" {
		if script, ok := list[1].(string); ok && strings.HasPrefix(script, "/etc/init.d/") {
			return append(found, path.Base(script))
		}
		return found
	}
	for _, part := range list {
		found = initScripts(part, found)
	}
	return found
}
