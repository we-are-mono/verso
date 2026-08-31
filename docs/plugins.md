# Writing a Verso plugin

This is the author's guide. If you can serve HTTP on a unix socket and emit JSON,
you can write a Verso plugin — in any language, without touching the shell.

> **Status:** grows with the widget set. Manifest v1 whole-page plugins are the
> current implementation. Manifest v2 page contributions and coordinated Save &
> Apply are the accepted target contract and are documented here before their
> implementation. The widget vocabulary section is filled in as each widget lands;
> anything marked _(landing)_ is not yet renderable.
> The authority for design decisions is `docs/ADR/006-plugin-contract.md`
> (mechanics) and `docs/ADR/005-ui-consistency-contract.md` (what you may emit).

## What a plugin is

- **A separate program.** The shell runs it as its own process, supervised by
  procd. If your plugin crashes, hangs, or returns nonsense, the shell renders
  "plugin unavailable" in its chrome and stays up. You cannot take Verso down.
- **A schema emitter, never a page.** You return a *widget schema* (JSON); the
  shell renders it through its own templates and styling. You never emit
  HTML, CSS, or JavaScript. Two plugins by two strangers render identically —
  that is the point (ADR-005).
- **Reached over a unix socket.** The shell is an HTTP client to your socket and
  a renderer to the browser. You never see the browser.

## The manifest

Ship a `manifest.json`. The shell discovers it by globbing the plugins directory
(`$VERSO_PLUGINS_DIR`, default `/usr/share/verso/plugins/<id>/manifest.json`).

