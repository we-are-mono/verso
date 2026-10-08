// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import (
	"strings"
	"testing"
)

// roster is the listing in miniature: a leased device that could be reserved, a
// device whose address is already reserved, and one the box has only seen.
func roster() []Device {
	return []Device{
		{
			Name: "Gaming PC", Maker: "Intel", Icon: "gamepad", MAC: "a4:83:e7:2b:19:0c",
			DUID: "00:03:00:01:a4:83:e7:2b:19:0c",
			V4:   "192.168.1.104", V6: "2001:db8::4f",
			Interface: "br-lan", Zone: "lan", Presence: "online",
			Network: "lan", NetworkCIDR: "192.168.1.0/24", Port: "lan1", ThisBrowser: true,
			Addresses: []DeviceAddr{
				{Addr: "192.168.1.104", Family: "IPv4", State: "reachable"},
				{Addr: "2001:db8::4f", Family: "IPv6", State: "reachable"},
			},
			Connection: "lan1", Lease: "in 11h 12m",
			Leased: true, // ReserveHref: "/plugins/dnsdhcp/?reserve=a4%3A83%3Ae7%3A2b%3A19%3A0c",
		},
		{
			Name: "nas", MAC: "de:ad:be:ef:00:11", V4: "192.168.1.10",
			Interface: "br-lan", Zone: "lan", Presence: "idle",
			Network: "lan", NetworkCIDR: "192.168.1.0/24", Port: "lan3",
			Addresses: []DeviceAddr{{Addr: "192.168.1.10", Family: "IPv4", State: "stale"}},
			Lease:     "in 9h 02m", Leased: true, Reserved: true,
		},
		{
			Name: "Unknown device", MAC: "9e:2f:11:c4:08:5b", V4: "192.168.20.44",
			Interface: "br-lan.20", Zone: "guest", Presence: "offline",
			Network: "guest", NetworkCIDR: "192.168.20.0/24",
			Addresses: []DeviceAddr{{Addr: "192.168.20.44", Family: "IPv4", State: "stale"}},
			Lease:     "No DHCP lease",
		},
	}
}

// testActs stands in for the shell's slot vocabulary: one reservation action
// followed by limits, with only the reserve tab claimed in this fixture.
func testActs(d Device) []TableRowAct {
	titles := DeviceActTitles(d)
	var acts []TableRowAct
	if d.Reserved {
		acts = append(acts, TableRowAct{Icon: "pin-off", Title: titles["unreserve"]})
	} else {
		acts = append(acts, TableRowAct{Icon: "pin", Title: titles["reserve"], Opens: true})
	}
	return append(acts, TableRowAct{Icon: "sliders-horizontal", Title: titles["shape"]})
}

// TestRenderDevicesTable: the roster is one listing banded by network — name
// and maker, the port it attaches through, address, MAC, whether it is here —
// with its acts at the trailing edge and the full story behind each row.
func TestRenderDevicesTable(t *testing.T) {
	got := render(t, newRenderer(t), DevicesTable(roster(), testActs, false))
	for _, want := range []string{
		"Local network", "192.168.1.0/24", "1 online · 1 offline", // the lan band, and what it amounts to
		"guest", "192.168.20.0/24", "0 online · 1 offline", // the guest band
		"a4:83:e7:2b:19:0c", "192.168.1.104",
		"lan1", "lan3", // the port a device attaches through
		"Intel",                    // the maker, beside the name it qualifies
		"reserved", "this browser", // the chips that qualify a name
		"Online", "Offline",
		"Reserve an address",                        // the act that leads somewhere
		"Edit limits",                               // one that is drawn and inert
		"holding a lease now", "known, not present", // the legend for the marks
		"Devices with saved limits stay listed when offline. Open a device to edit its limits.",
		`data-verso-entity-url="/entity/device/a4:83:e7:2b:19:0c"`, // the row points at the shell's panel
	} {
		if !strings.Contains(got, want) {
			t.Errorf("devices table missing %q", want)
		}
	}
	// A device that is not here reads at the secondary step, all of it.
	if !strings.Contains(got, "text-meta") {
		t.Error("an absent device's row should read muted")
	}
	// Absent is not switched off: the marker belongs to a subject not in force.
	if strings.Contains(got, "data-verso-off") {
		t.Error("an absent device's name should not be marked over")
	}
	// Idle is not a third state in this listing: the kernel confirms a device
	// now, or it does not.
	if strings.Contains(got, ">Idle<") {
		t.Error("the roster states two presences, not three")
	}
}

