# Tasks

## 1. The manifest declares subpages

- [ ] 1.1 Test first in `internal/plugin/discovery_test.go`: a nav entry's `pages` parses as `true`, `false`, absent, or a list of `{label, path}`; a subpage with no label, or with a path that is absolute or holds `..`, `?`, `#` or `\`, or differs from its cleaned form, rejects the manifest, and the error names the plugin and the entry. Then the `NavPages` type with its own `UnmarshalJSON`, `Opens()` and `List()`, and one clean-relative-path helper shared with the `system_access` check. Verify: `go test ./internal/plugin/`.
- [ ] 1.2 Rewrite the `NavEntry` doc comment and ADR-006's nav and `pages` paragraphs to state what is: the manifest's list is the rail's source, and the envelope's `pages` is read for an entry that declares none. Verify: `make lint`.

## 2. Every group carries its subpages

- [ ] 2.1 Test first in `internal/server/nav_test.go`:
  - Network, System and a plugin declaring a list carry their subpages on every page.
  - Only the group holding the current page is `Open`.
  - A plugin's declared labels come from its catalog, and their hrefs sit under its mount.
  - A plugin with `pages: true` keeps its envelope's subpages on its own page only, and its row is not a group.
  - The manifest's list wins over a disagreeing envelope.

  `TestBuildSidebarOpensOnlyTheActiveRow` becomes the `Open` check. Then `buildSidebar` attaches `Children` to every group, adds `navRow.Open` and `Group()`, and shares href building with `subpageTabsAt`. Verify: `go test ./internal/server/ -run 'Sidebar|Rail|Row|Pages'`.

## 3. A group is a disclosure

- [ ] 3.1 Test first in `internal/server/nav_test.go` (rendered page):
  - A group renders as `<details name="verso-nav">` with its row as the `<summary>` and its subpages inside.
  - Only the current page's group carries `open`.
  - A row without subpages, and a `pages: true` row, stays an `<a href>`.
  - The summary carries no `href`, and the active subpage keeps `aria-current="page"` and the marker.

  Then split `nav-row.html.tmpl` into one row-face template rendered by both the link and the summary, and the `<details>` group. Verify: `go test ./internal/server/`.
- [ ] 3.2 The rail's stylesheet in `input.css`:
  - The summary loses the browser's marker.
  - The chevron turns with `details[open]`.
  - The branch unfolds in place through `::details-content` on the rail's easing, at once under reduced motion.
  - Hover and the 4px slide match a link row.

  Rebuild `verso.css`. Verify: `make lint`, `make test`, and in `make dev` on Chrome:
  - Pressing Firewall from the Overview opens its group in place.
  - Pressing System closes Firewall.
  - Enter and Space toggle a focused group.
  - A page change still glides the marker.
- [ ] 3.3 DESIGN.md's Rail and Rows-that-open entries say a group opens in place, one at a time, its chevron turning as it opens, and that a plugin declares its subpages in its manifest. Verify: the entries match what 3.2 shows.

## 4. Firewall declares its subpages

- [ ] 4.1 Test first in the Firewall plugin's tests: its `manifest.json` `pages` list equals `tabs()` in labels, paths and order. Then the manifest declares Rules, Port forwards, Zones, Settings and Activity. Verify: `cargo test` in `plugins/verso-plugin-firewall`, `go test ./internal/plugin/` (the shipped manifests load), and in `make dev`, Firewall's group lists the five from the Overview. `make dev` deploys the manifest.
- [ ] 4.2 `docs/BACKLOG.md`: the plugin repos (wireless, vpn, ddns, qos, dnsdhcp) declare their subpages in their manifests if they have any, needing this shell. Verify: the entry is present and states what is.

## 5. Integration

- [ ] 5.1 Full check: `make lint` and `make test` green; `graphify update .`. In `make dev`:
  - With scripting disabled, groups open and close.
  - In Slovenian, Firewall's subpages are translated.
  - On a phone width, the drawer's groups behave the same.

## Workflow follow-up

- Release notes: a plugin whose manifest lists its subpages needs this shell or later; `"pages": true` works on every shell.
- Archive once the ADR-006 amendment, DESIGN.md and docs/BACKLOG.md reflect what landed.
