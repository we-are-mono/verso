# ADR-026 — Slots: one way for plugins to place things on any page

- **Status:** Accepted
- **Date:** 2026-10-18
- **Deciders:** tomaz@zaman.io
- **Relates to:**
  - ADR-005: the slot widget's two uses; the closed vocabulary holds.
  - ADR-006: the manifest gains `contributes`, and `system_access` becomes its alias.
  - ADR-007: a contribution's post passes its plugin's own write gate.
  - ADR-022: plugin cards on the Overview.
  - ADR-017 §6: the trusted certificate fills the HTTPS certificate card.

## Context

A plugin could place four things: whole pages (`nav`), a device panel's tabs and row
acts (`entity_tabs`, `entity_acts`), and one block of Access (`system_access`). It
could place nothing else: no act and drawer in another's card, no card between cards,
no row in another's facts, no Overview card.

The trusted certificate showed the cost. verso-plugin-acme could only stand its own
"Trusted certificate" part beside the system plugin's "HTTPS certificate" card, so
Access described the served certificate twice. The shell named the plugin `system` to
put its part first. verso-plugin-system read `uci acme` to warn in its own drawers.

`docs/plugins.md` described "contributions at hooks": template seams with one outer
page form. No shell ever published a hook.

## Decision

1. **One primitive: a named slot that anyone opens and anyone fills.** A shell page
   opens slots in its own composition, and a plugin's page opens them in its widget
   tree. A manifest fills them through one `contributes` list:
   `{id, slot, path, position, order}`. The shell asks each contribution's `path` when
   it draws the page the slot stands on.

2. **A slot's name says who opened it.**
   - `shell.<page>.<place>` belongs to a shell page.
   - `<plugin id>.<id>` belongs to a plugin's page. The shell adds the opener's id, so
     no plugin opens a slot in another's name.
   - The id `shell` is reserved.
   - A contribution's full id, `<plugin id>.<contribution id>`, is its anchor for
     others and its Overview layout id.

   The shell's slots, kinds and anchors are one catalogue, the SDK's `SHELL_SLOTS`,
   written to `testdata/slots.json` and held to the shell's own list by a test. A name
   in it is a promise: a rename keeps an alias, and a name is never reused.

3. **Five kinds, checked by the shell.**

   | kind | answer |
   |---|---|
   | `block` | what a page holds |
   | `act` | a `link` with an icon, which may open the contributor's page as a drawer |
   | `fact` | a `properties` list, whose rows join the host's list in its column |
   | `notice` | a `callout` |
   | `overview-card` | the shell card's header in the envelope's `card`, and a body with no form |

   An answer of another kind is not drawn, and the shell logs it. The kinds keep the
   closed vocabulary where it matters most, inside another plugin's card: acme can add
   a row to the system plugin's facts, never a form. No new widget is needed. The slot
   widget gains `kind`, and `properties` gains `slot`.

4. **Positions and order.** A contribution stands `first`, `last` (the default),
   `before:<anchor>` or `after:<anchor>`. At one place, contributions sort by `order`
   (default 100), then plugin id, then contribution id. An anchor that is missing or
   loops back stands last, and the shell logs it once a load. There are no exclusive
   slots. The served certificate's one owner is a fact on the router, settled where the
   certificate is installed, not a page layout that would hide all contributors but
   one.

5. **Only what is drawn is asked, all at once.**
   - The shell asks the contributions to the slots on the page or drawer it draws, in
     parallel, each within the plugin timeout. A drawer is its own request.
   - Slots are filled two levels deep: the page's own, and those a contribution
     standing in them opens. A plugin's part of Access is itself a contribution, and
     its certificate card opens slots. Deeper slots are not filled, which bounds the
     fan-out and rules out loops.
   - A request carries `X-Verso-Slot`. A `204` answer is nothing to show and draws
     nothing.
   - A stopped plugin leaves no trace. A block or notice that fails is the shell's
     unavailable state in its place. An act, fact or card that fails is left out and
     logged.

6. **A contribution's form posts to its plugin alone.** It posts to its host page as
   `?contribution=<full id>`. The shell passes the post to that plugin only, behind its
   declared write gate (ADR-007), and asks every other part as on a visit. A save that
   stages, or a command that runs, reads the page again with the outcome as the
   notification. Access still reads `?plugin=<id>` for a part written before slots.

7. **Plugin cards on the Overview** (amends ADR-022).
   - **Drawing.** A card is drawn as the shell's own.
   - **Causes.** Its cause counts in the heading, hidden or not, so each installed
     plugin's card is asked once a draw.
   - **The stream** never asks a plugin. The session keeps the headers the page read.
   - **The layout cookie** is version 2, base64url JSON that names plugin cards by full
     id. Version 1 is still read.
   - **Places.** A card that did not draw keeps its place while its plugin is
     installed.

8. **A drawer's address opens it over its page.** A contributed act opens its
   plugin's page as a drawer over the page it stands on, so that page has no body of
   its own. Visited as a page (an Overview card's door, a bookmark), a page that is only
   an open drawer and names its `back` page lands there as `?drawer=<its address>`, and
   that page opens it by pressing the act already on it. Nothing else can be opened
   this way: the name must match a link the page holds.

9. **The shell names the features it speaks.** `X-Verso-Features: slots` rides every
   plugin request. `schema_version` stays the vocabulary's gate. Features change on
   another axis, and a static manifest cannot negotiate, but a plugin's page can.

10. **`system_access` is an alias.** It reads as a contribution to
   `shell.access.blocks` standing before the web interface's section, so a plugin
   written before slots keeps its part. The shell no longer names `system`: the system
   plugin's part leads by its own `order`.

## Consequences

### Positive
- Any page, card, act row or facts list can take another plugin's piece without the
  shell or the host knowing who fills it.
- Access shows one HTTPS certificate card. acme adds its Source row, its "Use your
  domain" act and drawer, its notice in Install and Make, and its Overview card.
  verso-plugin-system no longer reads the acme config.
- Removing a plugin removes all of it at the next render. Nothing about where its
  pieces stood is kept on the router.

### Costs / negatives
- Every contribution on a page is a request to a plugin each render: router CPU on
  busy pages. Asking only the drawn slots, in parallel and time-bounded, keeps it in
  proportion. A cache can stand behind the fetcher without a contract change.
- Plugin slots are hidden dependencies between plugins. An opener that drops one loses
  only the contributions to it, so each plugin's README names the slots it opens and
  fills.
- A downgrade to a shell before version 2 reads a version 2 layout as the default
  layout: lost, not corrupted.

### Neutral
- `nav`, `entity_tabs` and `entity_acts` keep their keys. Each is a slot with a shape of
  its own (a rail row with subpages, a tab with roster summaries, a posting act with
  `engaged`), and folding them in would move every manifest for no new ability.
- Not placeable through a slot:
  - columns in a core table, which need a per-row value from a plugin that does not
    own the rows;
  - settings rows inside another plugin's form, which would be one form posting to
    two plugins with no single outcome.

## Alternatives considered

- **Hooks at template seams with one outer form** (the never-built design). Rejected:
  only shell pages would have hooks, so plugins could not extend each other, and one
  form across plugins has no single outcome when one refuses. Staging (ADR-010) is
  already where changes come together.
- **A manifest key per place** (`system_access`, `overview_card`, …). Rejected: every
  new place would grow the shell and every manifest.
- **Unprefixed slot names.** Rejected: two plugins could claim one name.
- **One level of filling.** Rejected: a shell page's plugin part is a contribution, and
  its cards must open slots for the certificate card to be one card.
