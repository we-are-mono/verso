# Design

## Context

The shell draws every page from the widget schema (ADR-005): plugins send sections, forms, tables and action bars, and the shell's templates and stylesheets decide how they stand. `sections.css` already treats a section as one component whose air, rule and reach depend on the surface it stands on (a page or a drawer), and `page.html.tmpl` puts every page but the Overview, the light pages (Logs, Firewall › Activity, Diagnostics) and a full-width page on `.verso-notebook` under a Quiet Sand masthead bar. The Overview's cards (ADR-022) are their own templates in `overview.html.tmpl`.

The markup of the pages mocked (Devices, Firewall settings, Access, Packages) already has the right shape for cards: a page form's sections are siblings, a per-section form lives inside its section, a page-wide form's Save and configuration preview stand after its last section, and a listing's action bar stands right before its table. So the change is mostly in how those blocks are drawn, not in what is rendered.

## Goals / Non-Goals

**Goals:**
- Every page but the Overview and the live pages (Logs, Firewall › Activity, Diagnostics) reads as cards on Quiet Sand, per the `page-cards` spec.
- The live pages render exactly as they do now.
- No widget schema change: every installed plugin page turns into cards with the shell upgrade alone.
- Drawers render exactly as they do now.

**Non-Goals:**
- Fine-tuning padding and margins beyond the values the mocks settled. The owner will do that in a separate spec.
- Collapse, arrange or remembered layout outside the Overview.
- Status words in page card headers. A section has no status field, and adding one is a schema change.
- Changing the Overview.

## Decisions

### A page section becomes a card in the stylesheet, not in a new widget
A top-level page section with a heading band (`section[data-verso-section]:has(> [data-verso-section-band])`, not inside `dialog` and not inside another section) gets the card's frame, Paper fill and corners in `sections.css`. Its band becomes the header: 15px over and 14px under the title's 20px line, 20px in, at the Overview's card-title size (16px/600). The hairline under the band and the 20px body inset come from the same rules. Inside a card the reach variables (`--verso-reach-start`, `--verso-reach-end`) are 20px, so every rule that reaches past content stops at the card's edges, as in a drawer.

Alternative considered: a `PageCard` widget the page composers wrap sections in. Rejected: every plugin would have to adopt it, and the shell already knows what a section is and where it stands.

The section template adds one thing: a top-level section's `Icon` (already a schema field, used today by parts' ledger lines) leads its band's title, in Glyph. A section without an icon shows no glyph.

### The page frame: sand ground, no grid, no masthead bar
`page.html.tmpl` drops `.verso-notebook` and gives the page body `bg-quiet` on every page that is not the Overview, a light page or a full-width page. The light pages (`pageHeader.Light`: Logs, Firewall › Activity, Diagnostics) keep their branch of the template and their CSS untouched. None of them stands on the grid today, so deleting it does not reach them. The masthead keeps its element (the page acts' measure hangs off it) but loses its fill and hairline. That change is scoped to `[data-verso-masthead]:not([data-verso-masthead="light"])`, because the light masthead inherits the base rule's hairline and reach today. Its heading line gets 26px over and under it (`py-6.5`, rem), and the content's 40px top margin goes, so the first card starts right under it. `.verso-notebook`, its `::after` grid, its anchor rules and the dev session's G shortcut (`verso-dev.js`, `data-verso-grid-full`) are deleted.

### One listing is one card; its action bar is the card's top strip
A page-level listing, meaning a page stack holding an action bar and a table scroller or just a scroller, is framed as one untitled card by the stylesheet. The action bar inside it loses its Mid Sand box and becomes a strip on Paper with a hairline under it, 13px over its 34px controls and 12px under. Rows run to the card's edges with a 20px leading inset, and the last row's hairline gives way to the frame.

### Lanes become one card each, in the template
A laned table cannot be split into framed cards by CSS without losing repeated column heads. `table.html.tmpl` therefore renders a laned table as one `<table>` per lane, each inside its own card, sharing the listing's `colgroup` and `thead`, with `table-layout: fixed` so every lane's columns line up. The lane's band becomes the card's header (label and chain on the left, the tally and the lane's add act at the end). Only a listing that is one of the page's own blocks splits: the shell marks those as it renders the page (`markPageBlocks`), so a laned listing inside a section (Hardware's sensors) or a drawer keeps its lanes as rows of one table. Listings whose fixed widths starve the first column get narrower fixed columns from the closed measure set. Devices: Now takes the name measure (14rem, from the mock); This month and Status keep the word measure (9rem, a rem over the mock's 8, since the set has no 8rem step). The reorder column's head and cells are sized to their padding and grip whole, which a fixed layout needs. A fixed layout gives a growing column only what the fixed ones leave, so each lane's table has a floor (`laneMinWidth`: the measures plus 12rem per growing column) and scrolls inside its card below it. On a phone a card page takes the Overview's 20px side gutter, since the card's own 21px inset on top of the 40px frame left too little room for a reading row (Hardware) or a lane's measures.

This breaks the scripts' assumption that a listing is one table, in five places in `verso-tables.js` (the fifth is the reorder's announced sequence, which must hold every lane's rows because the file holds one order): the filter's row walk, a listing scope's `querySelector("table")`, the live inventory refresh that swaps one `tbody`, and the scroll-edge boxes. Each walks every table in the listing instead; a laned listing's live refresh swaps its lanes whole, since their headers count what is in them, and each lane's scroller measures its own table. Dragging already works per lane (`ReorderGroup`), so a drag stays inside one card.

Alternative considered: keep one table and draw lane frames with cell borders. Rejected: the column heads would stand once above the first card, and the borders on `border-collapse: separate` cells are fragile against seams and hover washes.

### Forms keep their markup
A per-section form already sits inside its section, so its Save is at the card's foot with no change. A page-wide form's Save already stands after its last section, so it is on the sand. The configuration preview (`[data-verso-code-divider]`) loses its divider hairline and gets the card frame; its file strip becomes the card's header.

### Self-framed blocks drop their frame inside a card
The `artifact` card style (the certificate) loses its border, fill and side padding when it stands inside a page card, through one stylesheet rule. Its title and sentence stay as they are.

### The exposure warning moves into the Web interface section
`password.go` appends the exposure callout as the first child of the web settings section, not as a page block before it.

### ADRs and docs
ADR-022's consequence "The Overview is the one page with cards, and the one exception to the flat direction" is amended in place to say every page is cards and the Overview's are the ones that arrange. A new `docs/ADR/023-pages-are-cards.md` records why: a different look on every page but home, the four mocks, and the choices made (no band, no grid, drawers unchanged, no collapse). `DESIGN.md` is rewritten where it describes the notebook grid, the masthead, section bands on pages, the control band, laned tables, and the flat direction.

## Risks / Trade-offs

- **The 20px grid stops ruling page edges.** The heading block is 86px, not whole cells. The 20px rhythm stays inside cards. DESIGN.md's grid rules move from "the page" to "inside a card".
- **Fixed column widths on laned listings** can squeeze a long name. Each laned listing is checked in the dev container, and its widths are adjusted where a name truncates.
- **Hand-built pages** (Maintenance and anything else composing its own sections) only turn into cards if they use the same section markup. Each is checked; a page that does not is converted to the section markup, not styled separately.
- **Third-party plugins** that put a heading-less section or raw blocks at the top level render as plain blocks on the sand. That is acceptable: the change is visual, and the shell still draws everything the plugin sent.
- **Selectors in `sections.css` grow.** The card rules are scoped to `main` and exclude `dialog`, so drawers cannot pick them up.