```json
{
  "manifest_version": 2,
  "id": "ntp",
  "name": "Time synchronization",
  "socket": "/var/run/verso/ntp.sock",
  "schema_version": 1,
  "contributions": [
    {
      "id": "general-time",
      "hook": "system.general.after-time",
      "path": "/contributions/general",
      "order": 100
    }
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
| `manifest_version` | manifest format version: `1` for whole-page plugins; `2` adds contributions |
| `id` | stable, URL-safe; mounts your plugin at `/plugins/<id>/` |
| `name` | display name |
| `socket` | absolute path your process listens on |
| `schema_version` | the widget vocabulary you emit (currently `1`) |
| `nav` | optional list of menu entries; one plugin may place several standalone pages |
| `nav[].section` | which shell nav group the entry appears under |
| `nav[].label` | the nav link text |
| `nav[].path` | page path, relative to your mount (`/` = your index) |
| `contributions` | optional list of fragments inserted at published shell hooks |
| `contributions[].id` | contribution id, unique within the plugin; namespaces its fields |
| `contributions[].hook` | globally unique shell hook id |
| `contributions[].path` | socket endpoint used to render and prepare the fragment |
| `contributions[].order` | order within the hook; plugin id and contribution id break ties |
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

These entries are registrations, not suggestions layered over a shell page list.
For example, the bundled System plugin creates **System → General** solely through
the first entry above. The shell contributes its own System pages separately and
does not hardcode General. A registration appears while the plugin socket is live;
stopping the service withdraws it on the next page render, while its direct
`/plugins/<id>/…` URL remains available to show the contained unavailable state.

Manifest v2 may declare `nav`, `contributions`, or both, but must declare at least
one. A contribution-only plugin creates no sidebar entry.

### Contributing to a shell page

Shell templates publish stable hooks at every semantic seam between their sections,
for example:

```text
system.general.before-identity
system.general.after-identity
system.general.after-time
system.general.end
```

Choose any published hook in the manifest. On page render the shell GETs the
contribution's `path` and places its widget fragment there. A fragment uses the same
closed widget vocabulary as a page, but must not contain a `form`: the shell wraps its
own fields and all contributions in one outer page form. Field names are automatically
namespaced on the browser side and stripped before your endpoint receives them.

Several plugins may share a hook. `order`, then plugin id, then contribution id make
the result deterministic. An unknown hook affects only that contribution and appears
as a compatibility warning under Software. A missing plugin leaves no placeholder; a
running plugin that fails produces a contained unavailable fragment without breaking
the surrounding page.

## The socket contract

Serve HTTP/1.1 on your `socket`. For a standalone page the shell forwards the
browser's request path (below your mount), method, query, and form body to you. For a
contribution it calls the manifest path itself and forwards only that contribution's
values during prepare. Both carry the read snapshot
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

- `widget` — one root widget: a whole page body or one contribution fragment.
- `title` — the standalone page heading; omit it for a contribution.
- `kicker` — optional eyebrow above a standalone page heading.
- `kicker_status` — optional short emerald state beside the kicker, such as
  `"Complete"` on a finished styleguide reference.
- `action` — optional `{label, href, icon?}`: the page's one primary doorway
  ("New rule", "Add forward"), rendered as a button hard right on the heading
  row. A page has at most one — it is *the* thing to do here, not a menu, so a
  second affordance belongs beside the content it acts on. `href` is a route
  through the shell, exactly like a `link` widget's, so address your own mount
  in full (`/plugins/<id>/…`).
- `banner` — optional page-level semantic notice rendered full-width directly
  beneath the subpage bar (or in its place when there is no bar). Reserve it for
  a state important enough to remain visible above the page heading; use an
  in-content `callout` for ordinary context.
- `notice` — optional `{level, text}`: the *outcome* of the action this render
  answers ("Saved.", "Could not read the form.", "Takes effect on the next
  reload."). `level` speaks the tone vocabulary (`success` | `warning` |
  `danger` | `info`). The shell renders it in its own flash slot, exactly where
  its own confirmations appear — state the outcome, never compose it from
  widgets. Standing state belongs in `banner`; context beside content in a
  `callout`.
- `immediate` — optional boolean for a page made only of direct commands rather
  than staged configuration. It omits the capsule when the shared UCI stage is
  clean; pending changes from elsewhere remain visible. It does not make a
  returned `commit` immediate—commit intents always stage.
- `commit` — optional; on a successful write, the uci changes for the shell to
  apply on your behalf (see [Writing config](#writing-config-the-commit-intent)).
  You never write config yourself.
- `apply` — optional; a tightly typed non-UCI operation the shell performs after
  the staged UCI transaction applies. Only shell-known operations are accepted,
  and the manifest must declare the exact matching rpcd write scope.

**GET** `<path>` → return the page as a schema envelope, HTTP 200.

**POST** `<path>` (standalone form or contribution prepare) → run your **semantic**
checks without writing or causing another side effect (the shell handles datatypes),
then:
- **success:** HTTP 200 with the re-rendered page (a success note), plus a
  `commit` intent for whatever changed. The shell — not you — merges it with every
  other changed owner and performs the write only after collective validation.
- **semantic failure:** HTTP **422**, the *same* form re-rendered with each bad
  field carrying its `error` and the submitted `value`, and **no** `commit`. The
  shell merges any datatype errors into the same form (see Validation).

You may serve multiple pages (multiple `nav` paths) from one socket; route on the
request path like any HTTP server. You may likewise serve several contribution paths.

### Declaring your ACL scopes

Verso — not your plugin — is the enforcement point for privilege (ADR-007). Your
plugin holds no session and touches config in neither direction: the shell holds
the operator's rpcd session and acts through it on your behalf. You declare, as
`{scope, object, function}` triples mirroring `session.access` one-to-one, the two
things you need:

- **`acl.read`** — what the shell reads and hands you. A `uci` scope names a config,
  which arrives as your snapshot (see
  [Reading config](#reading-config-the-read-snapshot)); a `ubus` scope on object
  `verso` names one of the privileged helper's read functions, which arrives beside
  it (see [Reading live state](#reading-live-state-brokered-helper-reads)).
- **`acl.write`** — the configs a save changes.

```json
"acl": {
  "read":  [ { "scope": "uci",  "object": "system", "function": "read"  },
             { "scope": "ubus", "object": "verso",  "function": "firewallCounters" } ],
  "write": [ { "scope": "uci",  "object": "system", "function": "write" } ]
}
```

The two are gated differently, because rpcd already gates them differently:

- **Writes are probed and refused up front.** Before dispatching any state-changing
  request (POST/PUT/PATCH/DELETE), the shell probes `session.access` for **every**
  `acl.write` entry. If the operator's ACLs cover them all, your handler runs. If
  any is denied, the shell returns **403** and your plugin is never called; if rpcd
  can't be reached, it fails closed with **503**. **A plugin that declares no
  `acl.write` cannot receive a state-changing request at all.**
- **Reads are scoped, not gated.** `acl.read` names *what* the shell reads for you;
  the operator's own sid scopes the actual read at rpcd, so an operator who may not
  read something simply gets nothing where the data would be — never a 403. Reads
  (GET/HEAD) are never gated by the shell. A read the shell cannot complete at all
  degrades the same way: the page still renders, without that data.

A `ubus` read scope is bounded twice over. The shell brokers only functions in its
own closed set, so naming one it does not know brokers nothing; and rpcd still
checks the operator's session on the call. Declaring a function is how you opt in,
never how you widen the surface.

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

### Reading live state (brokered helper reads)

Config is what the device was told; some pages also need what the device is doing —
a firewall rule's packet counters, say, which live in the kernel and take
`CAP_NET_ADMIN` to read. Your plugin holds no more privilege than the shell, so
these reads are brokered too: declare them in `acl.read` with scope `ubus`, object
`verso`, and the helper function's name, and the shell reads them with the
operator's sid and injects the results into every request as the **`X-Verso-Ubus`**
header, base64-encoded JSON shaped

```json
{ "<function>": <the result that function returned> }
```

Each value is the helper's own JSON, untouched — the shell does not interpret it,
and you own its meaning. The shell brokers only functions in its own closed set:

| Function | Returns |
|---|---|
| `firewallCounters` | `{"counters": [{"chain", "name", "packets", "bytes"}]}` — fw4's live nftables hit counters, one entry per (chain, rule name), summed across the several nft rules a single UCI section can render into. |
| `dhcpLeases` | `{"leases": [{"hostname", "mac", "ipv4", "ipv6s", "expires_at"}]}` — who holds an address right now, in address order. dnsmasq's DHCPv4 lease file is the list; odhcpd's DHCPv6 table adds `ipv6s` to the device it belongs to, joined by the link-layer address its DUID embeds or by the hostname both tables recorded. `hostname` is empty when the device offered none, `expires_at` is an epoch second (render the countdown against your own clock), and a DHCPv6 lease that joins no DHCPv4 lease is left out — it carries no MAC, which is the identity a reservation is keyed by. |

The header is absent when you declared no `ubus` read, when the function is not one
the shell brokers, and when the read failed or the operator was not allowed it. All
four are the same thing to you: no data. Render the page without it — a missing
count is a dash, not an error.

### Writing config (the `commit` intent)

Your plugin **does not write uci itself** — it runs unprivileged (the same
non-root user as the shell) and holds no session (ADR-007). To change config,
return a `commit` array next to your `widget` on a successful POST. This is a
declarative prepare result, not permission to write. The shell waits for every
changed owner, validates datatypes and ACLs, rejects conflicting writes, merges the
intents, stages the merged set through rpcd, and performs one UCI
apply/rollback/confirm cycle (ADR-010):

```json
{
  "schema_version": 1,
  "title": "General",
  "widget": { "type": "card", "...": "..." },
  "commit": [
    { "config": "system", "section": "ntp",
      "values": { "server": ["0.pool.ntp.org", "1.pool.ntp.org"] } }
  ]
}
```

An entry may also **create** a section: with `section` empty and a `type`
(`{ "config": "firewall", "section": "", "type": "rule", "values": { … } }`),
the shell adds a new anonymous section of that type and sets `values` on it —
one staged operation, so an "Add" drawer creates the row it promised.

An entry may **delete** a section: `delete: true` with a `section` and nothing
else (`{ "config": "firewall", "section": "block_telnet", "delete": true }`).
The whole section goes. A delete describes one operation, so an entry that also
carries a `type` or `values` is refused rather than guessed at.

Within `values`, `null` **clears** one option:

```json
{ "config": "firewall", "section": "allow_ping",
  "values": { "dest_port": null, "proto": ["tcp", "udp"] } }
