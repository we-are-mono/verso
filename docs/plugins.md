# Writing a Verso plugin

This is the author's guide. If you can serve HTTP on a unix socket and emit JSON,
you can write a Verso plugin — in any language, without touching the shell.

> **Status:** grows with the widget set. The mechanical contract (manifest,
> socket, envelope, isolation) is stable. The widget vocabulary section is filled
> in as each widget lands; anything marked _(landing)_ is not yet renderable.
> The authority for design decisions is `docs/ADR/006-plugin-contract.md`
> (mechanics) and `docs/ADR/005-ui-consistency-contract.md` (what you may emit).

## What a plugin is

- **A separate program.** The shell runs it as its own process, supervised by
  procd. If your plugin crashes, hangs, or returns nonsense, the shell renders
  "plugin unavailable" in its chrome and stays up. You cannot take Verso down.
- **A schema emitter, never a page.** You return a *widget schema* (JSON); the
  shell renders it through its own templates and design tokens. You never emit
  HTML, CSS, or JavaScript. Two plugins by two strangers render identically —
  that is the point (ADR-005).
- **Reached over a unix socket.** The shell is an HTTP client to your socket and
  a renderer to the browser. You never see the browser.

## The manifest

Ship a `manifest.json`. The shell discovers it by globbing the plugins directory
(`$VERSO_PLUGINS_DIR`, default `/usr/share/verso/plugins/<id>/manifest.json`).

```json
{
  "manifest_version": 1,
  "id": "hostname",
  "name": "System — General",
  "socket": "/var/run/verso/hostname.sock",
  "schema_version": 1,
  "nav": [
    { "section": "System", "label": "General", "path": "/" }
  ]
}
```

| field | meaning |
|---|---|
| `manifest_version` | manifest format version (currently `1`) |
| `id` | stable, URL-safe; mounts your plugin at `/plugins/<id>/` |
| `name` | display name |
| `socket` | absolute path your process listens on |
| `schema_version` | the widget vocabulary you emit (currently `1`) |
| `nav` | a **list** of menu entries (below); one plugin may place several pages |
| `nav[].section` | which shell nav group the entry appears under |
| `nav[].label` | the nav link text |
| `nav[].path` | page path, relative to your mount (`/` = your index) |

To place several pages in the menu, add more entries — each is grouped under its
own `section`:

```json
"nav": [
  { "section": "System", "label": "General", "path": "/" },
  { "section": "System", "label": "Time",    "path": "/time" }
]
```

## The socket contract

Serve HTTP/1.1 on your `socket`. The shell forwards the browser's request path
(below your mount), method, query, and form body to you, and expects a **schema
envelope** back — `Content-Type: application/json`:

```json
{
  "schema_version": 1,
  "title": "General",
  "widget": { "type": "card", "title": "Hostname", "children": [ /* … */ ] }
}
```

- `widget` — one root widget (usually a `card`) that is your whole page body.
- `title` — the page heading the shell renders above it.

**GET** `<path>` → return the page as a schema envelope, HTTP 200.

**POST** `<path>` (form submit) → validate and apply, then:
- **success:** HTTP 200, the re-rendered page (include a success note).
- **validation failure:** HTTP **422**, the *same* form re-rendered with each bad
  field carrying its `error` and the submitted `value`. You are authoritative —
  the shell renders exactly what you return.

You may serve multiple pages (multiple `nav` paths) from one socket; route on the
request path like any HTTP server.

### What the shell does when you misbehave

Any dial refusal, timeout, non-JSON body, undecodable schema, or 5xx becomes a
styled **"plugin unavailable"** card in the shell chrome, with a 200 to the
browser. The shell never crashes on your account, and never blocks unbounded
(the transport times out). Design accordingly: you own your own reliability, but
you cannot damage the shell.

## Running your plugin (procd)

