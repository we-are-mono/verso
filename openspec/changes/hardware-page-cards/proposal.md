# Proposal

## Why

System › Hardware shows most readings two or three times. The CPU temperature, main power rail and main fan open the page as a strip of tiles that repeats the Overview's Hardware card. Every temperature, rail, fan and fibre field shows again in a closing "All sensors" table. Since every page became cards, that tile strip, the shell's `strip` grid, is used nowhere else. The page should read as the box itself: one card per kind of reading, each reading once, with the kernel's own name kept beside the one the profile gives.

## What Changes

- The page opens with a card titled with the board's name (the profile's `name`, else the model OpenWrt reports), holding the device panel centred across the card, and under it what the box is made of: Processor (name, cores, top clock), Memory, Storage (the boot disk's size and kind) and the `board_name`. These facts appear nowhere else in the UI.
- The page heading says when the hardware needs attention, using the same grading and words as the Overview's Hardware card: "cluster is hot" in the warning or danger tone, else "System Fan 1 stopped" in the warning tone. Otherwise it reads "Hardware", as today.
- **Temperatures** is one table: Reading, Source, Temperature, Limits. The CPU leads, then the profile's entries in order. A board with no profile lists every temperature the kernel exposes, under the kernel's names. The Temperature column draws a headroom bar to the sensor's protect point, with ticks at its warn and protect limits and the reading beside it.
- **Power** is one table of every rail: Rail, Source, Voltage, Current, Power. The input rail the profile flags `main` leads, tagged "input"; the other rails follow, sorted by draw. The card's heading carries no summary: the input row already states the draw.
- **Cooling** is one table of every fan, whether one runs or several: Fan, Source, Speed, Duty, Status. Status is the last column.
- **Fiber modules** shows each SFP cage as a headed fact list: "SFP+ module" with the cage (`xfi0`) as its chip, the way the Overview heads an address family. A module is no longer a card inside the card.
- **Removed:** the tile strip at the top of the page, the "All sensors" table, and the shell's `strip` grid style, which nothing else uses. On a profiled board the page shows what the profile lists and nothing else, as `profiles/README.md` already promises; the "All sensors" table broke that promise.
- The table widget's `meter` cell gains optional warn and crit tick positions, a column that keeps a readable bar width beside other columns, and a figure slot wide enough for a reading with its unit. This is an additive, optional schema change: installed plugins keep working unchanged.

## Capabilities

### New Capabilities

- `hardware-page`: what System › Hardware shows: the board card with its panel, the four kinds of reading, which readings each lists and in what order, and what a board with no profile gets.

### Modified Capabilities

None. The Overview's Hardware card and the device panel keep their requirements: the panel still stands on System › Hardware, and every reading beyond the card's three still stays on the Hardware page.

## Impact

- **ADR-022 (Overview cards)** names "the Hardware page's tiles" as what the `cpu` and `main` flags lead; amended in place to say those readings lead the page's Temperatures, Power and Cooling cards.
- **ADR-005 (UI consistency contract)**: the meter cell's tick marks join the table schema; the SDK crate (`plugins/verso-plugin-sdk`) mirrors the new optional fields and its conformance test covers them.
- `internal/server/hardware.go`: the page composition, rebuilt from `Section`, `Table`, `Properties` and `Grid`; the tile builders (`hardwareVitals`, `cpuTile`, `powerTile`, `fanTile`, `fanStripTile`) and the readings table (`hardwareReadings`, `bandReadings`) go.
- A new package for the board's facts (processor, cores, clock, memory, boot disk), reading sysfs and procfs once per page load behind an injectable root, as `sensors.Reader` does. `openwrt.Board` gains the `system` string from `ubus call system board` as the processor's fallback name.
- `profile.json` gains an optional `processor` field (the DK: "NXP LS1046A"), documented in `profiles/README.md`. It is additive: a profile without it gets the kernel's name. The in-flight `board-profiles-runtime` change moves `profiles/` to `boards/`; whichever lands second carries the field to the new place.
- `internal/widget/overview_cards.go`: the hot-CPU-then-stopped-fan grading moves out of the Overview card's header into one function the card and the page both call.
- `internal/sensors/inventory.go`: the selectors the page composes with (`MainFan`, `OtherFans`, `RunningFans`) answer what the cards ask; their comments drop the "grid" they describe.
- `internal/widget/table.go`, `templates/table.html.tmpl`: the meter cell's ticks, width and figure slot. `templates/grid.html.tmpl` loses its `strip` branch.
- `internal/server/assets/sections.css` or `input.css`: the panel centred within the Hardware page's board card (the Overview's Ports card keeps its own placement); rebuilt `verso.css`.
- `profiles/README.md`: "Flags pick the headlines" speaks of the page's cards, not its tiles. `DESIGN.md` describes the Hardware page's cards and the meter cell's ticks.
- `i18n/sl/base.json`: new and removed strings ("input", "Limits", "Cooling", "Processor", "Storage", "Board name", the cores and clock phrasing; "%d rails · sorted by draw" goes); the i18n audit stays at zero.
- `docs/BACKLOG.md`: the "Live readings on System › Hardware" item rewords to name the cards.
- No on-device state moves; nothing needs a migration.
