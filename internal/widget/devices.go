// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

import "strconv"

// The Devices roster: one row per device the box has seen, and behind each row
// the whole story the row has no space for. It is the shell's own page content
// — like the overview, outside the plugin vocabulary and never decoded — built
// from a generic flat Table so the listing looks like every other listing.

// Device is one roster row and its drawer: the device name, its MAC, its primary
// v4/v6 addresses (the stacked cell), the Interface (logical network) it sits on,
// and presence ("online"|"idle"|"offline") — plus the fuller story the Details
// drawer opens: every address, its zone, connection, lease, and traffic.
// Interface ties the row back to the Interfaces listing; Zone rides the drawer,
// not the row (the interface already names the segment).
//
// Leased and Reserved place the device in the DHCP world: a device holding a
// lease it does not own permanently is the one that can be offered a
// reservation, and ReserveHref is where that offer leads.
type Device struct {
	Name      string
	Maker     string // who built it, when the MAC's OUI names someone; empty otherwise, and the row simply omits it
	Icon      string // device-type Lucide glyph, resolved deterministically from MAC OUI / hostname (internal/deviceicon)
	MAC       string
	V4        string
	V6        string
	Interface string
	Presence  string

	// Where the device sits, as the listing groups it: the network's own name
	// and the subnet that name resolves to. Port is the bridge port its MAC was
	// learned on — where it physically attaches.
	Network     string
	NetworkCIDR string
	Port        string
	// ThisBrowser marks the device the page is being read on. It is the one row
	// a person can place without thinking, so the listing says so.
	ThisBrowser bool

	DUID       string
	Zone       string
	Addresses  []DeviceAddr
	Connection string
	Lease      string
	Traffic    string
	Conns      string

	Leased       bool
	Reserved     bool
	Limit        string // configured policy, supplied by the plugin claiming the shape slot
	LimitDetails string // configured days, times and rates for the limits tooltip
	LimitTip     *DeviceLimitTip
}

// DeviceLimitTip is the shell's visual reading of configured device limits.
// It is table-chip content, outside the plugin widget vocabulary.
type DeviceLimitTip struct {
	Title, Hours, From, Until, Qualifier, Clock string
	Days                                        []DeviceLimitDay
	Rates                                       []DeviceLimitRate
}

type DeviceLimitDay struct {
	Label  string
	Active bool
}

type DeviceLimitRate struct {
	Label, Value, Unit, Icon string
}

// DeviceAddr is one row of a device's full address list in the drawer: the
// address, its family ("IPv4"|"IPv6"), and the kernel's confidence in it.
type DeviceAddr struct {
	Addr   string
	Family string
	State  string
}

// DeviceActTitles names the actions available for this device. Reserve and
// remove reservation are mutually exclusive. The shell supplies glyphs and order.
func DeviceActTitles(d Device) map[string]string {
	titles := map[string]string{
		"shape": "Edit limits",
	}
	if d.Reserved {
		titles["unreserve"] = "Remove reservation"
	} else {
		titles["reserve"] = "Reserve an address"
	}
	return titles
}

// DevicesTable is the roster, grouped by the network each device sits on: the
// name and whatever qualifies it, the port it attaches through, its address, its
// MAC, and whether it is here right now. Each row's acts sit at its trailing
// edge, and its Details opens the full story — every address, both families.
// This is the unified view (all devices, both families, DHCP or not), not a
// lease dump.
//
// Devices arrive already ordered by their network; the caller decides that
// order, and every change of network opens a band.
func DevicesTable(devices []Device, acts func(d Device) []TableRowAct) *Table {
	rows := make([]TableRow, 0, len(devices))
	network := ""
	for i, d := range devices {
		row := TableRow{
			// A device that is not here reads at the secondary step, all of it:
			// the values stay exact, the row stops competing for the eye.
			Muted: d.Presence != "online",
			Tags:  deviceTags(d),
			Facet: map[string]string{"network": d.Network},
			Cells: []TableCell{
				{Text: d.Name, Opens: true, Sub: d.Maker, Chips: deviceChips(d)},
				{Text: d.Port},
				{Text: d.V4, Copy: true, Emphasis: true},
				{Text: d.MAC, Copy: true, Emphasis: true},
				presenceCell(d.Presence),
				{Actions: acts(d)},
			},
			// The device's panel is the shell's — every plugin with a say about
			// a device contributes a tab to it — so the row carries the subject's
			// address and nothing else.
			Entity: &EntityRef{Kind: "device", ID: d.MAC},
		}
		if i == 0 || d.Network != network {
			network = d.Network
			row.Group = &TableGroup{
				Key: d.Network, Label: networkLabel(d.Network), Chain: d.NetworkCIDR,
				Tally: networkTally(devices, d.Network),
			}
		}
		rows = append(rows, row)
	}
	return &Table{
		Style: "flat",
		// Every column but the device's own is fixed, so the grid holds its shape
		// whatever this particular network happens to be named and however short
		// one device's address is.
		Columns: []TableColumn{
			{Label: "Device", Kind: "name"},
			{Label: "Port", Kind: "mono", Width: MeasureShort},
			{Label: "Address", Kind: "mono", Width: MeasureAddress},
			{Label: "MAC address", Kind: "mono", Width: MeasureAddress},
			{Label: "Status", Kind: "status", Width: MeasureWord},
			{Kind: "actions", Width: MeasureShort},
		},
		Rows: rows,
		Legend: []TableLegend{
			{Variant: "success", Label: "holding a lease now"},
			{Label: "known, not present"},
		},
		Note:      "Devices with saved limits stay listed when offline. Open a device to edit its limits.",
		EmptyText: "Nothing has joined this network yet.",
	}
}

