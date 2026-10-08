---
name: Verso
description: The web admin for OpenWrt routers, kept like an operator's notebook — calm, exact, alive.
colors:
  ground: "#faf9f7"
  canvas: "#ffffff"
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
  display:
    fontFamily: "Hanken Grotesk, ui-sans-serif, system-ui, sans-serif"
    fontSize: "1.875rem"
    fontWeight: 600
    lineHeight: 1.1
    letterSpacing: "-0.025em"
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
  group: "20px"
  page: "40px"
components:
  button-primary:
    backgroundColor: "{colors.denim}"
    textColor: "{colors.canvas}"
    typography: "{typography.body}"
    rounded: "{rounded.xs}"
    padding: "0 16px"
    height: "34px"
  button-primary-hover:
    backgroundColor: "{colors.denim-deep}"
    textColor: "{colors.canvas}"
  button-secondary:
    backgroundColor: "transparent"
    textColor: "{colors.ink}"
    typography: "{typography.body}"
    rounded: "{rounded.xs}"
    padding: "0 16px"
    height: "34px"
  button-secondary-hover:
    backgroundColor: "{colors.quiet}"
    textColor: "{colors.ink}"
  button-danger:
    backgroundColor: "{colors.crimson}"
    textColor: "{colors.canvas}"
    rounded: "{rounded.xs}"
    padding: "0 16px"
    height: "34px"
  button-danger-hover:
    backgroundColor: "{colors.crimson-deep}"
  button-danger-back:
    backgroundColor: "transparent"
    textColor: "{colors.crimson-deep}"
    typography: "{typography.body}"
    rounded: "{rounded.xs}"
    padding: "0 12px"
    height: "34px"
  button-danger-back-hover:
    backgroundColor: "{colors.crimson-line}"
  button-caution:
    backgroundColor: "{colors.marigold-deep}"
    textColor: "{colors.canvas}"
    rounded: "{rounded.xs}"
    padding: "0 16px"
    height: "34px"
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
    height: "34px"
  chip:
    backgroundColor: "{colors.quiet}"
    textColor: "{colors.meta}"
    typography: "{typography.value}"
    rounded: "{rounded.xs}"
    padding: "2px 6px"
  chip-accent:
    backgroundColor: "{colors.denim-soft}"
    textColor: "{colors.denim-deep}"
    rounded: "{rounded.xs}"
    padding: "2px 6px"
  control-band:
    backgroundColor: "{colors.mid}"
    rounded: "{rounded.xs}"
    padding: "12px"
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

Verso is kept the way a careful operator keeps a notebook for the router they run: warm sand paper, hairline rules, entries written in a fixed hand. The page is the ground, the router writes onto it, and nothing on the page decorates. Machine facts (addresses, interface names, versions) are set in mono like entries copied down verbatim; the words around them are plain sans. Every page but home and a log stands on the 20px notebook grid the charts sit on, and everything on it is ruled to that grid. State is carried by one small square, the packet, the unit a router handles, so the same mark means "up", "worth a look" or "down" everywhere it appears.

The mood is **calm, exact, alive**. Calm, because the resting state is silent and only failures earn words. Exact, because every value on screen maps one-to-one onto the config it came from and every measure repeats: a 20px grid, 40px rows, 34px controls. Alive, because calm is not grey. Live values glide, the waiting mark folds and unfolds, big numbers read at a glance, and colour appears the moment it means something. A page that reads as a dry grey box has missed the brief as badly as one that shouts.

The system is flat and dense enough for a power user, never cramped. Surfaces separate by tone and hairline, not by shadow. One colour, denim, means "act on this", and it is spent sparingly: one primary act per page.

**Key Characteristics:**
- Warm sand neutrals, eleven steps, each with a fixed job.
- One action colour (denim); four status hues used as marks, never as decoration.
- Two faces only: Hanken Grotesk for words, Inconsolata for machine strings.
- Flat surfaces, hairline borders, 2px corners, 1px for marks.
- A 20px notebook grid every edge lands on; 40px rows, 34px controls.
- Sections open on a Hairline with their heading under it, unfilled, so subjects scan by rule and size rather than by surface.
- A Mid Sand control band, boxed in the page's column, sits on the listing it narrows.

## Colors

A warm, near-neutral sand ground carries a single cool action blue and four status hues that only ever mark.

