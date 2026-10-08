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

	// Usage is what the device moves (ADR-018), nil while usage is off.
	Usage *DeviceUsage

	Leased       bool
	Reserved     bool
	Limit        string // configured policy, supplied by the plugin claiming the shape slot
	LimitDetails string // configured days, times and rates for the limits tooltip
	LimitTip     *DeviceLimitTip
}

// DeviceUsage is a device's usage as the roster says it: its rate now in
// Mbit/s, stated only while it is busy enough to read as in use, how hard that
// rate uses the line (Load: "light", "medium" or "heavy"), and its calendar
// month so far.
type DeviceUsage struct {
	Busy     bool
	Down, Up string
	Load     string
	Month    string
}

// loadInks is each load's ink, in the tone vocabulary a figures cell is inked
// with: calm green for light, marigold for medium, crimson for heavy.
var loadInks = map[string]string{"light": "success", "medium": "warning", "heavy": "danger"}

// LoadInk is the tone a load inks its figures in; none for an idle device.
func LoadInk(load string) string { return loadInks[load] }

// UsageNowCell is a device's Now: download then upload, inked by its load. An
// idle device keeps both figures empty, so the cell reads as the dash and a
// live listing has the slots to fill.
func UsageNowCell(u DeviceUsage) TableCell {
	down, up := TableFigure{Icon: "arrow-down", Label: "down"}, TableFigure{Icon: "arrow-up", Label: "up"}
	cell := TableCell{Key: "usage-now", Ink: true}
	if u.Busy {
		down.Text, up.Text = u.Down, u.Up
		cell.Variant = LoadInk(u.Load)
	}
	cell.Figures = []TableFigure{down, up}
	return cell
}

// usageLegend says what each load's ink stands for, in the ink itself, with
// the rates that bound it: the reader learns the scale where the colours are.
func usageLegend() []TableLegend {
	return []TableLegend{
		{Variant: "success", Ink: true, Label: "light", Detail: "under 10 Mbit/s"},
		{Variant: "warning", Ink: true, Label: "medium", Detail: "10 to 100 Mbit/s"},
		{Variant: "danger", Ink: true, Label: "heavy", Detail: "100 Mbit/s and over"},
	}
}

// UsageMonthCell is a device's calendar month so far.
func UsageMonthCell(u DeviceUsage) TableCell {
	return TableCell{Key: "usage-month", Figures: []TableFigure{{Text: u.Month}}}
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
//
// With usage on, the MAC gives its column to what each device moves now and
// this month (ADR-018); the device's panel still names it.
func DevicesTable(devices []Device, acts func(d Device) []TableRowAct, usage bool) *Table {
	rows := make([]TableRow, 0, len(devices))
	network := ""
	for i, d := range devices {
		// A device that is not here reads at the secondary step, all of it:
		// the values stay exact, the row stops competing for the eye. It is
		// muted cell by cell, not as a row, since a muted row is one switched
		// off and absent is not off.
		away := d.Presence != "online"
		cells := []TableCell{
			{Text: d.Name, Opens: true, Sub: d.Maker, Chips: deviceChips(d), Muted: away},
			{Text: d.Port, Muted: away},
			{Text: d.V4, Copy: true, Emphasis: true, Muted: away},
		}
		if usage {
			u := DeviceUsage{}
			if d.Usage != nil {
				u = *d.Usage
			}
			month := UsageMonthCell(u)
			month.Muted = away
			cells = append(cells, UsageNowCell(u), month, presenceCell(d.Presence))
		} else {
			cells = append(cells, TableCell{Text: d.MAC, Copy: true, Emphasis: true, Muted: away}, presenceCell(d.Presence))
		}
		row := TableRow{
			ID:    d.MAC,
			Cells: append(cells, TableCell{Actions: acts(d)}),
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
	legend := []TableLegend{
		{Variant: "success", Label: "holding a lease now"},
		{Label: "known, not present"},
	}
	if usage {
		legend = append(legend, usageLegend()...)
	}
	return &Table{
		Style: "flat",
		// Every column but the device's own is fixed, so the grid holds its shape
		// whatever this particular network happens to be named and however short
		// one device's address is.
		Columns:   devicesColumns(usage),
		Rows:      rows,
		Legend:    legend,
		Note:      "Devices with saved limits stay listed when offline. Open a device to edit its limits.",
		EmptyText: "Nothing has joined this network yet.",
	}
}

// devicesColumns is the roster's grid. Every column but the device's own is
// fixed; with usage on, the MAC's measure goes to Now and This month.
func devicesColumns(usage bool) []TableColumn {
	cols := []TableColumn{
		{Label: "Device", Kind: "name"},
		{Label: "Port", Kind: "mono", Width: MeasureShort},
		{Label: "Address", Kind: "mono", Width: MeasureAddress},
	}
	if usage {
		cols = append(cols,
			TableColumn{Label: "Now · Mbit/s", Kind: "figures", Width: MeasureAddress},
			TableColumn{Label: "This month", Kind: "figures", Width: MeasureWord},
			TableColumn{Label: "Status", Kind: "status", Width: MeasureWord},
		)
	} else {
		cols = append(cols,
			TableColumn{Label: "MAC address", Kind: "mono", Width: MeasureAddress},
			TableColumn{Label: "Status", Kind: "status", Width: MeasureWord},
		)
	}
	return append(cols, TableColumn{Kind: "actions", Width: MeasureShort})
}

// DevicesAct is the roster's act on its heading line. It is offered only where
// it leads somewhere: with no plugin serving reservations there is nothing to
// reserve an address with, and nil is returned. The href is the fallback a
// browser with no script follows; with one, the act opens the same panel a
// row's own reserve icon opens — on the device it would most likely be about,
// so the panel arrives with that device's own facts pinned rather than as an
// empty form.
func DevicesAct(devices []Device, reserveHref string, panelHref func(mac string) string) *ActionBar {
	if reserveHref == "" {
		return nil
	}
	return &ActionBar{
		Heading: true,
		Action:  &TableAction{Label: "Reserve an address", Href: reserveHref},
		Entity:  panelHref(reservableDevice(devices)),
	}
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
