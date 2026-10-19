# Tasks

## 1. The registry declares events

- [ ] 1.1 Test first in `internal/plugin/registry_test.go`:
  - `events` parses into each declaration's name, sentence, tone, read, alert and ends;
  - each of these is dropped with a logged reason while the plugin's other declarations stay: a name outside `[a-z0-9-]{1,64}`, an unknown tone, a read the registry does not register, an `ends` naming no event of the plugin's;
  - a plugin whose id is `wan`, `boot`, `changes`, `storage` or `certificate` has every declaration dropped;
  - a registry with no `events` parses as before.

  Then implement it (design decisions 3 and 4). Verify with `make test`.
- [ ] 1.2 Test first in `plugins/verso-plugin-sdk`: the registry builder's `event(…)` refuses every declaration 1.1 drops, and a registry with two events serializes into `testdata/commands.json`. The shell's conformance test decodes that fixture with the same declarations. Then implement it. Verify with `cargo test` in the SDK and `make test`.
- [ ] 1.3 Amend ADR-027 in place: the registry's `events`, and `announce` beside `own_write` in decision 9. Add the "Declaring events" part of a new `docs/plugins.md` section, "Events". Its example is checked by a doc test against the SDK's `Registry`. Verify with `cargo test` in the SDK.

## 2. One kind table

- [ ] 2.1 Test first in a new `internal/events`: the core kinds carry the words, tone, grant, said flag, alert kind and ends of design decision 5's table.
  - A plugin's declared events join the table as `<plugin>.<event>`.
  - A recorded plugin event whose plugin has been removed still gives its English sentence, its arguments filled in, and the unknown-kind visibility.

  Then implement the table, rebuilt from the manifests on every rescan. Verify with `make test`.
- [ ] 2.2 Test first in `history_events_test.go` and `config_history_grants_test.go`:
  - every existing row and grant test passes unchanged against the table;
  - a plugin event's row reads its sentence through the plugin's catalog, its arguments as written, in its tone's colour, with its subject in the Interface column;
  - a sentence with no translation reads in English;
  - the event shows only under `verso-plugin <plugin> <read>`.

  Then move `eventRow`, `eventGrants` and `eventsView.show` onto the table. Add `ubus verso accessCredentials` to `shellProbes`. Verify with `make test`: the root-grants test also holds the new probe.
- [ ] 2.3 Amend ADR-019 in place: the events table gains a plugin's announced events and the certificate's replacement, the VPN row leaves, and the kinds become one table. Verify by checking that `grep -n 'VPN tunnel' docs/ADR/019-history-and-events.md` finds nothing.

## 3. Announcing

- [ ] 3.1 Test first in `internal/events` (injected euid, temp spool, registries in a temp plugins dir):
  - each of these is refused, with a reason, leaving nothing: a non-root caller, an undeclared kind, arguments that do not fill the sentence, a subject that is not a device name, an argument over 256 bytes or with a newline, a ninth argument, 256 announcements waiting, a missing spool;
  - an accepted one is written `O_EXCL`, mode `0600`, renamed into place as `<ns:020>-<owner>.<event>.json`;
  - the reader's parser reads it back field for field.

  Then implement the writer and the reader's parser (design decision 2). Verify with `make test`.
- [ ] 3.2 Test first in `cmd/verso/main_test.go`:
  - `verso announce` exits 0 on an accepted announcement and non-zero, with the reason on stderr, on a refused one;
  - it then runs `ubus call service event` with type `verso.<owner>.<event>` and data `{subject, args}`, through an injected runner;
  - a failing ubus is said on stderr and still exits 0.

  Then add the subcommand. Verify with `make test`.
