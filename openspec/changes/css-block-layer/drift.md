# Drift

Where copies of one look, or one kind of text, differ today, and what they
converge on. The type mapping is task 3.1's; each block task adds the
convergences it makes. Measured from the baseline snapshot: computed type of
every element that holds its own text, all reference surfaces at 1440px
(51 distinct combinations).

## Type: combinations → roles

Columns: computed face, size/weight on line, tracking, case, numerals, count of
elements, what they are, and the role they take. "=" means the combination
already is the role; anything else is a convergence, its difference named.

| Today | n | What | Role | Difference |
|---|---|---|---|---|
| sans 14/400 on 24 | 3582 | body text, cells, descriptions | `type-body` | = |
| sans 14/400 on 24, tabular | 318 | counts and durations in cells | `type-body` + figures | = |
| sans 14/400 on 21 | 77 | select options | (native control) | not text the shell sets |
| sans 14/400 on 14 | 10 | Overview window select options | (native control) | not text the shell sets |
| sans 14/400 on 20.3 | 1 | restart takeover address sentence | `type-body` | line 20.3 → 24 |
| sans 14/400 on 22.75 | 1 | sign-in paragraph | `type-body` | line 22.75 → 24 |
| sans 14/400 on 28 | 14 | drawer tab names | `type-body` | line 28 → 24, the tab pads the 4px |
| sans 14/400 on 16 | 28 | sans chips ("router") | `type-chip` | = |
| sans 14/300 on 24 | 4 | ledes in drawers | `type-lede` | = |
| sans 14/300 on 21 | 1 | sign-in board name | `type-lede` | line 21 → 24 |
| sans 14/500 on 24 | 464 | row names, field labels | `type-field-label` / `type-value` | = |
| sans 14/500 on 24, tabular | 30 | chart legend figures | `type-value` + figures | = |
| sans 14/500 on 28 | 21 | acts ("Add rule") | `type-field-label` | line 28 → 24, the act pads the 4px |
| sans 14/600 on 24 | 303 | buttons, sub-headings | `type-button` / `type-subheading` | = |
| sans 14/600 on 16 | 183 | column heads in cards | `type-column-head` | = |
| sans 14/600 on 28 | 29 | Devices lane bands ("Local network") | `type-subheading` | line 28 → 24, the band pads the 4px |
| sans 14/600 on 21 | 1 | sign-in board maker | `type-subheading` | line 21 → 24 |
| sans 14/700 on 24 | 1 | upgrading lede's `<strong>` | `type-body`'s strong | 700 → 600, the weight `strong` takes in every role |
| sans 16/600 on 24, −0.4px | 108 | card titles | `type-card-title` | = |
| sans 16/600 on 24 | 2 | port forward drawer's fold summary | `type-subheading` | 16 → 14, tracking as the role |
| sans 18/600 on 28, −0.45px | 20 | drawer titles | `type-title` | = |
| sans 24/600 on 26.4, −0.6px | 55 | page h1 | `type-headline` | = |
| sans 12/500 on 24, 0.08em, caps | 20 | Overview gauge labels | `type-label` | line 24 → 16, the gauge pads the 8px |
| sans 12/500 on 18, 0.08em, caps | 1 | firmware-started kicker | `type-label` | line 18 → 16 |
| sans 12/500 on 16, 0.08em, caps | 1 | sign-in "Live" | `type-label` | = |
| mono 16/500 on 24 | 833 | fact values, log lines | `type-value` verbatim | = |
| mono 16/400 on 24 | 621 | names and addresses in cells, list-field values | `type-body` verbatim | = |
| mono 16/400 on 24, tabular | 328 | figures in mono cells | `type-body` verbatim | tabular → none: a machine string is not a count (the Verbatim Rule) |
| mono 16/700 on 24 | 27 | interface chooser names, log level | `type-value` verbatim | 700 → 500 |
| mono 14/500 on 16 | 272 | mono chips ("server") | `type-chip` verbatim | = |
| mono 14/400 on 16 | 249 | mono chips ("daemon", rule names) | `type-chip` verbatim | 400 → 500 (DESIGN: a mono chip is 500) |
| mono 16/400 on 16, tabular | 28 | traffic endpoint chips ("wan") | `type-chip` verbatim | 16/400 → 14/500; a 16px line role for them would be a 22nd role, and the chip's 16px line is what keeps every chip 18px |
| mono 14/500 on 24 | 18 | config file path strip | `type-chip` verbatim | line 24 → 16, the strip pads the 8px |
| mono 14/400 on 24 | 8 | editor line count, gutter | `type-code` | 14 → 16 |
| mono 14/400 on 24, tabular | 112 | editor line numbers | `type-code` + figures | 14 → 16 |
| mono 14/400 on 22.75 | 2 | file editor textarea | `type-code` | 14/22.75 → 16/24, as the read-only editor |
| mono 11.9/400 on 24 | 4 | inline `code` in prose | `type-body` verbatim | 11.9 → 16, the one step up every machine string takes |
| mono 36/700 on 40, −1.8px | 12 | Overview gauge figures | `type-figure` | = |
| mono 36/700 on 36, −1.8px, tabular | 3 | restart countdown | `type-figure` | line 36 → 40 |

