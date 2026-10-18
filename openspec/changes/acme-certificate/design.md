# Design

## Context

See proposal.md for why. The current state this design builds on:

- **Serving.** Verso serves `/etc/verso/tls.crt` and `tls.key`. The key is 0600
  and belongs to the `verso` account (ADR-017 §3). `SIGHUP` re-reads the pair
  and open sessions survive (§4). `tlscert.Store.Name` already redirects to the
  served certificate's DNS name when another party issued it (§7).
- **Writers today.** `verso certificate generate` (Go, run as root by the
  `verso` init) writes through `tlscert.Write`. `verso-rpcd`'s
  `setWebCertificate` (Rust) writes through `access::install`, which puts the
  old key back if the certificate's rename fails. The shell checks a pasted
  pair in `validateWebCertificate` (`internal/server/access_credentials.go`):
  the pair matches, it is currently valid, and the key is re-encoded as PKCS#8.
  `verso-rpcd` only accepts `BEGIN PRIVATE KEY`.
- **The acme package** (`acme-common` 1.5.3, `acme-acmesh` 3.1.3):
  - `/etc/init.d/acme` runs one procd instance per enabled `cert` section,
    named after the section.
  - Its config trigger on `acme` restarts those instances. A nightly cron line
    runs `renew`.
  - `hook.sh` calls `/usr/lib/acme/notify` with `issued`, `renewed`,
    `issue-failed` or `renew-failed`. It sends nothing when a renewal isn't due
    yet (exit 2). Notify runs `hotplug-call acme` with `ACTION` set; the hook's
    environment carries `main_domain` but not the section name.
  - Files land as symlinks: `/etc/ssl/acme/<domain>.fullchain.crt` and
    `<domain>.key`, pointing into `/etc/acme/<domain>_ecc/`. The key is root's
    alone, and with the default `key_type` `ec256` it is in SEC1
    (`BEGIN EC PRIVATE KEY`) form.
- **Precedent for root-side plugin work.** Plugins hold no privilege
  (ADR-007). The root-side halves of OpenVPN and DDNS live in the shell
  repository: a hotplug script writes a RAM record
  (`docker/rootfs/etc/hotplug.d/openvpn/50-verso`), and `verso-rpcd` reads it
  through a verb (`openvpn.rs`, `ddns.rs`) granted to the plugin.
- **Access composition.** Any plugin with `system_access` in its manifest adds
  a part to Access (`internal/server/password.go`). `acme.credentials` is
  already in `internal/secrets/secrets.json`.

## Goals / Non-Goals

**Goals:**
- The shell stays ignorant of ACME. Its only new surface is a generic, checked,
  root-only certificate install.
- Every outcome reaches the page from a daemon event or a file, never from
  logread.
- No plugin writes another plugin's config.
- Delivery heals itself after a missed event, without waiting for a renewal.

**Non-Goals:**
- HTTP-01, TLS-ALPN-01 and webroot validation. The router is not exposed for a
  challenge.
- Managing `cert` sections other than `verso`. They keep working through the
  acme package and are left alone.
- An ACME client of our own, and any choice of CA beyond the package's default
  (Let's Encrypt).
- Explaining why a request failed. The only source of the reason is the log.

## Decisions

### 1. Wrap OpenWrt's acme package
acme.sh, its 150+ DNS provider scripts, the procd instances and the cron line
are maintained upstream. The config is the same `/etc/config/acme` that
`luci-app-acme` edits. The certificate lands in `/etc/ssl/acme/`, where any
other service can use it.
- *Rejected: an ACME client in the plugin (`instant-acme`).* Small, but every
  DNS provider's API would become ours to write and keep working.
- *Rejected: an ACME client in the shell (`lego`).* It adds tens of MB to the
  shell, which shuts out the 16 MB tier. The unprivileged shell would also hold
  DNS credentials.

### 2. Root-side work lives in `verso-rpcd`, in a new `acme.rs`
`verso-rpcd acme deliver <event>` is a command-line mode, like
`firewall_log::command_line()`. The hotplug script stays a single line:
`exec /usr/sbin/verso-rpcd acme deliver "$ACTION"`. The `acmeState` verb lives
in the same module. Reasoning: decisions made as root (is the section enabled,
does the domain match, does the certificate differ) belong in tested Rust, not
in shell script. This also follows the OpenVPN and DDNS precedent.
- *Rejected: a root command mode in the plugin binary.* The plugin binary would
  then run as root, which goes against ADR-007's split even though it is the
  same file.
- *Rejected: logic in the hotplug script.* Untested root code.

### 3. The install command is in the Go binary: `verso certificate install CERT KEY`
The checks and the PKCS#8 conversion already exist in Go, and
`verso certificate generate` already writes the same pair as root.
- Move `validateWebCertificate` into `internal/tlscert` as `Validate`, used by
  both the server's Install drawer and the command (one reason to change, one
  place).
