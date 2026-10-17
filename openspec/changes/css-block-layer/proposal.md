# Proposal

## Why

Nothing on screen changes, except where copies of one look have drifted apart: once they share a block or a type role they become identical again, which is the fix this change is for. What changes is that every look the app draws, and every piece of text in it, gets one name and one definition. A fix to a card, a chip, a field row, a button or a card title then reaches every place that draws it. The same rules hold every future screen to that, with tests to enforce them.

Today a look repeats as a run of utilities in each place that draws it, so a fix to one copy leaves the others as they were. The card is the plainest case: page and drawer cards take their frame from `sections.css`, and the Overview card writes its own `rounded-xs border border-rule-strong bg-ground` with its own paddings. Measured across templates and client scripts:

- 47 utility bundles appear identically on two or more elements, 113 places in all. Copies that have already drifted apart don't show up in an exact match, and the card is one of those.
- 21 named templates (`verso-section-band-class`, `verso-chip-class`, …) exist only to print a class string. This was an earlier answer to repetition: it saves typing, but leaves the stylesheet unable to name the thing.
- 89 `[&…]` arbitrary-variant selectors and a run of `!` utilities live in markup. These are stylesheet rules written in the template.
- The stylesheet infers what an element is from where it stands. A card is `:is(.verso-page-body, [data-verso-drawer-body]) section[data-verso-section]:is(:has(> [data-verso-section-band]), [data-verso-headless][data-verso-ruled]):not(section section)`, and the edge rules built on that run to about 500 characters.
- DESIGN.md › Typography names about ten type roles (Headline, Card title, Lede, Label, Value, …), each a complete spec. The markup writes them as loose utilities: 252 elements use 74 distinct combinations of size, weight, line height, letter spacing and font family, 43 of them only once. The Value role (`font-mono text-base font-medium`) is retyped at about 20 sites behind `{{if .Mono}}`. The stylesheets add 64 raw `font-size`, `font-weight` and `line-height` declarations.
- About 150 `verso-*` classes already exist, spread over `input.css`, `forms.css` and `sections.css`. They are split between `@layer components`, `@layer utilities` and unlayered rules, and use three naming habits: single-dash parts, `--` modifiers, and `data-verso-*` hooks.

The project is in beta, and every screen built from here on copies whatever pattern it finds. So the pattern is settled now, applied to everything, and enforced by tests rather than remembered.

## What Changes

- **Type comes from a closed set of roles.**
  - Each role is one `type-<role>` class in `assets/type.css` that sets face, size, line height, weight, letter spacing and numerals together. The set is DESIGN.md's hierarchy plus the roles it describes elsewhere (row label, field label, chip, button, …).
  - A block picks a role and an ink for each of its parts. Nothing outside `type.css` writes a size, weight, line height, letter spacing or font family.
  - The face follows the content: the renderer marks a machine-written string (`data-verso-verbatim`) and a figure (`data-verso-figures`), and each role defines its mono or tabular form, so DESIGN.md's Verbatim Rule holds wherever a value stands.
  - **Visible:** text whose combination drifted off its role moves onto the role. No new role is invented to preserve drift.
