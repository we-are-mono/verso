# hardware-page Specification

## Purpose

What System › Hardware shows: the router as a board card with its panel, then one card per kind of reading (temperatures, power, cooling, fibre), each reading once, under the profile's name with the kernel's own name beside it.

## Requirements

### Requirement: The page is one card per kind of reading
System › Hardware SHALL show, in this order: the board card, Temperatures, Power, Cooling, then a card per SFP module. A card the board has no reading for does not stand. No reading SHALL appear on the page more than once, and the page SHALL show no tile strip and no separate table of all sensors.

#### Scenario: The Mono Gateway DK
- **WHEN** an operator opens System › Hardware on the DK
- **THEN** the page shows the cards Mono Gateway Development Kit, Temperatures, Power, Cooling, SFP+ module `xfi0` and SFP+ module `xfi1`, in that order
- **AND** the CPU temperature, the USB Power Delivery rail and System Fan 1 each appear once

#### Scenario: A board with no fans
- **WHEN** the board exposes no fan channel
- **THEN** the page has no Cooling card, and the other cards keep their order

### Requirement: The board card names the router and holds its panel
The first card SHALL be titled with the board's name: the profile's `name`, else the model OpenWrt reports. It SHALL hold the device panel, centred across the card's width. A board with no ports shows the card without a panel.

#### Scenario: A profiled board
- **WHEN** the page renders on a board whose profile is `mono,gateway-dk`
- **THEN** the first card is titled "Mono Gateway Development Kit"
- **AND** the DK's rear art stands centred in it

#### Scenario: A board with no profile
- **WHEN** the page renders on a board no profile describes, whose model is "Supermicro H13SAE-MF"
- **THEN** the first card is titled "Supermicro H13SAE-MF" and holds the generated panel, centred

