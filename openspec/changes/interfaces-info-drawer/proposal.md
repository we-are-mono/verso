# Proposal

## Why

The Interfaces listing is the one table in Verso whose rows unfold: a row opens into its network, its DHCP server and its device in place, under the row. Nothing else behaves like that, so the page reads as more complex than it is. Every other listing opens its row in a drawer from an icon. An Interfaces row is something you read first (addresses, uptime, MAC, link) and edit part by part, so its opener is the info icon, and the drawer holds those parts.

## What Changes

- An Interfaces row opens one drawer from the info icon, which takes the pencil's place among the row's acts: power, restart, info, delete. The row no longer unfolds. **BREAKING** for anyone used to the one-press pencil: editing a part is now the info drawer, then that part's Edit.
- The drawer has one card per part of the row: each network it carries, that network's DHCP server, and the device. Each card's header names the part and carries the act that edits it: Edit network, Edit device, Configure DHCP, or Add DHCP server for a network that has an address of its own and no server.
- The MAC address leaves the table and is shown on the device card, with its copy act. The table keeps Device · network, Type, Address, State and the acts.
- Interfaces and the DHCP page read "this network has a DHCP server" and "this network could take one" by one rule from the SDK's DHCP vocabulary. The row's DHCP chip follows that rule, so an uplink's "do not serve here" section no longer wears a chip.
- The expanded-row capability leaves the SDK and the shell (`TableRow.expanded`, the unfold control, its sheet and its script), because no listing uses it once Interfaces stops.

## Capabilities

### New Capabilities
- `interfaces-page`: the Interfaces listing: its columns, its row acts, and the drawer a row opens, with the cards it holds and the acts on them.

### Modified Capabilities

## Impact

- `plugins/verso-plugin-interfaces/src/page.rs` (row acts, columns, the drawer in place of `details`), `src/main.rs` (the drawer's route), `src/tests.rs`.
- `plugins/verso-plugin-sdk/src/dhcp.rs`: the shared rule for a network that has a server or could take one. `plugins/verso-plugin-dnsdhcp/src/dhcp.rs` reads it in place of its own.
- `plugins/verso-plugin-sdk/src/lib.rs` (`TableRow.expanded` goes), `plugins/verso-plugin-firewall` (three rows that set it empty), and the shell: `internal/widget/table.go`, `templates/table.html.tmpl`, `templates/table-interfaces.html.tmpl`, `assets/verso-tables.js`, `assets/blocks/table.css`.
- DESIGN.md: the "Expanded row" entry gives way to the info drawer; the row-opener rule names Interfaces among the info rows.
- ADR-005 §8 (an object lives in its drawer) is followed. No UCI changes, so no migration ships. A plugin built against an older SDK that still sends `expanded` keeps working, because the shell ignores a field it does not read.
