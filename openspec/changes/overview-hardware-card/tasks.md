# Tasks

## 1. Profile order in the sensor inventory

- [x] 1.1 The card reads the inventory's existing headline picks (`CPUTemp`, `MainPower`, `MainFan`), so the inventory is unchanged. The profile-order accessor built first (`Entry`, `Profiled()`) was removed once the card narrowed to three rows. Verify that `go test ./internal/sensors/...` passes.
- [x] 1.2 Folded into 1.1.

## 2. A card set that depends on the board

- [x] 2.1 Write layout tests in `internal/widget/overview_layout_test.go` first:
  - the default layout includes `hardware` at the end of the right column
  - a cookie naming `hardware` keeps it where it stood
  - a cookie naming `firmware` reads as `system`
  - an old cookie without `hardware` appends it open to the shorter column
  - `Without("hardware")` takes it out of its column, the closed cards and the shelf, and every other card keeps its place

  Then add `hardware` to `overviewDefault`, reduce `overviewMerged` to `firmware → system`, and add `OverviewLayout.Without`. Verify that `go test ./internal/widget/...` passes.
- [x] 2.2 Write a `cookieLayout` test: with `hardware` the layout keeps the card; without it, the card is dropped from the default and from a saved cookie. Then load the profile once at Server construction (`boardProfile`), and have `cookieLayout{hardware}` apply `Without("hardware")` when the profile lists no sensors. Verify that `go test ./internal/server/...` passes.

## 3. The shared sensor sampler

- [x] 3.1 Write `sensorSampler` tests with an injected clock and a counting reader:
  - two reads within five seconds call the reader once
  - a read after five seconds calls it again
  - concurrent reads on a miss call it once

  Then implement `sensorSampler` on the Server. Verify that `go test -race ./internal/server/...` passes.
- [x] 3.2 Time `ResolveAll` on the DK bench. Run the read in a loop on the router and record the median in the design's Risks entry. If it exceeds 100 ms, add a test that a miss serves the previous inventory while the refresh runs, then implement that. Verify that the recorded figure is in `design.md`.

## 4. The Hardware card

- [x] 4.1 Write `overview_cards_test.go` cases for `hardwareHead` first:
  - in band: no status, CPU summary
  - warm CPU: "cluster is hot", warning
  - critical CPU with a stopped fan: "cluster is hot", danger
  - fan fault alone: "System Fan 1 stopped", warning
  - no limits at 95 °C: silent (the server gives a CPU with no limits no tone, `TestHardwareView`)
  - no reading answered: "Unavailable", neutral

  Then add the `hardware` card spec (icon `cpu`, `Opens: "Hardware"`, href `/system/hardware`), `Overview.Hardware`, and `hardwareHead`. Verify that the tests pass.
- [x] 4.2 Write a render test for `hardwareBody` on the DK view: one `Properties` with no head and the default alignment, rows CPU, power, fan in that order keyed `hw-cpu`, `hw-power`, `hw-fan`, the power value "9.8 W · 20.10 V · 0.49 A", and no fan row when the view has none. Then implement it. Verify that the test passes and, on the DK (the dev container has no profile), that the card shows three rows with "USB Power Delivery" on one line.
- [x] 4.3 Write a test that the server formats the sampler's inventory into `Overview.Hardware` with the Hardware page's formatters (`celsiusWhole`, `powerText`, `rpmGroup`), and that the page render fills it only while the layout shows `hardware`. Then implement it. Verify that the test passes.
- [x] 4.4 Write a test that `issues()` ignores a Hardware view with a critical temperature, so the verdict stays "online". Verify that it passes with no change to `issues()`.
- [x] 4.5 Add the card's strings ("Hardware", "Thermal", "Power", "Fans", "%s is hot", "%s stopped", "No sensor answered.") to `i18n/sl/base.json`. Verify that the i18n audit test reports zero missing keys.

## 5. The live `sensors` frame

- [x] 5.1 Add stream test cases first:
  - `sensors` is sent while `hardware` is shown, open or closed
  - it is not sent, and the sampler is not read, while `hardware` is hidden
  - its `rows` carry each keyed value, and the `overview` frame carries Hardware's head

  Then add the frame to `handleOverviewEvents`, sent each tick from the sampler's cached read. Verify that `go test ./internal/server/...` passes.
- [ ] 5.2 Add the client's `sensors` handler in `internal/server/assets/verso-stream.js`, which writes each `rows` entry's text to `[data-verso-prop="<key>"]`. Verify on `make dev`: with the Hardware card open, a temperature row changes within five seconds without a reload, and the closed header's CPU summary updates.

## 6. Documents

- [x] 6.1 Amend ADR-022 in place:
  - the Overview is seven cards, Hardware standing only on a profiled board
  - the board's sensors stand in the Hardware card
  - the `sensors` frame, its five-second sampler, and what it skips while hidden
  - `hardware` is a card id again, and only `firmware` merges into System
  - Hardware's status stays out of the verdict

  Verify that the ADR reads with no line contradicting the spec.
- [x] 6.2 Update `DESIGN.md` "Overview cards" (the card list, and the Hardware card's sections as fact lists). In `profiles/README.md`, rewrite "Flags pick the home-dashboard headlines": every listed entry shows in the Hardware card in array order, `cpu` names its closed line, and `main` lifts the Hardware page's tiles. Verify that both documents name the same card set as the spec.
- [x] 6.3 Update `docs/BACKLOG.md`: remove anything this change lands and add any follow-up it leaves open. Verify that the diff touches only what this change affects.

## 7. Integration

- [ ] 7.1 Run `make lint` and `make test`. Then on `make dev`:
  - a fresh browser shows Hardware at the end of the right column
  - hiding Hardware stops the `sensors` frames (browser network panel)
  - Reset layout restores it
  - Hardware's link opens `/system/hardware`

  Verify that every item holds.
- [x] 7.2 Run `openspec validate overview-hardware-card --strict` and verify that it reports the change valid.

## Workflow follow-up

- Archive with `/opsx:archive` once the ADR, DESIGN.md, the profile README and `docs/BACKLOG.md` reflect what landed.
