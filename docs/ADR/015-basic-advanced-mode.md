# ADR-015 — Basic and Advanced: an app-wide reader mode

- **Status:** Deferred — Verso has no reader mode; every surface renders in
  full. This design returns once the core pages are complete.
- **Date:** 2026-09-01
- **Deciders:** tomaz@zaman.io
- **Relates to:** ADR-005 (the widget vocabulary carries the visibility
  declarations), ADR-006 (the manifest `nav` entries and envelope `pages`
  carry them), ADR-007 (privilege gating — what mode is *not*), ADR-009 (the
  navigation the mode filters), ADR-013 (why the preference is not a uci
  setting).

## Context

Verso serves two readers with one interface. The least-technical operator
came to do a task — share a port, pause a device, change the Wi-Fi password —
and every option beyond that task is noise that makes the page harder, not
richer. The power user came for the same tasks plus the machinery: match
fields, metrics, timeouts, the sections that tune rather than enable. Both
are legitimate; neither should pay for the other.

The firewall makes the tension concrete. A traffic rule needs about five
fields to work; everything else on the form is optional tuning. An interface
that shows all of it makes the basic operator feel the product is not for
them — while an interface that only ever shows the five makes the power user
leave for SSH. The classic resolutions are all bad: parallel UI trees rot
(LuCI shipped `admin`/`mini` trees and today carries only the vestigial
`admin/` prefix, ADR-009); per-page "show more" toggles scatter the choice
across every page and forget it between visits; and burying depth under a
separate "advanced" area of the navigation discloses *pages* but says nothing
about the advanced fields and sections *inside* a page every reader still
lands on.

The observation that resolves it: expertise is a property of the **reader**,
not of any page. One person's whole visit is basic or advanced; no one wants
a basic firewall page and an advanced DHCP page in the same session.

## Decision

**Verso has one app-wide reader mode — `basic` (the default) or `advanced` —
chosen by a single switch in the shell chrome and applied to every surface:
navigation entries, page sections, and form fields. Content declares which
mode it belongs to; the shell filters at render. Mode hides capability,
never state, and it is never a privilege boundary.**

1. **One mode, owned by the shell, persisted per browser.** The mode is a
   cookie the shell reads before rendering anything. It is *not* a uci
   setting: ADR-013 stores operator intent about the **device**, and mode is
   a preference of the **reader** — two people administering the same router
   from two browsers hold two modes. Storing it in uci would also drag a view
   toggle through the stage (ADR-010), turning "show me more" into
   a reviewable device change, which it is not. The switch flips the cookie
   and re-renders; nothing is staged, nothing is written to the device. The
   shell chrome renders the one switch; its exact placement is a design
   decision, not this ADR's. A browser without the cookie is in
   basic mode — the product meets the least-technical reader first.

2. **A three-state visibility vocabulary for pages and sections.** A
   navigation entry (manifest `nav[]`, ADR-006), a subpage entry (envelope
   `pages[]`), and a `section` widget each carry an optional `mode`:

   - `"basic"` — rendered only in basic mode.
   - `"advanced"` — rendered only in advanced mode.
   - absent — rendered in both.

   The three states are one enum, not two booleans, because they are
   mutually exclusive. `mode: "basic"` exists so a *simplified face* can
   stand in for the full machinery: a basic-mode section presents a fact in
   task language while its `mode: "advanced"` counterpart presents the same
   fact as the full table or field set. The two faces are alternatives the
   mode selects between, so no reader ever sees the same fact twice — the
   pairing is how a page offers two depths without duplicating a fact at two
   altitudes.

3. **A boolean for fields.** A `field` widget carries an optional
   `advanced: true`; absent means visible in both modes. Fields skip the
   basic-only state because a field has no simpler face — a *section* does.
   If a basic-only field case ever materializes, the boolean grows into the
   same enum as (2); nothing else moves.

4. **Mode hides capability, never state.** A tag hides what a reader could
   do, never what is already done:

   - A field holding a non-default value renders in every mode. The declarer
     enforces this: tags arrive per render (every envelope is a fresh
     declaration), and the plugin — the only party that knows its defaults —
     omits `advanced` from a field whose value is live.
   - An advanced-only section is legitimate only while its contents are at
     defaults, or while a paired basic-mode section represents the same
     facts. A section hiding live, unrepresented configuration makes basic
     mode lie about the router, and that is a plugin defect, not a style
     choice.
   - A form submitted in basic mode omits its hidden advanced fields, and
     the plugin treats each absent field as its default — the same
     "absent means default" convention ADR-013 fixes for Verso's own config.
     Because live values are never hidden (first point), absence genuinely
     means default, and a basic-mode submission always produces complete,
     valid configuration.

   The shell's filter is deliberately mechanical — drop what is tagged for
   the other mode — and the semantic obligation rides with the declaration,
   where the knowledge lives (ADR-006: the plugin is authoritative for its
   own semantics).

