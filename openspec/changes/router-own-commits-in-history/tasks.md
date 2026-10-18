# Tasks

## 1. The mark carries its cause

- [x] 1.1 Test first in `plugins/verso-plugin-sdk/src/root.rs`: `own_write_in(dir, "acme", "acme", "A certificate was installed by hand.")` writes `{"plugin":"acme","cause":"A certificate was installed by hand."}`, 0600, in a 0700 directory. A config that is no uci name is refused, and so is a cause over 512 bytes or holding a newline. Then change `own_write` to `own_write(plugin, config, cause)`. Verify with `cargo test` in the SDK.
- [x] 1.2 Test first in `verso-rpcd/src/config_watch.rs`:
  - `marked` returns the mark's plugin and cause from a JSON mark;
  - an empty mark, or one that does not parse, returns own with no cause;
  - a mark over 4 KiB is read no further and counts with no cause;
  - the existing refusals hold: a stale mark, one not root's, a directory others may write, a name that is no config.

  Then make `marked` and `owned` return the cause. Verify with `cargo test`.
- [x] 1.3 Test first in `config_watch.rs`: `own_write("verso", cause)` is taken by the next settled reading with its cause, once. A spooled record carries `"own"`, `"cause"` and `"plugin"` (empty for the helper's own). Then implement it and update `spool_config`. Verify with `cargo test`.
- [x] 1.4 Test first in `access.rs` and `arrival.rs`: `set_theme` and `set_web_listeners` mark `verso` with their causes, and an arrival marks each config it commits with the arrival cause (design decision 6). Then pass the causes. Verify with `cargo test`.

## 2. history.db keeps the cause

- [x] 2.1 Test first in `internal/history/migrate_test.go`: a file written at version 9 (`testdata`) steps to 10, keeps every change, and reads back with empty `Cause` and `CausePlugin`. A build at 10 refuses a file at 11. Then add `migrations/010_change_cause.sql`. Verify with `go test ./internal/history`.
- [x] 2.2 Test first in `internal/history/changes_test.go`: `RecordChange` stores `Cause` and `CausePlugin`, and the change list reads them back. Then add the fields to `history.Change` and its queries. Verify with `go test ./internal/history`.
- [x] 2.3 Test first: `RecordChange` stores `Own`, the change list reads it back, and a change recorded before step 11 reads as not the router's own. Then add `migrations/011_change_own.sql` and the field (design decision 4). Verify with `go test ./internal/history`.

## 3. The shell records the router's own commits

- [x] 3.1 Test first in `internal/server/config_history_direct_test.go`:
  - an `own` reading of `acme` that turns `acme.verso.enabled` from 1 to 0 records one applied change, with no user, `Cause` "A certificate was installed by hand.", `CausePlugin` "acme", and items in acme's words;
  - an `own` reading with no difference records nothing;
  - an `own` reading inside an apply's window records nothing.

  Then record own readings in `takeReading` (design decision 2). Verify with `go test ./internal/server`.
- [x] 3.2 Test first: an `own` reading of a secret option's change records the option as changed with neither value. Verify with the existing secrets tests and `go test ./internal/server`.
- [x] 3.3 Test first: an `own` mark from an earlier plugin (no cause) records a change with no user and empty cause. Verify with `go test ./internal/server`.

## 4. Acts started for an operator are theirs

- [x] 4.1 Test first in `verso-rpcd`:
  - `setTheme`, `setWebListeners` and `pkgInstall` mark their commits with the handle of the caller's session (the hex SHA-256 of its id), and the spooled record carries it as `by`;
  - a root half's mark that holds a `by` has it ignored.

  Then pass the session id from dispatch and mark with its handle (design decision 3). Verify with `cargo test`.
- [x] 4.2 Test first in `internal/server`:
  - an `own` reading whose `by` is the handle of a session signed in as root from 192.0.2.10 is a change by root from 192.0.2.10, with its cause;
  - a handle no session matches, or none at all, is the router's;
  - a reading that is not `own` is still edited directly.

  Then match the handle against the shell's sessions in `takeReading`. Verify with `go test ./internal/server`.

## 5. The page

- [ ] 5.1 Test first in `config_history_page_test.go`:
  - a change with a cause and no user reads "The router" in By;
  - Notes read the relation, then the cause, then the note;
  - a Slovenian viewer sees acme's cause from acme's catalog, and the shell's cause from the shell's;
  - a cause no catalog holds reads as written.

  Then extend `changeBy` and the Notes cell (design decisions 4 and 5). Verify with `go test ./internal/server`.
- [ ] 5.2 Add the shell's three causes and "The router" to the shell's Slovenian catalog. Verify with `make i18n-audit`.

## 6. verso-plugin-acme

- [x] 6.1 Test first in the acme repo: the hand-over marks `acme` with plugin `acme` and the cause "A certificate was installed by hand.", and the cause has its Slovenian in `i18n/sl.json`. Then call `own_write("acme", "acme", …)`. Verify with `cargo test` and `make lint` in the acme repo, and commit there.

## 7. Documents

- [ ] 7.1 Amend ADR-021 in place: the router's own commits are recorded as its acts, with their causes, and acts the shell starts for an operator are that operator's. Amend ADR-027 where it names the mark: the mark now carries its cause and plugin. Rewrite the `root::own_write` row in `docs/plugins.md`, with an example. Verify the doc example compiles as an SDK doc test (`cargo test --doc`).
- [ ] 7.2 Update `docs/BACKLOG.md` if anything is left open, and draft the release notes: history now shows the router's own changes, and verso and verso-plugin-acme are released together. Verify that `make lint` and `make test` pass.

## 8. Checks on a running router

- [ ] 8.1 On `make dev`, with Playwright:
  - choose a theme, and the row names this browser's operator with its cause;
  - install a certificate by hand with `acme.verso` on, and the row reads "The router" with "A certificate was installed by hand.";
  - install https-dns-proxy from Packages, and its row says it arrived switched off.

  Save the screenshots to `~/Share/screenshots`, and record what was seen in the change's notes.
- [ ] 8.2 On the DUT, with local apks: upgrade verso and verso-plugin-acme, check that history.db steps to 10 with every earlier row kept, and repeat the hand install. Record it in the notes.
