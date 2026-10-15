# Design

## Context

See proposal.md, Why. The settled look is the Temperature History Mock (https://claude.ai/artifact/WZxLD7W84RUgT62MxGMjqj), built from real widget markup. What exists today:

- **`widget.Chart`** (`internal/widget/chart.go`, `chart.html.tmpl`) draws the plot server-side as a stretched SVG, plus HTML scale labels in a 64px gutter and the window's two ends under it.
  - Its scale is `units.RateScale`, so its labels always end in Mb/s or Gb/s.
  - In Card mode it shifts the rules box up a pixel (`-mt-px pt-px`, gutter `-mt-px`). That suited frames that stood a pixel up. Since the 8px grid (2026-10-14) frames sit on lines, so every rule now lands a pixel above its line, the live Overview included.
- **`widget.Traffic`** puts download and upload in one Chart on one shared scale. The window dropdown sits on the figures line and swaps `closest section`. The Overview card and the device drawer (`device_panel.go`, `Panel: true`) both use it.
- **`verso-chart.js`** glides one SVG's two series, rewrites one shared scale, and travels the section's 16px background grid with the curve.
- **History** (`internal/history`) keeps the average and peak per minute, quarter hour, hour and day. Temperatures are recorded as `temp.<SeriesNames>` (`internal/sensors/series.go`, the kernel's names). The Internet graph lays out its past windows with `trafficWindows` (`overview_traffic.go`).
- **`widget.Grid`** with `Join` draws a joined field ("[25] per [Second ▾]"). Its Fields stage UCI writes; none can fetch a view.
- **Overview card headers** (`overviewCardSpec`, `overview.html.tmpl`) carry title, summary, status and buttons. A click anywhere on a header toggles the card (`verso-overview.js`).

## Goals / Non-Goals

**Goals:**
- One chart composition used by all three graphs, with each look rule (square grid, scale, axis, legend, peak) in exactly one place.
- Chart sizing derived from where the chart stands, so callers never pass pixel numbers.
- A scale strategy per unit, so the next chart in a new unit (watts, rpm, %) adds a scale, not a branch.

**Non-Goals:**
- The Narrow Space Rule's code change. It is a separate backlog item, and this change writes units as `internal/units` does today.
- Warn and protect limit lines.
- Hover readouts and crosshairs (the Lanes prototype had them; they were not adopted).
- A plugin-facing chart widget. `Chart` and `Traffic` stay the shell's own compositions.

## Decisions

### 1. Chart owns the whole graph: legend, plot, gutter, axis
`widget.Chart` grows from a plot into a graph: one legend line per series, the plot, the scale gutter and the time axis. A lane is one Chart, and `Traffic` composes two (download with no axis, upload with the axis).
- **Alternative rejected:** a new `History` widget wrapping Chart. It would split the legend's alignment (the peak sits flush with the plot's right edge) from the plot that sets the width, across two widgets.

### 2. Size comes from a place, not pixels
`Chart.Card bool` and `Short bool` give way to `Chart.Place`, either `PlacePage` (32px cells) or `PlaceCompact` (24px cells: Overview cards and drawers). The template sets `--chart-cell` from it. CSS derives the rest:
- the plot height, `calc(var(--chart-cell) * 4)`;
- the rules, two 1px gradients on a `var(--chart-cell)` grid, plus an inset right-end line;
- the plot's width, `grid-template-columns: round(down, 100% - 4rem, calc(var(--chart-cell) * 4)) 4rem`, so every quarter is whole cells.

The legend line and the axis take the same width, so the peak and "now" end at the plot's edge. The pixel-up shift goes. `round()` already ships in `verso.css`, so it adds no new browser floor.
- **Alternative rejected:** computing widths in Go. The server doesn't know the column width, and the layout must reflow with the window.

### 3. A scale per unit
`units.Numbers` gains `Scale(unit, peak) (top float64, labels []string)` behind a small `Unit` value. It is `Rate` (today's `RateScale`, unchanged) or `Celsius` (the same round steps, figures by `Figure`, the top label carrying "°C"). `Chart.Unit` picks the scale, and the legend's figure and peak are written by the same unit. `verso-units.js` keeps only the rate scale, since only traffic is live.
- **Alternative rejected:** callers pre-writing labels. Every caller would then repeat the round-step rule.

### 4. The time axis is computed from the window
`Chart.Span time.Duration` replaces `AxisStart`/`AxisEnd`. Every window, live or past, names its five quarters, from `units.DurationWithin` (the duration rule in no unit larger than a quarter reaches, so a day reads "24 h · 18 h …" and the live minute "60 s · 45 s · 30 s · 15 s · now"), the last one "now". Labels sit at 0/25/50/75/100% of the snapped width, so they fall on lines.
- **Changed during build (owner's call, 2026-10-16):** the grid no longer travels with a live curve. The owner wanted the live minute labelled at its quarters too, and a fixed label can't follow a moving line.

### 5. Legend and peak are series data
`ChartSeries` gains `Name`, `Figure` (already written, with the unit), `Caption` ("now" or "on average") and `Peak` (a value on the scale, plus a written figure). The template draws the legend from them and a dotted peak `<line>` at the peak's y.
- **Temperatures:** each period plots its `Average`, and the window's peak is the highest `Peak` among the points read.
- **Traffic, past windows:** the peak is the highest `Peak` of `iface.<dev>.rx`/`.tx`.
- **Traffic, live minute:** the peak is the highest sample in the minute, kept current by the live layer.

### 6. Traffic is two lanes everywhere
`Traffic` renders download (Denim) over upload (Amethyst), each its own Chart and scale, 24px apart, one axis under upload. `Title`, `Meta` and the sand non-card variant go, since the Overview card and the drawer's Usage section are the only callers and both already supply the title. `Traffic.Card` becomes the Chart's `PlaceCompact`, as does the drawer's `Panel`. The window dropdown leaves Traffic (see 8).

### 7. Live lanes in `verso-chart.js`
`[data-verso-live-chart]` holds two `[data-verso-live-lane]` plots. Each lane keeps its own series, scale (re-written only when a step is crossed) and clip.
- **One sample** slides both lanes' curves over one interval with CSS transforms, as today. The grid stands still (see decision 4), so the time labels keep their lines.
- **Figures:** each lane's figure, peak text and dotted peak line are rewritten through `verso-units.js`.
- **Reduced motion** steps.

### 11. A word in context
A lane's name ("Download") is a noun, but the catalog already keys "Download" as the button's verb (Slovenian "Prenesi"). `i18n.InContext`/`i18n.Contextual` key a word said in a context apart (`"Download|traffic"`) and fall back to the English word, never to the word's other meaning. The owner chose this over renaming the lanes (2026-10-16), since the same collision will recur.

### 8. Where the choices stand
- **Overview Traffic window:** `overviewCardSpec` gains an optional `control` that renders into the header before the open-close button, hidden in Arrange mode like the other header buttons. Traffic's control is the compact window `<select>`, with `hx-get=/overview/traffic`, `hx-target=#overview-traffic-graph` and `hx-swap=outerHTML`. `verso-overview.js` ignores header clicks that start inside `[data-overview-card-control]`, so choosing never toggles the card.
- **Temperatures field:** a `widget.Grid` with `Join: "over"` and two select Fields.
  - **The extension:** this needs a Field mode that fetches a view instead of staging a write. A shell-only `Field.Fetch` (`json:"-"`, so the plugin contract is untouched) names a URL and a target. A fetching field renders `hx-get`, `hx-include` of its row's other fields and `hx-target`, and drops the change-field attributes.
  - **Approval:** APPROVED by the owner (2026-10-16). It follows the precedent of `Field.Theme` (ADR-024): a shell-only field flag, hidden from plugins (`json:"-"`), that gives a select its own behaviour on change. `Field.Theme` itself can't be reused, because it posts the pick to the theme endpoint.
  - **Alternative rejected:** a hand-built template, which goes against "components over hand-built".

### 9. One window table for every history graph
`trafficWindows` becomes `historyWindows` in `internal/server` (day, week, month and year at minute, quarter, hour and day resolution), used by the Internet graph and the temperature graph alike. Each window's label and span come from the same table.

### 10. The temperature graph's route and sensor identity
`GET /system/hardware/temperature?sensor=<series>&range=<window>` answers the graph block (legend, plot, axis, or the empty message).
- **Sensor identity:** sensors are identified by their history series name. `ResolveAll` records each `TempReading`'s series name (`TempReading.Series`, filled once from `SeriesNames`), so the page, the history collector and the route share one source.
- **The options:** `PageTemps()` in order, then each fibre module's temperature, labelled "SFP+ module <cage>".
- **Validation:** the route accepts only a series the current inventory offers, so it never reads an arbitrary series. An unknown sensor or window answers 400.
- **The page's first paint:** it draws the CPU over 24 hours inline, with no second request.

## Risks / Trade-offs

- **[Snapping leaves a sliver]** The snapped plot leaves up to one quarter of cells empty at the card's right (under 128px on a page, under 96px in a card). → The legend, plot and axis snap together, so the sliver reads as margin, not misalignment. This was accepted in the mock.
- **[A header control inside a click-to-toggle header]** → The click guard in `verso-overview.js` gets a test, and the select stops propagation of the keyboard and pointer events that toggle the card.
- **[A day of minutes is 1,440 points per lane]** The SVG paths are large. → LAN bytes are free (the performance budget). Path building is linear in Go and runs only on a window change.
- **[Live scale churn on two lanes]** Upload's small scale crosses steps more often. → Scales still move only on a step crossing, as the One Rate Rule asks.
- **[A Field mode that fetches]** If the owner declines `Field.Fetch`, the temperature field needs another home. → It is approved first, and nothing downstream is built until it is.
- **[Plugins]** None draw `Chart` or `Traffic`, and `Field.Fetch` is shell-only, so installed plugins keep working.

## Migration Plan

No on-device state changes: the history series and their names are as recorded. Rolling back to an earlier release restores the old graphs over the same history.

## Open Questions

- **An unplugged SFP module:** it leaves the sensor list with its history unread. Whether to keep listing modules seen in the last 30 days can wait until someone asks for it; it changes neither the approach nor the tasks.
