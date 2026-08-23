// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package ubus

import "testing"

// TestClientAgainstRealUbus exercises the full client against a live ubus. It
// skips where no socket is present (dev hosts, CI), and runs inside the OpenWrt
// container where ubusd is up.
func TestClientAgainstRealUbus(t *testing.T) {
	c, err := Dial("")
	if err != nil {
		t.Skipf("no ubus socket available: %v", err)
	}
	defer c.Close()

	id, err := c.Lookup("system")
	if err != nil {
		t.Fatalf("Lookup(system): %v", err)
	}

	res, err := c.Invoke(id, "info")
	if err != nil {
		t.Fatalf("Invoke(system, info): %v", err)
	}
	if _, ok := res["uptime"]; !ok {
		t.Errorf("system info missing uptime, got keys: %v", keys(res))
	}
}

func keys(m map[string]any) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
