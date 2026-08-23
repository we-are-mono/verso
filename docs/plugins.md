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
  ],
  "acl": {
    "read": [
      { "scope": "uci", "object": "system", "function": "read" }
    ],
    "write": [
      { "scope": "uci", "object": "system", "function": "write" }
    ]
  }
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
| `acl` | the rpcd access scopes you need ([below](#declaring-your-acl-scopes)): `read` to render config, `write` to change it |
| `acl.read[]` | one `{scope, object, function}` grant naming a config the shell reads and hands you as a snapshot |
| `acl.write[]` | one `{scope, object, function}` grant the shell checks before a POST |

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
(below your mount), method, query, and form body to you — plus the read snapshot
of the configs you declared, in the `X-Verso-UCI` header (see
[Reading config](#reading-config-the-read-snapshot)) — and expects a **schema
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
- `commit` — optional; on a successful write, the uci changes for the shell to
  apply on your behalf (see [Writing config](#writing-config-the-commit-intent)).
  You never write config yourself.

**GET** `<path>` → return the page as a schema envelope, HTTP 200.

**POST** `<path>` (form submit) → run your **semantic** checks (the shell handles
datatypes), then:
- **success:** HTTP 200 with the re-rendered page (a success note), plus a
  `commit` intent for whatever changed. The shell — not you — performs the write,
  after it has validated every datatype.
- **semantic failure:** HTTP **422**, the *same* form re-rendered with each bad
  field carrying its `error` and the submitted `value`, and **no** `commit`. The
  shell merges any datatype errors into the same form (see Validation).

You may serve multiple pages (multiple `nav` paths) from one socket; route on the
request path like any HTTP server.

### Declaring your ACL scopes

Verso — not your plugin — is the enforcement point for privilege (ADR-007). Your
plugin holds no session and touches config in neither direction: the shell holds
the operator's rpcd session and acts through it on your behalf. You declare, as
`{scope, object, function}` triples mirroring `session.access` one-to-one, the two
things you need:

- **`acl.read`** — the configs the shell reads and hands you as a snapshot (see
  [Reading config](#reading-config-the-read-snapshot)).
- **`acl.write`** — the configs a save changes.

```json
"acl": {
  "read":  [ { "scope": "uci", "object": "system", "function": "read"  } ],
  "write": [ { "scope": "uci", "object": "system", "function": "write" } ]
}
```

The two are gated differently, because rpcd already gates them differently:

- **Writes are probed and refused up front.** Before dispatching any state-changing
  request (POST/PUT/PATCH/DELETE), the shell probes `session.access` for **every**
  `acl.write` entry. If the operator's ACLs cover them all, your handler runs. If
  any is denied, the shell returns **403** and your plugin is never called; if rpcd
  can't be reached, it fails closed with **503**. **A plugin that declares no
  `acl.write` cannot receive a state-changing request at all.**
- **Reads are scoped, not gated.** `acl.read` names *which* configs the shell reads
  for you; the operator's own sid scopes the actual read at rpcd, so an operator
  who may not read a config simply gets an empty snapshot — never a 403. Reads
  (GET/HEAD) are never gated by the shell.

This gates *who* may act; you remain authoritative for deciding *what* to write and
for semantic validation (below).

### Reading config (the read snapshot)

Your plugin **does not read `/etc/config`** — it runs unprivileged and holds no
session (ADR-007). Instead, the shell reads each config you declared in `acl.read`
with the operator's sid and injects the result into every request to your socket
as the **`X-Verso-UCI`** header: base64-encoded JSON, shaped

```json
{ "<config>": { "<section>": { ".type": "…", ".name": "…",
                               "<option>": "…", "<list-option>": ["…", "…"] } } }
```

— exactly rpcd's `uci get <config>` output. Each section carries its `.type`,
`.name`, and `.index` meta; a scalar option is a string, a list option a JSON
array. Decode the header, then read config from it: filter sections by `.type` to
enumerate them (this is how you walk anonymous sections — a firewall's rules, a
WireGuard interface's peers), and read options by name. A missing or empty header
means the operator couldn't read the config, or you declared no `acl.read`; render
empty rather than erroring.

You link no uci library and open no file — the snapshot is your whole view of
config. A GET carries it too, so a fresh page render reads current state.

### Writing config (the `commit` intent)

Your plugin **does not write uci itself** — it runs unprivileged (the same
non-root user as the shell) and holds no session (ADR-007). To change config,
return a `commit` array next to your `widget` on a successful POST; the shell
executes each entry through rpcd with the operator's session:

```json
{
  "schema_version": 1,
  "title": "General",
  "widget": { "type": "card", "...": "..." },
  "commit": [
    { "config": "system", "section": "@system[0]",
      "values": { "hostname": "verso-lab" } },
    { "config": "system", "section": "ntp",
      "values": { "server": ["0.pool.ntp.org", "1.pool.ntp.org"] } }
  ]
}
```

Each entry is one `uci set`: `config` + `section` + a `values` map of
option→value, where a value is a string (an option) or an array of strings (a
list option). The shell runs `set` then `commit`, and only then renders your
`widget`. Two rules bound it, both enforced by the shell — not by your good
behaviour:

- **You can only write configs you declared** in `acl.write` (`scope: "uci"`,
  `object: "<config>"`). A `commit` for any other config is refused and nothing
  is written.
- **rpcd re-checks the operator** on every write, so a session that may not write
  that config is refused even if you ask.

If the write is refused or fails, the shell shows a contained notice instead of
your page. A `commit` on a GET is ignored.

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
- `datatype`: a datatype name the shell enforces (see [Datatypes](#datatypes)). Optional.
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

> **The schema's edges are a finding.** A single value repeats with `list`; a group
> of fields that maps to uci sections repeats with
> [`repeater`](#repeater--a-repeatable-group-of-sections); a field-set appears on a
> toggle with [`conditional`](#conditional--a-field-set-behind-a-toggle). What still
> can't be expressed — a value *computed* from another field, or a field-set gated
> on a multi-value `select` — is a *finding*: tell us, so the next behavioural widget
> is the right one.

### repeater — a repeatable group of sections

A **behavioural** widget: a group of widgets that repeats, backed by a set of uci
sections. You declare the *intent* — the items are `section_type` sections of
`config` — and render the items you read from the snapshot. **The shell owns the add
and remove affordances and performs the uci section add/delete itself** (through
rpcd, within your `acl.write`); you ship no add/remove logic and no client code
(ADR-005 §7).

```json
{ "type": "repeater", "config": "network", "section_type": "wireguard_wg0",
  "add_label": "Add peer",
  "items": [
    { "section": "cfg0a1b2c",
      "widget": { "type": "card", "title": "Peer: phone",
                  "children": [ /* this peer's form */ ] } }
  ] }
```

- `config` + `section_type` — the uci backing the shell adds to and deletes from. It
  must be a config you declared in `acl.write`, or the shell refuses the change.
- `items[].section` — the item's uci section id (the snapshot's section key), so the
  shell's Remove can name it.
- `items[].widget` — the subtree that renders the item (typically a `card` with a
  `form`). Namespace its field names by the section so a save targets the right one.

On **add**, the shell creates a new empty section of `section_type` and re-renders;
its fields are blank for the operator to fill and save through the ordinary form
path. On **remove**, the shell deletes the item's section and re-renders. You never
see the add/remove request — you only ever render the sections that exist.

### conditional — a field-set behind a toggle

A **behavioural** widget: a field-set the shell shows only when its toggle is on.
You declare the intent — this toggle gates these fields — and the initial state; the
shell owns the toggle and the show/hide, realized in **pure CSS** (no JavaScript, no
round-trip). The toggle posts its own value, so your handler can read it to decide
whether to write the gated option (ADR-005 §7).

```json
{ "type": "conditional", "name": "use_psk", "label": "Use a pre-shared key",
  "checked": true,
  "fields": [
    { "type": "field", "name": "preshared_key", "label": "Pre-shared key", "value": "…" }
  ] }
```

- `name` — the toggle's form field name. It posts `name=on` when checked, nothing
  when unchecked; read it in your handler to gate the save.
- `label` — the toggle's caption.
- `checked` — whether the field-set starts visible; derive it from state (e.g. "the
  pre-shared key is set").
- `fields` — the field-set revealed when the toggle is on.

A hidden gated field still submits its value (it is only visually hidden), so decide
from the toggle: when it is off, ignore or omit those options. (Gating on a `select`
with several values is a later realization under the same declaration.)

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

Two layers, split by what each can know (ADR-008):

- **Declarative — the `datatype`, enforced by the shell.** Put a `datatype` on a
  `field` or `list` (the names below). On a POST the shell validates every
  submitted value against its datatype, and if any fail it re-renders your form
  with the message under the offending field — or, for a `list`, the offending
  row — and **does not apply your `commit`**. You declare it; the shell enforces
  it. You do not re-check datatypes yourself.
- **Semantic — cross-field and stateful, in your POST handler.** Rules only you
  can know: "is this port already used," "do these ranges overlap," "start ≤ end."
  Validate these yourself; on failure return **HTTP 422** with the same form
  re-rendered — each bad field's `error`, a `list`'s index-keyed `errors`, or the
  form-level `error` for a message tied to no single field — and **no** `commit`.

Both layers feed the same error slots and render identically, so the operator
can't tell which produced a message. (Live client-side validation is out of scope.)

### Datatypes

Declare one of these as a field's or list item's `datatype`; the shell enforces
it. They are LuCI's names, so OpenWrt authors already know them.

| datatype | accepts |
|---|---|
| `hostname` | a hostname label or dotted name — `router`, `my-host`, `host.lan` |
| `fqdn` | a fully-qualified domain name — a hostname with at least one dot, `host.example.com` |
| `ip4addr` | an IPv4 address — `192.168.1.1` |
| `ip6addr` | an IPv6 address — `2001:db8::1` |
| `ipaddr` | an IPv4 or IPv6 address |
| `host` | a hostname or an IP address |
| `port` | a port number, 1–65535 |

## A worked example: verso-plugin-hostname

The reference plugin (`verso-plugin-hostname`, its own module) manages the system
hostname and NTP server list. It was written **only from this document** — it
shares no Go code with the shell. Its whole shape:

- **manifest.json** places one entry under System → General, and declares
  `acl.read` and `acl.write` for the `system` config.
- **GET /** reads the hostname and NTP servers from the `X-Verso-UCI` snapshot the
  shell brokered (it links no uci library) and returns a
  `card` → `form` → (`field` hostname + `list` servers) envelope.
- **POST /** reads `hostname` and the multi-value `server` field and returns a
  `commit` intent setting `system.@system[0].hostname` and the `server` list,
  which the shell writes and commits through rpcd. The plugin declares the
  datatypes (`fqdn` for the hostname, `host` for each server) and does no
  validation of its own; if a value fails its datatype the shell re-renders the
  form with the error and applies nothing. The plugin runs unprivileged and
  touches no config itself.

The shell writes and commits, but the values apply on the next service reload —
the plugin surfaces that live-apply caveat with a `raw` note. That is the honest
use of the bridge: an explanatory message no widget yet covers, which is exactly
the signal for whether a future "note" widget is worth building.

It runs under procd (above); point the shell's `$VERSO_PLUGINS_DIR` at the
installed manifest and the page appears at `/plugins/hostname/`.

## Conformance checklist

A correct plugin:

1. Serves HTTP on its `socket`; a **GET** to each nav path returns a valid schema
   envelope (`application/json`, a `schema_version` the shell supports, one root
   `widget`).
2. Emits **only** the documented widget types with semantic props — no HTML/CSS,
   no colours or spacing.
3. Declares a `datatype` on each field/list to be validated (the shell enforces
   it), does any **semantic** checks in its handler, and on a semantic failure
   returns **422** with `error`/`errors` and no `commit`.
4. Reads a `list` as a multi-value form field, dropping blank slots.
5. Never assumes it is reachable: it is fine for the shell to render
   "unavailable", and your plugin must not depend on always being up.
6. Runs as a single procd instance with output/crashes visible in `logread`.

Hold all six and your plugin renders natively and cannot take the shell down.
