# ADR-005 — UI consistency contract: Tailwind's palette, a closed widget set, a governed "raw" bridge

- **Status:** Accepted
- **Date:** 2026-08-23
- **Deciders:** tomaz@zaman.io
- **Relates to:** ADR-001 (single static binary), ADR-004 (frontend stack). The
  *mechanical* plugin contract (manifest format, socket protocol, schema versioning)
  is a separate, later ADR — this one governs only the visual/consistency model.

## Context

Verso's value is that third-party plugin pages look native and stay consistent across
the whole end-user experience, no matter who wrote them. Studying LuCI showed the
failure mode to avoid: its raw escape hatch was unconstrained and permanent, authors
fled to it because it was easier than the structured layer, and the "consistent UI"
evaporated. The goal is extensibility **without** letting authors invent their own
looks — while still offering a legitimate bridge when no widget yet fits.

## Decision

1. **Semantic vocabulary, not presentation.** The widget schema exposes *semantic*
   props (`variant`, `size`, `label`, `datatype`), never *presentational* ones (color,
   spacing, font). Authors express intent; Verso maps intent → Tailwind's palette → pixels.
   Example: `{"type":"badge","variant":"success","text":"up"}` — never a color.
2. **The shell owns all appearance, via Tailwind's palette.** Plugins emit semantic
   schema; the shell renders it to HTML styled with Tailwind's default palette — `sky`
   for accent, `slate` for neutrals, `red`/`green`/`amber` for states. The consistency
   contract is that a plugin *never expresses appearance*, not any particular colour
   system; the palette is the shell's alone to change, across the whole UI at once.
3. **Closed widget set, open composition.** Verso defines and exposes a fixed set of
   reusable elements (card, table, form, badge, …). Plugins compose and nest them; they
   cannot add widget types or styles. Consistency is enforced by construction — plugins
   emit schema, never markup or CSS.
4. **A governed "raw" bridge.** When no widget fits, an author may emit a `raw`
   element: **display-only**, **Markdown** (not HTML/CSS), rendered through Verso's
   styling and sanitizer. It grants content freedom, never appearance control.
   Interactivity (inputs, forms) is never available in raw. Raw is for **prose** —
   running text no widget shapes. An outcome notice, a machine value, or an empty
   state rendered through raw is a defect, not a style choice: each has an owner
   (the envelope's `notice` channel — ADR-006 §4 — `code`/`properties`, and
   `empty`).
5. **Raw stays a bridge by mechanism, not goodwill.** `raw` is an explicit type,
   rendered with a visible "raw" affordance, and instrumented — its usage is the demand
   signal for the next widget. Lifecycle: author ships raw → Verso ships the widget →
   author migrates. Health metric: raw usage *declines* for recurring needs.
6. **Tailwind v4, built on its default palette.** The templates use Tailwind's
   default utility classes as the base, over a small custom `@theme` — a few semantic
   tokens (`--color-canvas`, `--color-surface-subtle`) and a dark-mode palette that remaps
   the neutrals and state tints under `:root.dark`. The stylesheet compiles via the
   standalone CLI (no Node) to an embedded file. This is an engine choice, not a contract
   change: plugins never see classes, so the styling stays reversible with zero plugin
   impact. Shell templates use utilities; plugins never do.