```

The shell issues an option-level delete for every null and one `uci set` for
what remains. Reach for it whenever an editor owns a setting the submission no
longer carries: an empty string is a value, and a rule matching the empty port
is not a rule matching every port.

Each ordinary entry is one `uci set`: `config` + `section` + a `values` map of
option→value, where a value is a string (an option), an array of strings (a
list option), or null (clear it). Service reload and the rollback safety net
belong to the shell and OpenWrt, never to a plugin. Three rules bound the merge,
enforced by the shell — not by your good behaviour:

- **You can only write configs you declared** in `acl.write` (`scope: "uci"`,
  `object: "<config>"`). A `commit` for any other config is refused and nothing
  is written.
- **rpcd re-checks the operator** on every write, so a session that may not write
  that config is refused even if you ask.
- **Two owners cannot silently overwrite one another.** Incompatible intents for
  the same config/section/option reject the complete page submission; manifest order
  never decides whose value wins.

If any changed owner fails validation, authorization, or transport, no intent is
staged. If staging itself fails partway through, the shell restores the stage that
existed before the submission and does not apply. A `commit` on a GET is ignored.

### Privileged post-apply actions

Rare settings have a non-UCI tail. For example, disabling NTP is a UCI change,
but setting the kernel clock is not. A successful POST may therefore return an
`apply` array beside `commit`:

```json
"apply": [{
  "name": "set-system-time",
  "args": {
    "datetime": "2026-08-30T12:34:56",
    "timezone": "CET-1CEST,M3.5.0,M10.5.0/3"
  }
}]
```

This is not an extensible command channel. The shell recognizes a closed set of
action names, verifies the plugin declared the corresponding ACL scope (for this
one, `{ "scope":"ubus", "object":"verso",
"function":"setSystemTime" }`), and passes only structured arguments to
`verso-rpcd`. The helper validates them again at the root boundary and invokes no
shell. A validation failure prepares neither UCI writes nor an action; Discard
drops both; Save & Apply runs the action only after UCI apply.

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
    procd_open_instance system       # a NAMED instance -> procd runs exactly one
    procd_set_param command /usr/bin/verso-plugin-system
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

### grid — responsive columns

Use `grid` to place related widgets beside one another. `columns` is the desktop
target; the shell owns responsive collapse and spacing. `style:"form"` makes a
field grid stack to one column on narrow screens and uses the form rhythm. A
three-column form grid gives each field roughly 30% of the available width.

```json
{ "type": "grid", "style": "form", "columns": 3,
  "children": [ /* one to three fields */ ] }
