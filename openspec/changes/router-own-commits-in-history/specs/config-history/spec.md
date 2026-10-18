# Spec Delta

## Purpose

Config history is the record of every change to a config the plugins declare: what changed, who or what changed it, and why. The router's current settings can always be explained by its rows.

## ADDED Requirements

### Requirement: Every commit to a declared config is a row
Config history SHALL record a row for each commit that changes a config the shell or an installed plugin declares, whoever made it:
- an apply from the Pending changes page;
- an edit made outside Verso;
- a commit the router makes by itself.

A commit the router makes by itself SHALL never be taken in without a row. A reading taken while an apply's rollback window is open, or within its recording slack, SHALL stay that apply's and add no row.

#### Scenario: A hand install turns the trusted certificate off
- **WHEN** the operator installs a certificate by hand on Access while the trusted certificate for router.example.com is on
- **THEN** config history gains a row that turns the trusted certificate for router.example.com off, and the history never shows it on while the router holds it off

#### Scenario: A commit inside an apply's window
- **WHEN** a config the apply wrote is read again within its window
- **THEN** no row is added beside the apply's

### Requirement: The router's own commit names itself and its cause
A row for a commit the router made by itself, on no operator's behalf, SHALL read in By as the router, never "Edited directly", and never as Verso by name. Notes SHALL say what caused the commit, in the viewer's language: from the shell's words for the shell's own commits, from the plugin's catalog for a plugin's. Each change in the row SHALL be described in the words of the plugin that owns the config, as any other row is.

#### Scenario: ACME's hand-over
- **WHEN** ACME's root half turns `acme.verso.enabled` off after a hand install
- **THEN** the row's By reads the router, its Notes read "A certificate was installed by hand.", and its change reads as the trusted-certificate page words it

#### Scenario: A package arrives switched off
- **WHEN** https-dns-proxy is installed from the Packages page and the router commits its config switched off
- **THEN** the https-dns-proxy row's Notes say it arrived switched off with its package

#### Scenario: Notes in the viewer's language
- **WHEN** a viewer reading in Slovenian opens history, and the plugin's catalog holds the cause
- **THEN** the cause reads in Slovenian; a cause no catalog holds reads as written

### Requirement: A commit made for an operator is that operator's
A commit the router makes through its own helper as part of an act an operator started SHALL be recorded as that operator's, from the address they acted from, as an apply is. Notes SHALL keep the commit's cause. This covers the theme, the web interface's ports, and a package installed from the Packages page.

#### Scenario: Choosing a theme
- **WHEN** an operator signed in from this browser chooses a theme
- **THEN** the row for `verso` names that operator and this browser in By, and its cause in Notes

#### Scenario: Two operators
- **WHEN** one operator moves the web interface's ports while another is signed in
- **THEN** the row names the operator who moved them

### Requirement: What a package changes as Verso installs it is the install's
Every change to a declared config committed while the router installs, removes or upgrades packages for an operator, the package's own scripts included, SHALL be recorded as part of that operation: by that operator, from their address, with Notes naming the package and what happened to it.

#### Scenario: A cause of its own during the operation
- **WHEN** a package arrives switched off during its install
- **THEN** that commit's row keeps the arrival's cause

#### Scenario: Outside Verso
- **WHEN** the same package is installed from the command line and its scripts commit a config
- **THEN** that row reads "Edited directly"

#### Scenario: A package's own scripts during an install
- **WHEN** an operator installs https-dns-proxy from Packages, and its service rewrites dnsmasq's servers as it starts
- **THEN** the `dhcp` row names that operator and their browser in By, and its Notes read "Changed by https-dns-proxy as it was installed."

#### Scenario: A removal
- **WHEN** an operator removes a package from Packages, and its scripts commit a config as it goes
- **THEN** that row's Notes read that the package changed it as it was removed

#### Scenario: Notes in the viewer's language
- **WHEN** a viewer reading in Slovenian opens the row
- **THEN** the Notes read in Slovenian, the package's name as written

### Requirement: A mark without a cause still counts
A commit an older plugin marks as the router's own, with no cause, SHALL be recorded as the router's act with empty Notes, never as "Edited directly" and never skipped.

#### Scenario: A plugin built before causes
- **WHEN** a root half built on the earlier SDK marks and commits its config
- **THEN** the row's By reads the router and Notes are empty

### Requirement: The router's own rows keep no secret
A row for the router's own commit SHALL keep a secret option's value no more than any other row: a changed secret reads as changed, with no value before or after.

#### Scenario: A secret the router commits
- **WHEN** the router commits a config whose secret option changed
- **THEN** the row says the option changed and holds neither value
