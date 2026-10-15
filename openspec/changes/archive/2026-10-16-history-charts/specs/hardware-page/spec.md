# Spec Delta

## MODIFIED Requirements

### Requirement: Temperatures lists the CPU first
The Temperatures card SHALL hold the temperature history graph, then one table with the columns Reading, Source, Temperature and Limits. Its rows SHALL be the CPU first, then the board's other temperatures: on a profiled board in profile order, on an unprofiled board each chip's primary channel and every zone before secondary channels. A temperature an SFP module reports SHALL stand on its fibre list instead.

#### Scenario: Profile order
- **WHEN** the DK's profile flags `cluster-thermal` as the CPU and lists Memory, Network, SerDes and Crypto after it
- **THEN** the rows read cluster, Memory, Network, SerDes, Crypto, then the profile's remaining entries in order

#### Scenario: An SFP module's temperature
- **WHEN** a module in cage `xfi0` reports its temperature
- **THEN** that temperature shows on the `xfi0` fibre list and not in Temperatures

#### Scenario: The graph stands over the table
- **WHEN** an operator opens System › Hardware on a router with history
- **THEN** the Temperatures card shows its "Show chart for" field, then the graph, then the table, whose rows stay inert

## ADDED Requirements

### Requirement: Temperatures charts one reading at a time
Over its table, the Temperatures card SHALL chart one temperature's history, chosen in a field labelled "Show chart for": the reading, from the table's rows in their order and then each SFP module ("SFP+ module xfi0"), joined by "over" to the window, from Last 24 hours, Last 7 days, Last 30 days and Last year. The CPU and Last 24 hours SHALL be chosen when the page opens.

#### Scenario: Opening the page
- **WHEN** an operator opens System › Hardware on the DK
- **THEN** the field reads [CPU ▾] over [Last 24 hours ▾] and the graph shows the CPU's last 24 hours

#### Scenario: Choosing a module
- **WHEN** the operator picks "SFP+ module xfi1" and Last 7 days
- **THEN** the graph shows that module's temperature over the last 7 days, and the table does not change

#### Scenario: The list follows the table
- **WHEN** the table's rows read CPU, Memory, Network
- **THEN** the reading's choices start CPU, Memory, Network, in that order, then the modules

### Requirement: A temperature's history keeps its kernel name
The graph SHALL read a temperature's history under the name the kernel gives its sensor, so a reading renamed in a profile shows everything recorded before the rename.

#### Scenario: A renamed reading
- **WHEN** a profile renames `cluster-thermal` from "cluster" to "CPU"
- **THEN** the CPU's graph shows its readings from before the rename

### Requirement: A window shows averages and its peak
A temperature graph over a past window SHALL plot each period's average, its legend saying the window's average "on average" and its highest reading as its peak.

#### Scenario: A day with a hot evening
- **WHEN** the CPU averaged 48.7 °C over 24 hours with one minute at 59.0 °C
- **THEN** the legend reads "CPU 48.7 °C on average" and "peak 59.0 °C"

### Requirement: No history is said, not drawn
When the router has recorded nothing for the chosen reading over the window, or keeps no history, the graph SHALL say so in place of a plot, and the field and table SHALL stand as usual.

#### Scenario: A module just plugged in
- **WHEN** the operator picks a module whose temperature has never been recorded
- **THEN** the graph's place reads that nothing was recorded for it over that window