7. **Behaviour is declared as intent; the shell realizes it.** The closed widget set
   includes *behavioural* widgets — the first is `repeater`, a repeatable group of
   widgets backed by a set of uci sections — but they obey the same rule as every other
   widget: the plugin declares *intent* ("this group repeats"), never a mechanism and
   never client code. The shell owns the add/remove affordances and the realization.
   The realization is a re-render **round-trip** — the affordance POSTs, the shell
   re-renders — and for a uci-backed repeater the shell performs the section
   `add`/`delete` itself through rpcd, within the plugin's declared write scope
   (ADR-007), then re-renders from a fresh read; the plugin stays pure schema and writes
   no add/remove logic. Because the plugin declared only intent, the realization is
   **swappable**: a hot case can become instant client-side later with no plugin change
   — the bet is on the vocabulary, not the transport. This keeps the `raw` bridge
   (point 4) firmly display-only: behaviour has a governed home, so it never leaks
   through raw. A second behavioural widget, `conditional`, gates a field-set on a
   controlling toggle; it follows the same rule, but its realization is *pure CSS*
   (`:has()`) rather than a round-trip, because show/hide has no state to persist —
   the shell still owns every line of it, and the plugin still only declares the
   intent. Behavioural widgets whose interaction never touches the server are realized
   without a round-trip: `modal` and `drawer` in **shell-owned client JS** (Alpine's CSP
   build, ADR-004) — the plugin emits only `{type:"modal", …}` and the shell owns the
   open/close, focus-trap, and transition — while `tabs` needs no JS at all, a pure-CSS
   radio group (`:has()`) driving which panel shows. Pure CSS, a round-trip,
   and shell JS are all just realizations of a declared intent.
   A realization is chosen per widget for what it does — the repeater's rpcd
   round-trip, the conditional's CSS, and a form's secondary *action*, which submits
   the form for the **plugin** to compute on and re-render (generating a keypair); the
   invariant is that the plugin declares intent and the shell owns the behaviour. The
   round-trip's
   *mechanics* are the mechanical contract's (ADR-006); what a plugin may *emit* is
   this ADR's.

## Consequences

### Positive
- Consistency by construction: an author physically cannot express an off-brand look,
  and the one deliberate leak (raw) is still Verso-styled.
- LuCI's escape-hatch failure is designed out: raw is constrained, temporary and
  instrumented, so it stays a bridge instead of becoming the norm.
- The "schema won't be expressive enough" risk becomes a roadmap input: raw usage tells
  us which widget to build next.
- CSS is entirely shell-internal and swappable — the palette is the shell's to change —
  without touching a single plugin.
- The static-schema bet survives a *behavioural* page (the WireGuard plugin's
  add/remove peer): the escape hatch for behaviour is a shell-realized behavioural
  widget declaring intent, not plugin code — and because the bet is on the vocabulary,
  the realization (round-trip now, client-side later) is swappable behind it.

### Costs / negatives
- Appearance is tied to the shell's Tailwind palette. Building on Tailwind's defaults
  keeps the token layer small — a few semantic tokens plus the dark-mode remap — rather
  than a full bespoke system, and plugins carry none of it.
- Any author need not yet covered forces a raw stopgap until a widget lands; if the
  widget roadmap lags, raw accretes. The instrumentation and the visible affordance are
  what hold this in check — they are not optional extras.
- Semantic-only props mean Verso owns every visual decision; an author with a strong
  design opinion cannot express it (by design).

### Neutral
- Governs the visual/consistency model only. Manifest, socket protocol and schema
  versioning live in the mechanical plugin-contract ADR — as does the round-trip that
  realizes a behavioural widget (point 7).

## Alternatives considered

- **Unconstrained raw (HTML/CSS/JS), à la LuCI** — maximum author reach. Rejected: it
  destroys consistency (demonstrated by LuCI), reopens XSS into the shell's origin, and,
  being easier than widgets, becomes the default rather than the exception.
- **No escape hatch at all** — purest consistency. Rejected: authors would be blocked
  whenever the widget set lags, which kills adoption; a governed bridge is the pragmatic
  middle.
- **Presentational props on widgets** (authors pass colors/spacing) — more flexibility.
  Rejected: it is inconsistency by another name; two plugins would diverge immediately.
- **Prebuilt client-side behavioural widgets as the foundation** (rather than a
  re-render round-trip) — snappier. Rejected as the *foundation*: it grows an unbounded
  catalogue of bespoke client behaviours and still cannot express an interaction no
  widget covers. It returns as an *optimization* of the same declared intent (point 7),
  not a fork to be chosen once and be stuck with.
- **Plugins ship JS/HTML for behaviour** — maximum reach. Rejected for the same reason
  as unconstrained raw: it dissolves crash isolation and the token-enforced look the
  whole model exists to guarantee.
