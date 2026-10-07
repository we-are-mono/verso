# ADR-017 — Verso serves HTTPS with a certificate it owns

- **Status:** Accepted
- **Date:** 2026-10-07
- **Deciders:** tomaz@zaman.io
- **Relates to:** ADR-001 (Verso brings its own HTTP server), ADR-006 (plugins own
  their configs), ADR-007 (the shell is unprivileged; root acts ride `verso-rpcd`),
  ADR-013 (Verso's settings live in `/etc/config/verso`)

## Context

Verso is its own HTTP server (ADR-001). It listens in plain HTTP, so the session
cookie travels unencrypted and is marked `Secure` only when a request arrives over
TLS.

The Access page's Certificates card reads, replaces and generates the certificate
named by `uhttpd.@uhttpd[0].cert`, and its Web interface section edits uhttpd's
listeners. uhttpd is LuCI's web server. Verso does not run behind it, and cannot:
uhttpd runs CGI, Lua, ucode and ubus handlers but does not reverse-proxy. On an
image without LuCI, uhttpd holds ports 80 and 443 and serves nothing of Verso's;
the card manages a certificate no Verso page is served with.

Three situations have to work:

- **A Verso image.** No LuCI, no uhttpd. Verso is the router's web interface from
  first boot.
- **Verso added to a router running LuCI.** uhttpd holds 80 and 443. The operator
  wants to try Verso without losing LuCI, and to switch over when ready.
- **An operator with their own domain.** They want a publicly trusted certificate
  for a name such as `router.example.com`, renewed without their involvement, with
  the router reachable only from the LAN.

## Decision

**Verso terminates TLS itself, with a certificate and key under `/etc/verso/`.
A self-signed certificate is made at first boot; any other certificate, including
one from Let's Encrypt, is installed through the one write path that replaces it.
uhttpd is not part of Verso.**

### 1. Listeners are a Verso setting

The listeners live in `/etc/config/verso`, section `config web 'web'`:

| Option | Absent means |
|---|---|
| `list listen_https` | `0.0.0.0:443` and `[::]:443` |
| `list listen_http` | `0.0.0.0:80` and `[::]:80` |
| `option redirect_https` | `1` — every HTTP request is redirected to HTTPS |

The init script reads the options and hands them to the shell on its command
line, as uhttpd's init script hands its own to uhttpd; the shell holds the
defaults an absent option means. The Web interface section on Access edits these
options and is rendered by the shell, which owns the `verso` config (ADR-013).

**Listeners change at once, in the running shell.** The Web interface section
is a form with its own **Apply**, not a staged setting: moving the address the
page is on is an act whose outcome the person must see as it happens, and
restarting the shell on new listeners would end every session with it. Apply:

1. Checks the ports — numbers from 1 to 65535, HTTP and HTTPS apart — and binds
   the listeners the shell lacks, alongside the ones it has. A port that cannot
   be opened is said under its field, and nothing changes.
2. Answers by sending the page where the shell answers when the page's own
   listener is going; its button reads **Redirecting…** meanwhile. It is the
   same process, and a host's cookies reach every port on it, so the session
   holds, and the notification says the web interface moved.
3. Writes `verso.web` once the browser arrives there, through `verso-rpcd`'s
   `setWebListeners`, which commits those options alone and leaves whatever
   the session stages; the shell then closes the listeners left behind. A
   change that does not move the page is written and kept at once.
4. A browser that has not arrived within 90 seconds — long enough to pass a
   certificate warning on the new port — leaves nothing written; the shell
   closes what it added. A shell that restarts meanwhile starts where
   `verso.web` says, which is where it answered before.

This is the one Verso setting that does not stage (ADR-013 §3): its apply is
the move itself.

The init script declares `verso` as a reload trigger and sets `reload_signal
HUP`. procd sends that signal in place of a restart when the instance changes,
and records the new command line: the write that changed the listeners leaves
the running shell as it is, and a later respawn starts on them. A hand edit
over SSH takes effect on `/etc/init.d/verso restart`.

`CAP_NET_BIND_SERVICE`, kept by the init script, is what lets the unprivileged
`verso` user bind 80 and 443. procd grants capabilities only through ujail, and
runs ujail only for an instance that declares a jail, so the script declares
one that names the service and asks for no namespace: the shell sees the
router's filesystem as it is.

### 2. Beside uhttpd, Verso takes 8443

When the listeners are absent and another process holds the router's ports —
or the shell was not given the capability to bind them — Verso listens on
`:8443` for HTTPS alone and logs why. It never edits uhttpd's
configuration on its own. Explicit listeners are bound as written; a port in use
is a start failure, logged.

When uhttpd is installed, Access carries a **LuCI** section saying where each
server answers and offering the move, confirmed inline in marigold because the
address the operator is on changes:

- Beside LuCI: **Make this the router's web interface**.
- Holding the router's ports: **Hand the web interface back to LuCI**.

With listeners set in `verso.web`, or no uhttpd, the section is absent and
nothing moves. A shell on 8443 because it was refused the router's ports
offers no move either: taking them would fail again, and uhttpd, moved to
8443, would find Verso there. Its **Web interface** section says instead that
the service lacks the permission to bind ports below 1024. `verso-rpcd`'s `setWebOwner` runs
`/usr/libexec/verso/web-owner` after answering: it stops both servers, moves
uhttpd's listeners (`:8080`/`:8443` when Verso takes the ports, `:80`/`:443`
when LuCI does), and starts the new owner first, so it binds the router's ports
while they are free, then the other. LuCI stays installed and reachable.

