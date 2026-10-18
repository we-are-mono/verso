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

- **Step 4: `verso-plugin-acme`.** A trusted certificate for a domain the
  operator controls, by DNS-01 only, over OpenWrt's `acme` package (§6):
  - a non-core plugin in a repository of its own, `we-are-mono/verso-plugin-acme`,
    laid out like `verso-plugin-dnsdhcp` (SDK pinned by a verso tag, its own
    Makefile and apk), added to `NONCORE_PLUGINS` in `local.mk`
  - a Rust plugin, its own apk depending on `verso`, `acme-acmesh` and
    `acme-acmesh-dnsapi`, owning `/etc/config/acme`; Verso's certificate is the
    `config cert 'verso'` section;
  - its part of Access beside Certificate (`system_access`): the domain, a
    curated set of DNS providers with named credential fields, and **Other**
    for the raw `credentials` list; credentials are write-only in the UI;
  - an offer to answer the domain on the LAN with the router's address, as a
    `config domain` in `/etc/config/dhcp` (dnsmasq's rebind protection refuses
    a public record pointing at a private address);
  - delivery from `/etc/hotplug.d/acme/50-verso` on `issued`/`renewed`. The hook
    runs as root without a session, and the helper's zero-session grant is two
    reads (ADR-014), so it needs a root-local way in — e.g. a
    `verso certificate install` command — rather than `setWebCertificate`;
  - installing or making a certificate by hand sets `acme.verso.enabled` to 0,
    said in the Install and Make drawers before confirming while it is on.
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
- **A deadline on bodies before sign-in.** The listeners bound a connection's
  handshake, headers and idle wait, but not a body: a client that has not
  signed in can POST `/login` and trickle its body to hold a connection until
  it ends, inside the per-source cap. A read deadline set through
  `http.ResponseController` on bodies of requests without a session would close
  that. Governed by ADR-017 §1.
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
- **One SO_PEERCRED reader.** `internal/openwrt/helper_peer_*.go`
  (`socketPeerUID`) duplicates `internal/peercred.UID`; point the helper client
  at the shared package.
- **The shell masks a spooled reading itself.** `takeReading` keeps a config
  reading as verso-rpcd spooled it, trusting the helper to have fingerprinted
  every secret the shell knows. The two read manifests alike for every case in
  `internal/server/testdata/plugin-secrets.json`, but serde_json drops a whole
  manifest Go reads (a lone surrogate escape such as `"\ud800"`, nesting deeper
  than 128), so its plugin's secrets reach the
  shell in plaintext until the next scrub. Applying `history.WithoutSecrets`
  with `secretOptions()` to each section on intake closes it whatever the
  helper read. Governed by ADR-021.
- **The remaining shell-held readings under a grant.** The overview's past
  Internet graph (`pastTraffic`: history.db's interface traffic, gateway loss
  and latency, and its outage and boot marks) and the online-device count
  (`onlineDevices`, the neighbour table) still reach every signed-in session.
  Gate them as ADR-007 §9 gates the roster and the events page: the traffic
  and marks under `ubus network.interface dump`, the count under `ubus dhcp
  ipv6leases`. Governed by ADR-007.
- **A sign-in file saved as `.conf` reads verbatim.** `openvpnFiles` lists
  every `.ovpn` and `.conf` directly in `/etc/openvpn` as a profile and folds
  only inline blocks, so an `auth-user-pass` file named `*.conf` there (a
  username and password, no blocks) reaches the page as written. Listing only
  files an `openvpn` section names as its `config` (or skipping any that one
  names as its `auth_user_pass`/`askpass`) would close it.
