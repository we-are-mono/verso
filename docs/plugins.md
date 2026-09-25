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

The bundled **Interfaces** plugin owns network, device, VLAN, bridge, and tunnel
configuration. Its manifest contributes one main navigation destination, with no
subpages. **System General** remains a bundled System plugin. On **Access**, the
shell owns password changes and login sessions; the System plugin contributes
SSH and uhttpd configuration through its `system_access: "/access"` route.
Only that contributor receives its own submitted form. Password submissions are
never forwarded to plugins.

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
| `nav[].icon` | optional Lucide glyph *name* for the row; the shell owns the artwork, and an entry naming none takes its section's glyph |
| `nav[].mode` | optional reading the entry belongs to: `"basic"`, `"advanced"`, or absent for both ([below](#basic-and-advanced-the-reader-mode)) |
| `contributions` | optional list of fragments inserted at published shell hooks |
| `contributions[].id` | contribution id, unique within the plugin; namespaces its fields |
| `contributions[].hook` | globally unique shell hook id |
| `contributions[].path` | socket endpoint used to render and prepare the fragment |
| `contributions[].order` | order within the hook; plugin id and contribution id break ties |
| `acl` | the rpcd access scopes you need ([below](#declaring-your-acl-scopes)): `read` to render config, `write` to change it |
| `acl.read[]` | one `{scope, object, function}` grant naming a config the shell reads and hands you as a snapshot |
| `acl.write[]` | one `{scope, object, function}` grant the shell checks before a POST |
| `entity_tabs` | optional: what this plugin has to say about a subject some other page lists ([below](#entity-panels)) |
| `entity_acts` | optional: this plugin's direct acts on such a subject, which open nothing |

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

## Entity panels

A device is dnsdhcp's reservation, the firewall's verdict on its traffic, and a
QoS plugin's shaping — three plugins, three separate processes, none of which can
call another. So the panel that shows one subject is the **shell's**: it asks each
live contributor for its tab and frames the answers together. A plugin declares
which kind of subject it has something to say about and under which *slot*; the
shell owns everything else — the frame, the pinned facts, the order tabs appear
in, the icon each slot wears in a listing's row, and which shortcut opens which
tab. A slot no live plugin claims draws nothing, which is how a board with no QoS
plugin simply has no limits tab, with no shell change anywhere.

```json
"entity_tabs": [
  { "entity": "device", "slot": "shape", "label": "Limits & schedule" }
],
"entity_acts": [
  { "entity": "device", "slot": "unreserve", "path": "/config/reservations/{id}/delete" }
]
```

A tab is answered on the reserved path `entity/<kind>/<id>` under your own mount,
GET to render and POST to save — the same request shape as a page, brokered reads
included. `{id}` in an act's path is the subject's own identity (a MAC, a uci
section name), substituted by the shell.

The envelope an entity request answers with is an ordinary one, read a little
differently:

- `title` names the tab.
- `widget` is its body. Give the form `style: "page"` and no `submit`: the panel
  draws the commit row, and the form posts back into the panel rather than
  navigating, so the drawer stays open over the listing that opened it.
- `cta` and `consequence` are that commit row's words — the verb, and what
  applying it costs. Only you know; a tab that stages nothing sets neither and no
  row is drawn.
- `state` is where the subject stands in a word or two — `"blocked"`, `"no
  limit"`, an address. The shell wears it as a chip beside the tab's label, so a
  panel with several tabs answers what it was opened to ask before a tab is
  chosen.
- `commit` stages as it does anywhere else; a submission you refuse answers 422
  with the offending controls marked and asks for no writes.

The subject of a panel is settled — the panel is headed with it — so do not draw
a control that could point your object at a different one. Carry it hidden
instead: the MAC of a device whose reservation is being edited is the panel's
subject, not a field.

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
  `"Complete"` beside a finished reference page's kicker.
- `tone` — optional; declares the title a message about now rather than a
  place-label: the shell tints the heading by the closed tone vocabulary
  (`info` | `success` | `warning` | `danger` | `neutral` — never a colour) and
  drops the " — <tab>" navigation suffix, both from the one word. `neutral`
  drops the suffix without a tint. The masthead states the page's current
  state — tint only what is true right now, on the render that makes it true.
  Anything outside the vocabulary is ignored.
- `ruled` — optional; ends the masthead in a hairline, so the title and lede
  are ruled off from the page's first section the way ruled sections are from
  each other. For a page built of ruled sections (a settings page); a listing,
  whose toolbar follows the heading with no rule, leaves it unset.
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
  than staged configuration: nothing on it stages, though the staged-changes
  chip still shows pending changes from elsewhere. It does not make a returned
  `commit` immediate—commit intents always stage.
- `commit` — optional; on a successful write, the uci changes for the shell to
  apply on your behalf (see [Writing config](#writing-config-the-commit-intent)).
  You never write config yourself.
- `width` — use `form` for a 640px settings page, `narrow` for 896px,
  `normal` for 1024px, or `wide` for 1152px. Layout uses the available width
  on smaller screens.
- `commands` — optional array containing one immediate, structured action.
  Supported names are `interface-restart`, `set-system-time`, `ssh-key-add`,
  `ssh-key-remove`, `certificate-generate`, and `certificate-install`. Commands
  require their exact declared write scope, a valid CSRF-protected POST, and a
  valid widget schema. They cannot be combined with `commit` or `apply`.
  Invalid credentials return a form error; private keys never appear in reads
  or previews. Configuration changes continue to use staged `commit` intents.
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

The query string never selects the page — it addresses something *within* the page
the path named, so a link from elsewhere in the shell can arrive with a row already
open (`/plugins/dnsdhcp/?reserve=<mac>` opens that device's panel). Read it beside
the path (`request.query`), and let a value that names nothing you have change
nothing: a link written for a device that has since left must still render the page.

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
| `dhcpState` | `{"networks": {"lan": {"enabled": true, "state": "running", "pool": "192.168.1.100–192.168.1.249", "lease_time": "12h", "leases": 12}}}` — DHCPv4 service observations from procd, generated dnsmasq ranges, netifd and unexpired leases. `enabled` comes from applied configuration, not the operator’s staged changes. States are `running`, `warning`, `stopped` and `disabled`; optional `reason` distinguishes failures, unavailable readings and full pools. Missing lease counts stay unknown. |
| `networkState` | `{"interfaces": [...], "devices": {...}, "protocols": {...}}` — netifd logical-interface dump, kernel-device state, and installed protocol handlers (when available). The shell adds physical-port and parent relationships from its existing topology reader. |
| `accessCredentials` | Public SSH key comments/fingerprints and public web-certificate metadata. No authorized-key options, private keys, or router passwords are included. |
| `firewallCounters` | `{"counters": [{"chain", "name", "packets", "bytes"}]}` — fw4's live nftables hit counters, one entry per (chain, rule name), summed across the several nft rules a single UCI section can render into. |
| `wirelessState` | `{"radios": {"radio0": {"up": true, "channel": 6, "txpower": 20, "busy": 61, "hardware": "NXP 88W9098"}}, "networks": {"default_radio0": {"up": true, "clients": 3}}}` — what the radios are doing now, keyed by `wireless` config section. netifd's `network.wireless status` says whether each radio and network is up and which kernel interface a network became; iwinfo reads that interface for the channel and transmit power (dBm) in use, the in-use channel's busy share (whole percent of active airtime) and the associated clients. A number that could not be read is absent, never zero. The operator's grant must cover `network.wireless status`, and `iwinfo info`/`assoclist`/`survey` for the numbers. |
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
one staged operation, so an "Add" drawer creates the row it promised. Supplying
both `type` and a nonempty `section` creates a **named** section instead, for
example `{"config":"network","type":"interface","section":"guest","values":{"proto":"dhcp","device":"eth1"}}`.
The name must be unused; the broker preflights named creates before staging the batch.

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

### Describing your changes (the `describe` hook)

The shell's staged-changes chip opens a review drawer that lists what is waiting
to be applied, by the page it belongs to. By default each pending change reads as its raw uci
line — `firewall: cfg0abc.enabled = 0` — which is honest but not plain. You can do
better for your own config: give `serve` a fourth argument, a **describe** hook,
and the shell will ask you to put your pending changes in words.

```rust
fn main() {
    verso_plugin::serve_described("firewall", get, post, describe);
}

fn describe(changes: &[Change], snapshot: &Snapshot) -> Vec<Description> {
    // one sentence per changed object; cover the changes it accounts for
    // e.g. Description::new("Turned off the rule “Block Telnet”.", vec![0])
}
```

When a Review drawer is about to render, the shell sends each owning plugin the
coalesced changes for the configs it declared — the same `X-Verso-UCI` snapshot a
render gets rides along, so you can resolve a section handle to the name a person
set. You answer with `Description`s: a `plain` sentence and the `covers` indices
(into the `changes` slice) it accounts for. One sentence may fold several changes
(a rename writes two options); a change you do not cover keeps its raw line, and a
plugin with no describe hook leaves every change raw. Describe is **always
optional** — the raw line is the always-present fallback, so describe what reads
well and leave the rest.

Each `Change` is one net change with a normalized `op`, so you never reason about a
raw tuple's length:

| `op` | meaning |
| --- | --- |
| `set` | `option` set to `value` on `section` |
| `add-section` | new `section`; `option` is its uci type |
| `remove-option` | `option` cleared from `section` |
| `remove-section` | `section` removed whole |
| `list-add` / `list-del` | `value` added to / removed from the list `option` |

A removed section is already gone from the injected snapshot (which reflects the
staged config), so a `remove-section` can be named only as far as its handle
allows — leave it to the raw line otherwise. The describe hook never writes and
never renders a page; it only turns changes into words.

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
drops both; the apply runs the action only after UCI apply.

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
heading treatment as an active item in the conditions editor. `open:true`
renders it already expanded — for content that is the page's focus right now
(an update's package manifest) yet still folds away once read.

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

A row's value is prose unless it declares otherwise, and the declaration also
tells the shell's localization what the value is: `mono` sets a machine string
in monospace, `chip` renders an entity identity, and `verbatim` marks data that
keeps sans type — a size, a rate, an uptime. Declare one of them on every value
that is data, not words; an undeclared value is treated as prose and translated
when a catalog covers it.

`variant` tones a value whose reading is also a verdict — `success`,
`warning`, `danger`, `info`, the same vocabulary a badge speaks. It names a
meaning, never a colour: the shell picks the ink, in both themes, and a word
outside the vocabulary tones nothing.

### actionbar — a listing's controls

The band between a page's heading and its listing: `filter` (the search's
placeholder), `tabs`, `select`, `live`, and `action`. Over a table it is the
listing's one filled surface — quiet sand between two hairlines, every control
1rem from its edges — and the table sits flush under it; a `live` log's bar is
the same controls, unfilled. `tabs` are the coarse cuts, drawn as one dropdown
beside the search with each option priced by its `count` ("IPv4 · 14"); name
the whole set in the first one's label ("All families", not "All"). `select` is
a second dropdown, hard right, for a facet the rows carry.

`action` is the page's one forward act. Declared on the bar that is the first
child of the page's stack, the shell lifts it onto the heading line, where every
listing keeps its primary; a bar left with nothing else on it is dropped. A
`quiet` act (one that takes something away, like a download) stays on the bar.
`opens_panel: true` makes the act open the listing's own panel blank (making
one is editing one that does not exist yet); `drawer` carries that blank panel
when the address asks for it (`?open=new`).

```json
{ "type": "actionbar", "opens_panel": true,
  "action": { "label": "Add network", "href": "/plugins/wireless/?open=new", "icon": "plus" } }
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

Every row is inset from the table's edges. That inset is what a row's hover tint
and a group band's fill — which run the table's full width — are held off by, so
a value never sits on the tint's own boundary. How far in follows how much
horizontal room the columns leave: **`dense: true`** pulls it from 16px to 8px,
for a listing carrying many columns. Rows keep their height either way — density
here is horizontal, and a shorter row is a rhythm no design asks for.

Every row is inset from the table's edges. That inset is what a row's hover tint
and a group band's fill — which run the table's full width — are held off by, so
a value never sits on the tint's own boundary. How far in follows how much
horizontal room the columns leave: **`dense: true`** pulls it from 16px to 8px,
for a listing carrying many columns. Rows keep their height either way — density
here is horizontal, and a shorter row is a rhythm no design asks for.

Column kinds, one treatment each (never mix them per row):

- `"text"` (default) — plain ink text.
- `"name"` — the row's identity (a zone, an interface): bold ink, nothing else —
  in a column where the type never varies, even an icon is noise.
- `"mono"` — verbatim machine strings only: addresses, ports, device names.
- `"keyword"` — closed-vocabulary words (`tcp`, `udp`, `icmpv6`): sans, muted.
- `"comment"` — optional free text such as a UCI `name`; muted, blank when absent.
- `"num"` — right-aligned tabular figures (counters); muted.
- `"meter"` — a share of something: a 6px bar filled to the cell's `fill`
  (0–100) with `text` as the figure beside it (`"61 %"`). `variant` bands the
  fill — `warning` marigold, `danger` crimson, anything else green — so the
  listing sets its own thresholds. A cell with no `text` is the faint dash.
- A `text` cell may carry a `chip`: the config value the word stands for
  (`"WPA3"` beside `sae`).
- `"toggle"` — an on/off checkbox; the cell carries `on` and an optional form `name`.
- `"pill"` — an enum value as a status pill; the cell carries `text` plus a
  `variant` from the badge vocabulary (`success`/`warning`/`danger`/`info`/
  neutral) — e.g. `accept`→success, `reject`→warning, `drop`→danger. An empty
  cell renders a faint dash, which is what keeps the pills meaningful.
- `"endpoint"` — one or more traffic endpoints; each is `{ "kind", "label" }`
  where kind is `"zone"` (sans + shield), `"device"` (mono address + screen),
  `"router"` (this device, accented), or `"any"` (muted globe).
- `"reorder"` — a drag handle, first column only; see **Reorderable rows** below.

A row's `id` is its stable handle — use the UCI section name.

A column may declare `"fit": true` to squeeze to its content's width instead of
sharing the table's slack, which collects in the growing columns — how a group
of related fact columns (a version pair and the arrow between them) huddles at
one edge instead of drifting apart. Pair it with kinds that do not wrap.

A column may instead fix its `width` at a measure named by what it holds, so a
column keeps its place when one row's value is shorter and every listing's
address column is the same width. The set is closed: any other word (a CSS
length included) fails the decode. A column that states none grows, sharing
what is left.

| `width` | holds | measure |
| --- | --- | --- |
| `mark` | an order number, a grip, an icon | 2rem |
| `count` | a counter or a flag: hits, packets, yes/no | 4.5rem |
| `short` | a short token: port, protocol, PID, size, a verdict, row acts | 6rem |
| `word` | a state or a chip: status, zone, version | 9rem |
| `address` | an address: IPv4 with its prefix, a MAC, address:port | 12.5rem |
| `name` | a name, or a short list of them: rule, zone, networks | 14rem |
| `long` | a long identity: a service, a package | 17rem |

In the Rust SDK it is the `ColumnWidth` enum.

**Reorderable rows.** A listing whose *sequence* is meaning — evaluation order —
adds a leading `{ "kind": "reorder" }` column and names the uci config its rows
are sections of in `reorder_config`. The shell owns the interaction end to end:
it draws the handles, drags the row, and — the moment the row lands — stages a
`uci order` on that config, like every other write, so the review drawer's
apply is what makes it live. Your plugin ships no drag logic and never sees the
reorder POST; it renders the page again from the fresh snapshot.

A drop is a save. The reorderable table renders **the page's form** beside
itself, carrying the config and the row sequence as hidden fields, and the shell
posts it where it stands when a row lands, so the new order counts in the
staged-changes chip like any other change and shows in the review drawer.
`reorder_label` names the order in the operator's words there ("Rule order");
left empty it reads as the shell's generic "Order".

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

**Lanes.** A row's `group` opens a lane: a row of the listing's own height,
unfilled, over a strong hairline, with `label` (→ `to`), an optional mono
`chain` and a worded `tally`. Every lane after the first stands 2.5rem off the
one before, so each reads as its own small table. `add_href` puts the lane's add
hard right: `add_text` is its words ("Add rule"), `add_label` its tooltip naming
the lane ("Add rule to WAN → Router"), and `add_panel: true` opens the listing's
panel in place, seeded by the href.

**Direct row action.** A `pill` cell may carry a compact immediate action instead
of a state. Set `button`, `action`, `confirm_title`, and `confirm`; the shell opens
its standard alert dialog and posts `_action=<action>` back to the page only after
the operator confirms. Use this for a command with no edit surface, such as ending
a login session.

```json
{ "button": "End session", "action": "end-session:iphone",
  "confirm_title": "End this session?",
  "confirm": "Anyone using this session will be signed out of Verso immediately." }
```

An individual icon in an `actions` cell can also carry `confirm_title` and
`confirm`. Only that action opens the dialog; its neighboring toggle or edit
action keeps its usual behavior. The shell substitutes the row's name for `%s`
in the title after translation. On confirmation it posts the action's `name`
and `value` to the listing. Identify the removal target in that value rather
than the `open` query parameter, which may still name a different editor.

```json
{ "actions": [
  { "icon": "trash-2", "title": "Remove network",
    "name": "_remove", "value": "guest",
    "confirm_title": "Remove network “%s”?",
    "confirm": "Devices using this network will disconnect when the change is applied." }
] }
```

**Row drawer.** A row with a `drawer` opens a right slide-in panel on the object
behind the row, beside the listing it belongs to: its facts as `properties`, a
callout naming what depends on it, and the form that edits it. Keep removal on
the listing's trash action with a focused confirmation, rather than duplicating
it beneath Save in the editor. **An object lives in its
drawer.** The listing's add opens the same panel blank (`add_panel`, or a link
such as `?open=new`), so making one and editing one are one surface drawn from
one set of fields, and a Save inside the panel stages like any other write (see
[Writing config](#writing-config-the-commit-intent)). Keep which panel is open, and which of its
tabs, in the address (`?open=<section>&tab=<name>`): the panel then survives a
reload, a back button and a shared link without client state. The row gets a
trailing chevron and the pointer; controls inside the row (toggles) keep their
own meaning. Set `hide_title:true` when the selected row
and first section already establish the panel's identity; the title remains
available to assistive technology and the close control remains visible. That
first section may set `flush:true` so the drawer supplies the outer top inset
instead of stacking two layers of padding.

Set `open:true` to render the panel already open — how a link from elsewhere in
the shell arrives with the row's panel already in front of the operator
(`/plugins/dnsdhcp/?reserve=<mac>` opens that device's panel).

```json
{ "id": "lease-iphone", "cells": [ /* … */ ],
  "drawer": { "title": "iPhone — 10.0.0.23",
              "children": [ /* properties, callout, form */ ] } }
```

**Seam.** A table may fold extra rows behind a collapsed block *inside the same
card* — e.g. the stock rules a fresh install ships with, present and honest but
not carrying the page:

```json
{ "type": "table", "columns": [ /* … */ ], "rows": [ /* your sections */ ],
  "seam": { "summary": "OpenWrt defaults — 9 stock rules that ship with a fresh install",
            "rows": [ /* the folded sections, same cell shapes */ ] } }
```

**Nothing to list.** A table with no rows at all — none of its own and none
folded — renders neither `<thead>` nor rows: column headings describe data, and
over no data they are chrome. In their place the shell draws one row carrying
`empty_text`, where the first row would sit — a row's height, meta words, the
usual hairline. Leave `empty_text` unset and it reads "Nothing here yet" in the
operator's language; set it and say something true about *this* listing, in two
sentences at most: that nothing is here yet, and what the daemon does without it
("No extra names yet — reserved devices already answer by name."). Tell a
filtered nothing ("No package matches …") apart from a nothing-yet.

```json
{ "type": "table", "columns": [ /* … */ ], "rows": [],
  "empty_text": "No reserved addresses yet — reserve one from a device on the Leases page." }
```

Every listing says its nothing this way, whether it is one section among several
or the whole page: no illustration and no button in the row — the toolbar's
primary action is the way in. The `empty` widget is for a state that is not a
listing at all (a takeover's phase, a page waiting on a precondition).

**A live listing (`stream`).** Some listings are not a state of the config but a
run of events — firewall verdicts, DHCP handshakes, the system log. Such a table
declares `stream` and renders with no rows: the rows arrive afterwards, newest
on top, over a connection the shell holds open.

```json
{ "type": "table", "dense": true,
  "stream": { "source": "firewall-log", "ring": 200 },
  "columns": [ /* the columns the events arrive under */ ],
  "rows": [],
  "empty_text": "Waiting for the first logged event…" }
```

`source` is a name from a **closed set**, never a URL — a plugin cannot point
the shell at an endpoint of its choosing, the same bound the brokered helper
reads keep. The set today is:

| source | what arrives |
| --- | --- |
| `firewall-log` | one row per logged firewall verdict: when, verdict, ingress zone, source, destination, protocol, port, and the rule that decided it |

A source the shell does not serve is not an error: the table renders as an
ordinary still listing, so a page written against a newer shell still works on
an older one. (A dev shell logs the unwired source, since silence is the bug
there.)

`ring` is how many rows the browser keeps before the oldest fall off the bottom
— 200 by default, clamped to 20…500. Everything else is the shell's:

- **The transport.** One Server-Sent Events connection per listing, behind the
  same session gate as every page, carrying structured rows — never markup.
- **The waiting state.** A live listing keeps its column heads while empty (they
  name what is about to arrive) and states `empty_text` in one quiet row that
  the first event replaces.
- **Repeats.** Consecutive identical events collapse into the row already there:
  its counter climbs (`× 38`) and its clock moves up. A repeat that is *not*
  consecutive starts a fresh row, so the order never lies.
- **Pause.** The action bar's `live` control is the stream's own indicator and
  its pause, and stands on the heading line with the bar's act: it reads `Live`,
  its spinner turning, while events flow, `Paused · N new` while they are
  held (its title says the act, Pause or Resume), `Connecting…` while the
  stream is down, and nothing on the page moves — not even the relative
  times — until it is pressed again.
- **Narrowing.** Every value in a streamed row is a control: click a verdict, a
  zone, or an address and the listing narrows to it, with what was clicked
  travelling to a shelf above the table wearing the treatment it had in the row.
  The page-wide `filter` composes with it (the lens dims, a plucked value
  hides), so a live listing should declare one.

A plugin writes none of that, and none of it is configurable: it declares the
source, the ring, the columns, and the words.

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

A tab may carry `"mode"`, filtered exactly as a manifest `nav` entry is
([below](#basic-and-advanced-the-reader-mode)).

### Basic and Advanced — the reader mode

Verso has one app-wide reader mode, `basic` (the default) or `advanced`, chosen
by a single switch in the shell chrome (ADR-015). Expertise is a property of the
reader, not of a page: you never own the switch, read the mode, or filter
anything yourself. You **declare** which reading your content belongs to and the
shell filters at render.

- a `section` carries `"mode": "basic" | "advanced"` — absent means both,
- a `field` carries `"advanced": true` — absent means both,
- a `nav` entry and a `pages` tab carry the same `"mode"` as a section.

```json
{ "type": "section", "title": "Time synchronization", "mode": "advanced",
  "children": [ /* the servers, the hand-set fallback */ ] }
```

A `"basic"` section is the *simplified face* of what its `"advanced"`
counterpart states in full — the mode picks exactly one of them, so no reader
ever sees the same fact twice.

**Mode hides capability, never state.** A tag hides what somebody *could* do,
never what is already done, and you are the only party that knows your defaults:

- Drop `advanced` from a field whose value differs from its default. Tags travel
  per render, so this is a per-render decision. The bundled firewall shows the
  shape — its rule editor tags the address family only while the rule matches
  both families, so a rule someone narrowed says so in every reading.
- An advanced-only section is honest only while its contents are at their
  defaults, or while a paired basic-mode section represents the same facts. A
  section hiding live configuration makes basic mode lie about the router.
- A form submitted in basic mode omits its hidden advanced fields, and you treat
  each absent field as its default — the same "absent means default" convention
  as everywhere else. Because live values are never hidden, absence genuinely
  means default.

The shell's filter is deliberately mechanical — drop what is tagged for the other
reading, and drop a container the filter emptied so no heading outlives its
contents. Everything untagged renders identically in both modes. Mode is never a
privilege boundary: a direct URL to an advanced page renders in basic mode, and
every write stays gated by ADR-007 whatever the reader's mode.

In the Rust SDK:

```rust
Widget::section("Time synchronization", "", children).in_mode(MODE_ADVANCED)
Widget::select("family", "Address family", &value, options, "")
    .advanced_when(value.is_empty())   // tagged only while it is at its default
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

**Whether it renders is the shell's call, not yours.** Before rendering a page
the shell counts its filterable entries — every table row, folded ones included,
plus every settings option row, folded ones included — and **removes** the
filter when that count is 20 or fewer: a page you can read in one glance is not
helped by a search box over it, and a lens with nothing to sift is a control
answering a question nobody asked. Removal, not concealment: no dead dock, no
"/" shortcut into nothing. So declare the filter your page would want and carry
no threshold of your own — a page's own count changes as its config does, and
the rule that decides is one rule for every plugin.

### settings — a card of option rows

The "config defaults" pattern: each row a plainly-named option with a one-line
description, the underlying option name as a mono code chip, and its state on
the right — a `toggle` (an on/off checkbox) for an on/off option, or `pills`
(badge vocabulary) for a row that reads rather than toggles.

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

**Seam.** A block may fold its long tail of rare options behind a collapsed line
inside the same block, so the everyday rows carry it:

```json
{ "type": "settings", "items": [ /* the everyday rows */ ],
  "seam": { "summary": "5 more options", "items": [ /* the rare ones */ ] } }
```

A seam holding fewer than **three** rows draws no fold: the summary line
occupies the height one hidden row would, so under three the fold saves nothing
and charges a click for it — the shell renders those rows on the block, with
their controls and their place in the page form intact. Declare your seam as you
mean it; the rule is applied at render, so a block whose tail grows past two
starts folding on its own.

### form — a standalone page form

On a standalone plugin page, renders its `fields` inside a `POST` form that submits
**back to the same page** (you don't set an action — the shell owns the URL). A
configurable page has one form and one Save action, which stages what the form
holds; the operator applies from the staged-changes chip (ADR-010):

```json
{ "type": "form", "submit": "Save", "success": "",
  "fields": [ /* field and list widgets */ ] }
```

A contribution fragment must not emit `form`; emit its field-bearing `section`,
`stack`, or other root directly. The shell places those fields inside the shell
page's outer form and coordinates its Save with every other changed owner.

`submit` defaults to `"Save"`; a `"style": "page"` form that states none gets
the shell's "Save changes". A form whose `fields` carry a `confirm` renders the
submit itself — state a `submit` label explicitly if you want both.

**Secondary actions.** Besides Save, a standalone form may declare `actions` — extra buttons
that submit the form (all its fields) with an `_action` marker you read in your
handler, so you can *compute* on the submitted values and re-render, without a save.
This is the plugin-computed round-trip (ADR-005 §7): the shell renders the button and
forwards the submission; you do the work and return fresh schema. The WireGuard
plugin uses it to generate a keypair — the shell can't compute a WireGuard key, so
the plugin does, fills the field, and re-renders; the operator then saves, and
applies from the staged changes.

```json
{ "type": "form", "submit": "Save",
  "actions": [ { "label": "Generate keypair", "action": "generate-keypair" } ],
  "fields": [ /* … */ ] }
```

On a POST, read `_action`: when it names one of your actions, compute and re-render
(return no `commit`); otherwise treat it as the save's prepare request.

### field — one labelled control

```json
{ "type": "field", "name": "hostname", "label": "Hostname",
  "kind": "text", "value": "OpenWrt", "datatype": "hostname",
  "error": "", "help": "The device's hostname." }
```

- `kind`: `"text"` (default), `"select"`, `"checks"`, `"password"`,
  `"textarea"`, `"time"` (a clock time on its own — a curfew's edge), or
  `"datetime-local"`.
- `value`: the current value; echo the submitted value back on a failed POST.
- `datatype`: a datatype name the shell enforces (see [Datatypes](#datatypes)). Optional.
- `autocomplete`: optional browser autofill purpose. Password fields default to
  `new-password`; use `current-password` only for the existing credential.
- `advanced`: keeps the field out of the basic reading — set it only while the
  field is at its default ([the reader mode](#basic-and-advanced-the-reader-mode)).
- `error`: an inline error to show under the field (you set this on a 422).
- `key`: the option this field writes, verbatim — `"ipaddr"`, `"leasetime"`. It
  rides beside the label as a mono chip, so someone who knows the config sees
  which line they are editing and someone who does not can ignore it.
- `unit`: what the number in the box is counted in — `"Mbit/s"`, `"seconds"`. It
  sits inside the field's trailing edge, so the value and what it means read as
  one thing and the label is left to say what the setting is.
- `style: "reveal"` on a `password` field: a shared secret someone reads out — a
  Wi-Fi key. The `value` is in the field, masked, with an eye beside it that shows
  it. A password field without this style never reflects its value.
- `style: "locked"`: a value fixed once written — a network's mode, a radio's
  band. The row keeps its `label` and `key`; `value` is shown, said plainly
  (`"Access point"`, translated like a label), in a quiet box with a padlock,
  and `help` is the reason it cannot change. It posts nothing.
- `style: "segmented"`: draws a `checks` field as one strip of togglable chips
  rather than a grid of boxes — for a set short enough to show whole (the days of
  the week), where which members are on is a shape rather than a list to read. A
  set long enough to wrap belongs in the grid. On a `select` it draws the pick as
  one strip of radios — only for a policy triplet (accept | reject | drop), where
  every answer is one short word and showing all three costs less than hiding
  two. Anything else — a protocol, a family, ECN, a list that can grow — stays a
  box. The segment in force fills in the body ink, as every selected segment in
  the shell does; the action colour is never a "selected" colour.
- `tip` and `source`: what the field *is*, raised from the label on hover or
  focus, closed by a mono line pairing `key` with what reads it. Use it where
  the label is a term of art the operator did not choose — a DUID, an interface
  identifier — and leave it off where the label is already the plain word for
  the thing. `help` still says what to put in the control and stays on screen;
  `tip` answers "what even is this", which would shout beside every row:

```json
{ "type": "field", "name": "duid", "label": "DUID", "key": "duid",
  "help": "Match a DHCPv6 client by DUID instead of MAC.",
  "tip": "Asking for an address over IPv6, a device names itself by a DUID …",
  "source": "dhcp host" }
```

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
The shell draws it as an 18px checkbox, never as a toggle: every state in Verso is
staged for the commit, nothing flips live, and a toggle's knob would promise that
it had. It is the same compact control used in table toggle columns; checked
switches post their `name` with the browser's standard `on` value, while unchecked
switches omit it.

```json
{ "type": "switch", "name": "enabled", "label": "Enabled", "on": true,
  "help": "Disabled rules remain configured but are not evaluated." }
```

For object-level state, a section may place an inline switch in its heading with
`control`; set the switch's `style` to `"inline"` and provide `off_label`. The
state label follows the switch and changes with it. This keeps identity and state
together without turning the switch into a form field row.

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
- `key` — the option the toggle writes, worn as the mono chip a field's label
  wears. The gate is a form row like the ones it gates, at the same measure.
- `help` — the line under the toggle: what turning it on means.
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

## Translations

Write only English. The shell localizes your envelopes for you (ADR-012): the
English source string is the catalog key, and translation happens in the
shell's render walk — your plugin never calls a translation function, in any
language, with or without the SDK.

To ship a language, drop a catalog beside your manifest:

```
my-plugin/
  manifest.json
  i18n/
    sl.json        ← {"Rules": "Pravila", "Add rule": "Dodaj pravilo", …}
    de.json
```

The file's basename is the base language code (`sl`, never `sl-SI`), the keys
are your English source strings exactly as your envelopes emit them, and the
catalog installs and uninstalls with your package — prose and translation stay
version-locked. A key the catalog misses renders your English, per key; shell
widget defaults (Save, Details) and the section your nav entries join are
translated from the shell's own catalog, so a plugin with no catalog at all
still sits in an otherwise localized page. Community translations are pull
requests against your repository, not files someone installs beside your
plugin.

**What your catalog covers.** The shell translates your envelope's prose: the
page `title` and `subheading`, `pages` and manifest `nav` labels, and every
prose field of every widget — section titles and subs, field labels, help and
placeholder text, option labels, form submits, callout and empty-state bodies,
table column labels and prose cells, confirm messages, `raw` Markdown. It
deliberately leaves your data alone: entity chips and tags, machine-kind table
cells (`name`, `mono`, `keyword`, `num`, …), `mono`/`chip`/`verbatim` property
values, hrefs, icons, form field names, and datatype validation messages
(shell-owned). Your catalog holds prose keys only.

**Keep data out of prose.** A string assembled from English and data —
`"Delete “" + name + "”?"`, `"3 more options"` — makes a new key every render,
so no catalog can ever match it: it stays English in every language. Emit
fixed sentences, and carry the data in the fields built for it (a cell in a
machine-kind column, a `verbatim` property, a chip). Where a property value
can be either words or data, declare the data (`mono`, `chip`, `verbatim`),
so a device named "Online" can never come back translated into words. Select
option labels are translated as prose; an identity in an option (a zone, an
interface) misses the catalog and passes through unchanged.

**Checking coverage.** A dev shell started with `VERSO_I18N_RECORD=1` logs
every string that falls back to English as it renders, labeled with your
plugin id — browse your pages in your target language and the log is your
catalog's to-do list.

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

### Conditional fields and interface editors

`when` groups fields belonging to a select value:
`{"type":"when","name":"proto","value":"static","active":true,"children":[...]}`.
`value` may list several values space-separated (`"sae sae-mixed psk2"`); the
branch shows while the select holds any one of them. The shell hides and disables inactive branches and skips them during datatype
validation; the plugin must still validate the selected protocol and its values.

A list with `style:"rows"` renders ordered values with individual remove controls
and an always-visible Add input. Enter adds a value, invalid tokens are refused,
and duplicates clear the input without adding a second row.

A table row can supply `depth` and `expanded` widgets. Only the identity control
expands those details. Expanded content participates in localization, filtering,
and schema traversal. Row actions keep their own links or submitted verbs.

`grid` style `editor` lays out a 640px interface form beside an anchor rail and
configuration preview, collapsing to one column when space is limited. Reuse
`link` style `rail` and a live `code` preview in that column. A chooser drawer uses
`link` style `choice`, with optional `desc` prose and a `code` label. Stored keys
and certificates use `card` style `artifact`; form sections remain unboxed.

The Interfaces inventory uses `table` style `interfaces`: its six columns are
identity, device type, IPv4 address, MAC, operational state, and actions. Identity
cells use `lead_icon:"physical"` or `"software"` for the topology mark and `chips`
for logical network references. State remains plain text with a square mark.
`depth` describes a transport dependency (VLAN or PPP), not bridge membership.
The matching `actionbar` style supplies the physical/software legend and a single
optional Problems filter. A chooser uses drawer `size:"choices"`, kicker
sections, and icon-bearing choice links. Expanded details use `grid` styles
`facts` and `configurations`; an editor rail may use `card` style `preview` with
an unlabeled live code block.

`verso.networkState` brokers normalized netifd booleans and kernel device facts.
`interface-up`, `interface-down`, and `interface-restart` commands each require
the matching `network.interface` write grant and are checked against the
operator's concrete netifd object permissions. They affect runtime only; UCI
`auto` stays in the staged editor. The shell refreshes the inventory without
resetting its filter, expanded row, or the session's inactivity deadline.

### DNS settings and custom option files

The DNS/DHCP plugin serves one settings page at `/plugins/dnsdhcp/`. Interface
DHCP pools are edited by the Interfaces plugin; reservations remain device
contributions. `dhcp` sections of type `verso_defaults` hold the reservation-only
default for new networks and the ordinary upstreams retained while encryption is
selected. Existing pools are never rewritten by that default.

The shell brokers `verso.dnsState` for installed DNS capabilities and custom file
contents. The `config-file-stage` command takes `path`, `expected` (the version
returned by the read), and `content`; its manifest must declare both
`verso.stageConfigFile` and write access to `dhcp`. The helper accepts only
`/etc/dnsmasq.conf` and safe `.conf` basenames under `/etc/dnsmasq.d/`, rejects
symlinks and stale edits, and checks dnsmasq syntax before staging. Include files
also stage dnsmasq's `confdir`, which its jail must mount.

File changes join the existing review/apply/discard flow as `file` review records,
not UCI options. The root helper keeps the shared stage and rollback journal in
`/var/run/verso-config-files`; saving never rewrites an active file. Apply preserves
original contents and permissions, validates the complete include set, and arms
an independent rollback timer. Confirm keeps the files, discard removes their
stage, and an unsuccessful UCI apply restores them. A helper restart recovers
expired rollback journals. Files are limited to 32 KiB each and 128 KiB of staged
text in total.

The shell's reusable `settings` grid and form styles provide a section index,
256px controls, and a Save button enabled by edits. `checkbox` switches, `code`
fields, `path`/`count` table columns, and `form` drawers keep their rendering in
Tailwind templates owned by the shell.


DHCP service visibility is shared by the Interfaces and DNS & DHCP plugins via
`verso_plugin::dhcp`. The SDK merges applied observations with each plugin's
staged snapshot, labels pending enable/disable changes explicitly, and provides
links to `/plugins/interfaces/edit?network=…#dhcp-server`. The interface identity
carries one DHCP chip per configured network: success, warning or danger, with a
neutral disabled state. The DHCP broker obtains protected include directives through a bounded helper read; handwritten exclusions are included in its service assessment. Chip `title` is localized independently of its verbatim
identity label. Expanded network facts show the observed pool, lease duration and
active lease count. Physical bridge members do not inherit their bridge's chip.

Table style `live` uses the shell's existing ten-second inventory refresh to
replace only its rows. It preserves the surrounding settings form, skips hidden
pages and open dialogs, and shows a stale-data notice when a read fails. The DNS
network summary uses this style; configuration still belongs to the interface
editor and follows the normal Save/Apply flow.
