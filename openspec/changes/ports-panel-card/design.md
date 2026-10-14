# Design

## Context

See proposal.md for why. What exists today:

- `widget.Ports` (`internal/widget/ports.go`) draws either a profile's panel art, lit by `litPanel` adding `is-linked`/`is-active`/`is-wan` to each `data-verso-port` group, or a generated strip: a dark chassis with screws and per-port inline SVG jacks whose LEDs are `led-link`/`led-act`. It is wire-decodable (`"ports"` in the widget registry), so plugins can send it; `PortItem.Active` is a boolean.
- Only `hardwarePanel` (`internal/server/hardware.go`) builds one. Its `panelPorts` takes the profile's order or the telemetry snapshot's physical interfaces, sets every `Kind` to `"rj45"`, and sets `Active` from `RxBPS > 0 || TxBPS > 0`.
- The telemetry sampler reads every netdev's counters once a second and already computes `RxPPS`/`TxPPS`. It marks a netdev physical when it has a `device` link and is not wireless, so a DSA conduit counts as physical. `network_topology.go` passes that flag to plugins.
- The overview stream (`/overview/events`, ADR-022) sends `interfaces` frames of `interfaceReading` (name, operstate, state, variant, rx/tx rate) only while the Ports or Tunnels card is shown. `verso-stream.js`'s `applyInterface` updates table rows keyed `interface:<name>` and nothing else. The Hardware page opens no stream.
- `nameplate.go` already reads `/etc/board.json` once per process for the board's identity.
- `back.svg` (reshaped in the working tree) uses `led-link` and `led-traffic`, and each port is a `<g data-verso-port="…" class="port">` with the outline as its one direct-child `<path>`.

## Goals / Non-Goals

**Goals:**
- One panel builder feeds both the Ports card and the Hardware page.
- Board art and the generated panel light and blink by one set of CSS rules, driven by one server-computed activity level.
- Port discovery that is right on DSA routers, x86 boxes and VMs with no profile.
- No change to what plugins receive today; the `ports` widget stays backward compatible.

**Non-Goals:**
- Mirroring the hardware's real LEDs (`/sys/class/leds` netdev triggers). PHY LEDs are rarely exposed there, and the counters already carry the truth.
- Front-panel art for the DK, or art for other boards.
- Sub-second sampling. The blink pattern is drawn in the browser at a rate set by the last second's packets.
- Clicking a port to open anything. The panel is display-only, as now.

## Decisions

### 1. One panel builder, two surfaces

`hardwarePanel`'s body moves into a server-side `devicePanel(ctx, sid) *widget.Ports` that both handlers call. The Overview gains a `Panel *Ports` field; `portsBody` renders the panel, then the table. The table takes its row order from the panel's port order (a name→index map), so the card has one order. The panel is part of the Ports card's body, so a hidden Ports card renders none, as ADR-022 already says of hidden cards.

*Alternative:* render the panel as a separate block above the card's table from the template. Rejected: the card body is one function per ADR-022's card registry, and the panel would then need its own hide/collapse wiring.

### 2. Port inventory as its own small unit

A new `internal/ports` package owns "which ports, in what order, of what kind" and nothing else (single responsibility). It takes its inputs through narrow interfaces, so tests run on a fixture sysfs tree and fake probes:

- **Membership:** telemetry's physical interfaces, minus DSA conduits. The sampler gains a `Conduit bool` (the netdev has a `dsa/` directory in sysfs) instead of turning `Physical` off. That keeps `network_topology.go`'s `physical` field, which plugins read, exactly as it is. The Overview's Ports listing and status skip conduits.
- **Order:** the profile's `ports`; else `/etc/board.json` (`network.wan.device` or `.ports`, then `network.lan.ports` or `.device`), with the remaining ports after it; else a natural sort (`eth2` before `eth10`). Putting WAN first is a guess: board.json records roles, not positions. The raw `/etc/board.json` bytes are read once per process and shared with `nameplate.go`. Its parser stays separate from `boardIdentity`.
- **Kind:** `sfp` when `/sys/class/net/<if>/device/of_node/sfp` exists (the device tree wires the MAC to a cage, true even when the cage is empty), else when `SIOCETHTOOL`/`ETHTOOL_GLINKSETTINGS` reports `PORT_FIBRE` or `PORT_DA`, else `rj45`. The ioctl needs the two-call handshake: the first call learns the link-mode word count, the second returns `port`.
  - *Why ioctl over ethtool netlink:* it needs no generic-netlink family lookup, works whether or not the kernel was built with `ETHTOOL_NETLINK`, and a GET needs no capability. `golang.org/x/sys/unix` is already in the module graph and becomes a direct dependency.
  - Kind is probed when a page renders and once per stream connection, never per tick.

