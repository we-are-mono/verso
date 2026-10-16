# Design

## Context

See proposal.md for why. Today the rail (`internal/server/nav.go`, `templates/nav-row.html.tmpl`) is a list of links. A row that opens into subpages links to its first page, and only the active row carries its subpages (`navRow.Children`). Network and System are the shell's collapsed sections (`sections.go`): their pages are computed on every request (`sectionPages`), but they are only attached to the active row. A plugin row knows only that it opens (`NavEntry.Pages bool`); which subpages it has comes from the envelope of the page just rendered (`gateway.go`, `subpageTabsAt`). Only Firewall declares `"pages": true`, and Firewall ships in the same `verso` apk as the shell.

The rail's page-change motion is a cross-document view transition. The row, the branch, each subpage and the marker carry `view-transition-name`s derived from their hrefs (`railTransition`).

## Goals / Non-Goals

**Goals:**
- Every group's subpages are in the page on every request, from one source per group.
- Open and close work with no script, and the browser enforces one open group.
- The rail's look, marker, chevron and page-change motion are unchanged.

**Non-Goals:**
- Remembering which group was open across page loads: a page opens its own group (spec: the current page's group opens with the page).
- Subpages that change at runtime. A manifest's list is fixed per plugin version.
- Changing the envelope's `pages` for anything but the rail's fallback.

## Decisions

### A group is a named `<details>`

Each row with subpages renders as `<details name="verso-nav">` with the row as its `<summary>` and the branch inside. The `open` attribute is set on the group that holds the current page. A shared `name` makes the browser close the other group when one opens (Chrome 120, Safari 17.2, Firefox 130). In an older browser several groups can be open at once, which is a harmless degradation. `<summary>` is focusable, toggles on Enter and Space, and exposes its expanded state, so the keyboard and accessibility requirements cost nothing.

*Alternatives:* An Alpine accordion would need script for a menu, CSP-safe getters for every row's state and its own ARIA wiring, all of which the platform already does. A `<button aria-expanded>` with a small script has the same cost without the no-script behaviour.

### The row's look is shared between a link and a summary

The row's inner markup (icon, label, what waits, chevron) moves into one template that both the `<a>` row and the `<summary>` row render, so the two cannot drift. The summary drops the browser's disclosure marker (`list-style: none`, `::-webkit-details-marker`). The chevron turns with the group's state (`details[open] > summary`), not only on page change. It keeps its view-transition name, so a page change still turns it on the rail's easing.

### Opening in place animates where the browser can

The branch unfolds on the rail's easing (360ms, `cubic-bezier(0.22, 1, 0.36, 1)`) through `::details-content` with `interpolate-size: allow-keywords` and `content-visibility` as a discrete transition. That works in Chromium 131 and later; elsewhere, and under reduced motion, the branch appears at once. The page-change transition between pages is untouched. Because a closed group's content is not rendered, the view-transition names inside it are not captured, so keeping every branch in the page adds no duplicate names.

### The nav model carries every group's subpages

`buildSidebar` attaches `Children` to every row that has them, not only to the active one:
- Network and System: `sectionPages(section, active)`, as today.
- A plugin entry declaring a list: that list, built into hrefs under the plugin's mount (one function shared with `subpageTabsAt`), labels through the plugin's catalog, the active subpage marked by `bestHref`.
- A plugin entry with `pages: true` and no list: the envelope's pages, only on its own page (today's behaviour), and its row stays a link.

`navRow` gains `Open bool` (the group holds the current page) and a `Group()` method (has children and is not a fallback link). The template branches on `Group`. The active subpage's marker logic is unchanged.

### `NavEntry.Pages` reads a list or `true`

`Pages` becomes a type (`NavPages`) with its own `UnmarshalJSON` that accepts `true`, `false` or an array of `{label, path}` (the envelope's `PageTab` shape). It reports `Opens()` and `List()`. `validate()` rejects a subpage with no label, or a path that is not clean and relative: absolute, containing `..`, `?`, `#` or `\`, or different from its cleaned form. Those are the same rules the manifest already applies to `system_access`; they move into one helper both use.

*Alternative:* A new field (`subpages`) beside `pages: true` would let one manifest work on old and new shells. It was set aside because it makes the contract say the same thing twice. Firewall upgrades with the shell, and no plugin from another repo declares subpages today.

### The manifest wins over the envelope

When an entry declares a list, the envelope's `pages` is not read for the rail. Firewall's pages keep sending `with_pages(tabs())`, so a shell from before this release still shows its subpages. The duplication lives in the plugin's code, not in the contract.

## Risks / Trade-offs

- [A plugin's manifest and its pages disagree] → The rail shows the manifest's list (spec). A Firewall test holds its manifest list equal to `tabs()`, so the two in this repo cannot drift.
- [A third-party manifest with a list on an older shell fails to parse, and that shell rejects the plugin] → The release notes name the minimum shell. `true` keeps working everywhere.
- [Every group's subpages in every page add a little HTML] → A few dozen links; LAN bandwidth is free, and router CPU is unchanged because the pages were already computed or are static.
- [Firefox before 130 does not close the other group] → Degrades to several open groups; nothing breaks.

## Migration Plan

No on-device state moves. The shell and Firewall upgrade together in the `verso` apk, and roll back together by installing the previous package. An older shell cannot parse a list in `pages`, so the two must never be split, and the shared apk already guarantees that.
