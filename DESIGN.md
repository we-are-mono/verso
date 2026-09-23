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
  body:
    fontFamily: "Hanken Grotesk, ui-sans-serif, system-ui, sans-serif"
    fontSize: "0.875rem"
    fontWeight: 400
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
  drawer-form:
    backgroundColor: "{colors.ground}"
    width: "640px"
  drawer-wide:
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
- A 40px page rhythm; 44px rows, 52px bands, 36px controls.
- A full-width sand control band is the seam between a page's heading and its content.

## Colors

A warm, near-neutral sand ground carries a single cool action blue and four status hues that only ever mark.

### Primary
- **Denim** (#3568a8): the only colour that means "act on this, or this changed" — the primary button, links, focus, the selected tab underline. Nothing else is denim, and a page with no primary act has no denim.
- **Deep Denim** (#285184): denim one step darker, for hover and focus rings.
- **Denim Hairline** (#cbd8e6) and **Denim Wash** (#e8eef5): the hairline and ground of the accent chip ("configured by hand": reserved, static) and of info bands.

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
- **Strong Hairline** (#d2ccc2): hairlines between sections, under a lane, every input border.
- **Inert** (#c9c3b8) and **Parked** (#c1bab0): an off switch track, an empty port; the waiting mark's parked squares.
- **Faint** (#a09a8e) and **Glyph** (#8b857a): decorative and action icons. Never text.
- **Meta** (#6f6a60): kickers, column heads, units, placeholders — the lightest ink that may carry words.
- **Body** (#5c574f): body copy and every label on a card.
- **Ink** (#1b1917): headings, values, a checked box.

### Named Rules
**The One Voice Rule.** Denim means act. One denim button per page, on the heading line; a page with nothing to make has no denim at all.

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
- **Title** (600, 1.125rem/18px, -0.025em): band and drawer titles, in Body ink on sand.
- **Sub-heading** (600, 0.875rem/14px): a section inside another section, as an h3 in Ink ("Authorized keys" under SSH), so a part of a subject never reads as a peer of its heading. It stands on a **ledger line**: the name, then how many things the part holds (500, Meta, tabular), then a Hairline running on to the column's end, so it reads as the start of a group and never as one more field label. The count is the set's own length. When it differs from the last time the page was shown, the old number rolls out and the new one rolls in (up for more, down for fewer, 420ms exponential ease-out), arriving in Ink and settling to Meta. Under reduced motion it simply shows the number. The part's first box stands 24px under the name, as a form's rows stand apart (a set's first item gives up its own air above).
- **Body** (400, 0.875rem/14px, 1.5): all prose, labels (600), buttons (600), cells. Ledes cap at 60ch.
- **Label** (500, 0.75rem/12px, 0.08em, uppercase): kickers and column heads, in Meta.
- **Value** (Inconsolata 500, 1rem/16px): machine strings in rows — addresses, interface names, MACs, versions. Inputs for identifiers use Inconsolata 400 at 16px.
- **Figure** (Inconsolata 700, 2.25rem/36px, -0.05em): big numbers.

### Named Rules
**The Verbatim Rule.** Mono is for strings the machine wrote and a user might copy: identifiers, addresses, versions, config keys. Never for a count, a duration or a sentence. Counts are sans with tabular figures.

**The Even Pitch Rule.** Mono never takes negative tracking, except the 700-weight Figure, whose wide glyphs can carry it.

## Layout

The page is left-aligned against a 288px rail, never centred. The frame keeps 40px of air on every side, and inside it the page declares a measure: wide pages cap at 1152px, forms at 640px. The top bar's right group ends on the same column edge, so a page's primary act sits exactly under Log out.

**The rhythm is 40, 24 and 20.** Blocks on a page stand 40px apart, the same as the body inset. Every line that divides a page keeps the title's own air on both sides: the title stands 20px under the page's top edge in a 36px heading line, so its words sit about 24px from the edge and from its line, and every section rule, the page Save's rule and a plugin's seam keep 24px above and below. A section holds that air itself: the rows it starts and ends with give their own padding back (a titled section keeps its first row's, as the air under its heading), and the last row of a hairline-divided list drops its padding with its hairline. The same holds with or without a heading. The 24px is measured from a row's box, not its words: a row is always its control's 36px, so a checkbox row, whose label is centred on that line as the title is on its own, keeps a few pixels of its own air below its words, and that is not trimmed. A rule runs out by 40px to the rail on the left, and stops where the column ends on the right, clear of any sidebar beside the column. The rule is the page's, so it starts at the page's frame, while what it separates stays in the column. The masthead ends in one line 20px under the title's heading line (or under its lede, when it has one), and what follows it starts 24px under that line: a masthead labels the page and keeps no more air than that. The line runs to the rail as a section rule does. The home page's sentence keeps the full 40px above it. Unlike a section rule, it ends on the right where Log out does, whatever measure the column keeps, and the page's acts on the heading line end there too. Its line is a hairline on a page of forms or sections, the control band's top edge on a list or log, never both. Only the home page, whose heading is a sentence, stands unruled. A drawer keeps its own rules inside its padding. Inside a group (heading to toolbar to table) the step is 20px. Rows are 44px (a 24px line plus 10px above and below), section bands are 52px, controls are 36px, and a control inside a row is 28px. Heights come from line plus padding, never from a fixed height, so a row stays a row whatever it holds.

Acts sit at the level they act on. A page's acts stand on its heading line. A form that is the whole page commits every section on it, so it closes on a section rule of the page's (40px either side, run out to the rail) with its Save under it. A form in a drawer closes on its own hairline inside the drawer. A form that commits one section draws no hairline: its Save stands 20px under its last field, and the next section's rule is the only line between them. Within a section, its settings and their Save come first, then what the section holds (keys, a certificate), with that thing's own acts 16px under it.

Every list page reads top to bottom as: the heading line (h1 left, page acts right), then the **control band**, then the content flush beneath it. The band spans the full page width, from the rail to the window edge, while its controls stay in the page's column, level with the h1. It is the seam between "the page" and "the thing on the page".

Grouped listings split into lanes: each lane head is a 44px row on a strong hairline, and every lane after the first stands 40px off the one before, so each chain reads as its own small table.

On a phone the rail becomes an off-canvas drawer, bands stack their controls, and wide tables scroll inside their own box rather than widening the page.

## Elevation & Depth

Verso is flat at rest. Depth comes from tone (Paper → Quiet Sand → Mid Sand) and from hairlines, not shadows. Shadows exist only on things that float over the page, and only as much as it takes to lift them.

### Shadow Vocabulary
- **Field inset** (`box-shadow: inset 0 1px 2px rgba(27,25,23,.06)`): every input and select — a field reads as a slot cut into the paper.
- **Tooltip** (`box-shadow: 0 4px 12px rgba(27,25,23,.14)`): tooltips and hover tips.
- **Drawer** (`box-shadow: -8px 0 32px rgba(27,25,23,.12)`): the 640px side panel, cast leftward over the page.
- **Dialog** (`box-shadow: 0 8px 32px rgba(27,25,23,.16)`): modal dialogs and alerts. A confirmation asks in place and casts none.

### Named Rules
**The Flat-By-Default Rule.** Nothing on the page casts a shadow. Only a layer that floats above the page (drawer, dialog, tooltip) does, and a pressed button loses even its press shadow.

## Shapes

Corners are nearly square: 2px on every button, field, chip, band and card; 1px on state marks. Circles appear only for toggle knobs, meter pills and the interface tree's physical/software encoding.

The state mark is a 6px square (5px in the nav), the packet. Filled means present in its hue; hollow (Faint border, no fill) means absent. The waiting mark is four of those squares folding on an 8px grid. Never a circular spinner.

Borders are hairlines, 1px, in Hairline or Strong Hairline. Tables have no vertical rules and no stripes; rows are separated by a single hairline.

## Components

### Buttons
Plain and exact.
- **Shape:** gently squared (2px), 36px tall, 16px side padding, 14px/600 text.
- **Primary:** Denim fill, white words, and always a leading 16px plus glyph, unless the act leads elsewhere and names its own (download, upload, open-out). It lives on the heading line, one per page.
- **Secondary:** the subsection act's colours at any size: transparent, Meta words, Strong Hairline border; on hover the border turns Parked, the fill Hairline, the words Ink. Every quiet button answers the pointer alike (a list's Add, Cancel, a log's Live · Download · Settings, a row button). Words only; no decorative icons.
- **Danger:** Crimson fill, white words, for the confirmation step only, and always beside its way back (see Confirmation).
- **Subsection acts:** the acts on a part of a section: "+ Add a key", and a certificate's Install · Make a new one · Download. One dress for all of them:
  - 28px tall, 14px/500 words in Meta, a Strong Hairline border on no fill.
  - Always a leading 16px glyph that names the act (plus, upload, refresh, download), with 8px padding before it and 10px after the words.
  - On hover the border turns Parked, the fill Hairline, the words Ink.
  - One exception: an act on a section's heading line (General's "Use my computer's time") stands in the field column, where a field would, so it starts where the fields' controls start and takes a field's 36px height with 16px side padding, in the same colours.
- **Row buttons:** 28px, 12px side padding, the same secondary dress. Every icon-only act wears one dress, wherever it stands (a row's edit or remove, a list's remove ×, a copy beside a value or on a code box): a 28px square with a 16px glyph in Glyph on no border, taking a Parked border, the Hairline fill and Ink on hover, with a tooltip. Beside a line of text it keeps the line's height.
- **Press:** every button nudges down 1px and drops its shadow when pressed (off under reduced motion).
- **Resting:** a settings form's Save before anything in it has changed is disabled in Quiet Sand, a Strong Hairline border and Meta words, and turns denim the moment something changes. A page with no primary act and nothing changed therefore shows no denim.
- **Waiting:** a button busy with a slow act keeps its footprint, goes Quiet Sand with Parked words, shows the waiting mark and a present-participle label with a real ellipsis ("Applying…").

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
- **Accent:** Denim Wash with Deep Denim words and a leading 12px icon, for what was configured by hand. A green variant marks "you" (this browser, this session). No other chip colours.
- **Entity chips:** a neutral icon plus label for an interface, zone or port cited inside another row.
- **One box, always a border:** every chip that cites something is the same box: a row's tag ("this browser"), the router at one end of a firewall rule, a rule's action (`accept`, `reject`, `drop`, `mark`; `drop` has no hue and is the neutral chip), the protocol beside IPv4, a panel tab's state, a choice's config value, an interface in a row. That box is 14px/400 words on a 16px line, 2px above and below, 6px sides and a 1px border, 22px tall. The border is its hue's Hairline on its hue's Wash, or Hairline on Quiet Sand for a neutral one. Only the face varies: mono for a string the machine wrote (`accept`, `lan`, `dhcp`), sans for words ("this browser", the router, which names a thing rather than a value). A mono chip is one step heavier, 500, because Inconsolata at 400 reads a size smaller than the Hanken beside it.
- **One mark or none:** a chip leads with nothing, a 14px Lucide icon, or the 6px packet square in its hue (hollow when the chip has none, the packet absent). Never two; an icon wins. The packet in a chip is still: a chip states a fact, and the only pulse is the page's live mark. A firewall verdict (`accept`, `reject`, `drop`, `mark`) always leads with its packet, and `drop`'s is hollow. On a 20px detail line the chip overhangs by a pixel either side rather than making the line taller. The removable token inside a list control is not a citation and keeps its control-sized box.

### Control Band
The signature seam of every list and log page.
- **Surface:** Quiet Sand, a Hairline above and below, 16px vertical padding, spanning the page from rail to window edge. It stands 20px under the title, and its top Hairline is the masthead's line; the masthead draws none of its own above a band.
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

### What needs a package
A capability a page can't offer until a package is installed (encrypted DNS, a blocklist, a local resolver). It's offered where the need is, and installed there.
- **A settings row:** the hollow packet and the fact ("Queries leave in plain text") stand where a setting's name stands, with a sentence under it if it has one. The act stands where a setting's control stands: a field-height quiet button with the download glyph that names its package ("Install https-dns-proxy"). It's never a band ruled off above and below, and never a link out of the page.
- **The package's own drawer:** the act opens, over the page, the drawer Packages opens from the package's row (what it is, its facts, Install). The page keeps its address. After installing, the drawer closes and the page reads itself again, so the fact becomes the setting the package adds.
- **Packages by name:** a search that reaches Packages without a view searches everything, not only what's installed.

### Collections
A short set of machine strings someone keeps by hand: authorized keys, a tunnel's peers. It's not a table. A handful of identities needs no rules, and every act on the set happens where the set is.
- **Items:** the identity in 16px mono Ink over one line of detail in 14px mono Meta (a fingerprint, an address), wrapping anywhere on a phone. Items are separated by their own 10px of air, never a hairline.
- **Empty:** where the first item would stand, a slot: Quiet Sand inside a dashed Strong Hairline (the one dashed line in the app; it marks a place, not a thing), 2px corners, 16px sides, a 24px line with 10px above and below. It holds the hollow packet and the set's empty words ("No keys are authorized.") in 14px Meta. It is inert, with no hover and nothing to press. The add stands 16px under it, where it stands under the items once there are some.
- **Remove:** a 28px trash icon on the item's first line, named with the item for screen readers. Pressed, the crimson confirmation drops full-width under the item ("Remove this key?", **Remove key** · Not now), focus on the act, and Escape or Not now returns focus to the icon.
- **Add:** "+ Add a key" (a subsection act) at the foot unfolds in place into a mono box (cursor in it), the plugin's live reading of what is typed (a key reads as `ssh-keygen -l` prints it), and **Add key** · Not now. The reading stays hidden until there is something to read. A refused paste comes back open, as typed, with its reason under the box.
- **After:** the change runs at once and the page reads the router again, saying once what happened ("Key added.").

### Inputs / Fields
- **Style:** White fill, Strong Hairline border, 2px corners, field inset shadow, 36px tall. Sans 14px for words, mono 16px for identifiers.
- **Width by content:** full for free text and every select; 176px for fixed-format identifiers (IPv4, MAC, time); 96px for numbers (port, MTU, VLAN id).
- **List fields** (a UCI `list`, such as time servers): one value per row in the control column, 16px mono, with its 28px remove on the right and a Hairline between rows. A row is the remove and 8px above and below, 44px, never a fixed height. The first row gives back 4px above, so its value centres on the label's line as every control does. The add box and its Add stand 16px under the last remove. With nothing in the list, the box stands level with the label. A value too long for the column wraps after its own separators: slashes first, then dots, then colons (`/corp.example.com/` over `10.66.0.53`). It breaks anywhere else only when one part alone is wider than the column. Its remove hangs from the first line.
- **States:** only the border changes — Strong Hairline at rest, Faint on hover, Denim while focused. A refused field stays the one that is wrong while it is corrected: Crimson at rest, Deep Crimson under the pointer, and a 2px Deep Crimson edge while focused (an inset line, so the box never grows), with its reason under it on a band of Crimson Wash (12px side padding, 8px vertical, 2px corners: the crimson square, then the words). The band is the same under a field, a key's box or a list's box. A page that comes back refused opens at its first refused field, in the middle of the view with the cursor in it, never at the page's top. The first change to its value answers the refusal, so the box returns to the Strong Hairline and Denim focus and the reason goes; submitting asks again. Placeholders are Meta, never lighter.
- **Checkboxes:** 18px, 2px corners, White with the inset shadow; checked is Body ink with a white check. Every switch in the app is drawn as a checkbox, because every change is staged.

### Navigation
- **Rail:** 288px, no fill of its own, one hairline against the page. Top-level rows carry a 16px Lucide icon in Glyph (Ink when active), wash to Mid Sand and slide 4px right on hover. The row you are on is the only surface: Quiet Sand with a 2px Ink bar on its right edge, against the page it opens. A parent whose sub-page is open goes bold Ink without the surface, and its sub-pages hang off a hairline under its icon, with no icons of their own; the open sub-page carries the 2px Ink bar on the same right edge.
- **Top bar:** White, 56px, the hostname in bold mono on the left, the staged-changes chip and Log out on the right, aligned to the content column.
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
- **Shell:** a panel from the right on Paper, behind a Strong Hairline and cast with the drawer shadow. It slides in over 300ms (ease-out) and out over 200ms (ease-in). Its width follows its content: 640px for a form (the standard edit drawer), 768px for a wide read, 448px for a short one. Header is a 52px Quiet Sand band holding only the title ("New rule", "Edit Allow-Ping") and a 28px close.
- **Footer:** trailing acts sit below a Hairline, buttons only.

### Live Control
- The Live button on a log: the waiting mark turns inside it while lines arrive and stands still when paused or disconnected. Its label is the state (Live, Paused · 12 new, Connecting…); its tooltip names the act (Pause, Resume). It stands on the heading line beside Download and Settings, all three as equals in the secondary dress.

## Do's and Don'ts

### Do:
- **Do** keep one denim primary per page, on the heading line, with the plus glyph.
- **Do** put everything that narrows a list or log on the full-width Quiet Sand control band, and everything that acts on the page on the heading line.
- **Do** use a counted `<select>` for every cut ("IPv6 · 19"), naming the whole set in its first option ("All devices", not "All").
- **Do** space blocks 40px apart and give a section hairline 40px on both sides; step 20px inside a group.
- **Do** build rows from a 24px line plus padding (44px), bands at 52px, controls at 36px (28px inside a row).
- **Do** set machine strings in Inconsolata 16px and everything else in Hanken Grotesk 14px.
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
