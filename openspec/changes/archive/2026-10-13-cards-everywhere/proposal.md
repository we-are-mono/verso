# Proposal

## Why

Every page outside the Overview reads as a different app from the one it opens on: the Overview stands Paper cards on Quiet Sand, while every other page is a white notebook sheet with a grid, a masthead bar and hairline-ruled sections. Mocks of Devices, Firewall settings, System › Access and System › Packages showed that the card look carries every kind of page (a laned listing, a page-wide form, per-section forms with tables and collections, a filtered listing). The owner chose it for the whole app.

## What Changes

Seen by the operator: every page looks like the Overview. The page ground is Quiet Sand, each subject of a page stands in its own Paper card with a header, and the notebook grid and the masthead bar are gone.

- Every page but the Overview and the live pages (Logs, Firewall › Activity, Diagnostics) stands on Quiet Sand with no visible grid. The masthead bar and its hairline are gone: the heading line (h1 and page acts) stands straight on the sand, 26px over and under it, and the first card starts right under it.
- Each ruled section of a page renders as a card: Paper, a single Strong Hairline frame, 2px corners, a header with the section's glyph (when it has one) and its title, the body behind a hairline. Cards stand 20px apart in one column, at the page's measure.
- A listing of lanes (Devices by network, Firewall rules by zone pair) renders one card per lane, its lane head becoming the card's header. A page whose body is one listing renders it as one card with no title of its own.
- The control band (search and counted cuts) leaves the page and becomes the top strip of its listing's card, on the card's Paper, behind a hairline. The Mid Sand box is gone.
- A form that saves one section keeps its Save inside its card, at the foot. A form that saves the whole page stands its Save on the sand under the last card.
- Cards on pages other than the Overview do not collapse and cannot be arranged: no chevron, no grip.
- A component that draws its own frame (the `artifact` card style, as the HTTPS certificate uses) drops that frame inside a page card, so no card holds a card; its title and sentence become the card's lede.
- The live pages (Logs, Firewall › Activity, Diagnostics) stay exactly as they are: the light masthead on Paper with its hairline, their output flush under it.
- Drawers are unchanged: a drawer is already a card of its own.
- No widget schema change: plugins keep emitting sections, tables and forms, and the shell draws them as cards.

ADRs touched: **ADR-022** (Overview cards) is amended in place where it calls the Overview "the one page with cards, and the one exception to the flat direction". A new **ADR-023 (Pages are cards)** keeps the reasons for the move. The change rests on **ADR-005** (UI consistency contract): the shell, not the plugin, decides how a section is drawn.

## Capabilities

### New Capabilities
- `page-cards`: how every page outside the Overview lays out: the sand ground with no visible grid, the heading line without a masthead bar, sections as cards, laned and single listings as cards, the control strip inside a listing's card, where a form's Save stands, which pages stay flush, and how self-framed components behave inside a card.

### Modified Capabilities
None. The Overview's requirements (`overview`) are unchanged; only ADR-022's prose about the Overview being the sole card page changes.

## Impact

- **Shell templates:** `internal/server/templates/page.html.tmpl` (ground, masthead, grid class), `internal/widget/templates/section.html.tmpl`, `table.html.tmpl` (lanes), `actionbar.html.tmpl`, `card.html.tmpl` (`artifact` style), `form.html.tmpl` (Save placement), the hand-built pages that compose sections (Maintenance).
- **Styles:** `internal/server/assets/input.css` (`.verso-notebook`, the masthead rules, the control band), `sections.css` (a page section becomes a card surface; the rule and band reach no longer apply on pages).
- **Docs:** `DESIGN.md` (Layout, the notebook grid, Section headings, Control Band, Tables, Overview cards, the flat direction), `docs/ADR/022-overview-cards.md`, new `docs/ADR/023-pages-are-cards.md`, `docs/BACKLOG.md`.
- **Plugins:** none rebuilt; every first-party plugin page is checked in the dev container.
- **Beta:** nothing a router holds changes, so no migration.
