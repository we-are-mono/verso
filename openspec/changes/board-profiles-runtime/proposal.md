# Proposal

## Why

A board's profile (its port order, its sensor map, its rear-panel art) is compiled into the shell, so supporting a new board means a Verso release. A board vendor should be able to ship support for their board as a package of their own, the way a plugin author ships a plugin. Board support is not a plugin, though: it is data that the Ports card, the System card, the Hardware page and the history collector read on every render and every tick, and none of them may wait on a process that can be stopped or crash.

## What Changes

- The shell reads board profiles from `/usr/share/verso/boards/<folder>/` at startup, instead of from its binary. Choosing a profile is unchanged: its `id` or one of its `aliases` equals the board's `board_name`.
- First-party boards ship inside the `verso` package. Their source moves from `profiles/` to `boards/` in this repository; the Mono Gateway DK keeps its profile on any install of `verso`, with no extra package.
- A board vendor ships `verso-board-<name>`: a package that installs one folder, with no code and no process. Settings that belong to the board (a fan curve, LEDs) are a plugin, `verso-plugin-<name>`, that depends on its board package.
- Panel art (`back.svg`, `front.svg`) loads with its profile, once, and is sanitized before it is used: the allowlist `profiles/README.md` specifies, and every `data-verso-port` checked against the board's interfaces. Art that fails is logged once, naming the cause, and the Ports card and Hardware page draw the generated panel in the profile's port order.
- The Ports card and the Hardware page render the loaded art on every render without reading or sanitizing it again.

## Capabilities

### New Capabilities

- `board-profiles`: where the shell finds board profiles, how one is chosen for the running board, how its panel art is loaded and sanitized, and what every consumer gets when no profile matches or the art is rejected.

### Modified Capabilities

None.

## Impact

- **A new ADR, ADR-023 (board profiles are packaged data)**: board support is a folder the shell reads, not a plugin it asks, and why; the `verso-board-<name>` naming beside ADR-006's `verso-plugin-<name>`.
- **ADR-006 (plugin contract)** §10 names the board package beside the plugin package, so the two are not confused.
- **ADR-022 (Overview cards)**: the Ports card's art comes from the loaded profile.
- `profiles/` → `boards/` (with its `README.md`); `profiles/embed.go` is replaced by a loader over the runtime directory; `sensors.LoadProfile` takes that directory's `fs.FS`, and the five call sites (`hardware.go`, `history.go`, `server.go`, `port_panel.go` through the profile) read the one loaded profile.
- `readPanel` is replaced by the profile's loaded, sanitized art; a new SVG sanitizer in the shell, with its own tests.
- Tests that read the DK profile (`internal/server/port_panel_test.go`, `internal/sensors/sensors_test.go`) read it from `boards/`.
- The `verso` package installs `boards/*` into `/usr/share/verso/boards/`. The folder is shipped content, not on-device state, so the move needs no migration.
