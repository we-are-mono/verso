# rail Specification

## Purpose
How the rail lets an operator move around the router's pages: a destination with subpages is a group that opens in place, one group open at a time, and every group's subpages are known on every page, a plugin's from its manifest.

## Requirements

### Requirement: A group opens in place
A rail row with subpages SHALL be a disclosure, not a link: pressing it SHALL show its subpages under it without leaving the page. Pressing an open group SHALL close it. A row with no subpages SHALL stay a link to its page.

#### Scenario: Opening Firewall from the Overview
- **WHEN** the operator is on the Overview and presses Firewall in the rail
- **THEN** the page stays the Overview, Firewall's subpages appear under it, and its chevron turns down

#### Scenario: Closing an open group
- **WHEN** Firewall's group is open and the operator presses Firewall
- **THEN** its subpages fold away and the page does not change

#### Scenario: A row without subpages
- **WHEN** the operator presses Devices
- **THEN** the Devices page opens

### Requirement: One group is open at a time
Opening a group SHALL close any other open group.

#### Scenario: Moving from Network to System
- **WHEN** Network's group is open and the operator presses System
- **THEN** System's subpages show and Network's fold away

### Requirement: The current page's group opens with the page
A page SHALL load with the group that holds it open and every other group closed, and the subpage the page is SHALL carry the rail's marker.

#### Scenario: Arriving on a firewall page
- **WHEN** the operator opens Firewall › Zones
- **THEN** Firewall's group is open, Zones carries the marker, and no other group is open

#### Scenario: A page outside every group
- **WHEN** the operator opens Devices
- **THEN** every group is closed

### Requirement: A group opens without a script and from the keyboard
A group SHALL open and close with scripting unavailable. A group SHALL be reachable with Tab, open and close with Enter or Space, and announce whether it is open to assistive technology.

#### Scenario: Keyboard
- **WHEN** focus is on System's row and the operator presses Enter
- **THEN** System's subpages show and the row reports itself expanded

### Requirement: Every group's subpages are in the page
Every group's subpages SHALL be in the page as it loads, so opening a group SHALL NOT wait on the router. A group's subpages SHALL be Network's and System's own pages, or the subpages a plugin declares in its manifest.

#### Scenario: No request on opening
- **WHEN** the operator opens Firewall's group from the Overview
- **THEN** the browser makes no request and the subpages show at once

#### Scenario: A stopped plugin
- **WHEN** a plugin's service is not answering
- **THEN** its row and its subpages are absent from the rail, as its row is today

### Requirement: A plugin declares its subpages in its manifest
A nav entry's `pages` SHALL be either a list of `{label, path}` or `true`. Each path SHALL be relative to the plugin's mount, as the envelope's pages are, and the shell SHALL build each subpage's address. A manifest whose subpage lacks a label or whose path is not a clean relative path SHALL be rejected at load, naming the plugin and the entry. Labels SHALL be localized from the plugin's catalog.

#### Scenario: Firewall's manifest
- **WHEN** Firewall's manifest declares Rules, Port forwards, Zones, Settings and Activity
- **THEN** its group lists those five, in that order, under `/plugins/firewall/`

#### Scenario: A path that leaves the mount
- **WHEN** a manifest declares a subpage with path `../system`
- **THEN** the plugin is not loaded and the log names the plugin and the subpage

#### Scenario: Slovenian
- **WHEN** the reader's language is Slovenian
- **THEN** each declared subpage's label is the plugin catalog's translation

### Requirement: The manifest's list is the one source of a plugin's subpages
When a nav entry declares a list, the rail SHALL draw that entry's subpages from it on every page and SHALL ignore the subpages the plugin's page envelope names.

#### Scenario: An envelope that disagrees
- **WHEN** Firewall's manifest declares five subpages and a Firewall page's envelope names four
- **THEN** the rail shows the manifest's five

### Requirement: An entry that declares no list keeps leading to its page
A nav entry whose `pages` is `true` SHALL be a link to its page, as a row without subpages is, carrying the chevron that says it opens. On that plugin's own pages the rail SHALL show the subpages its envelope names, open under the row.

#### Scenario: An older plugin
- **WHEN** a plugin's manifest says `"pages": true` and the operator presses its row from the Overview
- **THEN** the plugin's page opens, and its envelope's subpages show under its row

### Requirement: The stage's row stands first in the rail
While the stage holds a change, the rail SHALL show a Pending changes row above every destination. It SHALL belong to no group, SHALL NOT open or close any group, and SHALL carry the rail's marker while the Pending changes page is open. Under the pointer it SHALL slide as every rail row does. While the stage is clean, the row SHALL be absent and the rail SHALL start with Overview.

#### Scenario: Changes waiting
- **WHEN** the stage holds changes and the operator is on Firewall › Rules
- **THEN** the rail's first row is Pending changes, Firewall's group is open below it, and Rules carries the marker

#### Scenario: On the Pending changes page
- **WHEN** the operator opens the Pending changes page
- **THEN** the Pending changes row carries the marker and every group is closed

#### Scenario: A clean stage
- **WHEN** nothing is pending, even while a form holds an unsaved edit
- **THEN** the rail's first row is Overview

### Requirement: The phone's menu button carries the stage's count
On a layout where the rail sits behind the menu button, the menu button SHALL show the pending count beside its icon while the stage holds a change. An unsaved edit SHALL NOT change it.

#### Scenario: A phone with changes waiting
- **WHEN** the rail is behind the menu button and 12 changes are pending
- **THEN** the menu button reads its icon and 12

#### Scenario: Editing on a phone
- **WHEN** 12 changes are pending and the operator edits a rule without saving
- **THEN** the menu button still reads 12
