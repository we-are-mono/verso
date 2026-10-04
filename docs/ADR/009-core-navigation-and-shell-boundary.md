# ADR-009 — Core navigation and the shell/plugin ownership boundary

- **Status:** Accepted
- **Date:** 2026-08-24
- **Deciders:** tomaz@zaman.io
- **Relates to:** ADR-005 (the visual/UI contract the nav chrome is part of),
  ADR-006 (the plugin contract — every section, core or not, is served through it),
  ADR-007 (privilege gating — core sections are privileged like any plugin),
  ADR-010 (UCI staging does not govern immediate platform operations),
  ADR-011 (shell-owned package, plugin, and service management).

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

Verso embodies the good half of this: routing is flat (`/`, `/login`,
`/plugins/<id>/…`, no prefix), auth is middleware over the whole mux, and the
sidebar is built from shell-owned pages plus live plugin manifest registrations.

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

2. **The shell owns a fixed, ordered core section taxonomy.** The core sections
   Verso guarantees, in this canonical order, are:

   | Order | Section  | Covers (illustrative)                                        |
   |-------|----------|--------------------------------------------------------------|
   | 1     | Status   | overview, logs, realtime graphs, processes, routes           |
   | 2     | Network  | interfaces, wireless, DHCP/DNS, static routes, diagnostics   |
   | 3     | Security | firewall zones, port forwards, traffic rules, NAT            |
   | 4     | System   | general/time, admin (password/SSH), startup, cron, backup, reboot |

   This ordered list is data the shell owns — the analog of `luci-base` declaring
   the top-level slots — but it does not drive the top-level chrome directly. The
   sidebar is device-first: a few everyday, plain-language rows (Home, Internet,
   Devices, Wi-Fi, Family, Security, System — several still `#` placeholders) sit up
   top for the least-technical operator, and the remaining core sections render
   below them as section rows. Three sections are not section rows. Status
   is the **Home** overview at the head of the everyday rows, so status never
   appears twice and the nav carries no live state; Security and System are
   everyday rows of their own, each leading to the first live page registered
   under it and lit anywhere inside that domain. The section rows
   render in this canonical order, ahead of any non-core section, so the sidebar
   is coherent and deterministic regardless of plugin discovery order.

