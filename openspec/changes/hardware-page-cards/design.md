# Design

## Context

`handleSystemHardware` (`internal/server/hardware.go`) builds the page as a `Stack`: the bare `Ports` panel, a `Grid{Style:"strip"}` of three `Stat` tiles, then curated sections (Temperatures as compact `Meter` rows, Power as a table of the non-input rails, Fans as tiles only when two or more run, Fiber modules as a grid of `Card{Style:"artifact"}`), then "All sensors": one table banded by kind that lists every reading again. The `sensors.Inventory` selectors were shaped for that composition: `CuratedTemps` leaves the CPU out because the CPU led the tile strip, `OtherFans` leaves the main fan out for the same reason, `RunningFans` drives the "two or more" rule.

The approved mock (artifact `HsudbJCfv2QTtpmryY8qLK`) was rendered from these same widgets with a DK-shaped inventory. It is the visual reference. The spec (`specs/hardware-page/spec.md`) states what each card lists.

Constraints that shape the approach: the closed widget set (ADR-005, a new widget needs approval), the plugin SDK mirrors the schema (ADR-006), the 8px grid and the cards-everywhere section rules in `sections.css`, and DESIGN.md's single table look.

## Goals / Non-Goals

**Goals:**
- Every kind card is composed from existing widgets (`Section`, `Table`, `Properties`, `Grid`), with the shell's own rules handling spacing.
- The `sensors` package owns which readings each card lists and in what order; `hardware.go` only lays them out.
- The meter cell's growth is generic: any listing with a share or a headroom can use the ticks.

**Non-Goals:**
- The SFP modules' identity (vendor, part, wavelength, serial from the module's EEPROM). It needs a privileged reader in `verso-rpcd` and is a change of its own.
- Which A/B slot the DK booted. It is specific to one board's layout; the board facts stay generic.
- Live readings. The panel stays live from the stream as it is; the readings stay as the page loaded them. That backlog item is unchanged.
- The Overview's Hardware card. It keeps its three rows and its selectors (`CPUTemp`, `MainPower`, `MainFan`).
- Stacking the two fibre lists on narrow screens. `Grid` with two columns stays two columns at every width, today as before.
- Sorting or filtering the readings. Each card is a short, fixed listing with no control band.

## Decisions

### The page composes four tables and a fact-list grid, no hand-built markup
Temperatures, Power and Cooling are `Section{Hairline}` + `Table` with fixed column measures; Fiber modules is `Section` + `Grid{Columns: 2}` of `Properties{Align:"left", Head:{Label:"SFP+ module", Chip: cage}}`. The board card is `Section{Title: boardName}` around the `Ports` widget.
- *Alternative: keep Temperatures as compact `Meter` rows with a source column added.* Rejected after the mock: it made Temperatures the one card without column heads, against DESIGN.md's single table look.
- *Alternative: keep each SFP module a `Card{Style:"artifact"}`.* Rejected: a card holding a card. The shell already strips the inner frame, but keeps the inner card's top and bottom air, so the spacing doubles. `Properties` with a `Head` is the Overview's address-family pattern and needs no new widget.

### The meter cell gains ticks, a "no track" state and a figure slot
`TableCell` gains `WarnMark`, `CritMark` (percent of the track, 0 = no tick) and `NoTrack`, the names and meaning the `Meter` widget already uses. The template draws the ticks as the compact `Meter` draws them (2px, a pixel past the track above and below, marigold-deep and crimson), so the track no longer clips its children (the fill already clips itself through `clip-path`). `NoTrack` shows the figure without a track, the way the compact meter shows a reading with no limits. The figure slot widens from `w-10` to a width that holds "100.0 °C".
- *Alternative: a new `headroom` cell kind.* Rejected: it would duplicate `meter` for one difference (the ticks). A field on the existing kind keeps the vocabulary closed.
- The SDK's `TableCell` gains `warn_mark`, `crit_mark` and `no_track` (all optional, `skip_serializing_if` default). Installed plugins that send none render as today.

### The meter column takes the table's slack through the existing measures
The Temperatures table fixes Reading at `MeasureName`, Source at `MeasureLong` and Limits at `MeasureName`, so the meter column (`MeasureGrow`) takes what remains, the same way other listings give a column the slack. The meter cell also gets a minimum track width, so a narrow window scrolls the table rather than shrinking the bar to a stub.
- *Alternative: a per-column min-width option on `TableColumn`.* Rejected as a schema addition the measures already cover.

### Inventory selectors answer the cards, not the tiles
`profiles/README.md` already promises that leaving a reading out of a profile's array keeps it off the Hardware page; the "All sensors" table broke that promise. The selectors keep it:
- Each reading records whether a profile entry named it (temperatures already do, through `Curated`/`CPU`; power and fan readings gain the same mark where `matchPowerEntry` and the fan match succeed).
- `PageTemps()`: the CPU, then the rest by rank, leaving out SFP chips. On a profiled board that is the profile's entries only; on an unprofiled board every temperature, primaries and zones (`curateGenericTemps`' order) before secondary channels. It replaces `CuratedTemps`.
- `PageRails()`: on a profiled board the profile's rails, the main first, the rest by draw; unprofiled, every channel by draw. It replaces `OtherPowers`.
- `PageFans()`: the same rule, the main fan first, the rest in channel order. It replaces `OtherFans` and `RunningFans`.
- `CPUTemp`, `MainPower` and `MainFan` stay for the Overview card.

