# ADR-012 — Flat tables: one hairline listing, no stripes, inert rows

- **Status:** Accepted
- **Date:** 2026-08-27
- **Deciders:** tomaz@zaman.io
- **Relates to:** ADR-005 (widget chrome is shell-owned; a restyle lives in the
  shell templates, so plugins need no change), ADR-011 (the services page already
  chose a hairline "lined" table over stripes — this generalises that choice).

## Context

Listings across Verso rendered as **zebra-striped** rows (`odd:bg-slate-50`), and
an interactive table made the **whole row** the click target: the row was wrapped
so clicking anywhere opened its drawer. Two problems followed. The stripe is
decoration that carries no meaning, and it competes with the data. And a
whole-row click steals text selection — a person cannot select or copy an
address, MAC, or port out of a row, because the row is a button.

The reference "Connected devices" design settled the target: a calm listing with
a header that reads as the table's own top row, hairline rules between rows, and
values that stay selectable — with any "more" behind an explicit link, not a
whole-row click.

## Decision

There is one table look, and it is the default (`Style: ""`):

- **No stripes.** Rows divide with a hairline (`border-b border-slate-200`). The
  zebra path is removed from the `table`, `settings`, and `properties` widgets;
  the `properties` `"striped"` style is retired and falls back to the hairline
  default.
- **Rows are inert.** No whole-row click, no hover tint — cell values stay
  selectable and copy-pastable. A row that has more behind it exposes a trailing
  **"Details"** link (hard-right), the only click target in the row; it opens the
  same drawer the whole row used to.
- **Flush edges.** The first column aligns to the table's left border, the last to
  the right (`first:pl-0 last:pr-0`).
- **Optional header band.** A top row aligned to those same edges: a title, an
  optional detail (a count/summary), and an optional right-aligned link. A table
  with no column labels draws no `<thead>`.
- **Condensed variant.** `Condensed` lowers row padding only — same anatomy.
- **`lined` and `card` remain** as frame variants (inset hairlines; the boxed
  card), and adopt the same inert-row model.

The shell owns all of this (ADR-005). Plugins emit `Style: ""` and inherit the
flat look with no code change.

## Consequences

- Every existing table, settings block, and properties list loses its stripe and
  becomes non-clickable in one shell change; the interactive ones grow a "Details"
  link where the row click used to be.
- The whole-row `showFromRow` interaction is no longer used by tables. The
  connected-devices roster (a `row`+`drawer` list, not a `table`) still opens from
  the whole row; converting it to a flat `table` is separate work, gated on which
  device facts the box can actually report.
- "Details" is one word everywhere, including where the drawer is an edit form; a
  per-context label ("Edit") can be added later without changing the model.
