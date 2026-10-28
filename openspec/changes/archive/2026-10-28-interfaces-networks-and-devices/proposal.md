# Proposal

## Why

An operator who wants to change the LAN's address opens a row, reads a drawer
that mostly repeats the editor, presses Edit network, and lands in a second
drawer, having first had to learn that OpenWrt splits an interface from its
device. Rows are devices, so a device carrying two networks (`wan0` with `wan`
and `wan6`) has acts that reach only the first: `wan6` cannot be restarted,
brought down or deleted from Interfaces. Kernel tunnel bases (`sit0`,
`ip6tnl0`) take rows nobody manages. OPNsense, the reference the owner
named, makes the logical interface the object, keeps devices in their own
inventory, and folds IPv6 into the interface it belongs to.

## What Changes

- The Interfaces page holds two cards: **Networks**, one row per network
  (UCI `interface`), and **Devices**, one row per device (ports, bridges,
  VLANs). **BREAKING** for the device-first row: a row no longer
  stands for a device with the networks it carries.
- Each row opens **one drawer**, from its pencil, that is the object's editor
  with a live **Status** card on top. The read-only details drawer is removed;
  the editor's address is the row's, and `/details?device=` opens the
  device's drawer.
- A network's drawer: Status (Restart and Bring down on its header), General
  (Bring up at boot, Name, Runs on, Firewall zone), IPv4, IPv6, Advanced, the
  `/etc/config/network` preview, Save.
- An **IPv6 companion** (a DHCPv6 network on the same device as another
  network, such as `wan6` beside `wan`) has no row of its own: it is the
  IPv6 card of its network's drawer, the card's header naming `wan6`. One
  Save stages both sections; a row's power, restart and delete act on both.
- A device's drawer: Status (MAC, link, driver, errors), then its kind's
  settings (a bridge's ports, STP and IGMP snooping; a VLAN's parent, ID and
  tag protocol), MTU and MAC override.
- A software device the kernel made on its own, which no section names, which
  carries no network and which nothing else uses, has no row.
- **Add interface** on the page's heading makes a network, an internet
  connection or a tunnel (a tunnel is a UCI `interface`); **Add device** on
  the Devices card's header makes a bridge or a VLAN.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `interfaces-page`: rows split into a Networks and a Devices card; the
  details drawer is replaced by an editor drawer with a Status card; an IPv6
  companion folds into its network's drawer and row; kernel-made devices
  without a use have no row; the add acts split by kind.

## Impact

- ADR-005 §8 ("An object lives in its drawer") is what this change applies:
  one object per drawer, read and edited there. No ADR is overturned.
- `plugins/verso-plugin-interfaces/src/page.rs` (listing, rows, Status cards),
  `editor.rs` (drawer sections, IPv6 companion, add acts by kind),
  `model.rs` (companion resolution), `main.rs` (`/details` becomes an alias
  of the device drawer; `/edit` takes Status card commands), `tests.rs`.
- The shell's `interfaces` table style retires: `internal/widget/table.go`,
  `templates/table-interfaces.html.tmpl`, `table_interfaces_test.go`. Both
  cards use the one flat table.
- Docs: `DESIGN.md` (the interface drawer paragraph), `docs/plugins.md` (the
  Interfaces inventory section), `docs/BACKLOG.md`.
- No UCI option is renamed or moved; nothing on a router needs migrating. An
  old `/plugins/interfaces/details?device=<name>` link opens that device's
  drawer.
