# Design

## Context

See proposal.md for why. What exists today:

- **Links into plugins.**
  - `exposure.go:208` links "Open Firewall" to `/plugins/firewall/` whether or not the plugin answers.
  - `fwlog.go:43-44` addresses a logged rule at `/plugins/firewall/?open=<section>` and a redirect at `/plugins/firewall/port-forwards?open=<section>`. The resolver holding them is rebuilt at most once per `fwLogMapTTL`, 30 s (`streams.go:39`, `:184`).
  - The log is drawn only on the firewall plugin's own Activity page. The stream behind it is the shell's.
  - The Overview finds its doors by nav label: `ov.InterfacesHref = s.navLabelHref("Interfaces")` and `ov.TunnelsHref = s.navLabelHref("VPN")` (`server.go:1211`). The cards hold fixed words (`Opens: "Interfaces"`, `"VPN"`, `overview_cards.go:48-50`). `tunnelsHref` falls back to Interfaces while the word stays "VPN".
- **Entity kinds.**
  - `entity_acts` is `{entity, slot, path, post}`.
  - `entitySlots` (`entity.go`) knows one kind, `device`. Every kind in it is a panel route: `entityRef` (`entity.go:379`) admits it, and `entityPanel` assumes a device.
  - `entityActs(kind, id)` lets the last live claimant win. `entityToggle` takes the first. Discovery is sorted by manifest path, so by id.
  - The shell already names config subjects in a closed set, `plugin.SubjectKinds` (`transport.go:302`): `network` (an interface), `rule`, `port-forward`, `zone` and more. Chips use the same names.
- **The rail.**
  - `railOrder` (`nav.go:130`) sorts rows by label: Overview, Devices, Traffic, Journal, Network, Wireless, VPN, Firewall, Routing, Storage, System. Traffic, Journal, Routing and Storage match nothing that exists.
  - `navIcon` (`nav.go:138`) picks glyphs by the labels Wireless, Firewall, VPN, Routing and Storage before falling back to the section's glyph.
  - `collapsedSections` (`sections.go:25`) orders Network's pages by Interfaces, DHCP, DNS, Routes and System's by General, then appends the shell's System pages.
  - Plugin sections that are not core sort after core by title (`sectionLess`). Entries filed under Status get no row.
- **Manifests.**
  - They decode without `DisallowUnknownFields`, so an older shell ignores a field it does not know.
  - A field that fails to decode fails the whole manifest, and `Discover` skips the plugin. `contributes` avoids that by reading each entry on its own (`readContributes`, `contribution.go:156`).
- **The apply card.** `serviceWords` (`apply_steps.go:34`) holds words for base services, plus `ddns`; `openvpn` shows raw.
- **Wi-Fi state.**
  - `coreRead("wirelessState")` (`gateway.go:1353`) calls `NativeBackend.WirelessState` (`openwrt/wireless_state.go`): netifd's `network.wireless status` joined to iwinfo `info`, `assoclist` and `survey`, run with the operator's session.
  - Its only reader is verso-plugin-wireless (`live.rs:15`), which is unpackaged.
  - The wireless repo's rpcd ACL file grants `verso wirelessState`, `network.wireless status` and three iwinfo calls.
  - It deploys itself through its own `deploy-dev.sh`, which `make dev` leaves to it (`scripts/dev.sh:176`) and which installs no `commands.json`.
