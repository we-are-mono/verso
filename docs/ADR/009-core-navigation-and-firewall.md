# ADR-009 — Core navigation: a flat, shell-owned section taxonomy that plugins extend; Firewall is core

- **Status:** Accepted
- **Date:** 2026-08-24
- **Deciders:** tomaz@zaman.io
- **Relates to:** ADR-005 (the visual/UI contract the nav chrome is part of),
  ADR-006 (the plugin contract — every section, core or not, is served through it),
  ADR-007 (privilege gating — core sections are privileged like any plugin).

## Context

Stock LuCI (OpenWrt 24.10) was stripped to zero `luci-app-*` to see its true core.
The result is a clean lesson:

- **A small, fixed core.** `luci-base` declares the top-level menu; three modules
  fill it — `luci-mod-status`, `luci-mod-network`, `luci-mod-system`. Everything
  else (Firewall, package manager, VPNs, DDNS, …) is a `luci-app-*` plugin.
- **Named extension slots.** `luci-base` also declares empty **Services** and
  **VPN** top-level slots that resolve to 404 until a plugin fills them. The
  top-level taxonomy is owned centrally; plugins slot into it rather than each
  inventing its own top-level menu.
- **The `admin/` prefix is vestigial.** Every one of the 50 core menu entries sits
  under `admin/`. It survives from when LuCI shipped parallel UI trees (`admin`,
  `mini`, public status). Those are gone; the prefix now carries no information.
  Its only real job — anchoring the auth requirement (`admin` declares
  `auth.login`) — is better done in middleware.

Verso already embodies the good half of this: routing is flat (`/`, `/login`,
`/plugins/<id>/…`, no prefix — routes.go), auth is middleware over the whole mux
(`securityHeaders(hostGuard(requireAuth(mux)))` — server.go), and the sidebar is
built from a hardcoded **Status** group plus plugin manifests (nav.go). What is
*not* yet decided: which sections constitute Verso's core, in what order, whether
"core" is a different mechanism from a plugin, and where the firewall lives.

This ADR fixes that, adopting LuCI's shape (small ordered core + extension) while
dropping its accidents (the dead `admin/` prefix), and diverging on one product
call: **on a gateway the firewall is not optional, so it is core.**

## Decision

1. **Flat URLs, no navigational path prefix. Auth is middleware.** Pages live at
   their own paths; there is no `/admin` (or equivalent) segment. "You must be
   logged in" is enforced once, in `requireAuth`, over every non-public route —
   not by a prefix node. This is already the shell's shape; this ADR ratifies it
   and records *why* (LuCI's prefix is a historical no-op whose only real function
   belongs in middleware).

2. **The shell owns a fixed, ordered core section taxonomy.** The top-level nav
   groups Verso guarantees, in this canonical order, are:

   | Order | Section  | Covers (illustrative)                                        |
   |-------|----------|--------------------------------------------------------------|
   | 1     | Status   | overview, logs, realtime graphs, processes, routes           |
   | 2     | Network  | interfaces, wireless, DHCP/DNS, static routes, diagnostics   |
   | 3     | Firewall | zones, port forwards, traffic rules, NAT                     |
   | 4     | System   | general/time, admin (password/SSH), startup, cron, backup, reboot |

   This ordered list is data the shell owns — the analog of `luci-base` declaring
   the top-level slots. Core sections always render in this order, ahead of any
   non-core section, so the chrome is coherent and deterministic regardless of
   plugin discovery order.

