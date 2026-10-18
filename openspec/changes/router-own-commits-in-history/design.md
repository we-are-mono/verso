# Design

## Context

Config history (ADR-021) takes readings of `/etc/config` from the helper's spool every two seconds (`internal/server/config_history_direct.go`). A reading is recorded in one of two ways:
- **An apply** is recorded by the shell itself (`recordApply`), with the operator's user and address. Readings within its window are absorbed (`expectedWrites`).
- **Any other change** becomes a direct change, with an empty user, which the page shows as "Edited directly".

A reading the helper marks `own` is a third case. `takeReading` moves the baseline and records nothing. The marks come from:
- `config_watch::own_write` in the helper. It is called by `access.rs` (`set_web_listeners`, `set_theme`, both config `verso`) and by `arrival.rs` (each `uci commit` of an arriving package, such as `https-dns-proxy` or `adblock`).
- `verso_plugin::root::own_write`. It leaves an empty file in `/tmp/run/verso-own/<config>`, which `config_watch::marked` takes. Its one caller is verso-plugin-acme's hand-over (`certificate.rs`).

The shell calls the helper's committing acts from three places, each with the operator's session:
- `theme.go` (`SetTheme`);
- `web_settings.go` (`SetWebListeners`);
- `packages.go` (`PkgInstall`, through the act table).

`history.Change` has `User`, `Address`, `Note`, `Relation` and `Items`. history.db is at migration 009.

## Goals / Non-Goals

**Goals:**
- Every commit to a declared config is a row, and the row says who or what made it and why.
- The cause is written once, where the commit is made, and travels with the reading.
- A plugin built against the earlier SDK keeps working with no conflict floor.

**Non-Goals:**
- Configs no plugin declares stay unwatched. An example is `uhttpd`, which `web-owner` commits.
- Applies, their windows, and edits made outside Verso are recorded as today.
- Events (the plugin-contract theme after this one) are not built here. The cause is a sentence on a history row, not an event a plugin subscribes to.

## Decisions

### 1. The mark carries the cause and whose words it is

- **The helper.** `config_watch::own_write(config, cause)` keeps `(Instant, cause)` per config. The cause is a `&'static str`, an English sentence from a closed set in the helper.
- **A root half.** `verso_plugin::root::own_write(plugin, config, cause)` writes the mark file as `{"plugin": "<id>", "cause": "<sentence>"}`, 0600, in the 0700 root directory, as today.
- **Taking the mark.** `marked` reads at most 4 KiB of it. A file that is empty, or does not parse, still counts as own, with no cause. That keeps acme 0.1.0-r11 working.
- **The spool.** The record gains `"cause"` and `"plugin"` beside `"own"`.

*Alternatives:*
- **Take the plugin from the config's owner.** Wrong for `arrival.rs`: an arriving package writes `https-dns-proxy`, which dnsdhcp owns, but the cause is the shell's sentence.
- **A cause code mapped to text in the shell.** It would put every plugin's causes in core, the coupling ADR-027 removes.
- **Trusting a plugin id written by a root half.** Acceptable: the mark directory is root's alone, and the id only chooses which catalog translates a sentence.

### 2. The shell records an own reading as a change

In `takeReading`, a reading that is `own`, and outside an apply's window, is diffed and recorded like a direct change. It differs in two ways:
- `By` is the router, or the operator who started the act (decision 3);
- `Cause` and `CausePlugin` are set.

Like a direct change, it is `OldRead`: what each option held before comes from the last baseline.

A reading inside an apply's window is still absorbed, as today. A diff with no changes records nothing.

*Alternative: keep absorbing, and add a separate "router acts" log.* Rejected: one history, in one order, is what explains the current state, and undo works over it.

### 3. Acts the shell starts for an operator are that operator's

The helper's verbs that commit for an operator (`setTheme`, `setWebListeners`, `pkgInstall`) are called with that operator's rpcd session id. The helper's mark carries a **handle** of it: the hex SHA-256 of the session id. The spooled reading carries the handle as `by`. The shell holds every signed-in session with its rpcd session id, username and address, so it finds the session whose id hashes to the handle and records the row as that operator's, from that address. The mark's cause stays in Notes.

