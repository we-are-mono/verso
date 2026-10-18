# Spec Delta

## Purpose

Lets an operator who controls a domain have the router serve a publicly trusted
certificate for it, obtained and renewed by DNS-01 without the router being
reachable from the internet, with its state shown on Access and one clear owner
of the certificate Verso serves.

## ADDED Requirements

### Requirement: Access sets up a trusted certificate
When verso-plugin-acme is installed, Access SHALL show a Trusted certificate part after Certificate. It asks for a domain, a DNS provider, that provider's credentials, and whether the certificate is on. Saving SHALL stage the `acme` config's `verso` cert section with `validation_method` `dns`, the domain as its first `domains` entry, the provider's acme.sh name as `dns`, and the credentials as `credentials`.

#### Scenario: First save
- **WHEN** the operator enters `router.example.com`, picks Cloudflare, enters an API token, turns the certificate on and saves
- **THEN** `uci changes acme` shows a `verso` cert section with `enabled` 1, `domains` `router.example.com`, `validation_method` `dns`, `dns` `dns_cf` and a `credentials` entry carrying the token

#### Scenario: The plugin is not installed
- **WHEN** verso-plugin-acme is not installed
- **THEN** Access shows no Trusted certificate part and nothing reads or writes `/etc/config/acme`

### Requirement: Providers have named fields and Other takes any
The provider choice SHALL offer Cloudflare, DNSimple, deSEC, DuckDNS and dynv6, each with its credentials as named fields, plus Other. Other SHALL take the acme.sh provider name and the raw `credentials` list, so any acme.sh DNS provider is usable. A section whose `dns` is not a curated provider SHALL open as Other with its values.

#### Scenario: A curated provider's fields
- **WHEN** the operator picks DNSimple
- **THEN** the part asks for one field named for DNSimple's API token and nothing else

#### Scenario: A provider set up from the shell
- **WHEN** `acme.verso.dns` is `dns_gandi_livedns` with a `credentials` entry written by `uci`
- **THEN** the part shows Other, the provider name `dns_gandi_livedns`, and the credentials as set

### Requirement: Credentials are write-only
The part SHALL never show a secret credential's value: a stored secret SHALL read as set, with a way to replace it. Saving with a secret left blank SHALL keep the stored value. An identifier that is no secret (a zone's ID) shows its value. Changing the provider SHALL replace the whole `credentials` list. The contact email acme-common requires SHALL be asked for.

#### Scenario: Reopening a saved setup
- **WHEN** the operator opens Access after saving a Cloudflare token
- **THEN** the token field shows that a token is set and the page source holds no part of the token

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
- **THEN** a request for its domain starts without any further action, and the part shows it as requesting

### Requirement: An issued or renewed certificate is served without a restart
When the acme package issues or renews the certificate for the `verso` section's first domain while the section is enabled, Verso SHALL serve it from the next TLS handshake. Open sessions and connections SHALL survive. A certificate issued for any other domain or section SHALL leave the served certificate unchanged.

#### Scenario: First issue
- **WHEN** the request for `router.example.com` succeeds
- **THEN** the next visit to `https://router.example.com` gets the Let's Encrypt certificate, and the operator's session is still signed in

#### Scenario: Another section renews
- **WHEN** a second cert section for `nas.example.com` renews
- **THEN** the certificate Verso serves does not change

### Requirement: The renewal clock stays with the acme package
Renewal SHALL run on the acme package's own schedule. A renewal that acme.sh skips because the certificate is not yet due SHALL change nothing.

#### Scenario: A nightly check with nothing due
- **WHEN** the nightly renew runs 20 days after issue
- **THEN** the served certificate and the recorded outcome are unchanged

### Requirement: The part reports the certificate's state
The part SHALL say what last happened and how long ago: requesting, issued, renewed or failed. It SHALL also give the expiry date of the certificate acme holds, and whether Verso is serving it. The state SHALL come from the acme package's events and the certificate file, never from the system log. A failure SHALL be stated in plain words, with how long ago it came.