Both acts take effect at once rather than staging: they change which server
answers the page the operator is on. The answer is the restarting takeover,
told the address Verso answers at afterwards: once the old address stops
answering as Verso, the takeover follows to the new one as soon as it answers,
or after fifteen seconds regardless, so a browser that has not yet trusted the
new address shows why on a page it loads itself.

Both servers sign in through rpcd with the root password and read and write the
same uci configs, so nothing moves between them.

### 3. The certificate files

| File | Mode | Owner |
|---|---|---|
| `/etc/verso/tls.crt` | 0644 | root |
| `/etc/verso/tls.key` | 0640 | root:verso |

The certificate file holds the leaf followed by its chain. The key is the one file
under `/etc` the shell opens directly: it is credential material Verso must hold
to serve, not configuration, so the brokered-read rule of ADR-007 and ADR-013 does
not apply to it. `verso-rpcd` remains the only writer.

The package installs `/lib/upgrade/keep.d/verso` naming `/etc/verso/`, so the
certificate survives sysupgrade with the configs.

### 4. One write path, reloaded in place

`verso-rpcd`'s `setWebCertificate` writes both files (each to a private
temporary file, then renamed; the old key restored if the certificate's rename
fails) and asks procd to send Verso `SIGHUP` (`ubus call service signal`); Verso
re-reads the pair and serves it from the next handshake. Open connections and
sessions survive a certificate change. The signal is sent directly: procd sends
its `reload_signal` only when the instance changes, and a new certificate
changes no part of it.

The shell validates a pasted pair before passing it on: the certificate and key
must match, the certificate must be within its validity period, and the key is
normalised to PKCS#8.

### 5. A self-signed certificate from first boot

When `/etc/verso/tls.crt` is absent, the init script runs `verso certificate
generate` as root before starting the shell. **Make a new one** on Access runs
the same step: `verso-rpcd`'s `makeWebCertificate` calls the init script's
`certificate` command, then signals the shell as `setWebCertificate` does. The
key is made as root and never passes through the shell. The certificate is
ECDSA P-256, valid two years, `serverAuth`, and named for every way the LAN
reaches the router:

- the hostname (`system.@system[0].hostname`),
- the hostname under the local domain (`dhcp.@dnsmasq[0].domain`, `lan` when absent),
- the `lan` interface's static IPv4 addresses and its IPv6 ULA address.

Addresses from a delegated IPv6 prefix are not named: they change with the
upstream, and the certificate would not.

An IP address is named as an IP SAN, a name as a DNS SAN. A visit by IP or by name
matches the certificate once the operator trusts it on their device.

### 6. A trusted certificate from the operator's own domain

`verso-plugin-acme` (Rust) obtains a certificate for a domain the operator
controls, by DNS-01 challenge only. It ships as its own package, depending on
`verso`, `acme-acmesh` and `acme-acmesh-dnsapi`; an operator without a domain does
not install acme.sh, `wget-ssl`, `openssl-util` or `socat`.