3. **The shell serves its own machinery directly; every section that configures the
   device is a plugin.** The shell renders in-process only the read-only baseline and
   its own machinery — never a device operation. Three kinds of page qualify, each the
   shell's own concern rather than the device's:
   - **The read-only status baseline** (the Status overview). The shell must
     render a coherent first screen on a freshly-flashed device *before any plugin
     is up*, and this page only *reads* state — it has nothing to crash, nothing to
     privilege-gate, and no third-party author, so it gains nothing from the plugin
     mechanism and loses the always-present guarantee.
   - **The auth/credential surface** — login, logout, and the operator's own
     password/account change. The shell is the authority on its own authentication
     (ADR-007): it holds the session, and plugins hold no credentials. The
     credential that gates the entire shell must never be held or mutated by an
     out-of-process plugin. First boot is passwordless (`root:::`), so setting the
     first password is onboarding the shell owns, independent of any plugin.
   - **Plugin (extension) management** — the shell owns its own extension mechanism:
     which plugins exist, whether each is enabled and healthy, and the rpcd ACLs it is
     granted. This must not itself be a plugin, for the two reasons that make the auth
     surface shell-owned. *Bootstrap:* a disabled or broken plugin could otherwise lock
     the operator out of the very tool needed to repair it — the shell must manage its
     plugin set before and without any plugin being healthy. *Trust:* enabling a plugin
     grants it write ACLs, and the shell is the write-enforcement point (ADR-007), so
     control over the plugin set and its permissions cannot be delegated to a plugin
     without inverting the trust model. It lives in the System section beside Password.
     The underlying package install/remove still flows through rpcd like any privileged
     write — the shell owns the *authority* over its plugin set, not a bespoke installer.

   The test for what the shell serves in-process is not "is it important" or "is it
   read-only" but: *is it the shell's own machinery — authentication, or control of its
   own plugin set — or the baseline that must exist before plugins?* Anything that
   **operates or configures the device is a plugin**, including restarting device
   services (network, dnsmasq, a VPN, the firewall): a service restart is a device
   operation, done contextually inside the owning plugin's rpcd commit, with a general
   services/startup list living in the **System** plugin (§2) — never a shell "service
   manager." The one service-shaped thing the shell owns is restarting a *plugin's own
   process* and reporting its health — part of plugin management (the shell supervises
   plugin sockets), not device service control.

   Everything else — every page that *configures the device* — is served by an
   out-of-process bundled plugin through the ADR-006 gateway. There is still
   exactly one rendering path (the shell's widget renderer) and one privilege
   model; the shell grows **no** in-process fast path for device configuration.
   What makes a section *core* is unchanged: a guaranteed slot in the fixed
   taxonomy (2), always present — because the shell serves it (Status) or because a
   first-party plugin implementing it **ships bundled and always installed**
   (Network, Firewall, System). A plugin-backed core section whose plugin is
   missing or unhealthy degrades to the ADR-006 "unavailable" card in its own slot
   — never a 404 or a 500.

   A *section is a nav grouping, not a unit of ownership*: the shell may place its
   own pages into a section beside plugin pages. The **System** section holds the
   shell-owned **Password** page next to plugin-owned pages (hostname, time, SSH,
   startup). Status is shell-only; Firewall is one plugin end to end; System is
   mixed.

4. **Firewall is core, and — because it configures the device — it is a plugin.**
   In LuCI the firewall is `luci-app-firewall`, an optional app. In Verso it is a
   core section (slot 3, guaranteed present) backed by a bundled first-party plugin
   (`verso-plugin-firewall`). It falls on the plugin side of §3 by the same rule as
   Network and System: it *writes device configuration*. Rationale for core: a
   Verso device is a gateway, and a gateway without firewall management is not a
   product. Keeping it a plugin (not in-shell) is deliberate and *stronger* here,
   not weaker — the firewall is the most privileged, most nftables-adjacent
   surface, exactly the code you most want crash-isolated in its own process
   (ADR-006 §7) and privilege-gated (ADR-007). "Core" means bundled and guaranteed,
   never in-shell or privilege-exempt.

5. **Plugins extend the taxonomy; unknown sections sort after core.** A plugin's
   `nav[].section` (ADR-006) either names a core section — its links join that
   section in place — or names a new one. New (non-core) sections render **after**
   all core sections, ordered deterministically (by section title, then plugin id).
   This is the direct analog of a LuCI app filling the **Services** or **VPN**
   slot, and it is the flexibility the design requires: anything LuCI delivers as a
   `luci-app-*` (VPN, DDNS, statistics, a package manager) arrives in Verso as a
   plugin that either enriches a core section or opens its own — with no shell
   change, exactly as ADR-006 §2 intends.

6. **Core sections are privileged like any plugin.** Being core grants no ambient
   authority. `verso-plugin-firewall` declares its rpcd write scopes in its
   manifest `acl` and is gated by the shell's `session.access` probe and
   shell-brokered commit (ADR-007) identically to a third-party plugin. Core is a
   packaging and navigation guarantee, never a privilege exemption.

## Consequences

### Positive
- One mechanism for all device configuration: every configuring section — core or
  third-party — shares the ADR-006 gateway, the ADR-005 rendering path, and the
  ADR-007 privilege model. The shell serves only the read-only baseline and its own
  auth surface directly, so there is no second *configuration* code path to build,
  test, or keep visually consistent.
- A freshly-flashed device shows a working overview and can set its first password
  with **zero plugins running**: the shell's baseline and auth surface never depend
  on a plugin being up. Onboarding is impossible to brick by a plugin failure.
- The most dangerous surface (firewall) is crash-isolated and privilege-gated by
  the same contract as everything else — core status buys it *no* shortcut around
  isolation.
- Deterministic, coherent chrome: a fixed core order with extensions appended
  means the sidebar reads the same on every device, and a new plugin can never
  displace Status from the top.
- Verso improves on LuCI's shape: same small-core-plus-extension model, minus the
  vestigial `admin/` prefix, with the auth boundary where it belongs.

### Costs / negatives
- The core taxonomy (2) is now a shell-owned constant. Adding or reordering a core
  section is a shell change and an ADR amendment — intentional friction, so the
  core surface stays small and deliberate rather than accreting.
- "Always installed" core plugins are a packaging obligation: the image build must
  guarantee the bundled first-party plugins are present and supervised, or a core
  slot shows "unavailable" out of the box.
- nav.go must change from pure discovery-order grouping to core-first ordering
  with an extension tail — a small, well-scoped change to `buildNav`.

### Neutral
- Whether Verso pre-declares *empty* core slots (LuCI's Services/VPN-style 404
  placeholders) is moot here: core slots are backed by always-installed plugins,
  so they are never empty in a healthy device, and a new *extension* section
  simply appears when its plugin is present. Verso has no "declared-but-empty"
  state to resolve to 404.
- Section membership is the plugin's declaration (`nav[].section`); the shell does
  not police which section a third-party plugin joins. A plugin filing itself
  under "Firewall" places its links there — coherence of *membership* is a review
  concern, not a mechanical gate, consistent with ADR-006's data-driven nav.

## Alternatives considered

- **All device configuration in-shell (two mechanisms).** Serve Network/Firewall/
  System *configuration* directly from the shell binary for speed, reserve the
  plugin contract for third parties. Rejected: it forks the rendering path (ADR-005
  consistency now has two enforcers), forks the privilege model, and puts the
  firewall — the surface most deserving of isolation — inside the shell's own
  address space. The per-request socket cost (ADR-006) is negligible at admin-UI
  rates and not worth this. The shell's in-process surfaces (§3) are the bounded,
  principled exception — the read-only baseline, and the shell's own machinery (the
  auth/credential surface and control of its own plugin set). None *operates or
  configures the device*, so none reopens this fork.
