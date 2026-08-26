# ADR-011 — Plugin management: the shell's trust surface

- **Status:** Accepted
- **Date:** 2026-08-26
- **Deciders:** tomaz@zaman.io
- **Relates to:** ADR-005 (the pages compose from the existing widget
  vocabulary and shell chrome), ADR-006 (manifests and the plugin id are the
  contract being managed), ADR-007 (all privileged work rides the one
  session-gated helper; packages ship their own ACL files), ADR-009 (a surface
  that mutates what the shell trusts is shell-owned, like the password page),
  ADR-010 (package and service operations sit *outside* the staged-changes
  lifecycle).

## Context

Verso's plugins are separate de-privileged binaries: a package installs a
manifest under `/usr/share/verso/plugins/<id>/`, a procd service, and its ACL
files; the shell discovers manifests at startup and renders each plugin's
pages through the schema gateway. Nothing in the UI can see, control, or
acquire plugins — installing one today means a shell restart to be noticed at
all — and nowhere does the interface answer the question the architecture is
built around: *what is this plugin allowed to touch?* Every manifest already
declares its powers (ADR-007 ACL scopes); no page shows them.

LuCI's equivalents (System → Software, System → Startup) manage packages and
services generically, as root, with no notion of a plugin's declared powers.
Verso's de-privileged design allows something stronger: management as a trust
surface, where a plugin's capabilities are first-class content.

The deployment reality this decides against: the Mono image is built on the
apk-based OpenWrt 25.12 series, and its feeds are the device's configured
feeds — Mono's own merged with the official OpenWrt one (the same set the ASU
server builds from). Plugins are not limited to Mono's feed.

## Decision

**Plugin management is a shell-owned surface at `/system/plugins` with two
faces — Installed (manage and monitor what is here) and Discover (browse the
configured feeds) — presenting each plugin with the powers its manifest
declares, driving the full lifecycle through the one privileged helper, with
runtime manifest rediscovery so the shell never restarts itself.**

1. **Shell-owned, not a plugin.** The surface mutates the set of things the
   shell trusts and must exist before any plugin does — the ADR-009 §3
   reasoning that made the password page shell-owned. Routes are
   `/system/plugins` (Installed) and `/system/plugins/discover`, rendered
   through the shell's own renderer with the standard subpage top bar; the
   sidebar row ships with the shell.

2. **Declared powers are the content; the screen informs, it does not gate.**
   Every presentation of a plugin — installed card or install drawer — leads
   with its manifest-declared ACL scopes in plain language ("will be able to:
   write firewall rules, read network state"). Enforcement is unchanged and
   stays where it lives: the shell's write gate and rpcd's ACLs (ADR-007).
   Packages ship their own ACL files, so the install screen is honest
   information, not an approval step; the wording ("will be able to") is true
   under exactly this model.

3. **A plugin is a package named `verso-plugin-*`.** The trailing segment is
   the plugin id: `verso-plugin-wireguard` installs id `wireguard`, mounted at
   `/plugins/wireguard/`. Discover filters feed indexes on the name
   convention; installed truth is a manifest present under
   `/usr/share/verso/plugins/<id>/`. Any configured feed can carry plugins —
   origin is shown, never restricted.

4. **Package operations are helper verbs on apk.** `verso-rpcd` grows narrow,
   sid-gated verbs beside the uci ones: list installed, list available,
   refresh index, install, remove, upgrade. The backend is apk (the 25.12
   target; the dev container runs the same series). The shell process never
   executes package tools itself; there is exactly one privileged path
   (ADR-007), and these verbs join it.

5. **Lifecycle is procd; monitoring composes what the shell already knows.**
   Enabled-at-boot (rc.d enable/disable) renders as a switch, distinct from
   running-right-now (start/stop/restart with live state). The monitor view
   is facts the shell already holds — socket reachability, schema handshake,
   version, service uid — plus recent `logread` lines for the service,
   fetched through the helper.

6. **Discover reads a cached index; the network is touched only on request.**
   Opening the page never fetches; it shows when the index was last
   refreshed, and an explicit Refresh runs the index update through the
   helper. Navigation must never hang on a slow feed or phone out as a side
   effect.

7. **The shell rediscovers manifests at runtime.** After a helper-reported
   install or remove completes, the shell rescans the manifest directory and
   rebuilds its nav and routing — no self-restart. The trigger is the
   completed operation, not filesystem watching: deterministic, and always
   attributable to a known event.

8. **These are immediate acts, outside ADR-010.** Package and service
   operations are not uci writes; nothing about them stages. Destructive acts
   (uninstall, stop) carry their own inline confirms. Uninstalling removes
   the package; the plugin's uci config stays on the router, and the confirm
   says so.

## Non-goals

- **General package management.** This surface manages Verso plugins only.
  A software page for arbitrary packages is a different domain with a
  different audience; folding it in here would bury the trust story.
- **Shell-controlled ACL granting.** Moving ACL deployment from packages into
  the shell — turning the install screen's information into real consent —
  is a separate architectural decision touching packaging and ADR-007's
  install flow. This ADR presents powers; it does not arbitrate them.
- **Sandboxing changes.** How plugins are confined (uid, group, capabilities)
  is ADR-007's ground and is not altered here.

## Consequences

- The privileged helper's surface widens by the package and service verbs —
  each narrow, argument-validated, and gated by the caller's session exactly
  like the uci verbs. The helper remains the single audit point.
- Discovery becomes a runtime concern: the manifest scan moves from
  startup-only into a rescan the management flow (and startup) share.
- The shell gains its own nav rows and two shell-rendered pages beside the
  password page — the schema gateway is unchanged.
- The styleguide's Plugins pages are the visual reference for both faces;
  the shell implementation renders the same widget compositions from live
  data.
- The dev container tracks the apk-based 25.12 series so the package backend
  is exercised for real.
