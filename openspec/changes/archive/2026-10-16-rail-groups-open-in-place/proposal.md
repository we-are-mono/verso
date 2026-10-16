# Proposal

## Why

Pressing Firewall, Network or System in the rail takes you to that group's first page, so a press meant to look inside a group lands you somewhere you did not choose. After this change pressing a group opens it in place and closes the group that was open, and you choose the page yourself. The rail can only do that if it knows a plugin's subpages on every page, and today it learns them from the page the plugin has just rendered, so a plugin says what its subpages are in its manifest, the place the rail already learns everything else about it.

## What Changes

- A rail row with subpages is a disclosure, never a link: pressing it opens its subpages under it in place, and the one open group closes. One group is open at a time. The group holding the page you are on opens with the page; pressing an open group closes it. The keyboard opens and closes a group as it presses any button, and the rail does this with no script.
- Every group's subpages are in the page from the start, Network's, System's and every plugin's, so opening one never waits on the router.
- A plugin's nav entry declares its subpages in its manifest: `pages` is a list of `{label, path}`, each path relative to the plugin's mount as the envelope's pages are. The manifest is the one source the rail draws a plugin's subpages from, on every page; a subpage path is held to a clean relative path at load.
- An envelope's `pages` is read only for a nav entry that declares none in its manifest (an older plugin's `"pages": true`). Such a row opens as it does now, leading to its page, and its subpages appear under it once you are there.
- Firewall's manifest declares its five subpages: Rules, Port forwards, Zones, Settings and Activity.
- A row with no subpages stays a link, and nothing about the marker, the chevron, what waits at a row's end or the rail's page-change motion changes, save that the chevron now turns as its group opens in place.
- **Contract:** `pages` in a nav entry is a list or `true`. A plugin from another repo that declares a list needs a shell from this release on; the release notes say so. `true` keeps its meaning on every shell.

## Capabilities

### New Capabilities

- `rail`: how the rail's groups open, which group is open, where a group's subpages come from, and how a plugin declares them.

### Modified Capabilities

None. No existing spec states the rail's behaviour; `layout-grid` measures the rail's geometry, which does not change.

## Impact

- **ADR-006 (plugin contract)**: a nav entry's `pages` becomes the subpage list the rail draws from; the envelope's `pages` is the fallback for an entry that declares none. Amended in place.
- **ADR-005 (UI consistency contract)**: none of its rules change; the rail stays the shell's.
- **DESIGN.md**: the Rail and Rows that open entries say a group opens in place, one at a time, and that a plugin declares its subpages in its manifest.
- Shell: `internal/plugin` (`NavEntry.Pages` reads a list or `true`, and validates each subpage path), `internal/server/nav.go` (every group row carries its subpages; a row that has them is a group), `internal/server/templates/nav-row.html.tmpl` (a group is a named `<details>`), the rail's stylesheet for the opening branch and the chevron.
- SDK crate: no change; a manifest is a file a plugin ships, not something the SDK writes.
- Firewall plugin: its manifest's `pages` list. Its pages keep sending the envelope's `pages`, so the shell before this release still shows them.
- `docs/BACKLOG.md`: the plugin repos (wireless, vpn, ddns, qos, dnsdhcp) declare their subpages if they have any.
- No on-device state moves, so there is no migration.
