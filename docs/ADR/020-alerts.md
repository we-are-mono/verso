# ADR-020 — Alerts

- **Status:** Proposed
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

The shell sends one HTTP POST per incident to `verso.alerts.url`: a one-line
plain-text body, with `Title`, `Priority`, `Tags` and `Click` headers. ntfy reads
those headers and shows a push notification; any other receiver (Home
Assistant, a chat bridge, a script) reads the body. `Click` opens Verso on the
event's moment in the timeline.

An example body: `home-router: the internet was down 03:12–03:20 (8 min); the
gateway answered, 1.1.1.1 did not.`

### Which events alert

`verso.alerts.kind` lists the event kinds that alert. Absent, the list is:

- WAN down and up
- a boot that was not clean
- changes rolled back
- history storage unavailable

The other kinds of ADR-019 (new device, VPN tunnel up or down, DHCP pool nearly
full, firmware installed) and three sustained conditions can be added:

| Condition | Holds for |
|---|---|
| WAN packet loss over 5 % | 5 minutes |
| CPU over 90 % | 10 minutes |
| A temperature over the board's warning point | 5 minutes |

The thresholds are fixed. A condition counts once it has held for its whole
window, and ends when it has stayed clear for as long.

### Incidents, not events

- **One message per incident.** Events of one kind on one subject within five
  minutes of each other are one incident: a WAN that flaps ten times sends one
  alert.
- **Recovery closes it.** An incident that ends sends a second message naming
  its duration.
- **Sent when it can be.** An alert that cannot be delivered waits in
  `history.db` and is retried with backoff for 24 hours, then dropped. It keeps
  its times, so a WAN outage arrives once the line returns, saying when it
  started and ended. A receiver on the LAN gets it at once.
- **Every attempt is recorded.** Each attempt's time, status and HTTP code is
  kept beside the event, so "why did I not get it" has an answer.

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
