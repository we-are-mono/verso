# Proposal

## Why

The router has recorded every temperature each minute since ADR-019, but nothing draws it. An operator can see that the CPU reads 48 °C now but not whether it climbed to 58 °C last night. The one history chart that exists, the Overview's Internet graph, also falls short. Upload shares download's scale, so on a typical home line it is a flat line under a tall one. Its rules sit a pixel off the 8px grid, and its time axis names only the window's two ends. The design for every chart was settled in the Temperature History Mock (https://claude.ai/artifact/WZxLD7W84RUgT62MxGMjqj). This change builds that design once, in the chart widget, so the temperature graph, the Internet graph and a device's Usage graph all follow it.

## What Changes

- **Temperature history on System › Hardware.** The Temperatures card gains a graph above its table. A form field over the graph reads "Show chart for [CPU ▾] over [Last 24 hours ▾]". The sensor list is the table's rows in order, then each SFP module. The windows are 24 hours, 7 days, 30 days and a year. The CPU is selected when the page opens, and the table stays as it is.
- **One chart look, wherever a chart stands:**
  - **Grid:** square cells, and the plot always four squares tall, so every rule is a labelled quarter line. The squares are 32px on a page's card and 24px in an Overview card or a drawer. The plot's width snaps to whole quarters of squares, so the time labels land on their lines.
  - **Time axis:** a line at every cell column, with labels only at the quarters (24 h · 18 h · 12 h · 6 h · now).
  - **Legend:** above the plot, in the definition-list style: the series' packet square, its name in Meta, the figure in Ink 500 ("48.7 °C on average"), and the window's peak at the right edge ("peak 59.0 °C"). A dotted line marks the peak in the plot.
  - **Axis labels:** sans 12px, weight 500, Meta, on the 24px line.
  - **Grid fix:** the 1px offset that put every rule a pixel above its line goes.
- **The Internet graph in two lanes.** Download and upload each get a lane with its own scale, legend and peak, and one time axis under both. The window dropdown moves into the Traffic card's header, since it governs the whole card. A device's Usage graph in its drawer draws the same two lanes.
- **Scales in any unit.** The chart widget's scale is written by the series' unit (rates through the One Rate Rule, temperatures in °C), not hard-wired to Mb/s.

## Capabilities

### New Capabilities
- `charts`: how Verso draws a chart of a reading over time (grid, scale, time axis, legend, peak, lanes, window choice, live glide), wherever it stands: a page card, an Overview card or a drawer.

### Modified Capabilities
- `hardware-page`: the Temperatures card holds a history graph with its "Show chart for" field above its table, so the card is no longer only a table.
- `overview`: the Traffic card's window choice rides the card's header, and its Internet graph is two lanes.
- `layout-grid`: "Chart plots are ruled every 16px" becomes square cells, four squares tall, 32px on a page card and 24px in an Overview card or drawer.
- `palette`: a chart's wash fades to transparent at the plot's bottom border instead of to 4% of its hue.

## Impact

- **ADRs:** ADR-019 (history and events) gains its first temperature reader. ADR-022 (Overview cards) gains a header control on the Traffic card. ADR-018 (per-device usage) draws the device's graph in lanes. ADR-005 is touched because the chart widget's parameters change.
- **Widgets:** `widget.Chart` gains square grid sizing, a unit-agnostic scale, quarter time labels, a peak and a legend. `widget.Traffic` becomes two lanes. The Overview card header gains an optional control. A `widget.Grid` joined field gains a view-only "fetch on change" mode, which is an extension that needs the owner's approval (design.md says why).
- **Code:**
  - `internal/widget` (chart, traffic, overview cards, grid/field templates);
  - `internal/units` (a scale per unit);
  - `internal/server` (`hardware.go`, a temperature-history handler, `overview_traffic.go`, `device_panel.go`);
  - `verso-chart.js` (lanes, a per-lane scale, square-grid travel) and `verso-overview.js` (a header control must not toggle the card);
  - the CSS input (chart grid, end line, axis labels).
- **Specs and docs:** DESIGN.md's chart paragraphs, the `layout-grid`, `overview` and `hardware-page` specs, and docs/BACKLOG.md.
- **Out of scope:**
  - The Narrow Space Rule's code change (U+202F before units), which is its own backlog item.
  - Warn and protect limit lines, which were skipped on purpose.
- **Compatibility:** no on-device state changes. Plugins cannot emit `Chart` or `Traffic` (they are the shell's own compositions), so the plugin contract is unchanged.
