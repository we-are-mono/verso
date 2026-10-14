# Hardware profiles

A **profile** describes a device to the Verso dashboard: how to read its sensors,
which physical order its ports sit in, and (optionally) what its rear panel looks
like. It is a thin *semantic overlay*, not a device tree — the kernel already
describes the hardware (thermal zones, hwmon chips, interfaces); a profile just
says, for *this* device, which sensor is the "CPU temperature", which chip drives a
"Fan", which rail is the headline "Power", and which silicon port is leftmost.

Contributing support for a device is a data change (a folder), not a code change.

## Layout

One **folder per board**, named for the board, holding its assets:

```
profiles/
  mono_gateway-dk/
    profile.json     # the sensor / port map (below)
    back.svg         # optional rear-panel artwork (see "Back panel")
```

A board with **no matching folder needs none** — the fallback is auto-detection, not
a file (below).

## How a profile is chosen

The **`id` field in `profile.json` is the match** — it is the board's raw
`board_name`, from `ubus call system board` (the same `/tmp/sysinfo/board_name`
OpenWrt derives). The shell reads every `*/profile.json` and picks the one whose
`id` equals the device's `board_name`. On a device tree that name is `vendor,model`
(the DK reports **`mono,gateway-dk`**); on x86 it is folded from DMI
(`supermicro-h13sae-mf`) — either way the `id` matches it verbatim.

- **The folder name is only storage.** It never has to equal the `board_name`, so a
  name that carries a comma (`mono,gateway-dk`) lives in a plainly-named folder
  (`mono_gateway-dk/`). The folder is where the profile's `back.svg`/`front.svg`
  travel with it; the `id` is what selects it.
- **`aliases`** (optional, in `profile.json`): extra `board_names` this profile also
  covers — for a board that reports different names across revisions or variants.
- **No match → auto-detect.** When no directory matches the board_name, there is no
  fallback *file* — the shell simply enumerates whatever sysfs exposes. A plain
  OpenWrt-on-a-PC shows its standard `coretemp`/`k10temp` reading, and a VM shows its
  virtual CPU / memory / disk / net, with no board folder at all. A profile is only
  ever *overrides on top of* that auto-detection, so "empty profile" and "no profile"
  are the same thing.

Profiles earn their keep on SoC boards (whose sensor names are non-standard) and
for curating the multi-sensor facts (power, fans) and port order.

## The `profile.json` format

Each hardware kind is an **ordered array of `{ "<where>": "<name>" }` objects**. The
**key says where the sensor is**; the **value is the display name** you choose. No
searching, no label-matching, no traversal — the key *is* the location.

```json
{
  "id": "mono,gateway-dk",
  "name": "Mono Gateway Development Kit",

  "ports": ["eth1", "eth2", "eth0", "eth3", "eth4"],

  "fans": [
    { "i2c-mux@70/i2c@3/fan-controller@2e/fan@0": "System Fan 1", "main": true },
    { "i2c-mux@70/i2c@3/fan-controller@2e/fan@1": "System Fan 2" }
  ],

  "power": [
    { "i2c-mux@70/i2c@0/power_sensor@40": "USB Power Delivery", "main": true },
    { "i2c-mux@70/i2c@0/power_sensor@42": "1V Core PSU" }
  ],

  "thermal": [
    { "cluster-thermal": "cluster", "cpu": true },
    { "ddr-thermal": "Memory" }
  ]
}
```

### The key is a device-tree path

A sensor's key is a **tail of its `device/of_node` path** — the topology the kernel
fixes at probe. Verso builds a one-time index (walk `/sys/class/hwmon/*` and
`/sys/class/thermal/*`, read each `device/of_node` realpath) and matches a key when
it equals the final run of `/`-separated segments of a node's path. DT paths are
unique, so the shortest disambiguating tail is enough; authors copy whatever the
dump prints (see "Authoring").

This is why the key is stable where the obvious alternatives are not:

- **Not `hwmonN` / `thermal_zoneN`** — those renumber across boots and kernels.
- **Not the i2c `bus-addr`** — behind a mux (the DK's eight `ina234` sit on a
  PCA9545) the bus number is allocated dynamically and the reg address repeats
  across channels, so `3-0043` is neither stable nor unique.