5. **Navigation filters by the same declarations, across both tiers.** The
   navigation is two-tier: tier one is the left sidebar (the everyday rows
   and the sections), tier two is the top subpage bar (`pages`). Mode
   governs existence at both tiers — an entry tagged `advanced` is not
   collapsed or dimmed in basic mode, it is absent. In advanced mode the
   additional sections render as ordinary sidebar entries in the canonical
   core order with extension sections after (ADR-009 §2, §5); the mode
   changes which entries exist, never their order or grouping. The sidebar
   needs no second disclosure mechanism of its own: the mode switch *is* the
   disclosure.

6. **Mode is never authority.** Every write remains gated by ADR-007
   regardless of mode; flipping to advanced grants nothing, and hiding a
   page in basic mode protects nothing. Direct navigation to an
   advanced-tagged page works in basic mode — the URL renders, with the
   page's own sections and fields still filtered by the reader's mode. Mode
   is a reading aid; ADR-007 is the security boundary. Conflating them —
   "it's hidden, so it's safe" — is the failure this clause forbids.

7. **Everything untagged is unchanged.** All three declarations are
   optional. A manifest, envelope, or widget tree that never mentions mode
   renders identically in both modes — a plugin adopts progressive
   disclosure by tagging, not by rewriting.

## Consequences

### Positive

- One disclosure model at every altitude — nav entry, subpage, section,
  field — driven by one switch, remembered across visits. The basic reader
  gets a complete, truthful, small product; the power user gets everything,
  in place, without a parallel tree.
- The firewall case resolves as designed: a basic reader creates a working
  rule from the essential fields, the advanced reader tunes the rest, and a
  rule someone tuned shows its tuning to everyone (4).
- Plugins get progressive disclosure for free: tag a field `advanced`, tag a
  section `mode`, done. No plugin owns a toggle, a cookie, or a filter.
- The simplified-face pairing (2) gives pages a way to speak task language
  in basic mode without duplicating facts — the mode guarantees exactly one
  face renders.
- No parallel URL trees, no per-page toggles, no vestigial prefixes: one
  tree, one render path, filtered late.

### Costs / negatives

- Every page owner inherits an editorial decision per section and field:
  which mode does this belong to? The five-fields-that-work line is a
  product judgment nobody can automate, and a lazy tagging job produces
  either a noisy basic mode or a gutted one.
- The capability-not-state invariant (4) is a contract obligation the shell
  cannot fully police: the shell does not know a plugin's defaults, so a
  plugin that tags a live field `advanced` ships a basic mode that
  misrepresents its rules. Review and plugin tests carry this, not a
  mechanical gate.
- Paired faces are two representations of one fact to keep in sync; a pair
  whose basic face drifts from its advanced face is worse than no pair.
- The manifest, envelope, and widget schema all gain a field, and the shell
  render path gains a filter that every widget container must respect
  (pruning a section must prune its children, empty sections must not leave
  orphaned headings).

### Neutral

- Where the switch sits in the chrome, its labels, and its look are design
  decisions; this ADR fixes only that the shell owns exactly one.
- The mode names are user-facing vocabulary ("Basic", "Advanced") and ride
  the normal i18n path (ADR-012).
- A page may legitimately have no basic content beyond its heading (a purely
  diagnostic page tagged `advanced` end to end); the mode system neither
  requires nor forbids a basic face — it only requires honesty about state.

## Alternatives considered

- **Mode as a page property (each page picks its own rendering).** The
  landing page renders basic or advanced; other pages decide for
  themselves. Rejected: expertise describes the reader, not a page. A
  per-page choice fragments into per-page toggles the reader must re-make on
  every visit, and navigation — which belongs to no page — still needs an
  answer.
- **Parallel UI trees (`/basic/…`, `/advanced/…`).** LuCI's historical
  answer (`admin`/`mini`). Rejected: two trees mean every page exists twice,
  drift is structural, and the dead tree leaves vestiges (ADR-009 found
  exactly this fossil in LuCI). One tree, filtered at render, cannot drift.
- **A separate advanced area of the navigation instead of a mode.** Group
  the technical sections under a collapsible seam in the sidebar and let
  everyday rows stand in front. Rejected as the *whole* answer: it
  discloses navigation only. The basic reader still lands on pages whose
  sections and fields need the same basic/advanced judgment, so a second
  mechanism would be required inside pages — at which point the app-wide
  mode subsumes the seam and one mechanism serves both.
- **Mode in `/etc/config/verso` (ADR-013).** One store for all Verso
  settings is attractive. Rejected: mode is reader preference, not device
  intent — it must differ per browser, and it must not pass through the
  stage (1).
- **Client-side filtering (render everything, hide via CSS/JS by mode).**
  Rejected: it ships the advanced DOM to every reader on every page,
  contradicts the server-rendered model (ADR-004), and still needs the
  server-side declarations anyway for the state invariant (4) — so it buys
  only weight.
- **Booleans instead of the enum (`basic_only`/`advanced_only`).** Rejected:
  the two booleans admit a contradictory both-set state and hide that the
  three legal values are one mutually-exclusive choice.
- **Per-field enum (symmetry with sections).** Give fields the same
  three-state `mode`. Rejected for now: no field has a simpler face — a
  simplified representation is structural, which is what sections are for —
  and the boolean states exactly what a field can be. The upgrade path to
  the enum is mechanical if the case appears (3).
