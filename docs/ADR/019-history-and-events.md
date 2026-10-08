# ADR-019 — History and events

- **Status:** Proposed
- **Date:** 2026-10-08
- **Relates to:** ADR-007 (privileged reads), ADR-013 (Verso's settings),
  ADR-014 (unattended network access is opt-in), ADR-018 (per-device usage),
  ADR-020 (alerts)

## Context

Verso shows the router as it is now. The overview's Internet graph, an
interface's traffic, the CPU, the memory and the temperature are read live and
kept for a minute at most, in memory. An administrator who comes back to "the
internet was bad at three last night" finds nothing: no curve, no record that
the WAN went down, no sign the router rebooted. Per-device totals are the one
history Verso keeps, through nlbwmon (ADR-018).

A history has to survive reboots and firmware upgrades without wearing flash,
must not grow without bound, and must stay readable by the people who own the
router. Events — the WAN went down, the router booted, a change was rolled
back — belong with the numbers, so a spike on a graph and the outage that
caused it read together.

Interface counters stay true under hardware offload: CDX folds the offload
engine's statistics into them, so WAN and interface history is exact where
conntrack-based per-device usage is not.

## Decision

### The shell owns one database

History is part of the shell, in `internal/history`. The shell is the only
writer and the only reader; plugins do not open the file. Every surface that
draws history is the shell's.

The store is SQLite, through `modernc.org/sqlite`, so the shell stays one
static binary built with `CGO_ENABLED=0`. It adds about 4 MB to the arm64
binary. The file is `history.db`, in WAL mode with `synchronous=NORMAL`: a
power cut never damages it.

The schema has two tables:

| Table | Columns | Key |
|---|---|---|
| `points` | series, resolution, time, average, peak | series, resolution, time |
| `events` | time, kind, subject, detail (JSON) | time, kind, subject |

Series are named by what they measure (`iface.wan.rx`, `cpu`, `temp.cpu`), not
by row number, so a reader needs no lookup table. When the schema changes, the
shell rebuilds the file.

### Where it lives

`verso.history.directory` names the directory. Absent, it is `/data/verso` where
`/data` is a mount, else `/usr/share/verso`, which is listed in `keep.d`. The
shell resolves it on start and when the setting is applied; no code writes the
default back (ADR-013).

The same directory holds nlbwmon's database, as `<directory>/nlbwmon`
(ADR-018), so all of Verso's history lives where the operator put it.

`verso-rpcd` creates the directory and hands it to the `verso` user, since a
USB stick or a fresh `/data` is root's. Changing the directory moves the
history: the shell copies the database with `VACUUM INTO`, switches to the copy
and removes the old file; `verso-rpcd` stops nlbwmon, moves its directory and
starts it again.

A directory that cannot be written is warned about, never refused. The shell
keeps collecting in memory and says through the top-right notification that
history is not being saved, naming the cause. A directory on tmpfs is allowed,
with a warning that history ends at every reboot. A directory that disappears
while running (a pulled USB stick) falls back to memory the same way and is
recorded as an event once it returns.

### What is kept, and for how long

| Resolution | Kept for |
|---|---|
| 1 minute | 48 hours |
| 15 minutes | 31 days |
| 1 hour | 90 days |
| 1 day | 400 days |

Each point holds the period's average and its peak, so a short burst survives
the rollup into longer periods.

The series:

- each interface the Interfaces page lists, and each physical port: received
  and sent bytes per second;
- CPU use, memory in use, and every temperature sensor the board reports;
- conntrack entries in use;
- Wi-Fi clients per radio, and DHCP leases per pool;
- the WAN's latency and packet loss (below).

Per-device traffic is not repeated here; it stays nlbwmon's.

Minute points collect in memory and are written in one transaction every
fifteen minutes, and when procd stops the shell. A power cut loses at most the
last fifteen minutes. Once an hour the shell rolls finished periods up into the
next resolution, deletes what has expired and runs `incremental_vacuum`, so the
file stays near a fixed size: a few megabytes on a typical router.

A gap stays a gap. Nothing is interpolated across a reboot or a stopped shell;
the boot event explains it.

### Events

An event is a fact a daemon or the kernel announced, never a line parsed from
the log:

| Kind | Source |
|---|---|
| WAN down, WAN up | `network.interface` ubus events |
| Router booted, cleanly or not | boot time; a clean stop records itself |
| Changes applied, changes rolled back | the shell's own apply path (ADR-010) |
| Firmware installed | the release changing between boots |
| VPN tunnel up, down | the VPN plugin's daemon hooks |
| New device | a MAC address seen for the first time |
| DHCP pool nearly full | lease count against the pool size |
| History storage unavailable, back | the shell's own writes |

Events are kept for 400 days, like the daily points.

### The WAN probe

`verso-rpcd` sends an ICMP echo to the WAN's gateway every ten seconds and
keeps the minute's round-trip times and losses for the shell to collect.
Sending ICMP needs privilege, so the probe is the helper's.

`verso.history.probe` lists further targets. Absent, only the gateway is
probed: an address on the internet is a target the operator chooses, because
reaching out to it unattended is opt-in (ADR-014). A WAN with no gateway reads
as total loss.

### Where history is shown

- Every traffic and system chart gains a range: Live, 24 hours, 7 days, 30
  days, 1 year. Live is the chart as it is today.
- Events are marked on the charts they explain, at the time they happened.
- System → Logs gains an Events section: the timeline, newest first, filtered
  like every listing (still-lens).
- System → Maintenance gains the "History is kept in" row, its description
  stating the file's size and the free space beside it.

## Consequences

- The shell writes to persistent storage every fifteen minutes.
- The shell binary grows by about 4 MB, and its dependency tree gains the
  modernc packages.
- The chart widget gains a range and event marks, and the shell gains the
  Events section; both are new behaviour for existing widgets.
- What `history.db` stores is aggregates and announced events. No connection,
  peer address or destination is recorded.