```

### stack — vertical or inline rhythm

Use `stack` to group widgets without adding a surface. `compact:true` tightens
the vertical rhythm, while `inline:true` creates a wrapping row. For a small
settings group inside a wide editor, `width:"compact"` bounds it to the shell's
standard compact reading width.

### disclosure — optional detail

Use `disclosure` for advanced content that should remain available without
crowding the common path. `style:"condition"` gives it the same surface and
heading treatment as an active item in the conditions editor.

### properties — read-only facts

`properties` renders label/value facts. The default uses hairlines, `plain`
removes them, and `identity` renders one larger inline identity—useful for a
fixed system username without making it look like a table. Pair a short
explanation with a compact callout in a compact stack.

```json
{ "type": "stack", "compact": true, "children": [
  { "type": "properties", "style": "identity", "items": [
    { "label": "Username", "value": "root", "mono": true,
      "emphasis": true }
  ] },
  { "type": "callout", "variant": "neutral", "compact": true,
    "body": "Main system username cannot be changed." }
] }
```

### table — config sections as identical rows

The Advanced-view listing: one row per config section under fixed columns. Every
column declares a `kind`, and that kind renders every cell in it the same way —
rows cannot vary in shape. The tuple (from, to, protocol, port) is the row; the
name is an optional trailing comment.

```json
{ "type": "table",
  "columns": [
    { "kind": "toggle" },
    { "label": "From",     "kind": "endpoint" },
    { "label": "To",       "kind": "endpoint" },
    { "label": "Protocol", "kind": "keyword" },
    { "label": "Port",     "kind": "mono" },
    { "label": "Comment",  "kind": "comment" },
    { "label": "Hits",     "kind": "num" }
  ],
  "rows": [
    { "id": "force_dns_guest", "cells": [
      { "on": true, "name": "force_dns_guest" },
      { "endpoints": [ { "kind": "zone", "label": "guest" } ] },
      { "endpoints": [ { "kind": "router", "label": "router" } ] },
      { "text": "tcp/udp" },
      { "text": "53" },
      { "text": "Force-DNS-to-AdGuard-guest" },
      { "text": "0" }
    ] }
  ] }
