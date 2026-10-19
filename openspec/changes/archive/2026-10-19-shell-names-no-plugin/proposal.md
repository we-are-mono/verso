# Proposal

## Why

The shell still names particular plugins: their paths, their nav labels, a read and a spelling only one of them uses. So a plugin cannot be renamed, replaced or stopped without the shell going wrong:
- The exposure notice and the Firewall log link to `/plugins/firewall/` while the firewall plugin is stopped.
- The Tunnels card says "Open VPN" while it links to Interfaces.
- A third-party plugin cannot take a designed place in the rail or a word in the apply card.

This is the last part of the plugin contract, after slots (ADR-026), the command registry (ADR-027) and events (ADR-028). The rule: core may know OpenWrt and its own closed vocabularies (sections, entity kinds, slots, its own pages), never a plugin's id, path, nav label or release quirk.

## What Changes

- **A door into a plugin is the plugin's claim.**
  - The shell gains door kinds, named as its change chips already name them: `network`, `zone`, `rule` and `port-forward`, plus `tunnel`. Each has one slot, `open`.
  - A plugin claims `open` in `entity_acts` with the address of one subject (`{id}` in its path).
  - The Overview's Internet, Ports and Tunnels cards, the Access exposure notice and the Firewall log's rule names link only through a running plugin's claim, and draw no link while none claims.
  - A door to a kind leads to the claiming plugin's page that lists it: the exposure notice for a zone opens Zones.
  - A door's word is the claiming plugin's own nav label, in its catalog's translation. With no VPN plugin, the Tunnels card opens Interfaces and says "Open Interfaces".
- **A plugin places itself in the rail.**
  - A nav entry gains `order`, its place among its siblings: among the rail's rows, or among the pages of Network or System.
  - Overview is always first.
  - The shell's own rows and pages carry published numbers, so a plugin can stand before, between or after them.
  - An entry without `order`, or with one that is not a whole number, follows the ordered ones, by section, then label.
  - With today's plugins at their new versions, the rail looks as it does today.
- **A plugin names the services it brings.**
  - The manifest gains `services`, words for the init scripts the plugin brings, translated by its catalog.
  - The apply card's reload step says them: "Dynamic DNS" for `ddns`, and "OpenVPN" for `openvpn`, which today shows raw.
  - The shell keeps words only for base OpenWrt's services and its own.
  - A malformed `order` or `services` entry is logged and dropped on its own; the plugin still loads.
- **Wi-Fi state is the wireless plugin's own read.** Its root half asks netifd and iwinfo. The shell loses `wirelessState`.
- **A rate is split at the narrow space, whatever its unit.** The shell no longer reads verso-plugin-qos's older spelling, and qos writes its rates with U+202F.
- **The helper's clock check comes from the SDK**, not from the System plugin's source tree.
- **A guard test keeps it true.** Core's Go:
  - holds no `/plugins/<id>` path;
  - compares no plugin id with a fixed text, and passes none where an id is expected;
  - compares no nav label with a fixed text.

  The helper compiles in no plugin's source.
- **Not in this change.**
  - Interface up, down and restart stay the shell's acts: the Interfaces listing moves into the shell in a change of its own, and those acts are its own.
  - The tree table (`interfaces` style) stays the shell's.
  - No version floors. An older first-party plugin keeps working beside this shell. Until it is upgraded, it may:
    - stand after the ordered rows;
    - wear its section's glyph (an older VPN plugin);
    - leave its service's name as procd says it;
    - set its rate in one ink.

    An older wireless checkout reads no live state; wireless has no release, so no router holds one.

## Capabilities

### New Capabilities
- `plugin-doors`: how the shell links into a plugin. Covers the `open` claim on a door kind, which plugin answers it, the door's address, page and word, and no door without a live claim.
- `plugin-boundary`: what the shell may know of a plugin. Covers the rule and its guard; a read only a plugin draws being that plugin's own; and a plugin's figures arriving in the shell's unit spelling.

### Modified Capabilities
- `rail`: rows and section pages are ordered by each entry's declared `order` after a fixed Overview, replacing the label-matched design order.
- `overview`: each card's door comes from a plugin's claim, and the Tunnels card falls back to the network door with that door's word.
- `staged-changes`: the reload step's words come from base OpenWrt's services, the shell's own, and the words a plugin declares for the services it brings.

## Impact

- **ADRs, amended in place:**
  - ADR-006: the manifest's `order` and `services`; `entity_acts` gains doors.
  - ADR-009 §2, §3 and §5: plugins place themselves; Overview first; the designed order becomes published numbers.
  - ADR-007 §4: the helper's brokered reads no longer include a plugin's state.
  - ADR-022: a card's door is a claim.
- **Shell:**
  - `internal/server`:
    - `nav.go`: `railOrder`, `navIcon`'s label cases and `navLabelHref` go.
    - `sections.go`: `collapsedSections.Order` goes.
    - `entity.go`: door kinds, claim checks at load, and the door lookup.
    - `server.go`, `exposure.go`, `fwlog.go`, `streams.go`: links come from doors.
    - `apply_steps.go`, `pending.go`: plugin words.
    - `device_limits.go`: generic split.
    - `gateway.go`: `wirelessState` goes.
  - `internal/widget/overview_cards.go` and `overview.go`: a card's door kind replaces its fixed word.
  - `internal/plugin/manifest.go`: `order` and `services`, read per entry.
  - `internal/openwrt`: `WirelessState` and its fakes go.
  - A new guard test in `internal/boundary`; parameters that take a plugin id are named `pluginID`.
- **Helper:** `verso-rpcd` includes `datetime.rs` from the SDK.
- **SDK:** `datetime.rs` moves in. `root::ubus` is a bounded `ubus call` for root halves.
- **Plugins in this tree** (they ship inside `verso`):
  - firewall: claims `zone`, `rule` and `port-forward`.
  - interfaces: claims `network`.
  - interfaces, firewall and system take `order`.
- **Plugin repositories, each committed in its own repo:**
  - wireless: root half, `commands.json`, the registered read and its grant, `deploy-dev.sh` installing the registry, `order`.
  - vpn: claims `tunnel`; `order`; the `lock` glyph; `services` for `openvpn`.
  - qos: U+202F rates.
  - dnsdhcp: `order`.
  - ddns: `order`; `services` for `ddns`.
- **Docs:**
  - `docs/plugins.md` gains `order` (beside `contributes[].order`), `services` and doors, and drops `wirelessState`.
  - `entity_acts` is no longer described as acts that open nothing.
  - `docs/BACKLOG.md` loses the item once this lands and gains "the Interfaces listing in the shell".
- **Compatibility:** a manifest without `order` or `services` loads as before. No floors.
