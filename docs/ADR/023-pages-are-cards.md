# ADR-023 — Pages are cards

- **Status:** Accepted
- **Date:** 2026-10-13
- **Deciders:** tomaz@zaman.io
- **Relates to:** ADR-005 (the closed widget set: the shell decides how a
  section is drawn), ADR-022 (Overview cards)

## Context

The Overview stands Paper cards on Quiet Sand (ADR-022). Every other page was
a white notebook sheet: a visible 20px grid, a masthead bar with a hairline
under it, sections ruled off by hairlines running out to the rail, and a Mid
Sand box over a listing for its search and cuts. Opening the app on the
Overview and then any other page read as two different apps.

Four mocks put the Overview's look on the other kinds of page: Devices (a
laned listing), Firewall settings (a page-wide form), System › Access
(per-section forms holding tables, collections and a certificate) and System ›
Packages (a listing with a search and a cut). The card look carried every one.

## Decision

### The page is sand, its subjects are cards

Every page but the live ones stands on Quiet Sand with no grid drawn. Each
titled section of a page is a card: Paper in a single Strong Hairline frame
with 2px corners, its band the card's header (the section's glyph when it has
one, the 16px title, its metadata and control on the title's line), its body
20px in behind a hairline. Cards stand 20px apart in one column at the page's
measure. A page form's untitled run of fields is the same card with no header,
so no field stands loose on the sand. A part inside a section stays on its
ledger line inside its section's card, and a component that frames itself (the
certificate) gives up its frame inside a card: no card holds a card.

### No masthead bar

The heading line (the h1 and the page's acts) stands straight on the sand,
26px under the top bar and 26px over the first card. A bar over sand cards
read as one more card; without it the cards start where the bar's hairline
stood.

### Listings are cards too

A page that is one listing is one card with no header, because the page's
heading names it. Its control band is the card's top strip, on the card's
Paper behind a hairline, 13px over its controls and 12px under; the Mid Sand
box is gone. A laned listing that is one of the page's own blocks (Devices by
network, Firewall rules by chain, Routes by family) is a card per lane: the
lane's name is the card's header and the card repeats the column heads. The
lanes' tables are laid out fixed from the listing's one set of column measures,
so every lane's columns line up. A laned listing inside a section or a drawer
keeps its lanes as rows of one table, since a card does not hold cards.

### Where Save stands

A form that saves one section keeps its Save inside that section's card. A
form that saves the whole page stands its Save on the sand under the last card,
with no rule; its configuration preview is a card of its own above it. A
warning about one card's subject opens that card's body.

### What stays

- **Drawers** keep their own layout: a drawer is a card of its own, and its
  sections keep their bands and rules.
- **Live pages** (Logs, Firewall › Activity, Diagnostics) keep the light
  masthead on Paper with its hairline and their output flush under it: a log
  has no subjects to frame, only a stream.
- **Page cards neither fold nor move.** Arranging is the Overview's, where an
  operator chooses what to glance at first; a settings page's order is the
  order its subjects are read in.
- **No schema change.** Plugins emit the same sections, tables and forms, and
  the shell draws them as cards (ADR-005).

## Consequences

- Every page reads as the Overview's app; DESIGN.md describes the sand ground,
  the cards, the control strip and lane cards.
- Inside a card, the 20px cell still sets every measure; the page itself is
  measured in the sand, not in cells.
- A laned page listing renders a table per lane, so the listing's scripts read
  every lane: the reorder announces the whole listing's order, a cut puts away
  an emptied lane, and the live refresh swaps the lanes whole.
- A column's measure caps it in every lane; a listing whose first column is
  starved gets narrower fixed columns from the closed measure set.
