# Design

## Context

The DHCP page is `plugins/verso-plugin-dnsdhcp/src/dhcp.rs` (its own git repository). Its Servers card is drawn from `verso_plugin::dhcp::servers`, which returns every network with a static address plus every network a `dhcp` section or the live `dnsdhcp.dhcp-state` read names, so a network with no section shows up as an "Off" row and the stock `wan` section shows up as "Client — uplink". The page's one heading act (`HeadingAct`) is "Add reservation" and also carries the blank reservation drawer when the address is `?open=new`. The heading act's `drawer` is the only page-level place a blank object's drawer rides. The Routes page's lane adds and the SDK Files card's lane-head add rely on it. Interfaces reads the same `servers()` for its DHCP chips (filtered on `configured`) and links each server to `?open=<network>`.

`write()` already creates `config dhcp '<network>'` with `option interface` when a network has no section. `validate()` already refuses a pool on a network with no static address.

## Goals / Non-Goals

**Goals:**
- The page's heading act makes servers, the Reservations card makes reservations, and both blank drawers open from their address with or without script.
- A server has a lifecycle on this page: add, edit, switch off, remove.

**Non-Goals:**
- The Interfaces page (its expanded rows and the "Configure DHCP" act) is the next change. Its links keep working as they are.
- No change to what `verso_plugin::dhcp::servers` returns or to the `dhcp-state` read.

## Decisions

**The listing rule lives in the DHCP page, not the SDK.** The page lists a network when it has a `dhcp` section and that network either has a static address or its section is set to hand out addresses. `servers()` stays as it is because Interfaces depends on its shape. The page filters the list it reads; the live state still comes from `servers()`. Alternative: filter in the SDK. Rejected because it changes Interfaces' chips in the same release, and every installed plugin built against the SDK would read a different list.

**`?open=new-server` names the blank server.** UCI section and interface names are `[A-Za-z0-9_]`, so a hyphenated key can never collide with a network or a reservation section. Alternative: `?open=new&kind=server`. Rejected because `new` already means a blank reservation, and the Reserve links carry it with `reserve=<mac>`. `?open=<network>` for a network that can serve and has none resolves to the same blank drawer with the network chosen. The address then means "this network's server" whether or not it exists yet, and `Server::href` needs no change.

**The blank server drawer is the server drawer plus a Network choice.** `server_drawer` takes the network as an `Option`. With none, it leads with a select field `network` (writes `interface`) offering the free networks, titled "New DHCP server", with the same Save ("Save DHCP server"). The default pool is the one a new network gets (`DEFAULT_START`/`DEFAULT_LIMIT`). The save resolves the chosen network, refuses one not offered with a field refusal, then runs the existing `validate()` and `write()` against it. One drawer and one set of checks cover both cases, as Routes does with its blank route.

**The heading act carries only the blank server; the blank reservation rides its own listing.** When the address is `?open=new`, the Reservations table gets one cell-less row whose only content is the open blank drawer. That is how the SDK Files editor answers a panel's address. The shell must draw such a row as its drawer alone, with no empty row in the listing. The first task proves this in the shell with a test, and the shell gets that rule if it lacks it. Alternative: let the heading act carry whichever blank drawer the address names. Rejected because the act is absent when no network is free, which would leave `?open=new` with nowhere to open, and an act's drawer should be the object its label makes.

**"Add reservation" is the Reservations section's `control`: an act-style `Link` with the plus glyph, its words, `panel: true` and `place: true`.** This is the act a part's ledger line already carries (Interfaces' "Configure DHCP"), and DESIGN.md's rule that a plus on a head always has words. A `panel` link opens its drawer as the chrome's, leaving the page's address alone, because it was made for a thing of its own (a package, a certificate). A blank reservation is a place of this page, so `Link` gains `place` (SDK and shell, optional, off by default): the address follows the drawer as a row's and the heading act's do. Every plugin that never sets it renders as before. Alternative: a lane head (`TableGroup` with `add_text`), as on Files and Routes, whose add already moves the address. Rejected because the lane's band would repeat the card's title "Reservations" one line under it.

**Removing a server is a row act with a confirm, staging `commit_delete(dhcp, <section>)`.** It posts under its own name (`_remove_server`), apart from the reservation's `_remove`, so `post()` routes each to its own handler. Crimson is right here because the act erases a section and stops a network being served (confirmation-pattern). The words: "Remove the DHCP server on %s?", "Devices on this network keep their addresses until their leases run out, then get none.", "Keep DHCP server".

**A switched-off server's row is `muted`.** The row sets `muted` when its pool does not serve, so the shell sinks it and adds the "disabled" chip, as every other listing does. The state cell keeps the server's own label.

## Risks / Trade-offs

- [A router whose operator relied on the "Off" row to switch a network's server on finds that row gone.] → "Add DHCP server" offers that network. The page then reads as what exists.
- [Changing how the shell draws a cell-less row could change another table.] → The shell test covers a listing with rows plus a drawer-only row, and the SDK Files editor's drawer-only answer keeps its own test.
- [The plugin then needs a shell that draws a drawer-only row.] → If the shell gains that rule, the plugin's apk dependency floor moves to that shell release, as `build(apk)` commits have done before, and the release notes say to upgrade them together.
- [Removing the server of a network the Interfaces plugin created leaves that network unserved.] → That is the operator's choice and the confirm says so. The network returns to "Add DHCP server".

## Migration Plan

None. Nothing on the router is renamed or moved: a new server is the same `config dhcp '<network>'` section OpenWrt and the Interfaces plugin write, and removing one deletes only that section. Rollback is a downgrade of the plugin; the sections it wrote stay valid for any version.