- `tlscert.Write` gains the old-key restore, so the Go and Rust writers give the
  same guarantee.
- The command checks and writes; it does not signal. The Go binary runs no
  external programs, and `verso-rpcd` already owns the signal (`announce()`).
  `deliver` sends it after a successful install, only when Verso's procd
  instance is running. When it isn't, the next start reads the pair.
- *Rejected: `verso-rpcd`'s `setWebCertificate` path.* It refuses SEC1 keys.
  Adding pair checks to it would mean an x509 crate in the helper and the same
  checks maintained twice.
- *Rejected: `ubus call verso setWebCertificate` from the hook.* A call with no
  session is granted two reads only (ADR-014), and widening that grant would
  give every local root caller a write it doesn't need.

### 4. Delivery: one deliver step, three triggers
`deliver` takes `issued`, `renewed`, `issue-failed`, `renew-failed` or `sync`:
1. Read `acme.verso` with `uci -q`. Stop if it is absent or disabled, or if,
   for a hotplug event, `main_domain` is not its first domain.
2. For an event, write the record `/var/run/verso-acme/verso`: the event and
   the time. The directory is 0755 root and the file is 0644, written to a
   temporary file and renamed. Only root writes here, unlike the OpenVPN
   directory.
3. Unless the event is a failure: when `/etc/ssl/acme/<domain>.fullchain.crt`
   exists, is currently valid, and its SHA-256 differs from `/etc/verso/tls.crt`,
   run `verso certificate install` with it and its key. Equal files do nothing.

Triggers, all shipped by verso-plugin-acme, so a router without the plugin
carries no ACME hook:
- **hotplug** (every event): the plugin's `/etc/hotplug.d/acme/50-verso`.
- **The plugin's init `start_service`,** as root, before it starts the plugin
  process: `verso-rpcd acme deliver sync`. The plugin starts before the shell
  at boot (START 93 against 95), so the first serve already has the right pair.
- **A procd config trigger on `acme`** in the plugin init's `service_triggers`,
  running `verso-rpcd acme deliver sync`. This covers re-enabling a section
  while a valid certificate is on disk, where acme.sh skips the renewal and
  sends no event.

`sync` writes no record: it does not change what last happened.

### 5. State: the record plus daemon facts
`acmeState` (granted to verso-plugin-acme, read-only) returns:
- `outcome` and `at` from the record (absent after a reboot);
- `requesting`: whether the acme procd instance named `verso` is running, from
  `ubus call service list`;
- the facts of acme's certificate file: subject, issuer, not-after, and
  whether it is the one served (fingerprint equality). It never returns the key.

The part reads its state from these. After a reboot it still has expiry and
serving from the file. A failure since the reboot shows again after the next
nightly run.

### 6. One owner: hand acts turn `acme.verso` off, in `verso-rpcd`
`setWebCertificate` and `makeWebCertificate` read `acme.verso.enabled`. When it
is 1, they set it to 0 and commit `acme`, after the pair is written and in the
same locked operation. This is immediate, like the act itself. Committing uses
root's own uci delta directory, so a session's staged changes are untouched.
`/etc/init.d/acme`'s trigger stops the instance. verso-plugin-system declares a
read of `uci acme`. When the section is enabled, its Install and Make drawers
add a sentence before the confirm.
- *Rejected: the plugin decides by fingerprint (serve only while the served
  certificate is the one it last delivered).* It writes nothing across configs,
  but it needs a hidden "claimed" state that `uci show acme` cannot express,
  and re-enabling would need a claim verb.

