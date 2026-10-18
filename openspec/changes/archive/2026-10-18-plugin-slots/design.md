# Design

## Context

See proposal.md for why. The current state this design builds on:

- **Places a plugin can reach today.** `nav` gives it pages, `entity_tabs` and
  `entity_acts` give it a device panel's tabs and row acts, and `system_access` gives it
  one block of Access.
- **How Access draws parts.** `renderAccess` (`internal/server/password.go`) fetches
  each part with `pluginBodyAt`, embeds the rendered HTML, routes a post with
  `?plugin=<id>`, and redirects after a staged save or a command. `accessParts` sorts
  the plugin `system` first by name, which ties core to that plugin.
- **The `slot` widget** (`internal/widget/slot.go`) marks a place in a plugin's tree that
  the shell fills with a block of its own. `fillSlots` (`internal/server/appearance.go`)
  knows exactly one: `appearance`.
- **Manifest v2 "contributions at hooks"** is described in `docs/plugins.md` and was
  never built: the `Manifest` struct has no such field, and no hook id is published.
- **Overview cards** are a fixed registry, `overviewCardSpecs`. The layout cookie
  `verso_cards` (`1.<left>.<right>.<closed>.<hidden>`) accepts ids of `a-z` only.
  ADR-022 says a plugin contributes no card.
- **Manifest problems** are logged at discovery (`logProblems`, `cmd/verso/main.go`).
  No page lists them.

## Goals / Non-Goals

**Goals:**
- One mechanism that places every kind of contribution, so a new place on a page never
  needs a new manifest key or shell code that names a plugin.
- Plugins extend each other's pages without knowing each other's ids.
- Every contribution is checked against the closed widget vocabulary and contained if
  it fails.
- Router cost grows only with the contributions on the page being drawn.

**Non-Goals:**
- **acme's root side** (`verso-rpcd/src/acme.rs`, `hand_over`, the `acmeState` broker).
  The privileged registry and events change removes it.
- **Folding `nav`, `entity_tabs` and `entity_acts` into `contributes`.** Each is
  already a slot with a shape of its own: a rail row with subpages, a tab with roster
  summaries, a posting act with `engaged`. Folding them would move every first-party
  manifest and gain no new ability.
- **Columns in core tables.** A column needs a value for every row from a plugin that
  does not own the rows, which means a per-row fetch or a bulk read contract. No plugin
  needs one yet.
- **Settings rows inside another plugin's form.** One form posting to two plugins has
  no atomic outcome. A contributor brings its own block, which has its own Save.
- **Search, and entity kinds a plugin defines.** The search filters rows already on the
  page, so it sees contributed tables with no change. A new entity kind is an entity
  panel contract change, not a slot.

## Decisions

### 1. One primitive: a named slot that anyone opens and anyone fills

A slot is a named place in a page's tree. A shell page opens slots in its own
templates. A plugin opens them in its widget tree. A manifest fills them through
`contributes`:

```json
"contributes": [
  { "id": "source",  "slot": "system.certificate.source",  "path": "/slots/source" },
  { "id": "use",     "slot": "system.certificate.acts",    "path": "/slots/use" },
  { "id": "replace", "slot": "system.certificate.notices", "path": "/slots/replace" },
  { "id": "card",    "slot": "shell.overview.cards",       "path": "/slots/card",
    "position": "after:system" }
]
```

**Alternatives:**
- **Hooks at every template seam, with one outer form** (the documented v2). Rejected
  for two reasons. Hooks only exist on shell pages, so plugins could not extend each
  other. And one form across plugins has no atomic outcome when one plugin refuses.
  Staging (ADR-010) is already the coordination point.
- **A manifest key per place** (`system_access`, `overview_card`, …). Rejected: every
  new place grows the shell and the manifest.

### 2. Names say who opened the slot

- **Shell slots** are `shell.<page>.<place>`.
- **Plugin slots** are `<opener id>.<id>`. The shell prefixes the opener's id when it
  reads the tree, so a plugin cannot open a slot in another plugin's name. Plugin ids
  already cannot contain `.`, so the first `.` always ends the owner.
- The id `shell` is reserved.
- A contribution's own id is `<plugin id>.<contribution id>`. That is its anchor, and
  it is its Overview layout id.

**Names are public API.** The shell's slots live in one catalogue,
`plugins/verso-plugin-sdk/testdata/slots.json`, listing each name, kind and anchor
list. It follows the existing fixture convention (`subject-kinds.json`):
- the SDK's conformance test writes it from the slot constants the SDK exports;
- the shell's test reads it and fails if a page publishes a slot that the catalogue
  does not hold, or the reverse.

A rename keeps the old name as an `alias` in the catalogue, and the shell resolves it.
A name is never reused for something else. Plugin slots are each plugin's own API,
documented in its README.

**Alternative:** flat, unprefixed names. Rejected: two plugins could squat one name.

### 3. Five kinds, checked by the shell

