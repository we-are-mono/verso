# Design

## Context

After `palette-token-leaks`, every colour the shell paints reads a `palette.css` token, and `internal/palette` parses the embedded `palette.css` at startup. Shadows are whole-shadow custom properties (`--shadow-dialog: 0 8px 32px rgba(…)`) and chart washes are bare strengths (`--chart-wash-from: .16`), neither of which a colour-only theme can reach. The shell takes its own settings as init-script flags (`-alert-url` from `verso.alerts.url`), since it holds no session at startup, and re-reads what lands on disk on SIGHUP (`Server.Rescan`, plugin and certificate reloads). `verso-rpcd` already writes Verso's own config outside any session for the web listeners (`access::web_listeners`: `uci set …`, `uci commit verso` as root, through root's own save directory, so an operator's staged changes, which rpcd keeps per session, are never swept in). System › General is the system plugin's page: one `Widget::Form` holding an untitled section (router name, IPv6 prefix, packet steering) and the Time section; the plugin ships inside the `verso` package.

## Goals / Non-Goals

**Goals:**
- A theme is data: a file of colours a stranger can write and package, validated so it cannot break a page.
- The theme in force reaches every page in its first response, with no flash and no client script.
- The Appearance setting writes at once without touching the operator's stage, and repaints in place.

**Non-Goals:**
- Per-browser or per-account themes: the theme is the router's.
- Themes changing anything but colour: geometry, type and the 8px grid stay the shell's.
- A packaging target for third-party theme packages in this repository; ADR-024 states the package shape.
- Catppuccin's Latte, Frappé and Macchiato flavours.

## Decisions

**1. A theme file is a flat map of palette colour names.**
```json
{ "name": "Mocha", "family": "Catppuccin", "scheme": "dark",
  "colors": { "ground": "#1e1e2e", "ink": "#cdd6f4", "shadow-dialog": "#00000080" } }
```
The id is the file's stem (`[a-z0-9-]+`). The keys a theme may set are exactly the `--color-*` tokens `palette.css` declares, read from it at startup by `internal/palette` (which today returns only the hex-valued ones; it gains the full set of names, including tokens whose default is a `var()` or a `color-mix()`). A value is `#` and 3, 4, 6 or 8 hex digits, nothing else: the value is written into a `<style>` block, and the regex is the injection guard. *Alternative:* accept any CSS colour (`rgb()`, `oklch()`, names). Rejected: every extra form widens what reaches the stylesheet for no colour a hex cannot write.

**2. Everything a theme changes is a colour; shapes stay in the shell.** Each shadow splits into a shape the shell keeps and a colour token: `--shadow-dialog: 0 8px 32px var(--color-shadow-dialog)`, with `--color-shadow-dialog: #1b191729`. The lifted row's two layers take two colours (`shadow-lifted`, `shadow-lifted-near`). Chart washes become two colour tokens per series hue (`chart-denim-from`, `chart-denim-to`, and the same for `amethyst`, `violet`, `emerald`), defaulting to `color-mix(in srgb, var(--color-denim) 16%, transparent)` and so on, so a theme that changes Denim moves its wash with it and a theme that wants a stronger wash sets an 8-digit hex. The chart stops read `stop-color: var(--color-chart-…)`, and the `--chart-wash-*` strengths go. These colour tokens sit in a `:root` block of `palette.css` beside the shapes, outside `@theme`, so Tailwind always emits them; `internal/palette` reads names from both. *Alternatives:* a shell-owned dark shadow set keyed on `scheme` (rejected by the owner: a hidden rule a theme file cannot see); opaque precomputed washes (rejected: they hide the grid rules under the curve and one series' wash under the other's).

**3. Sand is built in; three choices name it.** `sand` (and an unset option) follows the browser; `sand-light` is `palette.css` with no override; `sand-dark` is the shipped `sand-dark.json`. `sand` and `sand-light` are reserved ids, so a file cannot take them. Any other id is a theme file, fixed. If `sand-dark.json` is missing, Sand following the browser draws Sand light in both, logged.

