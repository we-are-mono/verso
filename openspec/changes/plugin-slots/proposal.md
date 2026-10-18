# Proposal

## Why

Operators see the router's HTTPS certificate described twice on Access: once in "HTTPS
certificate" from the system plugin, once in "Trusted certificate" from
verso-plugin-acme. The two cards compete over replacing it. The cause is the contract:
a plugin can add a whole page, a whole part of Access, a tab on a device, and nothing
else. So acme could only stand beside the system plugin's card, and the system plugin
had to read `uci acme` to warn about it. Plugins need to place things where they
belong (a button and its drawer, a card between cards, an Overview card, a table, a
fact row, a warning in another plugin's drawer) without the shell or the other plugin
knowing who they are.

## What Changes

- **One HTTPS certificate card on Access.** It has a Source row and a "Use your
  domain" act. verso-plugin-acme fills both, and the act opens acme's setup in a
  drawer. The separate Trusted certificate part is gone.
- **Named slots.** Every shell page publishes named places, and a plugin's own page can
  open them too. A plugin's manifest fills them through one `contributes` list.
  - Each place is a list: a contribution stands `first`, `last`, or `before` or `after`
    another item, named by its id.
  - Each place takes one kind: page blocks (cards, sections, tables), acts (with the
    drawer they open), fact rows, notices, or Overview cards. The shell refuses a
    contribution of the wrong kind, so the closed widget vocabulary holds.
- **Plugin cards on the Overview.** A plugin can add an Overview card with the same
  header, status and verdict cause as the shell's own cards. Its cause counts in the
  page's heading, and Arrange mode moves, hides and remembers it.
- **The `slot` widget gains a second use.** A plugin page can now open an empty place
  that other plugins fill. Shell blocks such as `appearance` are drawn as before.
  Properties lists and act rows can hold a slot, so contributed facts and acts join
  the host's own.
- **Feature header.** The shell names the contract features it speaks in a request
  header, so a plugin can tell whether its slots will be filled.
- **`system_access` is read as a contribution to Access.** Installed plugins that use
  it keep working unchanged. First-party plugins move to `contributes`.
- **The documented "manifest v2 contributions at hooks" is replaced.** It was never
  built (no hook was ever published), and `contributes` takes its place in
  `docs/plugins.md`.
- **The Overview layout cookie gains a version that can name plugin cards.** The
  current cookie is still read.
- **verso-plugin-system no longer reads `uci acme`.** The warning in its Install and
  Make drawers becomes a notice that acme contributes there.

Not in this change:
- acme's root-side coupling (`verso-rpcd/src/acme.rs`, `acme::hand_over` in
  `setWebCertificate`/`makeWebCertificate`, the shell's `acmeState` broker). The
  privileged command registry and core events change takes those.
- Columns in core tables, settings rows inside another plugin's form, and moving
  `nav`, `entity_tabs` and `entity_acts` into `contributes`. Design says why.

## Capabilities

### New Capabilities
- `plugin-slots`: how places on pages are named and filled.
  - Slot names and kinds, positions and anchors, and ordering.
  - The manifest's `contributes`, how contributions are fetched, checked, posted to
    and contained.
  - Plugin-opened slots, the feature header, and what an unknown slot or anchor does.
  - How a removed plugin leaves no trace.

### Modified Capabilities
- `overview`: the Overview shows its own cards plus any plugin cards. A plugin card's
  cause counts in the heading. The layout cookie gains a version that names plugin
  cards. An installed plugin whose card is temporarily unavailable keeps its place;
  an uninstalled one leaves the layout.
- `trusted-certificate`: the Trusted certificate part becomes acme's Source row, its
  "Use your domain" act and the drawer that act opens, all on the one HTTPS
  certificate card. The warning in Install and Make drawers comes from acme as a
  contributed notice.

## Impact

- **ADRs:**
  - New ADR-026 (slots: one way for plugins to place things).
  - Amends ADR-006: the manifest gains `contributes`, `system_access` becomes its
    alias, and the never-built hooks are struck.
  - Amends ADR-005: the slot widget's two uses.
  - Amends ADR-022: plugin Overview cards, replacing "a plugin contributes no card".
- **Shell (Go):**
  - `internal/plugin/manifest.go`: the `Contributes` field, its validation, and the
    reserved id `shell`.
  - A slot registry and filler that generalizes `fillSlots` (`internal/server/appearance.go`).
  - Contribution fetch, kind check, posting and containment, generalized from
    `renderAccess`/`accessParts` (`internal/server/password.go`).
  - Overview plugin cards (`internal/widget/overview_cards.go`,
    `overview_status.go`, `overview_layout.go`) and the page script that writes the
    layout cookie.
  - The `X-Verso-Features` header (`internal/plugin/transport.go`).
- **SDK (`plugins/verso-plugin-sdk`):** slot positions on properties and stacks, the
  Overview card envelope, and conformance fixtures plus a published `slots.json`
  catalogue of shell slot names.
- **Plugins:**
  - verso-plugin-system opens `certificate.source`, `certificate.acts` and
    `certificate.notices` and stops reading `uci acme`.
  - verso-plugin-acme fills them, moves its form into a drawer and adds a Certificate
    Overview card.
  - Both move from `system_access` to `contributes`.
- **Compatibility:** an installed plugin that uses `system_access` keeps working. A
  plugin that uses `contributes` needs a shell that speaks slots. The release notes say
  to upgrade verso, verso-plugin-system and verso-plugin-acme together.
- **Docs:** `docs/plugins.md` (manifest, slots, the slot catalogue), DESIGN.md (the one
  certificate card), `docs/BACKLOG.md`.
