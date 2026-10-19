# Tasks

## 1. The registry declares events; core's names are reserved

- [x] 1.1 Test first in `internal/plugin/registry_test.go`:
  - `events` parses into each declaration's name, sentence, tone, read, notify, alert and its list of ends;
  - `notify` absent reads as false;
  - each of these is dropped with a logged reason while the plugin's other declarations stay: a name outside `[a-z0-9-]{1,64}`, an unknown tone, a read the registry does not register, an end naming no event of the plugin's;
  - a registry with no `events` parses as before.

  Then implement it (design decision 3). Verify with `make test`.
- [x] 1.2 Test first in `internal/plugin/manifest_test.go`: a manifest whose id is any of `wan`, `boot`, `changes`, `rollback`, `storage`, `certificate`, `firmware`, `device` or `dhcp` is refused with a logged reason, as `shell` is. Then extend `Manifest.validate` (design decision 4). Verify with `make test`.
- [ ] 1.3 Test first in `plugins/verso-plugin-sdk`:
  - the registry builder's `event(…)` refuses every declaration 1.1 drops;
  - a registry with two events serializes into `testdata/commands.json`, and the shell's conformance test decodes that fixture with the same declarations;
  - `registry::check_catalog` fails for a declared sentence a shipped catalog lacks, or translates with another count of `%s`.

  Then implement them. Verify with `cargo test` in the SDK and `make test`.
- [ ] 1.4 Amend ADR-027 in place: the registry's `events`, and `announce` beside `own_write` in decision 9. Add the "Declaring events" part of a new `docs/plugins.md` section, "Events": the fields, `notify` off unless declared, an event's name as its contract, and the catalog check. Its example is checked by a doc test against the SDK's `Registry`. Verify with `cargo test` in the SDK.

## 2. One kind table

- [ ] 2.1 Test first in a new `internal/events`: the core kinds carry the words, tone, grant, announced and said flags, alert kind and ends of design decision 5's table.
  - A plugin's declared events join the table as `<plugin>.<event>`.
  - A recorded plugin event whose plugin has been removed still gives its English sentence, its arguments filled in, and the unknown-kind visibility.

  Then implement the table, rebuilt from the manifests on every rescan. Verify with `make test`.
- [ ] 2.2 Test first in `history_events_test.go` and `config_history_grants_test.go`:
  - every existing row and grant test passes unchanged against the table;
  - a plugin event's row reads its sentence through the plugin's catalog, its arguments as written, in its tone's colour, with its subject in the Interface column;
  - a sentence with no translation reads in English;
  - the event shows only under `verso-plugin <plugin> <read>`.

  Then move `eventRow`, `eventGrants` and `eventsView.show` onto the table. Add `ubus verso accessCredentials` to `shellProbes`. Verify with `make test`: the root-grants test also holds the new probe.
- [ ] 2.3 Amend ADR-019 in place: the events table gains a plugin's announced events and the certificate's replacement, the VPN row's source becomes the VPN plugin's announced events, and the kinds become one table. Verify by checking that `grep -n 'daemon hooks' docs/ADR/019-history-and-events.md` finds nothing.

## 3. Announcing

- [ ] 3.1 Test first in `internal/events` (injected euid and shell uid, temp spool, registries in a temp plugins dir), in the manner of `cmd/verso/historydir_test.go`:
  - each of these is refused, with a reason, leaving nothing: a non-root caller; an undeclared kind; a core kind other than `certificate.replaced`; arguments that do not fill the sentence; a subject that is not a device name; an argument over 256 bytes or holding any control character; a ninth argument; 256 announcements waiting; a missing spool;
  - each of these is refused, and nothing is written at a link's target: a link at the spool's name, a directory owned by another account, a directory with any group or other bit;
  - an accepted one is written through the checked descriptor, mode `0644`, renamed to `<ns:020>-<pid>-<random>.json`, and two announced in one nanosecond both stand;
  - the reader's parser reads it back field for field.

  Then implement the writer and the reader's parser (design decision 2). Verify with `make test`.
- [ ] 3.2 Test first in `cmd/verso/main_test.go`:
  - `verso announce` exits 0 on an accepted announcement and non-zero, with the reason on stderr, on a refused one;
  - it then runs `ubus call service event` with type `verso.<owner>.<event>` and data `{subject, args}`, through an injected runner;
  - a failing ubus is said on stderr and still exits 0.

  Then add the subcommand. Verify with `make test`.
- [ ] 3.3 Make `/var/run/verso/events` in the helper's init script (`docker/rootfs/etc/init.d/verso-rpcd`), `verso:verso 0700`. Keep a sound directory across restarts; remove anything else at the name and make it again with a plain `mkdir`, as `changes` is made. Verify on the lab container under `make dev`:
  - `ls -ld /var/run/verso/events` reads `drwx------ verso verso`;
  - a file waiting in it survives `/etc/init.d/verso-rpcd restart` and `/etc/init.d/verso restart`;
  - a symlink planted at its name is replaced by a directory;
  - after `docker restart`, acme's catch-up announcement made at START=93 is recorded once the shell starts.
