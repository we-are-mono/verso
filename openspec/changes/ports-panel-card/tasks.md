# Tasks

## 1. Telemetry: conduits and link speed

- [ ] 1.1 Write `sampler_test.go` cases on a fixture sysfs tree: a netdev with a `dsa/` directory is `Conduit` and stays `Physical`; a plain NIC is neither a conduit nor non-physical. Then add `Conduit` to `telemetry.Interface` and set it in `readInterfaceMetadata`. Verify with `go test ./internal/telemetry`.
- [ ] 1.2 Write sampler tests for `SpeedMbps`: a fixture `speed` of `1000` reads 1000; `-1`, a missing file, or a non-physical netdev reads -1. Then read `speed` for physical interfaces each tick. Verify with `go test ./internal/telemetry`; `make lint` green.

## 2. Port inventory (`internal/ports`)

- [ ] 2.1 Write table tests for `Activity(pps)` at 0, 1, 9, 10, 999, 1000, 99999 and 100000 (levels 0,1,1,2,2,3,3,4), and for `Phase(name)`, which must be stable and one of 0, 1/8 … 7/8. Then implement both. Verify with `go test ./internal/ports`.
- [ ] 2.2 Write tests for the speed formatter: 10→`10M`, 100→`100M`, 1000→`1G`, 2500→`2.5G`, 10000→`10G`, -1→dash. Then implement it. Verify with `go test ./internal/ports`.
- [ ] 2.3 Write tests for board.json port order, using fixtures with:
  - `wan.device` + `lan.ports`;
  - `wan.ports` + `lan.device`;
  - no network section;
  - malformed JSON (no order, no error surfaced).
  
  Then implement the parser, separate from `boardIdentity`. Share one once-per-process read of `/etc/board.json` with `nameplate.go`. Verify with `go test ./internal/ports ./internal/server`.
- [ ] 2.4 Write tests for ordering: profile order wins; board.json order next, with the rest after it; natural sort last (`eth1, eth2, eth10`); conduits, wireless and virtual devices are excluded. Then implement `Order`. Verify with `go test ./internal/ports`.
- [ ] 2.5 Write tests for kind detection with a fixture sysfs tree and a fake link-settings prober:
  - `device/of_node/sfp` present → sfp;
  - prober says FIBRE or DA → sfp;
  - TP, an error, or no answer → rj45.
  
  Then implement the prober over `SIOCETHTOOL`/`ETHTOOL_GLINKSETTINGS` with the two-call handshake. `golang.org/x/sys/unix` becomes a direct dependency. Verify with `go test ./internal/ports` and `make lint` (deadcode on arm64 and x86_64).
- [ ] 2.6 Run the kind prober read-only on the bench DUT or dev container and record the result in the task note: what eth3/eth4 report on the DK, and what a VM's virtio NIC reports. Verify against the panel's drawn kinds.

## 3. The `ports` widget: activity, speed, generated art

- [ ] 3.1 Write `ports_test.go` cases:
  - effective level: `Activity` 3 wins over `Active`; `Active: true` alone is level 2; neither is 0;
  - a wire-decoded `{"active": true}` port renders `act-2`;
  - `litPanel` adds `act-N` and `--led-phase` beside `is-linked`/`is-wan`, and no longer adds `is-active`.
  
  Then add `Activity` and `Speed` to `PortItem` and update `litPanel`. Verify with `go test ./internal/widget`.
- [ ] 3.2 Write template tests for the generated branch:
  - an RJ45 port renders the outline, mouth and 8 pins;
  - an SFP port renders the lowered outline, mouth and socket;
  - both carry `led-link`/`led-traffic`, the `act-N` class and `--led-phase` on the `data-verso-port` element;
  - the speed slot holds the formatted speed;
  - each generated port's SVG has the `viewBox` `0 0 48 48`;
  - the wrapper carries `--ports: N`, and the art wrapper carries `--art-h` and `--art-aspect` read from the art's `viewBox`.
  
  Then rewrite the generated branch of `ports.html.tmpl` with the connector geometry from `back.svg`, dropping the chassis and screws. Verify with `go test ./internal/widget`.
- [ ] 3.3 Write the CSS in `input.css`:
  - one rule set lighting `led-link`/`led-traffic` for art and generated ports alike;
  - `act-1`–`act-4` keyframes with `--led-period`, and a delay from `--led-phase`;
  - a reduced-motion override that keeps the LED lit without blinking;
  - neutral connector fills from palette tokens for light and dark;
  - a left-aligned single row whose port art is `round(down, min(3rem, …), 0.5rem)` from `--ports`, 8px apart;
  - board art height `min(var(--art-h), round(down, 100cqi * var(--art-aspect), 0.5rem))`, never scaled up;
  - a minimum label size and ellipsis, with labels on 24px lines.
  
  First add a guard test to `assets_test.go`'s eight-pixel-grid family: every panel art height in the stylesheet is a `round(down, …, 0.5rem)`. Remove the old chassis/screw/`led-act` rules. Then `make css`, and verify with `make test` and by loading System › Hardware in the dev container.