// TestRenderDevicesRowsCarryNoPanel: a device's detail is the shell's panel,
// assembled from every plugin with a say about a device — so the listing ships
// each row's address and none of its contents. A page of thirty devices that
// carried them would ask every contributing plugin thirty times before anyone
// clicked anything.
func TestRenderDevicesRowsCarryNoPanel(t *testing.T) {
	got := render(t, newRenderer(t), DevicesTable(roster(), testActs, false))
	for _, unwanted := range []string{
		"2001:db8::4f",                // an address only the panel shows
		"in 11h 12m", "No DHCP lease", // lease facts
		"00:03:00:01:a4:83:e7:2b:19:0c", // the DUID
		"Reserve its address",           // the old inline drawer's offer
	} {
		if strings.Contains(got, unwanted) {
			t.Errorf("the listing ships %q, which belongs to the panel", unwanted)
		}
	}
	if n := strings.Count(got, `data-verso-entity-url="/entity/device/`); n != 3 {
		t.Errorf("every row should address its own panel, got %d:\n%s", n, got)
	}
}

// TestRenderDevicesUsage: with usage on, the roster says what each device moves
// (ADR-018). Now is the rate down and up, each figure over a meter of its share
// of the WAN, drawn only on a busy row; an idle row is the empty dash, its
// figures kept in place for the stream to fill. This month is the calendar
// month's total alone: a month has no ceiling to measure against, so a meter
// under it would only restate the number. The MAC gives up its column to them;
// the panel carries it.
func TestRenderDevicesUsage(t *testing.T) {
	devices := roster()
	devices[0].Usage = &DeviceUsage{Busy: true, Down: "38.2", Up: "1.1", DownFill: 76, UpFill: 2, Month: "212 GiB"}
	devices[1].Usage = &DeviceUsage{Month: "41 GiB"}
	devices[2].Usage = &DeviceUsage{}
	got := render(t, newRenderer(t), DevicesTable(devices, testActs, true))
	for _, want := range []string{
		">Now · Mbit/s<", ">This month<",
		`data-verso-cell="usage-now"`, `data-verso-cell="usage-month"`,
		">38.2<", ">1.1<", ">212 GiB<", ">41 GiB<",
		"clip-path: inset(0 24% 0 0)", "clip-path: inset(0 98% 0 0)",
		"bg-green", "bg-amethyst",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("usage roster missing %q", want)
		}
	}
	at := strings.Index(got, `data-verso-cell="usage-month"`)
	month := got[at : at+strings.Index(got[at:], "</td>")]
	if strings.Contains(month, "data-verso-figure-meter") {
		t.Errorf("a month has nothing to measure against and draws no meter:\n%s", month)
	}
	if strings.Contains(got, ">MAC address<") || strings.Contains(got, "a4:83:e7:2b:19:0c</") {
		t.Error("the MAC gives up its column to usage")
	}
	// The idle and the offline rows' Now is the dash, their figures hidden.
	if n := strings.Count(got, "data-verso-figures-none hidden"); n != 3 {
		t.Errorf("rows with something to state should hide the dash: %d, want 3 (one busy Now, two months)", n)
	}
	// An empty slot keeps its width, unseen, so a column of them holds still
	// as the stream fills and empties them: the idle and offline rows' two Now
	// slots and the offline row's month.
	if n := strings.Count(got, ` invisible" data-verso-figure`); n != 5 {
		t.Errorf("empty figure slots kept in place: %d, want 5", n)
	}
	if strings.Contains(got, `data-verso-figure="emerald" hidden`) || strings.Contains(got, `data-verso-figure="" hidden`) {
		t.Error("an empty figure slot was taken out of the layout")
	}

	off := render(t, newRenderer(t), DevicesTable(roster(), testActs, false))
	if strings.Contains(off, "usage-now") || !strings.Contains(off, ">MAC address<") {
		t.Error("with usage off the roster keeps its MAC column and draws no usage")
	}
}

// TestRenderDevicesActsFollowThePanelsTabs: a row's icons read in the order of
// the tabs its panel opens on — the shell's Details first, then each plugin's
// slot — so the door to Details leads the row's acts rather than trailing them.
func TestRenderDevicesActsFollowThePanelsTabs(t *testing.T) {
	got := render(t, newRenderer(t), DevicesTable(roster(), testActs, true))
	row := got[strings.Index(got, `data-verso-row-id="de:ad:be:ef:00:11"`):]
	details, unreserve, limits := strings.Index(row, `aria-label="Details nas"`), strings.Index(row, `aria-label="Remove reservation"`), strings.Index(row, `aria-label="Edit limits"`)
	if details < 0 || unreserve < 0 || limits < 0 || !(details < unreserve && unreserve < limits) {
		t.Errorf("acts should read Details, then the reservation, then limits: details=%d unreserve=%d limits=%d", details, unreserve, limits)
	}
}

// TestRenderDevicesTableEmpty: a roster with nobody on it says so in its own
// words rather than drawing column headings over nothing.
func TestRenderDevicesTableEmpty(t *testing.T) {
	got := render(t, newRenderer(t), DevicesTable(nil, testActs, false))
	if !strings.Contains(got, "Nothing has joined this network yet.") {
		t.Errorf("empty roster missing its own empty text:\n%s", got)
	}
	if strings.Contains(got, "<thead") {
		t.Errorf("empty roster should draw no column headings:\n%s", got)
	}
}
