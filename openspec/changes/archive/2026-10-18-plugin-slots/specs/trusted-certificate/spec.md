# Spec Delta

## MODIFIED Requirements

### Requirement: Access sets up a trusted certificate
When verso-plugin-acme is installed, Access's HTTPS certificate card SHALL carry a "Use your domain" act, contributed by acme, that opens acme's setup in a drawer. Access SHALL show no separate Trusted certificate part. The drawer asks for a domain, a DNS provider, that provider's credentials, and whether the certificate is on. Saving SHALL stage the `acme` config's `verso` cert section with `validation_method` `dns`, the domain as its first `domains` entry, the provider's acme.sh name as `dns`, and the credentials as `credentials`.

#### Scenario: First save
- **WHEN** the operator presses "Use your domain", enters `router.example.com`, picks Cloudflare, enters an API token, turns the certificate on and saves
- **THEN** `uci changes acme` shows a `verso` cert section with `enabled` 1, `domains` `router.example.com`, `validation_method` `dns`, `dns` `dns_cf` and a `credentials` entry carrying the token

#### Scenario: The plugin is not installed
- **WHEN** verso-plugin-acme is not installed
- **THEN** the HTTPS certificate card has no "Use your domain" act, Access shows nothing of acme, and nothing reads or writes `/etc/config/acme`

#### Scenario: One card for the certificate
- **WHEN** verso-plugin-acme is installed and serves the certificate
- **THEN** Access shows one card about the certificate, HTTPS certificate, and nothing else on the page describes the served certificate

### Requirement: Providers have named fields and Other takes any
The provider choice SHALL offer Cloudflare, DNSimple, deSEC, DuckDNS and dynv6, each with its credentials as named fields, plus Other. Other SHALL take the acme.sh provider name and the raw `credentials` list, so any acme.sh DNS provider is usable. A section whose `dns` is not a curated provider SHALL open as Other with its values.

#### Scenario: A curated provider's fields
- **WHEN** the operator picks DNSimple
- **THEN** the drawer asks for one field named for DNSimple's API token and nothing else

#### Scenario: A provider set up from the shell
- **WHEN** `acme.verso.dns` is `dns_gandi_livedns` with a `credentials` entry written by `uci`
- **THEN** the drawer shows Other, the provider name `dns_gandi_livedns`, and the credentials as set

### Requirement: Credentials are write-only
The drawer SHALL never show a secret credential's value: a stored secret SHALL read as set, with a way to replace it. Saving with a secret left blank SHALL keep the stored value. An identifier that is no secret (a zone's ID) shows its value. Changing the provider SHALL replace the whole `credentials` list. The contact email acme-common requires SHALL be asked for.

#### Scenario: Reopening a saved setup
- **WHEN** the operator opens the drawer after saving a Cloudflare token
- **THEN** the token field shows that a token is set and the drawer's source holds no part of the token

#### Scenario: Saving without retyping
- **WHEN** the operator changes only the domain and saves
- **THEN** the staged change keeps the stored `credentials` as they were

#### Scenario: Switching provider
- **WHEN** the operator switches from Cloudflare to deSEC, enters a token and saves
- **THEN** the staged `credentials` hold the deSEC token alone

### Requirement: Applying the setup requests a certificate
Applying a change to the `verso` cert section while it is enabled SHALL start a certificate request through OpenWrt's acme package, with no other operator step. The router SHALL NOT need to be reachable from the internet, and no port SHALL be opened for it.

#### Scenario: Applying a first setup
- **WHEN** the operator applies a staged, enabled `verso` section
- **THEN** a request for its domain starts without any further action, and the certificate card's Source row shows it as requesting

### Requirement: The part reports the certificate's state
While the `verso` section is enabled, acme SHALL contribute a Source row to the HTTPS certificate card's facts. The row names the domain and says what last happened and how long ago: requesting, issued, renewed or failed. The drawer SHALL give the expiry of the certificate acme holds and whether it is served. State SHALL come from the acme package's events and the certificate file, never the system log. A failure is said in plain words, with how long ago.

#### Scenario: After a renewal
- **WHEN** the certificate renewed three days ago and expires on 1 January
- **THEN** the Source row names the domain and says it renewed three days ago, and the drawer gives 1 January as the expiry and says the certificate is served

