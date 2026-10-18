# Tasks

## 1. Manifest: `contributes` and the alias

- [x] 1.1 Test first in `internal/plugin/manifest_test.go`: `contributes` entries decode. A missing `path`, a repeated id, or an id outside `[a-z0-9-]` skips that entry with a logged reason and keeps the rest of the manifest. Id `shell` refuses the manifest. A contribution-only manifest loads. Then add `Contribution{ID, Slot, Path, Position, Order}` and its validation. Verify with `make test`.
- [x] 1.2 Test first: `system_access` reads as `{id: access, slot: shell.access.blocks, position: before:web}`, and is dropped when the manifest also contributes to `shell.access.blocks`. Then implement it in the manifest reader. Verify with `make test`.
- [x] 1.3 Test first: the position parser accepts `first`, `last`, `before:<anchor>` and `after:<anchor>`, and refuses anything else as a skipped entry. Then implement it. Verify with `make test`.

## 2. The catalogue and the slot widget

- [x] 2.1 Test first in `plugins/verso-plugin-sdk/tests/conformance.rs`: the SDK exports the shell slot constants and writes `testdata/slots.json` (name, kind, anchors). Add `Widget::Slot { id, kind }` and `Properties.slot`, plus fixtures `widget-slot-act.json` and `widget-properties-slot.json`. Verify with `cargo test` in the SDK.
- [x] 2.2 Test first in `internal/widget`: `slot` decodes `kind` (`block`, `act`, `notice`), and `properties` decodes `slot`. Both decode the new SDK fixtures. An unknown kind decodes as a slot that draws nothing. Then implement it. Verify with `make test`.
- [x] 2.3 Document `contributes`, positions, kinds, plugin-opened slots and the catalogue in `docs/plugins.md`. Replace the "Contributing to a shell page" hooks section and the manifest table rows, and rewrite "slot — a block the shell owns" for both uses. Verify that the doc's JSON examples decode, using the manifest test's fixture loader.

## 3. Slot resolution and ordering

- [x] 3.1 Test first, in a new `internal/server/slots_test.go`, collecting slots from a tree:
  - a plugin slot is named `<opener>.<id>`;
  - `appearance` still fills with the shell's block;
  - slots inside an embedded contribution are not collected; the fetcher (4.1) fills a contribution's own slots, two levels deep.
  Then implement the collector, generalizing `fillSlots`. Verify with `make test`.
- [x] 3.2 Test first, ordering:
  - order is position, then `order`, then plugin id, then contribution id;
  - an anchor to another contribution resolves;
  - a missing anchor or a cycle lands at `last` and is logged once per load;
  - a shell slot missing from the catalogue, or a slot whose opener is not installed, is logged once per load.
  Then implement it as a single-responsibility orderer, with no Server dependency. Verify with `make test`.

## 4. Fetching, kind checks, containment and posting

- [x] 4.1 Test first with the fake transport:
  - only slots on the drawn tree are fetched;
  - fetches run in parallel (three 200 ms fakes finish in under 400 ms);
  - each request carries `X-Verso-Slot`, `X-Verso-Features: slots`, `X-Verso-Plugins` and the brokered reads;
  - 204 draws nothing and logs nothing;
  - a stopped plugin leaves no placeholder;
  - a hang or a bad answer draws the contained unavailable state;
  - the page answers 200.
  Then implement the contribution fetcher behind the `Transport` seam. Verify with `make test`.
- [x] 4.2 Test first: kind checks. A `section` in an act slot is dropped and logged. A `link` with an icon fills an act slot after the host's acts. `properties` rows join the host list's rows. A `callout` fills a notice slot. A `section`, `card` or `table` fills a block slot. Then implement the checker as one function per kind behind one interface. Verify with `make test`.
- [x] 4.3 Test first: posting. `?contribution=<plugin>.<id>` reaches only that plugin, behind its write gate (refused without dialing when the grant is missing). Other contributions are read as GETs. A staged save or a command redirects with its notice. A preview answers with the fragment alone. `?plugin=<id>` still reaches a legacy Access part. Then lift this logic out of `renderAccess` into the shared contribution renderer. Verify with `make test`.
- [x] 4.4 Test first: the reader's language reaches each contribution, and its words are translated with the contributor's catalogue. Then wire the language in. Verify with `make test`.

## 5. Shell pages publish their slots

- [x] 5.1 Test first: Access publishes `shell.access.blocks` with anchors `password`, `sessions`, `web` and `luci`, matching `slots.json`. `accessParts` and its `system`-first rule are gone. A legacy `system_access` part stands before `web`. Then rebuild `renderAccess` on the slot renderer. Verify with `make test`.
- [ ] 5.2 Test first: every other shell-owned page publishes `shell.<page>.blocks`, with its sections as anchors, matching `slots.json`. The catalogue test fails on any mismatch. Then add the slots to each page. Verify with `make test`.
- [x] 5.3 Test first: a contribution-only plugin with no `nav` loads and fills a shell slot. The manifest load check counts `contributes`. Verify with `make test` and `make lint`.

## 6. Overview plugin cards

