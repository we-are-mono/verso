# ADR-019 — History and events

- **Status:** Accepted
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

The schema has three tables:

| Table | Columns | Key |
|---|---|---|
| `series` | id, name | id; name unique |
| `points` | series id, resolution, time, average, peak | series id, resolution, time |
| `events` | time, kind, subject, detail (JSON) | time (indexed); repeats are kept |

The tables are the same on every router. A series is a name for what it
measures (`iface.eth0.rx`, `cpu`, `temp.<sensor>`), and a point is one row: a
router with more ports or sensors has more rows, never more columns, and a
device that appears needs no change to the file. Points carry the series' id,
so a long sensor name is stored once rather than on every point. When the
schema changes, the shell rebuilds the file.

### Where it lives

`verso.history.directory` names the directory. Absent, it is `/data/verso` where
`/data` is a mount, else `/usr/share/verso/history`, which is listed in
`keep.d`. No code writes the default back (ADR-013).

The init script resolves the directory on every start, creates it and hands it
to the `verso` user, since a USB stick, a fresh `/data` and the overlay are
root's; the shell receives it as `-history-dir`.

The same directory holds nlbwmon's database, as `<directory>/nlbwmon`
(ADR-018), so all of Verso's history lives where the operator put it.

Changing the directory moves the history: `verso-rpcd` creates the new
directory, the shell copies the database there with `VACUUM INTO`, switches to
the copy and removes the old file, and `verso-rpcd` stops nlbwmon, moves its
directory and starts it again.

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

- every network device the kernel counts while it is not down, loopback
  aside: received and sent bits per second, from the per-second rates the
  overview's graphs draw. A device that is down (an idle tunnel placeholder, a
  port without link) leaves a gap;
- CPU use, memory in use, and every temperature sensor the board reports;
- conntrack entries in use;
- Wi-Fi clients per radio, and DHCP leases per pool;
- the WAN's latency and packet loss (below).

Per-device traffic is not repeated here; it stays nlbwmon's.

Rates count every second the sampler took; the other series are read every
ten seconds. A reading that fails leaves a gap, never a zero.

Minute points collect in memory and are written in one transaction every
fifteen minutes, and when procd stops the shell. A power cut loses at most the
last fifteen minutes. Once an hour the shell rolls finished periods up into the
next resolution, deletes what has expired and runs `incremental_vacuum`, so the
file stays near a fixed size: 12.5 MB for 42 series at full retention, about
300 KB a series.

A gap stays a gap. Nothing is interpolated across a reboot or a stopped shell;
the boot event explains it.

### Events

An event is a fact a daemon or the kernel announced, never a line parsed from
the log:

| Kind | Source |
|---|---|
| WAN down, WAN up | the kernel's main-table default routes, read over route netlink every ten seconds |
| Router booted, cleanly or not | the kernel's boot id and boot time; a clean stop leaves a mark beside the history |
| Changes applied, changes rolled back | the shell's own apply path (ADR-010): an apply confirmed is applied; one whose rollback window passes, plus five seconds, without a confirm was reverted by the router. Either names the pages its changes were made on |
| Firmware installed | the release changing between boots |
| VPN tunnel up, down | the VPN plugin's daemon hooks |
| New device | a MAC address seen for the first time |
| DHCP pool nearly full | lease count against the pool size |
| History storage unavailable, back | the shell's own writes |

The internet is up while a default route stands in the main table, IPv4 or
IPv6. A default route in a policy table carries only the traffic marked for it
(a VPN's table), so it does not keep the internet up for everyone else. Route
netlink answers any user, so the shell reads it without a session.

A boot is known by the kernel's boot id, not its boot time: a router without a
clock boots in the past and its boot time moves once NTP sets the clock. The
shell records a boot when it starts under a boot id it has not recorded. When
procd stops the shell it leaves the file `stopped` in the history directory,
and the next boot that finds it is clean; one that does not followed a power
cut or a crash.

Events are kept for 400 days, like the daily points.

### The WAN probe

The shell sends one ICMP echo to each target every ten seconds, all at once,
waiting two seconds for each answer. An answer is its round trip in
milliseconds (`wan.<target>.latency`); every echo is a loss of 0 or 100
(`wan.<target>.loss`), so a minute's average is its loss in percent and its
peak says whether any echo went unanswered.

The echoes go over the kernel's unprivileged ping sockets. The package grants
the `verso` group (6000) those sockets with
`/etc/sysctl.d/05-verso-ping.conf` (`net.ipv4.ping_group_range = 6000 6000`):
ICMP echo and nothing else, no raw socket, so the shell keeps no privilege it
does not use and no helper loop hands results over. The file sorts early, so a
later one that sets the range for other users wins; where the kernel refuses
the socket, the probe says once in the log that it cannot run, and records a
gap rather than a loss.

The gateway is the next hop of the main table's IPv4 default route
(`/proc/net/route`). A point-to-point uplink (PPPoE) has none, so it records
no gateway series.

`verso.history.probe` lists further targets, as addresses; the init script
hands each to the shell as `-probe`. Absent, only the gateway is probed: an
address on the internet is a target the operator chooses, because reaching
out to it unattended is opt-in (ADR-014).

### Where history is shown

- Every traffic and system chart gains a range: Live, 24 hours, 7 days, 30
  days, 1 year. Live is the chart as it is today.
- Events are marked on the charts they explain, at the time they happened,
  behind the curves: an outage a crimson wash from going down to coming back
  (to now while it lasts), a boot a dashed rule, each titled for the pointer.
- The Internet graph's past windows mark the stretches whose periods lost 5 %
  or more of the gateway's echoes as a marigold wash, titled with the worst
  period; loss inside an outage is the outage's. Beside the traffic's averages
  stands the gateway's average latency over the window.
- System → Events lists the timeline, newest first: when, on which interface,
  and what happened as a status coloured by its weight. It is a page of its
  own beside Logs, whose console fills its page.
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
