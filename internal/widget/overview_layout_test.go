// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"reflect"
	"testing"
)

func TestOverviewLayoutDefault(t *testing.T) {
	got := DefaultOverviewLayout()
	want := [2][]string{{"internet", "ports", "tunnels"}, {"traffic", "resources", "hardware", "firmware"}}
	if !reflect.DeepEqual(got.Cols, want) {
		t.Fatalf("default columns = %v, want %v", got.Cols, want)
	}
	if len(got.Closed) != 0 || len(got.Hidden) != 0 {
		t.Fatalf("default layout closes or hides cards: %+v", got)
	}
	if got.Encode() != "1.internet-ports-tunnels.traffic-resources-hardware-firmware.." {
		t.Fatalf("default encodes as %q", got.Encode())
	}
}

func TestParseOverviewLayout(t *testing.T) {
	def := DefaultOverviewLayout()
	cases := []struct {
		name, value string
		cols        [2][]string
		closed      []string
		hidden      []string
	}{
		{name: "empty", value: "", cols: def.Cols},
		{name: "valid",
			value:  "1.traffic-internet.ports-tunnels-resources-hardware-firmware.firmware.",
			cols:   [2][]string{{"traffic", "internet"}, {"ports", "tunnels", "resources", "hardware", "firmware"}},
			closed: []string{"firmware"}},
		{name: "wrong version", value: "2.traffic.internet..", cols: def.Cols},
		{name: "garbage", value: "%%%not a layout", cols: def.Cols},
		{name: "too few fields", value: "1.internet-ports", cols: def.Cols},
		{name: "unknown ids are dropped, a card the Overview had before among them",
			value: "1.internet-firewall-wifi-ports-tunnels.traffic-resources-hardware-firmware.wifi-firewall.zones",
			cols:  def.Cols},
		{name: "a duplicate keeps its first place",
			value: "1.internet-ports-tunnels-traffic.traffic-resources-hardware-firmware-internet..",
			cols:  [2][]string{{"internet", "ports", "tunnels", "traffic"}, {"resources", "hardware", "firmware"}}},
		{name: "missing cards are appended open to the shorter column",
			value:  "1.internet-ports-tunnels-traffic.resources.hardware.",
			cols:   [2][]string{{"internet", "ports", "tunnels", "traffic"}, {"resources", "hardware", "firmware"}},
			closed: []string{}},
		{name: "a closed card the columns do not hold is dropped",
			value: "1.internet-ports-tunnels.traffic-resources-hardware.firmware.firmware",
			cols:  [2][]string{{"internet", "ports", "tunnels"}, {"traffic", "resources", "hardware"}}, hidden: []string{"firmware"}},
		{name: "a hidden card leaves its column",
			value: "1.internet-ports-tunnels.traffic-resources-hardware-firmware..resources",
			cols:  [2][]string{{"internet", "ports", "tunnels"}, {"traffic", "hardware", "firmware"}}, hidden: []string{"resources"}},
		{name: "ids outside a-z make the value garbage", value: "1.Internet-ports.traffic..", cols: def.Cols},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ParseOverviewLayout(c.value)
			if !reflect.DeepEqual(got.Cols, c.cols) {
				t.Errorf("columns = %v, want %v", got.Cols, c.cols)
			}
			if len(got.Closed)+len(c.closed) > 0 && !reflect.DeepEqual(got.Closed, c.closed) {
				t.Errorf("closed = %v, want %v", got.Closed, c.closed)
			}
			if len(got.Hidden)+len(c.hidden) > 0 && !reflect.DeepEqual(got.Hidden, c.hidden) {
				t.Errorf("hidden = %v, want %v", got.Hidden, c.hidden)
			}
		})
	}
}

func TestOverviewLayoutRoundTrips(t *testing.T) {
	value := "1.traffic-internet.ports-tunnels-hardware-firmware.internet-firmware.resources"
	if got := ParseOverviewLayout(value).Encode(); got != value {
		t.Fatalf("round trip = %q, want %q", got, value)
	}
}

func TestOverviewLayoutStates(t *testing.T) {
	l := ParseOverviewLayout("1.internet-ports-tunnels.traffic-hardware-firmware.ports.resources")
	if !l.IsClosed("ports") || l.IsClosed("internet") {
		t.Error("IsClosed disagrees with the layout")
	}
	if !l.IsHidden("resources") || l.IsHidden("ports") {
		t.Error("IsHidden disagrees with the layout")
	}
	if !l.Shows("ports") || l.Shows("resources") {
		t.Error("Shows must be true for every card not hidden")
	}
}
