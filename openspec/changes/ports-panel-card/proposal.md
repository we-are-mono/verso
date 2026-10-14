# Proposal

## Why

The Overview's Ports card is a table, so you have to read words to find out which socket has its cable in. The router's own panel answers that at a glance. Verso can already draw that panel (the Mono Gateway DK's `back.svg`, or a generated strip), but only on System › Hardware. There it is drawn once at page load: an unplugged cable stays green until a reload, and the traffic LED either glows steadily (DK art) or blinks at one fixed speed whether a port carries one packet a second or a gigabit. On a board without a profile, every port is drawn as copper in whatever order sysfs lists it, and a switch chip's CPU port (DSA conduit) shows up as a port the case does not have.

## What Changes

- **The Ports card opens with the device's panel**, its table below. On the DK it is the board's own art; on any other board it is a generated panel of the ports Verso finds.
- **The LEDs are live.** Link lights follow the cable each second. The traffic LED blinks at a rate that follows the port's packets per second: dark when idle, an occasional blip on a trickle, a steady blink, a fast flicker, and a near-solid flicker under heavy load. Busy ports do not blink in step. With reduced motion the traffic LED stays lit while traffic flows.
- **The generated panel uses the DK's connector shapes**: one outline, lip and LED pair for both kinds; RJ45 has a dark mouth with eight contact pins, SFP+ a shorter cage with a dark mouth and the connector socket inside. It is drawn in the shell's palette, not the DK's dark finish.
- **Ports are found, not assumed.** Order comes from the board profile, else from OpenWrt's `/etc/board.json`, else from a natural sort of the names. A port is SFP+ when the device tree wires its MAC to an SFP cage or the driver reports fibre or direct-attach, else RJ45. Wireless, virtual and DSA conduit devices are not ports. Each port is labelled with its name and link speed.
- **Many ports fit**: the panel stays one row and the connectors draw smaller to fit the card; labels keep a readable minimum size and every port keeps its hover detail.
- **The panel stands on the 8px grid, at the card's left**: its art is always a whole number of 8px steps tall, shrinking step by step, and it starts at the card's left edge like the text and tables, never centred.
- **System › Hardware shows the same live panel.**
- **The DK's `back.svg` is redrawn** (already in the working tree):
  - drawn at 504×136 px, the size it shows in an Overview card, with every edge on the 8px grid and the USB-C contacts on the vertical centre;
  - RJ45 and SFP+ share outlines, lips and LEDs, and both have dark mouths;
  - the RJ45s have marigold pins;
  - the SFP+ cages stand 8 lower at the top, with a connector socket inside.
- The `ports` widget gains a graded activity level. A plugin that still sends the boolean `active` keeps working: `true` reads as a steady blink.

## Capabilities

### New Capabilities

- `device-panel`: the drawing of a router's physical ports, from board art or generated, with live link and traffic LEDs, port discovery and ordering, and how it scales.

### Modified Capabilities

- `overview`: the Ports card holds the device panel above its table, the table lists ports in the panel's order, and a DSA conduit is not listed as a port.

## Impact

- **Code:** `internal/widget/ports.go` and `ports.html.tmpl` (generated panel, activity level), `internal/widget/overview_cards.go` (panel in the Ports card body), `internal/server/hardware.go` (shared panel builder), `internal/server/telemetry.go` and the overview stream (activity level, link speed per interface), `internal/telemetry/sampler.go` (marks DSA conduits, reads link speed), `internal/server/assets/verso-stream.js` and `input.css` (live LEDs, blink rates), a new port-kind probe (device tree `sfp` property, `SIOCETHTOOL` link settings), `profiles/mono_gateway-dk/back.svg`.
- **Wire contract:** the `ports` widget's `PortItem` adds an activity level; the boolean `active` stays accepted. The overview stream's interface frames add activity and speed fields; the Hardware page opens the same stream for interface frames only.
- **ADRs:** ADR-022 (Overview cards: the Ports card's body and which frames the stream sends) and ADR-005 (the `ports` widget's place in the closed widget set) are amended in place.
- **Specs:** `layout-grid`'s rules apply unchanged; the panel's grid behaviour is stated in `device-panel`.
- **Docs:** `profiles/README.md` (panel art contract: connector shapes, the activity LED rule), DESIGN.md (the panel's look and LED behaviour), `docs/BACKLOG.md`.
- **Budget:** on the router, one sysfs read (link speed) per physical port per stream tick; the port kind is probed once per page render and stream connection. In the browser, a few small CSS animations.
