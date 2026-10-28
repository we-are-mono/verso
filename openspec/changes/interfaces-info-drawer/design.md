# Design

## Context

`plugins/verso-plugin-interfaces/src/page.rs` builds the listing. Each row carries `expanded: details(...)`: a flush section holding one `part()` per uci section (network, its `dhcp_part`, the device), each a section with a glyph, a kind, a name as `meta` and an edit act as `control`, over a two-column `facts` grid. The row's acts are four slots, POWER, RESTART, EDIT and DELETE. EDIT's href is the editor's address (`/edit?network=&device=`), which is also the row's `panel`, so the pencil fetches the editor drawer. The editor answers its address with `page::with_drawer`, which shows the listing with the drawer on the heading act. The MAC is a table column. The DHCP chip stands for any `dhcp::Server` with `configured`, which includes an uplink's `ignore 1` section.

The DHCP page decides "has a server" and "could take one" in its own `State::has_server` and `State::free`. The SDK's `dhcp::servers` lists every static network, every network with a section, and every live one.

The shell supports unfolding rows only for this listing: `TableRow.expanded` (SDK and shell), `Expandable` cells, the `data-verso-expand-row` control and `versoExpandRow` in `verso-tables.js`, the `__expand` and `__sheet` elements in `blocks/table.css`, and their tests. Firewall's rows set `expanded: Vec::new()`.

## Goals / Non-Goals

**Goals:**
- One opener per row (the info act), one drawer, cards built from the parts the row already reads.
- One meaning of a DHCP server on both pages.
- No unfolding rows anywhere, in the shell or the SDK.

**Non-Goals:**
- Moving the Interfaces listing into the shell. That stays its own backlog item; the `interfaces` table style stays the shell's.
- Changing the editor drawer, the row's power, restart and delete acts, or what the facts say.

## Decisions

**The drawer is a page of its own, `/details?device=<name>`, keyed by the row's device.** A row is a device with the networks it carries, so the device is its identity, as the row's `id` already is. `get` routes `/details` to `page::details(m, device)`, which builds the same row data the listing reads and answers `with_drawer(m, drawer)`, so the address opens the listing with the drawer in front and a panel fetch is answered from the same tree. The row's `panel` and its info act's href are this address. Alternative: reuse the editor's `network=&device=` pair. Rejected because the drawer is about the whole row, and a row of several networks has no one network to name.

**Each part becomes a top-level section of the drawer, so the shell draws it as a card.** `part()` already sets the glyph, kind, name and control; dropping `details()`'s flush frame lets each part stand as a drawer card (drawers-are-cards). Facts become a single fact list per card (`Widget::properties`), the list every drawer and card uses, instead of the expanded row's two-column grid. The device card gains `MAC address` (verbatim, with copy) and is always present, where the expanded row added it only for a configured device section.

**A card's edit act is a plain act link to the part's editor address.** A link from inside a drawer to another drawer's address loads the listing with the editor in front, as every editor address does. The editor closes onto the listing. Alternative: open the editor as a second panel over the first, or swap it in like a drawer tab. Rejected: stacked drawers have no precedent, and a tab strip would make the editor a tab of the info drawer, which is not what the row offers. The acts are the ones the row reaches today: Edit network (`/edit?network=<net>`), Edit device (`/edit?device=<name>`, which answers an OpenVPN-made device with its handoff), and Configure DHCP or Add DHCP server (`Server::href`).

**The SDK's DHCP vocabulary owns the rule.** `dhcp::Server` gains what the rule needs: whether the network has a section, whether it has an address of its own (proto `static` with an `ipaddr`), and whether its section hands out addresses. Two methods state it: `exists()` and `can_be_added()`. The DHCP page's `has_server` and `free` read them, and so do the Interfaces chip and card. Alternative: copy the rule into Interfaces. Rejected because two copies drift, and the dhcp module exists so "a server has the same meaning on either page".

**The expanded-row capability is removed, not left unused.** Nothing else unfolds, and a closed widget set keeps only what pages use. The SDK field goes. The shell drops its decoding, rendering, script and CSS. The `interfaces` style keeps its nesting by depth and its chips; its tree lines and node marks went too, after the owner tried the page, because the order and a 24px indent a level say what carries what. A plugin built against an older SDK still serialises `expanded`; the shell's decoder ignores unknown fields, so it renders the row without the unfold.

## Risks / Trade-offs

- [Editing takes two presses where it took one.] → The decision is the owner's (one opener per row), and each card puts its edit act where the part's facts are read.
- [A device card now shows on every drawer, including a device with no configured section and nothing to edit.] → The card states facts and carries no act when the row's pencil had none (an unconfigured, unreported device).
- [Removing `expanded` from the SDK breaks a plugin that sets it in source.] → Only Interfaces and Firewall (both in this tree) set it; the plugin repos do not. The release notes name the removal.
- [The shell's `PLUGIN_FLOORS` / dnsdhcp dependency.] → dnsdhcp's switch to the SDK rule changes no output, so it needs no new floor; Interfaces ships with the shell.

## Migration Plan

None on the router: nothing in UCI moves. The shell and Interfaces ship together. An older dnsdhcp keeps its own rule and keeps working.