- **Not the DTS label** — a label can be renamed, and some drivers expose none at
  all (the DK's `emc2305` publishes no `fan*_label`). The value is *your* name,
  decoupled from the DTS, so a label rename never breaks a profile.

### How each kind resolves

The uniform JSON hides four small mechanics — the resolver picks by kind:

- **Single-sensor chip** (each `ina234` is its own hwmon): the key matches that
  hwmon's of_node directly → read its `*_input`.
- **Multi-channel chip with child nodes** (the `emc2305` is *one* hwmon with
  `fan@0`/`fan@1` child nodes): the key points at the **child**. Resolve by finding
  the hwmon whose of_node is the child's parent, then use the child's `reg` as the
  channel index — `reg 0 → fan1_input`. (Verified on the DK: `fan@0` reg 0 →
  `fan1_input`.)
- **Multi-reading chip without child nodes** (a `tmp431` is *one* hwmon exposing
  `temp1_input` from its local/on-die diode and `temp2_input` from a remote diode,
  with no per-channel of_node child to point at): the key names the chip's of_node
  tail followed by a **`:tempN` channel selector** — `…/temp-sensor@4c:temp1` reads
  `temp1_input`, `:temp2` reads `temp2_input`. A key with no selector reads the
  chip's sole channel (the single-sensor case above), so the selector is only
  needed where one of_node carries more than one reading. (Verified on the DK
  against the running kernel: two `tmp431` sit behind the i2c0 PCA9545 —
  `i2c-mux@70/i2c@1/temp-sensor@4c` measuring the board near the CPU (local) and
  the CPU through a remote diode (remote), and `…/i2c@2/temp-sensor@4c` measuring
  the SFP+ area (local) with its remote diode unconnected, which reads 0 and is
  left out of the profile.)
- **Thermal zone** (not hwmon): match `/sys/class/thermal/thermal_zone*/type`
  against the key (`cluster-thermal`). The zone carries its own trip points, so a
  reading surfaces its warn/critical limits for free.

### The arrays are the Overview's Hardware card

Every entry in `thermal`, `power` and `fans` shows on the Overview's **Hardware
card**, as three lists (Thermal, Power, Fans) in array order, under the names you
give them. An entry the kernel does not expose is left out, and a kind the profile
does not list has no list. A board with no profile, or one whose profile lists no
sensors, has no Hardware card at all. The **Hardware page** shows every reading the
box exposes, profiled or not.

Optional flags pick the headlines:

- **`"cpu": true`** on a thermal entry → *the* CPU temperature: the Hardware card's
  line while it is closed, and the Hardware page's Processor tile.
- **`"main": true`** on a fan entry → the Hardware page's Cooling tile, even when
  another channel spins faster.
- **`"main": true`** on a power entry → the Hardware page's Power draw tile. Use
  the board **input** rail: downstream regulators (Core/DDR/…) are fed from it, so
  summing all rails would double-count.

The card's header speaks only when a reading is out of band: a temperature past its
own warn or critical limit, or a fan with its fault or alarm set. A temperature with
no limits of its own is never out of band.

There is no separate overrides block — the arrays *are* the labels, the order, and
the visibility. To keep a reading off the Hardware card, leave it out of the array.

### `ports` — physical order, left to right

`ports` is a plain **ordered array of interface names**, listed as they appear on the
device's rear, **left to right**. It is deliberately *not* a `{ path: name }` object:
the interface name is already the identity (nothing to resolve), the order is the
whole payload, and a port's role name (WAN, LAN 1) belongs to network config, which
the user changes — not to fixed hardware.

This fixes the common board bug where Linux's `ethN` numbering doesn't match the
silk-screen order. The DK enumerates `eth1, eth2, eth0, eth3, eth4` but that *is* its
left-to-right order, so the shell draws port 1 = `eth1`, port 3 = `eth0`, and so on —
and the same order lines up the `data-verso-port` tags on `back.svg`.

## Don't re-transcribe the device tree

A profile is an **editorial overlay**, not a copy of the DTS. The running kernel
already exposes — from the device tree — the sensor **labels** and the thermal
**trip points** (`/sys/class/thermal/thermal_zone*/trip_point_*_temp`). Verso reads
those at runtime. So a profile carries **no thresholds** — only the decisions the
kernel can't make: which zone is *the* CPU, which rail is *the* headline power, which
readings to show, and what to call them.

The reader scales by fact type (temperature is milli-°C, power is µW, fan is raw
RPM), so a profile only names sensors — it carries no units.

## The Sensors view (show everything)

An Advanced page that lists **every** reading the box exposes — for the power user,
off the calm home glance. **It needs no profile at all:** by default the shell
enumerates every `/sys/class/thermal/*` and `/sys/class/hwmon/*` and renders it with
the label the kernel gives — so an unknown board still shows all its temps, rails,
and fans. A profile's arrays only supply nicer names and a deliberate order.

Two rules make this complete across boards:

- **Read both sources.** Temperatures come from `/sys/class/thermal/thermal_zone*`
  *and* hwmon `temp*_input` — because some boards have only zones, some (x86/ACPI,
  no device tree) have only hwmon, and some (the LS1046A) have both.
- **Dedup the mirror.** When a sensor appears as both a zone and a hwmon entry (the
  LS1046A's TMU exposes each site twice — `cluster-thermal` and `cluster_thermal`),
  keep the **zone** (it carries the trip points) and drop the hwmon twin, matched by
  the shared `device/of_node`.

**Fan presence** is read, never pinned. A channel's state comes from its reading:

- RPM > 0 → running.
- RPM 0 **and** `fan*_fault`/`fan*_alarm` set → a real fault (a plugged fan that
  died) — this alarms.
- RPM 0, no fault → **not connected**: show "—", quietly, no fault styling.

Trust the controller's **DT-declared** channels (`device/of_node` children) for how
many connectors physically exist, so an unpopulated header isn't shown as a phantom
fan.

## Back panel (optional)

A board folder may carry a **`back.svg`** — artwork of the device's rear that the
Overview's Ports card and the Hardware page show in place of the panel the shell
generates. It is a **skin over the live ports layer, not a static picture**, and the
shell lights it exactly as it does the generated panel — no code per board. No
`back.svg` → the generated panel is drawn instead, its ports in the order `ports`
gives.

What the art carries:

- **One group per port:** `<g data-verso-port="<iface>" class="port">`, named for
  the silicon interface, in the board's left-to-right order.
- **The outline as the group's only direct-child `<path>`.** The shell strokes it to
  mark a runtime role (the WAN port), so the role never lives in the art; put any
  other path (a mouth that follows the outline) inside a nested `<g>`.
- **Two LEDs drawn unlit:** `class="led-link"` and `class="led-traffic"`. The shell
  paints the lit ones.
- **An empty role slot over the port:** `<text class="verso-port-role" …></text>`,
  centred 8px above the port's top. The shell writes "WAN" into the WAN port's slot;
  the art never names the role itself.
- **Its own size in pixels.** Draw it at the size an Overview card shows it (the
  DK's is 504×136), every box edge on an 8px line and every detail on a 4px one,
  1px strokes half a pixel in, the chassis filling the `viewBox` from `0 0`. The
  `viewBox` height is a multiple of 8. The shell draws the art at that size where it
  fits and shrinks it in whole 8px steps where it does not, so it stays on the
  layout's grid.

How the shell lights it: each port group gains `is-linked` while it has link,
`act-0` … `act-4` for how busy its traffic LED blinks (from its packets per second),
`is-wan` on the WAN port, and a `--led-phase` so busy ports blink out of step. It is
reachable by keyboard and labelled with its name, link and speed, and its detail
(role, address, link) shows on hover or focus.

**A submitted SVG is untrusted content and is sanitized before it renders:** inline
only, no `<script>`, no event handlers (`on*`), no remote refs (`href`/`<image>`),
no `<foreignObject>`, no `url()` in CSS — an allowlist of elements and attributes,
the same posture as the `raw` widget but a larger surface. The validator checks
the tagged `data-verso-port` ids against the board's real interfaces.

## Authoring a profile

Dump the device's real sensors and copy the of_node path tails in as keys:

```sh
for z in /sys/class/thermal/thermal_zone*; do echo "$(cat $z/type)"; done
for h in /sys/class/hwmon/hwmon*; do
  echo "$(cat $h/name)  ->  $(readlink -f $h/device/of_node)"
done
```

(A `verso sensors` command will do this properly — print each live reading next to
the exact key to paste, and validate a profile against the hardware — planned.)

See `mono_gateway-dk/profile.json` for a worked example: the LS1046A's TMU names the
CPU site `cluster-thermal` (not `cpu-thermal`), fans come from a multi-channel
`emc2305`, "Power" is the input rail, and the ports enumerate out of silk-screen
order. Its `mono_gateway-dk/back.svg` is a worked back panel, 504×136 on the 8px
grid: USB-C and reset, then five `data-verso-port` groups in that left-to-right
order, the RJ45 jacks and SFP+ cages sharing one outline, lip and pair of LEDs
drawn dark, and no WAN asserted.