On OpenWrt, run your plugin as a **procd service** so the system — not you —
guarantees the plumbing. Ship an `/etc/init.d/<verso-plugin-name>` and let procd
own the process:

```sh
#!/bin/sh /etc/rc.common
USE_PROCD=1
START=80          # before the shell (verso is START=95): your socket is up sooner
STOP=10

start_service() {
    procd_open_instance hostname     # a NAMED instance -> procd runs exactly one
    procd_set_param command /usr/bin/verso-plugin-hostname
    procd_set_param respawn 3600 5 5 # self-heal crashes; give up after 5 in 3600s
    procd_set_param stdout 1
    procd_set_param stderr 1         # your logs/crashes -> syslog -> `logread`
    procd_close_instance
}
```

This buys you four things for free:

- **Single instance.** A named `procd_open_instance` means procd refuses to start
  a second copy — nothing else binds your socket.
- **Crashes in `logread`.** `stdout`/`stderr` go to syslog; `logread | grep
  <name>` shows every start and every crash, with a reason.
- **Self-healing.** `respawn` restarts a crashed plugin within seconds. A plugin
  that *crash-loops* (here, >5 times in 3600s) is given up on: it stays down, the
  reason is in `logread`, and the shell serves "Plugin unavailable."
- **Boot order.** `START=80` brings your plugin up before the shell (`START=95`).
  This only *shrinks* the window where a freshly-booted plugin page shows
  "unavailable" — the shell's crash isolation, not start order, is the real
  guarantee that a not-yet-listening plugin breaks nothing.

Enable it once (`/etc/init.d/<name> enable`) and procd starts it on every boot.
To show isolation with a plugin that stays down, `/etc/init.d/<name> stop` —
procd will not respawn a stopped service.

## The widget vocabulary

You compose a **closed set** of widgets with **semantic** props — `label`,
`kind`, `datatype` — never colours, spacing, or fonts (ADR-005). You cannot add
widget types; you nest and compose these. When nothing fits, `raw` is the
display-only bridge (last). Every node is `{"type": "...", …}`.

### card — a titled container

Nests other widgets. This is how you lay out a page.

```json
{ "type": "card", "title": "Hostname",
  "children": [ /* any widgets */ ] }
```

### table — read-only rows

```json
{ "type": "table",
  "columns": ["Field", "Value"],
  "rows": [ ["Uptime", "1h 2m"], ["Load", "0.10"] ] }
```

### form — an interactive form