### 7. Config shape
The part edits:
- `config cert 'verso'`: `enabled`, `domains` (one entry), `validation_method
  dns`, `dns`, `credentials`.
- `config acme`'s `account_email`, required: acme-common refuses to run any
  request without it, and the `email@example.org` it ships is a contact Let's
  Encrypt refuses, so the form shows that one as empty and does not take it.

`key_type` is left at the package default (`ec256`), as are `staging` (0) and
`acme_server`. Other sections, including the package's disabled examples, are
left alone. Providers are a closed table in the plugin, as `PROVIDERS` is in
verso-plugin-ddns: label, acme.sh `dns` name, and each credential's variable
and field label. The acme.sh variable names are checked against the pinned
acme.sh's `dnsapi/` scripts in a test fixture. Other shows the `dns` name and
the `credentials` list as a text list (existing list widget). Credential fields
use the write-only secret field verso-plugin-ddns already renders.

### 8. The LAN record: hand-off to dnsdhcp, using installed plugin ids
- **acme reads** `uci dhcp` (`domain` sections) and `uci network` (the `lan`
  interface's IPv4 address) to decide whether to offer a record, and what
  address to fill in.
- **The offer** links to `/plugins/dnsdhcp/dns/records/new?name=<domain>&target=<address>`.
  dnsdhcp's `records::blank` takes `name` and `target` (its own field names)
  from the query; anything else in the query is ignored.
- **Knowing dnsdhcp is installed.** The shell passes the installed plugin ids
  with each plugin request, and the SDK exposes them on `Request`.
- *Rejected: acme writes the `domain` section.* That breaks ADR-006 ownership
  and would duplicate dnsdhcp's checks.
- *Rejected: always link.* With dnsdhcp absent, the link would be a 404.

### 9. Packaging
`we-are-mono/verso-plugin-acme` is laid out like verso-plugin-ddns:
- a static musl Rust binary, its init script with its own uid, and an rpcd ACL;
- `DEPENDS := verso>=<this release> acme-acmesh acme-acmesh-dnsapi`;
- manifest: `system_access: "/access"`, read of `uci acme`, `uci dhcp`,
  `uci network` and `ubus verso acmeState`, write of `uci acme`.

The hotplug script and both catch-up triggers ship in the plugin's package.
The `verso` package carries only what needs root and is ACME-agnostic or
general: `verso certificate install`, `verso-rpcd`'s `acme` module, and the
installed plugin ids.

## Risks / Trade-offs

- **[acme.sh changes its notify events or file layout]** → `deliver` reads only
  the documented hotplug actions and the `/etc/ssl/acme/` symlinks. A fixture
  test against the pinned `acme-common` and `acme-acmesh` files flags a change
  on the next bump.
- **[A failed renewal goes unseen after a reboot until the next nightly run]**
  → The expiry is always shown from the certificate file. Renewal starts 30
  days before expiry, so roughly 30 nightly retries come before the date shown.
- **[`credentials` hold a plaintext token in `/etc/config/acme`]** → This is
  the acme package's own format. The file is root's. `acme.credentials` is
  already a secret in Verso's history and diffs. The UI never returns a value.
- **[Rate limits from repeated failed requests]** → The acme package sets the
  request pace (config change, nightly cron). Verso adds no retries of its own.
- **[Turning `acme.verso` off from `verso-rpcd` is a commit outside the staged
  flow]** → It is a consequence of an immediate operator act the drawer warned
  about, and it shows in `uci show acme`.

## Migration Plan

Nothing on a router is renamed or moved.
- Routers without the plugin: the new hotplug script and the config trigger do
  nothing while `acme.verso` is absent.
- An operator who set up `acme.verso` by hand: the first `sync` serves its
  certificate.
- Rollback: removing the plugin package leaves the served certificate as it
  is. Verso keeps serving it until it expires or the operator installs another.
- The plugin's apk requires the Verso release that carries `acmeState` and
  `certificate install`. The release notes say so.
