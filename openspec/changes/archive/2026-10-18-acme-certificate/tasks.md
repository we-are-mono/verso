# Tasks

Every task ends green under `make lint` and `make test` in the repository it
touches. In a plugin repository, that repository's own `make lint` and
`make test` are the gate. Tests are written first; each task names its test.
Public files (specs, docs, tests, commits) use `example.com` for domains.

## 1. One pair check, one safe write (shell, `internal/tlscert`)

- [x] 1.1 Write `tlscert` tests first for `Validate(cert, key)`, then move
  `validateWebCertificate` out of `internal/server/access_credentials.go` into
  it, and have the Install drawer's path call it. The tests:
  - a matching pair whose key is in `BEGIN EC PRIVATE KEY` form comes back as
    `BEGIN PRIVATE KEY`;
  - a mismatched pair, an expired certificate, a not-yet-valid certificate and
    an oversized input each come back as the existing authored errors.

  Verify: `access_credentials_test.go` passes unchanged.
- [x] 1.2 Write a `tlscert` test first: when the certificate's rename fails
  (directory made read-only after the key's write), `Write` puts the old key
  back and the old pair still matches. Then add the restore to `Write`.
  Verify: the test passes, and `TestCertificateGenerate*` in
  `cmd/verso/main_test.go` still pass.

## 2. `verso certificate install` (shell, `cmd/verso`)

- [x] 2.1 Write `main_test.go` cases first for
  `certificate install DIR CERT KEY`. Then implement it beside `generate`,
  with the usage line naming both. The cases:
  - a valid pair lands in DIR with the key owned by the given uid, mode 0600,
    in PKCS#8 form;
  - a mismatched pair exits non-zero, names the mismatch, and leaves DIR's
    files byte-identical;
  - a missing file exits non-zero.
- [x] 2.2 Write a test first that `certificate install` prints what it
  installed (the subject and not-after) on success and touches nothing else.
  Then implement the message. The reload signal belongs to `deliver` (task
  3.2). Verify: the test passes.

## 3. The root side of ACME (shell, `verso-rpcd/src/acme.rs`)

- [x] 3.1 Add fixtures from the pinned packages under
  `verso-rpcd/testdata/acme/`: the hotplug actions `hook.sh` sends, extracted
  as a list (acme-common and acme-acmesh are GPL-3.0-only, so their scripts
  are not copied into this repository), and a `service list` shape with an
  `acme` instance named `verso`. Write a test first that the hotplug actions
  `deliver` handles are exactly the ones `hook.sh` sends. Verify: the test
  passes against the fixtures.
- [x] 3.2 Write tests first for `deliver`, with the uci reader, the clock, the
  file roots and the install runner injected. Then implement it. The tests:
  - `issued` for `acme.verso`'s first domain writes the record, runs install
    with `/etc/ssl/acme/<domain>.fullchain.crt` and `.key`, then signals Verso
    when its procd instance runs, and not when it isn't running;
  - a failed install sends no signal;
  - `renew-failed` writes the record and runs no install;
  - an event for another domain does nothing;
  - a disabled or absent section does nothing;
  - `sync` with acme's certificate equal to the served one runs no install;
  - `sync` with a different, valid one runs install and writes no record;
  - `sync` with an expired one changes nothing: `verso certificate install`
    refuses it, and no signal is sent;
  - the record is replaced by rename, never left half-written.
- [x] 3.3 Add the `acme deliver <event>` command-line mode to `main()`, beside
  `firewall_log::command_line()`. Write a test first that an unknown event
  exits non-zero and `--socket` parsing is unchanged. Verify: the test passes,
  and `verso-rpcd --socket PATH` still serves.
- [x] 3.4 Write tests first for `acmeState`. Then implement the verb and add it
  to `EVERY_METHOD`. The tests:
  - outcome and time from the record, and none when there is no record or the
    record is for another domain;
  - `requesting` from the `service list` fixture;
  - acme's certificate as PEM (the shell turns it into facts, task 3.7);
  - `serving` true only when it equals the served certificate;
  - no key material in any answer.
- [x] 3.5 The grant of `acmeState`, the reads of `uci acme`, `dhcp` and
  `network`, and the write of `uci acme` come from the plugin's rpcd ACL file
  (`/usr/share/rpcd/acl.d/verso-plugin-acme.json`, task 8.1), as for every
  plugin; `grants.rs` reads it and needs no change. Verify: the existing
  grant tests pass.
- [x] 3.6 Write tests first: with an enabled `acme.verso`, `setWebCertificate`
  and `makeWebCertificate` leave `enabled` 0 after the pair is written; with it
  disabled or absent, the acme config is untouched; a refused pair changes
  nothing. Then implement it inside the existing maintenance lock, with root's
  uci delta directory. Verify: the tests pass.
- [x] 3.7 Write a shell test first: `acmeState` reaches a plugin with acme's
  certificate as the Certificate card's facts, never as PEM; an answer without
  a certificate passes through; an unreadable certificate is dropped and the
  rest kept. Then add `ACMEState` to the backend and `acmeState` to the
  brokered reads. Verify: the test passes.

## 4. The shell's part stands (shell)

The hotplug script and the catch-up triggers ship with verso-plugin-acme
(tasks 8.2–8.3), so the `verso` package carries no ACME hook.

- [x] 4.1 Run `graphify update .`. Verify: `make lint` and `make test` are
  green in the shell repository (apart from failures already on main before
  this change), and groups 1–3 are committed as ordered units.

## 5. Plugins learn which plugins are installed (shell + SDK)

- [x] 5.1 Write a shell test first: a plugin request carries the installed
  plugin ids. Then pass them in the plugin request envelope beside the
  snapshot, as the `X-Verso-Plugins` header. Verify: the test passes, and a
  plugin built against the previous SDK still renders (an unknown header is
  ignored).
- [x] 5.2 Write an SDK test first that parses the header exactly as the shell's
  transport test sends it: `Request` exposes the installed ids, plus a
  `has_plugin(id)` helper. Then implement it in `verso-plugin-sdk`, fix the
  in-tree plugins' test requests for the new field, and document the header
  (and `acmeState`) in `docs/plugins.md`. Verify: `cargo test` passes in the
  SDK and every in-tree plugin.

## 6. dnsdhcp's New record drawer takes a prefill (verso-plugin-dnsdhcp)

- [x] 6.1 Write tests first in `src/records.rs`, then let `blank` read the
  query. The tests:
  - `/dns/records/new?name=router.example.com&target=192.168.1.1` opens as an A
    record with both filled in;
  - an invalid address or name is shown as typed with the drawer's existing
    validation marks;
  - other query keys are ignored;
  - nothing is staged.

  Verify: the tests pass.

## 7. The Install and Make drawers warn (verso-plugin-system)

- [x] 7.1 Add a read of `uci acme` to the manifest and its grant. Write tests
  first in `src/credentials.rs`: with `acme.verso.enabled` 1, both drawers
  carry the sentence that confirming turns off the trusted certificate from the
  operator's domain; with it 0 or absent, they don't. Then implement it, with
  the words in the plugin's catalog and Slovenian added. Verify: the tests
  pass.

## 8. verso-plugin-acme (new repository)

- [x] 8.1 Scaffold `plugins/verso-plugin-acme` from verso-plugin-ddns's layout:
  - Makefile, `rust-toolchain.toml`, init script with its own uid, rpcd ACL,
    `manifest.json` as design §9;
  - `DEPENDS := verso>=<this release> acme-acmesh acme-acmesh-dnsapi`, with the
    SDK pinned by the verso tag;
  - `git init`, and add `acme` to `NONCORE_PLUGINS` in `local.mk`.

  Verify: `make build` gives arm64 and amd64 static binaries, and an empty
  part renders on Access in the dev container.
- [x] 8.2 Add `rootfs/etc/hotplug.d/acme/50-verso`
  (`exec /usr/sbin/verso-rpcd acme deliver "$ACTION"`) to the package. Verify:
  `make apk` lists it, and in the dev container
  `ACTION=renew-failed main_domain=router.example.com hotplug-call acme` with
  an enabled `acme.verso` for that domain leaves a record that
  `ubus call verso acmeState` reads back.
- [x] 8.3 In the plugin's init script, run `verso-rpcd acme deliver sync` in
  `start_service` before the plugin's instance, and add
  `procd_add_config_trigger "config.change" "acme"` running the same command.
  Verify in the dev container:
  - a fixture certificate under `/etc/ssl/acme/`, with `acme.verso` enabled, is
    served after `/etc/init.d/verso-plugin-acme restart`;
  - turning `acme.verso` on through Verso's apply, with it off before, serves
    it without a restart.
- [x] 8.4 Write a test first that every provider's `dns` name and credential
  variables appear in the pinned acme.sh `dnsapi/` scripts. The fixture is an
  extracted list of each script's name and variables, since acme.sh is
  GPL-3.0-only and its scripts are not copied. Then write the `PROVIDERS`
  table. Verify: the test passes.
- [x] 8.5 Write tests first for the part's form, then implement it. The tests:
  - a first save stages the `verso` section as the spec's first scenario;
  - a section with an uncurated `dns` opens as Other with its list;
  - credentials render as set without their value;
  - a blank credential keeps the stored list;
  - a provider switch replaces the list;
  - `account_email` is required (acme-common runs no request without one) and
    the shipped `email@example.org`, which Let's Encrypt refuses, is not taken.

  The form is built from existing widgets only.
- [x] 8.6 Write tests first for the state lines from `acmeState` answers. Then
  implement them. The tests cover:
  - requesting;
  - issued or renewed on a date, with expiry and served;
  - failed on a date, with the plain advice;
  - no record (expiry and served from the file);
  - a held certificate Verso doesn't serve.
- [x] 8.7 Write tests first for the LAN offer. Then implement it. The tests:
  - no `domain` record for the domain and dnsdhcp installed → a link to the
    prefilled New record drawer with the `lan` IPv4 address;
  - a record exists → no offer;
  - dnsdhcp absent → the record named in words, with no link.
- [x] 8.8 Write tests first for the describe hook (pending-changes sentences
  for `acme` changes, never echoing a credential). Then implement it, and put
  every string in the plugin's catalog with Slovenian added. Verify: the
  tests pass and the shell's i18n audit stays at zero.
- [x] 8.9 Write the README (what it does, what it depends on, the provider
  table, `uci show acme` as the page reads it). Verify: `make apk` builds for
  both arches, `make dev` deploys it to the container, and its part renders.

## 9. Documents (shell)

- [x] 9.1 Amend ADR-017 §6 in place: the decisions of design.md §2–§8 (delivery
  path, `verso certificate install`, the record and its catch-up, one owner in
  `verso-rpcd`, the hand-off to dnsdhcp, the provider set). Update §3–§4
  wherever they name the writers. Verify: §6 no longer names
  `ubus call verso setWebCertificate` from the hook.
- [x] 9.2 Update `docs/BACKLOG.md`'s step 4 entry to what landed and what
  stays open. Verify: the entry matches the merged code.

## 10. On the DUT

- [x] 10.1 Install the Verso release, verso-plugin-acme and the acme packages
  on the DK. Set up the operator's test domain with its real DNS provider,
  then verify:
  - a certificate is issued and served without a restart;
  - the part shows issued and served;
  - `https://<domain>` opens on the LAN with no warning (its LAN answer kept by
    AdGuard Home, which the part's lookup reads as answered).

  Verified in the dev container, open on the DK in docs/BACKLOG.md: the LAN
  offer through dnsdhcp, the HTTP redirect, Install by hand turning
  `acme.verso` off, and re-enabling serving the held certificate.
- [x] 10.2 Force a failure, then verify the part says failed with how long ago
  and the served certificate is unchanged. Verified on the DK with a user token
  ("no account associated with this token"), another account's token
  ("Subscription required"), and a token acme.sh's saved copy overrode, which
  Verso now forgets.

## Workflow follow-up

- Cut a Verso release carrying groups 1–5 before publishing the plugin, whose
  apk depends on it; the release notes name the plugin and the dependency.
- Archive the change once ADR-017 and docs/BACKLOG.md reflect what landed.