**4. The theme block is rendered by the shell, after the stylesheet.** `internal/theme` turns the choice into CSS once, when the choice or the themes change, never per request:
```
sand           @media (prefers-color-scheme: dark) { :root { color-scheme: dark; --color-…: …; } }
sand-light     (empty)
<fixed theme>  :root { color-scheme: <scheme>; --color-…: …; }
```
It stands in `<style id="verso-theme">` straight after `<style id="verso-css">` in the four templates that inline the stylesheet (`page`, `upgrading`, `restarting`, `firmware-started`; sign-in renders through `page`). An unlayered `:root` rule outranks Tailwind's `@layer theme` variables, so no `!important` is needed. The takeovers' `theme-color` reads the theme's Canvas or Paper; under `sand` they declare two metas with `media="(prefers-color-scheme: …)"`. *Alternative:* a stylesheet route per theme. Rejected: a second request per page, and the inline block is what makes the first paint themed (the LAN budget is CPU, not bytes).

**5. The choice reaches the shell as a flag, then lives in memory.** The init script passes `-theme "$(config_get ui theme)"`, as it passes the alert settings. The server holds the theme set and the choice behind an atomic pointer, swapped by a successful write and by SIGHUP's re-read of the themes directory; the choice itself is not re-read on SIGHUP (procd keeps a changed command line for the next start), so a hand edit of `verso.ui.theme` takes effect on `/etc/init.d/verso restart`, as every other shell setting's does.

**6. The write goes through `verso-rpcd setTheme`, outside the stage.** `setTheme {theme}` checks the id's shape, creates `verso.ui=ui` if missing, sets `verso.ui.theme` (or deletes it for `sand`), and runs `uci commit verso` as root, through root's save directory, so the operator's staged changes, held by rpcd per session, stay staged; it does not reload the service, since the running shell swaps its own state. It is granted in `verso-helper.json` beside `setWebListeners`. The Go backend gains `SetTheme(ctx, sid, id)`; the shell route checks the id against the loaded themes before calling it. *Alternative:* `uci set` + `commit` through the operator's rpcd session. Rejected: a commit takes the whole config, staged alerts or update-check changes with it.

