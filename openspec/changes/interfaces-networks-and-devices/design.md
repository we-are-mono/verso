# Design

## Context

The interfaces plugin (`plugins/verso-plugin-interfaces`) draws one table whose
row is a device (`page.rs::rows` maps each network to its L3 device and groups
them), a read-only details drawer at `/details?device=` built from fact
helpers in `page.rs`, and an editor drawer at `/edit?network=&device=` built by
`editor.rs::drawer`. Row acts post `restart|up|down=<network>` to the listing,
which `editor::action` turns into `interface-*` shell commands. The shell
draws the table through a dedicated `interfaces` style
(`table-interfaces.html.tmpl`) that once drew a tree and now draws flush rows.

The DHCP page already shows the target shape: a stack of titled sections, each
holding a table whose rows open drawers, and a card-header add act
(`Widget::Link { panel: true, place: true }`).

## Goals / Non-Goals

**Goals:**
- One object per drawer, opened in one hop, with what the router reports
  above what the operator sets.
- The OpenWrt `wan`/`wan6` pair reads and saves as one internet line, with
  each UCI section still named where its settings stand.
- Only existing widgets: Section, Properties, Table, Form, Link, Button.

**Non-Goals:**
- Making a companion from the drawer (a switch that adds `wan6`). A new
  internet connection's IPv6 stays as it is today.
- Moving the listing to the shell (the BACKLOG item "The Interfaces listing in
  the shell"). This change keeps it in the plugin and does not make that move
  harder.
- Wireless networks: they are the Wi-Fi page's.

## Decisions

### Two tables in two sections, the flat table for both
The page body becomes `Widget::stack([section("Networks", networks_table),
section("Devices", devices_table)])`, the Devices section's `control` holding
Add device. Both tables use the flat table's `live` style, which refreshes
their rows every ten seconds as the old inventory did; the `interfaces`
style, its template and its tests retire. The shared table learns the one
thing only the inventory drew: an act with no glyph holds its slot empty, so
each act keeps its column down the rows.
*Alternative:* one table with two row groups (`TableGroup`, as Routes' lanes).
Rejected: the two kinds have different columns, and a group header reads as a
filter rather than as a second inventory.

### Companion resolution lives in the model, once
`Model::companion(net) -> Option<&Record>` and `Model::companion_of(net) ->
Option<&str>` answer the spec's rule (DHCPv6, device `@net` or the same
resolved device as exactly one non-DHCPv6 network, no contention). The
listing, the drawer, the save, the delete and the commands all ask the model;
none re-derives it. This keeps the rule testable in one place and lets the
shell-side listing reuse the same answer if the listing moves.
*Alternative:* match on the name `wan6`. Rejected: names are the operator's;
`@wan` and same-device pairs are what netifd actually binds.

### The editor drawer is the row's drawer; Status is its first card
`editor::drawer` puts the Status card first among the editor form's cards
for an existing object, and leaves it out of a new one. The Status card is
built by a new
`status.rs` module that takes over the fact helpers from `page.rs`
(`addresses`, `uptime`, `speed`, `device_facts`) and drops the facts the form
already shows (protocol, firewall zone, MTU). `page.rs` keeps only the
listing; `editor.rs` keeps only the form. Each module has one reason to change.

### Status acts post with the editor's form
The Status card's header `control` is an inline stack of two `act` Buttons
named `restart` and `down`/`up` with the network as value. The card stands
inside the editor's form, so each button submits that form to the drawer's
own address. `editor::save` checks for a command first and answers as the
row's act does: the `interface-*` commands for the network and its
companion, the notice, and Back to the listing; nothing is staged. The shell
returns a panel that ran a command to its Back, so the drawer closes on the
live listing, whose State column reads the change as it happens; a drawer
left open would show the state from before the act, since it does not
refresh.
*Alternative:* a field-less form of the card's own. Rejected: a plugin form
always draws a Save button.

### One save, several sections
`editor::operations` already returns a list of `CommitOp`s and the preview
already prints one block per network op. The companion adds one more
`commit("network", companion, …)` with `reqaddress`, `reqprefix` and
`peerdns`; `relationships` takes the companion beside the network, so zone
membership and a rename's references move both, and a rename rewrites the
companion's `device` when it reads `@<old>`. Validation keys the companion's
fields under their own names (`ip6_reqaddress`, …) so a refusal lands on the
right field.

### Add acts split by kind, one route
`KINDS` splits into `NETWORK_KINDS` (Network, Internet connection, Tunnel)
and `DEVICE_KINDS` (Bridge, VLAN). `/new?kind=` stays the one route; the
kind named picks which list the Type select offers, so reshaping never
crosses from a network to a device. Add interface points at `/new`, Add
device at `/new?kind=bridge`.

### Hiding kernel-made devices
A device is listed when the config names it, a network resolves to it, a
bridge or VLAN uses it, or the shell's topology marks it physical
(`physical: true`). Everything else the kernel reports (`sit0`, `ip6tnl0`,
`gre0`, `teql0`, …) is left out. The rule reads facts the model already has;
no name list.

## Risks / Trade-offs

- [The section header renders one control] → Two Buttons in a `stack` must sit
  on the header line. If the header lays the stack out vertically, the fix is
  the header's CSS for a stacked control, not a new widget; if it needs more
  than CSS, the case goes to the owner first.
- [A companion edited outside Verso stops matching the rule] → It reappears
  as its own row, which is the honest reading; nothing is lost.
- [Restarting `wan` also restarts `wan6`] → This is what the operator means
  by restarting the internet line; the spec states it, and the notice names
  both.
- [Existing links to `/details?device=`] → The route stays as an alias of the
  device drawer.
- [Two tables lengthen the page] → Hiding kernel-made devices and companions
  removes more rows than the second header adds on every router seen so far
  (lab: 6 rows → 3 + 4; Zamanifi-style home router loses `wan6` and the
  tunnel bases).

## Migration Plan

Nothing on the router changes: no UCI option is renamed or moved, so no
migration ships. The shell drops the `interfaces` table style; an older
interfaces plugin still asking for it gets the default flat table, so a
mismatched pair keeps working. Both ship in the same release, as they live in
this repo. Rollback is the previous release.