- **Rates.** `deviceLimitTip` (`device_limits.go:130`) strips `U+202F Mb/s`, ` Mb/s` or ` Mbit/s` and sets the unit to "Mb/s". qos 0.4.0 writes `"{value} Mb/s"` with a plain space (`summaries.rs:77`).
- **The helper's clock check** is `#[path = "../../plugins/verso-plugin-system/src/datetime.rs"]` (`verso-rpcd/src/main.rs:34`). The helper already includes the SDK's `sha256.rs` the same way (`config_watch.rs:35`). The check's tests live in `verso-plugin-system/src/tests.rs` and `verso-rpcd/src/main.rs`.
- **Root halves and ubus.** Root halves call `/bin/ubus` themselves (dnsdhcp `root.rs:39`; the SDK's `signal_shell`). The SDK has no shared helper for it.

## Goals / Non-Goals

**Goals:**
- Every link, word, place and read that belongs to a plugin comes from that plugin's manifest, registry or claim.
- One mechanism per job: doors are claims, places are numbers, words are the plugin's catalog.
- A test that fails the build when core names a plugin again.
- Every build green at every commit, across the core repository and five plugin repositories.

**Non-Goals:**
- **The Interfaces listing in the shell.**
  - Interfaces are the device's own and belong in the shell, with the interfaces plugin contributing its editor and config writes through slots, as the Devices page works. That is a change of its own.
  - Interface up, down and restart stay core acts here: they name no plugin, and they become that listing's own acts.
- **The tree table's `interfaces` style.** It is the shell's widget and draws the listing that change moves into the shell.
- **Version floors.** Of the first-party plugins, only an older wireless loses a function beside this shell, and it has no release (Risks).
- **A door to one network or tunnel.** The cards open listings. The claims carry `{id}` so a door to one subject works the day a page asks for it.
- **dnsdhcp's own `ubus` helper.** It stays in its repo; `root::ubus` is there for it to adopt in its own change.

## Decisions

### 1. Doors are `open` claims on door kinds

**Door kinds.** The shell gains `doorKinds`: `network`, `tunnel`, `zone`, `rule` and `port-forward`, each with one slot, `open`.
- The names are `plugin.SubjectKinds`'s, so a door and a change's chip say the same thing. `tunnel` is a door kind only.
- `doorKinds` is kept apart from `entitySlots`, so a door kind is never a panel route: `entityRef` still admits only panel kinds, and `/entity/zone/x` answers 404.
- The slot carries no glyph: it is a door, never a row icon.

**The claims.** A plugin claims a door in `entity_acts`:
- firewall: `rule` → `/?open={id}`; `port-forward` → `/port-forwards?open={id}`; `zone` → `/zones?open={id}`;
- interfaces: `network` → `/edit?network={id}`;
- vpn: `tunnel` → `/?open={id}`.

**Checks at load.** When the shell loads manifests, it drops each `entity_acts` claim on a kind or slot it does not have, and each `open` claim whose path holds no `{id}`. It logs the plugin and the claim, and the rest of the manifest stands. The `{id}` rule is the door's alone: dnsdhcp's `add` claim (`/?open=new`) holds none.

**One lookup, `s.door(kind) (entityDoor, bool)`.** It returns the live claimant whose id sorts first, its path template, and the nav entry holding that path (`navEntryAt` on the path without its query). A claim under no nav entry gives no door. The door has three methods:
- `To(id)` is the subject's address: the path under `pluginHref`, with `{id}` URL-escaped, through the same URL policy as every plugin link.
- `Listing()` is the most specific page holding the path without its query: an entry or a declared subpage, through the existing `pageHref` (`stage_authors.go:46`).
- `Word(pluginTr)` is the entry's label in the plugin's catalog.

`entityActs` takes the first claimant by id, as `entityToggle` already does. Discovery is id-sorted, so every claim lookup then agrees.

*Alternatives:*
- **A manifest field naming the card an entry serves** (`serves: "tunnels"`). Rejected: a second vocabulary beside the kinds for the same question, "where is this kind of thing opened".
- **New names (`interface`, `firewall-rule`).** Rejected: a third name for things `SubjectKinds` and the chips already call `network` and `rule`.
- **Door kinds in `entitySlots`.** Rejected: every kind there is a panel route that assumes a device.

### 2. The door's word is the claiming plugin's nav label

The word comes from the nav entry the claim's path sits under, so the rail row, the card's door and the exposure notice always say the same name. A renamed page renames its door with no further field.

*Alternatives:*
- **A `label` on the claim.** Rejected: a second name for one page, free to drift from the rail.
- **A word per kind in core** ("Interfaces", "Tunnels"). Rejected: core would name a plugin's page, the same mismatch today's "Open VPN" shows.

### 3. Who draws a door

- **Overview.**
  - `overviewCardSpec` gains `door` (a door kind) for cards that open a plugin: internet and ports `network`; tunnels `tunnel`, falling back to `network`.
  - System and Hardware keep `Opens` and `href` for the shell's own pages.
  - `Overview` carries `Doors map[string]OverviewDoor{Href, Word}`, filled by the server with words already in the plugin's language. The widget formats them with `tr("Open %s")` without translating the word again.
  - `InterfacesHref`, `TunnelsHref` and `navLabelHref` go.
- **Exposure notice.** Its link is the listing door of its first cause's kind: the Zones page for a zone, Rules for a rule, Port forwards for a forward. Its label is `Open %s` with the door's word. With no door, the callout has no link.
- **Firewall log.**
  - `fwLogReader` gains a `door func(kind string) (entityDoor, bool)`, set by the server.
  - `refresh` resolves the `rule` and `port-forward` doors once and hands them to `newFWResolver`, which addresses each rule through `To(section)`.
  - No door means an empty href, which the row already draws as plain text, as it does for `nat`.
  - Liveness is checked when the mapping refreshes, never per row.

### 4. Places in the rail are numbers

**The field.** `NavEntry` gains `Order *int` (`order`), read for each entry on its own beside the main decode, as `contributes` is. Absent and malformed are both "no place": a malformed value is logged and the manifest loads. A value below 1 reads as 1.

**The comparator.** One function, `placeLess`, orders siblings:
1. by `order`, the unordered after the ordered;
2. among the unordered, by section (`sectionLess`);
3. then by English label;
4. then by plugin id.

`buildSidebar` uses it for rows, `sectionPages` for a collapsed section's pages. Entries filed under Status still get no row. Overview is prepended, never sorted.

**The shell's places** are named constants beside the code that draws them:
- rows: Devices 10, the Network group 20, the System group 90;
- System's pages: Hardware 20, Access 30, Packages 40, Services 50, Maintenance 60, Logs 70, Events 80, Changes 90, Diagnostics 100.

A plugin's page under System sorts among the shell's own by the same comparator.

**What goes.** `railOrder`, the label cases of `navIcon`, and `collapsedSections.Order`. `navIcon` becomes the section glyph, and Overview's and Devices' glyphs move onto their row literals. The rows Traffic, Journal, Routing and Storage named in `railOrder` drop with it.

**Docs.** `docs/plugins.md` sets nav `order` beside `contributes[].order`, whose absence means 100, so authors see that the two fields treat a missing value differently.

*Alternative:* **Wireless and VPN as core sections**, so section order alone gives the rail. Rejected by the owner: a plugin's place is the plugin's to choose.

### 5. Plugins declare words for their services

**The field.** `Manifest.Services` (`services`), read on its own like `order`. Each entry's key must be an init-script name (`svcNameOK`) and its value non-empty text, English that is a key of the plugin's catalog. A failing entry is dropped with a log line. A `services` that is not an object is dropped whole, with a log line. Either way the manifest loads.

**The namer.** `reloadNames` takes one:
1. The shell's base word (`serviceWords`, with `ddns` removed) wins.
2. Otherwise the word of the first installed manifest by id that declares the script, translated by that plugin's catalog. Installed, not live: a stopped plugin's service still reloads under its own name.
3. Otherwise procd's name.

`pending.go` builds the namer from `manifestList()` and `pluginTranslators(r)`.

### 6. Wi-Fi state is the wireless plugin's root read

- **Root half.** verso-plugin-wireless gains a root half, `verso-plugin-wireless root`, and `rootfs/usr/share/verso/plugins/wireless/commands.json` with a read `state`, version 1.
- **The read.** It runs `network.wireless status`, then iwinfo `assoclist` for each network with an interface, and `info` and `survey` on each up radio's first answering interface.
- **The SDK's `root::ubus`.** A bounded `/bin/ubus call`: no stdin, output capped at 1 MiB, one JSON object or a failure.
- **Same result as today.** The join is today's `wirelessState`, ported with its test cases, and the result keeps its shape. `live.rs` changes only how it reads: `ubus.read("wireless.state", &[1])`.
- **Manifest and grants.**
  - The manifest's `acl.read` replaces `ubus verso wirelessState` with `verso-plugin wireless state`.
  - The rpcd ACL file grants `"verso-plugin": {"wireless": ["state"]}` in place of `verso wirelessState`, `network.wireless` and `iwinfo`. The shell no longer asks those as the operator.
- **Lab deploy.** The repo's `deploy-dev.sh` installs `commands.json` as root:root 0644. It stays, because it seeds a radio and lifts the hardware guard in the container.
- **Core loses** the `coreRead` case, `NativeBackend.WirelessState`, `wireless_state.go`, the `Backend` method, the fakes, the tests, the `docs/plugins.md` row and ADR-007 §4's mention.

The gate becomes the operator's grant on `wireless.state` (ADR-027 §2). Without it the page gets no entry and shows its "unknown" state, which `Live` already handles.

*Alternative:* **Leave it in core as OpenWrt knowledge.** Rejected: ADR-027 keeps in core only the root work the shell draws or that guards access, and only the Wi-Fi page draws this.

### 7. Figures split at U+202F

`deviceLimitTip` takes `figure, unit, ok := strings.Cut(value, units.Space)`. With no U+202F the value is shown whole. The shell names no unit. qos writes `format!("{value}\u{202f}Mb/s")`.

### 8. The clock check moves into the SDK

`plugins/verso-plugin-sdk/src/datetime.rs`, exported as `verso_plugin::datetime`, takes the check and the System plugin's tests of it. The System plugin imports it. The helper includes it by path beside `sha256.rs` and keeps its own boundary test.

### 9. The guard reads types, not words

Plugin ids are OpenWrt's config names (`wireless`, `firewall`, `system`, `ddns`, `acme`, `qos`), and "Firewall", "DNS" and "DHCP" are core's own words for a service and protocols. Matching words would fail correct code.

The guard is one test file in `internal/boundary`, a package of only `boundary_test.go`:
- it runs `go list -export -deps` once over `./internal/... ./cmd/...`;
- it type-checks each package's non-test files with `go/types` and `importer.ForCompiler(fset, "gc", lookup)`, the export data located by that listing.

This is stdlib only; type-checking `internal/server` this way took 130 ms. It fails, naming the file and line, on:

1. **A path into one plugin:** a string constant starting `/plugins/` with more after it.
2. **A fixed plugin id:**
   - a comparison (`==`, `!=`, a switch case) of a non-empty string constant other than `plugin.ShellID` with `plugin.Manifest.ID` or any struct field named `PluginID` or `Plugin`;
   - a non-empty constant passed to a parameter named `pluginID`.

   Every function that takes a plugin id names that parameter `pluginID`: `pluginHref`, `manifestByID`, the translator `pluginTranslators` returns, and the rest found while renaming.
3. **A fixed nav label:** a comparison of `plugin.NavEntry.Label`, `plugin.PageTab.Label` or the rail's carriers (`navRow.Label`, `pageTab.Label`) with a non-empty constant; or such a label as the key of a map literal's index, or the argument of `slices.Index`/`Contains` over a fixed list.
4. **Plugin source in the helper:** a `#[path = …]`, `include!` or `include_str!` in `verso-rpcd/src` that reaches `plugins/verso-plugin-*` other than the SDK.

Each check is itself tested against fixtures in `internal/boundary/testdata`, one violation each, so a check that silently matches nothing fails too. A clean fixture holds what must pass: the `firewall` config, `m.ID == plugin.ShellID`, `== ""`, a sort by label, and two labels compared with each other.

*Alternatives:*
- **Word lists with an allowlist.** Rejected: the allowlist would hold `firewall`, `system` and `wireless`, the very words it exists to catch.
- **A named type `plugin.ID`.** Rejected for the churn of about 110 sites, while the field- and parameter-based checks find the same comparisons.

## Risks / Trade-offs

- [The Wi-Fi read starts processes on each render: the root half, then one `ubus` per network and two per up radio, about nine on a two-radio, four-network router] → Per render only, never per stream tick (ADR-027 §4). Each process is short and bounded, and the helper's timeout stops the whole run.
- [A Firewall log already open keeps a stopped plugin's links until its mapping refreshes] → At most 30 s; a page load resolves afresh.
- [Third-party plugins choose clashing numbers] → Ties break by label, then id, so the rail is still stable; the published numbers leave gaps of ten.
- [A label passed through a variable to a lookup, as `navLabelHref` did, escapes check 3] → It removes the one such lookup. The checks catch fixed labels and fixed tables, which is how the label lookups being removed here were written.
- [An older first-party plugin beside this shell] → Until upgraded:
  - dnsdhcp and ddns pages stand after Routes;
  - an older vpn row stands after System and wears the default glyph;
  - `ddns` and `openvpn` reload under procd's names;
  - a qos rate shows in one ink;
  - an older wireless checkout reads no live state. Wireless has no release, so no router holds one.
- [The guard needs the Go toolchain at test time] → `make test` always has it; `go list -export` reuses the build cache.

## Migration Plan

Nothing on a router changes: no UCI option, init flag or file under `/data` moves, so no migration ships. Manifests and ACL files are package files and are replaced with their packages.

The tasks run in this order, and every build stays green. An older shell ignores every new manifest field and claim, so a plugin repository's commit that only adds fields may land before or after core's support. Two plugin commits must land before core removes what they replace:
- qos's U+202F spelling (2.1) before core drops the old spellings (2.2);
- wireless's root read (3.1) before core drops `wirelessState` (3.3).

The order:
1. **SDK.** `datetime.rs` moves, and the helper and System include it from there; then `root::ubus`.
2. **Rates.** qos writes U+202F; then core splits generically.
3. **Wi-Fi.** Wireless gains its root half, registry, grant and new read; then core drops `wirelessState`.
4. **Doors.** Core adds the door kinds and the lookup; vpn claims `tunnel` (with its `order`, glyph and service words); core moves the Overview, the exposure notice and the log onto doors, with the firewall's and interfaces' claims.
5. **Places.** Core reads `order` with the in-tree manifests' numbers; then dnsdhcp, ddns and wireless declare theirs.
6. **Service words.** ddns declares its words; core reads `services` and drops `ddns` from its own.
7. **The guard.**
8. **Docs.** ADRs 006, 007, 009 and 022, `docs/plugins.md` and `docs/BACKLOG.md`, each with its group.

Rollback: an older verso ignores every new manifest field, and its own paths and labels still match the plugins' pages. A wireless plugin with the new read keeps working on it, since an older shell brokers registered reads too and the grant is the plugin's own ACL file.
