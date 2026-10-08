# ADR-018 — Per-device usage

- **Status:** Accepted
- **Date:** 2026-10-08
- **Relates to:** ADR-007 (privileged reads), ADR-011 (services and packages)

## Context

The question a router is opened for most often is who is using the connection:
now, while it feels slow, and over the month. Stock OpenWrt answers it only with
an optional add-on page (`luci-app-nlbwmon`) of pie charts and address tables,
apart from the place where a device is managed. Verso's Devices roster already
names every device, so usage belongs on it.

Usage needs two readings with different costs. A live rate needs counters read
every second or two, but only while someone looks. A history needs counting
around the clock, catching each connection's final bytes as it closes, and a
store that survives reboots and firmware upgrades without wearing flash.

Conntrack accounting carries both, offloaded flows included: firewall4's
flowtable declares `counter`, so the software fast path adds its bytes to the
connection's accounting, and a hardware flowtable driver reports its counters
through `FLOW_CLS_STATS` into the same place. OpenWrt enables
`net.netfilter.nf_conntrack_acct` by default (`sysctl-nf-conntrack.conf`).

## Decision

### The shell owns the surfaces

Usage is part of the shell, not a plugin. Every place it appears is the shell's:
the Devices roster, the device panel and the overview. It configures no UCI
domain of its own. A plugin contributes acts on a device ("Limit this device")
through its existing entity slot; usage adds none.

### History is nlbwmon's

The `verso` package depends on `nlbwmon`. It collects from conntrack, holds the
current period in memory, commits once a day, and keeps compressed archives.
Verso configures it so that each period is one day:

| Option | Value |
|---|---|
| `database_interval` | `2026-01-01/1` (a period every day) |
| `database_generations` | `400` (13 months of days) |
| `database_directory` | `/data/nlbwmon` where `/data` is a mount, else `/usr/share/nlbwmon/db` |
| `commit_interval` | `24h` (unchanged) |

The package's post-install (`/usr/libexec/verso/usage-setup`) sets the interval
and the generations on every install and upgrade, because the shell reads days
and nothing else. It moves the directory only while it is still the stock
`/var/lib/nlbwmon`, which is tmpfs and loses its history on every reboot; an
owner who chose a directory keeps their choice. `/usr/share/nlbwmon/db` is
listed in `keep.d`; `/data` survives a flash on its own. A power cut costs at
most the day since the last commit. An installation that kept monthly periods
before keeps its current month as the archive of the month's first day.

Months are calendar months, summed by the shell from the days. Days before today
never change, so the shell keeps each day's per-device totals once read and asks
`nlbwmon` only for today. Totals are keyed by MAC, the identity the roster uses;
`nlbwmon` records it per stream from the neighbour table.

Usage is on while the `nlbwmon` service is enabled, and the Services page is
where it is switched. With it disabled, the roster draws no usage columns and the
device panel no Usage tab.

### A broken day is moved aside

`nlbwmon` writes its database in place (`open` with `O_TRUNC`) and refuses to
start when the current period's file cannot be read. A power cut during the
commit therefore stops collection until the file is removed. `verso-rpcd`
checks once a minute: when `nlbwmon` is enabled but its socket does not answer,
and the current period's file fails to decompress, it renames that file with a
`.broken` suffix and restarts the service. The day's earlier counts are lost;
every other day stays.

### Live rates are read only while watched

`verso-rpcd` answers `usageLive` from a ctnetlink dump of conntrack, not from
`/proc/net/nf_conntrack`: the proc file walks the table again from its start
for every page it hands out, so a busy router's ten thousand connections cost a
second and more of kernel time per read, where the dump takes a twentieth of
that. For each
connection it credits the original direction's bytes as sent by the original
source and received by the reply source, and the reply direction's the other
way, so a NATed connection counts for the LAN device that opened it and a
forwarded one for the LAN device that answers it. It keeps each connection's
last bytes and adds only what is new to a running total per address. The totals
only grow, so any reader turns two readings into a rate. Connections that close
between readings lose their last bytes; at a reading every two seconds that is
a rounding error.

The shell reads it every two seconds while a Devices page or a device panel is
open, and not at all otherwise. It folds addresses into devices by the neighbour
table.

### What is never stored

Only byte totals per device. No connection, peer address or destination is
recorded: a year of who visited what is not something a router should keep. A
device unseen for the retention window drops out with its last day's file.

### The Devices roster

The roster keeps its network lanes and its order; nothing re-ranks. MAC moves to
the device panel to make room for two columns: **Now** (download over upload,
Mbit/s) and **This month** (the calendar month's total). On a busy row each Now
figure stands over a 2px meter: download green and upload amethyst, the WAN
chart's series, measured against the WAN's current total, at most full. A month
has no ceiling to measure against, so it carries no meter. Idle rows show the
empty dash. The device panel opens on the shell's Details tab: the machine facts
the roster does not show (the MAC among them), then the device's live traffic
graph, the overview's Internet graph drawn for the device, and Today, Last 7
days, This month and Last month. The shell keeps the last minute of rates it
took while the roster streamed, so the graph opens on what was watched before
the panel, flat where nothing was, and the roster's stream feeds it from there;
it is drawn only in the panel's own fetch, when a device is opened.

## Consequences

- Every Verso install runs `nlbwmon` and writes to flash once a day.
- Traffic between a LAN device and the router itself counts toward the device,
  as `nlbwmon` counts it; a meter clamps at full.
- A daily breakdown in the panel can reuse the overview's chart at daily
  granularity; it is not drawn yet.
