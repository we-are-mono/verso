# Spec Delta

## Purpose

How Verso draws a reading over time (temperatures, the Internet's traffic, a device's usage): one look for every chart, wherever it stands, so a reader learns it once.

## ADDED Requirements

### Requirement: A plot is four squares tall
A chart's plot SHALL be ruled in square cells and stand four cells tall, so each of its four horizontal rules is a quarter line of its scale. The cell SHALL be 32px on a page's card and 24px in an Overview card or a drawer.

#### Scenario: On a page's card
- **WHEN** an operator opens System › Hardware with the Temperatures graph showing
- **THEN** the plot is 128px tall, ruled every 32px across and down

#### Scenario: In an Overview card
- **WHEN** an operator opens the Overview with the Traffic card open
- **THEN** each lane's plot is 96px tall, ruled every 24px across and down

#### Scenario: In a drawer
- **WHEN** an operator opens a device's drawer that shows its Usage graph
- **THEN** each lane's plot is 96px tall, ruled every 24px across and down

### Requirement: A plot's width is whole quarters of cells
A plot's width SHALL be the widest whole number of quarters, each a whole number of cells, that fits beside its 64px scale gutter, so every time label stands on a vertical line. The plot SHALL draw a line at its right end, and its left edge SHALL be a line.

#### Scenario: A page card 960px wide
- **WHEN** the Temperatures card leaves 960px for the plot and its gutter
- **THEN** the plot is 896px wide, 28 cells of 32px, with lines at 0, 224, 448, 672 and 896px

#### Scenario: A narrower card
- **WHEN** an Overview card leaves 448px for the plot and its gutter
- **THEN** the plot is 384px wide, 16 cells of 24px

### Requirement: The rules stand on the page's lines
Every rule and line a plot draws SHALL stand on the page's 8px grid, its top rule on the line the plot starts on.

#### Scenario: Grid on
- **WHEN** a developer turns the grid on over the Hardware page
- **THEN** each of the plot's horizontal rules coincides with a grid line

### Requirement: The scale is round and written in its unit
A chart's scale SHALL start at zero and top out at a round value that clears its tallest point by at least a tenth, chosen as the One Rate Rule chooses. Its top label SHALL carry the unit and the labels under it figures in that unit. A rate chart writes rates; a temperature chart writes °C.

#### Scenario: A day of the CPU
- **WHEN** the CPU's day peaks at 59.0 °C
- **THEN** the scale reads 80 °C, 60, 40, 20 from the top

#### Scenario: A day of downloads
- **WHEN** the download lane's day peaks at 57 Mb/s
- **THEN** its scale reads 80 Mb/s, 60, 40, 20

### Requirement: The time axis is labelled at the quarters
Under the plot, the window's quarters SHALL be labelled on their lines with how long ago each stands, as the One Duration Rule writes it, the last one "now" (24 h · 18 h · 12 h · 6 h · now for a day). Every other cell column SHALL draw a line with no label. A live window SHALL label only its two ends.

#### Scenario: A week
- **WHEN** the operator picks Last 7 days
- **THEN** the axis reads 7 d · 5 d 6 h · 3 d 12 h · 1 d 18 h · now, each on its line, with unlabelled lines between

#### Scenario: The live minute
- **WHEN** the Traffic card shows the live minute
- **THEN** the axis reads "60 s ago" and "now" at its ends, and the vertical lines travel with the curve

### Requirement: Axis labels are quiet
Time and scale labels SHALL be set alike: the sans face at 12px, weight 500, in Meta, on a 24px line, with tabular figures. A scale label stands centred on its rule in a gutter right of the plot, and nothing is laid over the curve.

#### Scenario: Reading the axes
- **WHEN** the Temperatures graph renders
- **THEN** "80 °C" and "24 h" share one face, size, weight and colour

### Requirement: The legend says the series, its figure and its peak
Above each plot, a legend line SHALL show the series' packet square in its hue, its name in Meta at 400, its figure in Ink at 500, a Meta word saying what the figure is ("now" live, "on average" for a past window), and at the plot's right edge "peak" in Meta with the window's highest reading in Ink at 500. All are 14px sans.

#### Scenario: A past window
- **WHEN** the CPU's last 24 hours average 48.7 °C and peak at 59.0 °C
- **THEN** the legend reads "CPU 48.7 °C on average" and, at the plot's right edge, "peak 59.0 °C"

#### Scenario: The live minute
- **WHEN** the Traffic card shows the live minute
- **THEN** each lane's legend shows its newest rate followed by "now", and its peak over the minute

### Requirement: The peak is marked in the plot
A plot SHALL draw a dotted 1px line in its series' hue across its width at the window's peak.

#### Scenario: Matching the legend
- **WHEN** the legend says "peak 59.0 °C"
- **THEN** a dotted line crosses the plot at 59.0 °C on its scale

### Requirement: Gaps, reboots and outages stay visible
A minute nothing was recorded SHALL break the line. A reboot SHALL draw a dashed rule in Meta at its moment, and an outage a crimson wash over its span, both behind the curve.

#### Scenario: A shell restart
- **WHEN** the history has no CPU reading for 20 minutes at 10:40
- **THEN** the line stops at 10:40 and starts again at 11:00

#### Scenario: A reboot
- **WHEN** the router started at 03:10
- **THEN** a dashed rule stands at 03:10, titled "Router started at 03:10"

### Requirement: Two directions stand in two lanes
A chart of download and upload SHALL draw two lanes, download over upload, each with its own legend, scale and peak, 24px apart, and one time axis under the upload lane.

#### Scenario: A lopsided line
- **WHEN** a day's download peaks at 57 Mb/s and its upload at 8.5 Mb/s
- **THEN** the download lane's scale tops out at 80 Mb/s and the upload lane's at 10 Mb/s, each curve filling its own lane

### Requirement: A graph's own choices are a form field over it
A choice that governs one graph alone SHALL be a labelled form field standing over the graph, its controls 32px, never on its card's heading. A choice that governs a whole card SHALL stand in that card's header.

#### Scenario: The Temperatures graph
- **WHEN** the Hardware page renders
- **THEN** a field labelled "Show chart for" stands over the graph, reading [CPU ▾] over [Last 24 hours ▾]
- **AND** the card's heading carries no control

#### Scenario: Choosing changes the graph alone
- **WHEN** the operator picks Memory in that field
- **THEN** only the graph and its legend redraw, for Memory, and the page does not reload

### Requirement: A live chart glides
A live chart SHALL draw the server's minute first, then slide each new sample in from the right, the curves and the vertical lines moving together. Its scale SHALL change only when the readings cross a step. Under reduced motion it SHALL step instead of slide.

#### Scenario: A sample lands
- **WHEN** a new traffic sample arrives
- **THEN** both lanes slide one sample left over a second, their vertical lines with them

#### Scenario: Reduced motion
- **WHEN** the reader prefers reduced motion
- **THEN** each sample redraws the lanes in one step, with no slide