- [ ] 3.3 Make `/var/run/verso/events` in the shell's init script (`docker/rootfs/etc/init.d/verso`), `verso:verso 0700`, keeping a sound directory across restarts and removing and remaking anything else, as `changes` is made. Verify on the lab container under `make dev`:
  - `ls -ld /var/run/verso/events` reads `drwx------ verso verso` after `/etc/init.d/verso restart`;
  - a file waiting in it survives that restart;
  - a symlink planted at its name is replaced by a directory.
- [ ] 3.4 Test first in the SDK: `root::announce(plugin, event, subject, args)` runs the verso binary with exactly the arguments 3.2 takes, bounded to 10 s, and turns a non-zero exit into an error carrying stderr. The binary's path is injectable, and the test uses a fake executable in a temp dir. Then implement it. Verify with `cargo test` in the SDK.
- [ ] 3.5 Write ADR-028, "Events", from design decisions 1, 2, 7 and 9. Complete `docs/plugins.md`'s "Events" with "Announcing" and "Reacting": `procd_add_raw_trigger` and the service event's data, and that a plugin's page process is told nothing. Verify by checking that `grep -n 'ADR-028' docs/ADR/*.md docs/plugins.md` finds every cross-reference.

## 4. Recording once

- [ ] 4.1 Test first in `internal/history`: `Store.RecordOnce` records an event, records nothing for a second with the same time, kind and spool name, and records one whose spool name differs. Then implement it. Verify with `make test`.
- [ ] 4.2 Test first in `internal/server` (temp spool, injected owner check):
  - files are taken oldest name first;
  - each of these is removed unrecorded with a logged reason: a file not owned by root, a link, one over 8 KiB, an unknown format version, a kind the table does not declare;
  - a store that fails leaves the file for the next pass;
  - a file recorded and then left behind is removed with no second row;
  - the taker runs while an apply's rollback window is open.

  Then implement the taker and start it in `StartHistory` beside `watchSpool`. Verify with `make test`.

## 5. Saying

- [ ] 5.1 Test first in `session_test.go` (injected clock, fake grants):
  - a said event is queued only for sessions whose grant probe answers yes;
  - a queue holds at most 8 events and drops each a minute after it was recorded;
  - taking empties the queue, so a second taker gets nothing;
  - the session's wake channel is signalled;
  - an ended session's queue goes with it;
  - a core kind is never queued.

  Then implement the queue in `Sessions`. Verify with `make test`.
- [ ] 5.2 Test first in `internal/server`: `GET /events/stream`
  - sends the queued events on connect, each as the `verso-flash` fragment in the request's language and its kind's tone;
  - sends one queued while it is open;
  - writes a comment every keep-alive interval (injected);
  - ends when the session ends, without touching its idle clock.

  Then implement `serveSSEOn` and the route. Verify with `make test`.
- [ ] 5.3 Test first in a new `scripts/outcome-stream.test.cjs`:
  - a signed-in page opens `/events/stream` once;
  - each frame reaches `versoOutcome.show`;
  - a notification arriving while another is shown waits until that one leaves.

  Then implement it in `verso-page.js`. Verify with `make test`.
- [ ] 5.4 Amend ADR-006 §4 in place: an outcome that comes after its request reaches the notification as an event (ADR-028). Verify with `grep -n 'ADR-028' docs/ADR/006-plugin-contract.md`.

## 6. Alerts

- [ ] 6.1 Test first in `internal/alerts`:
  - every existing WAN, boot, rollback and storage test passes unchanged;
  - a plugin kind declared as ending another: the opening event is held a minute; ended within it, it is rewritten into one message; ended later, the recovery message names the duration;
  - repeated opening events while the incident is open send nothing more;
  - an alerting kind nothing ends sends one message each time, titled `<host>: <sentence>`;
  - a non-alerting kind and an unchosen plugin kind send nothing.

  Then generalise incidents over the kind table (design decision 8). Verify with `make test`.