3. **The shell serves its own machinery and generic platform administration
   directly; feature-specific device configuration is a plugin.** Four bounded
   kinds of page qualify for in-process ownership:
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
   - **Package and plugin (extension) management** — the shell owns the package
     substrate and its own extension mechanism: package inventory and acquisition,
     which plugins exist, whether each is enabled and healthy, and the rpcd ACLs it is
     granted. This must not itself be a plugin, for the two reasons that make the auth
     surface shell-owned. *Bootstrap:* a disabled or broken plugin could otherwise lock
     the operator out of the very tool needed to repair it — the shell must manage its
     plugin set before and without any plugin being healthy. *Trust:* enabling a plugin
     grants it write ACLs, and the shell is the write-enforcement point (ADR-007), so
     control over the plugin set and its permissions cannot be delegated to a plugin
     without inverting the trust model. Because plugins are packages and may depend on
     non-plugin packages, the same shell-owned package surface handles the complete
     package set rather than inventing an incomplete plugin-only view (ADR-011).
   - **Generic service management** — the shell owns the complete procd service
     inventory and the narrow lifecycle operations start, stop, restart, enable, and
     disable (ADR-011). This is platform administration, not ownership of any service's
     feature semantics: the shell may change whether `dnsmasq` runs, but it does not
     understand or edit DHCP/DNS configuration. Keeping the generic service table in
     the shell also preserves the recovery path when a plugin or its service is broken.
     An owning plugin may request the required lifecycle action while applying its
     configuration, but the privileged act still passes through the shell's broker and
     helper rather than giving the plugin ambient service-control authority.

   The test for what the shell serves in-process is not "is it important" or merely
   "is it read-only." The shell owns its authentication and extension machinery, the
   always-present status baseline, and the explicitly bounded generic platform
   primitives: packages and procd services. These actions
   require no knowledge of a particular feature's configuration. Anything that
   interprets or changes
   **feature-specific device configuration** is a plugin: network, DHCP/DNS, Wi-Fi,
   firewall, VPN, SSH, hostname/timezone, NTP policy, and similar domains all stay
   outside the shell.

   Everything else — every capability that configures a device feature — is served by
   an out-of-process bundled plugin through the ADR-006 gateway, either as its own page
   or as a contribution to a shell page. There is still
   exactly one rendering path (the shell's widget renderer) and one privilege
   model; package and service operations are narrow administrative verbs, not an
   in-process fast path for interpreting device configuration.
   What makes a section *core* is its position in the fixed taxonomy (2), not a
   hardcoded page list. The shell contributes only pages it owns; plugin pages
   exist in navigation only through their live manifest registrations. Bundled
   first-party plugins provide Network, Security, and System configuration on a
   healthy image, but stopping one withdraws its navigation entries just like any
   other plugin. Its direct URL remains mounted and degrades to the ADR-006
   "unavailable" card — never a 500.

   A *section is a nav grouping, not a unit of ownership*: the shell may place its
   own pages into a section beside plugin pages. The **System** section holds the
   shell-owned **Access**, **Packages**, **Services**, and **Maintenance** pages
   next to manifest-registered plugin pages (General, SSH, cron). The bundled
   System plugin's manifest registers General; the shell neither predeclares nor
   conditionally hides it. The System destination is the first live registered
   page, falling back to Access when no System plugin page is available. The
   Security destination resolves the same way, with no shell-owned pages behind
   it: the section's one occupant is the firewall plugin, so the row leads
   nowhere while that plugin is not answering, and the shell never merges its
   heading or subpage bar the way the mixed System frame does. Status is
   shell-only; Security is one plugin end to end; System is mixed.

   **Page ownership is not exclusive composition.** Shell page templates publish a
   stable hook at every semantic seam between their sections (ADR-005, ADR-006). A
   plugin may choose any published hook and contribute a native widget fragment there
   without claiming a navigation row. A shell-owned page therefore composes its own
   fields with, for example, a plugin-owned fragment contributed at a published hook
   on the same page.
   The shell owns section placement and the one apply transaction; each plugin
   retains ownership of its fragment's semantics, validation, ACL, and declarative
   write intent. A missing or failed contribution degrades only its hook and never
   removes or blocks unchanged shell content.

4. **Security is core, and — because it configures the device — it is a plugin.**
   In LuCI the firewall is `luci-app-firewall`, an optional app. In Verso it fills
   a core section (slot 3, guaranteed present) named for what the operator came to
   do rather than for the subsystem that does it, backed by a bundled first-party
   plugin (`verso-plugin-firewall`). It falls on the plugin side of §3 by the same
   rule as Network and System: it *writes device configuration*. Rationale for core: a
   Verso device is a gateway, and a gateway without firewall management is not a
   product. Keeping it a plugin (not in-shell) is deliberate and *stronger* here,
   not weaker — the firewall is the most privileged, most nftables-adjacent
   surface, exactly the code you most want crash-isolated in its own process
   (ADR-006 §7) and privilege-gated (ADR-007). "Core" means bundled and guaranteed,
   never in-shell or privilege-exempt.

5. **Plugins extend the taxonomy; unknown sections sort after core.** A plugin's
   `nav[].section` (ADR-006) either names a core section — its links join that
   section in place — or names a new one. New (non-core) sections render **after**
   all core sections among the section rows, ordered
   deterministically (by section title, then plugin id).
   This is the direct analog of a LuCI app filling the **Services** or **VPN**
   slot, and it is the flexibility the design requires: feature applications LuCI
   delivers as `luci-app-*` (VPN, DDNS, statistics) arrive in Verso as a
   plugin that either enriches a core section or opens its own — with no shell
   change, exactly as ADR-006 §2 intends. Package and service management are the
   shell-owned platform exceptions defined in (3), not extension sections.

6. **Core sections are privileged like any plugin.** Being core grants no ambient
   authority. `verso-plugin-firewall` declares its rpcd write scopes in its
   manifest `acl` and is gated by the shell's `session.access` probe and
   shell-brokered commit (ADR-007) identically to a third-party plugin. Core is a
   packaging and navigation guarantee, never a privilege exemption.

## Consequences

### Positive
- One plugin mechanism for all feature-specific configuration: every contributed or
  standalone capability shares the ADR-006 gateway, ADR-005 rendering path, and
  ADR-007 privilege model. The bounded shell-owned settings use the same widget,
  validation, intent-merge, and apply transaction, so mixed ownership does not
  produce a mixed user experience.
- A freshly-flashed device shows a working overview and can set its first password
  with **zero plugins running**: the shell's baseline and auth surface never depend
  on a plugin being up. Onboarding is impossible to brick by a plugin failure.
- Package repair and service recovery remain available with **zero healthy
  plugins**. A failed extension cannot take the shell's own administrative tools
  down with it.
- The most dangerous surface (firewall) is crash-isolated and privilege-gated by
  the same contract as everything else — core status buys it *no* shortcut around
  isolation.
- Deterministic, coherent chrome: a fixed core order with extensions appended
  means the sidebar reads the same on every device, and a new plugin can never
  displace Status from the top.
- Contributions can deepen an existing task page without creating sidebar clutter;
  a plugin process can fail without taking the surrounding shell page with it.
- Verso improves on LuCI's shape: same small-core-plus-extension model, minus the
  vestigial `admin/` prefix, with the auth boundary where it belongs.

### Costs / negatives
- The core taxonomy (2) is now a shell-owned constant. Adding or reordering a core
  section is a shell change and an ADR amendment — intentional friction, so the
  core surface stays small and deliberate rather than accreting.
- Every template hook is a public compatibility point. The shell may redesign the
  page around it, but removing or renaming the hook needs manifest-version-aware
  degradation and migration.
- Bundled core plugins are a packaging obligation: the image build must guarantee
  the first-party plugins are installed and supervised, or their pages are absent
  from navigation out of the box. A direct request still gets the contained
  "unavailable" state.
- The root helper necessarily gains a small set of high-impact operations. Each one
  needs its own narrow argument schema, rpcd ACL, native validation, failure tests,
  and destructive-action confirmation; a generic command runner is never acceptable.
- nav.go carries the core-first ordering (`buildNav`) beneath a device-first
  sidebar (`buildSidebar`) — everyday rows plus the section rows — so the chrome is a shell concern a plugin cannot rearrange.

### Neutral
- Whether Verso pre-declares *empty* core slots (LuCI's Services/VPN-style 404
  placeholders) is moot here: core slots are backed by always-installed plugins,
  so they are never empty in a healthy device, and a new *extension* section
  simply appears when its plugin is present. Verso has no "declared-but-empty"
  state to resolve to 404.
- Section membership is the plugin's declaration (`nav[].section`); the shell does
  not police which section a third-party plugin joins. A plugin filing itself
  under "Security" places its links there — coherence of *membership* is a review
  concern, not a mechanical gate, consistent with ADR-006's data-driven nav.

## Alternatives considered

- **All device configuration in-shell (two mechanisms).** Serve Network/Security/
  System *configuration* directly from the shell binary for speed, reserve the
  plugin contract for third parties. Rejected: it forks the rendering path (ADR-005
  consistency now has two enforcers), forks the privilege model, and puts the
  firewall — the surface most deserving of isolation — inside the shell's own
  address space. The per-request socket cost (ADR-006) is negligible at admin-UI
  rates and not worth this. The shell's in-process surfaces (§3) are the bounded,
  principled exception — the read-only baseline, the shell's own machinery (the
  auth/credential surface and control of its own plugin set), and the explicitly
  bounded package and service surfaces. Those generic operations do not interpret
  feature configuration, so they do not create a second configuration mechanism.
