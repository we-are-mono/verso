# ADR-025 — Stylesheet blocks: every look named once, every text a role

- **Status:** Accepted
- **Date:** 2026-10-17
- **Deciders:** tomaz@zaman.io
- **Relates to:** ADR-005 §6 (Tailwind v4 over the palette, amended to point
  here), ADR-024 (themes are colours over the same tokens), DESIGN.md
  (Typography, the visual contract every block and role draws).

## Context

The shell draws its pages from server templates styled with Tailwind
utilities (ADR-005 §6). A look the shell draws in many places (a card, a
chip, a field row, a card title) was written out as a run of utilities in
each place that draws it, so fixing one copy left the others as they were,
and copies drifted apart: the Overview's card and a page's card drew the same
frame from two definitions, and 252 elements wrote 74 distinct combinations of
type for the ten roles DESIGN.md names. Where a rule had to treat an element
by what it was (a card, the first visible row of a card), the stylesheet
worked it out from the element's ancestors and children, in selectors that
ran to hundreds of characters, though the shell had known the answer when it
rendered the element.

## Decision

**Every look the shell draws is a named block, defined once; every piece of
text takes its type from a closed set of roles; the shell writes into the
markup what an element is, and the stylesheet reads only that. Tests hold all
of it.**

1. **Three layers.** Tokens: the palette (`palette.css`), the theme, and the
   type roles (`type.css`). Blocks: one file per block under
   `internal/server/assets/blocks/`, in the components layer. Tailwind
   utilities: in markup, for layout glue and true one-offs. A utility outranks
   every block by layer order, so a one-off never needs a stronger selector.
   The entry stylesheet holds only imports, sources, font faces, the theme,
   utility definitions and rules on bare elements; no rule outside a block
   file or `type.css` selects a `verso-*` class or a `data-verso-*`
   attribute.

2. **Naming.** A block's root is `verso-<block>`; a named part is
   `verso-<block>__<element>`; a variant or a state is a `data-verso-*`
   attribute on the root, never a class, because the client already toggles
   attributes, a CSP-safe Alpine binding sets an attribute from a getter, and
   a variant with a value (a tone, the columns a table narrows) has no clean
   class form. Two blocks may share an element when one adds what only it has
   (`verso-card verso-overview-card`). The `verso-` prefix keeps every name
   clear of Tailwind's own (`table`, `grid`, `hidden`).

3. **A block's rules start from the block.** Every selector in
   `blocks/<block>.css` starts at that block's root or one of its elements; it
   may reach inside, never start from an ancestor. A block that adjusts
   another (a card bleeding its rows to its borders, handing its edge to the
   last row) sets only the custom properties the other block exposes; the
   exposing block reads each with its default. These properties are
   registered with `@property … inherits: false`, so a value set on one
   element never reaches a block nested inside it, and where an edge is handed
   down through containers each container forwards it explicitly. Each block
   file opens with its elements, variants, the properties it reads and the
   properties it sets on others.

4. **The shell names roles; the stylesheet does not infer them.** Where a rule
   treats an element by its role (a section rendered as a card, a part, a
   machine-written value), the renderer marks the role as it renders the
   element: a card is a section of kind `card` carrying `verso-card`; a value
   the machine wrote carries `data-verso-verbatim`; a count, duration, rate or
   size carries `data-verso-figures`. Only state that changes in the browser
   (`:checked`, `[open]`, `[aria-expanded]`, hover and focus) is read from the
   document. "First or last visible child" is written
   `:nth-child(1 of :not(input[type="hidden"], [hidden], template))`.

5. **Type comes from roles.** A role is one `@utility type-<role>` in
   `type.css` that sets face, size, line height, weight, letter spacing, case
   and numerals together, and its verbatim (mono) and figures (tabular) forms,
   so the Verbatim Rule holds wherever a value stands. A role carries no ink;
   the block that applies it sets the colour. DESIGN.md › Typography lists
   every role by its class with its full spec; adding a role is a DESIGN.md
   change with its reason, as adding a widget is. Nothing outside `type.css`
   writes a size, weight, line height, letter spacing or family, as a utility
   or a declaration.

6. **`@apply` where it is shorter.** A block is plain CSS over tokens; it
   applies its type roles with `@apply type-<role>`, and uses `@apply` for a
   bundle of variants (`hover:`, `focus-visible:`) where that is shorter than
   the rules it stands for.

7. **Guards.** Tests in `internal/server/stylesheet_guards_test.go` read the stylesheets,
   templates, client scripts and the Go that writes classes, and fail on:
   - a rule outside a block file selecting a block's class or attribute;
   - a block-file selector starting outside its block;
   - a rule reaching another block that declares anything but a custom
     property;
   - a `verso-*` class in any other form than block or element, a
     view-transition name and a name a published data contract carries (the
     board art's `verso-port-role`, profiles/README.md) aside;
   - the same set of four or more utilities on two elements;
   - an element carrying more than eight utilities;
   - an arbitrary-variant (`[&…]:`) or important (`!`) utility in markup;
   - a template whose whole output is a class list;
   - a `verso-*` class written but never defined, or defined but never
     written;
   - a size, weight, line-height, tracking or family utility in markup, or
     such a declaration outside `type.css`;
   - a role in `type.css` and DESIGN.md's list that disagree.

8. **A change shows what it moved.** `make layout-snapshot` records every box
   and type property of the reference pages (every page the rail reaches, a
   drawer of each kind, the sign-in page, the takeovers) at 1440px and 390px,
   and `make layout-diff` names each element whose style changed and counts
   what moved because of it.

## Consequences

- A fix to a look or a role is one edit that reaches every place it is drawn.
  Copies that had drifted apart become identical when they take the block or
  the role.
- New UI names blocks and roles rather than writing utility runs; a look with
  no block gets one, and a text with no role takes the nearest or goes to
  DESIGN.md as a new one.
- Plugins are untouched: they send the widget schema, never classes
  (ADR-005), so a plugin's sections take the blocks without a rebuild.
- A block that adjusts another depends on the properties it exposes, listed in
  both files' headers; that coupling is visible in one form only.
