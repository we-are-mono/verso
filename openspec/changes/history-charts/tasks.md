# Tasks

## 1. Approval

- [x] 1.1 Put the `Field.Fetch` case (design.md, decision 8) to the owner: a shell-only Field mode that fetches a view instead of staging a write, for the "Show chart for" field. Verify: the owner's answer is recorded in design.md. If declined, revise decision 8 and its tasks before group 8 starts.

## 2. Scales and history windows

- [x] 2.1 Write `internal/units` tests for `Scale(Celsius, peak)`: a 59.0 °C peak gives 80 °C · 60 · 40 · 20; a 0 peak still gives a drawable scale; the rate cases keep today's `RateScale` output. Then add the `Unit` value and `Scale`, with `RateScale` as the rate case. Verify: `go test ./internal/units`.
- [x] 2.2 Write a test pinning `historyWindows` (day, week, month, year → resolution, period count, span, label), then move `trafficWindows` to `historyWindows` in `internal/server` and point `pastTraffic` at it. Verify: `go test ./internal/server -run 'Traffic|HistoryWindows'` passes unchanged.

## 3. The chart: square grid, axis, legend, peak

- [x] 3.1 Write `chart_test.go` cases: `PlacePage` renders `--chart-cell` 32px and `PlaceCompact` 24px; no rules box or gutter carries `-mt-px`; a 24 h `Span` renders the axis "24 h · 18 h · 12 h · 6 h · now"; a 7 d span renders "7 d · 5 d 6 h · 3 d 12 h · 1 d 18 h · now"; `Live` renders only "60 s ago" and "now". Then replace `Card`/`Short`/`AxisStart`/`AxisEnd` with `Place`, `Span` and `Live` in `chart.go` and `chart.html.tmpl`. Verify: `go test ./internal/widget -run Chart`.
- [x] 3.2 Write tests that a series with `Name`, `Figure`, `Caption` and `Peak` renders the legend line (packet square in its hue, name, figure, caption, "peak" with its figure) and a dotted peak `<line>` at the peak's y; and that `Unit: Celsius` writes the scale and figures in °C. Then implement them in the Chart template and view. Verify: `go test ./internal/widget -run Chart`.
- [x] 3.3 Add the chart CSS to the input stylesheet: `--chart-cell`, plot height `cell × 4`, the square rules (two 1px gradients plus the right-end line), the snapped width `round(down, 100% - 4rem, cell × 4)` shared by the legend and axis, axis labels in sans 12px/500 Meta on 24px lines, the peak line and the dotted style. Rebuild `internal/server/assets/verso.css`. Verify: the 8px-grid guard test passes, and the Hardware page measured in the dev container puts every rule on a grid line (playwright measurement, as in the mock check).
- [x] 3.4 Rewrite DESIGN.md's chart paragraphs (the Traffic card graph in the Overview cards section, and the 16px-rule wording) to the square grid, legend, peak, axis labels and lanes. Verify: no DESIGN.md sentence still says the plot is ruled every 16px or that labels are mono.

## 4. Traffic in two lanes, live

- [x] 4.1 Write `traffic_test.go` cases: Traffic renders two Charts (download in Denim with no axis, upload in Amethyst with the axis), each with its own scale (57 Mb/s → 80 Mb/s; 8.5 Mb/s → 10 Mb/s); a past window's legends say "on average", the live minute's "now"; `Panel` and the Overview card both render `PlaceCompact`. Then rebuild `Traffic` as two lanes and drop `Title`, `Meta` and the sand variant. Verify: `go test ./internal/widget -run Traffic`.
- [x] 4.2 Feed the peaks: `pastTraffic` reads the highest `Peak` of rx and tx over the window, and the live minute's peak is its highest sample. Test it in `overview_traffic` tests. Verify: `go test ./internal/server -run Traffic`.
- [x] 4.3 Write a `scripts/*.test.cjs` test for the live layer's pure parts (per-lane scale crossing a step, the peak), then rework `verso-chart.js`: two `[data-verso-live-lane]` plots per chart, a scale per lane, the grid standing still under the gliding curve (owner's call: the live minute is labelled at its quarters, 60 s · 45 s · 30 s · 15 s · now), the lane's figure, peak and peak line rewritten each sample, and stepping under reduced motion. The lane names are keyed in context (`Download|traffic`) so they never take the button verb's translation. Verify: `node --test scripts/*.test.cjs` and, in `make dev`, the Overview's live minute glides both lanes with no console errors.
- [x] 4.4 Point the device drawer's Usage graph (`device_panel.go`) at the lanes with `PlaceCompact`, with a test that the drawer's markup holds two lanes. Amend ADR-018 in place to say the device's graph is two lanes. Verify: `go test ./internal/server -run DevicePanel` and, in `make dev`, a device drawer shows the lanes.

