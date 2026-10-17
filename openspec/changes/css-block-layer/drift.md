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
| mono 16/400 on 16, tabular | 28 | zone chips ("wan") | `type-chip` verbatim | 16/400 → 14/500 |
| mono 14/500 on 24 | 18 | config file path strip | `type-chip` verbatim | line 24 → 16, the strip pads the 8px |
| mono 14/400 on 24 | 8 | editor line count, gutter | `type-code` | 14 → 16 |
| mono 14/400 on 24, tabular | 112 | editor line numbers | `type-code` + figures | 14 → 16 |
| mono 14/400 on 22.75 | 2 | file editor textarea | `type-code` | 14/22.75 → 16/24, as the read-only editor |
| mono 11.9/400 on 24 | 4 | inline `code` in prose | `type-body` verbatim | 11.9 → 16, the one step up every machine string takes |
| mono 36/700 on 40, −1.8px | 12 | Overview gauge figures | `type-figure` | = |
| mono 36/700 on 36, −1.8px, tabular | 3 | restart countdown | `type-figure` | line 36 → 40 |

Six groups below fit no approved role. They are in the next section, waiting
on the owner.

## Proposed roles (owner's decision)

| Today | n | What | Proposal |
|---|---|---|---|
| sans 12/500 on 24, tabular | 78 | chart axis labels | **`type-axis`**: DESIGN › Charts already specifies it ("sans 12px at 500 in Meta on 24px lines, tabular figures") |
| sans 14/400 on 17.5 (and tabular) | 740 | tooltips (icon-button tips, copy tips) | **`type-tip`**: a tip is compact on purpose; on 24px lines a three-line tip grows by 20px |
| mono 20/700 on 36, −0.5px (and on 20) | 57 | the router's name, in the rail head, sign-in and takeovers | **`type-nameplate`**: no role is a 20px mono mark; the figure is 36px |
| sans 16/400 on 40 | 9 | the unit beside a gauge figure ("%") | **`type-figure-unit`**, or converge on `type-body` (14px, a visible step down) |
| sans 16/300 on 24; sans 16/400 on 28 | 4 | takeover ledes | **`type-takeover-lede`**: DESIGN says "A takeover's lede keeps 1rem/16px", so it is a role of its own |
| sans 24/600 on 30; 24/600 on 40; 36/500 on 40 | 4 | takeover headings | converge on `type-headline` (firmware-started's 36/500 becomes 24/600), or **`type-display`** for the takeovers' one heading |