### 3. Activity level and speed are computed on the router

A pure function `ports.Activity(pps uint64) int` maps `RxPPS + TxPPS` to 0–4 at the spec's thresholds (0, <10, <1,000, <100,000, ≥100,000). Every caller uses it: the first paint's `PortItem` and each stream frame. So the thresholds have one definition and Go tests, and the first paint and the stream never disagree.

The sampler reads `/sys/class/net/<if>/speed` for physical interfaces each tick, alongside the counters it already reads, and stores it as `SpeedMbps` (-1 when unknown or down). One formatter renders it as `100M`, `1G`, `2.5G`, `10G`, or a dash.

`interfaceReading` gains `activity` (int), `linked` (bool) and `speed` (string). These fields are additive; the table code ignores them.

### 4. One lighting vocabulary on the port group

Board art and generated ports wear the same classes on their `data-verso-port` element: `is-linked`, `is-wan`, and `act-0` … `act-4`. Each port also carries an inline `style="--led-phase: N"`, where N is a fraction from 0 to 7/8. The generated template renames its traffic LED from `led-act` to `led-traffic`, so one rule set covers both. `litPanel` adds the activity class and the phase variable beside what it adds today. `is-active` goes away; nothing outside these files uses it.

- **Blinking** is pure CSS. Each level sets `--led-period` and names its keyframes:
  - level 1: a short on-pulse in a long period;
  - level 2: an even blink;
  - levels 3–4: short periods, level 4 with a long on-share so it reads near-solid.
  
  The exact periods are tuned on the device, starting from 1.8 s, 0.5 s, 0.16 s and 0.08 s.
- **Busy ports don't blink in step:** the traffic LED's `animation-delay` is `calc(var(--led-period) * var(--led-phase) * -1)`. The server derives the phase from an FNV hash of the interface name, so a port keeps its phase across reloads. Inline custom properties are within the CSP (`style-src 'unsafe-inline'`; the meter and table templates set `style` already).
- **Reduced motion:** `animation: none`, with the LED lit at levels 1–4.

*Alternative:* JS drives each LED with `setInterval` or Web Animations. Rejected: that is per-frame script work the LAN performance budget avoids, and CSS animations pause by themselves in hidden tabs.

### 5. Live wiring

`applyInterface` in `verso-stream.js` also finds `[data-verso-port="<name>"]`. It toggles `is-linked`/`is-empty`, swaps the `act-N` class, and writes the speed into the port's `[data-verso-port-speed]` slot. It leaves `--led-phase` alone, because that never changes. The stream script's start condition adds `[data-verso-port]`.

The Hardware page opens `/overview/events?reads=interfaces`. With `reads` set, the stream sends only the named frames, which must be in an allowlist (`interfaces`), and ignores the Overview layout cookie. Without it, the Overview's layout-driven selection is unchanged. This keeps one stream implementation and one client, and costs the Hardware page no system or WAN sampling.

*Alternative:* a separate `/hardware/events` endpoint. Rejected: it would duplicate the tick loop and the interface reading for one frame type.

### 6. Generated panel art

`ports.html.tmpl`'s generated branch drops the dark chassis and screws. Each port is an inline SVG in a 48×48 box (pixels at native size), using the connector geometry from `back.svg` translated to the origin. Generated ports and the DK's ports are therefore the same size.

- **Shared:** a 48-wide outline with its body 40 tall and a 16-wide lip 8 under it, and two 8×4 LEDs beside the lip. The generated `viewBox` is `0 24 48 48`, the DK's port coordinates with x from 0, so the connector fills its box.
- **RJ45:** a 40×32 mouth and 8 pins, 1.5×8, 3 apart.
- **SFP+:** the outline top 8 lower, a 40×24 mouth, and the 24×8 socket with its slit.

Strokes are 1px and sit half a pixel in, so at 1:1 every box edge stands on an 8px line and every detail on a 4px one.

