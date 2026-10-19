// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package plugin

import (
	"strings"
	"testing"
)

// TestDiscoverRefusesCoreNames: the names of core's events and alert kinds
// are core's (ADR-028), so a plugin cannot speak as the router, and its
// alert kind cannot merge with one of core's. They are refused as the
// shell's own id is.
func TestDiscoverRefusesCoreNames(t *testing.T) {
	for _, id := range []string{"wan", "boot", "changes", "rollback", "storage", "certificate", "firmware", "device", "dhcp"} {
		fsys := mapFS(map[string]string{"plugins/" + id + "/manifest.json": validManifest(id)})
		got, problems := Discover(fsys, testGlob)
		if len(got) != 0 {
			t.Errorf("%s: loaded %+v", id, got)
		}
		if len(problems) != 1 || !strings.Contains(problems[0].Error(), "reserved") || !strings.Contains(problems[0].Error(), id) {
			t.Errorf("%s: problems = %v, want one saying the id is reserved", id, problems)
		}
	}
	if got, problems := Discover(mapFS(map[string]string{"plugins/dnsdhcp/manifest.json": validManifest("dnsdhcp")}), testGlob); len(got) != 1 || len(problems) != 0 {
		t.Errorf("dnsdhcp: got %v, problems %v; a name beginning like a core one is a plugin's", got, problems)
	}
}
