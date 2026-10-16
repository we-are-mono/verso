## MODIFIED Requirements

### Requirement: One verdict heads the page
The Overview SHALL open with one verdict sentence tinted by tone, the most important true sentence on the page. With the uplink known and down it SHALL read "Your network is offline" (danger), whatever else is wrong. Otherwise, with exactly one cause it SHALL read that cause as a sentence that stands on its own, in the cause's tone; with two or more it SHALL read "N things need attention", in danger when any cause is danger and warning otherwise; with none and the uplink up it SHALL read "Your network is online" (success); and with nothing known it SHALL read "Your network at a glance" (neutral). A cause is a firewall not fully loaded, a port a network uses or a tunnel without link, a Resources reading out of its normal band, or a Hardware reading past the board's own limit; every card counts, hidden or not. The router's live clock and time zone stand in the System card as Local time, not beside the verdict. The page shows no kicker over the verdict and no status-tile strip.

#### Scenario: The firewall is not running
- **WHEN** the uplink is up, fw4's ruleset is not loaded in the kernel, and nothing else is wrong
- **THEN** the verdict reads "The firewall is not running" in the danger tone, though no card shows the firewall

#### Scenario: A problem anywhere
- **WHEN** the uplink is up, one port a network uses has lost link, and nothing else is wrong
- **THEN** the verdict reads "1 port without link" in the warning tone

#### Scenario: One problem names itself
- **WHEN** the uplink is up and storage enters its warning band, and nothing else is wrong
- **THEN** the verdict reads "Storage is high" in the warning tone

#### Scenario: A tunnel down reads without its card
- **WHEN** the uplink is up, one of two tunnels has lost link, and nothing else is wrong
- **THEN** the verdict reads "1 of 2 tunnels down" in the danger tone, while the Tunnels header reads "1 of 2 down"

#### Scenario: Several problems are counted
- **WHEN** the uplink is up, one port a network uses has lost link, and storage enters its warning band
- **THEN** the verdict reads "2 things need attention" in the warning tone

#### Scenario: Offline outranks every cause
- **WHEN** the uplink is down and storage is high
- **THEN** the verdict reads "Your network is offline"

#### Scenario: A hot board on a healthy network
- **WHEN** the uplink is up, nothing else needs attention, and the Hardware header reads "cluster is hot"
- **THEN** the verdict reads "cluster is hot" in the warning tone

### Requirement: Hidden cards are neither rendered nor streamed
The shell SHALL NOT render a hidden card, and the Overview's live stream SHALL NOT send a frame that only hidden cards would read, nor take a reading that only such a frame needs. Readings the verdict needs are taken whatever is hidden. Closed cards still receive their frames, because their summaries are live.

#### Scenario: Resources hidden
- **WHEN** Resources is hidden
- **THEN** the live stream sends no system-meter frames

#### Scenario: A hidden card's problem still reaches the verdict
- **WHEN** Resources is hidden and memory enters its warning band, and nothing else is wrong
- **THEN** the verdict reads "Memory is high"
- **AND** in Arrange mode the shelf shows Resources with the warning packet

#### Scenario: Resources closed
- **WHEN** Resources is closed but not hidden
- **THEN** the live stream keeps sending system-meter frames, and the closed header's summary updates

#### Scenario: Hardware hidden
- **WHEN** Hardware is hidden
- **THEN** the live stream sends no sensor frames, and the shell still reads the sensors for the verdict
- **AND** in Arrange mode the shelf shows Hardware in its header's tone

## REMOVED Requirements

### Requirement: The Hardware card stays out of the verdict
**Reason**: The heading names the cause rather than speaking for the network alone, so a board past its own limits is a cause like any other.
**Migration**: None; the verdict reads Hardware's cause as it reads every card's.