Fills are classes (`verso-port-body`, `-mouth`, `-pin`, `-socket`) coloured from palette tokens in light and dark. The exact neutral finish is settled by building it and trying it on the device. Ports are inlined per port, not shared through `<use>`: per-port classes must reach each LED, and styling inside a `<use>` shadow tree is awkward.

**Scaling, on the 8px grid, left aligned:** the panel wrapper is a size container that starts at the content's left edge. The template sets `style="--ports: N"`, the port count. Each port's art is sized `round(down, min(3rem, (100cqi - (var(--ports) - 1) * 0.5rem) / var(--ports)), 0.5rem)`. It is drawn at its native 48px whenever the row fits, which is pixel-true on the grid, and shrinks in whole 8px steps when it doesn't. It never scales up.

- Every port gets the same size, 8px apart, as on the DK.
- The row starts on the content's left edge, and any spare width lies to the right of the last port.
- Labels and speed sit on 24px lines under the art (layout-grid), so the block ends on a line.
- Labels keep a 0.6875rem floor and clip with an ellipsis.
- The hover tip (HTML, as now) always names the port.

**Board art on the grid:** art is authored in pixels at its native size, the size it is drawn at in an Overview card. Every box edge is on an 8px line, every detail on a 4px one, and the chassis fills the `viewBox` edge to edge. The DK's `back.svg` is 504×136: an Overview column's card content at the widest page measure is 504px.

The art's `<svg>` is sized `height: min(var(--art-h), round(down, 100cqi * var(--art-aspect), 0.5rem))` with `width: auto`, so it:
- draws at 1:1 wherever it fits, and every line it draws is on the grid there;
- shrinks in whole 8px steps where it doesn't;
- never scales up;
- keeps its proportions and stands at the content's left edge.

The template sets `style="--art-h: Hpx; --art-aspect: H/W"` on the wrapper, and `readPanel` reads H and W from the art's `viewBox` when it loads the file. A board needs no setting for it, and the art contract (decision 8) requires a `viewBox` whose height is a multiple of 8.

CSS `round()` is in every engine Verso supports (Chromium 125+, Firefox 118+, Safari 15.4+). On an older engine the art falls back to the unrounded size, which is correct but off the grid.

The eight-pixel-grid guard tests in `assets_test.go` gain one: every panel art height in the stylesheet is a `round(down, …, 0.5rem)`.

The `ports` widget stays one widget. This is a restyle of its generated branch, not a new widget, so ADR-005's closed set is unchanged.

### 7. Widget contract

`PortItem` gains `Activity int json:"activity,omitempty"` and `Speed string json:"speed,omitempty"`, and keeps `Active bool json:"active"`. The effective level is `Activity` when it is above 0, else 2 when `Active` is set, else 0. A plugin built against today's SDK keeps blinking. The shell sets `Activity`.

### 8. Board art contract

`profiles/README.md`'s back-panel section states what art must carry:

- the `data-verso-port` group;
- `led-link` and `led-traffic` LEDs drawn dark;
- the outline as the group's only direct-child `<path>` (the WAN rule's target).

It also states that the shell lights them with the classes in decision 4, and that art is authored in pixels at its native size on the 8px grid, with a `viewBox` whose height is a multiple of 8. The redrawn DK `back.svg` (504×136) ships in this change.

## Risks / Trade-offs

- **A port's driver reports neither a DT `sfp` link nor `FIBRE`/`DA`** → it draws as RJ45. A profile fixes it; adding a `kinds` override to `profile.json` stays possible without a format break.
- **WAN-first ordering from board.json is a guess** → a board with its WAN on the right draws mirrored. A profile corrects it, and the ports stay correctly named and lit either way.
- **One-second sampling** → a burst shorter than a second shows a full second of blinking. That is acceptable for a glance instrument.
- **The table's order changes** to the panel's on boards whose kernel order differs (the DK). This is intended, so the card has one order.
- **Animation cost with many ports** → only traffic LEDs animate, a few small rects repainting at most about 12 times a second each. Hidden tabs pause them, and the stream already disconnects there.
- **`reads=` widens the stream's surface** → an allowlist of frame names; anything else is ignored. The session check is unchanged.

## Migration Plan

No on-device state changes, so there is nothing to migrate. The plugin contract is additive, so no plugin needs upgrading. Rolling back restores the table-only Ports card; nothing persists.