#### Scenario: A failed request
- **WHEN** the provider rejects the token and the request fails
- **THEN** the Source row says the request failed and how long ago, marked in the danger tone, and the drawer asks the operator to check the domain and the credentials

#### Scenario: After a reboot
- **WHEN** the router reboots after an earlier renewal
- **THEN** the drawer still gives the certificate's expiry and whether it is served, read from the certificate file

#### Scenario: Requesting
- **WHEN** acme's request for the domain is running
- **THEN** the Source row carries the waiting mark

#### Scenario: Held but not served
- **WHEN** acme holds a certificate for the domain that the router does not serve
- **THEN** the Source row, in the warning tone, says the certificate is not served yet and the router serves the one the card's facts show

#### Scenario: No trusted certificate set up
- **WHEN** there is no `verso` cert section, or it is disabled
- **THEN** acme contributes no Source row, and the card's own Signed by row says who signed the served certificate

### Requirement: A hand-installed certificate turns the trusted certificate off
Installing a certificate or making a new one on Access SHALL set `acme.verso.enabled` to 0, so no renewal replaces it. While the section is enabled, acme SHALL contribute a notice to the Install and Make drawers that says this before the operator confirms. verso-plugin-system SHALL NOT read the `acme` config.

#### Scenario: Pasting a certificate over an ACME one
- **WHEN** the `verso` section is enabled and the operator opens Install
- **THEN** the drawer shows acme's notice that installing turns off the trusted certificate from the operator's domain, and confirming leaves `acme.verso.enabled` at 0

#### Scenario: No trusted certificate set up
- **WHEN** there is no `verso` cert section, or it is disabled
- **THEN** the Install and Make drawers say nothing about it

#### Scenario: The system plugin's reads
- **WHEN** verso-plugin-system's manifest and rpcd ACL are read
- **THEN** neither names the `acme` config

### Requirement: The domain is offered an answer on the LAN
When the router's own lookup of the domain answers with none of the router's addresses, whatever keeps the LAN's names (dnsmasq, AdGuard Home), the drawer SHALL offer to add a record. With verso-plugin-dnsdhcp installed, the offer SHALL open its New record drawer with the domain and the router's LAN address filled in, and staging the record stays that drawer's act. Without it, the drawer SHALL name the record to add.

#### Scenario: Adding the LAN record
- **WHEN** no record answers `router.example.com` and the operator takes the offer
- **THEN** the New record drawer opens as an A record for `router.example.com` with the router's LAN address, and nothing is staged until the operator saves it

#### Scenario: A record already answers
- **WHEN** a record answers `router.example.com` with the router's address
- **THEN** the drawer makes no offer

#### Scenario: AdGuard Home keeps the LAN's names
- **WHEN** AdGuard Home on the router answers `router.example.com` with the router's address and `/etc/config/dhcp` holds no record of it
- **THEN** the drawer makes no offer

#### Scenario: The domain leads elsewhere
- **WHEN** the router's lookup answers `router.example.com` with an address that is not the router's
- **THEN** the drawer says this network finds the domain at another address and offers the record

#### Scenario: DNS and DHCP plugin not installed
- **WHEN** verso-plugin-dnsdhcp is not installed and no record answers the domain
- **THEN** the drawer names the record to add, the domain and the router's LAN address, and links nowhere

## ADDED Requirements

### Requirement: The Certificate card stands on the Overview
While the `verso` section is enabled, acme SHALL contribute a Certificate card to the Overview: the domain as its summary, the certificate's facts in its body, and a door that opens the setup. A failed request or renewal SHALL be the card's cause, in the danger tone once the served certificate has seven days or fewer left and in the warning tone before that.

#### Scenario: A healthy certificate
- **WHEN** acme serves a certificate that expires in 52 days
- **THEN** the Overview shows a Certificate card whose closed summary reads `router.example.com`, whose body gives whom it is issued to, who signed it, 52 days left, that it is served and how long ago acme last did something, and which states no cause

#### Scenario: A failed renewal
- **WHEN** acme's last renewal failed and the served certificate has 20 days left
- **THEN** the Certificate card states "The certificate could not be renewed" as its cause in the warning tone, and the verdict counts it

#### Scenario: Not set up
- **WHEN** there is no `verso` cert section, or it is disabled
- **THEN** the Overview shows no Certificate card
