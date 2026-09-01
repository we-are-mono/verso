# ADR-014 — Unattended update checks

- **Status:** Accepted
- **Date:** 2026-08-31
- **Deciders:** tomaz@zaman.io
- **Relates to:** ADR-013 (the gate is Verso's first setting), ADR-007 (the
  identity unattended work runs under is the decision here), ADR-010 (the
  toggle stages like any setting; nothing here touches the apply path).

## Context

Verso can now say whether the router is out of date — upgradable packages
from the feeds, a newer system build from the configured attended-sysupgrade
server — and three surfaces read that one truth: the homepage tile, the
"This device" footer row, and the Maintenance Updates section. But the truth
is only as fresh as the last *human-triggered* action: a feeds refresh, the
Check-for-updates button, a finished upgrade. Left alone, the router reports
whatever the last check found, forever; freshly booted, it reports nothing.
The badge that exists to tell an inattentive owner "your router has an
update" can only light for an owner who already went looking.

Scheduling is not the hard part; identity is. The check verbs
(`pkgUpgradable`, `firmwareCheck`) are session-gated at rpcd like every
privileged call (ADR-007), and an unattended run has no operator session to
borrow. Whatever runs the check must also put its result where the surfaces
already look, must be something an owner can turn off, and must not make a
fleet of routers hammer the feeds and the sysupgrade server in one
synchronized wave.

## Decision

**A daily cron job checks for updates: a constant shipped crontab line, a
script that runs as local root through rpcd's zero-session convention,
invoking only the two read verbs, sleeping a random offset first, gated by
one default-on uci option, and writing its result to the runtime state file
every surface reads. Checking is all it does — nothing installs unattended.**

1. **The scheduler is the platform's.** OpenWrt ships crond; the package
   ships one constant line in the root crontab running
   `/usr/libexec/verso/update-check` daily. The line never varies per device
   and is never edited by Verso at runtime — it is package content, not
   state.

2. **The gate is a setting, not the schedule — absent means off, and the
   package opts in explicitly.** The script's first act is consulting
   `verso.updates.autocheck` and exiting quietly unless it reads an explicit
   `1`. This deviates from ADR-013's absent-means-default convention on
   purpose: an unattended path that touches the network is opt-in at the
   system level — a bare config never phones out — and the shipped product's
   opt-in is *visible*, `uci show verso` displaying the behavior instead of
   an implicit default hiding it. The package's post-install seeds the
   option to `1` **only when it is absent** (a genuine first install), and
   the Maintenance toggle always writes explicitly (`1`/`0`), never clears —
   together those two rules mean an owner's off survives every upgrade.
   Turning unattended checks off is therefore one staged toggle — or one
   `uci set` over SSH — and never a crontab edit that a package upgrade
   would fight.

3. **The identity is local root, bounded to reads.** The script calls the
   helper's two read verbs through rpcd's local zero-session convention —
   the platform's own answer for root-owned automation. It invokes nothing
   else: no write verb, no upgrade, no apply. The blast radius of the
   unattended path is "the router found out".

4. **Jitter lives in the script.** A random sleep (up to an hour) precedes
   the check, so identical shipped crontabs across a fleet spread their load
   on the feeds and the sysupgrade server instead of synchronizing on the
   minute.

5. **The result is the shared state file.** The check writes its outcome
   atomically to the runtime state file under `/var/run/verso` (ADR-013's
   state side), and the shell's own button-triggered check converges on the
   same writer — one truth, one format, one freshness stamp, whoever
   produced it. The surfaces read the file; the shell's in-memory cache
   becomes a reader of it. A reboot clears it honestly and the next cron run
   refills it within a day.

6. **Checking only.** Unattended *installation* — packages or firmware — is
   explicitly out. It is the phone-like end state, and A/B devices make it
   genuinely safe someday, but it changes the device without a human present
   and earns its own decision as an opt-in, on top of this one. Nothing in
   this design forecloses it: the same gate section, the same state file,
   the same identity question answered.

## Consequences

- The badge works for the inattentive owner — the audience it exists for —
  within a day of an update appearing, with Verso not even running.
- The update truth survives shell restarts (it is a file, not process
  memory) and states its own age; "checked 3 hours ago" is now the normal
  reading instead of "whenever someone last pressed the button".
- The helper's ACL story gains one nuance to verify and pin in tests: the
  zero-session path must grant exactly the two read verbs and nothing more.
- The package grows the script, the crontab line, and their install steps;
  the dev image carries the same pieces so the path is exercised before it
  ships.
- The shell's `updateChecks` job refactors from cache-owner to
  file-writer/reader; the surfaces' rendering does not change.

## Alternatives considered

- **A ticker inside the shell, with a shell-owned service session.**
  Rejected: it invents service credentials and their storage, dies with the
  process, couples freshness to the shell's uptime, and re-implements a
  scheduler the OS already runs.
- **Editing the crontab as the on/off mechanism.** Rejected: Verso would
  need a privileged crontab-mutation verb, hand edits and the toggle would
  fight over the same line, and a package upgrade restoring its file would
  silently re-enable what an owner disabled. A constant line consulting a
  setting has none of these failure modes.
- **Checking on login or page render.** Rejected: it couples freshness to
  visits, which is precisely the backwardness being fixed — and it puts
  network latency inside renders that are otherwise honest about never
  blocking on the world.
- **Unattended installation now.** Deferred, deliberately: a device that
  changes itself with nobody present needs its own opt-in decision, its own
  failure story, and — for firmware — the A/B rollback narrative told end
  to end. Checking first makes that later decision informed instead of
  bundled.

## Implementation notes

- Script shape: gate check (`= "1"`, anything else exits), jitter sleep, the
  two ubus calls with the zero sid, one atomic rename into
  `/var/run/verso`. Small enough to read at a glance; owned by the package,
  not generated.
- The seeding lives in the package's post-install beside the existing user
  and service steps, guarded on the option's absence — never shipped as
  content inside `/etc/config/verso` itself, which a raw package upgrade
  would re-extract over an owner's edits.
- The state file's format is the one the shell already serializes for its
  cache today, versioned by nothing — pre-alpha, both writers ship
  together.
- crond on OpenWrt starts when a crontab exists; the package's install
  ensures the line's presence idempotently and never duplicates it.
- The Maintenance toggle's row copy stays in the person's language
  ("Check for updates automatically"), with the uci name as its code chip,
  per the settings-row vocabulary.