| sans 12/500 on 24, tabular | 78 | chart axis labels | `type-axis` | = |
| sans 14/400 on 17.5 (and tabular) | 740 | tooltips (icon-button tips, copy tips) | `type-tip` | = |
| mono 20/700 on 36, −0.5px; on 20 | 57 | the router's name, top left of the top bar, sign-in and the restart takeovers | `type-title` verbatim | line 36 / 20 → 28, the top bar pads the mark back to centre |
| sans 16/400 on 40 | 9 | the unit beside a gauge figure ("%") | `type-figure-unit` | = |
| sans 16/300 on 24 | 3 | restart takeover ledes | `type-takeover-lede` | = |
| sans 16/400 on 28 | 1 | firmware-installing lede | `type-takeover-lede` | 400 → 300, line 28 → 24 |
| sans 24/600 on 30 | 3 | restart takeover headings | `type-headline` | line 30 → 26.4 |
| sans 24/600 on 40 | 1 | upgrading heading | `type-headline` | line 40 → 26.4 |
| sans 36/500 on 40, −0.9px | 1 | "Your router is restarting" (firmware installing) | `type-headline` | 36/500 → 24/600, line 40 → 26.4 |

## Roles added by the owner (2026-10-17)

Four roles join the 17: `type-axis` (DESIGN › Charts already specified it),
`type-tip` (a tip is compact on purpose), `type-figure-unit` (the unit beside a
figure keeps its size) and `type-takeover-lede` (DESIGN gives a takeover's lede
16px). The router's name takes `type-title`'s verbatim form, 20/700 at
−0.025em, rather than a role of its own, and every takeover heading takes
`type-headline`. 21 roles in all.

## Blocks: convergences

Each block conversion lists here what its copies differed in and what they
became, measured by the layout snapshot against the task before it.

### `mark` (the 6px state square)

| Copies | n | Difference | Became |
|---|---|---|---|
| filled marks | ~250 | no border | a 1px transparent border, the fill under it: the same pixels, one box for every mark |
| hollow marks on Interfaces | 24 | Inert edge | Faint edge, as every other hollow mark |
| notice and confirm marks (callout) | ~12 | square corners | 1px corners, as every other mark |
| the stream's live retone of an interface mark | | `info` drawn hollow | `info` drawn Denim, as the server draws it |

The rail's 5px square stays as it is until the rail converts.

### `chip`

| Copies | n | Difference | Became |
|---|---|---|---|
| mono chips in tables and lists | ~440 | weight 400 (a table rule outranked the chip's 500) | 500, the chip role's verbatim form |
| traffic endpoint chips (firewall rules) | ~60 | mono 16/400, tabular figures | the chip role's verbatim form, 14/500, no tabular figures (an address is not a count); 3px narrower |
| a tag's or a pill's glyph | | Faint whatever the tone (tags), the words' ink (pills) | the tone's glyph ink, Faint when neutral |
| a toned chip's words | | each site's own class list | one `data-verso-tone` per chip |

### `link`

| Copies | n | Difference | Became |
|---|---|---|---|
| a notice's door | 3 | an unbreakable box (inline-flex) in the words' ink by a rule reaching in from the notice | a door the shell marks: inline words in their own ink, wrapping as the sentence does |
| the "router didn't answer" retry | 1 | 600 weight, Ink under the pointer | a door: the band's words' weight and ink, its underline thickening |
| row links and phrase links | | their own underline and focus utilities | the link block's door |

### `tip`

| Copies | n | Difference | Became |
|---|---|---|---|
| a setting's sentence tip | every labelled setting | its sentence on a 1.45 line, its footer on a 24px line | the tip role's 1.25 line; the footer the chip role's verbatim form on its 16px line |
| the "Remove" act tip in a row's acts | 1 | raised on hover only | raised on hover and on focus, as every act's tip |
| act tips in table cells | | kept their line from a `leading-tight` class a table rule looked for | the tip role's line; the table rule names the tip block (until the table converts) |

### `button`

| Copies | n | Difference | Became |
|---|---|---|---|
| buttons led by a glyph (the Button widget, the action bar's acts, Logs' acts) | ~20 | an 8px gap between glyph and words | the component's 6px (layout-grid); a right-aligned bar shifts 2px |
| the action bar's and Logs' buttons | 7 | hand-written flex boxes | the button block (inline-flex, the same box) |
| an inline form's submit | | `rounded-l-none` lost to a missing space in its class list | square on the left, joined to its field, as written |
| a confirm's trigger | 2 | its own wash classes | the block's danger-wash and caution-wash |
| the script's in-flight button | | a `verso-button-waiting` class | `data-verso-busy` |
