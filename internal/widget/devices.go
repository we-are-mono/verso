// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package widget

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
	Icon      string // device-type Lucide glyph, resolved deterministically from MAC OUI / hostname (internal/deviceicon)
	MAC       string
	V4        string
	V6        string
	Interface string
	Presence  string

	DUID       string
	Zone       string
	Addresses  []DeviceAddr
	Connection string
	Lease      string
	Traffic    string
	Conns      string

	Leased      bool
	Reserved    bool
	ReserveHref string
}

// DeviceAddr is one row of a device's full address list in the drawer: the
// address, its family ("IPv4"|"IPv6"), and the kernel's confidence in it.
type DeviceAddr struct {
	Addr   string
	Family string
	State  string
}

// DevicesTable is the roster: a device-type icon + name, the interface it sits
// on with its zone chip, the MAC, the IPv4 address, and presence. The row
// carries only the IPv4; each row's Details opens a drawer with the full address
// list (both families) and the rest of the story. This is the unified view (all
// devices, both families, DHCP or not), not a lease dump.
func DevicesTable(devices []Device) *Table {
	rows := make([]TableRow, 0, len(devices))
	for _, d := range devices {
		rows = append(rows, TableRow{
			Cells: []TableCell{
				{Text: d.Name, LeadIcon: d.Icon},
				{Text: d.Interface, Chips: zoneChip(d.Zone)},
				{Text: d.MAC, Copy: true, Emphasis: true},
				{Text: d.V4, Copy: true, Emphasis: true},
				presenceCell(d.Presence),
			},
			Drawer: deviceDrawer(d),
		})
	}
	return &Table{
		Style: "flat", Align: "top", Title: "Connected devices", Detail: countLabel(len(rows), "device"),
		Columns: []TableColumn{
			{Label: "Device", Kind: "name"}, {Label: "Interface", Kind: "reference"},
			{Label: "MAC", Kind: "mono"}, {Label: "IPv4", Kind: "addr"},
			{Label: "Status", Kind: "status"},
		},
		Rows:      rows,
		EmptyText: "Nothing has joined this network yet.",
	}
}

// presenceCell renders a device's presence as a status dot + word: online reads
// emerald, idle amber, offline a quiet grey.
func presenceCell(p string) TableCell {
	switch p {
	case "online":
		return TableCell{Text: "Online", Variant: "success"}
	case "idle":
		return TableCell{Text: "Idle", Variant: "warning"}
	default:
		return TableCell{Text: "Offline", Variant: "neutral"}
	}
}

// deviceDrawer builds the Details panel for one device: its full address list, a
// facts block (connection, DHCP lease, traffic), and — for a device holding an
// address it could keep — the door to the reservation that would keep it.
func deviceDrawer(d Device) *RowDrawer {
	addrRows := make([]TableRow, 0, len(d.Addresses))
	for _, a := range d.Addresses {
		addrRows = append(addrRows, TableRow{Cells: []TableCell{
			{Text: a.Addr, Copy: true, Emphasis: true},
			{Text: a.Family, Variant: family(a.Family)},
			{Text: orDash(a.State), Muted: true},
		}})
	}
	addresses := &Table{
		Style: "flat", Condensed: true,
		Columns: []TableColumn{
			{Label: "Address", Kind: "mono"}, {Label: "Family", Kind: "pill"},
			{Label: "State", Kind: "text"},
		},
		Rows: addrRows,
	}

	items := []Property{
		{Label: "MAC", Value: d.MAC},
		{Label: "DUID", Value: orDash(d.DUID)},
		{Label: "Interface", Value: orDash(d.Interface)},
		{Label: "Zone", Value: orDash(d.Zone)},
		{Label: "Connection", Value: orDash(d.Connection)},
		{Label: "DHCP lease", Value: orDash(d.Lease)},
	}
	// A reservation is a fact about the address, so it reads beside the lease
	// rather than as an action; the address itself is one panel above.
	if d.Reserved {
		items = append(items, Property{Label: "Reservation", Value: "Address reserved for this device"})
	}
	items = append(items,
		Property{Label: "Traffic", Value: orDash(d.Traffic)},
		Property{Label: "Connections", Value: orDash(d.Conns)},
	)

	children := []Widget{addresses, &Properties{Items: items}}
	// The offer only makes sense for a device that holds an address it does not
	// already own: the DHCP page is where an address becomes permanent.
	if d.Leased && !d.Reserved && d.ReserveHref != "" {
		children = append(children, &Link{Label: "Reserve its address", Href: d.ReserveHref, Style: "button"})
	}
	return &RowDrawer{Title: d.Name, Size: "wide", Children: children}
}

// family maps an address family to the pill palette — v6 the calmer tint.
func family(fam string) string {
	if fam == "IPv6" {
		return "info"
	}
	return "neutral"
}