- [ ] 6.2 Test first in `alerts_settings_test.go`:
  - each installed plugin declaring an alerting event is a checkbox labelled by its manifest name through its catalog;
  - with `verso.alerts.kind` absent, every box is ticked;
  - saving exactly the full set removes the option;
  - an id of a removed plugin left in the option changes nothing.

  Then implement it. Verify with `make test`.
- [ ] 6.3 Amend ADR-020 in place: a plugin's alerting events are a kind; absent means every kind; incidents come from `ends`. Verify by checking that `grep -n 'all four' docs/ADR/020-alerts.md` finds nothing.

## 7. The certificate's replacement

- [ ] 7.1 Test first in `cmd/verso/main_test.go`:
  - `verso certificate install` announces `certificate.replaced` with the leaf's common name and expiry day after it writes the pair;
  - `verso certificate announce DIR` announces the same for the pair in DIR;
  - `generate` announces nothing;
  - the core sentence fills with both arguments.

  Then implement it. Verify with `make test`.
- [ ] 7.2 Test first in `verso-rpcd` with a fake `Runner`: `setWebCertificate` and `makeWebCertificate` run `hotplug-call verso-certificate` (`ACTION=installed`), then `verso certificate announce /etc/verso`, then the shell's SIGHUP, in that order. A failing announcement is logged and the install still answers success. Then implement it in `access.rs`. Verify with `cargo test` in `verso-rpcd`.
- [ ] 7.3 Amend ADR-017 §6 in place: the replacement is `certificate.replaced`, and the reload stays on SIGHUP. Verify by reading the section against design decision 10.

## 8. verso-plugin-acme

- [ ] 8.1 Test first in acme: its `commands.json` parses with the SDK's `Registry` and declares `issued`, `renewed`, `issue-failed` and `renew-failed` with the sentences, tones, read, alerts and ends of the trusted-certificate spec. Then declare them. Verify with `cargo test` in `plugins/verso-plugin-acme`.
- [ ] 8.2 Test first with the fake `Router`, which gains `announce`:
  - `deliver` announces each outcome for the section's domain, before it installs;
  - `prepare`, `sync`, another section, another domain and a disabled section announce nothing;
  - a failing announcement is logged and the outcome still recorded.

  Then implement it. Verify with `cargo test` in acme.
- [ ] 8.3 Add the four sentences' Slovenian to acme's `i18n/sl.json`, and have the package depend on this verso. Verify with acme's catalog test and `cargo test`.

## 9. Integration

- [ ] 9.1 On the lab container under `make dev`, with Access open in the browser (headless with `reducedMotion: 'reduce'`, screenshots in `~/Share/screenshots`):
  - `verso announce acme.renewed router.example.com` as root says "The certificate for router.example.com was renewed." in the notification, and System → Events lists it;
  - the same command as the `verso` user is refused.

  Verify with the screenshot and the two outputs in the change's notes.
- [ ] 9.2 On the lab container, with the alert address pointed at a local listener: `acme.renew-failed`, then `acme.renewed` a minute and a half later, sends two messages, the second naming the duration. Verify with the received bodies in the notes.
- [ ] 9.3 On the lab container, a fixture init script with `procd_add_raw_trigger "verso.certificate.replaced"` runs its command after a certificate is installed through Access. Verify with its log line in the notes.
- [ ] 9.4 On the DK, from a build cut from a clean worktree: force an acme renewal with nobody signed in, then with an operator on Access. Check that the renewal is listed at its time on System → Events, and that the notification says it only in the second case. Verify with the notes, written with `router.example.com` in place of the real domain.
- [ ] 9.5 Write the change's `release-notes.md`, operator-facing (the release-notes style), and update `docs/BACKLOG.md`'s "Plugin contracts: events" item to what remains, staging only this change's hunks. Run `graphify update .`. Verify that `make lint` passes and `make test` passes but for the five known `dev-grid` tests, named in the notes.

## Workflow follow-up

- Archive the change once 9.4 has run on the DK, with the ADRs and `docs/BACKLOG.md` reflecting what landed.