- **Password change as a plugin (LuCI parity).** LuCI serves the router password
  under its System module like any other page. Rejected for Verso: the shell is the
  authentication authority (ADR-007) and plugins hold no credentials, so delegating
  the shell's own login credential to an out-of-process plugin inverts the trust
  model — and first-boot onboarding must set a password before any plugin is
  guaranteed up. Password stays a shell-owned page, displayed within the System
  section (§3).
- **Basic identity as a System plugin.** Put hostname and timezone behind a
  bundled System plugin rather than in the shell. **Adopted.** General (hostname,
  timezone) writes device configuration, so it falls on the plugin side of §3's
  rule like Network and Security: it ships as a bundled first-party plugin filing
  into the System section. Its manifest registration is the sole source of the
  General navigation entry; if the process is absent the entry is withdrawn, while
  a direct plugin URL degrades to the ADR-006 "unavailable" card. One configuration
  mechanism, and no shell route owns device identity. Service-specific time
  synchronization joins General through the same plugin.
- **Plugin management as a plugin.** Serve the install/enable/disable page through the
  ADR-006 gateway like everything else. Rejected for the same reasons the password page
  is shell-owned: bootstrap (a disabled or broken plugin must not be able to lock the
  operator out of repairing plugins) and trust (enabling a plugin grants it ACLs, and
  the shell is the write-enforcement point, so it cannot delegate control of its own
  plugin set). It is the shell's own machinery, not feature configuration — one of
  the bounded in-process surfaces (§3), not an exception to the rule.
- **Service management as a plugin.** Put the complete procd table and generic
  lifecycle controls in a System plugin. Rejected: the shell must be able to inspect
  and recover plugin services when no plugin is healthy, and delegating control of
  peer plugins to one plugin would invert the extension trust model. The shell owns
  only the narrow generic lifecycle verbs; each service's configuration and semantic
  apply logic remain with its owning plugin.
- **Firewall as an optional plugin (LuCI parity).** Keep the firewall a
  non-bundled `luci-app-firewall` equivalent. Rejected: a gateway ships firewall
  management as a baseline capability; making it optional is a worse default and
  buys nothing, since "core" already costs no extra mechanism (3).
- **Fully dynamic taxonomy (no fixed core order).** Let every section, including
  Status, sort purely by discovery — nav.go's original behavior. Rejected: the
  chrome would reorder as plugins come and go, and there would be no guaranteed
  home for the baseline surface. LuCI's central top-level taxonomy is the part
  worth keeping.
- **Retain an `admin/` (or `/ui`) prefix for namespacing.** Rejected: it carries
  no information (there is only one tree), and its one real job (auth anchoring) is
  already handled by middleware. A prefix would be cargo-culted from LuCI's
  history.