// deviceTags are the flags the action bar's tabs cut this listing by. They are
// not exclusive on purpose: a reserved device that is not here right now carries
// both "offline" and "reserved", and each tab finds it.
func deviceTags(d Device) []string {
	tags := []string{"offline"}
	if d.Presence == "online" {
		tags = []string{"online"}
	}
	if d.Reserved {
		tags = append(tags, "reserved")
	}
	if d.Limit != "" {
		tags = append(tags, "limited")
	}
	return tags
}

// DevicesBar is the roster's own controls: the cuts a person makes on a list of
// devices — which are here, which are pinned — the search over what is on
// screen, the network to look at, and the one act the page offers.
func DevicesBar(devices []Device, reserveHref string, panelHref func(mac string) string) *ActionBar {
	online, offline, reserved, limited := 0, 0, 0, 0
	for _, d := range devices {
		if d.Presence == "online" {
			online++
		} else {
			offline++
		}
		if d.Reserved {
			reserved++
		}
		if d.Limit != "" {
			limited++
		}
	}
	bar := &ActionBar{
		Tabs: []ActionTab{
			{Label: "All devices", Count: len(devices), Active: true},
			{Label: "Online", Count: online, Match: "online"},
			{Label: "Offline", Count: offline, Match: "offline"},
			{Label: "Reserved", Count: reserved, Match: "reserved"},
			{Label: "With limits", Count: limited, Match: "limited"},
		},
		Select: &ActionPick{Key: "network", Options: []ActionOption{{Label: "All networks"}}},
	}
	seen := map[string]bool{}
	for _, d := range devices {
		if d.Network == "" || seen[d.Network] {
			continue
		}
		seen[d.Network] = true
		bar.Select.Options = append(bar.Select.Options, ActionOption{Label: networkLabel(d.Network), Value: d.Network})
	}
	// The act is offered only where it leads somewhere: with no plugin serving
	// reservations there is nothing to reserve an address with.
	if reserveHref != "" {
		// The href is the fallback a browser with no script follows; with one,
		// the act opens the same panel a row's own reserve icon opens — on the
		// device it would most likely be about, so the panel arrives with that
		// device's own facts pinned rather than as an empty form.
		bar.Action = &TableAction{Label: "Reserve an address", Href: reserveHref}
		bar.Entity = panelHref(reservableDevice(devices))
	}
	return bar
}

// reservableDevice is the device the listing's own act opens on: one that holds
// an address it does not yet own, preferring one that is here right now. It is a
// guess, and a stated one — the panel that opens names the device it is about,
// and every other device is one row away.
//
// Empty when every device is already reserved, and the panel then opens on no
// subject at all: an empty form is the honest answer when there is nothing left
// to keep.
func reservableDevice(devices []Device) string {
	fallback := ""
	for _, d := range devices {
		if d.Reserved || !d.Leased {
			continue
		}
		if d.Presence == "online" {
			return d.MAC
		}
		if fallback == "" {
			fallback = d.MAC
		}
	}
	return fallback
}

// deviceChips are the qualifiers that ride after a device's name: that its
// address is pinned, and that this is the browser you are reading on.
func deviceChips(d Device) []TableChip {
	var chips []TableChip
	if d.Limit != "" {
		detail := d.LimitDetails
		if detail == "" {
			detail = d.Limit
		}
		chips = append(chips, TableChip{Icon: "sliders-horizontal", Label: "limits", Title: detail, Tone: "warning", LocalizeLabel: true, LimitTip: d.LimitTip})
	}
	if d.Reserved {
		chips = append(chips, TableChip{Icon: "pin", Label: "reserved", Tone: "success", LocalizeLabel: true})
	}
	if d.ThisBrowser {
		chips = append(chips, TableChip{Label: "this browser"})
	}
	return chips
}

// networkLabel words a network's own name. The config's handle is the chip
// beside it, so this is the sentence-case reading of the same thing.
func networkLabel(name string) string {
	switch name {
	case "":
		return "Elsewhere"
	case "lan":
		return "Local network"
	}
	return name
}

// networkTally is what a network's band says on its right: how many of its
// devices are here now, and how many are only known.
func networkTally(devices []Device, network string) string {
	online, offline := 0, 0
	for _, d := range devices {
		if d.Network != network {
			continue
		}
		if d.Presence == "online" {
			online++
		} else {
			offline++
		}
	}
	return strconv.Itoa(online) + " online · " + strconv.Itoa(offline) + " offline"
}

// presenceCell renders a device's presence as a dot and the word for it. Only a
// device the kernel confirms right now fills its dot; everything else wears the
// empty ring, which is what the listing's legend explains.
func presenceCell(p string) TableCell {
	if p == "online" {
		return TableCell{Text: "Online", Variant: "success"}
	}
	return TableCell{Text: "Offline"}
}
