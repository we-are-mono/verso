# ADR-005 — UI consistency contract: design tokens, a closed widget set, a governed "raw" bridge

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
   spacing, font). Authors express intent; Verso maps intent → design tokens → pixels.
   Example: `{"type":"badge","variant":"success","text":"up"}` — never a color.
2. **Design tokens are the rendering substrate.** Everything — every widget and the raw
   bridge — resolves through Verso's CSS-custom-property token layer. That layer *is*
   the consistency contract; a theme swaps token values, markup stays fixed.
3. **Closed widget set, open composition.** Verso defines and exposes a fixed set of
   reusable elements (card, table, form, badge, …). Plugins compose and nest them; they
   cannot add widget types or styles. Consistency is enforced by construction — plugins
   emit schema, never markup or CSS.
4. **A governed "raw" bridge.** When no widget fits, an author may emit a `raw`
   element: **display-only**, **Markdown** (not HTML/CSS), rendered through Verso's
   tokens and sanitizer. It grants content freedom, never appearance control.
   Interactivity (inputs, forms) is never available in raw.
5. **Raw stays a bridge by mechanism, not goodwill.** `raw` is an explicit type,
   rendered with a visible "raw" affordance, and instrumented — its usage is the demand
   signal for the next widget. Lifecycle: author ships raw → Verso ships the widget →
   author migrates. Health metric: raw usage *declines* for recurring needs.
6. **Tailwind v4 is the sanctioned way to author the tokens later** — an engine change,
   not a contract change. Because plugins never see classes, the token-authoring
   approach is reversible with zero plugin impact; hand-authored token CSS is the
   starting point.

## Consequences

### Positive
- Consistency by construction: an author physically cannot express an off-brand look,
  and the one deliberate leak (raw) is still Verso-styled.
- LuCI's escape-hatch failure is designed out: raw is constrained, temporary and
  instrumented, so it stays a bridge instead of becoming the norm.
- The "schema won't be expressive enough" risk becomes a roadmap input: raw usage tells
  us which widget to build next.
- CSS is entirely shell-internal and swappable (hand tokens now, Tailwind v4 later)
  without touching a single plugin.

### Costs / negatives
- Verso must invest in a genuinely complete token system up front — it is the substrate
  everything (raw included) renders through, so gaps show everywhere.
- Any author need not yet covered forces a raw stopgap until a widget lands; if the
  widget roadmap lags, raw accretes. The instrumentation and the visible affordance are
  what hold this in check — they are not optional extras.
- Semantic-only props mean Verso owns every visual decision; an author with a strong
  design opinion cannot express it (by design).

### Neutral
- Governs the visual/consistency model only. Manifest, socket protocol and schema
  versioning live in the mechanical plugin-contract ADR.

## Alternatives considered

- **Unconstrained raw (HTML/CSS/JS), à la LuCI** — maximum author reach. Rejected: it
  destroys consistency (demonstrated by LuCI), reopens XSS into the shell's origin, and,
  being easier than widgets, becomes the default rather than the exception.
- **No escape hatch at all** — purest consistency. Rejected: authors would be blocked
  whenever the widget set lags, which kills adoption; a governed bridge is the pragmatic
  middle.
- **Presentational props on widgets** (authors pass colors/spacing) — more flexibility.
  Rejected: it is inconsistency by another name; two plugins would diverge immediately.