| kind | the answer is | draws as |
|---|---|---|
| `block` | what a page holds: a `section`, `card`, `table`, `form`, a `stack` of them, a `collection` | a block of the page's stack |
| `act` | one `link` with an `icon` (`panel: true` opens the contributor's page in a drawer) | an act after the host's own in its act row |
| `fact` | one `properties` | its rows, appended to the host's facts list |
| `notice` | one `callout` | a notice where the slot stands |
| `overview-card` | the envelope's `card` (title, icon, tone, status, summary, cause, opens) plus a `widget` body with no form | a shell Overview card |

- **How the kind is known.** A plugin-opened slot declares its kind on the widget:
  `{"type":"slot","id":"certificate.acts","kind":"act"}`. A facts slot rides the list
  itself: `properties.slot`, so the contributed rows join that list's column. A slot id
  the shell owns (`appearance`) still draws the shell's block, which keeps every
  existing page unchanged.
- **Checking.** The shell decodes each answer through `internal/widget` and checks it
  against the kind before rendering. An answer of the wrong kind is dropped and logged.
- **Why typed.** Kinds keep the closed vocabulary (DESIGN.md: widget vocabulary,
  additions need approval). The host decides what can stand in its card: acme can add a
  row to the system card's facts, but not a form.
- **No new widget.** Only fields are added: `kind` on `slot`, and `slot` on
  `properties`.

### 4. Positions, anchors, order

- A position is `first`, `last` (the default), `before:<anchor>` or `after:<anchor>`.
- **Anchors.** A shell slot's anchors are its catalogue list. A plugin slot's anchors
  are the contributions in it, because an opener's own items are not addressable.
- **Order.** Contributions resolve in one pass. Each group at one place is sorted by
  `order` (default 100), then plugin id, then contribution id. An anchor that is
  missing, or that forms a cycle, sends its contribution to `last`, and that is logged.
- **No exclusive slots.** The one case that looked exclusive is "the active certificate
  source", but that is a fact on the router (one certificate is served), not a page
  layout. A slot that hid all but one contributor would say nothing about the others.
  The certificate's single owner is settled root-side, in the registry and events
  change.

### 5. Fetching

- **What is fetched.** When the shell draws a page, it first collects the page's slots,
  shell and plugin alike. Only then does it fetch their contributions, in parallel,
  each within the transport's timeout. Every contribution to a slot that is not on the
  page is skipped. A drawer is its own request, so its slots are fetched only when it
  opens.
- **Request.** A GET to the contribution's `path`, carrying:
  - the same brokered reads, language and `X-Verso-Plugins` as a page;
  - `X-Verso-Slot: <slot name>`;
  - `X-Verso-Features`.
- **Answer.** An ordinary envelope. `204 No Content` means nothing to show at this
  moment.
- **Two levels.** The page's own slots are filled, and so are the slots of a
  contribution standing in them, because a plugin's part of a shell page (the system
  plugin's Access part) is itself a contribution whose card opens slots. Slots in an
  answer standing in those are not filled. This bounds the fan-out of one page and
  rules out loops (a fills b fills a). The drawer a contributed act opens is the
  contributor's own page, so it starts again at the first level.
- **No cache.** The Overview's plugin cards are fetched once per render and never by the
  stream (ADR-004's stream stays plugin-free). A cache can be added behind the fetcher
  without a contract change if the Overview's render cost ever needs one.

### 6. Posting goes to the contributor alone

- A contributed form posts to its host page with `?contribution=<plugin>.<id>`. This
  generalizes Access's `?plugin=<id>`, which stays as an alias for a legacy
  `system_access` part.
- The shell passes the post only to that contributor, behind its write gate, and reads
  every other contribution as a GET. The redirect-after-stage/command and preview rules
  are lifted from `renderAccess` into the shared contribution renderer, so every host
  page gets them.
- Acts never post. A contributed act is a link, and the drawer it opens is the
  contributor's page, posting there as any page does.

### 7. Overview plugin cards

- **Drawing.** `overviewCardSpecs` stays the shell's registry. The page lays shell cards
  and contributed cards out together.
- **Header.** A contributed card's header is built from its answer's `card` fields
  through the same `cardHead`.
- **Body.** The body renders through the plugin path, with the contributor's catalogue.
- **Causes.** `causes()` appends each contributed card's `cause`. Hidden cards are
  fetched too, as ADR-022 requires: every card counts.
- **Cookie v2.** The cookie is `2.` followed by base64url JSON:
  `{"cols":[[…],[…]],"closed":[…],"hidden":[…]}`, under a 3 KB cap. Ids are a shell
  card id or `<plugin>.<id>`.
  - **Why JSON.** Plugin ids may hold `-`, which v1 uses as its separator.
  - **Reading.** A v1 value is still read as today (and `verso_overview` before it), so
    no browser loses its layout.
  - **Writing.** The page writes only v2.
- **A plugin that is not answering.** The shell lists the layout ids of installed
  plugins' cards that did not draw in a `data-` attribute. The page's script writes them
  back in their places, so a restarting plugin keeps its place. Ids of plugins that are
  not installed are dropped, which is how removal leaves no trace.
- **Default layout.** A contributed card's `position` (an anchor among the card ids) is
  used only when the cookie does not name it. A card the cookie does not name lands
  where its position says in the default layout, and at the end of the shorter column
  in an arranged one (ADR-022's rule).
- **Amends ADR-022:** "a plugin contributes no card" is replaced by this contract.

### 8. Telling plugins what the shell speaks

`X-Verso-Features: slots`, comma-separated, on every plugin request, built from one
list in `internal/plugin`.

**Why a header and not `schema_version`.** `schema_version` stays the widget
vocabulary's gate (ADR-006 §6). Contract features change on another axis, and a
static manifest cannot negotiate anyway. A plugin's page can read the header and
decide, for example to keep its own fallback act when `slots` is absent.

### 9. `system_access` becomes an alias

- **Reading.** The manifest reader turns `system_access: "<path>"` into
  `{id: "access", slot: "shell.access.blocks", path, position: "before:web"}`. If the
  manifest also contributes to `shell.access.blocks`, the alias is dropped.
- **No more name check.** `accessParts` and its `system`-first rule go.
- **Ordering.** The first-party system plugin contributes `before:web` with `order: 10`.
  Its SSH block and HTTPS certificate card still lead the plugin blocks.
- **Mixed versions.** A shell with slots and an old system plugin still works: both
  legacy parts sort by id. The release notes ask for the three packages to be upgraded
  together.
- **Load check.** The validation that a manifest declares at least one of `nav`, entity
  contributions or Access part becomes "at least one of `nav`, `entity_tabs`,
  `entity_acts`, `contributes`", with the alias counted as a contribution.

### 10. The one certificate card

**verso-plugin-system's card** opens three slots:
- `certificate.source`, a facts slot on its facts list;
- `certificate.acts`, in its act row after Install, Make and Download;
- `certificate.notices`, at the top of the Install and Make drawers.

It drops `trusted_on`, the `uci acme` read in its manifest, and `acme` in its rpcd ACL.

**verso-plugin-acme:**
- **Contributions** (each answers 204 while the `verso` section is disabled, except the
  act):
  - **Source row.** "Your domain" with the domain and the last outcome. It carries the
    waiting mark while a request runs, and is danger-marked after a failure.
  - **Act.** "Use your domain", which opens `/setup` in a drawer.
  - **Notice** in Install and Make: installing turns the trusted certificate off.
  - **Certificate Overview card.**
- **Its Access part becomes `/setup`**, the drawer. That page holds the form, the
  expiry, whether the certificate is served, and the LAN-record offer.
- **Its manifest** drops `system_access`.
- **Data stays as it is.** Its state still comes through the brokered `acmeState`.

**Source row only for a source.** With no source plugin serving, the card's existing
"Signed by" row already says who signed the certificate: "The router itself", or an
issuer. A Source row of the system plugin's own would repeat it. DESIGN.md records the
card's anatomy.

## Risks / Trade-offs

- [Slot names freeze page structure] → Only names listed in the catalogue are
  promised. Aliases cover renames, and a plugin's slots are promised only by their
  opener.
- [Per-render fetches cost router CPU on busy pages] → Fetches happen only for slots on
  the drawn page, in parallel and time-bounded. The Overview stream never dials. A cache
  can come behind the fetcher if measurements show the need.
- [A slow contributor slows the page to the transport timeout] → It is contained to its
  place. The timeout is the existing one, and its unavailable state is the existing
  one.
- [Plugin-to-plugin slots make a hidden dependency] → An opener that drops a slot loses
  only the contributions to it, and the rest of its page draws. The contributor sees no
  error, so its README states the slots it fills, and the conformance fixtures cover
  first-party pairs.
- [Cookie v2 written while an older shell is still served after a downgrade] → The
  older shell cannot parse v2, so it falls back to the default layout without an error
  (the existing rule). The layout is lost on downgrade, not corrupted.
- [Mixed versions during an upgrade] → The `system_access` alias keeps old plugins
  working. A new acme on an old shell loses its contributions, because the old shell
  ignores `contributes` and refuses a manifest with nothing else. The release notes
  name the three packages to upgrade together.

## Migration Plan

1. The shell lands slots, the catalogue, the features header, the `system_access` alias
   and Overview plugin cards. Old plugins render unchanged.
2. verso-plugin-system opens the three certificate slots, moves to `contributes`, and
   drops the acme reads.
3. verso-plugin-acme moves to `contributes` and serves `/setup` and its four
   contributions.
4. One release ships the three packages together, and its notes say so. No router state
   moves: no UCI option, file or init flag changes. Browsers keep their layouts through
   the v1 reader.

Rollback is a downgrade of the three packages. A browser's v2 layout cookie then falls
back to the default layout.
