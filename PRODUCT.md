# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

Primary: technical OpenWrt users — people who know what a zone, a lease, a VLAN
and a UCI option are, and who would otherwise reach for LuCI or SSH. They come
to configure and inspect a router they run themselves.

Also served:

- **Less technical owners** doing one task (share a port, pause a device,
  change the Wi-Fi password). A reader mode that hides the tuning from them
  is deferred (ADR-015); every page renders in full.
- **Plugin authors**: third parties who ship pages into Verso as a widget
  schema (ADR-005, ADR-006).

## Product Purpose

Verso is a web admin UI for OpenWrt that **replaces** LuCI on devices with the
resources for it. Where Verso runs, it is the one interface you use. Success
means an OpenWrt user can configure and inspect the router from Verso without
dropping to SSH or LuCI, and a plugin written by someone else looks and behaves
like part of the shell.

## Positioning

- **The stock interface of Mono Gateway hardware.** Verso ships as the default
  UI on Mono's routers (Mono Gateway Development Kit, LS1046A). It also runs on
  other OpenWrt boards that meet the flash floor.
- **Plugins that look native by construction.** Plugins are out-of-process
  services on unix sockets. They return a JSON widget schema and the shell
  renders it. Plugins never emit HTML or CSS and cannot take the shell down.
  Two plugins by two strangers render identically. The only escape hatch is
  `raw`: display-only Markdown, instrumented as a demand signal for the next
  widget.

## Operating Context

- Served from the router itself, reached from a browser on the LAN. Sign-in
  goes through rpcd sessions. Before sign-in the page shows only what the LAN
  already broadcasts.
- Every change is staged into UCI and applied together from a top-bar chip.
  The router rolls the changes back itself if it stops answering within 30
  seconds (ADR-010).
- Firmware upgrades go through owut and a self-hosted ASU server. Verso ships
  as apk packages in its own feed.
- `DESIGN.md` holds the design system and the rules that code follows; the
  running app is the visual reference.

## Capabilities and Constraints

- **Pages:** Overview, Devices, Network (interfaces, DHCP, DNS), Wireless, Firewall
  (rules, zones, redirects, activity, settings), Routing, System (general,
  maintenance, access, packages, services, logs, diagnostics). First-party
  plugins: system, firewall, interfaces, dnsdhcp, wireless, wireguard, qos.
- **Stack:** the shell is a single static Go binary (`net/http`,
  `html/template`, HTMX, Alpine in CSP mode). Styles come from the Tailwind v4
  standalone CLI. No SPA, no npm/Node build. A privileged Rust companion,
  `verso-rpcd`, handles what rpcd can't. All reads and writes are ACL-gated
  through rpcd, never ambient root (ADR-007).
- **Hardware floor:** 128 MB flash (NAND class). LuCI remains the right choice
  below that. The device build target is arm64.
- **Closed widget set:** plugins get generic, composable widgets (the
  HTML-element model), never domain-specific ones. A new widget needs approval.
- **Localization:** the English source string is the key. Language catalogs
  ship as data packages. Slovenian is first (`i18n/sl`). Layouts must survive
  longer translated strings (ADR-012).
- **Stage:** pre-alpha. No backwards compatibility until alpha.

## Brand Commitments

- Product name **Verso**, running on **Mono** hardware. The top bar reads
  "hostname | Mono · model".
- Voice: plain language and short sentences. The resting state is silent, and
  only failures earn words. Counts, not lists. Output is translation-proof.
- Icons: Lucide only.

## Evidence on Hand

- Live test setup: the Docker OpenWrt lab (`docker compose`) and the Mono
  Gateway DK board.
- No customers, testimonials, benchmarks, pricing or deployment numbers exist.
  Do not invent them.

## Product Principles

1. **Replace LuCI, don't wrap it.** A task that sends the user back to SSH or
   LuCI is a gap in Verso.
2. **Consistency by construction.** Semantic widget props, never presentational
   ones. Anything that can drift gets enforced in the shell, not left to the
   plugin author.
3. **Every change is reversible until applied.** Stage, review, apply, with
   rollback on the router. Nothing is live by surprise.
4. **Depth follows the reader.** The machinery is all there for the power user,
   and plain-language grouping keeps a one-task visit readable.
5. **UCI translated, not transcribed.** The screen maps one-to-one onto UCI
   (sections, options, lists), with each option's key shown as a chip, so a
   power user can predict `uci show` from the page. Labels, grouping, order and
   verdicts are in plain words, so a less savvy user never faces a wall of
   bare options.

## Accessibility & Inclusion

WCAG 2.2 AA. It covers contrast (placeholders count as text at 4.5:1), full
keyboard operation and screen-reader semantics. Translated strings must not
break layouts.
