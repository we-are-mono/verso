# ADR-020 — Alerts

- **Status:** Accepted
- **Date:** 2026-10-08
- **Relates to:** ADR-013 (Verso's settings), ADR-014 (unattended network
  access is opt-in), ADR-019 (history and events)

## Context

ADR-019 records what happened. An administrator still learns of it only by
opening Verso. The events worth knowing about at once — the internet went down,
the router rebooted on its own, a change was rolled back — are the ones that
happen while nobody is looking.

A router cannot report its own outage over the line that is down. Whatever it
sends during a WAN outage reaches only the LAN, and a router that has lost
power sends nothing at all. Silence has to be noticed from outside.

## Decision

### Two settings, both opt-in

| Option | Meaning |
|---|---|
| `verso.alerts.url` | Where events are sent. Absent, nothing is sent. |
| `verso.alerts.heartbeat` | Where the router reports that it is alive. Absent, nothing is sent. |

Both reach out to an address the operator chose, so both are off until one is
set (ADR-014). The shell sends both: it runs all the time, and neither needs a
privilege the `verso` user lacks.

### What is sent

The shell sends one HTTP POST per message to `verso.alerts.url`: a short
plain-text body, with ntfy's `Title` and `Priority` headers. ntfy shows them as
a push notification; any other receiver (Home Assistant, a chat bridge, a
script) reads the body. The title names the router by its hostname:

| Title | Body |
|---|---|
| `home-router: the internet is down` | `Since 03:12.` |
| `home-router: the internet was down` | `From 03:12 to 03:20 (8 min).` |
| `home-router: the internet is back` | `After 8 min, down since 03:12.` |
| `home-router: the router restarted` | `After losing power or crashing, at 13:49.` |

No link rides along: the shell does not know the address its operator reaches
it by from outside.

### Which events alert

`verso.alerts.kind` lists the kinds that alert. Absent, both do:

| Kind | Alerts on |
|---|---|
| `wan` | the internet going down and coming back (ADR-019's WAN events) |
| `boot` | a start after losing power or crashing; a clean reboot is not news |

### Incidents, not events

- **One message per incident.** A drop within five minutes of the last
  recovery reopens the incident: a WAN that flaps ten times sends one alert,
  and a recovery not yet sent is withdrawn.
- **A drop is held a minute.** Its first try waits a minute, and a drop that
  ends sooner, or before it could be delivered, is rewritten into one message
  that says when it started and ended. Without the hold, a retry that lands
  between the line returning and the shell noticing sends both halves.
- **Recovery closes it.** An incident whose drop was already delivered sends a
  second message naming its duration.
- **Sent when it can be.** An alert that cannot be delivered waits in the
  history's outbox (`alerts` in `history.db`) and is retried, thirty seconds
  doubling to ten minutes apart, for 24 hours, then dropped. A receiver on the
  LAN gets it while the internet is down.
- **Every attempt is recorded.** Each alert keeps its last attempt's time and
  answer (an HTTP status, or why there was none), for thirty days.

### Reaching the running shell

The init script hands `verso.alerts` to the shell as `-alert-url`,
`-heartbeat-url` and `-alert-kind`, which carry it to every start. An apply
reaches the running shell through the confirm that keeps it, under the
confirming operator's session.

### The heartbeat

Every five minutes the shell sends a GET to `verso.alerts.heartbeat`. Services
built for this (Healthchecks.io, Uptime Kuma's push monitor, Better Stack)
alert the operator when the requests stop, which covers a down WAN, a lost
power supply and a hung router alike. The grace period is set at the service,
not in Verso.

### Where it is set up

System → Events (ADR-019) carries an Alerts group: the
alert URL with a "Send test" act, the heartbeat URL, and the event kinds as
checkboxes (a set the operator chooses members of). Each URL's description
states its last delivery and whether it succeeded.

## Consequences

- With either URL set, the router contacts an address on the operator's say,
  unattended.
- During a WAN outage, alerts reach the operator late or through the heartbeat
  service; this is stated beside the URL fields.
- Email is not a channel: it needs SMTP credentials on the router and a mail
  server that accepts mail from a home address. A webhook covers push, chat and
  home-automation receivers with one field and no credentials.
