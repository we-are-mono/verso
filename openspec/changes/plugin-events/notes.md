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

## 5.4 The stacked layer, for the owner

Built and shown in the screenshots above; waiting on the owner's word before
the task is checked. DESIGN.md states the rule as built.
