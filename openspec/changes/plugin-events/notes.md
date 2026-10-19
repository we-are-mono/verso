# Notes

## 9.1 Announced, said once, refused to the shell's account (lab, 2026-10-19)

On the lab container under `make dev`, verso 0.9.0-r112 and acme from its
repository at c847e73, signed in as root in one headless Chromium
(`reducedMotion: 'reduce'`) with Access open in two tabs:

- `docker exec verso /usr/bin/verso announce acme.renewed router.example.com`
  exited 0 and left one file in the spool, root's, `0644`:
  `01792375776642927922-58762-024426fb.json`. The shell took it within its
  two seconds; the spool was empty after.
- The same command as the shell's account (`docker exec -u 6000`) exited 1:
  `verso: announce: the router's events are announced by root alone`.
- Each event was said in one tab and not the other: `said: [1, 0]` in light
  and dark.
- Only the first tab opened `/events/stream`, one at a time: its landing
  page's stream was aborted as it moved to Access, which opened its own; the
  second tab opened none:

  ```
  first at 1ms on http://localhost:8080/          answered 200
  failed net::ERR_ABORTED at 27ms
  first at 37ms on http://localhost:8080/system/access   answered 200
  ```
- System → Events listed "The certificate for router.example.com was renewed."

Screenshots, in `~/Share/screenshots`:

- `events-one-said-light.png`: the renewal said in the first tab.
- `events-stack-light.png`, `events-stack-dark.png`: an act's outcome
  ("Saved.") in its place, acme's failed renewal standing under it.
- `events-in-drawer.png`: an issued certificate said inside the open Alerts
  drawer, which lists acme ("Trusted certificate") as an alert kind.

## 3.3 The events spool (lab, 2026-10-19)

- `/var/run/verso/events` stood `drwx------ 6000 6000`, made by the helper's
  init.
- A file waiting in it survived `/etc/init.d/verso-rpcd restart` and
  `/etc/init.d/verso restart`.
- A symlink planted at its name (to `/tmp/planted`) was replaced by a sound
  directory at the helper's next start, and nothing was written at the link's
  target.
- acme's catch-up in the lab has no `verso` section to serve, so a fixture at
  acme's place in the boot stood in for it: `/etc/init.d/zz-boot-announce`,
  `START=93`, running `verso announce certificate.replaced
  boot-check.example.com 2027-01-01`. After `docker restart verso`, System →
  Events listed "The web interface’s certificate was replaced, for
  boot-check.example.com until 2027-01-01." once, and the spool was empty.

## 9.2 Alerts, without and across a restart (lab, 2026-10-19)

`verso.alerts.url` pointed at a receiver in the container (socat on
127.0.0.1:9911 answering 200, logging each POST's Title and body), no kinds
chosen. The two halves stood more than five minutes apart, so the second was
no reopening of the first (ADR-020's flap window).

Without a restart (`acme.renew-failed`, then `acme.renewed` 90 s later), two
messages:

```
04:25:38 | verso-lab: The certificate for router.example.com could not be renewed. | Since 04:24.
04:26:18 | verso-lab: The certificate for router.example.com was renewed. | After 1 min, since 04:24.
```

Across a restart (`acme.renew-failed`, its alert sent, `/etc/init.d/verso
restart`, `acme.renew-failed` again, then `acme.renewed`), two again, not
three:

```
04:33:08 | verso-lab: The certificate for router.example.com could not be renewed. | Since 04:32.
04:33:53 | verso-lab: The certificate for router.example.com was renewed. | After 1 min, since 04:32.
```

A first attempt ran while the receiver could not answer (the container's /tmp
is mounted noexec, so socat could not start its script). Its alerts waited in
the outbox and went out on their retries once the receiver answered, as
ADR-020 has them.

## 9.3 A plugin's procd trigger answers the certificate's replacement (lab, 2026-10-19)

A fixture service, `/etc/init.d/zz-events-fixture`, declared
`procd_add_raw_trigger "verso.certificate.replaced" 1000 /usr/bin/logger -t events-fixture "the web certificate was replaced"`.
A self-signed P-256 pair for router.example.com was installed through Access,
"Install a certificate" (the drawer answered "The router is serving the
installed certificate."). The log held the fixture's line once, two seconds
after:

```
Mon Oct 19 04:19:51 2026 user.notice events-fixture: the web certificate was replaced
```

System → Events listed "The web interface’s certificate was replaced, for
router.example.com until 2027-10-19."

## 9.5 Lint and tests (2026-10-19)

`make lint` passes. `make test` passes but for the five known
`scripts/dev-grid.test.cjs` tests (BACKLOG, "`scripts/dev-grid.test.cjs`
fails"), which fail on main as before this change:

- G draws the grid, and G again takes it away
- a G typed into a field stays in the field
- G with a modifier is the browser's, not the grid's
- the choice survives a reload in the same tab
- blocked storage still lets G work

## 9.4 On the DK

A build for the DK is served from this machine (verso 0.9.0-r117,
verso-plugin-acme 0.1.0-r14, built from a clean worktree of 58cb8e7 with each
plugin cloned at its committed head). acme's own hook stands in for a forced
renewal, so no request reaches Let's Encrypt:
`ACTION=renewed main_domain=<the domain> hotplug-call acme`.

The owner installed it on Zamanifi (2026-10-19) and ran the hook with Access
open: the notification said the renewal. The install also showed nlbwmon's
restart burst ("Too many pending MAC address lookups", 1,559 lines over two
seconds, 31,665 connections against a database_limit of 10,000, 232 records
that day), fixed apart from this change in 9562237: an upgrade no longer
restarts nlbwmon unless usage-setup changed a setting.

## 5.4 The stacked layer, for the owner

Built and shown in the screenshots above; waiting on the owner's word before
the task is checked. DESIGN.md states the rule as built.