- [ ] 3.4 Test first in the SDK: `root::announce(plugin, event, subject, args)` runs the verso binary with exactly the arguments 3.2 takes, bounded to 10 s, and turns a non-zero exit into an error carrying stderr. The binary's path is injectable, and the test uses a fake executable in a temp dir. Then implement it. Verify with `cargo test` in the SDK.
- [ ] 3.5 Write ADR-028, "Events", from design decisions 1, 2, 4, 7 and 9. Complete `docs/plugins.md`'s "Events" with "Announcing" and "Reacting":
  - `procd_add_raw_trigger`, with the event's data choosing only which trigger fires;
  - a reaction is a command that runs any number of times, reads the router's state itself and logs through `logger`;
  - a plugin's page process is told nothing.

  Verify by checking that `grep -n 'ADR-028' docs/ADR/*.md docs/plugins.md` finds every cross-reference.

## 4. Recording once

- [ ] 4.1 Test first in `internal/history`: `Store.RecordOnce` records an event, records nothing for a second with the same time, kind and spool name, and records one whose spool name differs. Then implement it. Verify with `make test`.
- [ ] 4.2 Test first in `internal/server` (temp spool, injected owner check):
  - files are taken oldest name first;
  - each of these is removed unrecorded with a logged reason: a file not owned by root, a link, one over 8 KiB, an unknown format version, a kind neither the table nor its plugin's registry on disk declares;
  - a kind the table lacks but its plugin's `commands.json` on disk declares is recorded;
  - a store that fails leaves the file for the next pass;
  - a file recorded and then left behind is removed with no second row;
  - the taker runs while an apply's rollback window is open.

  Then implement the taker and start it in `StartHistory` beside `watchSpool`. Verify with `make test`.

## 5. Saying

- [ ] 5.1 Test first in `session_test.go` (injected clock, fake grants):
  - a recorded event declared `notify` is kept only for sessions whose grant probe answers yes;
  - a session keeps at most 8, each for a minute from its recording;
  - reading does not remove them;
  - the session's wake channel is signalled;
  - an ended session's events go with it;
  - a core kind and an event without `notify` are never kept.

  Then implement them in `Sessions`. Verify with `make test`.
- [ ] 5.2 Test first in `internal/server`: `GET /events/stream`
  - sends the session's kept events newer than `Last-Event-ID` on connect, each as the `verso-flash` fragment in the request's language and its kind's tone, with its rowid as the id;
  - sends one kept while it is open;
  - writes a comment every keep-alive interval (injected);
  - ends when the session ends, without touching its idle clock.

  Then implement `serveSSEOn` and the route. Verify with `make test`.
- [ ] 5.3 Test first in a new `scripts/outcome-stream.test.cjs`, with fake Web Locks, BroadcastChannel, `localStorage` and visibility:
  - of three tabs, only the lock holder opens `/events/stream`, and another takes it when that tab closes;
  - an event is shown by one visible tab and recorded as shown;
  - a hidden tab shows it when it becomes visible within its minute, and not after;
  - an id already shown is not shown again after a reconnect;
  - an act's outcome takes its place at once and replaces the previous act's outcome, and no event;
  - an event stands under it, newest nearest it, each leaving on its own timer, a danger or warning one only when closed;
  - a fourth event pushes out the oldest;
  - without Web Locks, a tab opens a stream of its own.

  Then implement it in `verso-page.js`. Verify with `make test`.
- [ ] 5.4 Show the owner the stacked layer on the lab under `make dev`: an act's outcome with two events under it, in light and dark, and inside an open drawer. Revise until it is accepted, and record the rule in DESIGN.md. Then amend ADR-006 §4 in place: an outcome that comes after its request reaches the notification as an event (ADR-028). Verify with the owner's word in the change's notes, and with `grep -n 'ADR-028' docs/ADR/006-plugin-contract.md`.

## 6. Alerts

- [ ] 6.1 Test first in `internal/alerts`:
  - every existing WAN, boot, rollback and storage test passes unchanged;
  - a plugin kind that another's `ends` lists: the opening event is held a minute; closed within it, it is rewritten into one message; closed later, the recovery message names the duration;
  - either of two kinds listing the same failure closes it;
  - repeated opening events while the incident is open send nothing more;
  - an alerting kind nothing ends sends one message each time, titled `<host>: <sentence>`;
  - a non-alerting kind and an unchosen plugin kind send nothing;
  - kinds arrive through `Configure`, with no package-global default left.

  Then generalise incidents over the kind table (design decision 8). Verify with `make test`.
