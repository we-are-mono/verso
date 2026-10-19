# plugin-doors Specification

## Purpose

How the shell links into a plugin: a running plugin claims the door to a kind of thing on the router, and every link the shell draws to such a thing goes through that claim, in the claiming plugin's own words.

## Requirements

### Requirement: A plugin claims a kind's door
The kinds `network`, `tunnel`, `zone`, `rule` and `port-forward` SHALL each have a door, the slot `open`. A plugin SHALL claim it in `entity_acts` as `{entity, slot: "open", path}`, the path relative to its mount and holding `{id}` where the subject's identity goes. An `entity_acts` claim on a kind or slot the shell does not have, or an `open` claim whose path holds no `{id}`, SHALL be ignored and logged, and the plugin's other claims SHALL stand.

#### Scenario: The firewall claims port forwards
- **WHEN** the firewall plugin's manifest claims `{"entity": "port-forward", "slot": "open", "path": "/port-forwards?open={id}"}`
- **THEN** the shell's door to a port forward leads into the firewall plugin's Port forwards page

#### Scenario: A claim without its subject
- **WHEN** a manifest claims `open` on `zone` with the path `/zones`
- **THEN** the shell logs the claim as ignored, draws no zone door from it, and the plugin's other claims and pages work

#### Scenario: A door kind has no panel
- **WHEN** a browser asks the shell for `/entity/zone/lan`
- **THEN** the shell answers 404, as for any kind without a panel

### Requirement: A door to one thing names it in the address
A door to one subject SHALL be the claim's path under the plugin's mount with `{id}` replaced by the subject's identity, escaped for a URL. A network, zone, rule or port forward is identified by its UCI section name; a tunnel by the name its claiming plugin lists it under.

#### Scenario: A logged rule
- **WHEN** the Firewall log shows a verdict of the rule in UCI section `cfg0c92bd` and the firewall plugin claims `rule` with `/?open={id}`
- **THEN** the rule's name in that row links to `/plugins/firewall/?open=cfg0c92bd`

### Requirement: A door to a kind leads to the page that lists it
A door to a kind as a whole SHALL lead to the most specific of the claiming plugin's pages that holds the claim's path without its query: a nav entry, or one of an entry's declared subpages. A claim whose path sits under none of the plugin's nav entries SHALL give no door.

#### Scenario: Networks
- **WHEN** the interfaces plugin claims `network` with `/edit?network={id}`, and its nav entries are Interfaces at `/` and Routes at `/routes`
- **THEN** the door to networks leads to `/plugins/interfaces/`

#### Scenario: Zones
- **WHEN** the firewall plugin claims `zone` with `/zones?open={id}`, and its one nav entry at `/` declares the subpage Zones at `zones`
- **THEN** the door to zones leads to `/plugins/firewall/zones`

#### Scenario: A plugin with no page for the claim
- **WHEN** a plugin with no nav entries claims `tunnel` with `/?open={id}`
- **THEN** the shell draws no tunnel door from that claim

### Requirement: A door says the claiming plugin's word
A door that carries words SHALL name its destination with the label of the claiming plugin's nav entry that holds the claim's path, translated by that plugin's catalog. The shell SHALL hold no word of its own for a destination a plugin claims.

#### Scenario: The exposure notice
- **WHEN** Access warns that a firewall zone lets the internet reach the web interface, and the firewall plugin, whose nav entry is labelled Firewall, claims `zone`
- **THEN** the notice's link reads "Open Firewall" and leads to the Zones page

#### Scenario: Slovenian
- **WHEN** the reader's language is Slovenian
- **THEN** the destination's name in the link is the firewall plugin catalog's translation of "Firewall"

### Requirement: No door without a running claim
The shell SHALL draw a door only while a plugin that claims it answers. While none does, the place SHALL draw no link, and text that would have been a link SHALL stay as plain text.

#### Scenario: The firewall stopped
- **WHEN** the firewall plugin's service is stopped and the operator opens Access while the internet can reach the web interface
- **THEN** the exposure notice keeps its warning and has no link

#### Scenario: A Firewall log already open
- **WHEN** the Firewall log is open in a browser and the firewall plugin's service stops
- **THEN** rows that arrive once the log's rule mapping next refreshes, within 30 s, name their rule as plain text

#### Scenario: Started again
- **WHEN** the firewall plugin answers again
- **THEN** a page loaded afterwards links the exposure notice again

### Requirement: One plugin answers a kind's door
When more than one running plugin claims the same kind's `open`, the door SHALL be the claim of the plugin whose id sorts first. Every page SHALL agree on the claimant.

#### Scenario: Two tunnel plugins
- **WHEN** plugins `vpn` and `zerotier` both claim `tunnel` and both answer
- **THEN** every tunnel door leads into `vpn`