| Reading | Recorded as |
|---|---|
| `own`, with a handle of a signed-in session | that operator's row, with the cause in Notes |
| `own`, with no handle, or one no session matches | the router's row |
| not `own` | unchanged: an apply's, or "Edited directly" |

- **Why a handle, not the id.** The spool is the shell account's, and plugins share that account. A session id there could be read and used. A hash names the session to the shell, which holds the id, and opens nothing.
- **Root halves give no handle.** A root half's mark file holds only `plugin` and `cause`. A `by` it wrote is ignored, so a root half cannot sign a commit as an operator.
- **Alternative: an authored window**, registered by the shell around the act, and matched by config and time. Rejected:
  - a package install runs up to 90 s, and any root-half commit in that window (an ACME renewal) would be filed under the operator;
  - the shell can take the reading before it registers the window.

  The handle names the exact act and needs no window.

### 4. The cause is stored in English and translated when shown

Migration `010_change_cause.sql` adds `cause TEXT NOT NULL DEFAULT ''` and `cause_plugin TEXT NOT NULL DEFAULT ''` to `changes`. Migration `011_change_own.sql` adds `own INTEGER NOT NULL DEFAULT 0`: an older plugin's mark carries no cause, so a change with no operator and no cause could be the router's or one made outside Verso, and only the flag tells them apart. It is a step of its own because a dev shell had already written shape 10, and every shape a shell has written is a step (`migrate.go`). The page translates the cause with the shell's translator when `cause_plugin` is empty, or with that plugin's travelling catalog otherwise (ADR-012). A sentence no catalog holds shows as written.

Notes read in this order: the relation (Undoes…), then the cause, then the operator's own note.

*Alternative: store the translated text.* Rejected: the history would be in the language of whoever was signed in when the router committed.

### 5. By for the router

`changeBy` gains a third case: a change that is `Own` and has no user reads `tr("The router")`, in words as "Edited directly" is. Like "Edited directly", it names the channel and never "Verso" (copy rule). The change's drawer holds the cause before the note, as it holds what the row's one line may cut short.

### 6. The causes the helper gives

| Act | Config | Cause |
|---|---|---|
| `set_theme` | `verso` | "The theme was changed." |
| `set_web_listeners` | `verso` | "The web interface's ports were changed." |
| An arrival | the package's config | "The package arrived switched off, so it changes nothing until it is turned on." |

verso-plugin-acme's hand-over gives "A certificate was installed by hand.", with its Slovenian in acme's `i18n/sl.json`. The shell's sentences go in its catalog, and `make i18n-audit` holds them.

## Risks / Trade-offs

- **A mark taken by the wrong write.** A mark names the next settled write of its config within 30 s, as today. A hand edit landing in that span is folded into the router's row. → Unchanged from today, where it was folded into nothing. The window stays 30 s, and the mark is consumed once.
- **The operator signs out before the reading is taken.** → No session matches the handle, and the row is the router's, with the cause still saying what happened. The reading is taken within seconds, so this is rare.
- **An older acme marks with an empty file.** → It counts as own with no cause: a router's row with empty Notes, rather than a lost row.
- **Rows the router left out before this change cannot be restored.** → Recording starts with this build. The first own reading after the upgrade is diffed against the last baseline, so a gap since then still shows as one row.

## Migration Plan

- history.db steps to 011 at the shell's first start. A build before 010 refuses a file at 010 or later and keeps its history in memory (`shape` in `migrate.go`), so a downgrade leaves the file for the next upgrade to read again.
- Ship verso and verso-plugin-acme together. Acme's new `own_write` needs the SDK of this verso tag; the old acme keeps working on the new verso.
- To roll back, reinstall the earlier verso. Rows recorded meanwhile keep their causes in columns the old shell ignores.
