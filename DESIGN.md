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
    height: "36px"
  button-primary-hover:
    backgroundColor: "{colors.denim-deep}"
    textColor: "{colors.canvas}"
  button-secondary:
    backgroundColor: "transparent"
    textColor: "{colors.ink}"
    typography: "{typography.body}"
    rounded: "{rounded.xs}"
    padding: "0 16px"
    height: "36px"
  button-secondary-hover:
    backgroundColor: "{colors.quiet}"
    textColor: "{colors.ink}"
  button-danger:
    backgroundColor: "{colors.crimson}"
    textColor: "{colors.canvas}"
    rounded: "{rounded.xs}"
    padding: "0 16px"
    height: "36px"
  button-danger-hover:
    backgroundColor: "{colors.crimson-deep}"
  button-danger-back:
    backgroundColor: "transparent"
    textColor: "{colors.crimson-deep}"
    typography: "{typography.body}"
    rounded: "{rounded.xs}"
    padding: "0 12px"
    height: "36px"
  button-danger-back-hover:
    backgroundColor: "{colors.crimson-line}"
  button-caution:
    backgroundColor: "{colors.marigold-deep}"
    textColor: "{colors.canvas}"
    rounded: "{rounded.xs}"
    padding: "0 16px"
    height: "36px"
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
    height: "36px"
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
    backgroundColor: "{colors.quiet}"
    padding: "16px 40px"
  section-band:
    backgroundColor: "{colors.quiet}"
    textColor: "{colors.ink}"
    typography: "{typography.section-heading}"
    padding: "16px 0 16px 40px"
  table-row:
    backgroundColor: "{colors.ground}"
    textColor: "{colors.ink}"
    typography: "{typography.body}"
    padding: "10px 14px"
    height: "44px"
  band:
    backgroundColor: "{colors.quiet}"
    textColor: "{colors.body}"
    typography: "{typography.title}"
    padding: "0 16px"
    height: "52px"
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
    width: "768px"
---

# Design System: Verso

## Overview

**Creative North Star: "The Operator's Notebook"**

Verso is kept the way a careful operator keeps a notebook for the router they run: warm sand paper, hairline rules, entries written in a fixed hand. The page is the ground, the router writes onto it, and nothing on the page decorates. Machine facts (addresses, interface names, versions) are set in mono like entries copied down verbatim; the words around them are plain sans. Charts sit on a 20px notebook grid. State is carried by one small square, the packet, the unit a router handles, so the same mark means "up", "worth a look" or "down" everywhere it appears.

The mood is **calm, exact, alive**. Calm, because the resting state is silent and only failures earn words. Exact, because every value on screen maps one-to-one onto the config it came from and every measure repeats: a 40px page rhythm, 44px rows, 36px controls. Alive, because calm is not grey. Live values glide, the waiting mark folds and unfolds, big numbers read at a glance, and colour appears the moment it means something. A page that reads as a dry grey box has missed the brief as badly as one that shouts.

The system is flat and dense enough for a power user, never cramped. Surfaces separate by tone and hairline, not by shadow. One colour, denim, means "act on this", and it is spent sparingly: one primary act per page.