Unused selectors are deleted, not kept beside their replacements.
- *Alternative: list every kernel reading on every board, the profile's first.* Rejected: it breaks the profile README's contract, and it puts the DK's unconnected remote diode on the page as a permanent "sensor fault".

### The board card's title and the panel's centring
The title is `profile.Name`, else `board.Model` from the backend's `Board`, which the handler already reads. A board with neither (the backend failed) gets "Ports", so the panel is never under an empty heading. The panel's art is centred by one rule in `sections.css`, scoped to a `Ports` widget inside a page section (`.verso-page-body section[data-verso-section]`). The Overview's Ports card is an `article.verso-overview-card`, which that selector does not reach, so the card keeps its art flush with the listing under it.
- *Alternative: a `Centered` field on the `Ports` widget.* Rejected: placement inside a card is the page's concern, and no plugin needs it.

### The board's facts come from a reader of their own
A new package (name settled in apply, beside `internal/sensors`) reads the facts once per page load, behind a `Root` the tests point at a fixture tree, the way `sensors.Reader` does. Sensors are readings that change; these facts are the box's build. Keeping them apart keeps each package to one reason to change.
- **Processor**: the profile's new optional `processor` field, else the `system` string `ubus call system board` reports (which `openwrt.Board` gains). Cores: the count in `/sys/devices/system/cpu/online`. Top clock: the highest `cpufreq/cpuinfo_max_freq` across CPUs, formatted in GHz, left out when no CPU exposes cpufreq.
  - *Alternative: decode the CPU part from `/proc/cpuinfo` (0xd08 → Cortex-A72).* Rejected: a table of part numbers to maintain, and it names the core, not the SoC people know the box by. The profile knows the SoC; an unprofiled x86 box's kernel string is already the marketing name.
- **Memory**: `MemTotal` from `/proc/meminfo`, through `units.Bytes`. It is the memory the kernel manages, less than what is installed; the fact says what the router has to work with, so it does not round up to a module size.
- **Storage**: the root filesystem's device (or `/rom`'s under an overlay), found through `/proc/self/mountinfo`'s major:minor, then `/sys/dev/block/<maj:min>` to its whole disk, then its `size`. Its kind comes from the disk: `mmcblk*` with `device/type` MMC is eMMC and SD is "SD card", `nvme*` is NVMe, `sd*` and `vd*` are a disk. A root on no block device leaves the fact out.
- **Board name**: `openwrt.Board.BoardName`, which the handler already reads.

The facts stand as two `Properties{Align:"left"}` lists in a `Grid{Style:"facts"}` under the panel, the style an expanded row already uses: two columns where the card is wide, one where it is narrow.

### One grading for the Overview card and the page heading
The Overview card's header grades the hardware in `(*Overview).hardwareHead` from the `OverviewSensors` that `hardwareView` builds. The grading (a toned CPU, else a toned fan) moves into one method on `OverviewSensors` that returns tone and words. The card's header and `handleSystemHardware` both call it, so the two can never disagree. The page passes the words as `pageHeader.Heading` and the tone as `pageHeader.Tone`; with no verdict it passes "Hardware" and `neutral`, as today. This is the masthead tone contract's existing use (a toned heading is a message about now), and it needs no new widget.
- *Alternative: a callout or a band on the page.* Rejected: the masthead tone contract retired banners for this, and outcomes and status speak from the heading.

### The `strip` grid style is removed
Its one caller goes with the tiles. The template branch, its comment and its test case (`grid_test.go`) go. The `Grid` schema keeps its other styles. No plugin used `strip`: the SDK never exposed it.

## Risks / Trade-offs

- [On a profiled board, a reading the profile forgot no longer shows anywhere; "All sensors" used to catch it] → That is the profile README's contract, and the fix is the profile. `sensors` (the CLI) and sysfs still show every channel.
- [The boot disk is found wrongly on an unusual layout (a root on a device-mapper or UBI volume)] → The resolver walks only the plain block-device path; anything else leaves Storage out instead of guessing. Fixture tests cover eMMC, NVMe, a virtio disk, an overlay with `/rom`, and an overlay with no block device.
- [`board-profiles-runtime` moves `profiles/` to `boards/` while this change adds a field] → The field is one optional key; whichever change lands second carries it to the new place, and both READMEs document it.
- [Wider meter column on a phone] → The table scrolls inside its card as every listing does; the page never scrolls sideways.
- [A plugin built against a newer SDK sends `warn_mark` to an older shell] → The shell ignores unknown fields when it decodes, so the cell renders without ticks.
- [Dropping "All sensors" removes the one place a reading's raw kernel channel name (`temp1`, `power1`) stood beside its value] → The Source column keeps the chip and channel. The power and fan channel names ride the Source (`ina234 · 12-40`, `emc2305 · 15-2e fan1`), as the mock shows.

## Migration Plan

No on-device state changes. The page is rendered fresh on each load; an upgrade shows the new layout, and a downgrade shows the old one.
