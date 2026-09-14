# ADR-013 — Verso's own settings live in uci

- **Status:** Accepted
- **Date:** 2026-08-31
- **Deciders:** tomaz@zaman.io
- **Relates to:** ADR-007 (every write rides the sid-gated rpcd path), ADR-010
  (a settings change stages and applies through the review drawer like any other),
  ADR-006 (plugins own their declared configs — this one is the shell's),
  ADR-014 (the first setting: the unattended update check's gate).

## Context

Verso has configured everything except itself. The shell renders and stages
other owners' uci configs — network, firewall, dhcp, system — but holds no
setting of its own: no file, no section, no option answers "how should Verso
behave on this device". The first such setting is arriving (whether the router
checks for updates unattended, ADR-014), and where it lives decides four
stories at once: how a Verso setting is reviewed and applied, how it survives
a firmware upgrade, how a script reads it with Verso not running, and how a
person turns it off over SSH at two in the morning.

The temptation is a small JSON file under `/etc/verso/`. Everything Verso has
built this month argues against it: a parallel store needs its own privileged
writer (the shell runs unprivileged and writes nothing itself, ADR-007), its
own review presentation (the review drawer renders `uci changes`, not file diffs),
its own entry in sysupgrade's keep list, its own backup handling, and its own
conventions for hand edits. That is a second settings system, built for the
product whose purpose is making the first one humane.

## Decision

**Verso's settings are one uci config: `/etc/config/verso`. Operator intent
goes there and nowhere else; runtime state stays in `/var/run/verso`; a third
category does not exist until something real needs it.**

1. **One config, typed sections per concern.** Options group into named typed
   sections as concerns genuinely arise (`config updates 'updates'` first);
   no catch-all `main` section accumulating unrelated flags. Section and
   option names are plain words, documented where the option is consumed.

2. **Absent means default.** Every option's default is what an absent option
   means, so a fresh install ships an *empty* `/etc/config/verso` (the file
   must exist for rpcd to write sections into) and the config only gains
   content when an operator deliberately deviates. The file stays a diff
   against sane behavior, never a dump of it. A consumer therefore reads with
   an explicit fallback (`uci get verso.updates.autocheck` absent → on), and
   no code ever writes a default back.

3. **The shell owns it; writes ride the standard path.** Verso pages change
   these options through the same sid-gated rpcd `uci` calls that broker every
   plugin write (ADR-007), staged and applied from the review drawer (ADR-010) — a
   Verso setting is reviewed, discarded, and committed exactly like a firewall
   rule. The shell's rpcd ACL policy grants `uci verso` read and write.
   Plugins do not touch this config: a plugin wanting product-level behavior
   asks for a contract, not the file.

4. **Settings versus state.** The boundary this ADR fixes:
   - **Settings** — operator intent that must survive reboot and sysupgrade —
     live in `/etc/config/verso`. `/etc/config/*` rides sysupgrade's standard
     backup set and Verso's own backup/restore untouched, on single-slot and
     A/B devices alike.
   - **Runtime state** — truths with a shelf life: the update-check result, a
     pending upload token — lives in `/var/run/verso` (tmpfs), ephemeral by
     design and refilled by whatever produced it.
   - Durable machine state that is not operator intent (an install history, a
     dismissed notice) has no home because nothing needs one; the first real
     need gets its own decision rather than a drawer that attracts clutter.

## Consequences

- The review drawer, `uci changes`, hand edits over SSH, `uci export verso`,
  sysupgrade config-keep, and Verso's backup flow all cover Verso's settings
  from day one, with no new machinery.
- A root script reads a Verso setting with one `uci get`, whether or not any
  Verso process is running — which is exactly what ADR-014's cron gate needs.
- The package ships the empty `/etc/config/verso`; the shell's rpcd ACL file
  gains the `verso` config in its `uci` read and write lists.
- The config is enumerable by the stage's declared-configs union the moment
  the shell declares it, so a staged Verso setting counts and reviews like
  everything else.
- Homelabbers meet the store they expect: `uci show verso` answers what Verso
  is configured to do.

## Alternatives considered

- **A JSON (or TOML) file under `/etc/verso/`.** Rejected above: a parallel
  settings system needing its own privileged writer, review presentation,
  upgrade-survival arrangements, and hand-edit conventions — all of which uci
  already provides, integrated with the UI Verso has already built.
- **SQLite or another embedded database.** Wrong device class and wrong data
  class: a handful of scalar intents, on flash that appreciates fewer writes,
  needing hand-editability. Nothing relational exists here.
- **Per-feature dotfiles (`/etc/verso/autocheck`, …).** Fragments the store,
  multiplies keep-list entries, and drifts into ad-hoc formats; uci sections
  already give per-concern grouping inside one file.
- **Environment / procd arguments.** Not operator-changeable at runtime and
  invisible to review; configuration frozen at service start is a deployment
  knob, not a setting.

## Implementation notes

- First occupant: `verso.updates.autocheck` (ADR-014) — and immediately the
  documented exception to absent-means-default: ADR-014 makes its absence
  mean off and seeds the shipped `1` from the package's post-install, so an
  unattended network-touching behavior is present and inspectable in the
  config rather than implied by it.
- The dev image and the apk payload both carry the empty config file with
  root ownership and the same mode as its `/etc/config` siblings.
- The shell reads its own settings through the same brokered snapshot
  machinery it uses everywhere, never by opening the file — ADR-007's posture
  does not soften for the shell's own config.
