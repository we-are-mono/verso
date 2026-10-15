# Tasks

## 1. Meter cell: ticks, no track, figure slot

- [x] 1.1 Write failing tests in `internal/widget/meter_cell_test.go`: a meter cell with `WarnMark`/`CritMark` draws two ticks at those positions in the marigold-deep and crimson tones; `NoTrack` with a Text draws the figure and no track; a cell with neither renders exactly as today; the figure "100.0 °C" stays on one line in its slot
- [x] 1.2 Add `WarnMark`, `CritMark` and `NoTrack` to `TableCell` (`internal/widget/table.go`) and draw them in `templates/table.html.tmpl`: the track stops clipping its children, the figure slot widens, the track keeps a minimum width; verify 1.1 passes and the existing meter-cell and scroller tests stay green
- [x] 1.3 Mirror `warn_mark`, `crit_mark` and `no_track` on the SDK's `TableCell` (`plugins/verso-plugin-sdk/src/lib.rs`) as optional fields, with a conformance case in `tests/conformance.rs` that round-trips them and one that omits them; verify `cargo test` in the SDK crate
- [x] 1.4 Document the meter cell's ticks and no-track state in the table kinds comment (`table.go`), in DESIGN.md's table section, and in the widget schema reference (`docs/plugins.md`); rebuild `verso.css`; verify `make lint` and `make test`, then commit

## 2. Sensors: the page's selectors

- [x] 2.1 Write failing tests in `internal/sensors/page_test.go` for `PageTemps`: on the DK fixture, the CPU first, then the profile's entries by rank, no SFP chip and no unlisted channel (the unconnected `temp2`); on the unprofiled fixture, the generic CPU, then zones and primaries, then secondary channels, no SFP chip
- [x] 2.2 Write failing tests for `PageRails` (profiled: main first, the rest by draw, an unlisted channel absent; unprofiled: every channel by draw, none main) and `PageFans` (profiled: main first, then channel order; unprofiled: every channel)
- [x] 2.3 Mark power and fan readings a profile entry named, alongside the existing temperature marks, and implement `PageTemps`, `PageRails` and `PageFans`; verify 2.1 and 2.2 pass and `sensors_test.go` stays green
- [x] 2.4 Reword the comments on `CPUTemp`, `MainPower` and `MainFan` to say they serve the Overview card and lead the page's cards (deleting `CuratedTemps`, `OtherPowers`, `OtherFans` and `RunningFans` moves to 5.2, where their last caller goes); verify `make lint` and `make test`, then commit

## 3. Board facts

- [x] 3.1 Write failing tests for the board facts reader against fixture trees: the DK (4 CPUs online, cpufreq 1.6 GHz, `MemTotal`, root on `mmcblk0p2` whose disk `device/type` is MMC); an x86 box (16 CPUs, no cpufreq, root on `nvme0n1p2`); a virtio disk (`vda`); an overlay root whose `/rom` is on `mmcblk0p3`; an overlay with no block device (no Storage); verify each fact and each omission
- [x] 3.2 Implement the reader in its own package beside `internal/sensors` (`internal/machine`), with a `Root` the tests set, returning raw units (the page formats them through `internal/units` in 5.2); verify 3.1 passes
- [x] 3.3 Add `System` to `openwrt.Board` from `ubus call system board`'s `system` key, with a case in `backend_test.go`; add the optional `processor` field to the profile (`sensors.Profile`), the DK's `profile.json` ("NXP LS1046A"), and `profiles/README.md`'s field list, with a `LoadProfile` test that reads it and one that leaves it out; verify `make test` (the group commits with the page in 5.5: until the page calls `internal/machine`, `make lint`'s dead-code check rejects it)

## 4. One hardware grading

- [x] 4.1 Write a failing test in `internal/widget` for the grading method on `OverviewSensors`: a CPU toned warning reads "cluster is hot" warning; danger reads it danger; a toned CPU beside a toned fan names the CPU; a toned fan alone reads "System Fan 1 stopped" warning; neither returns no verdict
- [x] 4.2 Move the grading out of `(*Overview).hardwareHead` into that method and have the card's header call it; verify 4.1 passes and the Overview card tests (`overview_cards_test.go`, `overview_hardware_test.go`) stay green unchanged; verify `make lint` and `make test`, then commit