- [x] 6.1 Test first in `internal/widget/overview_test.go`: a contributed card's `card` fields build its header (title, icon, tone, status, summary), its closed summary, its Go link to the contributor's page, and Arrange's grip and hide. A body holding a form draws the contained unavailable state. Then implement it. Verify with `make test`.
- [x] 6.2 Test first: a contributed card's cause joins `causes()`, whether the card is hidden, closed or open. One plugin cause alone becomes the verdict in its own words. Then implement it. Verify with `make test`.
- [x] 6.3 Test first: the default layout places a contributed card by its position (`after:system`). The stream never dials a plugin. Each render fetches each card once. Then implement it. Verify with `make test`.
- [x] 6.4 Test first in `internal/widget/overview_layout_test.go`: cookie v2 (`2.` plus base64url JSON, 3 KB cap) parses. v1 and `verso_overview` still parse. A garbled value gives the default layout. An installed plugin's card id that did not draw is kept and emitted for the page to write back. An uninstalled plugin's card id is dropped. Then implement it, and change the page's script to write v2 with the kept ids. Verify with `make test`, plus a Playwright check that moving a plugin card survives a reload.
- [x] 6.5 Amend ADR-022 in place: plugin cards, cookie v2 and the kept ids replace "a plugin contributes no card". Update DESIGN.md's Overview section for plugin cards. Verify that the ADR and DESIGN.md name the same cookie format as the code.

## 7. Features header and ADRs

- [x] 7.1 Test first in `internal/plugin/transport_test.go`: every plugin request carries `X-Verso-Features` from the one features list. Then implement it. Verify with `make test`.
- [x] 7.2 Write ADR-026, the slots decision and its reasons, from design.md decisions 1–9. Amend ADR-006 in place: the manifest gains `contributes`, `system_access` becomes an alias, the hooks section is struck, and `X-Verso-Features` is added. Amend ADR-005 in place for the slot widget's two uses. Verify that every ADR cross-reference resolves (`grep -n 'ADR-026' docs/ADR/*.md`).

## 8. verso-plugin-system opens the certificate slots

- [x] 8.1 Test first in `plugins/verso-plugin-system/src/tests.rs`: the HTTPS certificate card's facts carry `slot: certificate.source`. Its act row holds `{slot certificate.acts, kind act}` after Install, Make and Download. The Install and Make drawers open `{slot certificate.notices, kind notice}` at their top. Then implement it. Verify with `cargo test`.
- [x] 8.2 Test first: no read of `uci acme` and no `TURNS_OFF` sentence remain. Then remove `trusted_on`, the manifest's `acme` read and `acme` in the rpcd ACL file. The manifest moves `system_access` to `contributes` (`before:web`, `order: 10`). Verify with `cargo test`, and check that `grep -rn acme plugins/verso-plugin-system` finds nothing.
- [x] 8.3 Document the plugin's three slots in its README. Verify that the README's slot names match the test fixtures.

## 9. verso-plugin-acme fills them

- [x] 9.1 Test first in `plugins/verso-plugin-acme`: `/slots/source` answers a properties list with one Source row (the domain and the last outcome). The row carries the waiting mark while a request runs and the danger mark after a failure. It answers 204 while the `verso` section is disabled. Then implement it. Verify with `cargo test`.
- [x] 9.2 Test first: `/slots/use` answers one "Use your domain" link to `/setup` with `panel: true`. `/slots/replace` answers the turn-off callout while the section is enabled, and 204 otherwise. Then implement them. Verify with `cargo test`.
- [x] 9.3 Test first: `/setup` serves the former Access part as a drawer page: the form, expiry and served state, and the LAN-record offer. Every trusted-certificate drawer scenario passes. Saving stages as before. Then move `page.rs` to it. Verify with `cargo test`.
- [x] 9.4 Test first: `/slots/card` answers the Certificate Overview card:
  - its summary reads the domain, and its body gives the time left as a duration;
  - a failed renewal is its cause, in warning tone, turning danger at seven days or fewer;
  - it answers 204 while the section is disabled.
  Then implement it. Verify with `cargo test`.
- [x] 9.5 The manifest declares the four contributions and drops `system_access`. Document the slots acme fills in its README. Verify with the shell's manifest test against acme's manifest.

## 10. Integration on the lab and the DK

- [x] 10.1 Run `make dev` with system and acme deployed. Check with Playwright that:
  - Access shows one HTTPS certificate card with the Source row and "Use your domain";
  - the drawer saves and stages;
  - Install shows acme's notice;
  - stopping acme removes its row and act with no placeholder;
  - the Overview shows the Certificate card, and Arrange keeps its place across an acme restart.
  Save screenshots to ~/Share/screenshots.
- [ ] 10.2 On the DK (router.example.com in every public artifact), check that the served ACME certificate shows as its Source row and that its Overview card reads its expiry. Removing verso-plugin-acme on Packages must leave no trace on Access or the Overview.
- [x] 10.3 Update `docs/BACKLOG.md`: the certificate-source entry's slot half is done. Its root half (the `acme.rs` move, `hand_over`, the `acmeState` broker) stays, under the registry and events entry. Write release notes asking to upgrade verso, verso-plugin-system and verso-plugin-acme together. Verify that `make lint` and `make test` are green.

## Workflow follow-up

- Archive the change once the DK checks pass, after confirming ADR-026, the ADR-005, ADR-006 and ADR-022 amendments, and docs/BACKLOG.md reflect what landed.
