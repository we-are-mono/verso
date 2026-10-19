# plugin-boundary Specification

## Purpose

What the shell may know of a plugin: OpenWrt and its own closed vocabularies, never a plugin's id, path, nav label or release quirk, so a plugin can be added, renamed, replaced or stopped without a shell release.

## Requirements

### Requirement: The shell names no plugin
The shell and its privileged helper SHALL name no plugin's id, address or nav label, and SHALL carry no code for one plugin's release. What the shell knows of a plugin SHALL come from that plugin's manifest, its registry and its claims. The shell MAY know OpenWrt's configs, services and objects, and its own sections, entity kinds, slots and pages.

#### Scenario: A plugin is replaced
- **WHEN** a third-party plugin with another id brings the Firewall page, claims `zone`, `rule` and `port-forward`, and declares its own `order`
- **THEN** the rail, the exposure notice and the Firewall log lead into it as they led into verso-plugin-firewall, with no shell release

#### Scenario: A plugin renames its page
- **WHEN** the VPN plugin's nav entry is relabelled Tunnels
- **THEN** its rail row and the Tunnels card's door say Tunnels, and its row keeps its place

### Requirement: A test keeps the shell from naming a plugin
`make test` SHALL fail, naming the file and line, when the shell's Go outside tests:
- holds an address into one plugin's mount (`/plugins/<id>…`);
- compares a plugin's id with a fixed text, or passes a fixed text where a plugin's id is expected;
- compares a plugin's nav label with a fixed text, or looks a fixed table up by it.

The shell's reserved id `shell` is not a plugin's. It SHALL also fail when the helper compiles in or includes a source file of a plugin other than the SDK.

#### Scenario: A hard-coded link
- **WHEN** a change adds the text `/plugins/firewall/zones` to the shell's Go
- **THEN** `make test` fails and names that file and line

#### Scenario: A link built from a fixed id
- **WHEN** a change builds a plugin address from the fixed id `firewall`
- **THEN** `make test` fails and names the call

#### Scenario: Finding a page by its label
- **WHEN** a change compares a nav entry's label with "VPN"
- **THEN** `make test` fails and names the comparison

#### Scenario: An OpenWrt name
- **WHEN** the shell reads the `firewall` config or names the `firewall` service
- **THEN** the test passes, since the name is OpenWrt's

#### Scenario: The reserved id
- **WHEN** manifest validation refuses a plugin whose id is `shell`
- **THEN** the test passes

### Requirement: A read only a plugin draws is that plugin's
The shell SHALL broker only reads that it draws itself or that guard access to the router. Any other read of root state SHALL be the registered read of the plugin that draws it. Wi-Fi's live state SHALL be the wireless plugin's `state` read: per radio whether it is up, its channel, transmit power, channel busy share and hardware; per network whether it is up and its client count. A number that cannot be read SHALL be absent, never zero.

#### Scenario: The Wi-Fi page
- **WHEN** the operator opens the Wi-Fi page and their grant covers `verso-plugin wireless state`
- **THEN** each radio row shows its channel, power and busy share, and each network its clients, read by the wireless plugin's root half

#### Scenario: A radio's interface not up
- **WHEN** a radio is up and none of its networks has an interface yet
- **THEN** the radio reads up with its numbers unknown, and each network reads down

#### Scenario: The old brokered read
- **WHEN** a plugin declares the read `ubus verso wirelessState`
- **THEN** its page receives no entry for it

### Requirement: A plugin's figures arrive in the shell's spelling
A figure with a unit that a plugin hands the shell to lay out SHALL be written with U+202F between figure and unit. The shell SHALL split figure from unit at the first U+202F whatever the unit, and SHALL read no other spelling. Text with no U+202F SHALL be shown whole.

#### Scenario: A device's rate limit
- **WHEN** QoS summarises a device's download limit as `25` U+202F `Mb/s`
- **THEN** the roster's limit readout shows 25 as the figure and Mb/s in the unit's ink

#### Scenario: An older plugin's spelling
- **WHEN** a QoS release writes `25 Mb/s` with a plain space
- **THEN** the readout shows `25 Mb/s` whole, in the figure's ink

#### Scenario: No limit
- **WHEN** QoS summarises the upload as `No limit`
- **THEN** the readout shows `No limit`
