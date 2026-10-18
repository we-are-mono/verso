# Spec Delta

## MODIFIED Requirements

### Requirement: A hand-installed certificate turns the trusted certificate off
Installing a certificate or making a new one on Access SHALL set `acme.verso.enabled` to 0, so no renewal replaces it. While the section is enabled, acme SHALL contribute a notice to the Install and Make drawers that says this before the operator confirms. verso-plugin-system SHALL NOT read the `acme` config. Core SHALL NOT read or write the `acme` config either: a hand install SHALL be announced to root's hotplug as `verso-certificate` with `ACTION=installed`, and acme's own hotplug script SHALL turn the section off.

#### Scenario: Pasting a certificate over an ACME one
- **WHEN** the `verso` section is enabled and the operator opens Install
- **THEN** the drawer shows acme's notice that installing turns off the trusted certificate from the operator's domain, and confirming leaves `acme.verso.enabled` at 0

#### Scenario: No trusted certificate set up
- **WHEN** there is no `verso` cert section, or it is disabled
- **THEN** the Install and Make drawers say nothing about it

#### Scenario: The system plugin's reads
- **WHEN** verso-plugin-system's manifest and rpcd ACL are read
- **THEN** neither names the `acme` config

#### Scenario: Making a certificate without acme installed
- **WHEN** verso-plugin-acme is not installed and the operator makes a new certificate
- **THEN** the certificate is made and served, the announcement runs no script, and `/etc/config/acme` is not created

#### Scenario: Turned off before the next catch-up
- **WHEN** the operator installs a certificate by hand while the `verso` section is enabled, and the router then reboots
- **THEN** the router serves the hand-installed certificate after the reboot, and acme's catch-up leaves it in place

## ADDED Requirements

### Requirement: The certificate's root work ships with acme
Recording acme's outcomes, serving acme's certificate, the catch-up, forgetting a stale saved credential, and the LAN lookup SHALL run from verso-plugin-acme's own root half, as root. They SHALL hand a certificate to the router's root install command. The privileged helper SHALL carry no ACME code and no `acme` command line. A router without the plugin SHALL carry no ACME hook.

#### Scenario: A renewal with no one signed in
- **WHEN** acme renews the certificate for the `verso` section's domain at night
- **THEN** acme's hotplug script runs the plugin's root half, which serves the renewal and records it, and the helper is not asked

#### Scenario: The helper without acme
- **WHEN** `verso-rpcd acme deliver sync` is run on a router with the registry
- **THEN** it exits non-zero as an unknown command and changes nothing

#### Scenario: Removing acme
- **WHEN** the operator removes verso-plugin-acme
- **THEN** `/etc/hotplug.d/acme/50-verso` and `/etc/hotplug.d/verso-certificate/50-acme` are gone, and the router keeps serving the certificate it served