**Key Characteristics:**
- Warm sand neutrals, eleven steps, each with a fixed job.
- One action colour (denim); four status hues used as marks, never as decoration.
- Two faces only: Hanken Grotesk for words, Inconsolata for machine strings.
- Flat surfaces, hairline borders, 2px corners, 1px for marks.
- A 40px page rhythm; 44px rows, 36px controls, and section bands sized by their content with 16px vertical padding.
- Quiet Sand section bands make subjects easy to scan, accepting a little more visual density for clearer grouping.
- A full-width sand control band is the seam between a page's heading and its content.

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
- **Quiet Sand** (#f5f2ec): every quiet surface — control bands, section bands, chips, row hover.
- **Mid Sand** (#ece8e0): the only mid ground — icon-button hover, the gateway card.
- **Hairline** (#e4e0d8): hairlines inside a card, between rows, around a control band.
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
- **Headline** (600, 1.5rem/24px, 1.1, -0.025em): every page's h1, on a heading line that is always 36px tall so pages never shift between each other.
- **Title** (600, 1.125rem/18px, -0.025em): table and drawer titles, and the overview's traffic graph title, in Body ink on sand.
- **Section heading** (600, 1.125rem/18px, 1.25, -0.025em): full-section h2s in Quiet Sand bands, in Ink. Page titles, dialog titles and the overview's graph title retain their own roles.
- **Sub-heading** (600, 0.875rem/14px): a section inside another section, as an h3 in Ink ("Authorized keys" under SSH), so a part of a subject never reads as a peer of its heading. It stands on a **ledger line**: the glyph of what the part is, when it has one (16px Lucide in Glyph: a network, a DHCP server, a bridge), then the name, then how many things the part holds (500, Meta, tabular), then a Strong Hairline running on to the column's end, the line that stands between sections, so it reads as the start of a group and never as one more field label, and is told from the Hairlines between the rows under it. The count is the set's own length. When it differs from the last time the page was shown, the old number rolls out and the new one rolls in (up for more, down for fewer, 420ms exponential ease-out), arriving in Ink and settling to Meta. Under reduced motion it simply shows the number. The part's first box stands 24px under the name, as a form's rows stand apart (a set's first item gives up its own air above).
- **Body** (400, 0.875rem/14px, 1.5): compact prose, labels (600), buttons (600), cells.
- **Lede** (Hanken Grotesk 300, 1rem/16px, 1.5): page and section introductions throughout the UI, including nested sections and drawers, capped at 60ch in Body ink. They wrap and grow with enlarged text and translations; labels and technical keys retain their compact roles and full contrast. Ledes stay Body gray while scrolling and focusing controls, with no color transition. Warning ledes retain their semantic warning color.
- **Label** (500, 0.75rem/12px, 0.08em, uppercase): kickers and column heads, in Meta.
- **Value** (Inconsolata 500, 1rem/16px): machine strings in rows — addresses, interface names, MACs, versions. Inputs for identifiers use Inconsolata 400 at 16px.
- **Figure** (Inconsolata 700, 2.25rem/36px, -0.05em): big numbers.

Full section headings and their ledes share a Quiet Sand band. The band has no
decorative marker or border; its surface establishes the section's hierarchy.
Smaller subsection headings retain their ledger lines; square marks carry state.

### Named Rules
**The Verbatim Rule.** Mono is for strings the machine wrote and a user might copy: identifiers, addresses, versions, config keys. Never for a count, a duration or a sentence. Counts are sans with tabular figures.

**The Even Pitch Rule.** Mono never takes negative tracking, except the 700-weight Figure, whose wide glyphs can carry it.

## Layout

The page is left-aligned against a 288px rail, never centred. The frame keeps 40px of air on every side, and inside it the page declares a measure: wide pages cap at 1152px, page forms at 768px (`max-w-3xl`). Form rows and the main columns of settings, editor and rail layouts share that 768px measure; supporting columns take the remaining space and stack when it is too narrow. Drawers stand at 768px, keep the page's 40px side inset, and use the same stacked fields and content measures as page forms. The top bar's right group ends on the same column edge, so a page's primary act sits exactly under Log out.

**The rhythm is 40, 24 and 20.** Blocks on a page stand 40px apart, the same as the body inset. A full section that used a leading divider keeps its 24px separation from the preceding content, but its heading band replaces the divider and the space below that divider. The band is the hairline, 32px to the title, and 20px from the title (or its lede, 4px under it) to the content; the section keeps 32px from its content to the next rule. Height follows the contents, with no fixed or minimum band height.

**Sections are one component** (`sections.css`), the same on a page and in a drawer. A section states its kind and the stylesheet gives it its air and its rule:

| Kind | What it is | Air and rule |
|---|---|---|
| ruled | one of the surface's subjects | 24px, hairline, 24px (32px under the first rule of a page form); with a heading the band carries the rule (above); 32px before the next rule when headless; the surface's first draws no rule |
| part | a ruled section inside a section | 24px, hairline, 24px, kept to the content's inset |
| continued | more of the rows above (the add act) | hairline always, 16px under it, nothing above |
| plain | an unruled block | 24px above (none on a page's own stack, which spaces its blocks 40px) |
| bare | a nested or flush block | none of its own |

A surface differs only in its reach, how far its rules and bands run past the content: a page 40px left to the rail and, in a form column, 40px back past its end; a drawer 40px, the same inset, to both edges. Every hairline divider (a section's, a band's, a page Save's, a config preview's) reaches by the same two measures and pads back in, so its words never move. Hand-drawn pages (Maintenance) use the same kinds.

Untitled sections, the page Save's rule and plugin seams retain their hairlines and 24px of air on either side. A plugin contribution that opens with a section band, as SSH does on Access, needs only the 24px gap before that band, with no extra hairline or padding. Section separation is measured from the field row's box. Stacked fields grow with their label, control and errors; checkbox rows follow their label's natural height. Sections give back their first and last rows' unnecessary padding; a titled section keeps its first row's padding under the band. A rule runs out by 40px to the rail on the left and stops where the column ends on the right, clear of a sidebar beside it. On page forms, section content keeps 40px of inset from both ends of these rules. Apply the right inset once inside the 768px form column; full-section headings, configuration dividers and Save rules reach through it, while nested content shares the same alignment. Heading bands follow the same horizontal reach, keeping their words aligned with the content. A drawer's section bands and configuration dividers span its full width; subsection and list rules keep the content inset.

The masthead has no divider beneath the h1 or its lede. Its 20px bottom padding is the complete gap before the content, with no additional bottom margin, letting the first section band establish its own boundary. Page acts end where Log out does, whatever measure the content column keeps. On a list or log, the control band starts 20px below the title and retains its own borders. The home page's sentence keeps the full 40px above it and stands unruled.

Inside a group (heading to toolbar to table) the step is 20px. Rows are 44px (a 24px line plus 10px above and below), drawer headers are 52px, controls are 36px, and a control inside a row is 28px. Section bands grow with wrapped headings, ledes and controls; their vertical padding stays 16px in every case.

Acts sit at the level they act on. A page's acts stand on its heading line. A form that is the whole page commits every section on it, so it closes on a section rule of the page's (40px either side, run out to the rail) with its Save under it. Whenever a form has a configuration card, on a page or in a drawer, the card sits immediately above Save; the actions finish that card without another rule. A drawer form without a configuration card closes on its own inset hairline. A form that commits one section draws no hairline: its Save stands 20px under its last field, and the next section's band identifies the next subject. Within a section, its settings and their Save come first, then what the section holds (keys, a certificate), with that thing's own acts 16px under it.

Every list page reads top to bottom as: the heading line (h1 left, page acts right), then the **control band**, then the content flush beneath it. The band spans the full page width, from the rail to the window edge, while its controls stay in the page's column, level with the h1. It is the seam between "the page" and "the thing on the page".

Grouped listings split into lanes: each lane head is a 44px row on a strong hairline, and every lane after the first stands 40px off the one before, so each chain reads as its own small table.

On a phone the rail becomes an off-canvas drawer, bands stack their controls, and wide tables scroll inside their own box rather than widening the page.

## Elevation & Depth

Verso is flat at rest. Depth comes from tone (Paper → Quiet Sand → Mid Sand) and from hairlines, not shadows. Shadows exist only on things that float over the page, and only as much as it takes to lift them.

### Shadow Vocabulary
- **Field inset** (`box-shadow: inset 0 1px 2px rgba(27,25,23,.06)`): every input and select — a field reads as a slot cut into the paper.
- **Tooltip** (`box-shadow: 0 4px 12px rgba(27,25,23,.14)`): tooltips and hover tips.
- **Drawer** (`box-shadow: -8px 0 32px rgba(27,25,23,.12)`): the 768px side panel, cast leftward over the page.
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
- **Contents:** the heading is 18px/600. Its metadata and controls share the heading row; a lede belongs inside the same band, on the next row with a 4px gap, in 16px/300 Body ink, with 1.5 line height and at most 60ch wide. Markdown and links retain their normal prose styling. Maintenance's Reboot uptime and Firmware check controls align right with a 16px inset, matching the tables' outer cell padding; they wrap within the band when needed.
- **Padding:** 32px from the hairline to the title, with or without a lede. Do not impose a fixed or minimum height; wrapping and controls determine the height. The content below starts after the band's 20px gap.
- **Content inset:** the first field row contributes another 12px above its contents, so its controls begin 32px below the band. Handwritten section bodies, such as Maintenance's action groups, supply that same 12px top inset themselves. Count it once; a body containing padded field rows already has it.
- **Content bottom spacing:** every headed section keeps 32px from the end of its content to the next rule, matching the 32px from its rule to its title.
- **Reach:** the page band extends through the left gutter to meet the menu, or the screen edge on mobile, and ends at the content column's right edge. Words keep their existing horizontal alignment. In a drawer the band reaches both edges while its contents keep the body's 40px inset.
- **Scope:** page h1s, drawer and dialog titles, navigation kickers and smaller subsection headings keep their own treatments. The overview's traffic h2 is a graph title: 18px in Body ink, inside the chart's own header with 16px horizontal padding and no section band extending into the gutter. Subsections retain their inset ledger lines.

### Buttons
Plain and exact.
- **Labels:** a button says what it does to what: a verb and its object, in sentence case. "Save rule", "Save zone", "Save SSH settings", "Add network", "Install certificate", "Delete interface". Never a bare "Save", "Update", "Commit", "Submit", "OK" or "Confirm": the reader should not have to look around the button to know what it will act on. A save stages (see Staging), so its verb is Save; an act that runs at once is named for what it does ("Install certificate", "Reboot"), never Save. A confirmation's button repeats the act it confirms ("Delete rule" asks, "Delete rule" answers), and that is the default when a confirm names none. A form that names no act falls back to "Save changes". An icon-only act in a row keeps its short title as a tooltip ("Edit") but is named with its row for a screen reader ("Edit lan"). A dialog that can only be acknowledged says what acknowledging lets you do ("Keep working"), not "OK".
- **Shape:** gently squared (2px), 36px tall, 16px side padding, 14px/600 text.
- **Primary:** Denim fill, white words, and always a leading 16px plus glyph, unless the act leads elsewhere and names its own (download, upload, open-out). It lives on the heading line, one per page.
- **Secondary:** the subsection act's colours at any size: transparent, Meta words, Strong Hairline border; on hover the border turns Parked, the fill Hairline, the words Ink. Every quiet button answers the pointer alike (a list's Add, Cancel, a log's Live · Download · Settings, a row button). Words only; no decorative icons.
- **Danger:** Crimson fill, white words, for the confirmation step only, and always beside its way back (see Confirmation).
- **Subsection acts:** the acts on a part of a section: "+ Add a key", and a certificate's Install · Make a new one · Download. One dress for all of them:
  - 28px tall, 14px/500 words in Meta, a Strong Hairline border on no fill.
  - Always a leading 16px glyph that names the act (plus, upload, refresh, download), with 8px padding before it and 10px after the words.
  - On hover the border turns Parked, the fill Hairline, the words Ink.
  - One exception: an act on a section's heading line (General's "Use my computer's time") aligns with the section's right content edge, including when it wraps below the heading. It takes a field's 36px height with 16px side padding, in the same colours.
  - An act that makes or replaces the thing it stands under (a certificate's Install · Make a new one) opens its form in the drawer over the page, which keeps its address. The drawer's button names the act ("Install certificate", never "Save": it runs at once, nothing stages). Once it has run, the drawer closes on the page read again, saying once what happened; a refusal stays in the drawer over what was typed. An act that hands over a file (Download) downloads.
- **Row buttons:** 28px, 12px side padding, the same secondary dress. Every icon-only act wears one dress, wherever it stands (a row's edit or remove, a list's remove ×, a copy beside a value or on a code box): a 28px square with a 16px glyph in Glyph on no border, taking a Parked border, the Hairline fill and Ink on hover, with a tooltip. Beside a line of text it keeps the line's height.
- **Copy controls:** an inline copy sits 4px after its value, with no extra parent gap. Its 28px hit area centers on the first text line without increasing the line height. Copy tooltips use the UI sans face, including beside mono values. A check and “Copied” appear only after a successful copy; the check is green on neutral surfaces and keeps the warning hue inside a warning. Failure is shown and announced with a manual-copy fallback. Keyboard focus stays on the control. Code cards keep the copy control in their top-right corner; the colophon copies its complete bug report.
- **Press:** every enabled button nudges down 1px and drops its shadow while pressed, including icon-only actions, drawer controls and client-created buttons. `input.css` owns the shared rule for native buttons, button inputs and `role="button"`; button-styled links join it through `verso-press`. Disabled and busy controls do not press. Reduced motion suppresses the movement. A button with an enlarged hit area moves only its `verso-press-content` child, preserving the click target. The effect changes no layout dimensions.
- **Resting:** a settings form's Save before anything in it has changed is disabled in Quiet Sand, a Strong Hairline border and Meta words, and turns denim the moment something changes. A page with no primary act and nothing changed therefore shows no denim.
- **Waiting:** a button busy with a slow act keeps its footprint, goes Quiet Sand with Parked words, shows the four-square waiting mark and a present-participle label with a real ellipsis ("Applying…"). Disable it immediately and mark it busy until the request finishes; repeated clicks or Enter must not submit again. Restore its original label, icon and availability after a refusal or connection failure. The same behavior applies to actions in drawers opened from any page. Under reduced motion, the four squares remain visible and still.

### Confirmation
How a consequential act asks first. The whole exchange is one hue, so the question and both answers read as a single voice. The hue is the act's cost:
- **Danger (crimson):** the act destroys or drops something: rebooting, deleting a rule.
- **Caution (marigold):** the act is disruptive but wanted, and is the point of being here: installing firmware. It steadies rather than scares.

The anatomy is the same in both:
- **In place:** the trigger gives way to the question where it stood. Nothing floats and nothing casts a shadow. Focus moves to the first answer; Escape and the way back return it to the trigger.
- **One hue, every step of it:** the tone's Wash ground inside its Hairline, 2px corners, 16px padding, every word in its Deep step. Full chroma appears as a mark only: the 16px triangle-alert icon, and for danger the fill behind the act's white words. No sand, no Ink, no second border.
- **Trigger:** the act's own name ("Reboot now", "Download and install 25.12.5") on the Wash with the Hairline, 14px/600 words in the Deep step, a 36px control like every other; the hairline turns full chroma on hover.
- **Question:** a 16px/600 question ("Reboot the router now?") over one or two sentences of consequence at 14px/1.45. The icon leads on the question's first line.
- **The answer pair:** the act (36px, named with the verb: "Reboot", "Install firmware"), then the way back as bare words in the Deep step ("Not now", "Not yet"): no border, no fill, 12px side padding, washing to the Hairline at half strength on hover. Danger's act is white on full Crimson. Caution's act is white on Deep Marigold, because white on full Marigold fails the 4.5 floor. The pair starts 24px below the message and indents 24px, so it lines up with the words, not the icon; 8px between the two. Never "OK" and "Cancel".
- **With a password:** when the act needs re-authorizing, the password field leads the answer row with a Crimson Hairline border.

### Chips
- **Style:** lowercase Inconsolata 14px in Meta on Quiet Sand, Hairline border, 2px corners, 2px 6px padding. They carry config values and config keys (`hostname`, `pppoe`), never decoration.
- **Accent:** Denim Wash with Deep Denim words and a leading 14px icon, for what was configured by hand. A green variant marks "you" (this browser, this session).
- **Entity chips:** an icon plus label for an interface, zone or port cited inside another row. Table chips use the shared semantic palette: network and zone references in denim, healthy DHCP and reservations in green, configured device limits in marigold. Reservation row actions use one icon family: pin to reserve, pin-off to remove a reservation. Each row offers only the action that applies to its state. Reserved devices carry the same pin icon and the short label `reserved`. The words `reserved` and `limits` use the sans face at regular weight. Limits chips say only `limits`; the tooltip is a compact readout: seven weekday cells with scheduled days in marigold, a prominent time window and quiet router-time caption, then paired download/upload values. Keep prose as the accessible text alternative. Keep category labels short and entity names exact. Colored chip icons follow their hue, using its Deep step for green, marigold and crimson.
- **One box, always a border:** every chip that cites something is the same box: a row's tag ("this browser"), the router at one end of a firewall rule, a rule's action (`accept`, `reject`, `drop`, `mark`; `drop` has no hue and is the neutral chip), the protocol beside IPv4, a panel tab's state, a choice's config value, an interface in a row. That box is 14px/400 words on a 16px line, 2px above and below, 6px sides and a 1px border, 22px tall. The border is its hue's Hairline on its hue's Wash, or Hairline on Quiet Sand for a neutral one. Only the face varies: mono for a string the machine wrote (`accept`, `lan`, `dhcp`), sans for words ("this browser", the router, which names a thing rather than a value). A mono chip is one step heavier, 500, because Inconsolata at 400 reads a size smaller than the Hanken beside it.
- **One mark or none:** a chip leads with nothing, a 14px Lucide icon, or the 6px packet square in its hue (hollow when the chip has none, the packet absent). Never two; an icon wins. The packet in a chip is still: a chip states a fact, and the only pulse is the page's live mark. A firewall verdict (`accept`, `reject`, `drop`, `mark`) always leads with its packet, and `drop`'s is hollow. On a 20px detail line the chip overhangs by a pixel either side rather than making the line taller. The removable token inside a list control is not a citation and keeps its control-sized box.

### Control Band
The signature seam of every list and log page.
- **Surface:** Quiet Sand, a Hairline above and below, 16px vertical padding, spanning the page from rail to window edge. It stands 20px under the title. These borders belong to the control band; the masthead has no divider.
- **Contents:** only what narrows the content, side by side from the left, 16px apart — search first (the app's field treatment), then counted dropdowns ("IPv4 · 14", "All families · 22"), then the dimension the listing is sliced along ("All networks"); never segmented switches, and no dropdown strays to the band's far end. Everything that acts (primary add, Live, Download, Settings) goes on the heading line instead.
- **Below it:** the table sits flush under the band with its 44px column-head row; a log sits flush on Paper.

### Tables
- **Rows:** 44px, Paper, one Hairline between rows, no stripes. Rows are inert: only the identity cell opens the row's drawer. A table closes on a Hairline under its last row, except when it ends its section: then it ends on its last row, and the section's rule (or the page's end) is the only line under it.
- **Column heads:** a full 44px row, 12px/500 uppercase Meta with 0.08em tracking, over a Strong Hairline.
- **Lanes:** a 44px text row on a Strong Hairline, label → destination with a faint arrow, a Meta tally, and a quiet "+ Add rule" at the right.
- **Empty:** one row where the first row would be, two sentences at most, no illustration, no button. A listing whose lane band stands with nothing under it ("Files · 0 · + New file") keeps the band and says its sentence under it. It is never a made-up row in a value column. The sentence gives up its own hairline to whatever closes the table.
- **Detail line:** when a value needs words to say what it is or since when (the browser behind an address, when a session started), they stand on a second 20px line under it in 14px Meta, never beside it, and the cell's tag rides that line. The row becomes a stacked row: two 20px lines with 12px above and below, 64px, so the first line keeps the 22px centre of a one-line cell and of the row's 28px acts. Folding a qualifier under its value is how a narrow measure holds a long identifier: Signed in now is Source (address over browser), Last seen ("3 min ago" over "since 22 Sep, 09:12") and a revoke icon, which fits 640px with a full IPv6 address.
- **Icon acts that end something** ask first, in the same alert as the labelled row act, with the act's own name on the danger button ("Revoke session").
- **Acts hang from the first line:** when the thing a row names spans more than one line (a key's name over its fingerprint, an address over its IPv6, a value that wraps), the icons that act on it (edit, delete, copy) align with its first line, not the middle of the block. The first line is the thing's identity, and the acts belong to it. The same holds for a label and a status pill beside a wrapping value, in tables and fact lists alike. On a one-line row, centred and first-line are the same place.
- **Expanded row:** an inventory row opens into the uci sections behind it, one part each, in the order they stack (a network, its DHCP server, the device under them). Each part stands on a ledger line: the glyph of its kind (the network mark, the globe for a line to the internet, the server, and for a device the mark the chooser made it with: a bridge, a VLAN, a tunnel, a port), the kind of section in 14px/600 Ink, its name as the config writes it in 16px mono, a strong hairline running on, and the act that edits it at the line's end (a subsection act with the edit glyph) unless the row's own pencil already does. A DHCP server's state leads its facts with its mark, and its pool is drawn as the stretch from its first address to its last, filled by the share that is leased. Under the line, the facts in plain words in two columns 48px apart, the first half in reading order on the left (a network's addresses) and the rest on the right (its protocol, uptime and zone); narrow, the right column stands under the left. The section as `/etc/config` spells it is not repeated here: the drawer that edits it shows what it writes. Parts stand 40px apart. Nothing stands loose between them: no button between blocks. A fact that would only say "—" because the section is off (a disabled server's pool and leases) is left out; the part says it is off once.
  - **It unfolds.** The row opens as a fold growing from nothing to the height of its content (360ms, exponential ease-out), the trunk of the tree growing down with it, and its parts arrive in reading order: each ledger line draws itself from its name outward (480ms), and what stands under it, the facts one by one 30ms apart, rises the height of a hairline's air into place. Closing folds it back. Under reduced motion, or when a refresh puts an open row back, the row is simply open.
  - **Its acts open the drawer.** The interface's editor is the drawer, open over the listing: the row's pencil and each part's act (Configure DHCP, Edit device) open it in place, an act into one part opens it scrolled to that section, and the editor's own address shows the listing with the drawer open. The form's preview of what it writes stands at its foot behind a hairline, as the configuration card's editor (see Configuration cards). A refused save stays in the drawer over what was typed.
  - **What changes, rolls.** When the page's live refresh finds a fact reading differently (an uptime a minute on, a lease taken), the old value rolls out and the new rolls in where it stands, up for more and down for fewer, and its label goes to Ink for a moment and settles back, the way a ledger count does.

### Uploads
A file the router has to check before it acts on it (a custom firmware image, a backup to restore) goes up in a dialog that says where it is.
- **Steps:** under the dialog's title, Choose, Verify, then the act (Install, Restore), with a short hairline between steps. Each step's packet is green when done, denim for the step in hand (Ink words, 600) and hollow for what's ahead (Meta words). The title says the trigger's words ("Upload a custom image").
- **The drop area:** the dialog's whole width, in the dashed Strong Hairline on Quiet Sand that marks a place. It holds the upload glyph in Glyph, the prompt in Ink 600 ("Drop a sysupgrade image here") and "or choose one from your computer" in Deep Denim, underlined. A file dragged over it turns the hairline Denim and the ground Denim Wash. Under the area, one Meta line says what fits: what it is, its type and its size limit ("A sysupgrade image built for Mono Gateway Development Kit · .bin, up to 128 MiB").
- **Going up:** the drop area gives way to the file itself: its name in 16px mono (breaking at its own hyphens), its size in tabular Meta, then a 6px Denim bar on Hairline that fills by scaling, and "2.4 of 6.0 MB" under it. The step line moves to Verify. Once the last byte is up, the dialog shows what the router is doing with it (the waiting mark, "Verifying firmware"). The answer takes the dialog's place on the step it reached: Choose again with the reason, or the act with the file's facts and the password that authorizes it (focus goes there). Installing firmware asks in caution's marigold.
- **Stopped:** a lost connection returns the drop area with a crimson band: "The upload stopped. Check the connection, then choose the file again."

### What needs a package
A capability a page can't offer until a package is installed (encrypted DNS, a blocklist, a local resolver). It's offered where the need is, and installed there.
- **A settings row:** the hollow packet and the fact ("Queries leave in plain text") stand where a setting's name stands, with a sentence under it: what the fact costs the person, if it isn't plain, then what installing adds here and that it arrives off ("Installing adds encryption here, off until you turn it on."). The act sits directly below the fact and its explanation: a field-height quiet button with the download glyph that names its package ("Install https-dns-proxy"). It's never a band ruled off above and below, and never a link out of the page.
- **Installing adds a setting, never a change:** a package that would switch itself on as it lands (a DNS proxy taking over dnsmasq, a blocklist loading) arrives off, so the router does what it did before. Turning it on is a change like any other: it stages, applies and rolls back.
- **The package's own drawer:** the act opens, over the page, the drawer Packages opens from the package's row (what it is, its facts, Install). The page keeps its address. After installing, the drawer closes, the outcome says the package is in, and the page reads itself again, so the fact becomes the setting the package adds. The rows the install added come into view if they're out of it, wash once in Green Wash (2s, fading out from the halfway point, reaching 12px past the words on either side), and the first one's control takes focus.
- **Packages by name:** a search that reaches Packages without a view searches everything, not only what's installed.

### Collections
A short set of machine strings someone keeps by hand: authorized keys, a tunnel's peers. It's not a table. A handful of identities needs no rules, and every act on the set happens where the set is.
- **Items:** the identity in 16px mono Ink over one line of detail in 14px mono Meta (a fingerprint, an address), wrapping anywhere on a phone. Items are separated by their own 10px of air, never a hairline.
- **Empty:** where the first item would stand, a slot: Quiet Sand inside a dashed Strong Hairline (the one dashed line in the app; it marks a place, not a thing), 2px corners, 16px sides, a 24px line with 10px above and below. It holds the hollow packet and the set's empty words ("No keys are authorized.") in 14px Meta. It is inert, with no hover and nothing to press. The add stands 16px under it, where it stands under the items once there are some.
- **Remove:** a 28px trash icon on the item's first line, named with the item for screen readers. Pressed, the crimson confirmation drops full-width under the item ("Remove this key?", **Remove key** · Not now), focus on the act, and Escape or Not now returns focus to the icon.
- **Add:** "+ Add a key" (a subsection act) at the foot unfolds in place into a mono box (cursor in it), the plugin's live reading of what is typed (a key reads as `ssh-keygen -l` prints it), and **Add key** · Not now. The reading stays hidden until there is something to read. A refused paste comes back open, as typed, with its reason under the box.
- **After:** the change runs at once and the page reads the router again, saying once what happened ("Key added.").

### Configuration cards
- **Placement:** always the last content inside the form, directly above Save and any companion actions, on pages and in drawers. Keep it out of side rails and above every action row; all editable sections come before it.
- **Top inset:** on pages and in drawers alike, leave 32px between the leading hairline and the card, matching the other full sections. A section that ends on its card gives back its own closing air, so Save stands 20px under the card whatever the card stands in.
- **Label:** the filename alone, in mono, heading the card. Keep live previews and copy controls with the card.
- **Editor:** a card whose text is in a grammar (`uci`: every `/etc/config` card) reads as an editor shows a file. The file heads it on a 36px Quiet Sand strip like an editor's tab: the path alone in mono 14px/500 Meta, and the copy control at the strip's end. Under it the text on a field's white, each line numbered in a Quiet Sand gutter behind a Hairline (Faint, tabular, at the text's own 26px line, apart from the text so selecting it takes no number), and every token in its own ink: the keyword (`config`, `option`, `list`) Deep Denim, a section's type Deep Amethyst, an option's name Ink, its value Deep Green, the quotes and a comment Meta. Lines never wrap; the text scrolls sideways. The file's closing newline is not a line.
- **Change gutter:** a live card remembers what it read when its form was first touched and marks, in the gutter, every line the form has since changed (a 3px Marigold bar, the number in Deep Marigold: the stage's colour) or added (the same in Green). A typed value changes only its own token, so the text keeps its inks while the plugin's reading is on its way. A value with no grammar (a key, a token) keeps the plain box, its copy control inside at the top right.
- **Actions:** Save follows with a 20px gap and no intervening hairline. The card's leading divider separates the fields from the configuration and its actions.

### Inputs / Fields
- **Anatomy:** a setting's row is one shape everywhere. The label comes first: the setting's name (14px/600 Ink), its UCI option as a mono key chip, the marigold `staged` mark while its change waits, and the explanation raised onto the name as a tooltip. Then the control, then its refusal band, then a description kept in view (settings rows only), then the remove lane when the row belongs to a set someone adds to. Fields, lists, switches, settings rows and a conditional's gate all draw it through the one `verso-field` partial (`fieldFrame` in `internal/widget/field_frame.go`); a widget supplies only its control. The sign-in form, outside any row, uses the same label, box and band atoms.
- **Layout:** every page form and drawer puts labels and their technical keys above the control, with an 8px gap. Checkboxes lead their labels on the same line, in matching DOM and visual order. Labels wrap without moving controls into a separate column.
- **Row spacing:** 16px above and below each field row, with the existing section-edge padding trims. Adjacent fields add no separate container gap. Inline search/action compounds keep their own compact spacing.
- **Fused values:** related values typed into boxes (rate and burst, address and netmask, a pool's first address and size, the web ports, a new password and its repeat) are one setting and one control. One label covers every part, one key chip names every option joined by a middot (`synflood_rate · synflood_burst`), and one `staged` mark stands for the row. One box holds the parts: the field's frame and states, parts split by a Hairline, each part as wide as its value's measure (96px for a number, 256px for a secret, 176px otherwise) plus its unit. A secret part is a password box: never reflected, sans 14px. A unit is Meta mono 16px/500 inside the frame after its value, never fixed padding, so the box fits the unit's word in every language. A press anywhere on a part puts the caret in it. Each part keeps its own name for a screen reader, and a refused part gets its own band naming it ("Burst: …"). The label's tip explains each part under its name when the group has no sentence of its own. A single value with a unit is the same box with one part.
- **Joined values:** a word between values always splits them. Parts that read as one sentence (a rule's Path, `lan to wan`; its clock window, `09:00 to 17:00`; its date window) are never fused: each stands as its own control, side by side, with the word between them in Body sans 14px and 12px either side, under the row's one label, one key chip and one mark. A dropdown measures its longest option and stays a dropdown however few options it holds; a typed clock time or date is mono at 128px, and a native clock control (a device's curfew Hours) is the same 128px. A native clock wears the Lucide clock in Meta, 16px, inset 12px from the right edge as a dropdown's chevron is; the platform's own glyph stays underneath, invisible, so a press on the clock still opens the platform's picker, which keeps its own look. Each part keeps its own name, change and refusal band ("Ends at: …"). The row wraps at a phone's width rather than squeezing its parts.
- **Added conditions:** a condition a rule carries is one named setting. Its name line is a row's label line: the condition's name (14px/600 Ink), the key it is listed under in the picker as a chip, what it is for raised on the name as a tip, and the quiet remove glyph at the line's end on the form measure's edge, taking the whole condition away; the line stays 20px tall. A condition of one control draws it 8px under the name, as a label's control, its own label kept for a screen reader only and its staged mark worn on the name. A condition of several parts sets each part's label in regular Body weight under the name, 12px below it, the parts 20px apart, and a part that reads as a sentence is a joined row (Rate: `1000 per second`). The conditions a rule carries are one block: they stand in the quiet card the Action reading's parameters wear (a Hairline frame on Quiet Sand, 20px in), which is there only while there is a condition in it. Two conditions meet on a Hairline across the card with 24px either side, and the seam says how they join: "AND", a Meta kicker (12px/500, tracked, uppercase) centred on the rule on the card's own sand, so the block's whole logic is read where the eye crosses from one condition to the next. The block's lede says only what the card cannot: several values inside one condition are alternatives. "Add a condition" stands under the card with no rule of its own; with no condition, the card goes and the empty note returns. An include/exclude condition keeps its Exclude list behind a quiet reveal, a plus and "Exclude some" in Deep Denim 14px/500 at the part step, which unfolds to the list in place and steps aside; it arrives open whenever the condition already excludes something or an exclusion was refused, so nothing a rule says is ever folded away. A schedule is one set of controls wherever it is asked: the When reading and the Schedule condition draw the same days strip, windows and clock.
- **Switch groups:** one setting asked of several things (which files fw4 loads) is one row: the group's label on top with the option chip once, then a checkbox row per thing, 12px apart, each with its description kept in view and its own `staged` mark. A label that is a path is set in mono (`verbatim`). A state nothing on the page changes (a folder fw4 always reads) is a locked checkbox: set or clear in Inert sand, posting nothing. A set of files or folders is never a table unless it has columns to compare.
- **Value chips:** a field of several values (a protocol list, addresses, ICMP types) is a white field box holding one chip per value, 26px tall with 4px of the box showing round it and between chips, so the box keeps a field's 36px, the value in mono 16px/500 Ink. The chip is Mid Sand behind a Strong Hairline, a step darker than a citation chip, because it sits on a field's white; its remove glyph is 20px, taking a Hairline-tone fill on hover so it shows on the chip.
- **Related fields:** opt into the shared `grid` with `style:"form"` for values entered or compared together: new password and confirmation, HTTP/HTTPS ports, rate and burst, range endpoints, address and netmask. Plain typed values and plain secrets fuse (above); a group holding a choice, a list or a secret with its own reveal keeps its columns. Keep labels above each control and retain DOM reading order. Use a 24px column gutter and the same 32px between stacked rows. The grid follows its own available width: one column below 36rem, two from 36rem, and a declared three-column group from 48rem. Existing content widths remain capped to each cell. Current password precedes the new-password pair; independent choices and editable lists keep their own rows.
- **Section submit spacing:** when a section saves its own fields, leave 32px from the last control or error to the action row, matching the separation between fields. Trim the final field or field group's bottom padding so it is counted once. Actions directly after a configuration card retain the card's 20px gap.
- **Time-server list:** on System → General, list rows have a 12px horizontal inset, matching the input's text inset. Values use 16px mono at 400 in Meta. Keep the standard 14px/600 Ink field label; the inset establishes the hierarchy. Apply the same treatment to existing and newly added rows.
- **Style:** White fill, Strong Hairline border, 2px corners, field inset shadow, 36px tall. Sans 14px for words, mono 16px for identifiers.
- **Width by content:** reusable measures come from field semantics on pages and in drawers: `host` is half the row width when at least 40rem is available, otherwise full width, and covers hostnames, local domains and upstream DNS servers; `secret` is 24rem (384px) for passwords and shared secrets, including their reveal control; `endpoint` is 32rem (512px) for compound domain/address rules, resolver URLs and address/port listener lists; `choice` lets the native select size to its longest option; `prefix` is 14rem (224px), enough for a compressed IPv6 prefix through `/64`. Every measure is capped at the available width. Free text fills the available measure; fixed-format identifiers retain their compact widths and numbers, including DNS cache size, use 96px. Lists include their values and Add/Remove controls in the same measure. Width follows the field container, so a narrow drawer stays usable even on a large screen.
- **List fields** (a UCI `list`, such as time servers): one value per row below the label, 16px mono, with its 28px remove on the right and a Hairline above every value, including the first. A row is the remove and 8px above and below, 44px, never a fixed height. The add box and its Add stand 16px under the last remove. With nothing in the list, the add box sits directly below the label and no empty hairline remains. A value too long for the column wraps after its own separators: slashes first, then dots, then colons (`/corp.example.com/` over `10.66.0.53`). It breaks anywhere else only when one part alone is wider than the column. Its remove hangs from the first line.
- **States:** only the border changes — Strong Hairline at rest, Faint on hover, Denim while focused. A refused field stays the one that is wrong while it is corrected: Crimson at rest, Deep Crimson under the pointer, and a 2px Deep Crimson edge while focused (an inset line, so the box never grows), with its reason under it on a band of Crimson Wash (12px side padding, 8px vertical, 2px corners: the crimson square, then the words). The band is the same under a field, a key's box, a list's box or a checkbox's label. A refused checkbox keeps its own look (a Crimson edge on a Denim fill fights itself); the band alone says it. A refusal is never a notice or callout set beside the control. A page that comes back refused opens at its first refused field, in the middle of the view with the cursor in it, never at the page's top. The first change to its value answers the refusal, so the box returns to the Strong Hairline and Denim focus and the reason goes; submitting asks again. Placeholders are Meta, never lighter.
- **Refusal navigator:** a refused page carries no "check the highlighted fields" band. While settings stand refused, a small tally pins itself under the top bar, 36px tall like a button, centred over the form holding the refused settings (not the wider content area); a page that arrives refused shows it still, and it slides in only when a refusal appears later: the crimson square, "2 settings refused", and **Next ↓**, which centres the next refused setting and puts the cursor in it. The shell counts the fields themselves (an invalid control or a standing refusal line, one per setting; a fused box's parts count apart), so no plugin writes the sentence. Each correction clears its refusal and the count turns over; at none the tally turns green, "Ready to save", and folds away. A form's own error band is only for a refusal no field owns, and keeps a dividing line's 24px below it.
- **Checkboxes:** first in the row, 8px before the label, aligned to its first line. 18px, 2px corners, White with the inset shadow and a Glyph outline; checked is Denim with a white check. Every switch in the app is drawn as a checkbox, because every change is staged.
- **Choice colours:** radios, checkboxes (including table and log controls) and multi-select chips share `--color-choice` (Denim) in `palette.css`; `--color-choice-border` (Glyph) defines custom empty checkbox outlines. Selection keeps a visible radio dot or check, and keyboard focus gets a separate outline. The selected fill contrasts 5.4:1 with Paper and 5.7:1 with white checks or chip text; the empty outline contrasts 3.5:1 with Paper and 3.3:1 with Quiet Sand. Native controls retain platform rendering and forced-colour support. Labels keep their existing text colour.
- **Single-choice sets:** up to three choices use visible native radios in a vertical group; four or more use a native dropdown whose width follows the longest option. Radio rows are at least 32px tall, with 4px vertical padding and an 8px gap between circle and label; wrapped labels grow naturally. Apply this throughout page forms and drawers, including Accept / Reject / Drop. The shared `RadioOptionLimit` constant in `internal/widget/field.go` owns the cutoff; change it there to adjust every form. Radio groups keep their field label, keyboard navigation, help and error associations. Segmented chips are reserved for selecting multiple members, such as weekdays; boolean settings keep their checkboxes.

### Navigation
- **Rail:** 288px, no fill of its own, one hairline against the page. Top-level rows carry a 16px Lucide icon in Glyph (Ink when active), wash to Mid Sand and slide 4px right on hover. The row you are on is the only surface: Quiet Sand with a 2px Ink bar on its right edge, against the page it opens. A parent whose sub-page is open goes bold Ink without the surface, and its sub-pages hang off a hairline under its icon, with no icons of their own; the open sub-page carries the 2px Ink bar on the same right edge.
- **Top bar:** White, 56px. On the left, the router: the hostname in bold mono, its maker and model, and the staged-changes chip beside them, because the stage is the router's (held, applied and rolled back on the device), not the session's. The chip never gives way; the hostname truncates first. On a phone it says the square and the bare count. On the right, alone and aligned to the content column, the session: `root | Log out`.
- **Staged chip motion:** a saved change's square flies from its row to the chip, and the count turns over as it lands: the old count rises out of the chip's line as the new one rises in (140ms out, 260ms in, clipped by the chip). Under reduced motion the count simply changes.
- **Mobile:** the rail becomes an off-canvas drawer behind a menu button, over an Ink scrim.
- **Colophon:** every page's last line, and only a log goes without one (a log runs on as long as the router writes). It is a 52px Quiet Sand band (the 28px copy plus 12px above and below) under a Hairline, running from the rail to the window's edge. On a short page it stands at the bottom of the window, and on a long one under the last of the content. Its words keep to the page's column, level with the h1, and end under Log out. On the left is the OpenWrt release at 500, then its revision (from 640px) and its target (from 1280px), all in 16px mono Meta with Meta middots between them. On the right is the Verso build, then a 28px copy icon ("Copy for a bug report") that puts the release, revision, target, board, kernel and Verso build on the clipboard, one labelled fact a line and in English. The line answers the copy by washing each fact it shows in Strong Hairline, never a hue, with 6px of padding either side of its words, one after another from the left (70ms apart, 1200ms each), with the words in Ink under the wash, and the icon turns to a green check. Under reduced motion only the check shows.

### Staging
Save stages a change; nothing happens on the router until it is applied from the chip's review drawer. Marigold means exactly that, waiting, and nothing else says a staged save happened, because it has not.
- **No band:** a staged save puts no notice on the page, green or otherwise. The chip is what says a change waits. Only a plugin's warning or refusal about the change still speaks, in its own tone.
- **What waits, everywhere:** the router holds the stage, so every setting whose change waits carries the marigold "staged" mark beside its name (the chip's own 6px square and word) on every visit, from any page, until the change is applied or discarded. A control finds its change by the option's full address: its key, and the config and section it lives in, which the plugin states once on a form or section or on the control itself.
- **Where it went:** the page comes back where the person was (a page form's Save at the same height), its changed rows already marked. Once, a moment after the page is drawn, a marigold square lifts off the first changed row and travels in an arc to the chip's mark (680ms). The chip shows its old count until the square lands, then takes the new one and warms once to the marigold line. A panel's save flies from the row it saved, which washes marigold for a moment. An empty chip appears as the square arrives.
- **Discard and Apply show at once:** the moment the stage changes under an open drawer, the page behind it shows the settings as they are on the router, with the marks gone and nothing reading as unsaved. It isn't left stale until the drawer closes. Only a change to which rows a listing has waits for the page to be read whole, on close. The chip never shows the new count before a flying change reaches it; it is held back from the first paint of the page a save lands on.
- **Only on growth:** it flies only when the stage grew. Under reduced motion, or with no chip on screen, the chip only warms. A screen reader hears "Staged — N staged changes. Review and apply from the top bar."

### Drawers
- **Shell:** a panel from the right on Paper, behind a Strong Hairline and cast with the drawer shadow. It slides in over 300ms (ease-out) and out over 200ms (ease-in). Every drawer is 768px wide, whatever it holds (a form, a chooser, a reading, the staged review), so the frame never changes size from one object to the next. On a narrower screen it takes the full width. Header is a 52px Quiet Sand band holding only the title ("New rule", "Edit Allow-Ping") and a 28px close.
- **Sections:** each drawer starts its own heading hierarchy, independent of the page section that opened it. Full sections use the standard Quiet Sand heading band, edge to edge, with an 18px heading and any lede inside. The band has no border and keeps 16px vertical padding, 24px separation from the preceding section and 20px before its content. Headings, ledes and fields retain their 40px horizontal inset; subsection and list dividers stay inset. The opening section has no leading divider: the chrome's own Hairline (the header's, or the tab strip's) is its rule, and its title stands 32px under it, as under any rule. A body that opens on anything else keeps its 28px top inset.
- **Configuration card:** the drawer reads as form name → fields → configuration card → actions. The filename alone labels the card, above the code; it has no section heading, marker, “Written to” prefix or repeated explanation of staging. A full-width hairline separates it from the fields; the filename and code keep the content's 40px horizontal inset. Live previews and copy controls stay part of the card.
- **Footer:** when a configuration card ends the form, Save follows it with a small gap and no intervening hairline: the card and the act that saves it are one unit. Other drawer footers keep their inset Hairline. Removing an object belongs to its listing’s trash action, which opens a compact confirmation naming the object and its consequences, with the destructive act and Cancel. Edit drawers carry no duplicate removal action.

### Live Control
- The Live button on a log: the waiting mark turns inside it while lines arrive and stands still when paused or disconnected. Its label is the state (Live, Paused · 12 new, Connecting…); its tooltip names the act (Pause, Resume). It stands on the heading line beside Download and Settings, all three as equals in the secondary dress.

## Do's and Don'ts

### Do:
- **Do** keep one denim primary per page, on the heading line, with the plus glyph.
- **Do** put everything that narrows a list or log on the full-width Quiet Sand control band, and everything that acts on the page on the heading line.
- **Do** use a counted `<select>` for every cut ("IPv6 · 19"), naming the whole set in its first option ("All devices", not "All").
- **Do** space blocks 40px apart and give a section hairline 40px on both sides; step 20px inside a group.
- **Do** build rows from a 24px line plus padding (44px), bands at 52px, controls at 36px (28px inside a row).
- **Do** set machine strings in Inconsolata 16px, ledes in Hanken Grotesk 16px/300, and compact body text in Hanken Grotesk 14px.
- **Do** carry state with the 6px packet square in its hue; hollow for absent.
- **Do** keep the resting state silent: say only what is wrong, in plain words.
- **Do** ask before a consequential act, in place and in one hue: crimson when it destroys or drops something, marigold when it is disruptive but wanted (installing firmware).

### Don't:
- **Don't** use a segmented switch or toggle track on a control surface; cuts are dropdowns, switches are checkboxes.
- **Don't** set type in a full-chroma hue, or in Faint or Glyph.
- **Don't** cast shadows on anything that sits on the page; only floating layers get one.
- **Don't** use more than 2px corners on a surface, or circles for state.
- **Don't** mark a message with an alert glyph. An error, a notice, a warning, a banner or a confirmation says its tone with the 6px state square in that tone's hue, hung on its first line (the flash, a form's error, the no-password band, the crimson and marigold confirms).
- **Don't** repeat a failure in every form on the page. A failed act is said once, in the page's notice.
- **Don't** add icons to buttons for emphasis. Glyphs appear only on the primary (its plus), on the direction glyphs (download, upload, open-out), and on every subsection act, where the glyph names the act.
- **Don't** use a circular spinner; waiting is the four-square mark.
- **Don't** fix a row or band height with a height utility; heights come from line plus padding.
- **Don't** centre the page or let a band's controls leave the page's column.
- **Don't** fill a table with stripes or vertical rules, or make a whole row clickable.
- **Don't** pair a danger button with a sand or bordered secondary; the way back speaks the question's hue.
