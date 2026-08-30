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
disk — Installed inventory + Discover over the configured feeds) and
`/system/services` (procd's live table, the userspace half of `ps`). A
package is a group of files that may or may not provide a service; a service
is a running, stoppable thing — too different to share one view. Each drives
its acts through session-gated privilege — packages through the `verso-rpcd`
helper, services through rpcd's native `rc` object — with runtime manifest
rediscovery so the shell never restarts itself.**

1. **Shell-owned, not a plugin.** The surface mutates the set of things the
   shell trusts and must exist before any plugin does — the ADR-009 §3
   reasoning that made the password page shell-owned. Routes are
   `/system/packages` (Installed) and `/system/packages/discover`, rendered
   through the shell's own renderer with the standard subpage top bar; the
   sidebar row ships with the shell.

2. **The screen informs, it does not gate.** Each presentation of a package —
   installed row or Discover drawer — shows its plain facts: name,
   description, version, feed, license, size, and the install or remove
   action. Enforcement is unchanged and stays where it lives: the shell's
   write gate and rpcd's ACLs (ADR-007). Packages ship their own ACL files, so
   installing is not an approval step; the surface acquires and removes
   packages, it does not arbitrate a plugin's powers.

3. **A plugin is a package named `verso-plugin-*`; the surface handles every
   package.** The trailing segment is the plugin id: `verso-plugin-wireguard`
   installs id `wireguard`, mounted at `/plugins/wireguard/`; installed truth
   is a manifest under `/usr/share/verso/plugins/<id>/`. Discover searches the
   whole feed index and surfaces plugins first via the default `verso-plugin`
   query; Installed lists the device's full package set as one flat table in
   backend order, the page-wide lens keeping it one page. Any configured feed;
   origin shown, never restricted.
   A small keep-list (busybox, apk, procd, ubus, rpcd, …) refuses removal of
   what keeps the device and this surface alive.

4. **Package operations are helper verbs on apk.** `verso-rpcd` grows narrow,
   sid-gated verbs beside the uci ones: list installed, list available,
   refresh index, install, remove. The backend is apk (the 25.12 target; the
   dev container runs the same series). The shell process never executes
   package tools itself; package work rides the session-gated helper path
   (ADR-007), and these verbs join it.

5. **Lifecycle is procd, on the Services page — plugins are not special
   there.** `/system/services` renders procd's whole rc table, lined like
   the process list it corresponds to: every service as one row of columns —
   name, providing package (exact name match), live state, boot as a
   checkmark, and the on/off switch (on = enable+start, off = stop+disable —
   one human concept, both procd facts). No drawers: every fact is a column,
   and off→on covers restart. A service keep-list (verso, rpcd, ubus)
   refuses lifecycle acts from the UI — severing them severs the surface;
   the rest, network included, stays the operator's call. Verso plugin
   services sharpen the state with the socket probe (running / not
   responding). Turning a plugin off also withdraws its manifest-registered pages
   from navigation on the next render; the Services row remains available so it
   can be turned back on. The Packages inventory carries no lifecycle cells at all.

6. **Discover reads a cached index; the network is touched only on request.**
   Opening the page never fetches; it shows when the index was last
   refreshed, and an explicit Refresh runs the index update through the
   helper. Navigation must never hang on a slow feed or phone out as a side
   effect.

7. **The shell rediscovers manifests at runtime.** After a helper-reported
   install or remove completes, the shell rescans the manifest directory and
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
- The styleguide's Plugins pages are the visual reference for both faces;
  the shell implementation renders the same widget compositions from live
  data.
- The dev container tracks the apk-based 25.12 series so the package backend
  is exercised for real.
