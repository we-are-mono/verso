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

### Requirement: A plugin places its entry among its siblings
A nav entry MAY declare `order`, a whole number. An entry filed under Network or System SHALL stand among that section's pages; an entry with a row of its own SHALL stand among the rail's rows. Siblings SHALL stand in ascending `order`, then by English label, then by plugin id, so the rail is the same on every page, in every language, whatever order plugins are found in.

#### Scenario: A plugin between two others
- **WHEN** Wireless declares 30, Firewall 50, and a new plugin's row declares 45
- **THEN** the new row stands between Wireless and Firewall

#### Scenario: A page inside Network
- **WHEN** interfaces and dnsdhcp declare the bundled numbers, no other plugin files a page under Network, and a plugin files a Multicast page there with `order` 35
- **THEN** Network's pages read Interfaces, DHCP, DNS, Multicast, Routes

#### Scenario: Two entries with one number
- **WHEN** two rows both declare 60, labelled Storage and Printers
- **THEN** Printers stands before Storage, in English and in Slovenian alike

### Requirement: A place that is not a whole number is no place
An `order` that is not a whole number SHALL be read as absent and logged, and the manifest SHALL still load.

#### Scenario: A place written as text
- **WHEN** a plugin's row declares `order` `"40"`
- **THEN** the shell logs it, the row stands among the unordered rows, and the plugin's pages load

#### Scenario: A fraction
- **WHEN** a plugin's row declares `order` 4.5
- **THEN** the shell logs it and the row stands among the unordered rows

### Requirement: Overview stands first
Overview SHALL be the rail's first destination, at place 0. A plugin's `order` below 1 SHALL be read as 1, and the manifest SHALL still load. Only the Pending changes row SHALL stand above Overview.

#### Scenario: A plugin that asks for the top
- **WHEN** a plugin's row declares `order` -5
- **THEN** Overview stands first and the plugin's row stands second, before Devices

### Requirement: The shell's own places have published numbers
The shell's own rows and pages SHALL stand at fixed numbers that plugin authors are told:
- rows: Overview 0, Devices 10, Network 20, System 90;
- System's pages: Hardware 20, Access 30, Packages 40, Services 50, Maintenance 60, Logs 70, Events 80, Changes 90, Diagnostics 100.

Network holds no page of the shell's own.

#### Scenario: Before Devices
- **WHEN** a plugin's row declares 5
- **THEN** it stands between Overview and Devices

#### Scenario: Among System's pages
- **WHEN** a plugin files a Backup page under System with `order` 65
- **THEN** System's pages read General, Hardware, Access, Packages, Services, Maintenance, Backup, Logs, Events, Changes, Diagnostics

### Requirement: An entry without a place follows the placed ones
An entry that declares no `order` SHALL stand after every sibling with a number. Such entries SHALL stand by section, the shell's sections first in their own order, then by section title, then by English label, then by plugin id.

#### Scenario: An older VPN plugin
- **WHEN** the VPN plugin's manifest declares no `order`
- **THEN** its row stands after System

#### Scenario: An older DNS and DHCP plugin
- **WHEN** the DNS and DHCP plugin declares no `order` and Interfaces declares 10 and Routes 40
- **THEN** Network's pages read Interfaces, Routes, DHCP, DNS

### Requirement: The bundled plugins keep the designed rail
The first-party plugins SHALL declare the rail's designed places: Wireless 30, VPN 40 and Firewall 50 among the rows, and General 10 under System. Under Network they declare Interfaces 10, DHCP 20, DNS 30, Routes 40 and Dynamic DNS 50.

#### Scenario: Every first-party plugin installed
- **WHEN** interfaces, dnsdhcp, ddns, wireless, vpn, firewall and system are installed at their versions that declare `order`
- **THEN** the rail reads Overview, Devices, Network, Wireless, VPN, Firewall, System
- **AND** Network's pages read Interfaces, DHCP, DNS, Routes, Dynamic DNS
- **AND** System's pages read General, Hardware, Access, Packages, Services, Maintenance, Logs, Events, Changes, Diagnostics

### Requirement: A row's glyph comes from its entry or its section
A plugin's row SHALL wear the glyph its nav entry names, else its section's glyph. The shell SHALL choose no glyph by a plugin's label.

#### Scenario: VPN names its glyph
- **WHEN** the VPN plugin's entry names the glyph `lock`
- **THEN** its row wears the lock

#### Scenario: Firewall names none
- **WHEN** the firewall plugin's entry, filed under Security, names no glyph
- **THEN** its row wears Security's glyph
