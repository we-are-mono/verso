# Spec Delta

## ADDED Requirements

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
