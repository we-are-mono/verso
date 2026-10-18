# Spec Delta

## Purpose

Lets a plugin carry its own root work: a registry of root commands it ships, run by the
privileged helper only for an operator whose session holds the plugin's grant, so the
shell and the helper name no plugin and a removed plugin leaves no root code behind.

## ADDED Requirements

### Requirement: A plugin registers its root commands in one file
A plugin package SHALL declare its root commands in `/usr/share/verso/plugins/<id>/commands.json`. Each command SHALL have a name, a kind (`read` or `act`), an absolute executable path and fixed arguments. An act SHALL also carry an argument schema, and either kind MAY carry a timeout. The helper SHALL read the file when a command is asked for, so an install or a removal takes effect with no restart.

#### Scenario: A newly installed plugin
- **WHEN** a plugin package that ships `commands.json` is installed while the helper runs
- **THEN** the plugin's commands run for a session holding their grants, without restarting the helper or the shell

#### Scenario: A malformed registry
- **WHEN** `commands.json` does not parse, or a command in it has no executable or an unknown kind
- **THEN** that plugin's commands are refused with a logged reason naming the plugin, and every other plugin's commands still run

### Requirement: Only root-owned files are run
The helper SHALL run a command only when the registry file, the executable and every directory above each are owned by root and writable by root alone, and the executable is a regular file. Otherwise it SHALL refuse the command, run nothing, and log which path failed the check.

#### Scenario: A group-writable executable
- **WHEN** a registered executable is mode `0775`
- **THEN** its commands are refused, nothing is run, and the log names the executable

#### Scenario: An executable reached through a link
- **WHEN** the registered executable path is a symbolic link
- **THEN** its commands are refused

### Requirement: A command runs only under its grant
The helper SHALL run a plugin's command only for a session that rpcd grants scope `verso-plugin`, object `<plugin id>`, function `<command name>`: a read through the grant's `read` part, an act through its `write` part. The zero session SHALL hold no such grant, so unattended callers never reach the registry.

#### Scenario: An operator without the grant
- **WHEN** a session whose login lacks the vpn plugin's group asks for vpn's `state`
- **THEN** the helper answers permission denied and runs nothing

#### Scenario: The zero session
- **WHEN** a local root caller asks for any registered command with the zero session
- **THEN** the helper answers permission denied and runs nothing

#### Scenario: A plugin installed during a session
- **WHEN** the operator installs verso-plugin-ddns from Packages while signed in
- **THEN** ddns's registered reads answer for that session without signing in again

### Requirement: An act's arguments are checked against its schema
The helper SHALL check every act's arguments against the schema registered for it, and refuse the act without running it when an argument is missing, unknown, or not of its kind. The kinds SHALL be a closed set: uci name, hostname, IPv4 address, IPv6 address, IP address, port, bounded integer, enumeration, boolean, and bounded text. A read SHALL take no arguments.

#### Scenario: An argument outside its kind
- **WHEN** ddns's `update` act is asked with section `home; reboot`, and its schema names `section` as a uci name
- **THEN** the act is refused as invalid and nothing runs

#### Scenario: An unknown argument
- **WHEN** an act is asked with an argument its schema does not name
- **THEN** the act is refused as invalid and nothing runs

#### Scenario: A read asked with arguments
- **WHEN** a read is asked with any argument
- **THEN** the read is refused as invalid

### Requirement: A command runs bounded and without a shell
The helper SHALL run a command's executable directly as root, never through a shell. It SHALL pass the command's fixed arguments, the working directory `/`, an environment of `PATH` alone, and a JSON object of the command's name and arguments on stdin. It SHALL stop the command and its children at its timeout (10 s unless declared, never more than 120 s). It SHALL refuse output over 1 MiB, and output that is not one JSON object.

#### Scenario: A command that hangs
- **WHEN** a read's executable does not exit within its timeout
- **THEN** it and its children are killed and the read fails, and the helper answers other requests throughout

#### Scenario: Oversized output
- **WHEN** a read writes more than 1 MiB to stdout
- **THEN** the read fails and the output is not passed on

### Requirement: A command's outcome is one of three
A command that exits 0 with a JSON object SHALL succeed with that object as its result. A command that exits 3 with a JSON object holding `refusal` SHALL be refused, with that text as the reason the operator reads. Any other end SHALL be a failure whose detail is logged and never shown.

#### Scenario: A refused act
- **WHEN** an act's root half exits 3 with `{"refusal": "That section does not exist."}`
- **THEN** the form's notice says "That section does not exist.", in the reader's language when the plugin's catalogue translates it

#### Scenario: A crashing act
- **WHEN** an act's root half exits 101 and writes a panic to stderr
- **THEN** the notice says the router could not complete the action, and the panic is in the log only

### Requirement: The shell brokers declared reads
For every read a plugin declares in its manifest's `acl.read` as scope `verso-plugin`, the shell SHALL run the read through the helper with the operator's session and give the plugin the result under the key `<plugin>.<name>`. A read that is refused, fails, or names an uninstalled plugin SHALL give no entry. The page SHALL still render.

#### Scenario: A page with its own state
- **WHEN** the VPN page renders and vpn declares its `state` read
- **THEN** the plugin's request carries vpn's tunnel state under `vpn.state`

#### Scenario: A read that fails
- **WHEN** vpn's `state` read times out
- **THEN** the VPN page renders without it, and the log names the read

### Requirement: A plugin may read another plugin's state
A plugin MAY declare, and hold the rpcd grant for, another plugin's read. The shell SHALL broker it as it brokers the plugin's own. When the other plugin is not installed, the read SHALL give no entry.