### Primary
- **Denim** (#3568a8): actions, focus and selection — the primary button, links, the selected tab underline, and selected form choices. Keep selection colour on the control itself so primary actions remain easy to find.
- **Deep Denim** (#285184): denim one step darker, for hover and focus rings.
- **Denim Hairline** (#cbd8e6) and **Denim Wash** (#e8eef5): the hairline and ground of the accent chip (network and zone references, static addresses) and of info bands.

### Secondary
- **Leaf Green** (#0e9b6e): up, healthy, accept — as a mark (state square, the accept chip's mark). **Deep Green** (#066b4b) carries green words on **Green Wash** (#e4f2ec).
- **Marigold** (#f0a419): worth a look — a real trade-off, not an alarm. **Deep Marigold** (#96590a) carries words on **Marigold Wash** (#fdf0d4), which is also the staged-changes chip.
- **Crimson** (#d63b3b): down, invalid, rejected; an errored field's border. **Deep Crimson** (#9e2222) carries error text on **Crimson Wash** (#fbe9e9).

### Tertiary
- **Amethyst** (#8b52be): the second series on a graph (upload). One job, and it is not a verdict.

### Neutral
- **Paper** (#faf9f7): the page and every card.
- **White** (#ffffff): the one surface a step above the page — the top bar, input fills, a takeover's backdrop.
- **Quiet Sand** (#f5f2ec): every quiet surface — the masthead, chips, row hover.
- **Mid Sand** (#ece8e0): the only mid ground — the control band, icon-button hover, the gateway card.
- **Hairline** (#e4e0d8): hairlines inside a card, between rows.
- **Strong Hairline** (#d2ccc2): subsection ledger lines, the line under a lane, every input border.
- **Inert** (#c9c3b8) and **Parked** (#c1bab0): an off switch track, an empty port; the waiting mark's parked squares.
- **Faint** (#a09a8e) and **Glyph** (#8b857a): decorative and action icons. Never text.
- **Meta** (#6f6a60): kickers, column heads, units, placeholders — the lightest ink that may carry words.
- **Body** (#5c574f): body copy and every label on a card.
- **Ink** (#1b1917): headings and values.

### Named Rules
**The One Voice Rule.** One primary denim button per page, on the heading line. Radios, checkboxes and multi-select chips use the same hue in their selected marks; this is a control state, not another primary action.

**The Mark Rule.** A hue at full chroma is a mark — a square, a border, a line, a fill behind white. It never sets type. Words in a hue use its deep step, on its own wash.

**The 4.5 Floor.** Nothing below 4.5:1 against Paper carries words. That rules out Faint and Glyph for text, and every bare hue.

## Typography

**Display Font:** Hanken Grotesk (with ui-sans-serif, system-ui)
**Body Font:** Hanken Grotesk (with ui-sans-serif, system-ui)
**Label/Mono Font:** Inconsolata (with Menlo, SFMono-Regular)

**Character:** A friendly, open grotesk for every word, beside a narrow, even-pitched mono for every string the machine wrote. The split is semantic, not stylistic: you can tell a value from a label by its face alone.

### Hierarchy
- **Display** (600, 1.875rem/30px, 1.1): the home page's verdict heading only.
- **Headline** (600, 1.5rem/24px, 1.1, -0.025em): every page's h1, on a heading line that is always a control's 34px tall so pages never shift between each other.
- **Title** (600, 1.125rem/18px, -0.025em): table and drawer titles, and the overview's traffic graph title, in Body ink on sand.
- **Section heading** (600, 1.125rem/18px, 1.25, -0.025em): full-section h2s under the section's leading Hairline, in Ink. Page titles, dialog titles and the overview's graph title retain their own roles.
- **Sub-heading** (600, 0.875rem/14px): a section inside another section, as an h3 in Ink ("Authorized keys" under SSH), so a part of a subject never reads as a peer of its heading. It stands on a **ledger line**: the glyph of what the part is, when it has one (16px Lucide in Glyph: a network, a DHCP server, a bridge), then the name, then how many things the part holds (500, Meta, tabular), then a Strong Hairline running on to the column's end, the line that stands between sections, so it reads as the start of a group and never as one more field label, and is told from the Hairlines between the rows under it. The count is the set's own length. When it differs from the last time the page was shown, the old number rolls out and the new one rolls in (up for more, down for fewer, 420ms exponential ease-out), arriving in Ink and settling to Meta. Under reduced motion it simply shows the number. The ledger is one 20px line, and the part's first box stands a cell under it, as a form's rows stand apart.
- **Body** (400, 0.875rem/14px, 1.5): compact prose, labels (600), buttons (600), cells.
- **Lede** (Hanken Grotesk 300, 0.875rem/14px on 20px lines, a step under the section heading it explains): section introductions throughout the UI, including nested sections and drawers, never a page's h1, capped at 60ch in Body ink; paragraphs stand a cell apart. A takeover's lede keeps 1rem/16px. They wrap and grow with enlarged text and translations; labels and technical keys retain their compact roles and full contrast. Ledes stay Body gray while scrolling and focusing controls, with no color transition. Warning ledes retain their semantic warning color.
- **Label** (500, 0.75rem/12px, 0.08em, uppercase): kickers and column heads, in Meta.
- **Value** (Inconsolata 500, 1rem/16px): machine strings in rows — addresses, interface names, MACs, versions. Inputs for identifiers use Inconsolata 400 at 16px.
- **Figure** (Inconsolata 700, 2.25rem/36px, -0.05em): big numbers.

Full section headings and their ledes stand under the section's leading
Hairline, with no fill and no decorative marker; the rule and the heading's
size establish the section's hierarchy. Smaller subsection headings retain
their ledger lines; square marks carry state.

### Named Rules
**The Verbatim Rule.** Mono is for strings the machine wrote and a user might copy: identifiers, addresses, versions, config keys. Never for a count, a duration or a sentence. Counts are sans with tabular figures.

**The Even Pitch Rule.** Mono never takes negative tracking, except the 700-weight Figure, whose wide glyphs can carry it.

## Layout

The page is left-aligned against a 288px rail, never centred. The frame keeps 40px of air on every side, and inside it the page declares a measure, always whole cells of the grid: wide pages cap at 1140px, the default at 1020px, narrow at 880px, page forms at 760px (`max-w-form`, the redefined `--container-3xl`). A column the window squeezes rounds down to whole cells. Form rows and the main columns of settings, editor and rail layouts share that 760px measure; supporting columns take the remaining space beside it once the container has 1000px (`@form-split`: the form, the 40px gap, a 200px rail), and stack under it when narrower. Drawers stand at 760px, keep the page's 40px side inset, and use the same stacked fields and content measures as page forms. The top bar's right group ends on the same rounded column edge, so a page's primary act sits exactly under Log out.

**The notebook grid.** Every page but home and a log stands on a barely-there 20px grid, the Hairline's colour at 35%, strongest under the masthead's Hairline and fading out 24 cells down, so it marks the page as paper and never sits behind a long listing's last rows (a dev session shows it whole on G). It hangs from the masthead (CSS anchor positioning): the masthead's Hairline and the rail's border are its first lines however tall the masthead grows, and the 40px gutter starts the content just past a line. Everything on the page is ruled to it:

1. **One grid, 20px, never finer.** Measures are cells: 20 is a cell, 40 two.
2. **Every drawn edge sits on a line**: hairlines, borders, a band's or a frame's edges, the ends of a table's rules. A box starts just past a line and its bottom hairline is the next line; a top border or a frame lands on the line by a margin of whole cells less its own pixel (a frame stands a pixel out at the top and the left).
3. **Half a cell only inside**: padding and the gaps between unbordered things, never an edge.
4. **Text writes on 20px lines**, centred in its cell; a row is a cell more than its lines (10px over the first line, 9px under the last, the hairline the next pixel), so it ends on a line however it wraps.
5. **Anything under 10px stays inside a component** (a chip's padding, an icon's gap). A chip is 21px and stands a pixel over its 20px line, so its top and bottom borders are the grid's lines wherever that line sits in a cell (a field's label line); in a table row, whose line straddles a grid line, it keeps the same box; a 28px act rides a line 4px over at either edge.
6. **Controls are 34px** (`h-control`), centred in two cells, 3px clear of the label's line above.

Two lists keep half cells on purpose: a set of radios (Single-choice sets) and a rail's "On this page" links. Two cells a link is too loose for a run of 14px words and one is too tight, so each link is a cell and a half (30px: its 20px line, 5px over and under). The run reaches those 5px out at either end, so the first link's words stand in a cell, every second link's after them, and the ones between straddle a line, centred on it; the run's hairline stays on the column's line.

**The rhythm is 40 and 20.** Blocks on a page stand two cells apart, the same as the body inset; a group's steps (heading to toolbar to table) are a cell. A section's rule stands two cells under the content before it. The band is the rule, two cells, the title's 20px line, its lede's lines under it, then the content straight on: a field brings its own cell of air, and a band with a lede keeps a cell under it. Height follows the contents, with no fixed or minimum band height.

**Sections are one component** (`sections.css`), the same on a page and in a drawer. A section states its kind and the stylesheet gives it its air and its rule:

| Kind | What it is | Air and rule |
|---|---|---|
| ruled | one of the surface's subjects | two cells, hairline, a cell; with a heading the band carries the rule (above); the surface's first draws no rule, a page form's included, since it opens under the masthead's hairline |
| part | a ruled section inside a section | a cell, hairline, a cell, kept to the content's inset |
| continued | more of the rows above (the add act) | hairline always, on the last row's own line |
| plain | an unruled block | a cell above (none on a page's own stack, which spaces its blocks two cells) |
| bare | a nested or flush block | none of its own |

A surface differs only in its reach, how far its rules and bands run past the content: a page 40px left to the rail and, in a form column, 40px back past its end; a drawer 40px, the same inset, to both edges. Every hairline divider (a section's, a band's, a page Save's, a config preview's) reaches by the same two measures and pads back in, so its words never move. Hand-drawn pages (Maintenance) use the same kinds.

A page Save's rule and a configuration card's divider stand a cell under what they close, and what follows stands a cell under them. A plugin contribution that opens with a section band, as SSH does on Access, stands that band's rule two cells under the page before it, with no extra hairline or padding. Section separation is measured from the field row's box. Stacked fields grow with their label, control and errors by whole lines. An untitled section gives back its first row's cell of air, which its rule already holds; a titled section keeps it under the band. Rows end on lines by themselves, so no section trims its last. A rule runs out by 40px to the rail on the left and stops where the column ends on the right, clear of a sidebar beside it. On page forms, section content keeps 40px of inset from both ends of these rules. Apply the right inset once inside the 760px form column; full-section headings, configuration dividers and Save rules reach through it, while nested content shares the same alignment. Heading bands follow the same horizontal reach, keeping their words aligned with the content. A drawer's section bands and configuration dividers span its full width; subsection and list rules keep the content inset.

The h1 stands alone: a page carries no lede, because the reader already knows where they are. The masthead is a Quiet Sand bar, the colophon's ground, from the rail to the window's right edge, ending on a Hairline. The heading line stands 16px inside the bar at top and bottom, so an act on it sits in the bar's middle, and its Hairline is the grid's first line: the content starts two cells under it, as far as the page's gutter keeps it from the rail. Page acts end where Log out does, whatever measure the content column keeps. On a list too the bar ends on its Hairline, and the control band is a box two cells under it, inside the page's column: the bar is the page's, the box belongs to the listing. A live log has no band: its search and acts stand on the heading line, and its masthead is the light one, the same bar on the page's own ground (Paper, as the rail is), so the Quiet Sand log under it, the sand bar's own ground, is the page's one darker surface, flush on the Hairline. Diagnostics reads the same way: its row (tool, target, interface, IP family, then Run where Log out ends) stands on a line of its own 20px under the heading. The tool's field leads with that tool's glyph in Glyph (ping's radio echo, traceroute's route, the lookup's search), which a pick swaps in from a little smaller; the interface's field leads with the network glyph and reads "any interface" in the sans, a picked interface's name in mono. Run leads with play. While a run goes, Stop stands in its place with the waiting mark turning, as a log's Live does, and the mark holds still once Stop is pressed. The target offers back what this browser reached for before, and with a keyboard at hand the caret waits in it. Each run's output fills the sand as a log does, the program's own lines from its first, read in the code block's inks (addresses green, measurements denim, ping's statistics rule and an unanswered hop Meta), closed by how it ended. The home page's sentence keeps the full 40px above it and stands unruled.

Inside a group (heading to toolbar to table) the step is a cell. Rows are 40px (a 20px line, 10px over it and 9px under, and the hairline), drawer headers, drawer tab strips and the colophon are 56px as the top bar is (a 28px line plus 14px above and below, the Hairline drawn inside the band; they sit above the grid), controls are 34px, and a control inside a row is 28px, riding its line. Section bands grow with wrapped headings, ledes and controls by whole lines; a heading act stands centred on the title's line and reaches past it.

Acts sit at the level they act on. A page's acts stand on its heading line. A form that is the whole page commits every section on it, so it closes on a section rule of the page's (two cells under the content before it, run out to the rail) with its Save under it. Whenever a form has a configuration card, on a page or in a drawer, the card sits immediately above Save; the actions finish that card without another rule. The card's rule is a section's rule: two cells under the content before it, and the card two cells under it, as a title stands under its rule. A drawer form without a configuration card closes on its own inset hairline. A form that commits one section draws no hairline: its Save stands a cell under its last field, and the next section's band identifies the next subject. Every Save stands centred in two cells. Within a section, its settings and their Save come first, then what the section holds (keys, a certificate), with that thing's own acts a cell under it.

Every list page reads top to bottom as: the heading line (h1 left, page acts right), then the **control band**, then the content beneath it. The band is a box in the page's column, its edges level with the h1 and the page acts, so it reads as part of the thing on the page, never as more of the page's bar.

Grouped listings split into lanes: each lane head is a 40px row of the masthead's Quiet Sand between strong hairlines, and every lane after the first stands two cells off the one before, so each chain reads as its own small table.

On a phone the rail becomes an off-canvas drawer, bands stack their controls, and wide tables scroll inside their own box rather than widening the page.

## Elevation & Depth

Verso is flat at rest. Depth comes from tone (Paper → Quiet Sand → Mid Sand) and from hairlines, not shadows. Shadows exist only on things that float over the page, and only as much as it takes to lift them.

### Shadow Vocabulary
- **Field inset** (`box-shadow: inset 0 1px 2px rgba(27,25,23,.06)`): every input and select — a field reads as a slot cut into the paper.
- **Tooltip** (`box-shadow: 0 4px 12px rgba(27,25,23,.14)`): tooltips and hover tips.
- **Drawer** (`box-shadow: -8px 0 32px rgba(27,25,23,.12)`): the 760px side panel, cast leftward over the page.
- **Dialog** (`box-shadow: 0 8px 32px rgba(27,25,23,.16)`): modal dialogs and alerts. A confirmation asks in place and casts none.

### Named Rules
**The Flat-By-Default Rule.** Nothing on the page casts a shadow. Only a layer that floats above the page (drawer, dialog, tooltip) does, and a pressed button loses even its press shadow.

## Shapes

Corners are nearly square: 2px on every button, field, chip, band and card; 1px on state marks. Circles appear only for toggle knobs, meter pills and the interface tree's physical/software encoding.

The state mark is a 6px square (5px in the nav), the packet. Filled means present in its hue; hollow (Faint border, no fill) means absent. The waiting mark is four of those squares folding on an 8px grid. Never a circular spinner.

Borders are hairlines, 1px, in Hairline or Strong Hairline. Tables have no vertical rules and no stripes; rows are separated by a single hairline.

## Components

### Section headings
- **Surface:** no fill. The band is the section's leading hairline with the heading under it; no decorative marker.
- **Contents:** the heading is 18px/600 on a 20px line. Its metadata and controls share the heading line, centred on it, a control reaching past it rather than making it taller; a lede belongs inside the same band, the next line, in 14px/300 Body ink on 20px lines, at most 60ch wide. Markdown and links retain their normal prose styling. A section's metadata reads as a label in Meta and a value in Body at 600 (Reboot's "Running for"), or inline beside the title (when Firmware was last checked); its control stands at the band's right end (Check again) and wraps within the band when needed.
- **Lede, rarely:** a section has none by default. One is kept only for what the heading and the rows under it cannot carry: a consequence to know before touching anything ("Restoring replaces the saved files and reboots the router"), or a fact that changes how the whole section reads ("These apply to every server above"). A lede that restates the heading, previews the fields (each field's own tip or description says what it is), or advises leaving things alone is clutter.
- **Padding:** two cells from the hairline to the title, with or without a lede; the page's first band draws no rule and its title is the cell under the masthead's own two cells of air. Do not impose a fixed or minimum height; wrapping and controls determine the height.
- **Content inset:** the content stands a cell under the title (or its lede). A field brings that cell itself; a band with a lede keeps it; anything else (a listing, a reading, a set) is given it by the section. Handwritten section bodies, such as Maintenance's, supply the same cell themselves. Count it once.
- **Content bottom spacing:** a section keeps no closing air: the next section's rule stands two cells under the content, by its own margin.
- **Reach:** the page band extends through the left gutter to meet the menu, or the screen edge on mobile, and ends at the content column's right edge. Words keep their existing horizontal alignment. In a drawer the band reaches both edges while its contents keep the body's 40px inset.
- **Scope:** page h1s, drawer and dialog titles, navigation kickers and smaller subsection headings keep their own treatments. The overview's traffic h2 is a graph title: 18px in Body ink, inside the chart's own header with 16px horizontal padding and no section band extending into the gutter. The graph is one component (Traffic), so a device's panel draws the same graph for the device, under its facts, its title ("Usage") standing in for a section heading. Subsections retain their inset ledger lines.

### Buttons
Plain and exact.
- **Labels:** a button says what it does to what: a verb and its object, in sentence case. "Save rule", "Save zone", "Save SSH settings", "Add network", "Install certificate", "Delete interface". Never a bare "Save", "Update", "Commit", "Submit", "OK" or "Confirm": the reader should not have to look around the button to know what it will act on. A save stages (see Staging), so its verb is Save; an act that runs at once is named for what it does ("Install certificate", "Reboot"), never Save. A confirmation's button repeats the act it confirms ("Delete rule" asks, "Delete rule" answers), and that is the default when a confirm names none. A form that names no act falls back to "Save changes". An icon-only act in a row keeps its short title as a tooltip ("Edit") but is named with its row for a screen reader ("Edit lan"). A dialog that can only be acknowledged says what acknowledging lets you do ("Keep working"), not "OK".
- **No explanations beside acts:** a row of acts carries its buttons and nothing else. What an act does is its label's job, and what happens after is the product's (a save stages, the chip counts it). A line beside the acts is kept only for a fact someone must know before pressing and cannot learn anywhere else; a status reading that belongs to the controls ("Index refreshed 2 days ago") is not an explanation.
- **Shape:** gently squared (2px), 34px tall (`h-control`, the one height every field and button shares), centred in two cells, 16px side padding, 14px/600 text.
- **Primary:** Denim fill, white words, and always a leading 16px plus glyph, unless the act leads elsewhere and names its own (download, upload, open-out). It lives on the heading line, one per page.
- **Secondary:** the subsection act's colours at any size: transparent, Meta words, Strong Hairline border; on hover the border turns Parked, the fill Hairline, the words Ink. Every quiet button answers the pointer alike (a list's Add, Cancel, a log's Live · Download · Settings, a row button). Words only; no decorative icons.
- **Links out:** a link that opens in a new tab leaves the router, and says so: the Lucide external-link glyph, 16px, leads its words ("Project website"), and a screen reader hears "(opens in a new tab)" after them. A link that names its own glyph keeps it; the words still tell a screen reader.
- **Danger:** Crimson fill, white words, for the confirmation step only: the inline question's act, always beside its way back (see Confirmation), or the act a typed key unlocks.
- **Subsection acts:** the acts on a part of a section: "+ Add a key", and a certificate's Install · Make a new one · Download. One dress for all of them:
  - 28px tall, 14px/500 words in Meta, a Strong Hairline border on no fill.
  - Always a leading 16px glyph that names the act (plus, upload, refresh, download), with 8px padding before it and 10px after the words.
  - On hover the border turns Parked, the fill Hairline, the words Ink.
  - One exception: an act on a section's heading line (General's "Use my computer's time") aligns with the section's right content edge, including when it wraps below the heading. It takes a field's 34px height with 16px side padding, in the same colours, centred on the title's line and reaching past it.
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
- **One hue, every step of it:** the tone's Wash ground inside its Hairline, 2px corners, every word in its Deep step. Full chroma appears as a mark only: the 6px state square on the first line, and for danger the fill behind the act's white words. No sand, no Ink, no second border.
- **Trigger:** the act's own name ("Reboot now", "Download and install 25.12.5") on the Wash with the Hairline, 14px/600 words in the Deep step, a 34px control as wide as its words like every other; the hairline turns full chroma on hover.
- **Question:** the compact inline warning itself (Inline warnings), in the act's tone: danger's crimson band, caution's marigold one. The question ("Reboot the router now?") is its 14px/600 title over one or two sentences of consequence, the 6px square on the first line, the frame on the grid's lines and the words half a cell inside.
- **The answer pair:** the band's acts, on the line a cell under the words: the act (34px, named with the verb: "Reboot", "Install firmware"), then the way back as bare words in the Deep step ("Not now", "Not yet"): no border, no fill, 12px side padding, washing to the Hairline at half strength on hover. Danger's act is white on full Crimson. Caution's act is white on Deep Marigold, because white on full Marigold fails the 4.5 floor. Never "OK" and "Cancel".
- **With a password:** when the act needs re-authorizing, the password field leads the answer row with a Crimson Hairline border.

### Inline warnings
A warning (marigold) or an error (crimson) said in place, on the tone's Wash, every word in its Deep step, framed by the tone's Hairline, one step darker than the Wash; the frame stands on the grid's lines, compact or not. It is the only thing on a surface that explains itself: when nothing is wrong, nothing is said.
- **One size:** the title and the text under it are both 14px on 20px lines; the title is bold (600), the text regular, half a cell inside the band at top and bottom so it is whole cells. The title never steps up a size and the text is never a lede.
- **Mark:** the tone's 6px square on the title's first line.
- **The machine's words:** what a tool reported rides inside, under a Hairline of the tone, verbatim in mono.
- **The acts that resolve it:** a notice may carry the buttons that resolve what it says (install what is missing): a row inside the band, each button's top on the line a cell under its words and the rest of its two cells kept below it, so the padding under the buttons matches the air over the first line and the band stays whole cells tall. The buttons keep their own shape and take the band's ink: words and glyph in its deep step, the border that ink thinned, a wash of it under the pointer.
- A confirmation's question is this band, its answers the band's acts (Confirmation).

### Chips
- **Style:** lowercase Inconsolata 14px in Meta on Quiet Sand, Hairline border, 2px corners, 2px 6px padding. They carry config values and config keys (`hostname`, `pppoe`), never decoration.
- **Accent:** Denim Wash with Deep Denim words and a leading 14px icon, for what was configured by hand. A green variant marks "you" (this browser, this session).
- **Entity chips:** an icon plus label for an interface, zone or port cited inside another row. Table chips use the shared semantic palette: network and zone references in denim, healthy DHCP and reservations in green, configured device limits in marigold. Reservation row actions use one icon family: pin to reserve, pin-off to remove a reservation. Each row offers only the action that applies to its state. Reserved devices carry the same pin icon and the short label `reserved`. The words `reserved` and `limits` use the sans face at regular weight. Limits chips say only `limits`; the tooltip is a compact readout: seven weekday cells with scheduled days in marigold, a prominent time window and quiet router-time caption, then paired download/upload values. Keep prose as the accessible text alternative. Keep category labels short and entity names exact. Colored chip icons follow their hue, using its Deep step for green, marigold and crimson.
- **One box, always a border:** every chip that cites something is the same box: a row's tag ("this browser"), the router at one end of a firewall rule, a rule's action (`accept`, `reject`, `drop`, `mark`; `drop` has no hue and is the neutral chip), the protocol beside IPv4, a choice's config value, an interface in a row. That box is 14px/400 words on a 15px line, 2px above and below, 6px sides and a 1px border, 21px tall. The border is its hue's Hairline on its hue's Wash, or Hairline on Quiet Sand for a neutral one. Only the face varies: mono for a string the machine wrote (`accept`, `lan`, `dhcp`), sans for words ("this browser", the router, which names a thing rather than a value). A mono chip is one step heavier, 500, because Inconsolata at 400 reads a size smaller than the Hanken beside it.
- **One mark or none:** a chip leads with nothing, a 14px Lucide icon, or the 6px packet square in its hue (hollow when the chip has none, the packet absent). Never two; an icon wins. The packet in a chip is still: a chip states a fact, and the only pulse is the page's live mark. A firewall verdict (`accept`, `reject`, `drop`, `mark`) always leads with its packet, and `drop`'s is hollow. On any 20px line the chip overhangs by a pixel either side rather than making the line taller (`-my-px` in the chip's own shape). The removable token inside a list control is not a citation and keeps its 24px box.

### Control Band
What narrows a listing, boxed on top of it.
- **Surface:** the empty state's box drawn solid and a step darker: Mid Sand in a Strong Hairline, 2px corners, 12px padding (13px under the controls), the width of the page's column. Its frame is on the grid's lines, a pixel out at the top and the left: its top edge is two cells under the masthead's Hairline, and it is three cells tall. The bar and the box tell apart by shape and by tone: the bar runs edge to edge in Quiet Sand, the box holds to the column in Mid Sand. On a phone its controls stack inside it at the same inset.
- **Contents:** only what narrows the content, side by side from the left, 16px apart — over a live listing, or one whose search reaches a larger set the router holds (Packages' index), a search first (the app's field treatment), then counted dropdowns ("IPv4 · 14", "All families · 22"), then the dimension the listing is sliced along ("All networks"); never segmented switches, and no dropdown strays to the band's far end. An option that is another listing (Packages' All) loads it rather than narrowing, and goes unpriced while that listing is unread. A paged listing walks its pages on one line of prose under the rows ("6213 matches · Next"). Everything that acts (primary add, Live, Download, Settings) goes on the heading line instead.
- **No search over a still listing:** a listing that holds what it has, however long, is scrolled and searched with the browser's own find; a search field over it is a second, weaker find, and a cut by a word on the rows (a family, a network, a kind) is the same find again. Only a live listing (the firewall's activity, a log) carries a search, because its rows arrive while it is read. A still listing keeps a cut only where it sorts by a state no word on the row spells out (Devices: online, offline, reserved, with limits). A band left with nothing to narrow with is not drawn, so most pages run from the heading straight into their table.
- **Below it:** the table stands a cell under the band, its one-cell column-head row first, so the band holds to the listing it narrows, closer to it than to the masthead's Hairline; a log sits flush on Paper.

### Tables
- **Rows:** 40px, Paper, one Hairline under each row, no stripes. A cell writes on 20px lines with 10px over the first and 9px under the last, its hairline the next pixel, so a row is a cell more than its lines and lands on the grid however its values wrap; every cell stands at the row's top, so a row's first lines stay level. Rows are inert, and so is the name: it is words, never a control, and never turns denim under the pointer, because a reader cannot tell what pressing a name would do. The row's acts open it, saying what they do with their glyph: the edit pencil for an editor, Details for a drawer to read. A row whose name a plugin pointed somewhere is given that act by the shell when it lacks one, or the trailing Details link when it has no acts at all. A row's icons read in the order of the tabs its panel opens on: on a row whose panel is the shell's (a device), the Details door leads the acts, as the shell's Details tab leads the strip, and each plugin's slot follows; elsewhere the door trails the row's own acts. Only an expandable row's name is a control, and its chevron says so. A table closes on the Hairline under its last row, wherever it stands, so it always ends on a line of the grid.
- **Column heads:** one cell of the grid, 12px/500 uppercase Meta with 0.08em tracking on a 16px line, 2px over and a pixel under, the Strong Hairline the cell's last pixel.
- **Lanes:** a 40px text row filled with the masthead's Quiet Sand between Strong Hairlines (its 28px add centred in it): the first lane's top hairline is the column heads' own, a later lane draws its own on the line where its fill starts, and the two cells of air above it stay unfilled. The lane is the one fill a table carries besides the hover wash. Label → destination with a faint arrow, a Meta tally, and a quiet "+ Add rule" at the right.
- **Switched off:** a row whose subject is not in force (a rule, a Wi-Fi network, a route) has its name marked over with a sand marker, the way an operator runs a highlighter through an entry that no longer holds. The marker is Strong Hairline ink laid under the name's words alone (never the whole row) and overshooting them by 10px each side as a hand does, a band a little slimmer than the line, its edges frayed and streaked along the stroke, its chisel ends pooled a little darker; it never passes 80%, so the name, set in Body at the regular weight, keeps 4.9 : 1 over it. Every other word reads at Meta; every hue in the row falls to sand (marks to Glyph, chips to the neutral box), so colour in a table means "in force". Nothing fades below 4.5 : 1. The row's acts and the focus ring keep denim, and a screen reader hears "off" after the name. When the row's power act turns it off in place, the marker is drawn left to right in 520ms; turned back on, it lifts in 280ms; once each, and under reduced motion it simply appears or goes. A plugin sets the row's `muted`; the shell draws all of it.
- **A plus on a table head always has words.** Wherever a lane or a table head carries an add, the plus stands beside the verb and its object ("+ Add rule", "+ Add route"), never alone: the head has the room, and a bare glyph makes the reader guess what it adds. The words stay short because the lane already names where the new row lands; the accessible name carries the full phrase ("Add IPv4 route").
- **Empty:** the slot every set draws (Collections, Empty), where the first row would stand: the hollow packet and two sentences at most, no illustration, no button, its frame on the lines. Under a heading it stands a cell under the title. A live listing keeps a row instead, the wait rather than the absence, and a listing whose lane band stands with nothing under it ("Files · 0 · + New file") keeps the band and says its sentence in a row under it. It is never a made-up row in a value column.
- **Names:** a row's name is the listing's 14px at 600 in Ink, wherever its column stands.
- **Last hairline:** a listing that ends its section gives its last row's hairline to the rule that closes it (the next section's, the page Save's), so the two never stand as a pair; the row keeps the pixel, clear.
- **Detail line:** when a value needs words to say what it is or since when (the browser behind an address, when a session started), they stand on a second 20px line under it in 14px Meta, never beside it, and the cell's tag rides that line. The row becomes a stacked row: two 20px lines with 10px over and 9px under, 60px, its first line level with every one-line cell's and with the row's 28px acts. Folding a qualifier under its value is how a narrow measure holds a long identifier: Signed in now is Source (address over browser), Last seen ("3 min ago" over "since 22 Sep, 09:12") and a revoke icon, which fits 640px with a full IPv6 address.
- **Icon acts that end something** ask first, in the same alert as the labelled row act, with the act's own name on the danger button ("Revoke session").
- **Row acts on the grid:** a row's last icon act stands on a crossing of the notebook's grid: centred on the vertical line a cell in from the table's right edge (the actions cell keeps 7px on its right, 12px before the first act), and on the row's middle line (5px over the 28px act, 6px under; a stacked row's first line). A lane's add sits the same way, so on hover its border ends where the rows' acts do.
- **The grip on the grid:** a reorder grip stands on the first crossing: centred on the vertical line a cell in from the table's left edge (7px before the 24px grip, the 9px it gives up kept on its right so the column keeps its width) and on the row's middle line (7px over, 8px under).
- **Nothing to state:** a cell with no value, in any column, is one mark: the dash in the sans, in Faint, a step below a muted value, said "None" to a screen reader. Never a mono dash that reads as a minus, never a state's hollow packet with nothing after it, never left blank where the column expects a value. A check, a switch, a grip and a row's acts keep their own emptiness.
- **Inked figures:** a reading that comes in figures (the Devices roster's Now, a rate down and up; This month, one total) stands as figures side by side, each in a slot of one width (4rem, 16px apart) so a column of them lines up down a lane, right-aligned in sans tabular Ink, a 14px Lucide glyph after one that runs a way (arrow-down for download, arrow-up for upload) in Glyph. No meter stands under a figure: a rate has no ceiling the router knows (no line speed, and a share of whatever else moves says only who else is busy), so a bar under it would be pretty and meaningless. Where the size of a reading matters at a glance, the cell is inked instead: its figures are written in a tone's Deep step at the same weight, the listing owning the thresholds. Now inks a device's load, both ways together, by what the rate is enough for: light (a call, browsing) under 5 Mbit/s in Deep Green, medium (a film, HD to 4K) to 25 in Deep Marigold, heavy (a download, an update) past it in Deep Crimson. The glyphs stay Glyph. The legend says each ink in the ink itself, the word in its tone and its range after it in Meta ("light under 5 Mbit/s"), so the scale is read where the colours are; the figures already carry the size, so colour is never the only way to tell. A live listing changes the ink in place: it eases over 300ms through Oklab (each tone restated in OKLCH, since hex eases through sRGB and green to crimson would pass through mud), none under reduced motion, and a device waking from idle arrives already in its ink. A cell with no figure is the dash; its slots stay drawn and hidden so a live listing fills them in place, the figure changing without a roll, since a roll every second down a lane is noise. A screen reader hears the figure and its direction ("38.2 down").
- **Leading edge:** every row's first content (a lane's label, a column head, a first cell, the Interfaces tree) starts 11px in from the table's left edge, where the grip's glyph starts; the trailing edge keeps 16px.
- **Acts hang from the first line:** when the thing a row names spans more than one line (a key's name over its fingerprint, an address over its IPv6, a value that wraps), the icons that act on it (edit, delete, copy) align with its first line, not the middle of the block. The first line is the thing's identity, and the acts belong to it. The same holds for a label and a status pill beside a wrapping value, in tables and fact lists alike. On a one-line row, centred and first-line are the same place.
- **Expanded row:** an inventory row opens into the uci sections behind it, one part each, in the order they stack (a network, its DHCP server, the device under them). Each part stands on a ledger line: the glyph of its kind (the network mark, the globe for a line to the internet, the server, and for a device the mark the chooser made it with: a bridge, a VLAN, a tunnel, a port), the kind of section in 14px/600 Ink, its name as the config writes it in 16px mono, a strong hairline running on, and the act that edits it at the line's end (a subsection act with the edit glyph) unless the row's own pencil already does. A DHCP server's state leads its facts with its mark, and its pool is drawn as the stretch from its first address to its last, filled by the share that is leased. Under the line, the facts in plain words in two columns 48px apart, the first half in reading order on the left (a network's addresses) and the rest on the right (its protocol, uptime and zone); narrow, the right column stands under the left. The section as `/etc/config` spells it is not repeated here: the drawer that edits it shows what it writes. Parts stand two cells apart. Nothing stands loose between them: no button between blocks. A fact that would only say "—" because the section is off (a disabled server's pool and leases) is left out; the part says it is off once.
  - **It unfolds.** The row opens as a fold growing from nothing to the height of its content (360ms, exponential ease-out), the trunk of the tree growing down with it, and its parts arrive in reading order: each ledger line draws itself from its name outward (480ms), and what stands under it, the facts one by one 30ms apart, rises the height of a hairline's air into place. Closing folds it back. Under reduced motion, or when a refresh puts an open row back, the row is simply open.
  - **Its acts open the drawer.** The interface's editor is the drawer, open over the listing: the row's pencil and each part's act (Configure DHCP, Edit device) open it in place, an act into one part opens it scrolled to that section, and the editor's own address shows the listing with the drawer open. The form's preview of what it writes stands at its foot behind a hairline, as the configuration card's editor (see Configuration cards). A refused save stays in the drawer over what was typed.
  - **What changes, rolls.** When the page's live refresh finds a fact reading differently (an uptime a minute on, a lease taken), the old value rolls out and the new rolls in where it stands, up for more and down for fewer, and its label goes to Ink for a moment and settles back, the way a ledger count does.

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
- **Items:** the identity in 16px mono Ink over one line of detail in 14px mono Meta (a fingerprint, an address), wrapping anywhere on a phone, each on a 20px line. Items are separated by their own half cell of air above and below, never a hairline, so each is whole cells.
- **Empty:** where the first item would stand, a slot: Quiet Sand inside a dashed Strong Hairline (the one dashed line in the app; it marks a place, not a thing), 2px corners, 16px sides, its frame on the grid's lines and two cells tall, a 20px line between. It holds the hollow packet and the set's empty words ("No keys are authorized.") in 14px Meta. It is inert, with no hover and nothing to press. The add stands a cell under it, centred in the two cells after; under the items it stands centred in the two cells straight under the last.
- **Remove:** a 28px trash icon on the item's first line, named with the item for screen readers. Pressed, the crimson confirmation drops full-width under the item ("Remove this key?", **Remove key** · Not now), focus on the act, and Escape or Not now returns focus to the icon.
- **Add:** "+ Add a key" (a subsection act) at the foot unfolds in place into a mono box (cursor in it), the plugin's live reading of what is typed (a key reads as `ssh-keygen -l` prints it), and **Add key** · Not now. The reading stays hidden until there is something to read. A refused paste comes back open, as typed, with its reason under the box.
- **After:** the change runs at once and the page reads the router again, saying once what happened ("Key added.").

### Configuration cards
- **Placement:** always the last content inside the form, directly above Save and any companion actions, on pages and in drawers. Keep it out of side rails and above every action row; all editable sections come before it.
- **Top inset:** on pages and in drawers alike, the leading hairline stands a cell under the fields and the card a cell under the hairline, its frame on the grid's lines (a pixel out at the top and the left). Save stands a cell under the card whatever the card stands in.
- **Label:** the filename alone, in mono, heading the card. Keep live previews and copy controls with the card.
- **Editor:** a card whose text is in a grammar (`uci`: every `/etc/config` card) reads as an editor shows a file. The file heads it on a 40px Quiet Sand strip like an editor's tab (its Hairline on a line of the grid): the path alone in mono 14px/500 Meta, and the copy control at the strip's end. Under it the text on a field's white, each line written in a cell of the grid with a cell of air above the first and below the last, each numbered in a Quiet Sand gutter two cells wide behind a Hairline on the grid's line (three cells past 99 lines; Faint, tabular, on the text's own 20px line, apart from the text so selecting it takes no number), and every token in its own ink: the keyword (`config`, `option`, `list`) Deep Denim, a section's type Deep Amethyst, an option's name Ink, its value Deep Green, the quotes and a comment Meta. Lines never wrap; the text scrolls sideways. The file's closing newline is not a line.
- **Change gutter:** a live card remembers what it read when its form was first touched and marks, in the gutter, every line the form has since changed (a 3px Marigold bar, the number in Deep Marigold: the stage's colour) or added (the same in Green). A typed value changes only its own token, so the text keeps its inks while the plugin's reading is on its way. A value with no grammar (a key, a token) keeps the plain box, its copy control inside at the top right.
- **Actions:** Save follows a cell under the card, centred in two cells, with no intervening hairline. The card's leading divider separates the fields from the configuration and its actions.

### Inputs / Fields
- **Anatomy:** a setting's row is one shape everywhere. The label comes first: the setting's name (14px/600 Ink), its UCI option as a mono key chip, the marigold `staged` mark while its change waits, and the explanation raised onto the name as a tooltip. The tooltip rises above the name, never over the control under it, standing where the pointer arrived along the name and holding still there; it waits 200ms for the pointer to settle, so passing over a name on the way to its control raises nothing, and keyboard focus raises it at once, at the name's start. Only a name with no room above (a panel's first row under its title bar) drops it below. Then the control, then its refusal band, then a description kept in view (settings rows only), then the remove lane when the row belongs to a set someone adds to. Fields, lists, switches, settings rows and a conditional's gate all draw it through the one `verso-field` partial (`fieldFrame` in `internal/widget/field_frame.go`); a widget supplies only its control. The sign-in form, outside any row, uses the same label, box and band atoms.
- **Layout:** every page form and drawer puts labels and their technical keys above the control: the label's 20px line, then the control centred in the next two cells (3px clear of the line above). Checkboxes lead their labels on the same line, in matching DOM and visual order, the 18px box at the top of the label's line. Labels wrap without moving controls into a separate column. A description runs on 20px lines under the control, or under a checkbox's label.
- **Row spacing:** a cell of air above each field row and none below, the first in a form included, since it is the air under the title it follows (an untitled section's rule already holds it). Adjacent fields add no separate container gap. A field and the act it feeds (a search and its Search) stand side by side, 8px apart, each control whole with its own corners and border; a button never sits inside a field. A key typed to unlock a destroying act (Factory reset's hostname) is an ordinary field row, the act a cell under it in the danger dress (the form's `tone`), held shut until the box says what it shows.
- **Fused values:** related values typed into boxes (rate and burst, address and netmask, a pool's first address and size, the web ports, a new password and its repeat) are one setting and one control. One label covers every part, one key chip names every option joined by a middot (`synflood_rate · synflood_burst`), and one `staged` mark stands for the row. One box holds the parts: the field's frame and states, parts split by a Hairline, each part as wide as its value's measure (96px for a number, 256px for a secret, 176px otherwise) plus its unit. A secret part is a password box: never reflected, sans 14px. A unit is Meta mono 16px/500 inside the frame after its value, never fixed padding, so the box fits the unit's word in every language. A press anywhere on a part puts the caret in it. Each part keeps its own name for a screen reader, and a refused part gets its own band naming it ("Burst: …"). The label's tip explains each part under its name when the group has no sentence of its own. A single value with a unit is the same box with one part.
- **Joined values:** a word between values always splits them. Parts that read as one sentence (a rule's Path, `lan to wan`; its clock window, `09:00 to 17:00`; its date window) are never fused: each stands as its own control, side by side, with the word between them in Body sans 14px and 12px either side, under the row's one label, one key chip and one mark. A dropdown measures its longest option and stays a dropdown however few options it holds; a typed clock time or date is mono at 128px, and a native clock control (a device's curfew Hours) is the same 128px. A native clock wears the Lucide clock in Meta, 16px, inset 12px from the right edge as a dropdown's chevron is; the platform's own glyph stays underneath, invisible, so a press on the clock still opens the platform's picker, which keeps its own look. Each part keeps its own name, change and refusal band ("Ends at: …"). The row wraps at a phone's width rather than squeezing its parts.
- **Added conditions:** a condition a rule carries is one named setting. Its name line is a row's label line: the condition's name (14px/600 Ink), the key it is listed under in the picker as a chip, what it is for raised on the name as a tip, and the quiet remove glyph at the line's end on the form measure's edge, taking the whole condition away; the line stays 20px tall. A condition of one control draws it straight under the name, as a label's control, its own label kept for a screen reader only and its staged mark worn on the name. A condition of several parts sets each part's label in regular Body weight, the first straight under the name and each after a cell of air, as fields stand, and a part that reads as a sentence is a joined row (Rate: `1000 per second`). The conditions a rule carries are one block: they stand in the quiet card the Action reading's parameters wear (a Hairline frame on Quiet Sand, 20px in, on the grid's lines, a cell inside its edges), which is there only while there is a condition in it. Two conditions meet on a Hairline across the card a cell from each, and the seam says how they join: "AND", a Meta kicker (12px/500, tracked, uppercase) centred on the rule on the card's own sand, so the block's whole logic is read where the eye crosses from one condition to the next. The block's lede says only what the card cannot: several values inside one condition are alternatives. "Add a condition" stands under the card with no rule of its own; with no condition, the card goes and the empty note returns. An include/exclude condition keeps its Exclude list behind a quiet reveal, a plus and "Exclude some" in Deep Denim 14px/500 on a 20px line a cell under the list, as a field's label stands, which unfolds to the list in place and steps aside; it arrives open whenever the condition already excludes something or an exclusion was refused, so nothing a rule says is ever folded away. A schedule is one set of controls wherever it is asked: the When reading and the Schedule condition draw the same days strip, windows and clock.
- **Switch groups:** one setting asked of several things (which files fw4 loads) is one row: the group's label on top with the option chip once, then a checkbox row per thing, a cell apart, each with its description kept in view and its own `staged` mark. A label that is a path is set in mono (`verbatim`). A state nothing on the page changes (a folder fw4 always reads) is a locked checkbox: set or clear in Inert sand, posting nothing. A set of files or folders is never a table unless it has columns to compare.
- **Value chips:** a field of several values (a protocol list, addresses, ICMP types) is a white field box holding one chip per value, 24px tall with 4px of the box showing round it and between chips, so the box keeps a field's 34px; wrapped rows of chips stand 16px apart, so a box of any number of rows is whole cells less the 6px it is centred in. The value is in mono 16px/500 Ink. The chip is Mid Sand behind a Strong Hairline, a step darker than a citation chip, because it sits on a field's white; its remove glyph is 20px, taking a Hairline-tone fill on hover so it shows on the chip.
- **Related fields:** opt into the shared `grid` with `style:"form"` for values entered or compared together: new password and confirmation, HTTP/HTTPS ports, rate and burst, range endpoints, address and netmask. Plain typed values and plain secrets fuse (above); a group holding a choice, a list or a secret with its own reveal keeps its columns. Keep labels above each control and retain DOM reading order. Use a 20px column gutter and a cell between stacked rows. The grid follows its own available width: one column below 36rem, two from 36rem, and a declared three-column group from 48rem. Existing content widths remain capped to each cell. Current password precedes the new-password pair; independent choices and editable lists keep their own rows.
- **Section submit spacing:** when a section saves its own fields, the action row stands a cell under the last control or error, centred in two cells, as a field stands. Actions directly after a configuration card keep the same cell.
- **Time-server list:** on System → General, list rows have a 12px horizontal inset, matching the input's text inset. Values use 16px mono at 400 in Meta. Keep the standard 14px/600 Ink field label; the inset establishes the hierarchy. Apply the same treatment to existing and newly added rows.
- **Style:** White fill, Strong Hairline border, 2px corners, field inset shadow, 34px tall (`h-control`, the same as every button). Sans 14px for words, mono 16px for identifiers.
- **Width by content:** reusable measures come from field semantics on pages and in drawers: `host` is half the row width when at least 40rem is available, otherwise full width, and covers hostnames, local domains and upstream DNS servers; `secret` is 24rem (384px) for passwords and shared secrets, including their reveal control; `endpoint` is 32rem (512px) for compound domain/address rules, resolver URLs and address/port listener lists; `choice` lets the native select size to its longest option; `prefix` is 14rem (224px), enough for a compressed IPv6 prefix through `/64`. Every measure is capped at the available width. Free text fills the available measure; fixed-format identifiers retain their compact widths and numbers, including DNS cache size, use 96px. Lists include their values and Add/Remove controls in the same measure. Width follows the field container, so a narrow drawer stays usable even on a large screen.
- **List fields** (a UCI `list`: time servers, upstream servers, extra hostnames, forwarding): one value per row below the label, 16px mono in Meta at the regular weight, led by the packet in the same ink, with its 28px remove 12px in from the right, riding the value's first line. The packet stands centred on the first line and in the middle of the grid's first column, and the value starts at the next column line. Values are divided from each other by a Hairline, never from the label: the first row's goes clear and keeps its pixel. Every list wears this one look; no page restyles its own. A row writes the value on 20px lines with 10px over and 9px under, so it is a cell more than its lines, never a fixed height. The add box and its Add stand centred in the two cells after the last row. With nothing in the list, the add box sits directly below the label and no empty hairline remains. A value too long for the column wraps after its own separators: slashes first, then dots, then colons (`/corp.example.com/` over `10.66.0.53`). It breaks anywhere else only when one part alone is wider than the column. Its remove hangs from the first line.
- **States:** only the border changes — Strong Hairline at rest, Faint on hover, Denim while focused. A refused field stays the one that is wrong while it is corrected: Crimson at rest, Deep Crimson under the pointer, and a 2px Deep Crimson edge while focused (an inset line, so the box never grows), with its reason under it on a band of Crimson Wash (12px side padding, half a cell vertical so it is whole cells, 2px corners: the crimson square, then the words). The band is the same under a field, a key's box, a list's box or a checkbox's label. A refused checkbox keeps its own look (a Crimson edge on a Denim fill fights itself); the band alone says it. A refusal is never a notice or callout set beside the control. A page that comes back refused opens at its first refused field, in the middle of the view with the cursor in it, never at the page's top. The first change to its value answers the refusal, so the box returns to the Strong Hairline and Denim focus and the reason goes; submitting asks again. Placeholders are Meta, never lighter.
- **Refusal navigator:** a refused page carries no "check the highlighted fields" band. While settings stand refused, a small tally pins itself under the top bar, 34px tall like a button, centred over the form holding the refused settings (not the wider content area); a page that arrives refused shows it still, and it slides in only when a refusal appears later: the crimson square, "2 settings refused", and **Next ↓**, which centres the next refused setting and puts the cursor in it. The shell counts the fields themselves (an invalid control or a standing refusal line, one per setting; a fused box's parts count apart), so no plugin writes the sentence. Each correction clears its refusal and the count turns over; at none the tally turns green, "Ready to save", and folds away. A form's own error band is only for a refusal no field owns, and keeps a dividing line's 24px below it.
- **Checkboxes:** first in the row, 8px before the label, aligned to its first line. 18px, 2px corners, White with the inset shadow and a Glyph outline; checked is Denim with a white check. Every switch in the app is drawn as a checkbox, because every change is staged.
- **Choice colours:** radios, checkboxes (including table and log controls) and multi-select chips share `--color-choice` (Denim) in `palette.css`; `--color-choice-border` (Glyph) defines custom empty checkbox outlines. Selection keeps a visible radio dot or check, and keyboard focus gets a separate outline. The selected fill contrasts 5.4:1 with Paper and 5.7:1 with white checks or chip text; the empty outline contrasts 3.5:1 with Paper and 3.3:1 with Quiet Sand. Native controls retain platform rendering and forced-colour support. Labels keep their existing text colour.
- **Single-choice sets:** up to three choices use visible native radios in a vertical group; four or more use a native dropdown whose width follows the longest option. Each radio row is a cell and a half (its 20px line, 5px over and under), the 16px circle on the line and 8px before its label; wrapped labels grow by whole lines. The group stands half a cell under the field's label, so the first choice straddles a line, the second sits in a cell, and so on alternately, and the group ends on a line whatever its count. Apply this throughout page forms and drawers, including Accept / Reject / Drop. The shared `RadioOptionLimit` constant in `internal/widget/field.go` owns the cutoff; change it there to adjust every form. Radio groups keep their field label, keyboard navigation, help and error associations. Segmented chips are reserved for selecting multiple members, such as weekdays; boolean settings keep their checkboxes.

### Navigation
- **Rail:** 288px, no fill of its own, one hairline against the page. Top-level rows carry a 16px Lucide icon in Glyph (Ink when active), wash to Mid Sand and slide 4px right on hover. The row you are on is the only surface: Quiet Sand with a 2px Ink bar on its right edge, against the page it opens. A parent whose sub-page is open goes bold Ink without the surface, and its sub-pages hang off a hairline under its icon, with no icons of their own; the open sub-page carries the 2px Ink bar on the same right edge.
- **Rows that open:** a row that opens into subpages says so on every page with a 16px chevron in Meta at its end: pointing right while closed, turned down over its open branch. It is one chevron, turned, so a page change turns it on the rail's easing rather than swapping one picture for another. A row with none carries no chevron. System's pages are the shell's; a plugin's destination declares it in its manifest (`"pages": true`).
- **What waits:** a row's end says only what waits for you there, never how things are (the homepage says that), and says nothing when nothing waits. It is a word or two in 14px/500 with tabular figures, led by the rail's square and tinted with it: System says what is ready to install in denim, as update news is everywhere ("15 updates", "1 update", or "New firmware", which speaks for any packages too, since a firmware upgrade brings its own). The row's title carries the whole sentence ("15 package updates are ready to install."). The row's name never gives way to these words; they shorten instead.
- **Page change:** the rail carries itself from the page you leave to the page you open. The marker glides from where you were to where you are going, under the words it passes; the rows under a branch that opens slide down to make room while its hairline draws down and its subpages drop in, 30ms apart in reading order; a branch you leave is gone at once as the rows under it close up. One easing, the unfold's (360ms, `cubic-bezier(0.22, 1, 0.36, 1)`); a row that gains or gives up the weight swaps it in 140ms, so its words never read doubled. The page itself appears at once and nothing on it moves, and the rail never holds back a press. It needs a browser that carries a page change (Chrome, Edge, Safari) and the rail beside the page; on a phone, under reduced motion, or in another browser, the page simply loads. Over HTTPS, a row's page is fetched as the row is pressed.
- **Top bar:** White, 56px. On the left, the router: the hostname in bold mono, its maker and model, and the staged-changes chip beside them, because the stage is the router's (held, applied and rolled back on the device), not the session's. The chip never gives way; the hostname truncates first. On a phone it says the square and the bare count. On the right, alone and aligned to the content column, the session: `root | Log out`.
- **Staged chip motion:** a saved change's square flies from its row to the chip, and the count turns over as it lands: the old count rises out of the chip's line as the new one rises in (140ms out, 260ms in, clipped by the chip). Under reduced motion the count simply changes.
- **Mobile:** the rail becomes an off-canvas drawer behind a menu button, over an Ink scrim.
- **Colophon:** every page's last line, and only a log goes without one (a log runs on as long as the router writes). It is a 56px Quiet Sand band (the 28px copy plus 14px above and below) with its Hairline drawn inside its top edge, running from the rail to the window's edge. On a short page it stands at the bottom of the window, and on a long one under the last of the content. Its words keep to the page's column, level with the h1, and end under Log out. On the left is the OpenWrt release at 500, then its revision (from 640px) and its target (from 1280px), all in 16px mono Meta with Meta middots between them. On the right is the Verso build, then a 28px copy icon ("Copy for a bug report") that puts the release, revision, target, board, kernel and Verso build on the clipboard, one labelled fact a line and in English. The line answers the copy by washing each fact it shows in Strong Hairline, never a hue, with 6px of padding either side of its words, one after another from the left (70ms apart, 1200ms each), with the words in Ink under the wash, and the icon turns to a green check. Under reduced motion only the check shows.

### Staging
Save stages a change; nothing happens on the router until it is applied from the chip's review drawer. Marigold means exactly that, waiting, and nothing else says a staged save happened, because it has not.
- **No band:** a staged save puts no notice on the page, green or otherwise. The chip is what says a change waits. Only a plugin's warning or refusal about the change still speaks, in its own tone.
- **What waits, everywhere:** the router holds the stage, so every setting whose change waits carries the marigold "staged" mark beside its name (the chip's own 6px square and word) on every visit, from any page, until the change is applied or discarded. A control finds its change by the option's full address: its key, and the config and section it lives in, which the plugin states once on a form or section or on the control itself.
- **Where it went:** the page comes back where the person was (a page form's Save at the same height), its changed rows already marked. Once, a moment after the page is drawn, a marigold square lifts off the first changed row and travels in an arc to the chip's mark (680ms). The chip shows its old count until the square lands, then takes the new one and warms once to the marigold line. A panel's save flies from the row it saved, which washes marigold for a moment. An empty chip appears as the square arrives.
- **Discard and Apply show at once:** the moment the stage changes under an open drawer, the page behind it shows the settings as they are on the router, with the marks gone and nothing reading as unsaved. It isn't left stale until the drawer closes. Only a change to which rows a listing has waits for the page to be read whole, on close. The chip never shows the new count before a flying change reaches it; it is held back from the first paint of the page a save lands on.
- **Only on growth:** it flies only when the stage grew. Under reduced motion, or with no chip on screen, the chip only warms. A screen reader hears "Staged — N staged changes. Review and apply from the top bar."

### Drawers
- **Shell:** a panel from the right on Paper, behind a Strong Hairline and cast with the drawer shadow. It slides in over 300ms (ease-out) and out over 200ms (ease-in). Every drawer is 760px wide, whatever it holds (a form, a chooser, a reading, the staged review), so the frame never changes size from one object to the next. On a narrower screen it takes the full width. Header is a 56px Quiet Sand band (a 28px line plus 14px above and below, its Hairline drawn inside its bottom edge) holding only the title ("New rule", "Edit Allow-Ping") and a 28px close.
- **Tabs:** a drawer whose object answers more than one question carries a 56px strip under the header, one tab per reading, each its name alone: no chip of where the object stands beside it, since the reading answers that. A tab names a reading as a label names a setting, so it is set at the labels' 14px, and the 18px section heading under the strip stays the clear step up. Each tab stands the strip's full height (a 28px line plus 14px above and below), so its underline sits on the strip's Hairline. The reading in view is Ink at 500 over a 2px Deep Denim underline; the rest are Meta. A device's drawer, whose tabs the plugins contribute, draws the same strip.
- **Sections:** each drawer starts its own heading hierarchy, independent of the page section that opened it. Full sections use the standard Quiet Sand heading band, edge to edge, with an 18px heading and any lede inside. A drawer has no visible grid but keeps the page's geometry, measured from its chrome's Hairline: sections, bands, fields, rows and cards stand exactly as they do on a page, and blocks inside a drawer form stand a cell apart. Headings, ledes and fields retain their 40px horizontal inset; subsection and list dividers stay inset. The opening section has no leading divider: the chrome's own Hairline (the header's, or the tab strip's) is its rule, and its title stands two cells under it, as under any rule. The object's own state and name (Forward is active, Name) open the drawer under no heading, since the drawer's title already names the object, and the next section's rule stands two cells under that untitled block, as every section's does.
- **Configuration card:** the drawer reads as form name → fields → configuration card → actions. The filename alone labels the card, above the code; it has no section heading, marker, “Written to” prefix or repeated explanation of staging. A full-width hairline separates it from the fields; the filename and code keep the content's 40px horizontal inset. Live previews and copy controls stay part of the card.
- **Footer:** when a configuration card ends the form, Save follows it with a small gap and no intervening hairline: the card and the act that saves it are one unit. Other drawer footers keep their inset Hairline. Removing an object belongs to its listing’s trash action, which opens a compact confirmation naming the object and its consequences, with the destructive act and Cancel. Edit drawers carry no duplicate removal action.

### Live Control
- The Live button on a log: the waiting mark turns inside it while lines arrive and stands still when paused or disconnected. Its label is the state (Live, Paused · 12 new, Connecting…); its tooltip names the act (Pause, Resume). It stands on the heading line beside Download and Settings, all three as equals in the secondary dress.

## Do's and Don'ts

### Do:
- **Do** keep one denim primary per page, on the heading line, with the plus glyph.
- **Do** put everything that narrows a list or log on the boxed Mid Sand control band, and everything that acts on the page on the heading line.
- **Do** use a counted `<select>` for every cut ("IPv6 · 19"), naming the whole set in its first option ("All devices", not "All").
- **Do** put every drawn edge on a line of the 20px grid: blocks two cells apart, a cell inside a group, a top border or a frame landing on its line by a margin of whole cells less its own pixel.
- **Do** write every line of text on 20px lines and build rows from them (10px over, 9px under, the hairline: 40px for a line), controls at 34px centred in two cells (28px inside a row, riding its line).
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
