---
name: Verso
description: The web admin for OpenWrt routers, kept like an operator's notebook — calm, exact, alive.
colors:
  ground: "#faf9f7"
  canvas: "#ffffff"
  on-fill: "#ffffff"
  quiet: "#f5f2ec"
  mid: "#ece8e0"
  rule: "#e4e0d8"
  rule-strong: "#d2ccc2"
  inert: "#c9c3b8"
  sand-5: "#c1bab0"
  faint: "#a09a8e"
  glyph: "#8b857a"
  meta: "#6f6a60"
  body: "#5c574f"
  ink: "#1b1917"
  denim: "#3568a8"
  denim-deep: "#285184"
  denim-line: "#cbd8e6"
  denim-soft: "#e8eef5"
  green: "#0e9b6e"
  green-deep: "#066b4b"
  green-line: "#b3ddcb"
  green-soft: "#e4f2ec"
  marigold: "#f0a419"
  marigold-deep: "#96590a"
  marigold-line: "#f5d494"
  marigold-soft: "#fdf0d4"
  crimson: "#d63b3b"
  crimson-deep: "#9e2222"
  crimson-line: "#f0c2c2"
  crimson-soft: "#fbe9e9"
  amethyst: "#8b52be"
  amethyst-deep: "#6d3d99"
  amethyst-line: "#e3d1f0"
  amethyst-soft: "#f4ebfa"
typography:
  headline:
    fontFamily: "Hanken Grotesk, ui-sans-serif, system-ui, sans-serif"
    fontSize: "1.5rem"
    fontWeight: 600
    lineHeight: 1.1
    letterSpacing: "-0.025em"
  title:
    fontFamily: "Hanken Grotesk, ui-sans-serif, system-ui, sans-serif"
    fontSize: "1.125rem"
    fontWeight: 600
    lineHeight: 1.4
    letterSpacing: "-0.025em"
  section-heading:
    fontFamily: "Hanken Grotesk, ui-sans-serif, system-ui, sans-serif"
    fontSize: "1.125rem"
    fontWeight: 600
    lineHeight: 1.25
    letterSpacing: "-0.025em"
  body:
    fontFamily: "Hanken Grotesk, ui-sans-serif, system-ui, sans-serif"
    fontSize: "0.875rem"
    fontWeight: 400
    lineHeight: 1.5
  lede:
    fontFamily: "Hanken Grotesk, ui-sans-serif, system-ui, sans-serif"
    fontSize: "1rem"
    fontWeight: 300
    lineHeight: 1.5
  label:
    fontFamily: "Hanken Grotesk, ui-sans-serif, system-ui, sans-serif"
    fontSize: "0.75rem"
    fontWeight: 500
    lineHeight: 1.33
    letterSpacing: "0.08em"
  value:
    fontFamily: "Inconsolata, Menlo, SFMono-Regular, monospace"
    fontSize: "1rem"
    fontWeight: 500
    lineHeight: 1.5
  figure:
    fontFamily: "Inconsolata, Menlo, SFMono-Regular, monospace"
    fontSize: "2.25rem"
    fontWeight: 700
    lineHeight: 1.1
    letterSpacing: "-0.05em"
rounded:
  mark: "1px"
  xs: "2px"
  full: "9999px"
spacing:
  xs: "4px"
  sm: "8px"
  md: "16px"
  group: "24px"
  page: "40px"
components:
  button-primary:
    backgroundColor: "{colors.denim}"
    textColor: "{colors.on-fill}"
    typography: "{typography.body}"
    rounded: "{rounded.xs}"
    padding: "0 16px"
    height: "32px"
  button-primary-hover:
    backgroundColor: "{colors.denim-deep}"
    textColor: "{colors.on-fill}"
  button-secondary:
    backgroundColor: "transparent"
    textColor: "{colors.ink}"
    typography: "{typography.body}"
    rounded: "{rounded.xs}"
    padding: "0 16px"
    height: "32px"
  button-secondary-hover:
    backgroundColor: "{colors.quiet}"
    textColor: "{colors.ink}"
  button-danger:
    backgroundColor: "{colors.crimson}"
    textColor: "{colors.on-fill}"
    rounded: "{rounded.xs}"
    padding: "0 16px"
    height: "32px"
  button-danger-hover:
    backgroundColor: "{colors.crimson-deep}"
  button-danger-back:
    backgroundColor: "transparent"
    textColor: "{colors.crimson-deep}"
    typography: "{typography.body}"
    rounded: "{rounded.xs}"
    padding: "0 12px"
    height: "32px"
  button-danger-back-hover:
    backgroundColor: "{colors.crimson-line}"
  button-caution:
    backgroundColor: "{colors.marigold-deep}"
    textColor: "{colors.on-fill}"
    rounded: "{rounded.xs}"
    padding: "0 16px"
    height: "32px"
  confirm-caution:
    backgroundColor: "{colors.marigold-soft}"
    textColor: "{colors.marigold-deep}"
    typography: "{typography.body}"
    rounded: "{rounded.xs}"
    padding: "16px"
  confirm:
    backgroundColor: "{colors.crimson-soft}"
    textColor: "{colors.crimson-deep}"
    typography: "{typography.body}"
    rounded: "{rounded.xs}"
    padding: "16px"
  button-row:
    backgroundColor: "transparent"
    textColor: "{colors.ink}"
    rounded: "{rounded.xs}"
    padding: "0 12px"
    height: "28px"
  input:
    backgroundColor: "{colors.canvas}"
    textColor: "{colors.ink}"
    typography: "{typography.body}"
    rounded: "{rounded.xs}"
    padding: "0 12px"
    height: "32px"
  chip:
    backgroundColor: "{colors.quiet}"
    textColor: "{colors.meta}"
    typography: "{typography.value}"
    rounded: "{rounded.xs}"
    padding: "1px 6px"
  chip-accent:
    backgroundColor: "{colors.denim-soft}"
    textColor: "{colors.denim-deep}"
    rounded: "{rounded.xs}"
    padding: "1px 6px"
  control-band:
    backgroundColor: "{colors.ground}"
    rounded: "{rounded.xs}"
    padding: "15px 24px"
  section-band:
    backgroundColor: "transparent"
    textColor: "{colors.ink}"
    typography: "{typography.section-heading}"
    padding: "0"
  table-row:
    backgroundColor: "{colors.ground}"
    textColor: "{colors.ink}"
    typography: "{typography.body}"
    padding: "10px 14px 9px"
    height: "40px"
  band:
    backgroundColor: "{colors.quiet}"
    textColor: "{colors.body}"
    typography: "{typography.title}"
    padding: "0 16px"
    height: "60px"
  tooltip:
    backgroundColor: "{colors.ground}"
    textColor: "{colors.ink}"
    typography: "{typography.body}"
    rounded: "{rounded.xs}"
    padding: "8px 12px"
  top-bar:
    backgroundColor: "{colors.canvas}"
    height: "56px"
  drawer:
    backgroundColor: "{colors.ground}"
    width: "760px"
---

# Design System: Verso

## Overview

**Creative North Star: "The Operator's Notebook"**

Verso is kept the way a careful operator keeps a notebook for the router they run: warm sand paper, hairline rules, entries written in a fixed hand. The page is the ground, the router writes onto it, and nothing on the page decorates. Machine facts (addresses, interface names, versions) are set in mono like entries copied down verbatim; the words around them are plain sans. Every page stands on Quiet Sand, its subjects on Paper cards, and every edge is measured on one 8px grid. State is carried by one small square, the packet, the unit a router handles, so the same mark means "up", "worth a look" or "down" everywhere it appears.

The mood is **calm, exact, alive**. Calm, because the resting state is silent and only failures earn words. Exact, because every value on screen maps one-to-one onto the config it came from and every measure repeats: an 8px grid, 40px rows, 32px controls. Alive, because calm is not grey. Live values glide, the waiting mark folds and unfolds, big numbers read at a glance, and colour appears the moment it means something. A page that reads as a dry grey box has missed the brief as badly as one that shouts.

The system is flat and dense enough for a power user, never cramped. Surfaces separate by tone and hairline, not by shadow. One colour, denim, means "act on this", and it is spent sparingly: one primary act per page.

**Key Characteristics:**
- Warm sand neutrals, eleven steps, each with a fixed job.
- One action colour (denim); four status hues used as marks, never as decoration.
- Two faces only: Hanken Grotesk for words, Inconsolata for machine strings.
- Flat surfaces, hairline borders, 2px corners, 1px for marks.
- Paper cards on a Quiet Sand ground, no grid drawn; an 8px grid every edge lands on, 40px rows, 32px controls.
- Each subject of a page is a card, its title in the card's header; in a drawer a section opens on a Hairline with its heading under it, unfilled.
- What narrows a listing is the top strip of the listing's card.

## Colors

A warm, near-neutral sand ground carries a single cool action blue and four status hues that only ever mark.

