# ADR-011 — Plugin management: the shell's trust surface

- **Status:** Accepted
- **Date:** 2026-08-26
- **Deciders:** tomaz@zaman.io
- **Relates to:** ADR-005 (the pages compose from the existing widget
  vocabulary and shell chrome), ADR-006 (manifests and the plugin id are the
  contract being managed), ADR-007 (all privileged work is
  session-gated; packages ship their own ACL files), ADR-009 (a surface
  that mutates what the shell trusts is shell-owned, like the password page),
  ADR-010 (package and service operations sit *outside* the staged-changes
  lifecycle).

## Context

Verso's plugins are separate de-privileged binaries: a package installs a
manifest under `/usr/share/verso/plugins/<id>/`, a procd service, and its ACL
files; the shell discovers manifests at startup and renders each plugin's
pages through the schema gateway. Nothing in the UI can see, control, or
acquire plugins — installing one today means a shell restart to be noticed at
all.

LuCI's equivalents (System → Software, System → Startup) manage packages and
services generically, as root. Verso's de-privileged design keeps the same
management shell-owned and session-gated, and knows a plugin by its manifest
and id rather than as an anonymous package.

The deployment reality this decides against: the Mono image is built on the
apk-based OpenWrt 25.12 series, and its feeds are the device's configured
feeds — Mono's own merged with the official OpenWrt one (the same set the ASU
server builds from). Plugins are not limited to Mono's feed.

## Decision