#### Scenario: Interfaces with VPN installed
- **WHEN** both verso-plugin-interfaces and verso-plugin-vpn are installed
- **THEN** the Interfaces page receives `vpn.state` and lists the tunnels it reports

#### Scenario: Interfaces without VPN
- **WHEN** verso-plugin-vpn is not installed
- **THEN** the Interfaces page renders without tunnel state and shows no error

### Requirement: A read's result carries its version
Each registered read SHALL declare a version, a positive integer raised only when its result changes incompatibly. The shell SHALL give a plugin each registered read as its version and its result. A plugin SHALL use a result only when it knows the version, and SHALL treat an unknown version as no entry.

#### Scenario: A known version
- **WHEN** vpn's `state` read is version 1 and verso-plugin-interfaces knows version 1
- **THEN** the Interfaces page lists the tunnels it reports

#### Scenario: A version the reader does not know
- **WHEN** an upgraded verso-plugin-vpn raises its `state` read to version 2, and the installed verso-plugin-interfaces knows only version 1
- **THEN** the Interfaces page renders without tunnel state, shows no error, and logs the version it skipped

#### Scenario: An added field
- **WHEN** vpn adds a field to its `state` result and keeps the version
- **THEN** a reader that knows the version reads its known fields as before

### Requirement: An act runs at once, on its own save
When a plugin's answer to a POST names an act, the shell SHALL refuse it unless the plugin declares the act in its manifest's `acl.write` as scope `verso-plugin`. Otherwise the shell SHALL run it through the helper with the operator's session before answering. An act SHALL NOT share a save with staged changes. A file stage, which joins the stage, is the only command that may.

#### Scenario: Updating a DDNS name now
- **WHEN** the operator presses Update on a DDNS service
- **THEN** ddns's `update` act runs before the page answers, and the page says it ran

#### Scenario: An act beside a staged change
- **WHEN** a plugin answers one POST with both a uci commit and an act
- **THEN** the shell refuses the POST as not permitted and stages nothing

#### Scenario: An undeclared act
- **WHEN** a plugin names an act its manifest does not declare
- **THEN** the shell refuses it without asking the helper

### Requirement: A read result may carry a certificate as facts
A registered read MAY name fields of its result that hold a PEM certificate. The shell SHALL replace each such field with the facts the HTTPS certificate card shows (subject, issuer, validity, fingerprint, serial, days, expired, self-signed), so no certificate PEM reaches a plugin. A field that does not parse SHALL be left out.

#### Scenario: acme's certificate
- **WHEN** acme's `state` read returns its certificate as PEM, and its registry entry names that field
- **THEN** the acme plugin receives the certificate's subject, expiry and fingerprint, never the PEM

### Requirement: Core names no plugin's root work
The privileged helper SHALL hold no verb, module or command line that only one plugin uses. The shell SHALL broker reads and forward acts by registry, not by name. Core keeps the root work the shell itself renders (firewall status, the firewall log, DHCP leases, network state) and what guards access to the router (credentials, SSH keys, the web certificate, the clock, the password).

#### Scenario: A router without VPN
- **WHEN** verso-plugin-vpn is not installed
- **THEN** no root code on the router reads OpenVPN's state or folds its profiles, and nothing writes `/var/run/verso-openvpn`

#### Scenario: The helper's verbs
- **WHEN** the helper's verb list is read
- **THEN** it names none of `vpnState`, `openvpnFiles`, `ddnsState`, `ddnsUpdate`, `dhcpState`, `dnsState`, `firewallCounters`, `firewallFiles` or `acmeState`

### Requirement: A plugin's root half is its own binary
A first-party plugin's root work SHALL run from the plugin's own executable, started by root: by the helper for a registered command, and by the plugin's hotplug scripts and init for unattended work. Started by any other account, the root half SHALL refuse to act.

#### Scenario: Run by the plugin's own account
- **WHEN** `verso-plugin-acme root deliver sync` is started as `verso-plugin-acme`
- **THEN** it exits non-zero having changed nothing

#### Scenario: Unattended work
- **WHEN** acme renews the certificate at night and no one is signed in
- **THEN** acme's hotplug script runs acme's root half as root directly, and the helper is not asked

### Requirement: A removed plugin leaves no root trace
Removing a plugin package SHALL remove its registry, its root half and its grants. The helper SHALL then refuse that plugin's commands as unknown, and every reader of them SHALL get no entry.

#### Scenario: Removing DDNS
- **WHEN** the operator removes verso-plugin-ddns
- **THEN** `/usr/share/verso/plugins/ddns/commands.json` is gone, and a request for ddns's `state` is refused as unknown

### Requirement: Core and plugins built before the registry are not mixed
The `verso` package SHALL conflict with the acme, vpn, ddns and dnsdhcp versions that use the helper's removed verbs. Each of those plugins SHALL depend on the `verso` version that has the registry, so one upgrade moves them together. The upgrade SHALL keep the runtime paths those plugins read, so live tunnel records and staged file versions survive it.

#### Scenario: Upgrading verso alone
- **WHEN** the operator asks apk to upgrade verso while an older verso-plugin-vpn is installed
- **THEN** apk upgrades verso-plugin-vpn in the same transaction or refuses both, and never leaves the new core with the old plugin

#### Scenario: A tunnel up across the upgrade
- **WHEN** an OpenVPN tunnel is connected while verso and verso-plugin-vpn upgrade
- **THEN** the VPN page shows it connected, with its connected time, without the tunnel reconnecting