## 5. The page: board card, kind cards and heading

- [x] 5.1 Rewrite `internal/server/hardware_test.go` first, against `specs/hardware-page/spec.md`, on the DK and unprofiled fixtures: the card order; the board card titled with the profile name, else the model, else "Ports", with its four facts and an omitted Storage; each reading appearing once; Temperatures' columns, ticks, limits text, no-limits and fault rows; Power's "input" tag on the first row and a heading with no meta, no "input" when unprofiled; Cooling with one fan, a stalled fan and an unpopulated header; the fibre lists headed "SFP+ module" with the cage chip and no `data-verso-artifact`; no "All sensors" and no strip; the heading "Hardware" when well, "cluster is hot" in the warning tone when hot, "System Fan 1 stopped" when the fan stops; verify the tests fail against today's page
- [x] 5.2 Rebuild `hardwareBody` and its builders from `Section`, `Table`, `Properties` and `Grid` on the new selectors and the facts reader, with the board card from `profile.Name`/`board.Model`, and pass the grading's words and tone to `pageHeader`; delete `hardwareVitals`, `cpuTile`, `powerTile`, `fanTile`, `fanStripTile`, `hardwareReadings`, `bandReadings` and the readings-row helpers nothing calls, then `sensors`' `CuratedTemps`, `OtherPowers`, `OtherFans` and `RunningFans`, moving their tests in `inventory_test.go` onto the page selectors; verify 5.1 passes and `make lint` finds no unused code
- [x] 5.3 Centre the panel's art inside a page section's `Ports` widget in `internal/server/assets/sections.css`, with a guard test that the rule's selector reaches `.verso-page-body section[data-verso-section]` and not `.verso-overview-card`; rebuild `verso.css`; verify on `make dev` that System › Hardware centres the art and the Overview's Ports card does not
- [x] 5.4 Add and remove the page's strings in `i18n/sl/base.json` ("input", "Limits", "Cooling", "Source", "Stalled", "Not connected", "Processor", "Storage", "Board name", "%d cores", the limits phrasing; drop "%d rails · sorted by draw" and "1 rail · sorted by draw"); verify `make i18n-audit` reports none of this page's strings missing (it already reported "Cilj", "Jedro" and "WAN" before this change: the parked double-translation item)
- [x] 5.5 Update `profiles/README.md` ("Flags pick the headlines" names the page's cards, not its tiles), ADR-022's paragraph on the `cpu`/`main` flags, and DESIGN.md's Hardware page description (cards, board facts, toned heading); verify `make lint` and `make test`, then commit (the commit stages the rebuilt `verso.css`)

## 6. The strip grid style goes

- [x] 6.1 Remove the `strip` branch from `internal/widget/templates/grid.html.tmpl` and `"strip"` from the style list in `grid_test.go`; verify no Go or Rust source names the style (`grep -rn '"strip"'` finds only the unrelated days strip in `field_a11y_test.go`), `make lint` and `make test`, then commit

## 7. Integration

- [ ] 7.1 On `make dev`, open System › Hardware in light and dark themes and at phone width: the unprofiled dev box shows its board card with Processor, Memory and Board name (Storage absent under the container's overlay), Temperatures, and no empty cards; no horizontal page scroll. Then install the build on the bench DUT (`make apk APK_GOARCH=arm64`, served over LAN) and check the DK page against the approved mock, including Storage reading the eMMC
- [ ] 7.2 Update `docs/BACKLOG.md`: reword "Live readings on System › Hardware" to name the Temperatures, Power and Cooling cards; add the narrow-screen stacking of the fibre lists and the SFP module identity (EEPROM through `verso-rpcd`) as open items; verify `make lint`, then commit

## Workflow follow-up

- Archive the change once the bench DUT check passes, confirming ADR-005, ADR-022, DESIGN.md, `profiles/README.md` and docs/BACKLOG.md reflect what landed.