Renders its `fields` inside a `POST` form that submits **back to the same page**
(you don't set an action — the shell owns the URL). On a successful save, set
`success` to show a confirmation.

```json
{ "type": "form", "submit": "Save", "success": "",
  "fields": [ /* field and list widgets */ ] }
```

`submit` defaults to `"Save"`.

### field — one labelled control

```json
{ "type": "field", "name": "hostname", "label": "Hostname",
  "kind": "text", "value": "OpenWrt", "datatype": "hostname",
  "error": "", "help": "The device's hostname." }
```

- `kind`: `"text"` (default) or `"select"`.
- `value`: the current value; echo the submitted value back on a failed POST.
- `datatype`: a tier-1 datatype name (see Validation). Optional.
- `error`: an inline error to show under the field (you set this on a 422).
- For `kind:"select"`, supply `options` and set `value` to the selected one:

```json
{ "type": "field", "name": "zonename", "label": "Timezone", "kind": "select",
  "value": "UTC",
  "options": [ {"value":"UTC","label":"UTC"},
               {"value":"Europe/Ljubljana","label":"Europe/Ljubljana"} ] }
```

### list — a repeating text field

Several values under **one** `name`, each validated against the same `datatype`.
The shell renders each item as an input plus one trailing blank slot; **all share
`name`, so they post as a multi-value field** (`server=a&server=b&server=`). In
your POST handler, read every value for that name, drop the blanks, and rewrite
the list. Report a bad item with `errors` keyed by the item's index:

```json
{ "type": "list", "name": "server", "label": "NTP servers",
  "kind": "text", "datatype": "host",
  "items": ["0.openwrt.pool.ntp.org", "1.openwrt.pool.ntp.org"],
  "errors": { "1": "must be a hostname or IP address" },
  "help": "One server per line." }
```

> **This is the deliberate stress test for the schema bet (see FINDINGS F3).**
> If you find yourself wanting client-side add/remove buttons, cross-field
> conditionals, or a value computed from another field — the schema can't express
> those, by design. That gap is a *finding*: tell us, so the next widget is the
> right one.

### raw — the governed bridge

Display-only Markdown, for when no widget fits. Sanitised (no HTML passthrough,
dangerous URL schemes stripped) and rendered through Verso's tokens with a
visible "raw" affordance. **Never interactive** — no inputs, no forms. Its usage
is metered as the demand signal for the next widget; treat it as a temporary
bridge, not a home.

```json
{ "type": "raw", "markdown": "**Note:** applied on next reboot." }
```

## Validation

Two tiers (tier 2, live client validation, is intentionally out of scope):

- **Tier 1 — declarative `datatype`** on a `field`/`list`. Reuses LuCI's datatype
  names — currently `hostname`, `ip4addr`, `ip6addr`, `ipaddr`, `host`, `port`.
  It is carried in the schema and surfaced to the browser as a hint. **It is not
  enforced by the shell** — in the schema-gateway model the shell has no per-form
  state at POST time (see FINDINGS), so tier 1 is advisory and tier 3 is where
  safety lives.
- **Tier 3 — server-side, in your POST handler. Authoritative.** Validate the
  submitted values yourself; on failure return **HTTP 422** with the same form
  re-rendered, each bad field carrying its `error` and its submitted `value`
  (and, for a `list`, the `errors` map). Never trust tier 1 alone.

## A worked example: verso-plugin-hostname

The reference plugin (`verso-plugin-hostname`, its own module) manages the system
hostname and NTP server list. It was written **only from this document** — it
shares no Go code with the shell. Its whole shape:

- **manifest.json** places one entry under System → General.
- **GET /** reads the hostname and NTP servers from uci and returns a
  `card` → `form` → (`field` hostname + `list` servers) envelope.
- **POST /** reads `hostname` and the multi-value `server` field, validates each
  authoritatively (tier 3), and either:
  - returns **422** with the form re-rendered — bad fields carrying `error` and
    the submitted `value`, the list its index-keyed `errors` — and writes
    nothing; or
  - `uci set system.@system[0].hostname`, rewrites the `server` list,
    `uci commit`s, and returns **200** with a `success` note.

The write persists immediately, but the values apply on the next service reload —
the plugin surfaces that live-apply caveat with a `raw` note. That is the honest
use of the bridge: an explanatory message no widget yet covers, which is exactly
the signal for whether a future "note" widget is worth building.

It runs under procd (above); point the shell's `$VERSO_PLUGINS_DIR` at the
installed manifest and the page appears at `/plugins/hostname/`.

## Conformance checklist

Until an executable conformance kit lands (ADR-003), a correct plugin:

1. Serves HTTP on its `socket`; a **GET** to each nav path returns a valid schema
   envelope (`application/json`, a `schema_version` the shell supports, one root
   `widget`).
2. Emits **only** the documented widget types with semantic props — no HTML/CSS,
   no colours or spacing.
3. On **POST**, validates authoritatively (tier 3) and returns **422** with
   field/`errors` on failure or **200** on success — and never writes on invalid
   input.
4. Reads a `list` as a multi-value form field, dropping blank slots.
5. Never assumes it is reachable: it is fine for the shell to render
   "unavailable", and your plugin must not depend on always being up.
6. Runs as a single procd instance with output/crashes visible in `logread`.

Hold all six and your plugin renders natively and cannot take the shell down.