```

Column kinds, one treatment each (never mix them per row):

- `"text"` (default) — plain ink text.
- `"name"` — the row's identity (a zone, an interface): bold ink, nothing else —
  in a column where the type never varies, even an icon is noise.
- `"mono"` — verbatim machine strings only: addresses, ports, device names.
- `"keyword"` — closed-vocabulary words (`tcp`, `udp`, `icmpv6`): sans, muted.
- `"comment"` — optional free text such as a UCI `name`; muted, blank when absent.
- `"num"` — right-aligned tabular figures (counters); muted.
- `"toggle"` — an on/off switch; the cell carries `on` and an optional form `name`.
- `"pill"` — an enum value as a status pill; the cell carries `text` plus a
  `variant` from the badge vocabulary (`success`/`warning`/`danger`/`info`/
  neutral) — e.g. `accept`→success, `reject`→warning, `drop`→danger. An empty
  cell renders a faint dash, which is what keeps the pills meaningful.
- `"endpoint"` — one or more traffic endpoints; each is `{ "kind", "label" }`
  where kind is `"zone"` (sans + shield), `"device"` (mono address + screen),
  `"router"` (this device, accented), or `"any"` (muted globe).
- `"reorder"` — a drag handle, first column only; see **Reorderable rows** below.

A row's `id` is its stable handle — use the UCI section name.

**Reorderable rows.** A listing whose *sequence* is meaning — evaluation order —
adds a leading `{ "kind": "reorder" }` column and names the uci config its rows
are sections of in `reorder_config`. The shell owns the interaction end to end:
it draws the handles, drags the row, holds the new sequence as a pending change,
and — when the operator saves — stages a `uci order` on that config, like every
other write, so the capsule's Save & Apply is what makes it live. Your plugin
ships no drag logic and never sees the reorder POST; it renders the page again
from the fresh snapshot.

A drop writes nothing on its own. The reorderable table renders **the page's
form** beside itself, carrying the config and the row sequence as hidden fields,
so a dragged row counts in the staged-changes bar exactly like a dirty field,
appears in its review list, and un-drags itself on Discard. That form is the
page's only one: a page whose listing drags must not also compose a `"style":
"page"` form of its own — the gateway logs a page that breaks this, because
the capsule binds to exactly one form and everything past the first is lost.
`reorder_label` names the order in the operator's words for the capsule's
review ("Rule order"); left empty it reads as the shell's generic "Order".

```json
{ "type": "table", "reorder_config": "firewall", "reorder_label": "Rule order",
  "columns": [ { "kind": "reorder" }, { "label": "From", "kind": "endpoint" } ],
  "rows": [ { "id": "allow_ping", "cells": [ {}, /* … */ ] } ] }
```

Three things make the drag real, and the shell enforces all three: the config
must be one you declared in `acl.write` (an undeclared one is refused), every row
`id` must name a section that config really holds (an unknown id is refused), and
both the leading column and `reorder_config` must be present — a table naming no
config draws no handle, because a grip the device would not remember is a lie.
The drag is bounded by the row's `group`, so a grouped listing moves a row within
its lane and never across it; sections the listing never showed keep their places
in the file, and a drag that ends where it began stages nothing.

**Direct row action.** A `pill` cell may carry a compact immediate action instead
of a state. Set `button`, `action`, `confirm_title`, and `confirm`; the shell opens
its standard alert dialog and posts `_action=<action>` back to the page only after
the operator confirms. Use this for a command with no edit surface, such as ending
a login session. Actions that need configuration still belong in a drawer.

```json
{ "button": "End session", "action": "end-session:iphone",
  "confirm_title": "End this session?",
  "confirm": "Anyone using this session will be signed out of Verso immediately." }
```

**Row drawer.** A row with a `drawer` is an object you can open: clicking the row
slides in a right panel — typically a form prefilled with the section's values, a
warning callout naming the blast radius, and a `confirm` for deletion (delete
lives in the drawer, never on the row). The row gets a trailing chevron and the
pointer; controls inside the row (toggles) keep their own meaning. Set
`hide_title:true` when the selected row and first section already establish the
editor's identity; the title remains available to assistive technology and the
close control remains visible. That first section may set `flush:true` so the
drawer supplies the outer top inset instead of stacking two layers of padding.

Set `open:true` to render the panel already open. That is how a submission you
refused comes back: re-render the listing with the offending drawer open, its
fields carrying their errors, and the operator is looking at what to fix instead
of at a closed row.

```json
{ "id": "force_dns_guest", "cells": [ /* … */ ],
  "drawer": { "title": "Edit redirect — Force-DNS-to-AdGuard-guest",
              "children": [ /* form, callout, confirm */ ] } }
