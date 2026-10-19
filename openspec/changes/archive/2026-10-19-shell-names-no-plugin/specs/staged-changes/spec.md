# Spec Delta

## ADDED Requirements

### Requirement: The reload step names each service in its owner's words
The reload step SHALL name a service of base OpenWrt or of the shell by the shell's word for it. It SHALL name any other service by the word an installed plugin declares for it in its manifest's `services`, in that plugin's catalog's translation, and otherwise as procd names it. A plugin's word SHALL NOT replace the shell's. Two services with one word SHALL be said once.

#### Scenario: Dynamic DNS
- **WHEN** a change to the ddns config is pending and the DDNS plugin declares `{"ddns": "Dynamic DNS"}`
- **THEN** the reload step reads "Reloads Dynamic DNS."

#### Scenario: OpenVPN
- **WHEN** a change to the openvpn config is pending and the VPN plugin declares `{"openvpn": "OpenVPN"}`
- **THEN** the reload step reads "Reloads OpenVPN."

#### Scenario: A plugin that declares no words
- **WHEN** a change to the openvpn config is pending and the installed VPN plugin declares no `services`
- **THEN** the reload step reads "Reloads openvpn."

#### Scenario: A plugin that renames a base service
- **WHEN** a plugin declares `{"firewall": "Packet filter"}` and a change to the firewall config is pending
- **THEN** the reload step reads "Reloads Firewall."

#### Scenario: A stopped plugin
- **WHEN** the DDNS plugin's service is stopped and a change to the ddns config is pending
- **THEN** the reload step still reads "Reloads Dynamic DNS."

### Requirement: A malformed service word is dropped alone
A `services` entry whose key is not an init-script name, or whose words are not non-empty text, SHALL be ignored and logged. The plugin's other words, and the plugin itself, SHALL still load.

#### Scenario: Words that are not text
- **WHEN** a plugin declares `{"ddns": 1, "ddns-mon": "DDNS monitor"}`
- **THEN** the shell logs the `ddns` entry, the plugin loads, and a reload of `ddns-mon` reads "DDNS monitor"

#### Scenario: Not a list of words at all
- **WHEN** a plugin declares `"services": ["ddns"]`
- **THEN** the shell logs it, the plugin loads with no service words, and its pages work