- **Config.** The plugin owns `/etc/config/acme` (ADR-006). The certificate Verso
  serves is the section `config cert 'verso'`: `domains`, `validation_method dns`,
  `dns` (an acme.sh provider name such as `dns_cf`) and `credentials`. `uci show
  acme` reads as the page does.
- **Place.** The plugin contributes its part of Access through `system_access`,
  beside the Certificates card.
- **Providers.** A curated set of common DNS providers is offered with named
  credential fields. **Other** edits the raw `credentials` list, so any acme.sh
  provider remains usable.
- **Secrets.** DNS API credentials are write-only in the UI: the form shows that a
  credential is set and offers to replace it, never its value.
- **Delivery.** The plugin installs `/etc/hotplug.d/acme/50-verso`. On
  `ACTION=issued` or `ACTION=renewed`, running as root, it reads `acme.verso`'s
  first domain and hands `/etc/ssl/acme/<domain>.fullchain.crt` and its key to
  `ubus call verso setWebCertificate`, the same path as a pasted certificate.
  The shell knows nothing of ACME.
- **The name on the LAN.** The plugin offers to answer the domain on the LAN with
  the router's address, writing a `config domain` section in `/etc/config/dhcp`
  (the record LuCI calls a hostname). A public A record pointing at a private
  address is refused by dnsmasq's rebind protection; the local record is not.
- **One owner of the certificate.** Installing or making a certificate by hand
  sets `acme.verso.enabled` to `0`, so a renewal never overwrites it. The
  Install and Make drawers say so before the operator confirms when the section is
  enabled. Re-enabling it on the plugin's part of Access requests a certificate at
  once.

### 7. Where the browser is sent

With `redirect_https` on, an HTTP request is redirected to HTTPS on the same host.
When the served certificate is not self-signed and names a DNS SAN, the redirect
goes to that name instead. The shell reads the name from the certificate; it does
not consult the acme config.

## Consequences

- The session cookie is `Secure` on every device whose listeners include HTTPS,
  without a change to the cookie code.
- The Certificates card describes the certificate Verso serves. Its facts —
  subject, signer, validity, fingerprint — read the same files the server uses.
- The system plugin's uhttpd ACL scopes and its Web interface section go; the
  shell's Web interface section edits `verso.web`.
- A Verso image drops `uhttpd`, `uhttpd-mod-ubus`, `px5g-mbedtls` and `luci-ssl`.
  `libustream-mbedtls` stays only if another package needs it.
- An operator sees the browser's self-signed warning on first visit. Access
  shows the fingerprint written as the browser writes it, to compare before
  trusting.
- A trusted certificate requires a domain the operator controls and a DNS host
  with an API that acme.sh supports. Issued certificates are logged publicly in
  Certificate Transparency, so the chosen name is public.
- A device whose browser or OS resolves through its own DNS-over-HTTPS server, or
  a hard-coded public resolver, does not see the LAN record for the domain; it
  reaches the router by IP or by a public record the operator publishes.
- The router's admin interface stays off the WAN: DNS-01 needs no inbound
  connection.

## Alternatives considered

- **Verso behind uhttpd.** uhttpd cannot reverse-proxy.
- **Verso behind nginx.** A second web server, its TLS stack and its config to
  carry, for nothing Go's `crypto/tls` does not do in the shell.
- **The shell disables uhttpd at install.** Silently takes ports from an operator
  who chose LuCI. The switch-over is an act the operator takes.
- **HTTP-01 or TLS-ALPN-01 challenges.** Both need ports 80 or 443 reachable from
  the internet, which exposes the admin interface on the WAN and fails behind
  carrier-grade NAT.
- **A Mono-operated domain with per-router subdomains** (an acme-dns service under
  a Mono domain, as Plex does with `plex.direct`). Zero-configuration for the
  operator, but it makes Mono run an internet service every router depends on for
  renewal, needs a Public Suffix List entry to stay within Let's Encrypt's rate
  limits, and publishes every router's chosen name under one domain. The
  operator's own domain carries none of that.
- **ACME inside the shell** (`golang.org/x/crypto/acme` or lego). Duplicates
  acme.sh's provider catalogue in the shell and makes every install carry it.
  OpenWrt's `acme` package is maintained, configured in uci and signals renewals
  through hotplug.
- **HSTS.** Makes every certificate error on the host one the browser offers no
  way past, and a self-signed certificate not yet trusted on a device is exactly
  such an error.
