# Design

## Context

`palette.css` is the shell's one `@theme` (ADR-005 §6): the Tailwind utilities read its tokens through `var(--color-*)`, and `TestNothingWearsAStockTailwindRamp` keeps Tailwind's own scales out. What the guard does not catch is a colour written as a value. A sweep of the shell's sources finds these:

| Leak | Where |
|---|---|
| `text-white` / `bg-white` / `bg-white/50`, reading Tailwind's default `--color-white` | 14 widget and shell templates and `verso.js`; `forms.css` (`.verso-box`) |
| Shadows in Ink as `rgba(27,25,23,…)` | arbitrary `shadow-[…]` values in 11 templates, `forms.css` (field inset, twice), `verso-tables.js` (the lifted row) |
| A port tip's shadow, `rgb(0 0 0 / .7)` | `input.css` |
| Chart wash strengths as bare `stop-opacity` numbers | `input.css`, `.verso-chart--*` |
| `bg-[#f9f7f3]` | `table-interfaces.html.tmpl` |
| `<meta name="theme-color" content="#…">` | `upgrading`, `restarting`, `firmware-started` |

Left alone on purpose: the Safari pinned-tab colour in `icons.html.tmpl` and the manifest's `theme_color`/`background_color` (the brand's icon colour, not the page), the `#000` alpha in the scroll-edge mask, and the marker's data-URI SVG (a mask, coloured by Strong Hairline). Plugins express no appearance (ADR-005), and none writes a colour.

## Goals / Non-Goals

**Goals:**
- Each leak reads a palette token holding today's value, so the light look is unchanged to the pixel.
- One file states every colour and shadow; a guard keeps it that way.

**Non-Goals:**
- No new look, no second palette and no way to choose one. ADR-005's "one light theme" stands.
- No change to plugins, the widget schema or on-device state.

## Decisions

**1. On-fill is a new token; surfaces move to the existing Canvas.** White did two jobs that only agree while the page is light. `bg-white`/`.verso-box`'s fill become `bg-canvas`/`var(--color-canvas)` (Canvas is already the "one surface a step above the page"); `text-white` becomes `text-on-fill` (`--color-on-fill: #ffffff`). `palette.css` then unsets Tailwind's `--color-white` and `--color-black` (`initial`), so `text-white` compiles to nothing at all, rather than to a white nobody chose: the same move the font resets in `input.css` already make. *Alternative:* keep `--color-white` and only document its jobs. Rejected: one token cannot hold two values, which is the whole problem.

**2. Shadows are Tailwind `--shadow-*` theme tokens, one per job.** `--shadow-field`, `--shadow-tooltip`, `--shadow-menu` (the 8px/24px drop), `--shadow-dialog`, `--shadow-drawer`, `--shadow-edge` (the inset scroll edge), `--shadow-lifted` (the reordered row's two-part drop) and `--shadow-port-tip`, each holding its current value. Tailwind generates `shadow-field`, `shadow-dialog` and the rest, so templates trade `shadow-[0_8px_32px_rgba(27,25,23,.16)]` for `shadow-dialog`; a composite such as the invalid field's ring names the token inside it (`shadow-[inset_0_0_0_1px_var(--color-crimson-deep),var(--shadow-field)]`). A shadow is a whole token, colour, offset, blur and strength together, because a shadow that reads on a dark ground needs more strength as well as a different colour. *Alternative:* one `--color-shadow` mixed at each use's alpha. Rejected: it fixes the colour but leaves the strengths as literals scattered over 27 sites. The overview card lifted while arranging already casts the dialog's shadow; it reads `--shadow-dialog`.

**3. Chart washes are plain custom properties beside the palette.** `--chart-wash-from: .16`, `--chart-wash-to: .04` for the live traffic series (denim, amethyst) and `--chart-role-wash-from: .24`, `--chart-role-wash-to: 0` for a role series (violet, emerald). They are numbers, not colours, so they live in a `:root` block in `palette.css` rather than in `@theme`, where Tailwind would look for a utility namespace. The two pairs keep today's two looks; merging them would change a chart, which this change does not.

**4. `#f9f7f3` reads `ground`.** It is the Interfaces table's expanded panel, a hair off Paper (#faf9f7) and visually indistinguishable. Paper is the token for that job. This is the one place a value moves, by 1–2 units per channel, and it is named here so the review can look at it.

**5. The Go side reads `palette.css` itself.** A small `internal/palette` package parses the embedded `palette.css` source once at startup into token name → value, and the takeover templates take their `theme-color` from it (`canvas` for upgrading and firmware-started, `ground` for restarting). *Alternatives:* a Go constant (rejected: a second copy of a value that drifts the day the palette changes); dropping the meta (rejected: the takeover's bar would fall back to the browser's own colour, a visible change); reading the compiled `verso.css` (rejected: its minified form shortens values such as `#fff`, a format the shell does not own). The package parses only what `palette.css` writes: `--name: #hex;` lines inside `@theme`. Anything else it ignores, and a test pins that every colour token is read.

**6. The guard sits beside the stock-ramp guard.** `TestNothingPaintsALiteralColour` in `internal/widget/palette_test.go` walks the same roots as `TestNothingWearsAStockTailwindRamp` (shell and widget templates, `input.css`, `forms.css`, `sections.css`, the client's `verso*.js` but dev and boot) plus the shell's non-test Go sources. It flags a hex value (not an `&#…;` entity, a `#fragment` or `url(#id)`), `rgb(`/`rgba(`/`hsl(` with a number, a `white`/`black` colour utility (not `white-space`), and an arbitrary `[#…]`. Kept literals live in a map with a reason each, as `gridExceptions` does for the grid. `palette.css` is the one file it skips.

## Risks / Trade-offs

- [A swap changes a pixel by accident] → Every new token holds the exact value it replaces (except decision 4, called out). A test pins each new token's value in the compiled stylesheet; the review walks the Overview, a drawer, a dialog, an invalid field and the takeovers in the dev container.
- [The guard flags something that is not a colour] → Its exceptions map takes the line with its reason; a false positive costs one entry, not a weaker regex.
- [Tailwind drops an unused theme token] → Tailwind v4 emits only the theme variables a utility uses. Every new token has a use in this change, and the value test reads the compiled stylesheet, so a token that falls out fails there.
- [A plugin's bundled template writes `text-white`] → Plugins render through the shell's widgets and ship no templates or CSS (ADR-005), so none can.

## Migration Plan

None on the device: the change is stylesheet, templates and the shell binary. Rolling back is reverting the commit.