#### Scenario: After a renewal
- **WHEN** the certificate renewed three days ago and expires on 1 January
- **THEN** the part says it renewed three days ago, gives 1 January as the expiry, and says Verso serves it

#### Scenario: A failed request
- **WHEN** the provider rejects the token and the request fails
- **THEN** the part says the request failed and how long ago, and asks the operator to check the domain and the credentials

#### Scenario: After a reboot
- **WHEN** the router reboots after an earlier renewal
- **THEN** the part still gives the certificate's expiry and whether Verso serves it, read from the certificate file

### Requirement: A missed delivery catches up
When Verso starts, and whenever the `acme` config changes, Verso SHALL compare the certificate acme holds for the enabled `verso` section with the one it serves. It SHALL serve acme's when they differ and acme's is currently valid. When they are the same, it SHALL change nothing and send no reload.

#### Scenario: A power cut during delivery
- **WHEN** the router lost power after acme renewed but before Verso served the renewal
- **THEN** after the next boot Verso serves the renewed certificate

#### Scenario: Turning the certificate back on
- **WHEN** the operator re-enables the section while a valid certificate for its domain is already on disk
- **THEN** Verso serves that certificate as soon as the change is applied, without waiting for a renewal

#### Scenario: Nothing to catch up
- **WHEN** Verso starts and already serves acme's certificate
- **THEN** the certificate files are untouched and Verso is not signalled

### Requirement: A hand-installed certificate turns the trusted certificate off
Installing a certificate or making a new one on Access SHALL set `acme.verso.enabled` to 0, so no renewal replaces it. While the section is enabled, the Install and Make drawers SHALL say this before the operator confirms.

#### Scenario: Pasting a certificate over an ACME one
- **WHEN** the `verso` section is enabled and the operator opens Install
- **THEN** the drawer says that installing turns off the trusted certificate from the operator's domain, and confirming leaves `acme.verso.enabled` at 0

#### Scenario: No trusted certificate set up
- **WHEN** there is no `verso` cert section, or it is disabled
- **THEN** the Install and Make drawers say nothing about it

### Requirement: The root install command refuses a bad pair
The router's root-only path that installs a certificate SHALL accept only a certificate and key that match and are currently valid. It SHALL store the key in PKCS#8 form whatever form it was given in, and Verso SHALL serve the pair from its next handshake. A refused pair SHALL leave the served pair and its files untouched.

#### Scenario: An EC key from acme.sh
- **WHEN** the command is given a certificate and a key in `BEGIN EC PRIVATE KEY` form that match
- **THEN** the key is stored as `BEGIN PRIVATE KEY`, readable by the `verso` account alone, and Verso serves the pair from its next handshake

#### Scenario: A mismatched pair
- **WHEN** the command is given a certificate and someone else's key
- **THEN** it exits non-zero naming the mismatch and the served pair is unchanged

### Requirement: The domain is offered an answer on the LAN
When no DNS record in `/etc/config/dhcp` answers the domain, the part SHALL offer to add one. With verso-plugin-dnsdhcp installed, the offer SHALL open its New record drawer with the domain and the router's LAN address filled in, and staging the record stays that drawer's act. Without it, the part SHALL name the record to add.

#### Scenario: Adding the LAN record
- **WHEN** no record answers `router.example.com` and the operator takes the offer
- **THEN** the New record drawer opens as an A record for `router.example.com` with the router's LAN address, and nothing is staged until the operator saves it

#### Scenario: A record already answers
- **WHEN** a record answers `router.example.com`
- **THEN** the part makes no offer

#### Scenario: DNS and DHCP plugin not installed
- **WHEN** verso-plugin-dnsdhcp is not installed and no record answers the domain
- **THEN** the part names the record to add, the domain and the router's LAN address, and links nowhere
