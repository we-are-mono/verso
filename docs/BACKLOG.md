# Backlog

**This file exists to be deleted.** It is a transient queue of open, deferred work
— nothing here is a durable record. Delete a done item's line outright — do not
mark it done, strike it through, or keep a "done" section; git history is the
record. When the last item is gone, delete the file.

Durable records live elsewhere: decisions in `docs/ADR/`, current behaviour in the
code and `docs/`. This is the one place forward-looking "deferred / next / TODO"
language belongs — keep it out of ADRs and code comments. Each item stands on its
own and points at the decision that governs it; the rationale lives there, not here.

## Certificates and listeners (ADR-017)

- **`verso-plugin-acme`.** The plugin (§6, `plugins/verso-plugin-acme`, a
  repository of its own) issues a real certificate on the DK over DNSimple.
  Open:
  - the repository on GitHub as `we-are-mono/verso-plugin-acme`;
  - on the DK (where `https://<domain>` already opens with no warning, its LAN
    answer kept by AdGuard Home): the LAN record through dnsdhcp's offer, the
    HTTP redirect to the domain, a hand install turning `acme.verso` off, and
    turning it back on serving the held certificate;
  - the drawer refreshing itself while a request runs (a new widget
    behaviour, the owner's call), and a Request now act; acme's outcomes reach
    the top-right notification as its announced events (ADR-028);
- **The Interfaces listing in the shell.** Interfaces are the device's own, as
  its devices are: the shell lists them from netifd, configured ones that are
  down included, and the interfaces plugin contributes its editor and its
  config writes through `network` slots, as dnsdhcp and qos contribute to the
  Devices page. Up, down and restart (`interface-*`, `plugin_commands.go`) are
  the listing's own acts, and the tree table (`table.interfaces`) draws it.
- **VPN tunnel events.** OpenVPN runs its hooks as the profile's user once
  the profile drops privileges, and only root announces (ADR-028). A root-side
  source for the tunnel's state, the kernel's `net` hotplug for its device or
  netifd's `iface` hotplug, lets the VPN plugin announce its tunnel up and
  down, ADR-019's row.
- **Plugin repositories pin a verso tag.** acme, vpn, ddns and dnsdhcp build
  against the SDK in this tree (`path = "../verso-plugin-sdk"`), so each builds
  only beside a Verso checkout. Each pins the first verso tag that carries the
  root dispatcher, the registry types, `Ubus::read` and `FileSet::family`.
- **An image without uhttpd.** The Mono image still ships `uhttpd`,
  `uhttpd-mod-ubus` and `px5g-mbedtls`; drop them from the build (Gateway repo)
  so Verso holds the router's ports from first boot.
- **Check on the DK.** That `/etc/verso` survives an A/B flash (keep.d); and a
  port move from Access in Firefox, which asks about a self-signed certificate
  again on each port.

## Hardening

- **Per-plugin uid.** Plugins share the shell's `verso` uid, so a plugin is not
  isolated from the shell *process* (same-uid signal/ptrace). A dedicated uid per
  plugin isolates them. Governed by ADR-007.
- **Non-core plugins into their own socket directory.** ddns, dnsdhcp, qos, vpn
  and wireless still bind in the shared `/var/run/verso`, where the shell's peer
  check stops a sibling answering as them but not keeping them down during a
  restart. Each repo: init script makes `/var/run/verso-plugin-<id>` (owner the
  plugin, group verso, 0750), sets `VERSO_<ID>_SOCKET` inside it and leaves the
  old shared-dir socket file in place, never `rm -f` (the old shell an upgrade
  has not yet restarted still dials that name, and removing it frees the name
  for a sibling; the system/firewall/interfaces init scripts show the pattern);
  manifest `socket` follows;
  SDK pin bumped to a tag carrying the stale-socket abort. Governed by ADR-007.

## Correctness

- **`list` widget add/remove.** The `list` widget renders a fixed set of rows with
  no client-side add/remove. Blocked on the behavioural-pages item below.
- **Links to an absent plugin, in cells and heading acts.** The shell draws
  no link into a plugin that is not installed where a link stands alone, on a
  section's header or in a callout; a link carried in a table cell or a
  heading's acts still reaches the 404. Governed by ADR-009.
- **A history move on the DK.** Choose another `verso.history.directory`,
  restart the shell, and read that the history arrived and the old copy
  went; the move has run only in tests. Governed by ADR-019.
- **Services: whether a service starts at boot.** ADR-009 gives the shell
  enable and disable; the Services page offers start, stop and restart only
  (`rcActions`), its Enabled switch column gone since 7919a49. A service
  disabled in its own config but still linked in `/etc/rc.d` (banIP with
  `ban_enabled=0`) exits 1 at boot, reads Failed, and heads the Overview, and
  nothing in Verso clears it. The answer is a column holding the control
  vocabulary's switch: off = stop + disable, on = enable + start; firewall
  and the keep-list stay locked. Off also clears a dead instance: banIP's
  init script exits before procd's stop when nothing runs, so its Failed
  instance stays listed until `ubus call service delete`. Governed by
  ADR-009 and ADR-011.
- **Failed leads to an empty log.** A Failed state links to the log filtered
  by the service's name; once logd's ring has turned over, the line saying why
  is gone and the link shows nothing. Governed by ADR-011.

## Validation (ADR-008)

- **WireGuard declares `cidr`.** The vpn plugin can declare `cidr` on an
  interface's `addresses` and a peer's `allowed_ips`; a bare address (which
  wg reads as a /32 in `allowed_ips`) is refused by `cidr`, so that field
  checks its own. Governed by ADR-008.

## Product / definition-of-done

- **Timezone acceptance plugin.** A second plugin written only from
  `docs/plugins.md`, with no shell changes — the "someone else can write one"
  proof.
- **Conformance kit.** An executable check of the plugin contract
  (`docs/plugins.md` carries the checklist).
- **Narrow unit spaces in the plugin repos.** wireless (MHz, dBm, `%`), vpn and
  ddns (durations) and dnsdhcp (lease expiry as "11h 32m") write their own
  figures with a full space or none. A duration or rate goes to the
  shell as `duration` or `rate`; any other unit takes U+202F (DESIGN.md's
  Narrow Space Rule).

## Simplification

- **The interface and system editors on the SDK's fields.** Interfaces'
  `editor.rs` and system's form builders spell their own text and select
  fields over a `BTreeMap` of errors; moving them onto the SDK's `Errors`
  lets them take `text_field`/`select_field` as firewall does.

## Overview cards (ADR-022)

- **Layout on the router, per user.** Keep the card layout next to
  `history.db` per signed-in user, so it follows the operator to another
  browser; a store-backed `layoutSource` replaces `cookieLayout`.
- **Plugin-contributed cards.** A contract for a plugin to add a card to the
  Overview (WireGuard peers, QoS), as the entity panel's tabs have one.

## Router's panel (ADR-005, ADR-022)

- **A connector override in a profile.** A board whose driver reports no fibre
  port and whose device tree links no SFP cage draws its cages as RJ45; a
  `kinds` map in `profile.json` would name them. NXP's DPAA SDK driver is one:
  it reports `MII` for its 10G cages, and the port's node reaches the MAC's
  `sfp` link only through `fsl,fman-mac`.
- **Live readings on System › Hardware.** The panel is live; the
  Temperatures, Power and Cooling cards under it are as the page loaded them.
  The Overview's five-second sensor read (`sensorSampler`) could feed them.
- **Recently seen SFP modules in the Temperatures graph.** The graph's choices
  are the modules plugged in now, so an unplugged module's history goes
  unread; listing the modules seen over the last 30 days would reach it.
- **SFP module identity.** Each module's list gives its optical readings but
  not which module it is: vendor, part number, wavelength and serial live in
  the module's EEPROM (`ethtool -m`), which the shell cannot read unprivileged.
  A `verso-rpcd` reader would head each list with them.
- **Board profiles from packages.** The shell reads profiles from
  `/usr/share/verso/boards/`, so a vendor ships `verso-board-<name>` without a
  Verso release, and panel art is sanitized when it loads (OpenSpec change
  `board-profiles-runtime`).

## Page cards

- **An empty page listing.** VPN and Dynamic DNS with nothing in them stand
  their empty slot loose on the sand rather than in a card.

## Forms as ledgers (forms-ledger)

- **Help written for a tooltip.** The plugins in their own repos (ddns,
  dnsdhcp, qos, vpn, wireless) wrote `help` and `tip` to be read on hover; on
  the page some read long (dnsdhcp's DUID tip runs past 300 characters). Read
  each plugin's forms with descriptions shown and tighten what reads as prose.

## Stylesheet blocks (ADR-025, css-block-layer)

Every look is a block in `internal/server/assets/blocks/`, every text one of
the type roles in `type.css`, and the guards in
`internal/server/stylesheet_guards_test.go` hold both without exceptions.

- **Two looks for a listing in a card.** A page card runs its listing to the
  frame with Paper column heads; a list (`Style: "list"`, the Overview's
  Ports and Tunnels, who is signed in) stands in the card's inset under a fact
  list's heading. Decide whether a card's listing is ever more than a list.
- **System settings without script.** The Logs, Alerts and Packages settings
  stand on cards in their drawers; the pages a browser without script opens
  for them still stand their rows loose on the sand.

## Pending changes (ADR-010, pending-changes-page)

- **The reload step on the DUT.** The When you apply card names what procd's
  own triggers reload. Read it against what procd reloads for a dhcp +
  firewall + network apply on the DUT: stage one change in each, read step 2,
  compare it with `ubus call service list '{"verbose":true}'`, Discard all.
  Governed by ADR-010.

## Themes (ADR-024)

- **The sign-in card under Sand dark.** It stands on Canvas, which is the
  darkest step in Sand dark, so the card reads sunk into the page rather than
  lifted off it. Try it on the device and settle the step it stands on.

## Per-device usage (ADR-018)

- **First Devices visit on the router.** The first history read after the
  shell starts runs `nlbw` once per day of the last 62; time it on the DK.
- **nlbwmon writes in place.** Send a temp-file-and-rename save upstream and
  carry it in the Mono feed; the helper's watchdog covers stock installs.
- **SELinux: label `/data/nlbwmon`** before the policy goes enforcing.
- **Overview usage.** The sentence naming the heavy user and the household's
  month on the Overview, as a card of its own (agreed, not built).
- **Destinations.** What a device talks to needs a source for names that is
  not logs, and a privacy decision.

## Dynamic DNS (`verso-plugin-ddns`)

- **ddns-scripts restarts on an edit.** Its init gives each updater the command
  line `dynamic_dns_updater.sh -S <section>`, which an edit does not change, so
  procd keeps the running updater and Apply takes effect only at the next ifup
  or reboot. `procd_set_param file /etc/config/ddns` in `start_ddns` makes a
  config change restart the instances and leaves an unchanged reload alone
  (verified in the dev container). Send it upstream and carry it in the Mono
  feed.
- **A verdict when the address comes from the internet.** Behind NAT the router
  cannot see its public address, so an `ip_source 'web'` name reads only
  "running". The updater learns it on every check and keeps it only in its log;
  writing it beside `<section>.ip` (as `<section>.current`) lets the plugin's
  `ddns.state` read hand it over. Send with the restart fix, upstream and in the Mono feed.
## Line shaping (SQM)

- **SQM over the whole line (cake).** The QoS plugin caps devices one by one;
  nothing shapes the WAN as a whole. It needs a test device that is not a Mono
  Gateway: there, cake would clash with the hardware QoS (CEETM) and with CMM
  offload, whose flows bypass the qdisc.

## Multi-WAN (mwan3)

- **Failover, policy routing and balancing over mwan3.** A non-core plugin:
  each connection's health from mwan3's tracking, a failover order with "use
  both", and rules ("this traffic leaves through that connection"), with a
  "Send through…" act on a device's panel. It needs a second uplink on a Mono
  Gateway first: mwan3 routes by connection marks and `ip rule`, and whether CMM
  offload keeps honouring them once a flow is offloaded is untested. Failover
  likely survives (new connections route afresh); balancing and policy routing
  may not.

## Tooling

- **Router pull-watcher.** A small procd service on a dev router that polls a
  version stamp on the apk repo and runs the upgrade line in
  `docs/building.md` §5 when it changes, so publishing from the dev box is the
  whole loop and the router updates itself, with no SSH into it. Dev-only;
  ships disabled.
- **Ports in the dev container.** Its veths have no `device` link, so they are
  no physical ports and the router's panel has nothing to draw there; a dev
  box with virtio NICs (an x86_64 VM) would show the generated panel live.

## Research

- **Behavioural pages — the remaining edge.** Three realizations landed in the
  WireGuard plugin under the declare-intent/shell-realizes rule (ADR-005 §7):
  `repeater` (add/remove peers, rpcd round-trip), `conditional` (pre-shared-key
  toggle, pure CSS), and a form `action` (generate keypair, plugin-computed
  round-trip). The remaining edge is a field-set gated on a multi-value `select` (a
  proto switch): as a radio group it stays pure-CSS, but as a dropdown it would be
  the first case needing JavaScript. A candidate next behavioural widget. Governed
  by ADR-005.