- **Password change as a plugin (LuCI parity).** LuCI serves the router password
  under its System module like any other page. Rejected for Verso: the shell is the
  authentication authority (ADR-007) and plugins hold no credentials, so delegating
  the shell's own login credential to an out-of-process plugin inverts the trust
  model — and first-boot onboarding must set a password before any plugin is
  guaranteed up. Password stays a shell-owned page, displayed within the System
  section (§3).
- **Plugin management as a plugin.** Serve the install/enable/disable page through the
  ADR-006 gateway like everything else. Rejected for the same reasons the password page
  is shell-owned: bootstrap (a disabled or broken plugin must not be able to lock the
  operator out of repairing plugins) and trust (enabling a plugin grants it ACLs, and
  the shell is the write-enforcement point, so it cannot delegate control of its own
  plugin set). It is the shell's own machinery, not device configuration — a third
  in-process surface (§3), not an exception to the rule.
- **A general service manager in the shell.** Add a shell page to start/stop/restart
  device services. Rejected: a service restart is a *device operation*, not the shell's
  own machinery — it belongs in the owning plugin's rpcd commit (contextually) or the
  System plugin's startup/services page. Pulling it in-shell reopens the very
  device-configuration-in-shell fork this ADR rejects. The shell only ever restarts a
  *plugin's own process* (part of plugin management), never device services.
- **Firewall as an optional plugin (LuCI parity).** Keep the firewall a
  non-bundled `luci-app-firewall` equivalent. Rejected: a gateway ships firewall
  management as a baseline capability; making it optional is a worse default and
  buys nothing, since "core" already costs no extra mechanism (3).
- **Fully dynamic taxonomy (no fixed core order).** Let every section, including
  Status, sort purely by discovery — the current nav.go behavior. Rejected: the
  chrome would reorder as plugins come and go, and there would be no guaranteed
  home for the baseline surface. LuCI's central top-level taxonomy is the part
  worth keeping.
- **Retain an `admin/` (or `/ui`) prefix for namespacing.** Rejected: it carries
  no information (there is only one tree), and its one real job (auth anchoring) is
  already handled by middleware. A prefix would be cargo-culted from LuCI's
  history.