**Two shell-owned surfaces, split by nature: `/system/packages` (files on
disk — Installed, Upgradable and All listings over the cached feeds) and
`/system/services` (procd's live table, the userspace half of `ps`). A
package is a group of files that may or may not provide a service; a service
is a running, stoppable thing — too different to share one view. Each drives
its acts through session-gated privilege — packages through the `verso-rpcd`
helper, services through rpcd's native `rc` object — with runtime manifest
rediscovery so the shell never restarts itself.**

1. **Shell-owned, not a plugin.** The surface mutates the set of things the
   shell trusts and must exist before any plugin does — the ADR-009 §3
   reasoning that made the password page shell-owned. Routes are
   `/system/packages` (Installed, Upgradable and All) and
   `/system/packages/discover` (the Install drawer), rendered through the shell's
   own renderer. Installed is the default. All pages through the cached index
   in place; Installed and Upgradable filter the local inventory. Search and
   package actions return panel responses, keeping full pages out of drawers.
   The sidebar row ships with the shell.

2. **The screen informs, it does not gate.** Each presentation of a package —
   installed row or Available drawer — shows its plain facts: name,
   description, version, feed, license, size, and the install, remove or
   single-package upgrade action. Installed files are read on demand. Enforcement is unchanged and stays where it lives: the shell's
   write gate and rpcd's ACLs (ADR-007). Packages ship their own ACL files, so
   installing is not an approval step; the surface acquires and removes
   packages, it does not arbitrate a plugin's powers.

3. **A plugin is a package named `verso-plugin-*`; the surface handles every
   package.** The trailing segment is the plugin id: `verso-plugin-wireguard`
   installs id `wireguard`, mounted at `/plugins/wireguard/`; installed truth
   is a manifest under `/usr/share/verso/plugins/<id>/`. The Install drawer searches the
   cached feed index after an explicit package-name search; its initial
   state explains that results appear after Search rather than manufacturing a
   default result set. Installed lists the device's full package set as one flat
   table in backend order, the page-wide lens keeping it one page. Any
   configured feed; origin shown, never restricted.
   A small keep-list (busybox, apk, procd, ubus, rpcd, …) refuses removal of
   what keeps the device and this surface alive.

4. **Package operations are helper verbs on apk.** `verso-rpcd` grows narrow,
   sid-gated verbs beside the uci ones: list installed, list available,
   refresh index, install, remove, upgrade one package and list its installed
   files. Catalog reads use a bulk local APK query and bounded response pages. The backend is apk (the 25.12 target; the
   dev container runs the same series). The shell process never executes
   package tools itself; package work rides the session-gated helper path
   (ADR-007), and these verbs join it.

5. **Lifecycle is procd, on the Services page — plugins are not special
   there.** `/system/services` renders procd's whole rc table, lined like
   the process list it corresponds to: every service as one row of columns —
   name with an icon-bearing lifecycle chip (daemon, subsystem, or startup
   task), providing package, live state (running includes oldest-process
   uptime), compact runtime (PID/process count and aggregate RSS), immediate icon-only restart
   and start/stop actions. These change runtime without changing boot policy;
   enabled-but-crashed daemons and PID-less subsystems remain representable.
   An action reads procd once and returns only the affected runtime cells;
   it does not rebuild the page or re-read package ownership. A lost response
   never causes an automatic second POST. Runtime comes from
   procd's `service.list` PIDs and procfs;
   it needs no periodic sampler. Completed startup tasks read "runs at boot"
   rather than "stopped" and carry neither start/stop nor restart.
   Daemons may read running or stopped. PID-less subsystems never read stopped
   merely because the generic procd process check is false; a positive status
   may read active, while an indeterminate status remains blank. No drawers:
   every fact and immediate action is a column. A service keep-list
   (verso, verso-rpcd, rpcd, ubus)
   refuses lifecycle acts from the UI — severing them severs the surface;
   firewall remains restartable but has a locked stop action and rejects stop
   or disable, because firewall4's stop action flushes the kernel firewall, NAT,
   and forwarding rules rather than stopping a harmless resident process;
   the rest, network included, stays the operator's call. Verso plugin
   services sharpen the state with the socket probe (running / not
   responding). Turning a plugin off also withdraws its manifest-registered pages
   from navigation on the next render; the Services row remains available so it
   can be turned back on. The Packages inventory carries no lifecycle cells at all.

6. **Available reads a cached index; the network is touched only on request.**
   Opening the page never contacts the feeds; it shows when the index was last
   refreshed, and an explicit Refresh runs the index update through the
   helper. During refresh the browser keeps its current inventory, polls job
   status, and updates the listing in place. Refresh and package mutations
   re-read the local package update lane without contacting the firmware
   server. Navigation must never hang on a slow feed or phone out as a side
   effect.

7. **The shell rediscovers manifests at runtime.** After a helper-reported
   install, upgrade or remove completes, the shell rescans the manifest directory and
   rebuilds its navigation — no self-restart. The trigger is the
   completed operation, not filesystem watching: deterministic, and always
   attributable to a known event.

8. **These are immediate acts, outside ADR-010.** Package and service
   operations are not uci writes; nothing about them stages. Uninstall and
   stop submit directly and take effect at once, with no inline confirm step.

## Non-goals

- **Shell-controlled ACL granting.** Moving ACL deployment from packages into
  the shell — making the install an act of granting powers — is a separate
  architectural decision touching packaging and ADR-007's install flow. This
  surface acquires and removes packages; it does not arbitrate a plugin's
  powers.
- **Sandboxing changes.** How plugins are confined (uid, group, capabilities)
  is ADR-007's ground and is not altered here.

## Consequences

- The privileged helper's surface widens by the package verbs — each narrow,
  argument-validated, and gated by the caller's session exactly like the uci
  verbs. Service lifecycle rides rpcd's native `rc` object under verso's ACL;
  both are session-gated audit points.
- Discovery becomes a runtime concern: the manifest scan moves from
  startup-only into a rescan the management flow (and startup) share.
- The shell gains its own nav rows and two shell-rendered pages beside the
  password page — the schema gateway is unchanged.
- The design canvases are the visual reference for both faces; the shell
  implementation renders the same compositions from live data.
- The dev container tracks the apk-based 25.12 series so the package backend
  is exercised for real.