- **A key line put back as a block's opener.** `unfold` (verso-rpcd
  `openvpn.rs`) refuses a save that puts a block's line back where more of
  it leaves the helper, but not one where a line hidden in the original
  opens a block in the save: it stays hidden, yet its `<name>` becomes the
  new block's placeholder, shown on the next fold. Only a hidden line
  shaped `<name>` does so (no key's base64 is), behind a contrived `</X>`.
  Refusing a restored line that opens a block in the save but did not in
  the original would close it (docs/plugins.md, `openvpnFiles`).

## Correctness

- **An OpenVPN that drops privileges never gets a trusted record.** OpenVPN
  runs `up` (and `route-up` unless routes are delayed) as root, so root writes
  the record in `/var/run/verso-openvpn`; the hooks after run as the profile's
  `user`, which cannot rename over root's file in the 1733 directory, and
  `read_record` (verso-rpcd `openvpn.rs`) trusts only a record owned by the
  process's effective uid, so such an instance always falls back to the
  kernel's view (no `since`, no `server`). Accepting a record owned by root or
  by the instance's uid would close it; the hook then needs to carry over a
  root-owned record when running as the instance's user.
- **Clear-all NTP servers.** Whether an empty list in a brokered `uci.set` clears
  the option or leaves it stale is unverified (`internal/openwrt`).
- **`list` widget add/remove.** The `list` widget renders a fixed set of rows with
  no client-side add/remove. Blocked on the behavioural-pages item below.
- **Links to an absent plugin.** Interfaces links each network's DHCP server to
  `/plugins/dnsdhcp/?open=<network>` whether or not `verso-plugin-dnsdhcp` is
  installed; without it the link is a 404. Governed by ADR-009.
- **Unanswered grants on Access and usage.** Where rpcd does not answer
  `session list`, Access lists only the account's own sessions without saying
  why; where it does not answer `verso usageDays`, the roster and a device's
  panel drop usage silently. The roster, device panel, Changes and Events say
  "couldn't be checked" (`sessionGrants.answer` / `anyUnchecked`); these two
  could too. Governed by ADR-007 §9.
- **Moving the history directory.** A change of `verso.history.directory`
  moves nothing (ADR-019): the history in the directory chosen before stays
  behind, and a factory reset empties only the defaults and the directories
  chosen at the time (ADR-021). Copying the database with `VACUUM INTO` on a
  change would carry it over. Governed by ADR-019.
- **A factory reset's walk, by mount id and without depth.** verso-rpcd's
  `reset.rs` tells a filesystem mounted inside a default by its device or by
  its path in `/proc/self/mountinfo`, read once before the walk; a bind mount
  of the same device made or moved after that read is walked into. Comparing
  each held directory's `mnt_id` (`/proc/self/fdinfo/<fd>`) with its parent's
  would follow the mounts as they stand. `clear` also recurses, one open
  directory per level, so a tree in a default deeper than the descriptor
  limit fails the erase and the reset with it; an iterative walk that keeps
  only the current directory open would empty any depth. Governed by ADR-021.
- **Why a session ended, on the sign-in page.** A browser whose session was
  revoked on Access or ended by a password change lands on `/login?expired=1`,
  which says "You were signed out after a period of inactivity." Remembering
  why each token ended (briefly, by token) would let sign-in say "You were
  signed out from another browser" instead. Governed by ADR-007 §7.

## Validation (ADR-008)

- **A `cidr` datatype.** WireGuard addresses and allowed-IPs are CIDR
  (`10.0.0.1/24`); `internal/datatype` has `ipaddr` (a bare address) but no CIDR
  type, so the WireGuard plugin declares no datatype on those fields. Add a `cidr`
  (v4/v6 `addr/len`) datatype so they validate. Governed by ADR-008.

## Product / definition-of-done

- **Timezone acceptance plugin.** A second plugin written only from
  `docs/plugins.md`, with no shell changes — the "someone else can write one"
  proof.
- **Conformance kit.** An executable check of the plugin contract
  (`docs/plugins.md` carries the checklist).
- **Narrow unit spaces in the plugin repos.** wireless (MHz, dBm, `%`), vpn and
  ddns (durations), qos (Mb/s) and dnsdhcp (lease expiry as "11h 32m") write
  their own figures with a full space or none. A duration or rate goes to the
  shell as `duration` or `rate`; any other unit takes U+202F (DESIGN.md's
  Narrow Space Rule).
- **qos claims the device's pause toggle.** The shell draws a device row's
  power glyph and posts it to the plugin claiming `pause` (ADR-006,
  `entity_acts` with `post`). qos's branch `pause-act` claims it, answers
  `_act=pause`, and names paused devices in `engaged`; it builds once its SDK
  pin moves past v0.8.0, which has no `EntitySummary.engaged`.

## Simplification

- **`NativeBackend`'s ubus seams.** The helper seams are methods on
  `helperSocket`; 14 ubus seams (systemInfo, systemBoard, access, the uci
  set/config/add/delete/order and changes/revert/apply/confirm verbs, logRead,
  netIfaces) plus kernelBuild/packageArch remain fields, because their tests
  pin arguments and error mapping per field. They become methods once the
  openwrt tests fake ubusd at the socket (needs the blob encoder exported from
  `internal/ubus`).
- **Plugin helpers into the SDK.** Text/select field builders and panel-form
  constants; the describe skeleton ×3; `text_cell`/`pill_cell`/`EM_DASH` ×2.
  The SDK's `Section::flag` reads the init-script truth set; interfaces keeps
  its netifd reader and firewall its fw4 one, each matching its own daemon.
- **dnsdhcp on the next SDK pin.** `verso-plugin-dnsdhcp/src/dns.rs:308` builds
  `Widget::Link` with `desc`/`code`/`act`, which the SDK no longer has; it
  compiles against v0.8.0 and the shell ignores the keys, so bumping its pin
  needs those three lines dropped.
- **Wire keys no plugin sends.** `Property.Variant` and `remove: "lane"` are
  still read by their templates though no plugin sets them. The grid-style
  live-listing attributes (`data-verso-stream`, ring, empty) on tables wire
  nothing and only tests assert them.
- **Firewall vocabulary.** `mod vocabulary` is `#[allow(dead_code)]` and only the
  `#[cfg(test)]` reckoning reads it; it becomes `#[cfg(test)]`, and the SDK
  `vocabulary.rs` (firewall its only consumer) moves into firewall. Dropping the
  machinery outright is ~2,800 lines.
- **Small leftovers.** `FiberModule`'s Vcc/Bias/Tx limits may be read by
  nothing; the wireless `Cargo.toml` comment names
  `wire.rs`/`serve.rs`, which do not exist.

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
- **The temperature history under a grant.** `handleTemperatureHistory`
  answers history.db's temperatures to every signed-in session; gate it as
  ADR-007 §9 gates the roster and the events page.
- **The fibre lists side by side on a phone.** System › Hardware sets its SFP
  modules' fact lists in a two-column `Grid`, which stays two columns at every
  width, so on a phone each list is half the card and a dark receiver's
  "−25.2 dBm · no light" wraps. A grid that stacks under the card's own width
  would keep each list whole.
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
- **Release note: drawers are cards.** A drawer's settings stand in cards on
  sand, as a page's do, with even margins and the title on the cards' edge;
  every answer stands in one column beside its question, and the rows a
  switch reveals read as rows like the rest. Installed plugins need no
  upgrade.
- **The route drawer's Delete route.** The interfaces plugin stands "Delete
  route" under Save in a route's drawer; removal belongs to the listing's
  trash act (DESIGN.md › Drawers), so the drawer drops it.

## Forms as ledgers (forms-ledger)

- **Release note: forms are ledgers.** Every form reads as its questions on
  the left and its answers on one edge, with help on what each setting does
  a press away: "Show help" (or the ? key) opens it under every name, the
  glyph beside one name opens that one alone, and the browser keeps the
  choice. A setting whose change waits to be applied
  is marked by a marigold bar down its row's left edge. Installed plugins
  need no upgrade.
- **Maintenance's Firmware header on a phone.** At 390px the Firmware card's
  header wraps to 108px, half a line over the grid, so every card under it
  on Maintenance stands 4px off its lines.
- **Help written for a tooltip.** The plugins in their own repos (ddns,
  dnsdhcp, qos, vpn, wireless) wrote `help` and `tip` to be read on hover; on
  the page some read long (dnsdhcp's DUID tip runs past 300 characters). Read
  each plugin's forms with descriptions shown and tighten what reads as prose.

## Stylesheet blocks (ADR-025, css-block-layer)

Every look is a block in `internal/server/assets/blocks/`, every text one of
the 21 type roles in `type.css`, and the guards in
`internal/server/stylesheet_guards_test.go` hold both without exceptions.

- **Release note: one look per thing.** Cards, lists, folds, badges and the
  Overview's cards now share one definition each, so the same thing looks the
  same on every page; a few drifted copies changed to match (a list's words in
  line with its card's, a card's fact list giving up its first line's air,
  the device limits tip on the type roles). Installed plugins need no
  upgrade.
- **Two looks for a listing in a card.** A page card runs its listing to the
  frame with Paper column heads; a list (`Style: "list"`, the Overview's
  Ports and Tunnels, who is signed in) stands in the card's inset under a fact
  list's heading. Decide whether a card's listing is ever more than a list.
- **System panels on cards.** The Logs, Alerts and Packages settings panels
  stand their rows on the drawer's own Paper (`data-verso-ground="paper"`)
  rather than in cards, as every other drawer's settings do.
- **The interface inventory's markup.** `table-interfaces.html.tmpl` still
  draws its tree, its columns' widths and its unfold with utilities and
  arbitrary values (`py-row`, `pl-29.25`, `w-39`), the one listing not built
  from the table block's elements; `py-row` is the entry's last utility.
- **Code and properties leftovers.** The code block's token inks and the
  properties head still name colour and layout utilities per site
  (`text-denim-deep`, `sm:w-24`); a token's ink is the code block's, by
  `data-verso-token`.
- **The rail's page transition.** The view-transition rules stand in
  `input.css`; they are the nav block's.

## Pending changes (ADR-010, pending-changes-page)

- **Release note: pending changes live at the top of the sidebar.** The chip
  beside the router's name is gone. While changes wait, a Pending changes row
  stands first in the sidebar and opens a page listing them by the page each
  was made on: open its page, discard one or a page's worth, or apply them
  all and watch each step as the router reports it. An apply the router puts
  back keeps the changes pending. Installed plugins need no upgrade.
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
  writing it beside `<section>.ip` (as `<section>.current`) lets `ddnsState`
  hand it over. Send with the restart fix, upstream and in the Mono feed.
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