- [ ] 6.2 Test first in `internal/alerts`: `Begin` over a store holding an open WAN outage and an open plugin failure rebuilds both. The next repeat sends nothing, and each close sends its recovery message. A closed incident is not rebuilt. Then implement the rebuild. Verify with `make test`.
- [ ] 6.3 Test first in `alerts_settings_test.go`:
  - each installed plugin declaring an alerting event is a checkbox labelled by its manifest name through its catalog;
  - with `verso.alerts.kind` absent, every box is ticked;
  - saving exactly the full set removes the option;
  - an id of a removed plugin left in the option changes nothing.

  Then implement it. Verify with `make test`.
- [ ] 6.4 Amend ADR-020 in place:
  - a plugin's alerting events are a kind;
  - absent means every kind;
  - incidents come from `ends` and are rebuilt from history at start.

  Verify by checking that `grep -n 'all four' docs/ADR/020-alerts.md` finds nothing.

## 7. The certificate's replacement

- [ ] 7.1 Test first in `cmd/verso/main_test.go`:
  - `verso certificate install` announces `certificate.replaced` with the leaf's common name and expiry day after it writes the pair;
  - `verso certificate announce DIR` announces the same for the pair in DIR;
  - an announcement that fails is said on stderr and the install still exits 0;
  - `generate` announces nothing;
  - the core sentence fills with both arguments.

  Then implement it. Verify with `make test`.
- [ ] 7.2 Test first in `verso-rpcd` with a fake `Runner`: `setWebCertificate` and `makeWebCertificate` write the pair, send the shell's SIGHUP, run `hotplug-call verso-certificate` (`ACTION=installed`) and wait for it, then run `verso certificate announce /etc/verso`, in that order. A failing announcement is logged and the install still answers success. Then implement it in `access.rs`. Verify with `cargo test` in `verso-rpcd`.
- [ ] 7.3 Amend ADR-017 §6 in place: the replacement is `certificate.replaced`, and the reload stays on SIGHUP, for decision 10's reasons. Verify by reading the section against design decision 10.

## 8. verso-plugin-acme

- [ ] 8.1 Test first in acme: its `commands.json` parses with the SDK's `Registry` and declares the four events with the sentences, tones and alerts of the trusted-certificate spec, under `state`, said. `issued` and `renewed` must each end both failures. Then declare them. Verify with `cargo test` in `plugins/verso-plugin-acme`.
- [ ] 8.2 Test first with the fake `Router`, which gains `announce`:
  - `deliver` announces a success after the certificate is installed and the shell signalled, and a failure as it records it, for the section's domain;
  - a success whose install fails announces nothing;
  - `prepare`, `sync`, another section, another domain and a disabled section announce nothing;
  - a failing announcement is logged and the outcome still recorded.

  Then implement it. Verify with `cargo test` in acme.
- [ ] 8.3 Add the four sentences' Slovenian to acme's `i18n/sl.json`, and have the package depend on this verso. Verify with a test calling the SDK's `registry::check_catalog` over acme's registry and catalogs, and with `cargo test`.

## 9. Integration

- [ ] 9.1 On the lab container under `make dev`, with Access open in two tabs of one browser (headless with `reducedMotion: 'reduce'`, screenshots in `~/Share/screenshots`):
  - `verso announce acme.renewed router.example.com` as root says "The certificate for router.example.com was renewed." in the visible tab alone, and System → Events lists it;
  - the browser holds one `/events/stream`;
  - the same command as the `verso` user is refused.

  Verify with the screenshots and the outputs in the change's notes.
- [ ] 9.2 On the lab container, with the alert address pointed at a local listener:
  - `acme.renew-failed`, then `acme.renewed` a minute and a half later, sends two messages, the second naming the duration;
  - with a shell restart between a second `acme.renew-failed` and its `acme.renewed`, two messages are sent again, not three.

  Verify with the received bodies in the notes.
- [ ] 9.3 On the lab container, a fixture init script with `procd_add_raw_trigger "verso.certificate.replaced"` runs its command once after a certificate is installed through Access. Verify with its `logger` line in the notes.
- [ ] 9.4 On the DK, from a build cut from a clean worktree: force an acme renewal with nobody signed in, then with an operator on Access. Check that the renewal is listed at its time on System → Events, and that the notification says it only in the second case. Verify with the notes, written with `router.example.com` in place of the real domain.
- [ ] 9.5 Finish the paperwork, then verify that `make lint` passes and `make test` passes but for the five known `dev-grid` tests, named in the notes:
  - write the change's `release-notes.md`, operator-facing (the release-notes style), saying that an installed plugin's failures alert unless the operator chose kinds;
  - update `docs/BACKLOG.md`'s "Plugin contracts: events" item to what remains, staging only this change's hunks;
  - run `graphify update .`.

## Workflow follow-up

- Archive the change once 9.4 has run on the DK, with the ADRs and `docs/BACKLOG.md` reflecting what landed.