- [ ] 3.4 Update ADR-005 in place: the `ports` widget's generated branch, its activity level, and the boolean kept for plugins. Update the `ports` widget entry in `docs/plugins.md` with `activity` and `speed`. Verify both documents name `activity` (0–4), `speed`, and the boolean `active` fallback.

## 4. One panel builder, both surfaces

- [ ] 4.1 Write `hardware_test.go` cases for `devicePanel`:
  - the DK profile yields the art plus five items in profile order, with kinds from the profile's art;
  - an unprofiled inventory yields items with order, kind, activity, phase, speed and WAN role set;
  - a conduit is absent.
  
  Then move `hardwarePanel`/`panelPorts` into `devicePanel` on top of `internal/ports`. Verify with `go test ./internal/server`.
- [ ] 4.2 Write `overview_cards_test.go` cases:
  - the Ports body renders the panel before the table;
  - table rows follow the panel's order (DK: eth1, eth2, eth0, eth3, eth4);
  - a conduit has no row and does not count toward "ports without link".
  
  Then add `Panel` to `Overview`, render it in `portsBody`, order the rows, and skip conduits in `portsHead` and the listing. Verify with `go test ./internal/widget ./internal/server`.
- [ ] 4.3 Amend ADR-022 in place: the Ports card's body holds the device panel above its table, and the table follows the panel's order. Verify the ADR reads as what is, with no staging words.

## 5. Live LEDs over the stream

- [ ] 5.1 Write `overview_events_test.go` cases: interface frames carry `activity`, `linked` and `speed` for physical interfaces, computed by `ports.Activity` and the speed formatter. Then extend `interfaceReading`. Verify with `go test ./internal/server`.
- [ ] 5.2 Write stream tests:
  - `?reads=interfaces` sends only interface frames, whatever the layout cookie hides;
  - an unknown `reads` value sends nothing extra;
  - without `reads`, frames are unchanged.
  
  Then implement the `reads` allowlist. Verify with `go test ./internal/server`.
- [ ] 5.3 In `verso-stream.js`: add `[data-verso-port]` to the start condition. Have `applyInterface` toggle `is-linked`/`is-empty`, swap `act-N`, and write the speed slot on matching port groups. Make the Hardware page's panel open `/overview/events?reads=interfaces`. Verify in the dev container with the Playwright browser: generating traffic on a port changes its `act-N` class within a second on the Overview and on System › Hardware, with the Ports card hidden.
- [ ] 5.4 Update `profiles/README.md`'s back-panel section with the art contract and lighting classes (design decision 8). Update DESIGN.md with the panel's look, LED meanings and blink levels. Reword the comments in `input.css` and `hardware.go` that call the panel blink "a follow-up" so they state what is. Verify by grepping the touched files for "follow-up" next to the panel.

## 6. Board art and integration

- [ ] 6.1 Keep the redrawn `profiles/mono_gateway-dk/back.svg` (504×136) in this change. Extend `hardware_test.go`'s art test:
  - five `data-verso-port` groups, each with one direct-child `<path>`, one `led-link` and one `led-traffic`;
  - the file parses as XML;
  - its `viewBox` starts at 0,0 with width and height multiples of 8;
  - the chassis rect fills the `viewBox`.
  
  Verify with `go test ./internal/server`.
- [ ] 6.2 Integration in the dev container (`make dev` running):
  - the Overview's Ports card shows the generated panel above the table;
  - pulling a link (`ip link set … down`) darkens its LED within a second;
  - an eight-port fixture stays one row in a narrow card;
  - at window widths 1440, 1024, 768 and 390, the art's height (generated and DK) measures a multiple of 8px, and its left edge equals the card content's left edge (Playwright `getBoundingClientRect`, with the G grid overlay as a visual check);
  - reduced motion (Playwright `emulateMedia`) stills blinking.
  
  Verify by recording each observation in the task note.
- [ ] 6.3 Owner check on the Mono Gateway DK (bench DUT, or read-only on the owner's Gateway with their leave): the art lights and blinks by real traffic, and eth3/eth4 read as SFP+. Verify by the owner's confirmation of the look.
- [ ] 6.4 Update `docs/BACKLOG.md`: delete items this change closes (panel LED blink, live hardware readings for ports), add any follow-up found in 2.6/6.3 (e.g. a profile `kinds` override if a board misreports). Verify with `make lint` and `make test` green.

## Workflow follow-up

- Before archiving, confirm ADR-005, ADR-022, DESIGN.md, profiles/README.md and docs/BACKLOG.md state what landed.
- Archive the change once the owner has confirmed the look on the device.
