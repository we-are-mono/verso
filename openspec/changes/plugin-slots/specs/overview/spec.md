# Spec Delta

## MODIFIED Requirements

### Requirement: The Overview shows seven cards in two columns
The Overview SHALL show seven cards, each holding one kind of reading: Internet, Ports, Tunnels, Traffic, Resources, System and Hardware. Hardware stands only on a board that has a profile. Cards stand in two columns, each column stacking its cards at their own heights. The default layout puts Internet, Ports and Tunnels on the left, and Traffic, Resources, System and Hardware on the right. Each card a running plugin contributes to `shell.overview.cards` SHALL stand among them, in the default layout where its position puts it.

#### Scenario: First visit
- **WHEN** a signed-in operator opens the Overview with no layout cookie on a board that has a profile
- **THEN** the left column shows Internet, Ports and Tunnels in that order
- **AND** the right column shows Traffic, Resources, System and Hardware in that order
- **AND** every card is open

#### Scenario: First visit on a board with no profile
- **WHEN** a signed-in operator opens the Overview with no layout cookie on a board that has no profile
- **THEN** the right column shows Traffic, Resources and System in that order, and no Hardware card
- **AND** Arrange mode's shelf never lists Hardware

#### Scenario: A layout from before System
- **WHEN** the layout cookie names Firmware
- **THEN** System stands where Firmware stood, closed or hidden as that card was

#### Scenario: A layout that names Hardware
- **WHEN** the layout cookie names Hardware on a board that has a profile
- **THEN** Hardware stands where the cookie puts it, closed or hidden as the cookie says

#### Scenario: A layout that names Hardware on a board with no profile
- **WHEN** the layout cookie names Hardware on a board that has no profile
- **THEN** Hardware is dropped as a card the Overview does not have, and every other card keeps its place

#### Scenario: Narrow screen
- **WHEN** the page is too narrow for two columns
- **THEN** the cards stand in one column, the left column's cards first, then the right column's

#### Scenario: A plugin card in the default layout
- **WHEN** verso-plugin-acme contributes its Certificate card with position `after:system` and there is no layout cookie
- **THEN** the right column shows Traffic, Resources, System, Certificate and Hardware in that order

### Requirement: One verdict heads the page
The Overview SHALL open with one verdict sentence tinted by tone, the most important true sentence on the page. With the uplink known and down it SHALL read "Your network is offline" (danger), whatever else is wrong. Otherwise, with exactly one cause it SHALL read that cause as a sentence that stands on its own, in the cause's tone; with two or more it SHALL read "N things need attention", in danger when any cause is danger and warning otherwise; with none and the uplink up it SHALL read "Your network is online" (success); and with nothing known it SHALL read "Your network at a glance" (neutral). A cause is a firewall not fully loaded, a port a network uses or a tunnel without link, a Resources reading out of its normal band, a Hardware reading past the board's own limit, or the cause a plugin card states; every card counts, hidden or not. The router's live clock and time zone stand in the System card as Local time, not beside the verdict. The page shows no kicker over the verdict and no status-tile strip.

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
- **WHEN** the uplink is up, one of two tunnels, wg0, has lost link, and nothing else is wrong
- **THEN** the verdict reads "Tunnel wg0 is down" in the danger tone, while the Tunnels header reads "1 of 2 down"

#### Scenario: Several tunnels down
- **WHEN** the uplink is up, two of three tunnels have lost link, and nothing else is wrong
- **THEN** the verdict reads "2 of 3 tunnels down" in the danger tone

#### Scenario: Several problems are counted
- **WHEN** the uplink is up, one port a network uses has lost link, and storage enters its warning band
- **THEN** the verdict reads "2 things need attention" in the warning tone

#### Scenario: Offline outranks every cause
- **WHEN** the uplink is down and storage is high
- **THEN** the verdict reads "Your network is offline"

#### Scenario: A hot board on a healthy network
- **WHEN** the uplink is up, nothing else needs attention, and the Hardware header reads "cluster is hot"
- **THEN** the verdict reads "cluster is hot" in the warning tone

#### Scenario: A plugin card's cause
- **WHEN** the uplink is up, nothing else needs attention, and acme's Certificate card states the cause "The certificate could not be renewed" in the warning tone
- **THEN** the verdict reads "The certificate could not be renewed" in the warning tone, whether the card is open, closed or hidden

### Requirement: A damaged or stale layout falls back safely
The shell SHALL treat the layout cookie as untrusted input. A cookie it can't parse gives the default layout. Card ids it doesn't know are dropped, and cards it has that the cookie doesn't name are added open at the end of the shorter column. A card named twice keeps its first place. The card of a plugin that is installed but not answering SHALL keep its place in the layout, and the card of a plugin that is not installed SHALL be dropped.

#### Scenario: Garbled cookie
- **WHEN** the layout cookie holds text that isn't a layout
- **THEN** the Overview renders the default layout without an error

#### Scenario: A card the cookie doesn't know
- **WHEN** the cookie names six of the seven cards, or names a card the Overview no longer has
- **THEN** the card it leaves out stands open at the end of the shorter column, and the card it names that isn't there is dropped

#### Scenario: A plugin restarting
- **WHEN** the operator has moved acme's Certificate card to the top of the left column, and acme's service is restarting while the Overview loads
- **THEN** the Overview draws without the card, the operator's next arrangement keeps its place, and once acme answers again the card stands at the top of the left column

#### Scenario: A plugin removed
- **WHEN** the cookie names acme's Certificate card and verso-plugin-acme is not installed
- **THEN** the card is dropped from the layout, and the next layout the page writes does not name it

## ADDED Requirements

### Requirement: A plugin card has the shell card's anatomy
A contribution to `shell.overview.cards` SHALL answer with a card's title, Lucide icon, tone, status, summary, optional cause, optional page it opens on the plugin, and a body in the closed widget vocabulary. The shell SHALL draw its header, its open and closed states, its Go link and Arrange's grip and hide exactly as for its own cards.

#### Scenario: Closed plugin card
- **WHEN** the operator closes acme's Certificate card, whose summary is "expires in 52 d"
- **THEN** the closed header reads Certificate followed by "expires in 52 d", as a closed shell card reads its summary

#### Scenario: A body outside the vocabulary
- **WHEN** a plugin card's body holds a form
- **THEN** the shell draws the card's contained unavailable state and logs a compatibility warning naming the plugin and the contribution

### Requirement: A plugin card reads as of the page's load
The shell SHALL fetch each plugin card once each time it draws the Overview, hidden cards included, so their causes reach the verdict. The Overview's live stream SHALL NOT dial plugins.

#### Scenario: The stream runs
- **WHEN** the Overview has been open for ten minutes with acme's card on it
- **THEN** acme was asked for its card once, when the page was drawn

### Requirement: The layout cookie names plugin cards
The layout cookie SHALL be able to name a plugin card by its contribution's full id, `<plugin id>.<contribution id>`. The shell SHALL read a cookie written in the earlier format as before. The page SHALL write the version that can name plugin cards, keeping the ids of installed plugins' cards that did not draw.

#### Scenario: An existing layout after the upgrade
- **WHEN** a browser holds a layout written before plugin cards, with Traffic first on the left and System closed
- **THEN** after the upgrade the Overview draws Traffic first on the left and System closed

#### Scenario: Saving a layout with a plugin card
- **WHEN** the operator moves acme's Certificate card to the left column and reloads
- **THEN** the card stands in the left column
