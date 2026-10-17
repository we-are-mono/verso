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