### Primary
- **Denim** (#3568a8): actions, focus and selection — the primary button, links, the selected tab underline, and selected form choices. Keep selection colour on the control itself so primary actions remain easy to find.
- **Deep Denim** (#285184): denim one step darker, for hover and focus rings.
- **Denim Hairline** (#cbd8e6) and **Denim Wash** (#e8eef5): the hairline and ground of the accent chip (network and zone references, static addresses) and of info bands.

### Secondary
- **Leaf Green** (#0e9b6e): up, healthy, accept — as a mark (state square, the accept chip's mark). **Deep Green** (#066b4b) carries green words on **Green Wash** (#e4f2ec).
- **Marigold** (#f0a419): worth a look — a real trade-off, not an alarm. **Deep Marigold** (#96590a) carries words on **Marigold Wash** (#fdf0d4), which is also the rail's Pending changes row.
- **Crimson** (#d63b3b): down, invalid, rejected; an errored field's border. **Deep Crimson** (#9e2222) carries error text on **Crimson Wash** (#fbe9e9).

### Tertiary
- **Amethyst** (#8b52be): the second series on a graph (upload). One job, and it is not a verdict.

### Neutral
- **Paper** (#faf9f7): the page and every card.
- **Canvas** (#ffffff): the one surface a step above the page — the top bar, input fills, a takeover's backdrop.
- **On-fill** (#ffffff): the words and the check on a filled control — the primary, danger and caution buttons, a checked choice, a picked filter. White, as Canvas is, but its own token: a surface and a label on a fill are two jobs. Tailwind's own white and black are unset, so `text-white` draws nothing.
- **Quiet Sand** (#f5f2ec): every quiet surface — the page ground under its Paper cards, a listing's lane bands, chips, row hover.
- **Mid Sand** (#ece8e0): the only mid ground — icon-button hover, the gateway card, the colophon.
- **Hairline** (#e4e0d8): hairlines inside a card, between rows.
- **Strong Hairline** (#d2ccc2): subsection ledger lines, the line under a lane, every input border.
- **Inert** (#c9c3b8) and **Parked** (#c1bab0): an off switch track, an empty port; the waiting mark's parked squares.
- **Faint** (#a09a8e) and **Glyph** (#8b857a): decorative and action icons. Never text.
- **Meta** (#6f6a60): kickers, column heads outside a page's cards, units, placeholders — the lightest ink that may carry words.
- **Body** (#5c574f): body copy and every label on a card.
- **Ink** (#1b1917): headings and values.

### Named Rules
**The One Voice Rule.** One primary denim button per page, on the heading line. Radios, checkboxes and multi-select chips use the same hue in their selected marks; this is a control state, not another primary action.

**The Mark Rule.** A hue at full chroma is a mark — a square, a border, a line, a fill behind white. It never sets type. Words in a hue use its deep step, on its own wash.

**The 4.5 Floor.** Nothing below 4.5:1 against Paper carries words. That rules out Faint and Glyph for text, and every bare hue.

### Themes
The palette above is **Sand light**. A theme (ADR-024) is a file of colours keyed by these token names, laid over it after the stylesheet; whatever it leaves out stays Sand light's. Everything a theme changes is a colour: a shadow's shape and every measure stay the shell's, and a translucent colour (a shadow, a wash) is an 8-digit hex.
- **Sand, following the browser** (the default): Sand light under a light preference, **Sand dark** under a dark one.
- **Sand, always light** and **Sand, always dark**: one of the two, whatever the browser prefers.
- **Any other theme** (Catppuccin Mocha ships beside Sand dark): fixed, whatever the browser prefers, its scheme declared so the browser's own controls follow.

Sand dark mirrors Sand's steps at the same warm hue, each token keeping its job: the page (Quiet Sand) is the darkest ground, cards (Paper) one step up, Canvas the slot a field is cut into, Ink the lightest. A hue's deep step, which carries words on its wash, climbs toward light instead of falling, so it still clears the 4.5 Floor; On-fill turns dark, since the fills it sits on are lighter. Shadows are black at 35–50%, and chart washes start at 25%. Every theme Verso ships is held to the floors by a test; any other theme's shortfalls are logged when it loads.

## Typography

**Display Font:** Hanken Grotesk (with ui-sans-serif, system-ui)
**Body Font:** Hanken Grotesk (with ui-sans-serif, system-ui)
**Label/Mono Font:** Inconsolata (with Menlo, SFMono-Regular)

**Character:** A friendly, open grotesk for every word, beside a narrow, even-pitched mono for every string the machine wrote. The split is semantic, not stylistic: you can tell a value from a label by its face alone.

### Hierarchy
- **Headline** (600, 1.5rem/24px, 1.1, -0.025em): every page's h1, on a heading line that is always a control's 32px tall so pages never shift between each other.
- **Title** (600, 1.125rem/18px, -0.025em): table and drawer titles, in Body ink on sand.
- **Section heading** (600, 1.125rem/18px, 1.25, -0.025em): a drawer's full-section h2s under the section's leading Hairline, in Ink. On a page the section's h2 is its card's title. Page titles, dialog titles and a graph's title retain their own roles.
- **Card title** (600, 1rem/16px, -0.025em): a card's name in its header, on the Overview and on every page, in Ink.
- **Sub-heading** (600, 0.875rem/14px): a section inside another section, as an h3 in Ink ("Authorized keys" under SSH), so a part of a subject never reads as a peer of its heading. It stands on a **ledger line**: the glyph of what the part is, when it has one (16px Lucide in Glyph: a network, a DHCP server, a bridge), then the name, then a Strong Hairline running on to the column's end, the line that stands between sections, so it reads as the start of a group and never as one more field label, and is told from the Hairlines between the rows under it. The ledger is one 24px line, and the part's first box stands 24px under it, as a form's rows stand apart.
- **Body** (400, 0.875rem/14px, 1.5): compact prose, form labels (500, a step under a heading), buttons (600), cells.
- **Lede** (Hanken Grotesk 300, 0.875rem/14px on 24px lines, a step under the section heading it explains): section introductions throughout the UI, including nested sections and drawers, never a page's h1, capped at 60ch in Body ink; paragraphs stand 16px apart. A takeover's lede keeps 1rem/16px. They wrap and grow with enlarged text and translations; labels and technical keys retain their compact roles and full contrast. Ledes stay Body gray while scrolling and focusing controls, with no color transition. Warning ledes retain their semantic warning color.
- **Label** (500, 0.75rem/12px, 0.08em, uppercase): kickers and column heads, in Meta. A page card's column heads are the exception: 600 at 0.875rem/14px in Ink, sentence case, on their row of the card's Paper (Tables).
- **Value** (Inconsolata 500, 1rem/16px): machine strings in rows — addresses, interface names, MACs, versions. Inputs for identifiers use Inconsolata 400 at 16px.
- **Figure** (Inconsolata 700, 2.25rem/36px, -0.05em): big numbers.

On a page a section's heading is its card's header, and its lede stands in
the header under the title, at 400. In a drawer, full section headings and their ledes
stand under the section's leading Hairline, with no fill and no decorative
marker; the rule and the heading's size establish the section's hierarchy.
Smaller subsection headings retain their ledger lines; square marks carry
state.

### Named Rules
**The Verbatim Rule.** Mono is for strings the machine wrote and a user might copy: identifiers, addresses, versions, config keys. Never for a count, a duration or a sentence. Counts are sans with tabular figures.

**The Even Pitch Rule.** Mono never takes negative tracking, except the 700-weight Figure, whose wide glyphs can carry it.

**The Narrow Space Rule.** Every unit stands a narrow no-break space (U+202F) after its figure, wherever a figure carries one: 48.0 °C, 20.10 V, 0.49 A, 9.8 W, 2,622 rpm, 3.3 dBm, 12 Mb/s, 29 GiB, 24 h, 12 min, 30 %. The space keeps the figure and its unit apart as "8 min" and "8 minutes" are, reads as one word at a glance, and never lets a unit wrap onto a line of its own. No unit is written against its figure, the percent and the degree included, and none takes a full space. The languages that space a unit already set it with this character. The shell writes it through `internal/units` and the page through `verso-units.js`.

**The One Rate Rule.** Every rate reads one way, wherever it stands: one decimal under ten and whole from ten, its ".0" dropped (4.3, 4, 413), in Mb/s up to 999 and Gb/s from there (1.2 Gb/s). A rate above zero reads at least 0.1 Mb/s, so a trickle is never taken for silence; only nothing reads 0. The unit stands with the figure, never in a column head, because a column's rows can cross into Gb/s. A chart's scale is round: its top is four steps of 1, 1.5, 2, 2.5, 3, 4, 5, 6 or 8 times a power of ten (whole tenths, so no label needs a second decimal), at least a tenth over the tallest point, so its labels read as counted (60 · 45 · 30 · 15, 1.2 · 0.9 · 0.6 · 0.3), the curve fills at least three fifths of the plot, and a live chart's scale moves only when the traffic crosses a step. Its labels are bare figures, and the unit they count in (Mb/s, Gb/s) stands once on the scale's baseline (Charts). Totals follow the same figure rule in binary units (512 MiB, 3.5 GiB, 212 GiB). The decimal separator is the reader's language's ("4,3" in Slovenian). The shell writes them through `internal/units`, and the page writes live ones through `verso-units.js`; both are held to the same cases. A plugin never writes a rate: it hands the shell the number (`rate`, in bits per second, on a property or a table cell), so a port's link speed reads "port · 2.5 Gb/s" as the shell writes it.

**The One Duration Rule.** Every stretch of time reads one way, wherever it stands: seconds under a minute, minutes under an hour, and from there its two largest units, the second left out when it is nothing (45 s, 12 min, 2 h 14 min, 52 d 8 h, 3 d). Each figure is followed by its unit symbol (s, min, h, d) after a narrow no-break space, as a rate is (the Narrow Space Rule): never a clock's colon, which reads as a time of day, and never a zero-padded figure. A time ago is its largest unit alone in the language's pattern (12 min ago, 3 h ago; "pred 3 h"), and under a minute is "just now". A duration is one phrase in the font, weight and ink of where it stands, its units the figures' own, whether it stands as a value (a fact, a table cell, a live reading, with tabular figures) or in a sentence ("Internet came back after 3 min", Reboot's "Running for"). The shell writes them through `internal/units`, held to the cases in `testdata/durations.json`. A plugin never writes a duration: it hands the shell the number (`duration`, in seconds, on a property or a table cell).

## Layout

The page is left-aligned against a 288px rail, never centred. The frame keeps 40px of air on every side (16px at the sides on a phone, for a card page as for the Overview, since a card adds its own inset), and inside it the page declares a measure, always a multiple of 8px: wide pages cap at 1136px, the default at 1016px, narrow at 880px, page forms at 760px (`max-w-form`, the redefined `--container-3xl`). A column the window squeezes rounds down to a multiple of 8px, and every column is a pixel wider than its measure (1017px for the default), so a card's right border lands on a line as its left one does; Log out's measure takes the same pixel. Form rows and the main columns of settings, editor and rail layouts share that 760px measure; supporting columns take the remaining space beside it once the container has 1000px (`@form-split`: the form, the 40px gap, a 200px rail), and stack under it when narrower. Drawers stand at 760px, keep the page's 40px side inset, and use the same ledger rows and content measures as page forms. The top bar's right group ends on the same rounded column edge, so a page's primary act sits exactly under Log out.

**The 8px grid.** No page draws a grid: every page but the Overview and the live pages (Logs, Firewall › Activity, Diagnostics) stands on Quiet Sand. Every page, card and drawer is measured on one 8px grid, from the top bar's Hairline and the rail's border (a drawer's from its chrome's Hairline):

1. **Every fixed measure is a multiple of 8px**: a box's height or width, the air between boxes, a card's or a drawer's inset, a page measure. The layout's roles are theme tokens: a control is 32px (`h-control`), a card's inset and the air between cards, fields and a group's steps 24px (`inset`), the page's frame and a drawer's inset 40px (`frame`), an act 28px (`act`).
2. **Inside a component a measure steps by 4px**: a component's own box (a chip, a checkbox, a radio row, a row act) is a multiple of 4px. Inside it, padding centres its text on its line, and the 6px between a glyph and its words is its own. Marks and glyphs keep their sizes: the 6px packet, a 14px or 16px icon, a 1px hairline.
3. **Text writes on 24px lines.** A row is its lines and 16px (7px after the hairline over it, 8px over its own): 40px for a line, 64px for two. Air after a drawn edge is a pixel short and air before one is whole, so text fills whole 24px bands of the grid while every edge stays on a line: a card's body starts 23px after its frame or its header's Hairline and ends 24px over the frame; a run of rows with no hairline over it gives its first row the full 8px, and its last row draws none. Since a line is a multiple of 8px, a box that grows with its text ends on the grid as a fixed box does.
4. **Every drawn edge sits on a line**: a frame's top and left borders, a header's Hairline, a rule. A box's air under its content and the gap after it give back its bottom border's pixel, so the bottom border and what follows land on lines too; a column is a pixel wider than its measure for the same reason, so a frame's right border lands on a line. A framed box inside a frame starts a pixel past the line (the outer frame's border); its bottom border lands on a line, and a row of controls inside it (a notice's buttons) stands 23px under the words, so the controls' edges stay on lines too.
5. **Controls are 32px and a pixel**: a field, a dropdown or a button has its top and bottom borders both on lines, as a card's frame does, and gives the bottom border's pixel back, so whatever holds it measures it as 32px. It stands straight under its label's line, so a field with its air is 80px (24px air, the 24px label, the 32px control). A row of controls is 32px, its wrapped lines 40px apart. Across, a card's words stand 23px after its left border and 23px before its right one, so a control that starts a row has its left border on a line and one that ends a row its right border. A dropdown that rides a line of words (the Traffic card's window in its header) is the compact one: its 24px box is the line itself, 14px words 8px in, its 14px chevron 8px from the right edge (`verso-select-compact`).

A set of radios and a rail's "On this page" links are rows of 32px: their 24px line and 4px over and under, a run of any count a multiple of 8px.

In a dev session (`make dev`), G draws the 8px grid over the page and behind a drawer's body, and G again takes it away; the choice holds for the tab. A released build never draws it.

**The rhythm is 40 and 24.** Cards on a page stand 24px apart, and inside a card its blocks stand 24px apart too, as the first stands under the title: the card already holds them together. A field after a block takes no margin, since its own 24px of air is the step. A titled part inside a card starts a group of its own, 40px on. Inside a drawer, blocks stand 40px apart, the same as the body inset; a group's steps (heading to toolbar to table) are 24px. A drawer section's rule stands 40px under the content before it. The band is the rule, 40px, the title's 24px line, its lede's lines under it, then the content straight on: a field brings its own 24px of air, and a band with a lede keeps 24px under it. Height follows the contents, with no fixed or minimum band height.

**Sections are one component** (`sections.css`), the same on a page and in a drawer. A section states its kind and the stylesheet gives it its air and its rule:

| Kind | What it is | Air and rule |
|---|---|---|
| ruled | one of the surface's subjects | on a page, a card (below); in a drawer 40px, hairline, 24px, with a heading the band carrying the rule (above), the drawer's first drawing none |
| part | a ruled section inside a section | 24px, hairline, 24px, kept to the content's inset |
| continued | more of the rows above (the add act) | hairline always, on the last row's own line |
| plain | an unruled block | 24px above (none on a page's own stack, which spaces its blocks 40px) |
| bare | a nested or flush block | none of its own |

A surface differs only in its reach, how far its rules and bands run past the content: in a page's card 24px, the card's inset, to both of its edges; a drawer 40px, the same inset, to both edges. Every hairline divider (a section's, a band's, a config preview's) reaches by the same two measures and pads back in, so its words never move. Hand-drawn pages (Maintenance) use the same kinds.

**On a page, each top-level ruled section is a card**: Paper in a Strong Hairline frame with 2px corners, 24px in, the cards 24px apart in one column at the page's measure. A titled section's band is the card's header, 48px as an Overview card's is: the section's 16px Lucide glyph in Glyph when it has one, the Card title, its metadata and its control on the title's 24px line, 11px after the frame and 12px over the header's seam, so the title is centred on the header's middle line. The seam is a Strong Hairline, the frame's own ink, on a page card and an Overview card alike: the header reads as the card's, and the Hairlines under it, between questions, as what the card holds. A lede belongs to the header, its lines straight under the title's and over the seam, set at 400 there, one step heavier than a lede elsewhere, the header growing a 24px line for each of them. A page form's untitled run of fields (Router name on General, Upstream servers on DNS) is the same card with no header, its first name 15px after the frame, as after any line. A part inside a section stays on its ledger line inside its section's card, and a framed box a section holds (the certificate on Access) gives up its frame and fill, so no card holds a card. A listing in a card runs to the card's edges, its words 24px in, and one that ends the card gives its last hairline to the frame. A listing that opens the card's body stands straight under the header, its column-head row in place of the header's seam (Tables). A warning about one card's subject (the web interface reachable from the internet) opens that card's body. Page cards neither fold nor move; that is the Overview's own.

A configuration card's divider stands 24px under what it closes in a drawer, and what follows stands 24px under it. Section separation inside a drawer is measured from the field row's box. Stacked fields grow with their label, control and errors by whole lines. An untitled section gives back its first row's air, which its rule already holds; a titled section keeps it under the band. Rows end on lines by themselves, so no section trims its last. A drawer's section bands and configuration dividers span its full width; subsection and list rules keep the content inset.

The h1 stands alone: a page carries no lede, because the reader already knows where they are. The heading line stands straight on the sand, with no bar and no Hairline of its own: 24px under the top bar and 24px over the first card. Page acts end where Log out does, whatever measure the content column keeps. A live log has no cards: its search and acts stand on the heading line, and its masthead is the light one, a bar on the page's own ground (Paper, as the rail is) from the rail to the window's right edge, its heading line 16px inside it at top and bottom, ending on a Hairline, so the Quiet Sand log under it is the page's one darker surface, flush on the Hairline. Diagnostics reads the same way: its row (tool, target, interface, IP family, then Run where Log out ends) stands on a line of its own under the heading. The tool's field leads with that tool's glyph in Glyph (ping's radio echo, traceroute's route, the lookup's search), which a pick swaps in from a little smaller; the interface's field leads with the network glyph and reads "any interface" in the sans, a picked interface's name in mono. Run leads with play. While a run goes, Stop stands in its place with the waiting mark turning, as a log's Live does, and the mark holds still once Stop is pressed. The target offers back what this browser reached for before, and with a keyboard at hand the caret waits in it. Each run's output fills the sand as a log does, the program's own lines from its first, read in the code block's inks (addresses green, measurements denim, ping's statistics rule and an unanswered hop Meta), closed by how it ended. The home page's verdict is its h1, on the same heading line: the most important true sentence on the page. It names what is wrong in the words of the card that says it ("Storage is high"), counts several ("2 things need attention"), and reads in its tone's Deep step while something is wrong; "online" alone wears Deep Green.

Inside a group (heading to toolbar to table) the step is 24px. Rows are 40px (a 24px line, 7px after the hairline over it and 8px over its own), drawer headers, drawer tab strips and the colophon are 56px as the top bar is (a 28px line plus 14px above and below, the Hairline drawn inside the band), controls are 32px, and a control inside a row is 28px, riding its line. Section bands grow with wrapped headings, ledes and controls by whole lines; a heading act stands centred on the title's line and reaches past it.

Acts sit at the level they act on. A page's acts stand on its heading line. A form that is the whole page commits every card on it, so its Save stands on the sand 24px under the last card, with no rule. Whenever a form has a configuration card, on a page or in a drawer, the card sits immediately above Save; the actions finish that card without another rule. On a page the configuration card is a card of its own, 24px under the last section's; in a drawer its rule is a section's rule: 40px under the content before it, and the card 40px under it, as a title stands under its rule. A drawer form without a configuration card closes on its own inset hairline, 24px under its fields. A form that commits one section keeps its Save inside that section (its card, on a page) and draws no hairline: its Save stands 24px under its last field, and the next section's header identifies the next subject. Within a section, its settings and their Save come first, then what the section holds (keys, a certificate), with that thing's own acts 24px under it.

Every list page reads top to bottom as: the heading line (h1 left, page acts right), then the listing's card: the **control band** as its top strip when the listing has one, then the rows. A page that is one listing is one card with no header, since the page's heading names it.

Grouped listings split into lanes: one card for the listing, each lane opening on a Quiet Sand band among its rows (Tables).

On a phone the rail becomes an off-canvas drawer, bands stack their controls, and wide tables scroll inside their own box rather than widening the page.

## Elevation & Depth

Verso is flat at rest. Depth comes from tone (Paper → Quiet Sand → Mid Sand) and from hairlines, not shadows. Shadows exist only on things that float over the page, and only as much as it takes to lift them.

### Shadow Vocabulary
Each shadow is a shape the shell keeps (offset, blur, spread) over a colour token of the palette, Ink at the strength the job needs, written as a hex with alpha. It is cast with `shadow-(--shadow-…)` or `var(--shadow-…)`, never written where it is used; a theme changes only the colour.
- **Field inset** (`--shadow-field`: `inset 0 1px 2px`, `--color-shadow-field` Ink at 6%): every input and select — a field reads as a slot cut into the paper.
- **Float** (`--shadow-float`: `0 4px 12px`, Ink at 14%): tooltips and hover tips, the notification, a token's menu.
- **Menu** (`--shadow-menu`: `0 8px 24px`, Ink at 14%): a condition's menu.
- **Drawer** (`--shadow-drawer`: `-8px 0 32px`, Ink at 12%): the 760px side panel, cast leftward over the page.
- **Dialog** (`--shadow-dialog`: `0 8px 32px`, Ink at 16%): modal dialogs and alerts, and an Overview card in hand. A confirmation asks in place and casts none.
- **Edge** (`--shadow-edge`: `inset 0 4px 4px -4px`, Ink at 14%): a log's or a console's top edge, the lines sliding under the bar above.
- **Lifted** (`--shadow-lifted`: two drops, Ink at 24% and 18%, `--color-shadow-lifted` and `-lifted-near`): a table row in hand while it is moved.
- **Port tip** (`--shadow-port-tip`: black at 70%): a port's dark tip over the router's panel.

### Named Rules
**The Flat-By-Default Rule.** Nothing on the page casts a shadow. Only a layer that floats above the page (drawer, dialog, tooltip) does, and a pressed button loses even its press shadow.

## Shapes

Corners are nearly square: 2px on every button, field, chip, band and card; 1px on state marks. Circles appear only for toggle knobs, meter pills and the interface tree's physical/software encoding.

The state mark is a 6px square (5px in the nav), the packet. Filled means present in its hue; hollow (Faint border, no fill) means absent. The waiting mark is four of those squares folding on an 8px grid. Never a circular spinner.

Borders are hairlines, 1px, in Hairline or Strong Hairline. Tables have no vertical rules and no stripes; rows are separated by a single hairline.

## Components

### Section headings
- **On a page:** the band is the card's header (see Layout): the glyph, the 16px Card title, its metadata and control on the title's line, the header's seam, a Strong Hairline, under it. Padding, reach and the first band's missing rule below are a drawer's.
- **Surface:** no fill. The band is the section's leading hairline with the heading under it; no decorative marker.
- **Contents:** the heading is 18px/600 on a 24px line. Its metadata and controls share the heading line, centred on it, a control reaching past it rather than making it taller; a lede belongs inside the same band, the next line, in 14px/300 Body ink on 24px lines, at most 60ch wide. Markdown and links retain their normal prose styling. A section's metadata reads as a label in Meta and a value in Body at 600 (Reboot's "Running for"), or inline beside the title (when Firmware was last checked); its control stands at the band's right end (Check again) and wraps within the band when needed.
- **Lede, rarely:** a section has none by default. One is kept only for what the heading and the rows under it cannot carry: a consequence to know before touching anything ("Restoring replaces the saved files and reboots the router"), or a fact that changes how the whole section reads ("These apply to every server above"). A lede that restates the heading, previews the fields (each field's own tip or description says what it is), or advises leaving things alone is clutter.
- **Padding:** 40px from the hairline to the title, with or without a lede; the drawer's first band draws no rule, its title 40px under the chrome's Hairline. Do not impose a fixed or minimum height; wrapping and controls determine the height.
- **Content inset:** the content stands 24px under the title (or its lede). A field brings that air itself; a band with a lede keeps it; anything else (a listing, a reading, a set) is given it by the section. Handwritten section bodies, such as Maintenance's, supply the same air themselves. Count it once.
- **Content bottom spacing:** a section keeps no closing air: the next section's rule stands 40px under the content, by its own margin.
- **Reach:** in a drawer the band reaches both edges while its contents keep the body's 40px inset; on a page the header reaches the card's frame, its words 24px in.
- **Scope:** page h1s, drawer and dialog titles, navigation kickers and smaller subsection headings keep their own treatments. A graph draws no title of its own (Charts): the Overview's Traffic card names it in the card's header, and a device's panel under the section heading "Usage". Subsections retain their inset ledger lines.

### Buttons
Plain and exact.
- **Labels:** a button says what it does to what: a verb and its object, in sentence case. "Save rule", "Save zone", "Save SSH settings", "Add network", "Install certificate", "Delete interface". Never a bare "Save", "Update", "Commit", "Submit", "OK" or "Confirm": the reader should not have to look around the button to know what it will act on. A save stages (see Staging), so its verb is Save; an act that runs at once is named for what it does ("Install certificate", "Reboot"), never Save. A confirmation's button repeats the act it confirms ("Delete rule" asks, "Delete rule" answers), and that is the default when a confirm names none. A form that names no act falls back to "Save changes". An icon-only act in a row keeps its short title as a tooltip ("Edit") but is named with its row for a screen reader ("Edit lan"). A dialog that can only be acknowledged says what acknowledging lets you do ("Keep working"), not "OK".
- **No explanations beside acts:** a row of acts carries its buttons and nothing else. What an act does is its label's job, and what happens after is the product's (a save stages, the Pending changes row counts it). A line beside the acts is kept only for a fact someone must know before pressing and cannot learn anywhere else; a status reading that belongs to the controls ("Index refreshed 2 days ago") is not an explanation.
- **Shape:** gently squared (2px), 32px tall (`h-control`, the one height every field and button shares), 16px side padding, 14px/600 text.
- **Primary:** Denim fill, white words, and always a leading 16px plus glyph, unless the act leads elsewhere and names its own (download, upload, open-out). It lives on the heading line, one per page.
- **Secondary:** the subsection act's colours at any size: transparent, Meta words, Strong Hairline border; on hover the border turns Parked, the fill Hairline, the words Ink. Every quiet button answers the pointer alike (a list's Add, Cancel, a log's Live · Download · Settings, a row button). Words only; no decorative icons.
- **Links out:** a link that opens in a new tab leaves the router, and says so: the Lucide external-link glyph, 16px, leads its words ("Project website"), and a screen reader hears "(opens in a new tab)" after them. A link that names its own glyph keeps it; the words still tell a screen reader.
- **Danger:** Crimson fill, white words, for the confirmation step only: the inline question's act, always beside its way back (see Confirmation), or the act a typed key unlocks.
- **Subsection acts:** the acts on a part of a section: "+ Add a key", and a certificate's Install · Make a new one · Download. One dress for all of them:
  - 28px tall, 14px/500 words in Meta, a Strong Hairline border on no fill.
  - Always a leading 16px glyph that names the act (plus, upload, refresh, download), with 8px padding before it and 10px after the words.
  - On hover the border turns Parked, the fill Hairline, the words Ink.
  - One exception: an act on a section's heading line (General's "Use my computer's time") aligns with the section's right content edge, including when it wraps below the heading. It takes a field's 32px height with 16px side padding, in the same colours, centred on the title's line and reaching past it.
  - An act that makes or replaces the thing it stands under (a certificate's Install · Make a new one) opens its form in the drawer over the page, which keeps its address. The drawer's button names the act ("Install certificate", never "Save": it runs at once, nothing stages). Once it has run, the drawer closes on the page read again, saying once what happened; a refusal stays in the drawer over what was typed. An act that hands over a file (Download) downloads.
- **Row buttons:** 28px, 12px side padding, the same secondary dress. Every icon-only act wears one dress, wherever it stands (a row's edit or remove, a list's remove ×, a copy beside a value or on a code box): a 28px square with a 16px glyph in Glyph on no border, taking a Parked border, the Hairline fill and Ink on hover, with a tooltip. Beside a line of text it keeps the line's height.
- **Copy controls:** an inline copy sits 4px after its value, with no extra parent gap. Its 28px hit area centers on the first text line without increasing the line height. Copy tooltips use the UI sans face, including beside mono values. A check and “Copied” appear only after a successful copy; the check is green on neutral surfaces and keeps the warning hue inside a warning. Failure is shown and announced with a manual-copy fallback. Keyboard focus stays on the control. Code cards keep the copy control in their top-right corner; the colophon copies its complete bug report.
- **Press:** every enabled button nudges down 1px and drops its shadow while pressed, including icon-only actions, drawer controls and client-created buttons. `input.css` owns the shared rule for native buttons, button inputs and `role="button"`; button-styled links join it through `verso-press`. Disabled and busy controls do not press. Reduced motion suppresses the movement. A button with an enlarged hit area moves only its `verso-press-content` child, preserving the click target. The effect changes no layout dimensions.
- **Resting:** a settings form's Save before anything in it has changed is disabled in Quiet Sand, a Strong Hairline border and Meta words, and turns denim the moment something changes. A page with no primary act and nothing changed therefore shows no denim.
- **Waiting:** a button busy with a slow act keeps its footprint, goes Quiet Sand with Parked words, shows the four-square waiting mark and a present-participle label with a real ellipsis ("Applying…"). Disable it immediately and mark it busy until the request finishes; repeated clicks or Enter must not submit again. Restore its original label, icon and availability after a refusal or connection failure. The same behavior applies to actions in drawers opened from any page. A button whose press starts work the page then waits on names its busy words (`busy`, "Checking…"), and the shell swaps them in on the press. The look is one rule (`verso-button-waiting`) laid over the style's own dress, whether the server draws a button busy or the script makes it so, so either wait ends by taking the rule off. Under reduced motion, the four squares remain visible and still.

### Confirmation
How a consequential act asks first. The whole exchange is one hue, so the question and both answers read as a single voice. The hue is the act's cost:
- **Danger (crimson):** the act destroys or drops something: rebooting, deleting a rule.
- **Caution (marigold):** the act is disruptive but wanted, and is the point of being here: installing firmware. It steadies rather than scares.

The anatomy is the same in both:
- **In place:** the trigger gives way to the question where it stood. Nothing floats and nothing casts a shadow. Focus moves to the first answer; Escape and the way back return it to the trigger.
- **Over the page:** a row act in a table (deleting a rule, revoking a session) has no room to ask in its row, so the same band rises as an alert over the page: the band is the dialog's whole face and frame, lifted by the floating layer's shadow, centred on a wide window and at the foot of a phone's, under the thumb. The words and answers are the band's, unchanged.
- **One hue, every step of it:** the tone's Wash ground inside its Hairline, 2px corners, every word in its Deep step. Full chroma appears as a mark only: the 6px state square on the first line, and for danger the fill behind the act's white words. No sand, no Ink, no second border.
- **Trigger:** the act's own name ("Reboot now", "Download and install 25.12.5") on the Wash with the Hairline, 14px/600 words in the Deep step, a 32px control as wide as its words like every other; the hairline turns full chroma on hover.
- **Question:** the compact inline warning itself (Inline warnings), in the act's tone: danger's crimson band, caution's marigold one. The question ("Reboot the router now?") is its 14px/600 title over one or two sentences of consequence, the 6px square on the first line, the frame on the grid's lines and the words a row's air inside.
- **The answer pair:** the band's acts, 24px under the words and 16px over the band's lower edge: the act (32px, named with the verb: "Reboot", "Install firmware"), then the way back as bare words in the Deep step, named for what it keeps or defers ("Not now", "Not yet", "Keep rule"; "Keep it" when nothing names it): no border, no fill, 12px side padding, washing to the Hairline at half strength on hover. Danger's act is white on full Crimson. Caution's act is white on Deep Marigold, because white on full Marigold fails the 4.5 floor. Never "OK" and "Cancel".
- **With a password:** when the act needs re-authorizing, the password field leads the answer row with a Crimson Hairline border.

### Inline warnings
A warning (marigold) or an error (crimson) said in place, on the tone's Wash, every word in its Deep step, framed by the tone's Hairline, one step darker than the Wash; the frame stands on the grid's lines. Every notice is this one band: one under a control keeps the form's measure, one that leads a page or a card reaches its container's width, and nothing else differs. It is the only thing on a surface that explains itself: when nothing is wrong, nothing is said.
- **One size:** the title and the text under it are both 14px on 24px lines; the title is bold (600), the text regular, 7px after the frame's top border and 8px over its bottom one, framed as a card is: both borders land on lines, the bottom border's pixel is given back, so a line of it is a row's 40px and its words fill a band of the grid. The title never steps up a size and the text is never a lede.
- **Mark:** the tone's 6px square on the title's first line, 9px down it, in the 24px between the frame's outer edge and the words: 10px from the edge, in the middle 8px cell of the three. Every notice carries it. The words start 24px from the frame's outer edge, on a line.
- **Its door:** a link to where the matter is settled closes the notice's last line of words ("… from the internet. Open Firewall"), never a line of its own. It is the words' own ink and weight, told apart only by its underline, which thickens under the pointer; never Denim inside a band of another hue.
- **The machine's words:** what a tool reported rides inside, under a Hairline of the tone, verbatim in mono.
- **In a card:** a notice stands 24px from what is over it and what is under it, the card's step, whether it opens the card or stands between blocks.
- **The acts that resolve it:** a notice may carry the buttons that resolve what it says (install what is missing): a row of 32px buttons inside the band, 23px under its words and 16px over the frame, the row keeping a cell of its own under the band's buttons so they never crowd its edge, so the buttons stand on the band's lines and the band stays on the grid. The buttons keep their own shape and take the band's ink: words and glyph in its deep step, the border that ink thinned, a wash of it under the pointer.
- A confirmation's question is this band, its answers the band's acts (Confirmation).

### Chips
- **Style:** lowercase Inconsolata 14px in Meta on Quiet Sand, Hairline border, 2px corners, 1px 6px padding. They carry config values and config keys (`hostname`, `pppoe`), never decoration.
- **Accent:** Denim Wash with Deep Denim words and a leading 14px icon, for what was configured by hand. A green variant marks "you" (this browser, this session).
- **Entity chips:** an icon plus label for an interface, zone or port cited inside another row. Table chips use the shared semantic palette: network and zone references in denim, healthy DHCP and reservations in green, configured device limits in marigold. Reservation row actions use one icon family: pin to reserve, pin-off to remove a reservation. Each row offers only the action that applies to its state. Reserved devices carry the same pin icon and the short label `reserved`. The words `reserved` and `limits` use the sans face at regular weight. Limits chips say only `limits`; the tooltip is a compact readout: seven weekday cells with scheduled days in marigold, a prominent time window and quiet router-time caption, then paired download/upload values. Keep prose as the accessible text alternative. Keep category labels short and entity names exact. Colored chip icons follow their hue, using its Deep step for green, marigold and crimson.
- **One box, always a border:** every chip that cites something is the same box: a row's tag ("this browser"), the router at one end of a firewall rule, a rule's action (`accept`, `reject`, `drop`, `mark`; `drop` has no hue and is the neutral chip), the protocol beside IPv4, a choice's config value, an interface in a row. That box is 14px/400 words on a 16px line, 6px sides and a 1px border, 18px tall, 3px inside a 24px line so a field under the line stands clear of it. The option chip beside a setting's name is 21px, a pixel more air over its word and two more under it so the word reads centred in its box, 2px inside the name's line over it and 1px under, since a setting's answer stands beside its name, not under it. The border is its hue's Hairline on its hue's Wash, or Hairline on Quiet Sand for a neutral one. Only the face varies: mono for a string the machine wrote (`accept`, `lan`, `dhcp`), sans for words ("this browser", the router, which names a thing rather than a value). A mono chip is one step heavier, 500, because Inconsolata at 400 reads a size smaller than the Hanken beside it.
- **One mark or none:** a chip leads with nothing, a 14px Lucide icon, or the 6px packet square in its hue (hollow when the chip has none, the packet absent). Never two; an icon wins. The packet in a chip is still: a chip states a fact, and the only pulse is the page's live mark. A firewall verdict (`accept`, `reject`, `drop`, `mark`) always leads with its packet, and `drop`'s is hollow. On any 24px line the chip stands centred, 3px inside it at either end, rather than making it taller, its words on the line's baseline, so a key chip stands clear of the field under its label. The removable token inside a list control is not a citation and keeps its 24px box.

### Control Band
What narrows a listing, the top strip of its card.
- **Surface:** the listing card's own Paper and frame, its 2px corners at the top, a Hairline under it where the listing starts: 64px with the frame's pixel and the Hairline, the controls on the line 16px under its top border and 16px over its Hairline, 23px after the card's left border, so the first control's left border is a line. No fill of its own, so the controls and the rows read as one card. On a phone its controls stack inside it at the same inset.
- **Contents:** only what narrows the content, side by side from the left, 16px apart — over a live listing, or one whose search reaches a larger set the router holds (Packages' index), a search first (the app's field treatment), then counted dropdowns ("IPv4 · 14", "All families · 22"), then the dimension the listing is sliced along ("All networks"); never segmented switches, and no dropdown strays to the band's far end. An option that is another listing (Packages' All) loads it rather than narrowing, and goes unpriced while that listing is unread. A paged listing walks its pages on one line of prose under the rows ("6213 matches · Next"). Everything that acts (primary add, Live, Download, Settings) goes on the heading line instead.
- **No search over a still listing:** a listing that holds what it has, however long, is scrolled and searched with the browser's own find; a search field over it is a second, weaker find, and a cut by a word on the rows (a family, a network, a kind) is the same find again. Only a live listing (the firewall's activity, a log) carries a search, because its rows arrive while it is read. A still listing keeps a cut only where it sorts by a state no word on the row spells out (Devices: online, offline, reserved, with limits). A band left with nothing to narrow with is not drawn, so most pages run from the heading straight into their table.
- **Below it:** the listing joins the strip with no gap, its column-head row standing on the band's Hairline; a log sits flush on Paper.

### Tables
- **Rows:** 40px, Paper, one Hairline under each row, no stripes. A cell writes on 24px lines 7px after the hairline over the row and 8px over its own, so a row is 16px more than its lines; a table with no column heads stands a pixel down, as if under one, so every row lands on the grid however its values wrap; every cell stands at the row's top, so a row's first lines stay level. Rows are inert, and so is the name: it is words, never a control, and never turns denim under the pointer, because a reader cannot tell what pressing a name would do. The row's acts open it, saying what they do with their glyph: the edit pencil for an editor, Details for a drawer to read. A row whose name a plugin pointed somewhere is given that act by the shell when it lacks one, or the trailing Details link when it has no acts at all. A row's icons read in the order of the tabs its panel opens on: on a row whose panel is the shell's (a device), the Details door leads the acts, as the shell's Details tab leads the strip, and each plugin's slot follows; elsewhere the door trails the row's own acts. Only an expandable row's name is a control, and its chevron says so. A table closes on the Hairline under its last row, wherever it stands, so it always ends on a line of the grid.
- **Column heads:** 24px, 12px/500 uppercase Meta with 0.08em tracking on a 16px line, 4px over and 3px under, the Strong Hairline the row's last pixel.
- **Column heads in a page's card:** a row of the card's own Paper across the card between Hairlines, the heads 14px/600 Ink in sentence case on a 16px line, 15px after the edge over it and 16px over its own Hairline: 48px from line to line, as tall as a card's header. The row is what divides the card's header from its rows: the header draws no Hairline of its own and leaves no air over it, and where the row opens the card (an untitled card, or under its control band) the frame or the band's Hairline is its top edge. In a card a row's name sits at 500 and its other values at 400, a step lighter than a drawer's.
- **Lanes:** a grouped listing is one card on a page, one table in a section or a drawer, and each lane opens on a 40px text row filled with Quiet Sand between Strong Hairlines (its 28px add centred in it), straight after the row before it: the first lane's top hairline is the column heads' own, a later lane draws its own on the line where its fill starts. Label → destination with a faint arrow, the lane's chain in mono Meta beside it, and a quiet "+ Add rule" at the right; a lane carries no count. A drag stays in its lane, and the order it stages is the whole listing's. A lane the cut leaves empty is put away.
- **Switched off:** a row whose subject is not in force (a rule, a Wi-Fi network, a route) has its name marked over with a sand marker, the way an operator runs a highlighter through an entry that no longer holds. The marker is Strong Hairline ink laid under the name's words alone (never the whole row) and overshooting them by 10px each side as a hand does, a band a little slimmer than the line, its edges frayed and streaked along the stroke, its chisel ends pooled a little darker; it never passes 80%, so the name, set in Body at the regular weight, keeps 4.9 : 1 over it. Every other word reads at Meta; every hue in the row falls to sand (marks to Glyph, chips to the neutral box), so colour in a table means "in force". Nothing fades below 4.5 : 1. The row's acts and the focus ring keep denim, and a screen reader hears "off" after the name. When the row's power act turns it off in place, the marker is drawn left to right in 520ms; turned back on, it lifts in 280ms; once each, and under reduced motion it simply appears or goes. A plugin sets the row's `muted`; the shell draws all of it.
- **A plus on a table head always has words.** Wherever a lane or a table head carries an add, the plus stands beside the verb and its object ("+ Add rule", "+ Add route"), never alone: the head has the room, and a bare glyph makes the reader guess what it adds. The words stay short because the lane already names where the new row lands; the accessible name carries the full phrase ("Add IPv4 route").
- **Empty:** the slot every set draws (Collections, Empty), where the first row would stand: the hollow packet and two sentences at most, no illustration, no button, its frame on the lines. Under a heading it stands 24px under the title. A live listing keeps a row instead, the wait rather than the absence, and a listing whose lane band stands with nothing under it ("Files · 0 · + New file") keeps the band and says its sentence in a row under it. It is never a made-up row in a value column.
- **Names:** a row's name is the listing's 14px at 600 in Ink, wherever its column stands.
- **Last hairline:** a listing that ends its card gives its last row's hairline to the card's frame, the frame's border taking the row's pixel, and one that ends a drawer's section gives it to the rule that closes it, the row keeping the pixel, clear; the two never stand as a pair.
- **Detail line:** when a value needs words to say what it is or since when (the browser behind an address, when a session started), they stand on a second 24px line under it in 14px Meta, never beside it, and the cell's tag rides that line. The row becomes a stacked row: two 24px lines, 7px after the hairline over it and 8px over its own, 64px, its first line level with every one-line cell's and with the row's 28px acts. Folding a qualifier under its value is how a narrow measure holds a long identifier: Signed in now is Source (address over browser), Last seen ("3 min ago" over "since 22 Sep, 09:12") and a revoke icon, which fits 640px with a full IPv6 address.
- **Icon acts that end something** ask first, in the same alert as the labelled row act, with the act's own name on the danger button ("Revoke session").
- **Row acts:** a row's last icon act keeps 7px clear of the table's right edge (12px before the first act), centred on the row's middle line (5px over the 28px act, 6px under; a stacked row's first line). A lane's add sits the same way, so on hover its border ends where the rows' acts do.
- **The grip:** a reorder grip stands 7px in from the table's left edge, the rest of its column after it (the column its padding and the grip whole: 40px, 32px dense), on the row's middle line (7px over, 8px under).
- **Nothing to state:** a cell with no value, in any column, is one mark: the dash in the sans, in Faint, a step below a muted value, said "None" to a screen reader. Never a mono dash that reads as a minus, never a state's hollow packet with nothing after it, never left blank where the column expects a value. A check, a switch, a grip and a row's acts keep their own emptiness.
- **Inked figures:** a reading that comes in figures (the Devices roster's Now, a rate down and up; This month, one total) stands as figures side by side, each with its unit in a slot of one width (5.5rem, 16px apart) so a column of them lines up down a lane, right-aligned in sans tabular Ink, a 14px Lucide glyph after one that runs a way (arrow-down for download, arrow-up for upload) in Glyph. No meter stands under a figure: a rate has no ceiling the router knows (no line speed, and a share of whatever else moves says only who else is busy), so a bar under it would be pretty and meaningless. Where the size of a reading matters at a glance, the cell is inked instead: its figures are written at 500 in a tone's Deep step, the listing owning the thresholds. Now inks a device's load, both ways together, by what the rate is enough for: light (browsing, a video call, an HD film) under 10 Mb/s in Deep Green, medium (a 4K film or a few HD ones, a cloud sync) to 100 in Deep Marigold, heavy (a download, an update, a backup) past it in Deep Crimson. The glyphs stay Glyph. The legend says each ink in the ink itself, the word in its tone at 500 and its range after it in Meta ("light under 10 Mb/s"), so the scale is read where the colours are; the figures already carry the size, so colour is never the only way to tell. A live listing changes the ink in place: it eases over 300ms through Oklab (each tone restated in OKLCH, since hex eases through sRGB and green to crimson would pass through mud), none under reduced motion, and a device waking from idle arrives already in its ink. A cell with no figure is the dash; its slots stay drawn and hidden so a live listing fills them in place, the figure changing without a roll, since a roll every second down a lane is noise. A screen reader hears the figure and its direction ("38 Mb/s down").
- **Meters:** a share or a headroom in a listing is the meter at a row's scale: a 6px track on Rule, filled in green, Marigold past the listing's warn band and Crimson past its danger band, with the figure right-aligned beside it in a slot of one width (4rem, 12px from the track) so a column's bars end together. A headroom ticks its limits on the track as the compact meter does: 2px ticks reaching 4px past the track above and below, the warn limit in Marigold and the protect limit in Crimson, the hues the bar fills in past each, its right edge on the track's end. The track keeps a 4rem floor, so a narrow window scrolls the listing rather than shrinking the bar. A reading with no ceiling keeps its figure in its slot and draws no track.
- **Leading edge:** in a page's card, every row's first content (a column head, a first cell) starts 24px in, as the card header's words do; a grip keeps its own 7px. In a drawer it starts 11px in from the table's left edge, where the grip's glyph starts. The trailing edge keeps 16px.
- **Acts hang from the first line:** when the thing a row names spans more than one line (a key's name over its fingerprint, an address over its IPv6, a value that wraps), the icons that act on it (edit, delete, copy) align with its first line, not the middle of the block. The first line is the thing's identity, and the acts belong to it. The same holds for a label and a status pill beside a wrapping value, in tables and fact lists alike. On a one-line row, centred and first-line are the same place.
- **Expanded row:** an inventory row opens into the uci sections behind it, one part each, in the order they stack (a network, its DHCP server, the device under them). Each part stands on a ledger line: the glyph of its kind (the network mark, the globe for a line to the internet, the server, and for a device the mark the chooser made it with: a bridge, a VLAN, a tunnel, a port), the kind of section in 14px/600 Ink, its name as the config writes it in 16px mono, a strong hairline running on, and the act that edits it at the line's end (a subsection act with the edit glyph) unless the row's own pencil already does. A DHCP server's state leads its facts with its mark, and its pool is drawn as the stretch from its first address to its last, filled by the share that is leased. Under the line, the facts in plain words in two columns 48px apart, the first half in reading order on the left (a network's addresses) and the rest on the right (its protocol, uptime and zone); narrow, the right column stands under the left. The section as `/etc/config` spells it is not repeated here: the drawer that edits it shows what it writes. Parts stand 40px apart. Nothing stands loose between them: no button between blocks. A fact that would only say "—" because the section is off (a disabled server's pool and leases) is left out; the part says it is off once.
  - **It unfolds.** The row opens as a fold growing from nothing to the height of its content (360ms, exponential ease-out), the trunk of the tree growing down with it, and its parts arrive in reading order: each ledger line draws itself from its name outward (480ms), and what stands under it, the facts one by one 30ms apart, rises the height of a hairline's air into place. Closing folds it back. Under reduced motion, or when a refresh puts an open row back, the row is simply open.
  - **Its acts open the drawer.** The interface's editor is the drawer, open over the listing: the row's pencil and each part's act (Configure DHCP, Edit device) open it in place, an act into one part opens it scrolled to that section, and the editor's own address shows the listing with the drawer open. The form's preview of what it writes stands at its foot behind a hairline, as the configuration card's editor (see Configuration cards). A refused save stays in the drawer over what was typed.
  - **What changes, rolls.** When the page's live refresh finds a fact reading differently (an uptime a minute on, a lease taken), the old value rolls out and the new rolls in where it stands, up for more and down for fewer, and its label goes to Ink for a moment and settles back.

### Fact lists

The facts about one thing (the router's system, an address family, the firmware's parts) are one list wherever they stand: an Overview card, a page card, a drawer. It is a `Properties` widget, never a two-column table.

- **Rows:** 40px, a label in 14px Meta and its value in 14px/500 Ink (a machine string in 16px mono), on one 24px line, 7px after the Hairline over the row and 8px over its own; the first row keeps 8px over its line and the last draws no Hairline, so the list starts and ends on lines.
- **Where the value stands:** on the right edge, for readings taken in at a glance (System's board, firmware, uptime); or beside a 96px label column 24px on, for values read line by line (an address, a version). On a phone a left-set label stands over its value.
- **Head (optional):** a 40px line over the rows on a Strong Hairline, as a column head stands: either a group's title in 14px/600 Ink with its protocol as a chip beside it ("IPv4" · dhcp), or the names of its two columns ("Part" over the labels, "Installed" over the values). The first row stands 7px after the Hairline.
- **Empty:** with nothing to list, the empty set's slot stands where the first row would, its sentence in it ("No prefix was delegated.").
- **What it becomes:** an offered value stands under what it replaces, the arrow leading it in Denim, the old value receding to Meta (the firmware a build would install, a package's next version). A fact the offer leaves alone says so on a line under its value.
- **Live:** a fact the page keeps current carries its key, so its value and its mark change in place; on the right edge the mark leads the value on its line.

### Uploads
A file the router has to check before it acts on it (a custom firmware image, a backup to restore) goes up in a dialog that says where it is.
- **Steps:** under the dialog's title, Choose, Verify, then the act (Install, Restore), with a short hairline between steps. Each step's packet is green when done, denim for the step in hand (Ink words, 600) and hollow for what's ahead (Meta words). The title says the trigger's words ("Upload a custom image").
- **The drop area:** the dialog's whole width, in the dashed Strong Hairline on Quiet Sand that marks a place. It holds the upload glyph in Glyph, the prompt in Ink 600 ("Drop a sysupgrade image here") and "or choose one from your computer" in Deep Denim, underlined. A file dragged over it turns the hairline Denim and the ground Denim Wash. Under the area, one Meta line says what fits: what it is, its type and its size limit ("A sysupgrade image built for Mono Gateway Development Kit · .bin, up to 128 MiB").
- **Going up:** the drop area gives way to the file itself: its name in 16px mono (breaking at its own hyphens), its size in tabular Meta, then a 6px Denim bar on Hairline that fills by scaling, and "2.4 of 6.0 MB" under it. The step line moves to Verify. Once the last byte is up, the dialog shows what the router is doing with it (the waiting mark, "Verifying firmware"). The answer takes the dialog's place on the step it reached: Choose again with the reason, or the act with the file's facts and the password that authorizes it (focus goes there). Installing firmware asks in caution's marigold.
- **Stopped:** a lost connection returns the drop area with a crimson band: "The upload stopped. Check the connection, then choose the file again."
- **A text file read in place:** a small file a page reads rather than uploads (an OpenVPN profile) uses the same drop area in a form, without the steps. The browser reads it into the form, so it posts as any field does, and a file over 32 KiB is refused under the area before it is read. Once read, the area names the file in 16px mono with "Choose another" under it, and the form around it can be drawn again from what the file says.

### What needs a package
A capability a page can't offer until a package is installed (encrypted DNS, a blocklist, a local resolver). It's offered where the need is, and installed there.
- **A settings row:** the hollow packet and the fact ("Queries leave in plain text") stand where a setting's name stands, with a sentence under it: what the fact costs the person, if it isn't plain, then what installing adds here and that it arrives off ("Installing adds encryption here, off until you turn it on."). The act sits directly below the fact and its explanation: a field-height quiet button with the download glyph that names its package ("Install https-dns-proxy"). It's never a band ruled off above and below, and never a link out of the page.
- **Installing adds a setting, never a change:** a package that would switch itself on as it lands (a DNS proxy taking over dnsmasq, a blocklist loading) arrives off, so the router does what it did before. Turning it on is a change like any other: it stages, applies and rolls back.
- **The package's own drawer:** the act opens, over the page, the drawer Packages opens from the package's row (what it is, its facts, Install). The page keeps its address. After installing, the drawer closes, the outcome says the package is in, and the page reads itself again, so the fact becomes the setting the package adds. The rows the install added come into view if they're out of it, wash once in Green Wash (2s, fading out from the halfway point, reaching 12px past the words on either side), and the first one's control takes focus.
- **Packages by name:** a search that reaches Packages without a view searches everything, not only what's installed.

### Collections
A short set of machine strings someone keeps by hand: authorized keys, a tunnel's peers. It's not a table. A handful of identities needs no rules, and every act on the set happens where the set is.
- **Items:** the identity in 16px mono Ink over one line of detail in 14px mono Meta (a fingerprint, an address), wrapping anywhere on a phone, each on a 24px line. Items are separated by their own 8px of air above and below, never a hairline, so each is a row's height.
- **Empty:** where the first item would stand, a slot: Quiet Sand inside a dashed Strong Hairline (the one dashed line in the app; it marks a place, not a thing), 2px corners, 16px sides, its frame on the grid's lines and a row's 40px tall. It holds the hollow packet and the set's empty words ("No keys are authorized.") in 14px Meta. It is inert, with no hover and nothing to press. The add stands 24px under it, and 24px under the last item.
- **Remove:** a 28px trash icon on the item's first line, named with the item for screen readers. Pressed, the crimson confirmation drops full-width under the item ("Remove this key?", **Remove key** · Not now), focus on the act, and Escape or Not now returns focus to the icon.
- **Add:** "+ Add a key" (a subsection act) at the foot unfolds in place into a mono box (cursor in it), the plugin's live reading of what is typed (a key reads as `ssh-keygen -l` prints it), and **Add key** · Not now. The reading stays hidden until there is something to read. A refused paste comes back open, as typed, with its reason under the box.
- **After:** the change runs at once and the page reads the router again, saying once what happened ("Key added.").

### Configuration cards
- **Placement:** always the last content inside the form, directly above Save and any companion actions, on pages and in drawers. Keep it out of side rails and above every action row; all editable sections come before it.
- **On a page:** the configuration card is a card of the page, 24px under the last section's, its file strip the card's header and its frame the editor's, with no divider over it. Save stands on the sand 24px under it.
- **Top inset:** in a drawer, the leading hairline stands 24px under the fields and the card 24px under the hairline, its frame on the grid's lines (a pixel out at the top; its left border on the content's line). Save stands 24px under the card.
- **Label:** the filename alone, in mono, heading the card. Keep live previews and copy controls with the card.
- **Editor:** a card whose text is in a grammar (`uci`: every `/etc/config` card) reads as an editor shows a file. The file heads it on a 40px Quiet Sand strip like an editor's tab (its Hairline on a line of the grid): the path alone in mono 14px/500 Meta, and the copy control at the strip's end. Under it the text on a field's white, each line on a 24px line, as an editor sets code, so a file of any length closes the card on a line of the grid, with 24px of air above the first and below the last, each numbered in a Quiet Sand gutter 40px wide behind a Hairline on the grid's line (64px past 99 lines; Faint, tabular, on the text's own 24px line, apart from the text so selecting it takes no number), and every token in its own ink: the keyword (`config`, `option`, `list`) Deep Denim, a section's type Deep Amethyst, an option's name Ink, its value Deep Green, the quotes and a comment Meta. Lines never wrap; the text scrolls sideways. The file's closing newline is not a line.
- **Change gutter:** a live card remembers what it read when its form was first touched and marks, in the gutter, every line the form has since changed (a 3px Marigold bar, the number in Deep Marigold: the stage's colour) or added (the same in Green). A typed value changes only its own token, so the text keeps its inks while the plugin's reading is on its way. A value with no grammar (a key, a token) keeps the plain box, its copy control inside at the top right.
- **Actions:** Save follows 24px under the card, with no intervening hairline. In a drawer the card's leading divider separates the fields from the configuration and its actions.

### Inputs / Fields
- **Anatomy:** a setting's row is one shape everywhere, a ledger of its question and its answer. The question stands on the left: the setting's name (14px/500 Ink), its UCI option as a mono key chip (what reads the option, where the plugin names it, joins the key after a middot: `mac · dhcp host`), while its change waits the stage's mark, a 3px marigold bar down the row's left edge from line to line (on the card's border, or a drawer's edge) with its word said to a screen reader beside the name, and on 24px lines under the name its description, the field's help and then its tip. Nothing is raised onto the name: a description is on the page, read before the control is touched, and one act folds every description away for someone who knows the settings. The answer stands in the form's answer column: the control, then its refusal band, then its note (what the value does now, such as the theme pick's "Takes effect immediately, for everyone who signs in to this router."), then the remove lane when the row belongs to a set someone adds to. Fields, lists, switches, settings rows (a fact that only reads among them, its value or pills the answer) and a conditional's gate all draw it through the one `verso-field` partial (`fieldFrame` in `internal/widget/field_frame.go`); a widget supplies only its control. The sign-in form, outside any row, uses the same label, box and band atoms.
- **Descriptions and their fold:** "Hide descriptions" folds every description on the page away at once, for someone who knows the settings, and "Show descriptions" brings them back. It is a quiet labelled act (Lucide's `fold-vertical`, then `unfold-vertical` while folded) leading the acts on a page's heading line and standing before a drawer's close, and it stands only where its page's body or its drawer has a description to fold, so Devices shows none. A folded description closes over 200ms (at once where motion is reduced) as it fades, so the rows close up by whole lines and stay on the grid; it still describes its control to a screen reader. What says which thing an item is (a grouped file's detail), what a value does now (a note), a refusal and a card's own sentence under its title never fold. A stacked row (its answer under its label, as a list, a code area or a fused box too wide for the column stands) says what it is for under its answer, after its refusal and before its note, never between the name and the box it names; such rows are rare, so that sentence stays in view when the rest fold. The state is the browser's: the page root carries it, and a year-long `verso_descriptions` cookie keeps it, which the shell reads back so every page, and every drawer opened on it, arrives as the last one was left. It never hides a setting, only the words about it.
- **Layout:** every page form and drawer is a ledger: the question on the left and every answer in one 272px answer column (`--spacing-answer`) 24px after it, so a form's answers read down one edge. A 32px control centres on its name's 24px line, its value on the name's baseline, reaching 4px into the air over and under it: the one place a control's top and bottom borders stand off the grid's lines, because the words on a row share a baseline. A switch is an answer like any other, its box in the answer column. An answer too wide for the column stands under its question at the row's full width, and a row narrower than 36rem stacks, the answer straight under the question. The ledger replaced labels over controls with checkboxes before their labels: that card had two left edges, every control hugged its left third, radio choices were drawn as settings of their own, and what a setting does hid in a tooltip. One answer edge, one baseline per row and help written beside the setting are why it is a ledger.
- **Row spacing:** rows that follow each other meet on a Hairline: 16px under the one's last line, the line on the grid, the next name 15px after it, so a row of one line is 56px with its Hairline. In a card the Hairline runs from border to border while the words keep the card's 23px inset: a line that touches a card's borders always separates questions, and anything inside a row keeps its lines inside the gutter. A card's header seam and its frame are lines like any Hairline, so a card whose body opens on a row stands its first name 15px after the seam (or the headless frame), and one whose body ends on a row closes 16px under its last words, as rows stand around a Hairline, unless that row is stacked and ends on the sentence under its answer, which closes the card's own 24px as words do; after a block (words, a notice, a table) a row keeps the block's 24px, and a card that ends on a block or an act keeps its own 24px. Rows carry no fill and no hover tint: a fill would flip as a revealed row appears and would take the chips' own fill, and a tint would offer a click the row does not answer. A field and the act it feeds (a search and its Search) stand side by side, 8px apart, each control whole with its own corners and border; a button never sits inside a field. A key typed to unlock a destroying act (Factory reset's hostname) is an ordinary field row, the act 24px under it in the danger dress (the form's `tone`), held shut until the box says what it shows.
- **Fused values:** related values typed into boxes (rate and burst, address and netmask, a pool's first address and size, the web ports, a new password and its repeat, a service record's port and ranking) are one setting and one control, and so is a value with the dropdown of units it is counted in (a DDNS check interval: `10 │ minutes ▾`). One question covers every part (the set's label, or for a value and its unit the value's own name), one key chip names every option joined by a middot (`synflood_rate · synflood_burst`), and one `staged` mark stands for the row. One box holds the parts: the field's frame and states, parts split by a Hairline, each part as wide as its value's measure (96px for a number, 256px for a secret, 176px otherwise) plus the words after it. A unit dropdown is bare inside the box, as wide as its longest option, the chevron 12px in from the box's edge. A secret part is a password box: never reflected, sans 14px. After a value stands its unit, and in a labelled set each part's own name (`5060 Port │ 10 Priority │ 5 Weight`): Meta sans 14px inside the frame, never fixed padding, so the box fits its words in every language and reads without a tooltip. A press anywhere on a part puts the caret in it. Each part keeps its own name for a screen reader, and a refused part gets its own band naming it ("Burst: …"). The description is the set's help, then each part's led by the part's name; a part that alone says anything speaks for the row unprefixed. The box stands in the answer column when its parts fit it, and under its question at the row's width when they do not (three numbers with their names do not). A single value with a unit is the same box with one part.
- **Joined values:** a word between values always splits them. Parts that read as one sentence (a rule's Path, `lan to wan`; its clock window, `09:00 to 17:00`; its date window) are never fused: each stands as its own control, side by side, with the word between them in Body sans 14px and 12px either side, under the row's one label, one key chip and one mark. A dropdown measures its longest option and stays a dropdown however few options it holds; a typed clock time or date is mono at 128px, and a native clock control (a device's curfew Hours) is the same 128px. A native clock wears the Lucide clock in Meta, 16px, inset 12px from the right edge as a dropdown's chevron is; the platform's own glyph stays underneath, invisible, so a press on the clock still opens the platform's picker, which keeps its own look. Each part keeps its own name, change and refusal band ("Ends at: …"). The row wraps at a phone's width rather than squeezing its parts.
- **Added conditions:** a condition a rule carries is one named setting. Its name line is a row's label line: the condition's name (14px/500 Ink), the key it is listed under in the picker as a chip, what it is for raised on the name as a tip, and the quiet remove glyph at the line's end on the form measure's edge, taking the whole condition away; the line stays 20px tall. A condition of one control draws it straight under the name, as a label's control, its own label kept for a screen reader only and its staged mark worn on the name. A condition of several parts sets each part's label in regular Body weight, the first straight under the name and each after 24px of air, as fields stand, and a part that reads as a sentence is a joined row (Rate: `1000 per second`). The conditions a rule carries are one block: they stand in the quiet card the Action reading's parameters wear (a Hairline frame on Quiet Sand, 24px in, on the grid's lines, 24px inside its edges), which is there only while there is a condition in it. Two conditions meet on a Hairline across the card 24px from each, and the seam says how they join: "AND", a Meta kicker (12px/500, tracked, uppercase) centred on the rule on the card's own sand, so the block's whole logic is read where the eye crosses from one condition to the next. The block's lede says only what the card cannot: several values inside one condition are alternatives. "Add a condition" stands under the card with no rule of its own; with no condition, the card goes and the empty note returns. An include/exclude condition keeps its Exclude list behind a quiet reveal, a plus and "Exclude some" in Deep Denim 14px/500 on a 20px line 24px under the list, as a field's label stands, which unfolds to the list in place and steps aside; it arrives open whenever the condition already excludes something or an exclusion was refused, so nothing a rule says is ever folded away. A schedule is one set of controls wherever it is asked: the When reading and the Schedule condition draw the same days strip, windows and clock.
- **Field sets:** settings that belong together but cannot fuse into one box stand under one head. One setting asked of several things (which files fw4 loads) is the head (the set's label, the option chip once, the set's description), then a row per thing: its name, in mono where it is a path (`verbatim`), its detail under it, which never folds away, and its box and On/Off in the answer column, with its own `staged` mark. A labelled set of fields that cannot fuse (one holding a dropdown that is not a unit, a list or a switch) is the same: the head, then each field as its own row. No hairline runs inside a set: the first row stands 8px under the head's last line and each 24px after the one before, and the set meets its neighbours on the ledger's hairlines. The head is all question and draws no answer, so a stacked set leaves no empty control's height under it. A set with neither a label nor a description is no set: its fields join the form's run as rows of their own. A state nothing on the page changes (a folder fw4 always reads) is a locked checkbox: set or clear in Inert sand, posting nothing. A set of files or folders is never a table unless it has columns to compare.
- **A table among fields:** a set with columns to compare that stands among a form's fields (Custom rules' rule files, dnsmasq's option files) is one row of the ledger. Its group's name is the question, and its add (the quiet labelled act, "+ Add file") stands on the question's line at the row's end at every width, ending where the rows' edit acts end. Its rows are the answer at the row's width: a Hairline 8px under the question's words and one between rows, each 40px row on the grid, the words on the row's gutters, and no band, no fill and no tint under the pointer. Its lines stay 24px inside the card's borders, because a line that touches a card's borders separates questions and nothing else does; its last row draws none, so the hairline to the next question follows 16px after its words, as between its rows. With nothing in it, the set's empty slot stands 8px under the question. The same table outside a form is a listing, its group a band.
- **Choosers:** a choice of what a page shows rather than a setting (Hardware's "Show chart for [Tctl] over [Last 24 hours]") stages nothing and is no question of the ledger: its name stands over its controls at every width, as a control band's controls do.
- **Value chips:** a field of several values (a protocol list, addresses, ICMP types) is a white field box holding one chip per value, 24px tall with 4px of the box showing round it and between chips, so the box keeps a field's 32px; wrapped rows of chips stand 16px apart, so each row adds 40px. The value is in mono 16px/500 Ink. The chip is Mid Sand behind a Strong Hairline, a step darker than a citation chip, because it sits on a field's white; its remove glyph is 20px, taking a Hairline-tone fill on hover so it shows on the chip.
- **Related fields:** opt into the shared `grid` with `style:"form"` for values entered or compared together: new password and confirmation, HTTP/HTTPS ports, rate and burst, range endpoints, address and netmask, a value and its unit. Plain typed values, plain secrets and a value with its unit dropdown fuse (Fused values); parts a word joins stand as a sentence (Joined values); a set that can do neither is a field set (Field sets). A form's fields never stand side by side in columns: a question has one answer, in one column. The grid's columns are for blocks that are not settings (words beside a field), which keep the 24px gutter and follow the grid's own width: one column below 36rem, two from 36rem, a declared three from 48rem. Current password precedes the new-password pair.
- **Section submit spacing:** when a section saves its own fields, the action row stands 24px under the last control or error, as a field stands. Actions directly after a configuration card keep the same 24px.
- **Time-server list:** on System → General, list rows have a 12px horizontal inset, matching the input's text inset. Values use 16px mono at 400 in Meta. Keep the standard 14px/500 Ink field label; the inset establishes the hierarchy. Apply the same treatment to existing and newly added rows.
- **Style:** Canvas fill, Strong Hairline border, 2px corners, field inset shadow, 32px tall (`h-control`, the same as every button). Sans 14px for words, mono 16px for identifiers.
- **Width by content:** in the answer column a dropdown and free text fill the column's 272px, and a value with a measure of its own keeps it at the column's left edge: a number 96px, a port range or a clock 128px, an address 176px, a prefix 224px; a secret, with its reveal, is the column's width. An answer the column cannot hold stands under its question at the row's full width: value chips, rows of values, a code or text area, a weekday strip, an `endpoint` field, and a fused or joined value whose parts outgrow the column (a typed part counts its measure and its unit's word, a clock 128px, a dropdown its longest option and chevron, a joining word 12px either side), so a rate per period stays beside its name and an address with its netmask goes under it. Where a row stacks (narrower than 36rem), the field-semantic measures hold: `host` is the full width and covers hostnames, local domains and upstream DNS servers; `secret` is 24rem (384px) for passwords and shared secrets, including their reveal control; `endpoint` is 32rem (512px) for compound domain/address rules, resolver URLs and address/port listener lists; `choice` lets the native select size to its longest option; `prefix` is 14rem (224px), enough for a compressed IPv6 prefix through `/64`. Every measure is capped at the available width, and every fixed one is a pixel more than its round figure (an address 177px, a number 97px, a drawer form 641px), as a column is, so a field that starts on a line ends with its right border on one. A shared secret's reveal is a control's square (33px, giving back its pixel as a field does) 8px after the field. Free text fills the available measure; fixed-format identifiers retain their compact widths and numbers, including DNS cache size, use 96px. Lists include their values and Add/Remove controls in the same measure. Width follows the field container, so a narrow drawer stays usable even on a large screen.
- **List fields** (a UCI `list`: time servers, upstream servers, extra hostnames, forwarding): one value per row below the label, 16px mono in Meta at the regular weight, led by the packet in the same ink, with its 28px remove 12px in from the right, riding the value's first line. The packet stands centred on the first line and in the middle of the grid's first column, and the value starts at the next column line. Values are divided from each other by a Hairline, never from the label: the first row's goes clear and keeps its pixel. Every list wears this one look; no page restyles its own. A row writes the value on 24px lines, 7px after its hairline and 8px over the next, so it is 16px more than its lines, never a fixed height. The add box and its Add stand 8px under the last row, as a control stands under its label. With nothing in the list, the add box sits directly below the label and no empty hairline remains. A value too long for the column wraps after its own separators: slashes first, then dots, then colons (`/corp.example.com/` over `10.66.0.53`). It breaks anywhere else only when one part alone is wider than the column. Its remove hangs from the first line.
- **States:** only the border changes — Strong Hairline at rest, Faint on hover, Denim while focused. A refused field stays the one that is wrong while it is corrected: Crimson at rest, Deep Crimson under the pointer, and a 2px Deep Crimson edge while focused (an inset line, so the box never grows), with its reason under it on a band of Crimson Wash (12px side padding, 10px vertical so a line of it is 40px, 2px corners: the crimson square, then the words). The band is the answer's own: it stands in the answer column 8px under the control's line, before the note, whatever the description beside it says, or under a wide answer at the row's width. The band is the same under a field, a key's box, a list's box or a checkbox. A refused checkbox keeps its own look (a Crimson edge on a Denim fill fights itself); the band alone says it. A refusal is never a notice or callout set beside the control. A page that comes back refused opens at its first refused field, in the middle of the view with the cursor in it, never at the page's top. The first change to its value answers the refusal, so the box returns to the Strong Hairline and Denim focus and the reason goes; submitting asks again. Placeholders are Meta, never lighter.
- **Refusal navigator:** a refused page carries no "check the highlighted fields" band. While settings stand refused, a small tally pins itself under the top bar, 32px tall like a button, centred over the form holding the refused settings (not the wider content area); a page that arrives refused shows it still, and it slides in only when a refusal appears later: the crimson square, "2 settings refused", and **Next ↓**, which centres the next refused setting and puts the cursor in it. The shell counts the fields themselves (an invalid control or a standing refusal line, one per setting; a fused box's parts count apart), so no plugin writes the sentence. Each correction clears its refusal and the count turns over; at none the tally turns green, "Ready to save", and folds away. A form's own error band is only for a refusal no field owns, and keeps a dividing line's 24px below it.
- **Checkboxes:** a switch is its row's answer: the box in the answer column, centred on the name's line, and 8px after it the word for its state, On or Off, in Body 14px; the name, the box and the word all flip it, and the word is hidden from a screen reader, which hears the box's own state. A table's toggle cell is the bare box. 16px, 2px corners, Canvas with the inset shadow and a Glyph outline; checked is Denim with an On-fill check. Every switch in the app is drawn as a checkbox, because every change is staged. A membership set (which networks a zone covers) lists its boxes down the answer column, each 8px before its option.
- **Choice colours:** radios, checkboxes (including table and log controls) and multi-select chips share `--color-choice` (Denim) in `palette.css`; `--color-choice-border` (Glyph) defines custom empty checkbox outlines. Selection keeps a visible radio dot or check, and keyboard focus gets a separate outline. The selected fill contrasts 5.4:1 with Paper and 5.7:1 with white checks or chip text; the empty outline contrasts 3.5:1 with Paper and 3.3:1 with Quiet Sand. Native controls retain platform rendering and forced-colour support. Labels keep their existing text colour.
- **Single-choice sets:** up to three choices use visible native radios run across the answer in their order, 16px apart; four or more use a native dropdown that fills the answer column (and follows its longest option where a row stacks). Each radio is 32px (its 24px line, 4px over and under), so a run centres on the name's line as a control does, the 16px circle 8px before its label; a choice that does not fit wraps onto the next 32px line, so a group of any count is a multiple of 8px. Drawn down the column, choices read as settings of their own. Apply this throughout page forms and drawers, including Accept / Reject / Drop. The shared `RadioOptionLimit` constant in `internal/widget/field.go` owns the cutoff; change it there to adjust every form. Radio groups keep their field label, keyboard navigation, help and error associations. Segmented chips are reserved for selecting multiple members, such as weekdays; boolean settings keep their checkboxes.

### Navigation
- **Rail:** 288px, no fill of its own, one hairline against the page. Top-level rows are 36px and stand 40px apart from 24px under the top bar; each carries a 16px Lucide icon in Glyph (Ink when active) centred 32px in, its label 56px in, and washes to Mid Sand and slides 4px right on hover. The row you are on is the only surface: Quiet Sand with a 2px Ink bar on its right edge, against the page it opens. A parent whose sub-page is open goes bold Ink without the surface, and its sub-pages hang off a hairline under its icon, with no icons of their own; the open sub-page carries the 2px Ink bar on the same right edge.
- **Rows that open:** a row with subpages is a group: pressing it opens its subpages under it in place and closes the group that was open, and pressing it again closes it; it never leads to a page itself. One group is open at a time, and a page opens with the group that holds it. It says so on every page with a 16px chevron in Meta at its end, pointing right while closed and turned down over its open branch, one chevron turned on the rail's easing as the group opens. A row with none carries no chevron and stays a link. Network's and System's pages are the shell's; a plugin lists its subpages in its manifest (`"pages": [{label, path}]`). A plugin that says only `"pages": true` keeps its row a link, its subpages opening under it on its own pages.
- **What waits:** a row's end says only what waits for you there, never how things are (the homepage says that), and says nothing when nothing waits. It is a word or two in 14px/500 with tabular figures, led by the rail's square and tinted with it: System says what is ready to install in denim, as update news is everywhere ("15 updates", "1 update", or "New firmware", which speaks for any packages too, since a firmware upgrade brings its own). The row's title carries the whole sentence ("15 package updates are ready to install."). The row's name never gives way to these words; they shorten instead.
- **Page change:** the rail carries itself from the page you leave to the page you open. The marker glides from where you were to where you are going, under the words it passes; the rows under a branch that opens slide down to make room while its hairline draws down and its subpages drop in, 30ms apart in reading order; a branch you leave is gone at once as the rows under it close up. One easing, the unfold's (360ms, `cubic-bezier(0.22, 1, 0.36, 1)`); a row that gains or gives up the weight swaps it in 140ms, so its words never read doubled. The page itself appears at once and nothing on it moves, and the rail never holds back a press. It needs a browser that carries a page change (Chrome, Edge, Safari) and the rail beside the page; on a phone, under reduced motion, or in another browser, the page simply loads. Over HTTPS, a row's page is fetched as the row is pressed.
- **Pending changes:** the one row that comes and goes. While changes wait, it stands first in the rail, a row like the rest (40px, flush with Overview under it, no space of its own) and in no group: the `list-todo` icon, "Pending changes", and the count at its end in tabular figures, words and icon in Deep Marigold on Marigold Wash; on its own page the marker is a plain Marigold bar. It is the rail's second word of status beside System's update news, and keeps to the same rule: it says only what waits, and with nothing staged it is gone. It stands in the rail rather than the top bar because the rail is where a person looks for a place to go, and pending changes are a place: a page holding all of them. Under the pointer it slides 4px right as every row does, keeping its wash; otherwise it is still, showing the count the stage holds with nothing animated.
- **Top bar:** Canvas, 56px. On the left, the router: the hostname in bold mono and its maker and model; the hostname truncates first. On the right, alone and aligned to the content column, the session: `root | Log out`.
- **Mobile:** the rail becomes an off-canvas drawer behind a menu button, over an Ink scrim. While changes wait, the menu button wears Marigold Wash and says the count beside its icon ("☰ 3"), so the stage is in sight with the rail closed.
- **Colophon:** every page's last line, and only a log goes without one (a log runs on as long as the router writes). It is a 56px Mid Sand band, a step down from the page's ground (the 28px copy plus 14px above and below) with its Hairline drawn inside its top edge, running from the rail to the window's edge. On a short page it stands at the bottom of the window, the ground above it taking every pixel down to it, so it sits exactly where it sits at the foot of a long page and never moves between pages; on a long one it stands under the last of the content. Its words keep to the page's column, level with the h1, and end under Log out; on a phone the page's 16px gutter is its side air. It is always one line: a fact with no room ends in an ellipsis, since the copy carries every fact whole. On the left is the OpenWrt release at 500, then its revision (from 640px) and its target (from 1280px), all in 16px mono Meta with Meta middots between them. On the right is the Verso build, then a 28px copy icon ("Copy for a bug report") that puts the release, revision, target, board, kernel and Verso build on the clipboard, one labelled fact a line and in English. The line answers the copy by washing each fact it shows in Strong Hairline, never a hue, with 6px of padding either side of its words, one after another from the left (70ms apart, 1200ms each), with the words in Ink under the wash, and the icon turns to a green check. Under reduced motion only the check shows.

### Staging
Save stages a change; nothing happens on the router until it is applied from the Pending changes page. Marigold means exactly that, waiting, and nothing else says a staged save happened, because it has not.
- **One surface:** what waits has one place, the rail's Pending changes row (Navigation) and the page it opens. A page holds every change at once, each with a way back to where it was made; a drawer over the page you were on hid that page, and a chip beside the hostname read as part of the router's name rather than as work waiting.
- **No band:** a staged save puts no notice on the page, green or otherwise. The row is what says a change waits. Only a plugin's warning or refusal about the change still speaks, in its own tone.
- **What waits, everywhere:** the router holds the stage, so every setting whose change waits carries the marigold "staged" mark beside its name (the stage's 6px square and word) on every visit, from any page, until the change is applied or discarded. A control finds its change by the option's full address: its key, and the config and section it lives in, which the plugin states once on a form or section or on the control itself. The mark is named by that address, so the Pending changes page leads to it.
- **Save puts it there:** an edit not yet saved changes nothing outside its form; the Save under it is the act, and the row answers only once Save has staged something. With nothing pending, the row first appears then. A Save reloads the page, so the row simply arrives with the new count, with nothing animated; an act that stages in place (a drawer's Save, a row's one-press act) sets the count where it stands. Nothing previews an edit and nothing flies to the row: an edit shown in the rail before Save reads as if it already waited, and an object crossing the screen to a counter is no convention people read. A screen reader hears "Pending: <words>" when an act in place stages a change.
- **Where it went:** the page comes back where the person was (a page form's Save at the same height), its changed rows already marked.
- **Discard and Apply show at once:** the moment the stage changes under an open drawer, the page behind it shows the settings as they are on the router, with the marks gone and nothing reading as unsaved. It isn't left stale until the drawer closes. Only a change to which rows a listing has waits for the page to be read whole, on close.
- **The page:** the heading says the count ("3 pending changes") and nothing else, with nothing under it: a lede would be read once and never again. It lists changes as System › Changes does, so it stands on the wide column every page of listings takes, with changes or without, its cards the width of Firewall rules' listing. The When you apply card comes first, because the outcome belongs before the act: when a change touches the interface this browser reached the router through, a Marigold "May disconnect you" callout opens it, naming both addresses, with the packet and no buttons (the acts stand under the changes); then the apply's steps, numbered because they happen in that order, one line each: "Saves 3 changes.", "Reloads DNS & DHCP and Firewall." (read from procd's own reload triggers, Network first, four named at most, then "and N more"), "Keeps them if this page checks in within 30 s."; then the optional Note, its description under its box saying what it is for and where it goes ("Say why you're making these changes. It's kept with them in **System › Changes**.", an instruction, as a description of a box to fill says what to put in it, the page it names in bold so it reads as a place), its hint "Optional", never a summary that reads as a note already written. It is an exception to the ledger as the Hardware graph's "Show chart for" is: its label stands over its box, which takes the free-text measure, since a note is a sentence and a one-word question beside it would leave the card's width empty. Then one card per page the changes were made on, titled by that page's heading with no icon and no tally (the headings already tell the cards apart), with "Open page" and, for more than one row, "Discard these" in the act style. A row says the change in the Changes page's words and its status verb, then its acts: go to it on its page, discard it, its config lines. It says no age: a pending change is minutes old, a time kept only in the session's memory reads "—" after a restart, and the card already says where the change came from. Under the last card, "Apply 3 changes" and then Discard all, where a page form's Save stands (on the sand 24px under the last card, as one card stands under another): they come after the changes because the changes are what a person reviews before either.
- **Apply says only what happened:** Apply never asks first; the warning is already on the page. Each step says only what the router reports: Saving, then Saved; the reloads Started, never done, since procd reports no finish; the check-in counts the window down to Kept, or to Put back in Crimson. Kept, the page reads "Changes applied" with how many and the time, and no notification says it again: the page is the outcome, in view where the act was made; put back, the router stages the changes again and the page opens with the rollback callout saying when. Discard all is the secondary act, as a Cancel is: nothing was applied, so the router runs as it is either way, and it lets the stage go without asking and in no crimson. After either, the empty page leads back to the page of the latest change; opened with nothing on record, it offers no way back.
- **One verb, three reaches:** letting changes go is one act, named Discard wherever it stands: "Discard this change" on a row, "Discard these" on a card, "Discard all" under the cards. A row or a card goes without the rest; the notification offers "Put it back" once. Undo is System › Changes' word, for what was applied.

### Drawers
- **Shell:** a panel from the right on Paper, behind a Strong Hairline and cast with the drawer shadow. It slides in over 300ms (ease-out) and out over 200ms (ease-in). Every drawer is 760px wide, whatever it holds (a form, a chooser, a reading), so the frame never changes size from one object to the next. On a narrower screen it takes the full width. Its content column is the room inside its 40px insets rounded down to the grid and a pixel more, as a page's column is, the end inset taking what the rounding leaves, so a frame across the drawer ends on a line at any width; bands and rules still run to both edges. Header is a 56px Quiet Sand band (a 28px line plus 14px above and below, its Hairline drawn inside its bottom edge) holding only the title ("New rule", "Edit Allow-Ping") and a 28px close.
- **Tabs:** a drawer whose object answers more than one question carries a 56px strip under the header, one tab per reading, each its name alone: no chip of where the object stands beside it, since the reading answers that. A tab names a reading as a label names a setting, so it is set at the labels' 14px, and the 18px section heading under the strip stays the clear step up. Each tab stands the strip's full height (a 28px line plus 14px above and below), so its underline sits on the strip's Hairline. The reading in view is Ink at 500 over a 2px Deep Denim underline; the rest are Meta. A device's drawer, whose tabs the plugins contribute, draws the same strip.
- **Sections:** each drawer starts its own heading hierarchy, independent of the page section that opened it. Full sections use the standard Quiet Sand heading band, edge to edge, with an 18px heading and any lede inside. A drawer has no visible grid but keeps the page's geometry, measured from its chrome's Hairline: sections, bands, fields, rows and cards stand exactly as they do on a page, and blocks inside a drawer form stand 24px apart. Headings, ledes and fields retain their 40px horizontal inset; subsection and list dividers stay inset. The opening section has no leading divider: the chrome's own Hairline (the header's, or the tab strip's) is its rule, and its title stands 40px under it, as under any rule. The object's own state and name (Forward is active, Name) open the drawer under no heading, since the drawer's title already names the object, and the next section's rule stands 40px under that untitled block, as every section's does.
- **Configuration card:** the drawer reads as form name → fields → configuration card → actions. The filename alone labels the card, above the code; it has no section heading, marker, “Written to” prefix or repeated explanation of staging. A full-width hairline separates it from the fields; the filename and code keep the content's 40px horizontal inset. Live previews and copy controls stay part of the card.
- **Footer:** when a configuration card ends the form, Save follows it with a small gap and no intervening hairline: the card and the act that saves it are one unit. Other drawer footers keep their inset Hairline. Removing an object belongs to its listing’s trash action, which opens the danger confirmation over the page (Confirmation) naming the object and its consequences, with the destructive act and the way back named for the object ("Keep rule"). Edit drawers carry no duplicate removal action.

### Appearance
The theme is picked on System › General, in an **Appearance** card between the router's settings and Time, placed there by the system plugin's `appearance` slot and drawn by the shell. One dropdown, keyed `theme`: Sand following the browser, Sand always light, Sand always dark, then every other theme by family and name. Under it, in the card's body ink, one line: "Takes effect immediately, for everyone who signs in to this router." A pick is written at once, outside the stage: the page repaints in place, nothing on it is lost, the Pending changes row does not move, and Save above stays as it was. A refusal is the top-right notification, and the dropdown goes back to the theme in force.

### Overview cards
The Overview (ADR-022) is made of cards as every page is, and is the one page whose cards the operator arranges: each holds one kind of reading (Internet, Ports, Tunnels, Traffic, Resources, System, and Hardware on a board whose profile lists sensors), in two columns, each card folding and moving. Every other page's cards stand in one column and stay as they are. Cards are a page's subjects; inside a drawer, a frame still marks only an object instance.
- **Ground and frame:** the page's ground is Quiet Sand, from the masthead's place to the colophon. A card is Paper in a single Strong Hairline frame with 2px corners, flat at rest. Cards stand 24px apart in each column and the columns 24px apart, each column stacking its cards at their own heights; below a page two cards wide they read as one column, the left's cards first. The verdict is the page's h1 in Headline, its accent in Deep Green, with no kicker over it; the router's clock is the System card's Local time, its zone after it.
- **On the grid:** the page draws no grid, but measures on the 8px grid from the top bar's Hairline and the rail's border, as every page does; the gutter is 40px (16px on a phone). The heading line is every page's: 32px, 24px under the top bar and 24px over the cards, Arrange centred on it. Each column is its share rounded down to 8px and a pixel more, the next column 23px on, so every frame's four borders are lines; the shelf is a frame 24px under the heading line, its bottom border on a line, and the cards stand 24px under it less that pixel.
- **Header:** 48px with its Hairline, 24px in: the card's 16px Lucide glyph in Glyph, the Card title on its 24px line, 11px after the frame and 12px over the Hairline, then, while the card is closed, its one reading in Meta (truncated, tabular): the single reading most worth a glance, never the status again, and never a reading that changes which it is from one second to the next. A card's controls stand in its body, beside what they change (the Traffic card's window on its legend's line); the uplink's device is the Internet card's to name. At the end stand its status, the 6px packet and the word at 14px/500 in the tone's Deep step (Body when neutral, the packet hollow), then 28px icon acts riding the line: the door to the card's page (arrow-up-right) and the chevron. The body's Hairline, or the frame's bottom when closed, is the next pixel. A card of readings shows a status only when a reading is out of band. Clicking the header opens and closes the card; the chevron turns a quarter toward the title when closed.
- **Body:** behind a Hairline, 24px in, its content 23px after the Hairline and 24px over the frame, so its lines fill bands of the grid and the frame's bottom lands on a line, and the next card stands 23px on. Its facts are grid rows (40px: the 24px line, 7px after the hairline over it, 8px over its own; the last drawing none); each address family's name heads its rows as a row of its own over a Strong Hairline, and the families stand 24px apart. The Hardware card is one list with no head, three rows at most (the CPU, the main power rail as "9.8 W · 20.10 V · 0.49 A", the main fan), its values on the right edge so a long name keeps its line, and a reading with none to give writing "—" in its place. A list that starts or ends the card gives up its row's own air there, so its first and last lines stand the card's 24px from the frame; one that ends the card draws no last hairline and keeps the pixel as air. A listing (Ports, Tunnels) stands in the card's 24px inset, its hairlines stopping there as a fact list's do, its first and last columns with no indent of their own; its column names are headed as a fact list is, 14px/600 Ink in sentence case, 8px over and under their line on the Strong Hairline. A gauge is its label's line, the figure on 40px, the bar centred on a line and the detail's line, two to a row 40px apart, the rows a Hairline apart with 24px of air either side, the rule running the card's width. The Traffic card's graph is the chart (Charts) in two lanes, download over upload, on 24px cells; its window is the compact dropdown in the card's header, before the open-close control, since it governs the whole card. A closed card eases its body shut over 300ms and fades its line in where the body was; under reduced motion both are instant.
- **Arrange:** a glyph act on the heading line (the 28px icon button, `layout-dashboard`), its tip "Arrange cards" opening under it, clear of the top bar; while arranging the same square fills in Denim with a check, its tip "Done", the one filled thing on the line. It keeps the one name and says pressed, as a toggle does. Arranging rules each column off with a dashed Strong Hairline 8px outside it, on the grid's lines on all four sides and with a card's 2px corners, puts a grip before each card's glyph and a hide (x, crimson on hover) where the door and chevron stood, and opens the shelf under the heading: a dashed Strong Hairline box on Quiet Sand listing the hidden cards as chips that wear their card's packet, with "Reset layout" in Deep Denim at its end. A card in hand lifts with the dialog's shadow and leaves a slot of its own height (dashed Strong Hairline on Quiet Sand) where it would land.

### Router's panel
The router's ports drawn as its case shows them (device-panel): the board's own art (the Mono Gateway DK's dark rear) or, for a board without art, a generated panel in the page's neutrals. It opens the Overview's Ports card, the listing under it in the panel's order, and the board card of System › Hardware.
- **Connectors:** one family. RJ45 and SFP+ share a 48px outline, a half-width lip under it and two 8×4 LEDs beside the lip, link on the left and traffic on the right. An RJ45's black mouth follows the outline and lip 4px in, its eight contact pins in Marigold at half strength along its top; an SFP+ cage stands 8px lower at the top, its black mouth holding the connector socket a little under its middle. Ports stand 8px apart, 16px where the kind changes.
- **Generated finish:** a Quiet Sand chassis in a Strong Hairline, Paper jacks in a Strong Hairline with Ink mouths, unlit LEDs in Inert; each port's name in mono Ink and its speed (1G, 2.5G, or a dash without link) in mono Meta under it, on 24px lines.
- **On the grid:** drawn at its own size wherever it fits, so every edge it draws is on a line; where it does not fit it shrinks to a whole number of 8px steps tall, never past its own size. It stands at the content's left edge wherever it is drawn, its faces' switch and legend with it, flush with what stands under it: the listing on the Overview's Ports card, the board's facts on the Hardware page.
- **LEDs:** the link LED is lit green while the port has link. The traffic LED, amber, shows how busy the port is from its packets per second: dark when idle, an occasional blip under 10, a steady blink under 1,000, a fast flicker under 100,000, a near-solid flicker above. Its dark beat is the lit LED dimmed, and each port starts its blink a fraction of a period late, so busy ports never blink in step. Under reduced motion it stays lit while traffic flows.
- **WAN and focus:** the WAN port's outline is blue and "WAN" stands over it at 600 in the same blue, the hardware blue (#6ea8e8) on board art's dark finish and Denim on the generated panel; its name stays as every port's. The generated panel keeps the 24px line over its ports for the tag. The port the keyboard is on has its outline in Denim at 2px.
- **Detail:** no legend. Each port is reachable by keyboard and named by its name, link and speed; hovering or focusing it shows its detail (name, role, address, link) in the dark tip above it, or below it when there is no room above.

### Hardware page
System › Hardware is the box itself, one card per kind of reading, each reading once, on the wide column every page of listings takes.
- **Board card:** titled with the board's name (the profile's, else the model), the panel at its left edge, and under the panel what the box is made of in one fact list, read down: Processor (the chip, its cores and top clock), Memory, Storage (the boot disk's size and kind) and Board name in mono. A fact the box cannot answer is left out.
- **Temperature history:** the Temperatures card opens with a field, "Show chart for [CPU ▾] over [Last 24 hours ▾]": the reading (the table's rows in order, then each SFP module) and the window (24 hours, 7 days, 30 days, a year), the CPU's day chosen. The graph (Charts) stands 24px under it on 32px cells, then the table; choosing redraws the graph alone. A reading never recorded says so in the graph's place.
- **Readings:** Temperatures, Power and Cooling are each one table, the profile's headline reading first: the CPU, the input rail (tagged "input"), the main fan. Every row names the reading as the profile does and its Source as the kernel does, in mono, so the machine's name stands beside the person's without a second table. A temperature's Temperature is a headroom meter to its protect limit, ticked at both limits; Cooling's state is its last column. No card heading carries a total or a count its rows already state.
- **Fiber modules:** each SFP module is a headed fact list, "SFP+ module" with its cage as a chip on the strong hairline, the Overview's address-family head; never a card inside the card.
- **Heading:** "Hardware" while all is well; a hot CPU or a stopped fan retitles it in the warning or danger tone, in the Overview Hardware card's own words.

### Charts
A reading over time is drawn one way wherever it stands: the Temperatures graph on System › Hardware, the Overview's Traffic card, a device's Usage in its drawer.
- **Grid:** the plot stands over square cells in faint Mid Sand, four cells tall, so each of its four rules is a labelled quarter line: 32px cells on a page's card (a 128px plot), 24px in an Overview card or a drawer (96px). Its width is the most whole cells that fit beside the 40px scale column and cut into a whole number of the window's steps (Time axis), so every time label stands on its line, and every rule lands on the page's 8px grid. The left edge is the first column's line; the right edge and the bottom border, the zero line on the grid line under the plot, draw their own. A zero reading rides one pixel over the bottom border, never on it.
- **Scale:** zero to a round top at least a tenth over the tallest point, counted as the One Rate Rule counts; its figures stand bare in the column on the plot's right, centred on their rules (80 · 60 · 40 · 20), and what they count in (Mb/s, °C) stands once on the baseline, the zero line, clear of the legend's peak; nothing is laid over the curve.
- **Time axis:** under the plot, the window cut into steps, each named on its line with how long ago it stands, in no unit larger than a step reaches, the last "now". A window offers each cut whose step is a whole number of its unit, in order of preference: quarters, sixths, thirds (a minute in 15, 10 or 20 s; a day in 6, 4 or 8 h; a month in 5 or 10 d); one no such cut fits keeps its quarters (7 d · 5 d 6 h · 3 d 12 h · 1 d 18 h · now). The page takes the cut that gives the widest plot, the preferred one on a tie, and fits again when the room changes. Every other cell column draws a line with no label. The live minute is named the same way.
- **Axis labels:** time and scale labels alike, sans 12px at 500 in Meta on 24px lines, tabular figures: a step quieter than the legend.
- **Legend:** over each plot, 8px above it, as the Overview's facts read: the series' 6px packet in its hue, its name as the term (Meta, 400), its figure as the value (Ink, 500, with its unit) and what the figure is ("now" live, "on average" for a past window), all 14px sans; at the plot's right edge "peak" in Meta and the window's highest reading in Ink at 500. A dotted 1px line in the series' hue marks the peak in the plot.
- **Lanes:** download and upload stand in two lanes, download over upload, 24px apart, each its own legend, scale and peak, so a slow upload fills its own lane instead of lying flat under the download; one time axis under the lower lane. Download is Denim over a soft Denim wash, upload Amethyst over a soft Amethyst wash (16% at the line fading to nothing at the bottom border: `--color-chart-denim-from`/`-to` and `--color-chart-amethyst-from`/`-to`), so a quiet line rides clean paper.
- **What happened:** a minute nothing was recorded breaks the line; a reboot is a dashed Meta rule at its moment, an outage a crimson wash, packet loss a marigold one, all behind the curve.
- **Choices:** a choice that governs one graph alone is a labelled form field over it, its controls 32px ("Show chart for [CPU ▾] over [Last 24 hours ▾]"), never on its card's heading, which reads as the whole card's; a choice that governs the whole card rides the card's header. A graph draws no title of its own: its card or its drawer section names it.
- **Live:** a live chart draws the server's minute first and slides each sample in from the right while its grid stands still, so every time label keeps its line; its scale moves only when the readings cross a step; under reduced motion it steps.

### Live Control
- The Live button on a log: the waiting mark turns inside it while lines arrive and stands still when paused or disconnected. Its label is the state (Live, Paused · 12 new, Connecting…); its tooltip names the act (Pause, Resume). It stands on the heading line beside Download and Settings, all three as equals in the secondary dress.

## Do's and Don'ts

### Do:
- **Do** keep one denim primary per page, on the heading line, with the plus glyph.
- **Do** put everything that narrows a list on its card's control strip, a log's on its heading line, and everything that acts on the page on the heading line.
- **Do** use a counted `<select>` for every cut ("IPv6 · 19"), naming the whole set in its first option ("All devices", not "All").
- **Do** put every fixed measure on the 8px grid and every drawn edge on its line: blocks 40px apart in a drawer and 24px in a card, 24px inside a group, a box's air under its content and the gap after it giving back its bottom border's pixel.
- **Do** write every line of text on 24px lines and build rows from them (7px after a hairline, 8px over the next: 40px for a line), air after a drawn edge a pixel short and air before one whole, controls at 32px, straight under their label (28px inside a row, riding its line).
- **Do** check a page with G in a dev session: every frame, rule and control edge lands on the overlay's lines.
- **Do** set machine strings in Inconsolata 16px, ledes in Hanken Grotesk 14px/300, and compact body text in Hanken Grotesk 14px.
- **Do** carry state with the 6px packet square in its hue; hollow for absent.
- **Do** keep the resting state silent: say only what is wrong, in plain words.
- **Do** ask before a consequential act, in place and in one hue: crimson when it destroys or drops something, marigold when it is disruptive but wanted (installing firmware).

### Don't:
- **Don't** use a segmented switch or toggle track on a control surface; cuts are dropdowns, switches are checkboxes.
- **Don't** set type in a full-chroma hue, or in Faint or Glyph.
- **Don't** cast shadows on anything that sits on the page; only floating layers get one.
- **Don't** use more than 2px corners on a surface, or circles for state.
- **Don't** mark a message with an alert glyph. An error, a notice, a warning, a banner or a confirmation says its tone with the 6px state square in that tone's hue, hung on its first line (the notification, a form's error, the crimson and marigold confirms).
- **Don't** say how an act went in the content. Every outcome — a save confirmed, a notice, a request the router never answered — is the notification at the top right of the viewport, inside the open drawer when one is open. A form keeps only its refusals.
- **Don't** repeat a failure in every form on the page. A failed act is said once, in the notification.
- **Don't** add icons to buttons for emphasis. Glyphs appear only on the primary (its plus), on the direction glyphs (download, upload, open-out), and on every subsection act, where the glyph names the act.
- **Don't** leave a plus alone on a table head or lane; it says what it adds ("+ Add route").
- **Don't** use a circular spinner; waiting is the four-square mark.
- **Don't** fix a row or band height with a height utility; heights come from line plus padding.
- **Don't** centre the page or let a band's controls leave the page's column.
- **Don't** fill a table with stripes or vertical rules, or make a whole row clickable.
- **Don't** pair a danger button with a sand or bordered secondary; the way back speaks the question's hue.
