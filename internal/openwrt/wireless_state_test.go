// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package openwrt

import (
	"encoding/json"
	"errors"
	"testing"
)

// wirelessFixture is what netifd's `network.wireless status` answers for a
// board with two radios: radio0 up with two access points, radio1 switched
// off, its network configured but carrying no interface.
const wirelessFixture = `{
  "radio0": {"up": true, "disabled": false, "interfaces": [
    {"section": "default_radio0", "ifname": "phy0-ap0"},
    {"section": "iot", "ifname": "phy0-ap1"}
  ]},
  "radio1": {"up": false, "disabled": true, "interfaces": [
    {"section": "default_radio1"}
  ]}
}`

func decodeFixture(t *testing.T, raw string) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// iwinfoFixture answers iwinfo as it does on a live radio: the channel and
// power in use, the stations associated with each interface, and the survey
// the busy share comes from — one entry per frequency, the one in use first
// among others it scanned.
func iwinfoFixture(t *testing.T) func(method, device string) (map[string]any, error) {
	answers := map[string]string{
		"info phy0-ap0":      `{"phy": "phy0", "channel": 6, "frequency": 2437, "txpower": 20, "hardware": {"name": "NXP 88W9098"}}`,
		"info phy0-ap1":      `{"phy": "phy0", "channel": 6, "frequency": 2437, "txpower": 20}`,
		"assoclist phy0-ap0": `{"results": [{"mac": "aa"}, {"mac": "bb"}, {"mac": "cc"}]}`,
		"assoclist phy0-ap1": `{"results": []}`,
		"survey phy0-ap0": `{"results": [
			{"mhz": 2412, "active_time": 100, "busy_time": 90},
			{"mhz": 2437, "active_time": 1000, "busy_time": 612}
		]}`,
	}
	return func(method, device string) (map[string]any, error) {
		raw, ok := answers[method+" "+device]
		if !ok {
			return nil, errors.New("no such answer")
		}
		return decodeFixture(t, raw), nil
	}
}

// TestWirelessStateJoinsRadiosAndNetworksToTheirSections: the read is keyed
// by config section — the names the plugin already holds — so the page never
// has to learn a kernel interface name to put a number on a row.
func TestWirelessStateJoinsRadiosAndNetworksToTheirSections(t *testing.T) {
	state := wirelessState(decodeFixture(t, wirelessFixture), iwinfoFixture(t))
	got, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"networks":{"default_radio0":{"clients":3,"up":true},"default_radio1":{"up":false},"iot":{"clients":0,"up":true}},` +
		`"radios":{"radio0":{"busy":61,"channel":6,"hardware":"NXP 88W9098","txpower":20,"up":true},"radio1":{"up":false}}}`
	if string(got) != want {
		t.Errorf("wireless state\n got %s\nwant %s", got, want)
	}
}

// TestWirelessStateLeavesWhatItCannotReadUnknown: a radio that answers no
// iwinfo still reports whether it is up, and a number that cannot be read is
// absent rather than zero — "no clients" is a fact, "could not ask" is not.
func TestWirelessStateLeavesWhatItCannotReadUnknown(t *testing.T) {
	silent := func(string, string) (map[string]any, error) { return nil, errors.New("iwinfo unavailable") }
	state := wirelessState(decodeFixture(t, wirelessFixture), silent)
	got, _ := json.Marshal(state)
	want := `{"networks":{"default_radio0":{"up":true},"default_radio1":{"up":false},"iot":{"up":true}},` +
		`"radios":{"radio0":{"up":true},"radio1":{"up":false}}}`
	if string(got) != want {
		t.Errorf("unreadable iwinfo\n got %s\nwant %s", got, want)
	}
}