**7. The Appearance card is a slot the system plugin places and the shell fills.** `widget.Slot{ID}` (`{"type":"slot","id":"appearance"}`; `Widget::slot("appearance")` in the SDK) is approved as a new generic widget: a plugin places a shell-owned block where it belongs on its page. The shell keeps the ids (only `appearance`), fills each slot by walking the decoded envelope before rendering, and renders nothing for a slot it has no block for. The system plugin places the slot between its untitled section and Time. The block is a ruled `Section` titled "Appearance" holding one `Field` of kind select, its key chip `theme`, its description the takes-effect sentence: existing widgets. What makes it act on its own is a shell-only property of the select (`json:"-"`, as `Form.Frame` is, so no plugin can set it): it renders `form="verso-theme"` (the HTML attribute that ties a control to another form, so the plugin's form neither submits nor counts it), and `hx-post`, `hx-trigger="change"`, `hx-params="theme"` and `hx-swap="none"` on the select itself, since a control tied to another form still sits in the plugin's form in the DOM. The empty `<form id="verso-theme">` stands after the page's content, carrying the CSRF token. The response carries the new `<style id="verso-theme">` as an out-of-band swap; a refusal carries the outcome template the outcome layer shows (`HX-Reswap: none`).

**8. A page form's change tracking ignores a control it does not own.** `verso-forms.js` counts a change toward a form only when `event.target.form` is that form, so the theme select, which bubbles its change through the plugin's form, never marks it unsaved.

**9. Contrast is checked at load and enforced for shipped themes.** `internal/theme` computes WCAG ratios over the effective palette (Sand light overlaid with the theme): words at 4.5:1 (ink, body and meta on ground, quiet, canvas; on-fill on denim, denim-deep, crimson, marigold-deep; each `-deep` on its `-soft`), glyphs at 3:1 (glyph on ground, the choice border), meta on mid at 4.4:1 as Sand light keeps it. Marks Sand light deliberately keeps under 3:1 (the marigold square) are not checked. A dark theme whose shadow colours all equal Sand light's is logged. A test loads `themes/*.json` and fails on any pair under its floor.

**10. The shipped palettes.** Sand dark mirrors Sand's eleven steps at the same hue, with the page darkest and cards one step up; Mocha maps Catppuccin's roles onto the token jobs. Both cleared every floor in the mock (`claude.ai/artifact/SG4tRxMMsGtvCmdJQvm25t`).

| token | Sand dark | Mocha |
|---|---|---|
| canvas / ground / quiet | `#121110` `#1c1a17` `#151412` | `#11111b` `#1e1e2e` `#181825` |
| mid / rule / rule-strong | `#2a2723` `#2b2825` `#3d3934` | `#313244` `#2a2b3d` `#45475a` |
| inert / sand-5 / faint / glyph | `#4a453f` `#57514a` `#736d63` `#8f897e` | `#585b70` `#6c7086` `#6c7086` `#7f849c` |
| meta / body / ink | `#a49e92` `#bdb6aa` `#eeeae3` | `#9399b2` `#bac2de` `#cdd6f4` |
| denim / -deep / -line / -soft | `#6b9bd6` `#9cc0ec` `#344150` `#212834` | `#89b4fa` `#b4befe` `#3e4b6b` `#2b3046` |
| on-fill | `#0f141b` | `#11111b` |
| green / -deep / -line / -soft | `#34b98a` `#7dd6b0` `#234a3a` `#182a22` | `#a6e3a1` `#a6e3a1` `#475950` `#2e363c` |
| marigold / -deep / -line / -soft | `#f0a419` `#f2c266` `#5c4318` `#2e2414` | `#f9e2af` `#f9e2af` `#605955` `#38363d` |
| crimson / -deep / -line / -soft | `#e25a55` `#f3a29d` `#572d2a` `#311d1b` | `#f38ba8` `#f38ba8` `#5e3f53` `#382b3d` |
| amethyst / -deep / -line / -soft | `#a97fd8` `#d0b1f0` `#463851` `#2a2131` | `#cba6f7` `#cba6f7` `#52476a` `#332e46` |
| shadows (field, float, menu, dialog, drawer, edge) | black at 35, 45, 45, 50, 45, 40% | crust at the same strengths |
| chart washes (from / to) | hue at 25% / 5% | hue at 25% / 5% |

## Risks / Trade-offs

- [A theme's colour misses a surface Tailwind wrote a fallback for] → Tailwind writes a literal fallback for an opacity modifier (`bg-quiet/50`) only for browsers without `color-mix()`; every browser the shell supports takes the `var()` path.
- [The theme select's change marks the plugin form unsaved] → Decision 8, and an integration check with a half-typed router name.
- [htmx includes the plugin form's fields in the theme POST] → `hx-params="theme"` limits the request to the one value; the route ignores anything else and reads only `theme`.
- [A dark theme forgets its shadows] → the load-time log names it; first-party themes set them, held by their test.
- [Operators surprised by Sand dark after upgrading] → the release notes for the release that carries this say a dark-preferring browser now gets Sand dark, and where to force light.
- [`sand-dark.json` deleted by hand] → Sand following the browser draws Sand light in both, logged; nothing breaks.

## Migration Plan

No on-device state is renamed or moved: `verso.ui.theme` is a new option whose absence is the default, and `setTheme` creates `verso.ui` when it first writes. The release notes say that dark-preferring browsers get Sand dark. Rolling back the package returns the single light look and leaves an unread `verso.ui.theme` behind.