```

**Seam.** A table may fold extra rows behind a collapsed block *inside the same
card* — e.g. the stock rules a fresh install ships with, present and honest but
not carrying the page:

```json
{ "type": "table", "columns": [ /* … */ ], "rows": [ /* your sections */ ],
  "seam": { "summary": "OpenWrt defaults — 9 stock rules that ship with a fresh install",
            "rows": [ /* the folded sections, same cell shapes */ ] } }
```

### Subpages (`pages`) — the domain's top bar

A domain with more than one kind of visit (live state you watch, configuration
you edit) declares its subpages next to `widget`; the shell renders them as the
top bar — the third navigation tier (sidebar → domain, top bar → subpage,
in-page → position). Paths are relative to your mount; the shell builds every
href and marks the active tab from the request, so the bar cannot point outside
your plugin. Declare the same `pages` on every subpage's envelope.

```json
{ "schema_version": 1, "title": "DHCP",
  "pages": [ { "label": "Leases", "path": "" },
             { "label": "Configuration", "path": "config" } ],
  "widget": { /* … */ } }
```

### filter — the page-wide lens

One field that narrows **every** listing on the page at once — the scale answer
for long pages (never tabs, never pagination). The shell owns the behaviour:
while typing nothing moves — non-matching table rows dim in place, zero-match
sections ghost, a collapsed seam opens only when it holds a match. `/` focuses
it from anywhere; Escape clears. It rides a sticky dock that pins to the top of
the scroll. Declare it once, near the top of the page:

```json
{ "type": "filter", "placeholder": "Filter — zone, port, IP, comment…" }
```

### settings — a card of option rows

The "config defaults" pattern: each row a plainly-named option with a one-line
description, the underlying option name as a mono code chip, and its state on
the right — a `toggle` (switch) for an on/off option, or `pills` (badge
vocabulary) for a row that reads rather than toggles.

```json
{ "type": "settings",
  "items": [
    { "title": "Default policies", "desc": "What happens to traffic no zone claims.",
      "pills": [ { "variant": "warning", "text": "in: reject" },
                 { "variant": "success", "text": "out: accept" },
                 { "variant": "warning", "text": "fwd: reject" } ] },
    { "title": "SYN-flood protection",
      "desc": "Rate-limit half-open connections to blunt basic floods.",
      "code": "synflood_protect",
      "toggle": { "name": "synflood_protect", "on": true } }
  ] }
```

### form — a standalone page form

On a standalone plugin page, renders its `fields` inside a `POST` form that submits
**back to the same page** (you don't set an action — the shell owns the URL). A
configurable page has one form and one Save & Apply action:

```json
{ "type": "form", "submit": "Save & Apply", "success": "",
  "fields": [ /* field and list widgets */ ] }
```

A contribution fragment must not emit `form`; emit its field-bearing `section`,
`stack`, or other root directly. The shell places those fields inside the shell
page's outer form and coordinates its Save & Apply with every other changed owner.

`submit` currently defaults to `"Save"` for manifest-v1 pages. Manifest-v2 pages
should state `"Save & Apply"` until the renderer changes its default. Two forms
get no generated button: one with `"style": "page"`, whose submission the shell's
staged-changes bar owns, and one whose `fields` carry a `confirm`, which renders
the submit itself — state a `submit` label explicitly if you want both.

**Secondary actions.** Besides Save & Apply, a standalone form may declare `actions` — extra buttons
that submit the form (all its fields) with an `_action` marker you read in your
handler, so you can *compute* on the submitted values and re-render, without a save.
This is the plugin-computed round-trip (ADR-005 §7): the shell renders the button and
forwards the submission; you do the work and return fresh schema. The WireGuard
plugin uses it to generate a keypair — the shell can't compute a WireGuard key, so
the plugin does, fills the field, and re-renders; the operator then uses Save & Apply.

```json
{ "type": "form", "submit": "Save & Apply",
  "actions": [ { "label": "Generate keypair", "action": "generate-keypair" } ],
  "fields": [ /* … */ ] }