- **Drift converges, and every convergence is recorded.** Where two copies of a look (a card's paddings, a chip's weight, a label's line) differ today, they become the block's or the role's one form. Each convergence is listed in `drift.md` in this change, with the elements, the difference and a before/after screenshot, so the owner can review them.
- **The stylesheet has three layers.**
  - Tokens: `palette.css`, `@theme`, and the type roles in `type.css`.
  - Blocks: one file per block under `internal/server/assets/blocks/`, in `@layer components`.
  - Tailwind utilities: in markup, for layout glue and true one-offs. They outrank blocks by layer order.
  - `input.css` keeps only the entry: imports, sources, fonts, `@theme`, `@utility` definitions and base element rules.
  - `forms.css` and `sections.css` dissolve into blocks.
- **One naming scheme.**
  - Block: `verso-<block>`.
  - Element (a named part): `verso-<block>__<element>`.
  - Variant or state: a `data-verso-*` attribute on the block's root.
  - **BREAKING (markup only):** single-dash part names (`verso-chart-line`) and `--` modifier classes (`verso-chart--denim`) are renamed to this scheme.
- **Every look that repeats becomes a block.** That covers compositions (rhythm, stack, cluster, page, drawer), forms, the card, controls, data display, and the shell's chrome and takeover pages. The client scripts that build markup name the same blocks.
- **The renderer says what it renders.** Most visibly, a section the shell renders as a card is a section of kind `card` with `verso-card`. The stylesheet never infers an element's role from its ancestors or children.
- **Blocks talk through custom properties.** A block exposes the measures a container may turn as custom properties with defaults. A rule that reaches into another block may only set those properties.
- **Guards keep it so.** Tests fail on each of these:
  - a block rule that selects from outside its block;
  - a component rule outside `blocks/`;
  - a repeated utility bundle;
  - an element carrying more than eight utilities;
  - an arbitrary variant or `!` utility in markup;
  - a class-string template;
  - a block class used but never defined, or defined but never used;
  - a cross-block rule that sets anything other than a custom property.
  - Violations that exist today sit on a ratchet list that only shrinks, and that list is empty and deleted when this change is done.
- **Geometry parity is the acceptance test.** Every page in the navigation, every drawer kind, the login page and the takeover pages, at 1440px and 390px, keep every box and every computed type property, except the convergences recorded in `drift.md`. The check compares layout boxes before and after each conversion.

## Capabilities

### New Capabilities
- `stylesheet-blocks`: how the shell's stylesheet and markup name and define every look and every piece of text. Covers the three layers, the closed set of type roles and how a value's face follows its content, the naming scheme, where a block's rules live and what they may select, how blocks rank against utilities and adjust one another, what markup may carry, the card as one block across every card surface, and the guards that hold all of it.

### Modified Capabilities
_None._ What `page-cards`, `layout-grid`, `form-fields` and the other UI capabilities require of each look and measure holds unchanged. This change only moves where those looks are defined.

## Impact

- **ADRs:**
  - New ADR-025 "Stylesheet blocks" records the contract and the reasons for it.
  - ADR-005 Decision 6 ("Tailwind v4, over the design palette") is amended in place: utilities are for layout and one-offs, and a look is a block per ADR-025.
- **Code:**
  - `internal/server/assets/`: new `blocks/`. `forms.css` and `sections.css` are removed. `input.css` is reduced to its entry role. `verso.css` is rebuilt.
  - Every template in `internal/widget/templates/` and `internal/server/templates/` names blocks, and the class-string templates in `shared.html.tmpl` and elsewhere are removed.
  - The client scripts that build markup or toggle classes (`verso-tables.js`, `verso-forms.js`, `verso-stream.js`, `verso.js`, `verso-page.js`, `verso-overview.js`, `verso-system.js`, `verso-buttons.js`) name blocks and set state attributes.
  - The renderer (`internal/widget/renderer.go`, `section.go`) marks cards and the other roles the stylesheet used to infer.
- **Tests:**
  - Markup tests that assert utility strings or inferred hooks assert block names instead.
  - New guard tests live in `internal/server/assets_test.go`.
  - New `scripts/layout-snapshot.js` and test-rendered fixtures for pages without a dev route (login, takeovers).
- **Docs:**
  - CONTRIBUTING.md gains a short "Styling" section pointing at ADR-025.
  - DESIGN.md › Typography › Hierarchy names each role by its class, and gains the roles it describes elsewhere (row label, field label, chip, button, column head, editor), each with its full spec.
  - Adding a role goes through DESIGN.md with its reason, as a new widget does.
- **Renderer:** the property, table-cell and chip views that already carry `Mono` and `Duration` flags emit `data-verso-verbatim` and `data-verso-figures` instead of font utilities.
- **Plugins:** none. Plugins send the widget schema, never classes, and the raw bridge's sanitised HTML carries no shell classes. An installed plugin's sections render with the new blocks without a rebuild.
- **Upgrades:** nothing on the router changes. Templates and stylesheet ship together in the one binary.
