# Proposal

## Why

An operator who owns a domain should be able to open the router at
`https://router.example.com` with no browser warning. Today they get that only
by pasting a certificate they obtained elsewhere and replacing it by hand every
few months. ADR-017 §6 planned `verso-plugin-acme` for this, but reading the
code and the OpenWrt `acme` package turned up three gaps in that plan: the
delivery path it names cannot work, the LAN record it writes belongs to another
plugin, and nothing in it reports a failed request.

## What Changes

What the operator sees: on Access, below Certificate, a **Trusted certificate**
part asks for a domain, a DNS provider and that provider's token. Saving it
gets a Let's Encrypt certificate by DNS-01 within a minute or two. Verso starts
serving it with no restart. It renews on its own before it expires, and the part
says when it last succeeded or that a request failed. The router does not need
to be reachable from the internet.

- **New plugin `verso-plugin-acme`** (Rust, its own repository and apk, depending
  on `verso`, `acme-acmesh` and `acme-acmesh-dnsapi`). It owns `/etc/config/acme`
  and edits the `config cert 'verso'` section. Its part of Access offers a
  curated set of providers with named token fields (Cloudflare, DNSimple, deSEC,
  DuckDNS, dynv6) and **Other**, which takes the raw `credentials` list.
  Credentials are write-only.
- **OpenWrt's `acme` package does the ACME work.** Its config trigger issues on
  save and its nightly cron line renews. Verso adds no ACME client.
- **New root command `verso certificate install CERT KEY`** in the shell binary.
  It checks the pair (they match, it is currently valid), converts the key to
  PKCS#8, writes it to `/etc/verso/` and signals Verso to reload. It knows
  nothing of ACME. `tlscert.Write` gains the old-key restore that `verso-rpcd`'s
  write path already has.
- **Delivery through acme's hotplug.** `/etc/hotplug.d/acme/50-verso` runs
  `verso-rpcd acme deliver <event>` on `issued` and `renewed`. That command
  records the outcome in a RAM record and installs the certificate when
  `acme.verso` is enabled. Failures (`issue-failed`, `renew-failed`) are
  recorded too. No status is read from logs.
- **Catch-up.** The same deliver step runs when Verso starts and whenever the
  `acme` config changes. A missed event, or a section re-enabled while a
  valid certificate is already on disk, gets fixed without waiting for the next
  renewal.
- **New read verb `acmeState`** on `verso-rpcd`. It returns the recorded outcome
  and the facts of the certificate acme holds (never the key) for the plugin's
  part of Access.
- **One owner of the served certificate.** Installing a certificate or making a
  new one by hand sets `acme.verso.enabled` to `0`. The Install and Make drawers
  (verso-plugin-system) say so before the operator confirms, when the section is
  enabled.
- **The domain on the LAN.** When no DNS record answers the domain on the LAN,
  the plugin's part offers to add one. The offer opens verso-plugin-dnsdhcp's
  New record drawer, filled in with the domain and the router's LAN address.
  verso-plugin-dnsdhcp owns `/etc/config/dhcp`, so acme writes nothing there.
  The New record drawer gains that prefill.
- **Plugins learn which plugins are installed.** The shell passes every plugin
  request the ids of the installed plugins, and the SDK exposes them. This is
  how acme knows whether dnsdhcp is there to hand off to. The field is
  additive, so installed plugins keep working unchanged.

## Capabilities

### New Capabilities
- `trusted-certificate`: obtaining, delivering, renewing and reporting a
  publicly trusted web certificate for an operator's domain. Also covers its
  single owner and the domain's answer on the LAN.

### Modified Capabilities
None. The certificate, Access and DNS record behaviour this touches has no spec yet.

## Impact

- **ADR-017 §6** is amended in place: the delivery path (hook →
  `verso-rpcd acme deliver` → `verso certificate install`), the status record
  and its catch-up, the hand-off of the LAN record to dnsdhcp, and the provider
  set. §7 (redirect to the certificate's name) already stands as built and is
  unchanged.
- **ADR-006** (config ownership) and **ADR-007** (plugins hold no privilege)
  are followed: no plugin writes another's config, and the root-side work lives
  in `verso-rpcd`, as for OpenVPN and DDNS.
- **Shell repository:** `cmd/verso` (new `certificate install`),
  `internal/tlscert` (shared pair validation, old-key restore),
  `internal/server/access_credentials.go` (uses the shared validation),
  `verso-rpcd` (`acme.rs`: `acmeState`, the `deliver` command; turning
  `acme.verso` off in `setWebCertificate`/`makeWebCertificate`; grants),
  the request a plugin receives (installed plugin ids).
- **verso-plugin-sdk:** the installed plugin ids on `Request`.
- **verso-plugin-system:** a read of `acme`, plus the warning in the Install and
  Make drawers.
- **verso-plugin-dnsdhcp:** the New record drawer takes a prefill from its query.
- **New repository `we-are-mono/verso-plugin-acme`**, added to `NONCORE_PLUGINS`.
  Its package also carries the acme hotplug script and the catch-up triggers,
  so a router without it has no ACME hook.
- **Compatibility:** the plugin needs the Verso release that carries `acmeState`
  and `certificate install`, so its apk depends on `verso>=` that version.
  Nothing a router already holds is renamed or moved.
- **Footprint:** only on routers that install the plugin. acme.sh brings in
  `wget-ssl`, `openssl-util` and `socat`. The plugin itself is one static Rust
  binary.