```

On a POST, read `_action`: when it names one of your actions, compute and re-render
(return no `commit`); otherwise treat it as Save & Apply's prepare request.

### field — one labelled control

```json
{ "type": "field", "name": "hostname", "label": "Hostname",
  "kind": "text", "value": "OpenWrt", "datatype": "hostname",
  "error": "", "help": "The device's hostname." }
```

- `kind`: `"text"` (default), `"select"`, `"checks"`, `"password"`,
  `"textarea"`, or `"datetime-local"`.
- `value`: the current value; echo the submitted value back on a failed POST.
- `datatype`: a datatype name the shell enforces (see [Datatypes](#datatypes)). Optional.
- `autocomplete`: optional browser autofill purpose. Password fields default to
  `new-password`; use `current-password` only for the existing credential.
- `error`: an inline error to show under the field (you set this on a 422).
- For `kind:"select"`, supply `options` and set `value` to the selected one:

```json
{ "type": "field", "name": "zonename", "label": "Timezone", "kind": "select",
  "value": "UTC",
  "options": [ {"value":"UTC","label":"UTC"},
               {"value":"Europe/Ljubljana","label":"Europe/Ljubljana"} ] }
```

- For `kind:"checks"` — membership in a known set (checkboxes; per the control
  vocabulary, checks mean "include this one", a switch means on/off state).
  Supply `options` and put the checked ones in `values`; all boxes share `name`
  and post as a multi-value field, the same contract as `list`. Use it wherever
  the valid values are enumerable — a zone's networks, protocols, days — so a
  typo'd dead reference is untypeable:

```json
{ "type": "field", "name": "network", "label": "Networks", "kind": "checks",
  "values": ["lan", "lan2"],
  "options": [ {"value":"lan","label":"lan"}, {"value":"lan2","label":"lan2"},
               {"value":"guest","label":"guest"} ] }
```

### switch — one persistent on/off setting

Use a switch for binary object state, such as whether a firewall rule is enabled.
It is the same compact control used in table toggle columns; checked switches post
their `name` with the browser's standard `on` value, while unchecked switches omit it.

```json
{ "type": "switch", "name": "enabled", "label": "Enabled", "on": true,
  "help": "Disabled rules remain configured but are not evaluated." }
```

For object-level state, a section may place an inline switch in its heading with
`control`; set the switch's `style` to `"inline"` and provide `off_label`. The
state label follows the switch and changes with it. This keeps identity and state
together without turning the switch into a form field row.

Style `"hero"` is the reassuring lead for a page's one big state — a VPN, a
guest network: a status beacon (`icon`), a headline reading the current state in
plain language (`label` on, `off_label` off), an optional `meta` sub-line, and a
larger switch. Pure CSS, like every style — the page leads with "it's on" rather
than a checkbox.

```json
{ "type": "switch", "style": "hero", "icon": "shield", "name": "vpn_on",
  "on": true, "label": "Your home VPN is on", "off_label": "Your home VPN is off",
  "meta": "2 of 3 devices connected" }
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

For compact sets such as ports, addresses, or protocol names, set
`"style":"tokens"`. Each token still posts as a repeated value under the same
name; `prompt` labels the trailing add field. Enter, comma, or leaving the field
commits a token, and every token can be removed independently:

```json
{ "type": "list", "name": "dest_port", "label": "Included ports",
  "style": "tokens", "prompt": "Port or range",
  "items": ["53", "67", "547"] }
```