## 5. The Traffic card's header control

- [x] 5.1 Write overview tests: the Traffic card header renders the compact window select before the open-close button, hidden under Arrange mode, with `hx-target=#overview-traffic-graph`; no other card renders a control. Then add `control` to `overviewCardSpec` and the header template, and give the Traffic graph block its id. Verify: `go test ./internal/widget -run Overview`.
- [x] 5.2 Write a `verso-overview.js` test that a pointer or keyboard event starting in `[data-overview-card-control]` does not toggle the card, then add the guard. Verify: `node --test scripts/*.test.cjs` and, in `make dev`, choosing a window leaves the card open.
- [x] 5.3 Make `handleOverviewTraffic` answer the graph block for the header's target, with a test that a `range=day` request returns two lanes and no window select. Amend ADR-022 in place for the header control. Verify: `go test ./internal/server -run OverviewTraffic`.

## 6. Temperature series identity

- [x] 6.1 Write `internal/sensors` tests that `ResolveAll` fills `TempReading.Series` with the name `SeriesNames` gives (including a shared kernel name told apart by its chip, and an SFP module's temperature), then add the field and fill it once in `ResolveAll`; point the history collector's temperature source at it. Verify: `go test ./internal/sensors ./internal/server -run 'Series|Temperature'`.

## 7. The fetching field (after 1.1)

- [ ] 7.1 Write field tests: a select Field with `Fetch` renders `hx-get`, `hx-include` of its joined row and `hx-target`, and no `data-verso-change-field`/`data-verso-writes`; a Field without `Fetch` renders as today; `Fetch` never appears in the widget JSON. Then implement `Field.Fetch` in the field and grid templates. Verify: `go test ./internal/widget -run 'Field|Grid'`.

## 8. The Temperatures graph

- [ ] 8.1 Write `hardware_test.go` cases with the DK fixture: the Temperatures card renders the "Show chart for" field ([CPU ▾] over [Last 24 hours ▾]; sensor options in table order, then "SFP+ module xfi0" and "SFP+ module xfi1"; option values are series names), then the graph, then the unchanged table; the card's heading carries no control. Then compose it in `temperaturesCard`. Verify: `go test ./internal/server -run 'Hardware|Temperature'`.
- [ ] 8.2 Write route tests for `GET /system/hardware/temperature`: a known sensor and window answer the graph block with the window's averages and peak ("48.7 °C on average", "peak 59.0 °C" from a seeded store); an unknown sensor or window answers 400; a series with no points answers the empty message; no history store answers that the router keeps none. Then add the handler and route. Verify: `go test ./internal/server -run TemperatureHistory`.
- [ ] 8.3 Make the page's first paint draw the CPU over 24 hours inline, with a test that the page's HTML already holds the CPU's lane. Amend ADR-019 in place to name the temperature graph as a reader of the history. Verify: `go test ./internal/server -run Hardware`.
- [ ] 8.4 Update the `hardware-page` paragraphs of DESIGN.md (the Temperatures card's field, graph and table order). Verify: DESIGN.md describes the field's words "Show chart for" and "over".

## 9. Integration

- [ ] 9.1 In `make dev`, check the Hardware page, the Overview (live minute and a past window) and a device drawer in light and dark, against the mock. Measure the rules, legend, plot and axis on the 8px grid with playwright. Verify: every measured edge is on a line, and the screenshots match the mock's settled look.
- [ ] 9.2 Update docs/BACKLOG.md: remove what this change closed, and add any follow-up it left (for example, listing recently seen SFP modules). Verify: the backlog names no chart item that has landed.
- [ ] 9.3 Run `make lint` and `make test`, then `graphify update .`. Verify: both pass green and the graph is updated.

## Workflow follow-up

- Archive the change once it is reviewed on a router, confirming that ADR-018, ADR-019 and ADR-022 and docs/BACKLOG.md say what landed.