### Requirement: The board card says what the box is made of
Beside the panel, on its right (under it where the card is too narrow for both), the board card SHALL list the facts Processor, Memory, Storage, Boot slot, NOR flash, Clock and Board ID, in one list read down. Boot slot is the A/B slot whose root partition the kernel booted (U-Boot's `set_slot_` scripts against the command line's `root=`, else U-Boot's `slot`), followed by the slot U-Boot boots next when a switch is staged ("A on next boot") and, while a new image is on trial, its boots against U-Boot's limit ("trial boot 2 of 3"). Clock is the RTC the system time was set from at boot, named as its device tree names its part, followed by its backup battery where the RTC reports one ("battery good", "battery low", "battery empty"); a battery running down marks the row in the warning tone. Processor is the profile's `processor`, else the name the kernel reports, followed by the number of cores and, where the kernel reports it, the top clock. Memory is the total the kernel manages. Storage is the size and kind of the disk the firmware boots from, followed, for an eMMC that estimates its own wear, by the band of its rated life it has used ("0–10 % worn"), or "past its rated wear"; a wear the heading speaks marks the row in the heading's tone. NOR flash is the part as the kernel's SPI NOR framework names it, followed by the chip's size, as far as its MTD devices reach; a chip the framework names none reads its size alone. Board ID is the `board_name` (from the device tree, else the firmware's DMI), in mono; it is labelled an ID because it is an identifier firmware images are matched by, not a name a person gave. A fact the board cannot answer is left out.

#### Scenario: The Mono Gateway DK
- **WHEN** the board card renders on the DK, whose profile's `processor` is "NXP LS1046A", with four cores clocked up to 1.6 GHz, 7.7 GiB of memory and a 29 GiB eMMC
- **THEN** it reads Processor "NXP LS1046A · 4 cores · 1.6 GHz", Memory "7.7 GiB", Storage "29 GiB eMMC · 0–10 % worn", Boot slot "B", NOR flash "n25q512a · 64 MiB", Clock "pcf2131 · battery good" and Board ID `mono,gateway-dk`, each size in the binary units every size in the shell is written in

#### Scenario: A board with no profile
- **WHEN** a board no profile describes reports its processor as "AMD Ryzen 7 7700 8-Core Processor" with 16 CPUs
- **THEN** Processor reads "AMD Ryzen 7 7700 8-Core Processor · 16 cores", followed by the top clock when cpufreq reports one

#### Scenario: A boot disk the shell cannot find
- **WHEN** the root filesystem stands on no block device (an overlay in a container)
- **THEN** the card shows no Storage fact, and the other facts keep their places

### Requirement: The heading says when the hardware needs attention
The page heading SHALL grade the hardware as the Overview's Hardware card header does, in the same words: a CPU past its own warn limit reads "<CPU name> is hot" in the warning tone, past its protect limit in the danger tone; a stopped main fan reads "<fan name> stopped" in the warning tone; an eMMC that has used 80 % of its rated life or reports its reserve blocks in warning reads "Storage is wearing out" in the warning tone, and one past its rated life or with its reserve urgent reads "Storage is worn out" in the danger tone; a clock whose backup battery reports low or empty reads "Clock battery is low" or "Clock battery is empty" in the warning tone. The first danger speaks, else the first warning, in that order. With none, the heading reads "Hardware".

#### Scenario: All well
- **WHEN** the DK's CPU reads 48 °C and System Fan 1 runs
- **THEN** the heading reads "Hardware" in the neutral ink

#### Scenario: A hot CPU
- **WHEN** the DK's `cluster-thermal` reads 87 °C with its warn limit at 85 °C
- **THEN** the heading reads "cluster is hot" in the warning tone
- **AND** the Overview's Hardware card header reads the same words in the same tone

#### Scenario: A hot CPU and a stopped fan
- **WHEN** the CPU is past its protect limit and System Fan 1 has stopped
- **THEN** the heading reads "cluster is hot" in the danger tone

#### Scenario: A stopped fan
- **WHEN** the CPU is within its limits and System Fan 1 reads zero with its alarm set
- **THEN** the heading reads "System Fan 1 stopped" in the warning tone

#### Scenario: A worn-out eMMC beside a warm CPU
- **WHEN** the CPU is past its warn limit and the eMMC reports itself past its rated life
- **THEN** the heading reads "Storage is worn out" in the danger tone
- **AND** the Storage fact reads "past its rated wear", marked in the danger tone

### Requirement: A profile chooses the readings; without one, the kernel does
On a board with a profile, each kind card SHALL list the readings the profile names, under the profile's names, and no other. On a board with no profile, each kind card SHALL list every reading of its kind the kernel exposes, under the kernel's names.

#### Scenario: A reading the profile leaves out
- **WHEN** the DK's kernel exposes the unconnected remote diode `tmp431 · 14-4c temp2`, which the profile does not list
- **THEN** no card on the page shows it

#### Scenario: A board with no profile
- **WHEN** a board no profile describes exposes two NVMe drives' Composite and per-die sensors and a GPU edge sensor
- **THEN** Temperatures lists all of them, each under the name the kernel gives it

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

### Requirement: A temperature row shows its headroom against its own limits
Each temperature row SHALL show the Source the reading comes from in mono (the thermal zone's type, or the chip and channel), and a bar that runs from zero to the sensor's protect limit, with a tick at its warn limit and at its protect limit, the reading beside it. The Limits column SHALL say both limits. A reading with no limits has no bar and says so.

#### Scenario: A zone with trip points
- **WHEN** `ddr-thermal` reads 47.0 °C and warns at 85 °C and protects at 95 °C
- **THEN** its Source reads `ddr-thermal`, its bar fills about half its track with ticks at 85 °C and 95 °C, and 47.0 °C stands beside it
- **AND** its Limits read "warns 85 °C · protects 95 °C"

#### Scenario: A reading past its warn limit
- **WHEN** a reading passes its warn limit
- **THEN** its bar takes the warning tone, and past its protect limit the danger tone

#### Scenario: No limits
- **WHEN** a GPU edge sensor reports a temperature and no limits
- **THEN** its row shows no bar, and its Limits read "no limits reported"

#### Scenario: A faulted channel
- **WHEN** a remote-diode channel has its fault flag set
- **THEN** its row reads "—" for the temperature and "sensor fault" for its limits, in the muted tone

### Requirement: Power lists every rail, the input first
The Power card SHALL be one table with the columns Rail, Source, Voltage, Current and Power, one row per rail, not counting the optical power an SFP module reports. The rail the profile flags `main` SHALL lead, tagged "input". The other rails follow, sorted by draw, highest first. The card's heading SHALL carry only its title, with no total or count.

#### Scenario: The DK's rails
- **WHEN** the DK's USB Power Delivery rail draws 9.8 W at 20.10 V and 0.49 A and seven other rails report
- **THEN** the first row is USB Power Delivery tagged "input", reading 20.10 V, 0.49 A and 9.8 W
- **AND** the seven rails follow, the one drawing most first
- **AND** the card's heading reads "Power" and nothing beside it

#### Scenario: A sub-watt channel
- **WHEN** a rail draws 0.85 W
- **THEN** its Power reads "852 mW"

#### Scenario: No profiled input
- **WHEN** no profile flags a main rail
- **THEN** every power channel is listed by draw and none is tagged "input"

### Requirement: Cooling lists every fan
The Cooling card SHALL be one table with the columns Fan, Source, Speed, Duty and Status, one row per fan channel, the profile's main fan first. Status is the last column: Running, Stalled, or Not connected.

#### Scenario: Two running fans
- **WHEN** System Fan 1 spins at 2,622 rpm at 30 % duty and System Fan 2 at 2,588 rpm
- **THEN** Cooling shows both, System Fan 1 first, each reading its rpm and duty, and Running

#### Scenario: One fan
- **WHEN** the board has one fan channel
- **THEN** Cooling still stands, with that fan's row

#### Scenario: A stalled fan
- **WHEN** a fan reads zero with its fault or alarm set
- **THEN** its Speed reads "—" and its Status reads Stalled in the warning tone

#### Scenario: An unpopulated header
- **WHEN** a fan channel reads zero with no alarm
- **THEN** its Speed reads "—" and its Status reads Not connected, in the muted tone

### Requirement: Each SFP module is a card
Each SFP cage that holds a module SHALL have a card of its own, titled "SFP+ module" with the cage's name as a chip in the card's header. The modules' cards stand side by side where the page has room for two columns, and stack where it has not. Its facts are one list: the link verdict, transmit and receive power (dBm, then the raw power), temperature, supply and laser bias.

#### Scenario: Two modules
- **WHEN** cages `xfi0` and `xfi1` hold modules
- **THEN** the page shows two cards, "SFP+ module `xfi0`" and "SFP+ module `xfi1`"

#### Scenario: A dark receiver
- **WHEN** the `xfi1` module's receive power is under its light floor
- **THEN** its Link reads "No signal" in the warning tone and its Receive reads its dBm and "no light"