> **The schema's edges are a finding.** A single value repeats with `list`; a group
> of fields that maps to uci sections repeats with
> [`repeater`](#repeater--a-repeatable-group-of-sections); a field-set appears on a
> toggle with [`conditional`](#conditional--a-field-set-behind-a-toggle); a value is
> *computed* by a form [action](#form--an-interactive-form) that re-renders. What
> still can't be expressed — a field-set gated on a multi-value `select` — is a
> *finding*: tell us, so the next behavioural widget is the right one.

### conditions — optional typed form blocks

A complete editor may support many independent conditions while any one object
uses only a few. Declare the entire catalogue once and mark the currently present
items `active`. The shell renders active blocks and owns the Add/Remove interaction;
each block contains ordinary widgets and therefore preserves their normal POST and
validation contracts.

```json
{ "type": "conditions", "label": "Conditions",
  "help": "Every condition must match; values inside one condition are alternatives.",
  "items": [
    { "key": "dest_port", "label": "Destination ports", "active": true,
      "children": [
        { "type": "list", "name": "dest_port", "label": "Include",
          "style": "tokens", "items": ["53", "67", "547"] }
      ] },
    { "key": "rate", "label": "Rate limit",
      "children": [
        { "type": "field", "name": "limit", "label": "Packets", "value": "1000" }
      ] }
  ] }
```

- `key` uniquely identifies an optional block within this builder.
- `active` means the backing object currently carries that condition.
- Removing a block removes its child controls from the submitted form; adding one
  inserts the declared controls with their supplied defaults.
- The plugin remains responsible for interpreting the resulting fields and for
  returning per-field validation errors on 422.

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

A **behavioural** widget: a field-set the shell shows when its toggle is on, with an
optional alternate field-set shown when it is off. You declare the branches and the
initial state; the shell owns the toggle and the show/hide, realized in **pure CSS**
(no JavaScript, no round-trip). The toggle posts its own value, so your handler can
read it to decide which branch to interpret (ADR-005 §7).

```json
{ "type": "conditional", "name": "use_psk", "label": "Use a pre-shared key",
  "checked": true,
  "fields": [
    { "type": "field", "name": "preshared_key", "label": "Pre-shared key", "value": "…" }
  ],
  "otherwise": [
    { "type": "text", "markdown": "No pre-shared key will be used." }
  ] }
```

- `name` — the toggle's form field name. It posts `name=on` when checked, nothing
  when unchecked; read it in your handler to gate the save.
- `label` — the toggle's caption.
- `checked` — whether the field-set starts visible; derive it from state (e.g. "the
  pre-shared key is set").
- `fields` — the field-set revealed when the toggle is on.
- `otherwise` — an optional field-set revealed when the toggle is off.

Hidden fields in either branch still submit their values (they are only visually
hidden), so decide from the toggle which branch to interpret and ignore the other.
(Gating on a `select` with several values is a later realization under the same
declaration.)

### raw — the governed bridge

Display-only Markdown, for **prose** — running text no widget shapes. Sanitised
(no HTML passthrough, dangerous URL schemes stripped) and rendered through
Verso's styling. **Never interactive** — no inputs, no forms. Its usage is
metered as the demand signal for the next widget; treat it as a temporary
bridge, not a home.

An outcome message, a machine value, or an empty state in `raw` is a defect,
not a style choice — each has an owner: the envelope's `notice` for outcomes,
`code`/`properties` for values, `empty` for nothing-here states.

```json
{ "type": "raw", "markdown": "WireGuard keeps a tunnel silent until traffic flows, so a peer can look idle while it is perfectly healthy." }
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

## A worked example: verso-plugin-system

The reference plugin (`verso-plugin-system`, Rust on the `verso-plugin` SDK)
manages the device's identity and clock: hostname, timezone, and time
synchronization. It speaks only the documented JSON contract — it shares no
code with the shell. Its whole shape:

- **manifest.json** places one entry under System → General, and declares
  `acl.read` and `acl.write` for the `system` config (plus the rpcd scope for
  its one apply action).
- **GET /** reads the system section and the timeserver section from the
  `X-Verso-UCI` snapshot the shell brokered (it links no uci library) and
  returns a page-style `form` of three `section`s — identity, time and region,
  synchronization — built from the SDK's typed `Widget` model. The NTP block is
  a `conditional`: time servers while automatic sync is on, a manual
  `datetime-local` field while it is off.
- **POST /** echoes the submission and returns `commit` intents for
  `system.@system[0]` and the timeserver section (created by `type` when
  missing), which the shell stages through rpcd. Manual time also returns the
  typed `set-system-time` `apply` action the shell performs after the stage
  applies. The plugin declares datatypes (`hostname`, `host`) and adds its own
  semantic checks (the timezone must come from its catalog; the manual datetime
  must be calendar-valid), returning the re-rendered form with inline field
  errors when one fails — the shell blocks the write (ADR-008).

It runs under procd (above); point the shell's `$VERSO_PLUGINS_DIR` at the
installed manifest and the page appears at `/plugins/system/`.

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
