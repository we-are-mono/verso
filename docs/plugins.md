# Writing a Verso plugin

This is the author's guide. If you can serve HTTP on a unix socket and emit JSON,
you can write a Verso plugin — in any language, without touching the shell.

> **Status:** grows with the widget set; anything marked _(landing)_ is not yet
> renderable. The authority for design decisions is
> `docs/ADR/006-plugin-contract.md` (mechanics), `docs/ADR/026-slots.md`
> (where a plugin places things) and `docs/ADR/005-ui-consistency-contract.md`
> (what you may emit).

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
configuration, and the static routes (`config route`, `config route6`) in the same
file. Its manifest files two pages under Network: Interfaces (`/`) and Routes
(`/routes`). **System General** remains a bundled System plugin. On **Access**, the
shell owns password changes and login sessions; the System plugin contributes
SSH configuration and the certificate Verso serves HTTPS with through its
`system_access: "/access"` route.
Only that contributor receives its own submitted form. Password submissions are
never forwarded to plugins.

## The manifest

Ship a `manifest.json`. The shell discovers it by globbing the plugins directory
(`$VERSO_PLUGINS_DIR`, default `/usr/share/verso/plugins/<id>/manifest.json`).

```json
{
  "id": "ntp",
  "name": "Time synchronization",
  "socket": "/var/run/verso-plugin-ntp/ntp.sock",
  "schema_version": 1,
  "contributes": [
    {
      "id": "clock",
      "slot": "shell.overview.cards",
      "path": "/slots/clock",
      "position": "after:system"
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

Spell each key once in an object. The shell matches a key to its field in any
case, and a manifest spelling one twice (`"secrets"` and `"Secrets"`, or
`"acl"` twice) is not read: the shell serves no page of it and takes none of
its secrets, and neither does the helper. Nor is one nesting a value deeper
than 127, or escaping half a surrogate pair (`"\ud800"` alone).

| field | meaning |
|---|---|
| `id` | stable, URL-safe; mounts your plugin at `/plugins/<id>/` |
| `name` | display name |
| `socket` | absolute path your process listens on, in a directory of your own (see [Your account and your socket](#your-account-and-your-socket)) |
| `schema_version` | the widget vocabulary you emit (currently `1`) |
| `nav` | optional list of menu entries; one plugin may place several standalone pages |
| `nav[].section` | which shell nav group the entry appears under |
| `nav[].label` | the nav link text |
| `nav[].path` | page path, relative to your mount (`/` = your index) |
| `nav[].icon` | optional Lucide glyph *name* for the row; the shell owns the artwork, and an entry naming none takes its section's glyph |
| `contributes` | optional list of things this plugin puts in named slots ([below](#filling-slots)) |
| `contributes[].id` | lowercase letters, digits and hyphens, unique within the plugin; `<plugin id>.<id>` names it everywhere else |
| `contributes[].slot` | the slot's name: `shell.<page>.<place>` for a shell page's, `<plugin id>.<name>` for one a plugin opens |
| `contributes[].path` | the endpoint on your socket the shell GETs for it, and posts its form to |
| `contributes[].position` | `first`, `last` (the default), `before:<anchor>` or `after:<anchor>` |
| `contributes[].order` | order among contributions at one place (default 100); plugin id, then contribution id, break ties |
| `system_access` | read as a contribution to `shell.access.blocks` standing `before:web`; prefer `contributes` |
| `acl` | the rpcd access scopes you need ([below](#declaring-your-acl-scopes)): `read` to render config, `write` to change it |
| `acl.read[]` | one `{scope, object, function}` grant naming a config the shell reads and hands you as a snapshot |
| `acl.write[]` | one `{scope, object, function}` grant the shell checks before a POST |
| `entity_tabs` | optional: what this plugin has to say about a subject some other page lists ([below](#entity-panels)) |
| `entity_acts` | optional: this plugin's direct acts on such a subject, which open nothing |
| `secrets` | optional: the options your config keeps a secret in, as `config.option` in any section (`"ddns.password"`), each in a config your `acl` writes with scope `uci`; one in any other config, one your plugin only reads included, is not taken for a secret. Config history (ADR-021) records that they changed and never their values, and takes any it kept before out of the whole history. OpenWrt's own (`network.password`, `wireless.key`, `wireless.sae_password` and the like) are covered already; name any other your plugin writes |

To place several pages in the menu, add more entries — each is grouped under its
own `section`:

```json
"nav": [
  { "section": "System", "label": "General", "path": "/" },
  { "section": "System", "label": "Time",    "path": "/time" }
]
```

An entry filed under `Network` or `System` is not a menu row of its own: those two
sections are each one row, named for the section, that opens into every page filed
there, whichever plugin files it. Network's pages run Interfaces, DHCP, DNS, Routes, then
the rest by label; System's run General, then the rest by label, then the shell's
own pages. A page in either section keeps its own title as its heading, with no
section prefix. Any other section's entries are rows of
their own (Wireless files under `Wireless` to keep one).

These entries are registrations, not suggestions layered over a shell page list.
For example, the bundled System plugin creates **System → General** solely through
the first entry above. The shell contributes its own System pages separately and
does not hardcode General. A registration appears while the plugin socket is live;
stopping the service withdraws it on the next page render, while its direct
`/plugins/<id>/…` URL remains available to show the contained unavailable state.

A destination that opens into subpages (the envelope's `pages`) says so with
`"pages": true` on its entry, so its menu row points the way it opens on every
page, not only once someone is in it. Which subpages they are stays the
envelope's to say:

```json
"nav": [
  { "section": "Security", "label": "Firewall", "path": "/", "pages": true }
]
```

A manifest declares at least one of `nav`, `entity_tabs`, `entity_acts` and
`contributes`. A plugin that only contributes has no sidebar row.

### Filling slots

A **slot** is a named place on a page. The shell's pages open slots, and a plugin's
page can open them too. You fill one from your manifest, and the shell asks your
plugin for the piece each time it draws the page the slot stands on. Neither the
shell nor the plugin whose page it is needs to know who you are (ADR-026).

```json
"contributes": [
  { "id": "source", "slot": "system.certificate.source", "path": "/slots/source" },
  { "id": "use",    "slot": "system.certificate.acts",   "path": "/slots/use" },
  { "id": "card",   "slot": "shell.overview.cards",      "path": "/slots/card",
    "position": "after:system" }
]
```

**Names.** A slot's name says who opened it:

- `shell.<page>.<place>` is a shell page's. They are listed in the SDK's
  `SHELL_SLOTS` and in `plugins/verso-plugin-sdk/testdata/slots.json`, with each
  slot's kind and anchors. That list is the promise: a name in it keeps working, and a
  renamed one keeps its old name as an alias.
- `<plugin id>.<name>` is one a plugin's page opens. The opening plugin's README says
  which it opens. The shell adds the opener's id itself, so no plugin can open a slot
  in another's name.

The id `shell` is reserved, and no plugin may take it.

**Kinds.** Every slot takes one kind, and the shell draws only an answer of that kind:

| kind | answer with | it stands |
|---|---|---|
| `block` | what a page holds: a `section`, `card`, `table`, `form`, a `stack` of them, a `collection` | as a block of the page |
| `act` | one `link` with an `icon`; `panel: true` opens your page in a drawer | after the host's own acts in its act row |
| `fact` | one `properties` | its rows after the host list's own, in the same column |
| `notice` | one `callout` | where the slot stands |
| `overview-card` | the envelope's `card` and a `widget` body with no form ([Overview cards](#overview-cards)) | as an Overview card |

An answer of another kind is not drawn, and the shell logs it.

**Positions.** A contribution stands `first`, `last` (the default), or directly
`before:` or `after:` an anchor. An anchor is a shell slot's own item (Access's
`password`, `sessions`, `web`, `luci`; the Overview's cards by id) or another
contribution, named `<plugin id>.<contribution id>`. Contributions at one place stand
by `order`, then plugin id, then contribution id, so the result is the same on every
render. An anchor that is not there, or that points back at itself through another
contribution, puts the contribution last.

**The request.** The shell GETs your `path` with:
- the same brokered reads, language and `X-Verso-Plugins` your pages get;
- `X-Verso-Slot`, the slot's name;
- `X-Verso-Features` ([below](#what-the-shell-speaks)).

Answer with an ordinary envelope, or with `204 No Content` when you have nothing to
show right now (a certificate source that is not set up). Then nothing is drawn and
nothing is logged.

**Only what is drawn is asked for.** The shell asks only the contributions to slots on
the page or drawer it is drawing, all at once, each within the plugin timeout. A
drawer's slots are asked for when it opens. The slots your contributed answer opens
are filled too, as the system plugin's part of Access opens the certificate card's.
Slots in an answer standing in those are not filled. The drawer your contributed act
opens is your own page, so its slots are filled.

**Forms.** A contributed block's form posts to the page it stands on, and only your
plugin receives the post, behind your declared write gate. A save that stages, or a
command that ran, reads the page again with your notice in the top-right notification.
Every other plugin on the page is asked for its piece as on any visit.

**Failure.** A contribution whose plugin is not running leaves no trace on the page.
A block or notice that times out, fails or answers what the shell cannot read is
drawn as the shell's unavailable state. An act or fact that does is left out and
logged, since no notice fits an act row or a facts list. Either way the rest of the
page draws. A contribution to a shell
slot the list does not name, or to a slot whose opener is not installed, is logged
once when the plugins load. Removing your package removes every contribution and
every slot of yours from every page, and nothing about them is kept on the router.

**Opening a slot of your own.** Your page opens a slot where you want other plugins'
pieces to stand. A block, act or notice slot is a `slot` widget with a `kind`. A facts
slot is a `slot` on the `properties` list itself, so contributed rows read in its
column:

```json
{ "type": "properties", "items": [ … ], "slot": "certificate.source" }
{ "type": "stack", "inline": true, "children": [ …your acts…,
  { "type": "slot", "id": "certificate.acts", "kind": "act" } ] }
```

Their names are `<your id>.certificate.source` and `<your id>.certificate.acts`. Their
anchors are the contributions in them, since your own items are not addressable.

**Access.** `system_access: "/access"` is read as `{id: "access", slot:
"shell.access.blocks", position: "before:web"}`, so a plugin written before slots keeps
its part. A manifest that also contributes to `shell.access.blocks` is read from
`contributes` alone.

### Overview cards

Contribute to `shell.overview.cards` to stand a card on the Overview. Its anchors are
the shell's cards (`internet`, `ports`, `tunnels`, `traffic`, `resources`, `system`,
`hardware`); a position places it in the default layout, and once an operator
arranges the page, the card stays where they put it. Answer with the card's header in
the envelope's `card`, and its body as the `widget`:

```json
{
  "schema_version": 1,
  "card": {
    "title": "Certificate", "icon": "shield-check",
    "tone": "warning", "status": "Renewal failed", "summary": "router.example.com",
    "cause": "The certificate could not be renewed", "opens": "/"
  },
  "widget": { "type": "properties", "items": [ { "label": "Domain", "value": "router.example.com" } ] }
}
```

- `title` and `icon` (a Lucide name) head the card.
- `tone` is `success`, `warning`, `danger` or `neutral`.
- `status` is the word at the header's end. Leave it out while all is well.
- `summary` is the one reading a closed card says after its title. It is words, never
  a rate or a duration, which only the shell writes: give those as `rate` or
  `duration` facts in the body.
- `cause` is what is wrong, as a sentence that stands without the card. The Overview's
  heading says it when it is the one thing wrong, and counts it among the rest, whether
  the card is open, closed or hidden. Leave it out while all is well.
- `opens` is the page on your plugin the card's door leads to.

The body reads and never asks, so it holds no form. A card answered with one is not
drawn.

Every installed plugin's card is asked for once each time the Overview is drawn,
hidden ones too, because their causes reach the heading. The Overview's live stream
never asks: it says again what the page read. A card with nothing to show answers
`204` and keeps its place in the operator's layout.

### What the shell speaks

Every request the shell sends a plugin carries `X-Verso-Features`: the contract
features the shell speaks, comma-separated (`slots`). When the list holds `slots`,
your opened slots are filled and your contributions are drawn. Without it, keep
whatever your page would show in their place.

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
  { "entity": "device", "slot": "shape", "label": "Limits", "summaries": true }
],
"entity_acts": [
  { "entity": "device", "slot": "unreserve", "path": "/?open={id}" }
]
```

A tab is answered on the reserved path `entity/<kind>/<id>` under your own mount,
GET to render and POST to save — the same request shape as a page, brokered reads
included. `{id}` in an act's path is the subject's own identity (a MAC, a uci
section name), substituted by the shell.

An act that changes the subject in place — the device row's `pause` toggle, its
power glyph — declares `"post": true` and no path. Pressing it posts to that same
`entity/<kind>/<id>` with `_act` set to the slot and `to` set to `on` or `off`;
answer with the commit it stages and a notice, and the shell returns to the
listing with your notice. Say which subjects have it on by listing the slot in
the summary's `engaged`, so the glyph's title says what pressing it does:

```json
"entity_acts": [
  { "entity": "device", "slot": "pause", "post": true }
]
```

A plugin that only contributes entity controls may leave `nav` empty. To keep
configured subjects visible after they disappear from live discovery, declare
`summaries: true` on its tab. The shell then requests `GET /entity/<kind>/` once
for the roster, with the same scoped reads as the editor. Return an `entities`
array of `{id, name, state, details, engaged}` in the envelope. Optional `details` is a list
of `{kind, label, values}` facts for the tooltip, with each label and value translated
separately. Optional `kind` identifies the fact independently of its wording:
`weekdays`, `hours`, `from`, `until`, `block`, `download`, or `upload`. This lets
the shell compose a visual schedule and rate readout; unrecognized facts retain
their text alternative. Weekdays carry the actual day tokens, including all
seven for an everyday schedule.
State labels describe configuration,
not live enforcement. Return `[]` for a successful read with no configured
subjects, or omit `entities` when configuration could not be read. The shell
merges these records with live devices and owns the labels, filters and drawer
links. There is no separate Device limits page.

The envelope an entity request answers with is an ordinary one, read a little
differently:

- `title` names the tab.
- `widget` is its body. Give the form `style: "page"` and no `submit`: the panel
  draws the commit row, and the form posts back into the panel rather than
  navigating, so the drawer stays open over the listing that opened it.
- `cta` and `consequence` are that commit row's words — the verb, and what
  applying it costs. Only you know; a tab that stages nothing sets neither and no
  row is drawn.
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
  "widget": { "type": "card", "children": [ /* … */ ] }
}
```

- `widget` — one root widget: a whole page body or one contribution fragment.
- `title` — the standalone page heading; omit it for a contribution.
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
- `act` — optional `{label, href, icon?, style?, opens_panel?, drawer?}`: the
  page's one act ("Add rule", "Add forward"), drawn hard right on the heading
  row, with the blank panel it opens (see [act](#act--the-pages-one-act-on-its-heading-line)).
  A page has at most one — it is *the* thing to do here, not a menu, so a
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
  `danger` | `info`). The shell says it in its notification at the top right
  of the viewport, exactly where its own confirmations appear; no outcome is
  ever drawn in the content — state the outcome, never compose it from
  widgets. Standing state belongs in `banner`; context beside content in a
  `callout`.
- `immediate` — optional boolean for a page made only of direct commands rather
  than staged configuration: nothing on it stages, though the staged-changes
  chip still shows pending changes from elsewhere. It does not make a returned
  `commit` immediate—commit intents always stage.
- `not_found` — optional boolean: the plugin has nothing at the path it was
  asked for (a typo, a stale link). The shell answers with its own page for an
  address with nothing at it, with a 404, rather than a page of the plugin that
  was not asked for. The SDK's `Envelope::not_found()` sets it and carries a
  plain page a shell from before this field shows instead. Answer it for any
  path you do not publish, rather than falling back to your first page.
- `commit` — optional; on a successful write, the uci changes for the shell to
  apply on your behalf (see [Writing config](#writing-config-the-commit-intent)).
  You never write config yourself.
- `width` — use `form` for a 640px settings page, `narrow` for 896px,
  `normal` for 1024px, or `wide` for 1152px. Layout uses the available width
  on smaller screens.
- `commands` — optional array containing one immediate, structured action.
  Supported names are `interface-restart`, `set-system-time`, `ssh-key-add`,
  `ssh-key-remove`, `certificate-generate`, `certificate-install`, and
  `ddns-update` (`section`: one ddns-scripts service, sent its address now;
  needs `ubus verso.ddnsUpdate`, and the service switched on in the applied
  config). Commands
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
open (`/plugins/dnsdhcp/?open=<mac>` opens that device's reservation). Read it beside
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
| `networkState` | `{"interfaces": [...], "devices": {...}, "protocols": {...}, "routes": [...]}` — netifd logical-interface dump, kernel-device state, installed protocol handlers (when available), and the kernel's main routing table as `ip route` lists it (`family`, `target` in CIDR, `gateway`, `device`, `metric`, `proto`, `type` when not unicast, `source`; local, broadcast, multicast and cached entries left out). The shell adds physical-port and parent relationships from its existing topology reader. |
| `accessCredentials` | Public SSH key comments/fingerprints and public web-certificate metadata. No authorized-key options, private keys, or router passwords are included. |
| `firewallCounters` | `{"counters": [{"chain", "name", "packets", "bytes"}]}` — fw4's live nftables hit counters, one entry per (chain, rule name), summed across the several nft rules a single UCI section can render into. |
| `wirelessState` | `{"radios": {"radio0": {"up": true, "channel": 6, "txpower": 20, "busy": 61, "hardware": "NXP 88W9098"}}, "networks": {"default_radio0": {"up": true, "clients": 3}}}` — what the radios are doing now, keyed by `wireless` config section. netifd's `network.wireless status` says whether each radio and network is up and which kernel interface a network became; iwinfo reads that interface for the channel and transmit power (dBm) in use, the in-use channel's busy share (whole percent of active airtime) and the associated clients. A number that could not be read is absent, never zero. The operator's grant must cover `network.wireless status`, and `iwinfo info`/`assoclist`/`survey` for the numbers. |
| `openvpnFiles` | `{"files": [{"path": "/etc/openvpn/proton.ovpn", "family": "openvpn", "content": "...", "version": "...", "pending": false, "exists": true}]}` — OpenVPN's profiles (`.ovpn` and `.conf` directly in `/etc/openvpn`) as they will read once staged changes are applied, in the shape `dnsState` and `firewallFiles` give theirs. A file there is read as a profile only when an `openvpn` section names that exact path as its `config`, or it is staged; any other (a sign-in file saved as `.conf`, say, which holds no block to fold) is listed by its `path` alone, never opened, with an `error` in place of its `content` and `version`, so its name stays taken and nothing in it leaves the helper. `content` has every inline block folded to one line (`<tls-crypt> … 18 lines … </tls-crypt>`): every block OpenVPN would read inline, whatever its name (a `<connection>` block folds whole), found as OpenVPN finds it — a line whose only token is `<name>` after OpenVPN's quoting, comments and leading `--`, read in OpenVPN's own line-buffer pieces, closed by the first line that starts with `</name>` past C whitespace. `version` is the whole file's at its path, an HMAC of the path and the file under the helper's root-only key: it tells a save the file changed and gives nothing of a folded block away, here or at another path. Without the key (no randomness to make it) a present file has an `error` saying so in place of its `version`, and its save is refused. A file that does not exist has the fixed version the SDK names `NEW_FILE_VERSION`. Stage an edit with `config-file-stage` (declare `ubus verso.stageConfigFile` and `uci openvpn write`): a placeholder left as it was takes back its original block, so an editor that never saw a key keeps it. A save that would put a block back where OpenVPN no longer reads it inline (a BOM-led opener moved off the first line, or with a line added above it) is refused, so neither the next read nor `vpnState` shows what it holds; so is one that has a line of a block open a block it did not open before (a block opened above the placeholder and closed by a hidden `</name>`), whose `<name>` the next read would show as the placeholder. A new profile is written readable by root only. |
| `vpnState` | `{"instances": {"proton": {"config": "/etc/openvpn/proton.ovpn", "running": true, "state": "connected", "since": 1791295321, "server": "185.107.56.234:1194", "address": "10.96.0.14/16", "device": "tun0", "profile": {...}}}, "tunnels": [{"device": "tun0", "kind": "openvpn", "instance": "proton", "up": true, "rx": 1331000000, "tx": 190800000, "addresses": ["10.96.0.14/16"]}]}` — each `openvpn` section that names a profile file, and every tunnel device someone brought up (the kernel's own `sit0` and kin left out). `state` is what OpenVPN itself last reported through the hooks OpenWrt's init wires to every instance (`/etc/hotplug.d/openvpn/50-verso`, shipped by the verso package beside this helper, recording into `/var/run/verso-openvpn`): `connected` at `route-up`, `connecting` from `up` or `route-pre-down` (a reconnect under `persist-tun`), `stopped` without a process. Where OpenVPN has reported nothing this run (the hook arrived after the tunnel came up, or the profile sets `script-security` below 2), it is what the kernel proves over rtnetlink: `up` when the tun device carries with an IPv4 address, otherwise `connecting`. A record counts only when it carries the running process's PID and was written by root, which runs OpenVPN's first hooks and records under the instance's name, or by the user a profile drops to, which runs the hooks after, records under the name and its uid (`proton.65534`) and carries root's record of the run over; the user's is read first. `since` (an epoch second, connected only) and `server` come from the hook and are absent without it; `address` is the device's while it has one. No state is read from the log. `device` is the tun device the process holds. `profile` reads the file out — `client`, `dev`, `proto`, `remotes` (`host`, `port`, `proto`), `random`, `cipher`, `verify`, `asks_sign_in`, `redirect`, `nopull`, `blocks` (the inline block names) and `text`, the file folded as `openvpnFiles` folds it; the facts come from the lines outside the blocks, and `remotes` and `proto` also from each `<connection>` block's own options (a block inline in one left out), the block itself still folded. Profiles are read only under `/etc/openvpn`; key blocks and sign-in files never leave the helper. A tunnel's `kind` is `openvpn`, `wireguard`, `tailscale` or `tunnel`. |
| `ddnsState` | `{"services": {"home": {"address": "203.0.113.7", "running": true, "updated": 200, "next": 100}}}` — each ddns-scripts service with files in `/var/run/ddns`, keyed by section. `address` is what the name resolved to at the updater's last check, every address one per line (empty when it did not resolve); `updated` is seconds since an update was last sent and `next` seconds until the next check, both absent when the updater has not written them; `running` is whether the pid file names a live `dynamic_dns_updater`. No state is read from the log. |
| `dhcpLeases` | `{"leases": [{"hostname", "mac", "ipv4", "ipv6s", "expires_at"}]}` — who holds an address right now, in address order. dnsmasq's DHCPv4 lease file is the list; odhcpd's DHCPv6 table adds `ipv6s` to the device it belongs to, joined by the link-layer address its DUID embeds or by the hostname both tables recorded. `hostname` is empty when the device offered none, `expires_at` is an epoch second (render the countdown against your own clock), and a DHCPv6 lease that joins no DHCPv4 lease is left out — it carries no MAC, which is the identity a reservation is keyed by. The shell reads the lease file itself, so it answers only an operator whose grant covers `dhcp ipv6leases`. |

| `acmeState` | `{"domain": "router.example.com", "requesting": false, "lan": "here", "outcome": "renewed", "at": 1759000000, "certificate": {"subject", "issuer", "from", "until", "fingerprint", "serial", "days_total", "days_left", "expired", "self_signed"}, "serving": true}` — the trusted certificate of `config cert 'verso'` in `/etc/config/acme` (ADR-017 §6); `{}` when there is no such section. `outcome` (`issued`, `renewed`, `issue-failed` or `renew-failed`) and `at` (an epoch second) are the last event acme sent for the section's first domain, recorded in RAM by `verso-rpcd acme deliver` from acme's hotplug, and absent after a reboot or when the domain changed since. `requesting` is whether acme's procd instance for the section runs. `lan` is where the router's own lookup finds the domain, the answer the LAN gets from whatever keeps its names: `here` (one of the router's addresses), `elsewhere` or `none`, the lookup bounded at 1.5 s. `certificate` is acme's certificate for the domain, in the facts `accessCredentials` gives the served one (the key is never read); `serving` is whether it is the one Verso serves. Both are absent while acme holds none. No state is read from the log. |

The header is absent when you declared no `ubus` read, when the function is not one
the shell brokers, and when the read failed or the operator was not allowed it. All
four are the same thing to you: no data. Render the page without it — a missing
count is a dash, not an error.

### Which plugins are installed

Every request also carries the **`X-Verso-Plugins`** header: the ids of the
plugins installed on the router, comma-separated (`acme,dnsdhcp,system`), your
own among them. The SDK reads it as `Request::plugins`; ask
`request.has_plugin("dnsdhcp")` before offering an act another plugin carries,
such as a link to its drawer, so the link never leads to a page that is not
there. A shell older than the header sends none, which reads as nothing
installed beside you.

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
is not a rule matching every port. An empty array clears its list option the
same way: rpcd refuses a `uci set` that carries one, so the shell deletes the
option instead.

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

The shell's Pending changes page lists what is waiting to be applied, one card
per page it belongs to. By default each pending change reads as its raw uci
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

A sentence about one object names it with `about`, and config history (ADR-021)
names the change by it: `Allow-Ping` `target` ACCEPT → DROP rather than a raw
section handle.

```rust
Description::new("Turned off the rule “Block Telnet”.", vec![0])
    .about(SubjectKind::Rule, "Block Telnet")
```

The name is the one a person set. An object with no name of its own takes an
empty one, and the shell calls it by its section. `SubjectKind` is closed, each
kind drawn as its own chip:

| kind | what it is |
| --- | --- |
| `Rule` | a firewall traffic rule |
| `PortForward` | a firewall redirect |
| `Zone` | a firewall zone |
| `Crossing` | a forwarding between two zones |
| `Network` | an interface |
| `Route` | a static route |
| `Device` | a device on the network |
| `Wifi` | a Wi-Fi network |
| `Dhcp` | a DHCP pool or reservation |
| `Service` | a daemon and its settings |
| `Setting` | one setting on a page of settings, by its label |

A sentence that names no subject, or about several objects at once, leaves the
change to be named by its config and section, as a raw line is.

A change made on a page of settings is said as its row says it: the row's label,
the value it now holds, and the option as its key chip. Build it with
`Description::setting(label, value, key, covers)`, adding `.verbatim()` when
the value is a string the machine wrote (a name, an address) rather than words.
To keep those words the page's own, render the page from the snapshot and read
the control: `page.reading("drop_invalid")` gives its label and what it now
shows, `on`/`off` for a switch and the option's label for a pick.

```rust
let page = settings::page(&Firewall::read(snapshot)).widget;
if let Some(read) = page.reading(&change.option) {
    let said = Description::setting(&read.label, &read.value, &change.option, vec![i]);
    out.push(if read.words { said } else { said.verbatim() });
}
```

A sentence that stands for one option, with or without `setting`, carries that
option's key chip in the review.

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
allows — leave it to the raw line otherwise. Two SDK helpers do what every hook
starts with: `group_changes(changes, key)` gathers the changes into runs by the
key each maps to (a section, a setting), in the order the keys first appear, and
`section_type(changes, snapshot, config, section)` is a section's type, read off
the `add-section` change that made it or the snapshot. The describe hook never writes and
never answers with a page; it only turns changes into words, though it may build
one of its own pages to read a control's words from it.

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
    mkdir -p /var/run/verso-plugin-system   # your socket's own directory
    chown verso-plugin-system:verso /var/run/verso-plugin-system
    chmod 0750 /var/run/verso-plugin-system

    procd_open_instance system       # a NAMED instance -> procd runs exactly one
    procd_set_param command /usr/bin/verso-plugin-system
    procd_set_param env VERSO_SYSTEM_SOCKET=/var/run/verso-plugin-system/system.sock
    procd_set_param user verso-plugin-system  # your own account (below)
    procd_set_param group verso
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

### Your account and your socket

Your plugin runs as a system account of its own, named `verso-plugin-<id>` after
your manifest's `id`, with primary group `verso` (ADR-007). Your package or init
script adds it to `/etc/passwd`. The shell finds your account by that name and,
before it sends you a byte, asks the kernel which account is listening behind
your socket (`SO_PEERCRED`). Only `verso-plugin-<id>` is talked to: any other
listener at your path, and any plugin with no such account, gets nothing — no
snapshot, no form — and the page reads "plugin unavailable". So a plugin that
runs under another name, or as root, is never reached.

Bind your socket in a directory of your own, `/var/run/verso-plugin-<id>/`,
which your init script makes as root, owned by your account, group `verso`, mode
`0750`, before it starts you (above). `/var/run` is root's and the directory is
yours, so no other plugin can create a name there and take your path in the
moment a restart frees it. The SDK binds `VERSO_<ID>_SOCKET` when it is set, and
stops your start, with the reason on stderr, when something it cannot remove
holds the path. A socket in the shared `/var/run/verso` (the SDK's default) is
served too, but there another plugin can keep yours from binding while a restart
has the path free; the peer check keeps it from answering as you.

### Installing and removing

The shell reads the manifests at start, and again when Packages installs or
removes a package. A package `apk` installs or removes outside the shell must
tell it, or its pages and nav row wait for the shell's next start. End the
package's post-install and post-deinstall scripts with a SIGHUP to the shell,
which re-reads the manifests and catalogs and keeps every session:

```sh
ubus call service signal '{"name":"verso","signal":1}' 2>/dev/null
```

Send it through procd, not `/etc/init.d/verso reload`: a reload that changes
nothing in the shell's instance delivers no signal. On the same rescan the shell
gives every signed-in session the access list your package ships, as signing in
would (ADR-007 §8), so nobody signs in again to use your pages.

## The widget vocabulary

You compose a **closed set** of widgets with **semantic** props — `label`,
`kind`, `datatype` — never colours, spacing, or fonts (ADR-005). You cannot add
widget types; you nest and compose these. When nothing fits, `raw` is the
display-only bridge (last). Every node is `{"type": "...", …}`.

A `link` may point into another plugin's pages (`/plugins/dnsdhcp/?open=lan`)
without knowing whether that plugin is installed: the shell draws a link into
a plugin that is not installed nowhere — not on its own, not as a section's
act, not as a callout's door or act — so the page never leads to an address
with nothing at it. A link into an installed plugin stays even while that
plugin is down, since its address answers with the unavailable card.

### card — a container

Nests other widgets. This is how you lay out a page.

```json
{ "type": "card",
  "children": [ /* any widgets */ ] }
```

### grid — responsive columns

Use `grid` to place related widgets beside one another. `columns` is the desktop
target; the shell owns responsive collapse and spacing. `style:"form"` groups
related fields with labels above their controls and uses the form rhythm. It
stacks below 36rem of available container width, uses two columns from 36rem,
and reaches three from 48rem when requested. This works inside drawers as well
as pages. Values retain their content-based widths within each cell. Use
`Widget::form_grid(columns, children)` in Rust. Group meaningful pairs such as
a new password and its confirmation, rather than filling rows arbitrarily.

**Fused values.** When every child is a plain typed value (a `field` of kind
`text` or `password` with no `style` or `remove`) and the group has no `join`, the shell draws the group as one
control: one box split by hairlines, each part as wide as its value and its
`unit`. The row has one `label` covering every part ("Connection rate" over a
rate and a burst), one key chip naming every option (`synflood_rate ·
synflood_burst`), and one `staged` mark. Each field's own `label` becomes its
part's name for a screen reader and for its refusal ("Burst: Enter a whole
number."). The row's description under its label is the group's `help`, then
each part's own `help` led by the part's name ("Burst: …"), and each part
names itself inside the box after its value, where a unit stands. Without a
`label`, the parts' labels joined by `·` name the row. A new password and its repeat fuse as two secret parts. A group
holding a choice, a list or a revealable secret keeps its columns.
In Rust: `Widget::form_grid(2, fields).labelled("Connection rate", "")`.

```json
{ "type": "grid", "style": "form", "columns": 2, "label": "Connection rate",
  "children": [ /* plain typed fields */ ] }
```

**Joined parts.** A group whose parts read as one sentence names the word
between them in `join` ("to"): a path from one zone to another, a window from
one time to the next. A word between parts always splits them. Each part (a
`field` of kind `text` or `select` with no `style` or `remove`) stands as its
own control, side by side with the word between: a dropdown measures its
longest option and stays a dropdown however few options it holds, and a typed
value keeps its own measure. The row keeps one `label`, one key chip and one
mark, and each part its own name, option and refusal. Declare a typed end's
grammar so the shell sets it in mono at its measure and checks it:
`timehhmmss` and `dateyyyymmdd` accept what firewall4 reads (`09:30`,
`2026-03-01`). In Rust:
`Widget::form_grid(2, picks).labelled("Path", "").joined("to")`, and
`.typed("timehhmmss")` on a field.

```json
{ "type": "grid", "style": "form", "columns": 2, "label": "Path", "join": "to",
  "children": [ /* a select for src, a select for dest */ ] }
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

A rate is handed over as a number, never as text: `"rate": 2500000000`, in
bits per second, on a property (in place of `value`) or on a table cell (after
its `text`, "port · 2.5 Gb/s"). The shell writes it the way it writes every
rate, in the reader's language ("2,5 Gb/s" in Slovenian), which a plugin
cannot do: it never learns the reader's language. The SDK's
`Property::rate("Link", bps)` builds the property.

`variant` tones a value whose reading is also a verdict — `success`,
`warning`, `danger`, `info`, the same vocabulary a badge speaks. It names a
meaning, never a colour: the shell picks the ink, in both themes, and a word
outside the vocabulary tones nothing.

The default list is the shell's one fact sheet: the same rows an Overview
card's facts and the firmware ledger are. Values stand on the right edge, or
with `"align": "left"` beside a fixed label column, for values read line by
line (addresses, versions). A `head` names the facts on a line over them:
a group's title with a machine word beside it as a chip, or the names of the
two columns. `empty` is the sentence that stands in the empty set's slot when
there are no items, under the head when there is one. In the SDK they are the
`head` (a `PropertiesHead`) and `empty` of `Widget::Properties`.

```json
{ "type": "properties", "align": "left",
  "head": { "label": "IPv4", "chip": "dhcp" },
  "empty": "No address.",
  "items": [
    { "label": "Address", "value": "10.0.0.2/24", "mono": true, "copy": true }
  ] }
```

### actionbar — a listing's controls

The band between a page's heading and its listing, and only the listing's
controls: `filter` (the search's placeholder), `tabs` and `live`. Over
a table it is the listing's one filled surface — quiet sand between two
hairlines, every control 1rem from its edges — and the table sits flush under
it; a `live` log's bar is the same controls, unfilled. `tabs` are the coarse
cuts, drawn as one dropdown with each option priced by its `count` ("IPv4 ·
14"); name the whole set in the first one's label ("All families", not "All").
The search is drawn
only over a `live` listing: a listing that holds what it has is scrolled and
searched with the browser's own find, however long it is. A bar left with
nothing to show is not drawn. The page's act is not on the bar: it is the
envelope's `act` (below).

### act — the page's one act, on its heading line

The envelope's `act` (`Envelope::with_act`) is the page's one act, which the
shell stands on the heading line beside the title: making a new subject of the
kind the page lists, or, with `style: "quiet"`, taking something away from it (a
log's download). It travels beside the widget tree, not in it, because the
heading line is the page's. `opens_panel: true` makes the act open the page's
own panel blank (making one is editing one that does not exist yet); `drawer`
carries that blank panel, open when the address asks for it (`?open=new`), and
a panel request for that address is answered with it as it would be with a
row's.

```json
{ "label": "Add network", "href": "/plugins/wireless/?open=new", "icon": "plus",
  "opens_panel": true }
```

### table — config sections as identical rows

The Advanced-view listing: one row per config section under fixed columns. Every
column declares a `kind`, and that kind renders every cell in it the same way —
rows cannot vary in shape. The tuple (from, to, protocol, port) is the row; the
name is an optional trailing comment.

```json
{ "type": "table",
  "columns": [
    { "label": "From",     "kind": "endpoint" },
    { "label": "To",       "kind": "endpoint" },
    { "label": "Protocol", "kind": "keyword" },
    { "label": "Port",     "kind": "mono" },
    { "label": "Comment",  "kind": "comment" },
    { "label": "Hits",     "kind": "num" }
  ],
  "rows": [
    { "id": "force_dns_guest", "cells": [
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
- `"detail"` — a reading's facts in words (`"warns 81 °C · protects 85 °C"`),
  muted, each figure heavier than the words around it, as a gauge's
  caption weighs them. A figure is a word of digits alone (decimals after `.`
  or `,`), with its unit when the unit follows it across a narrow no-break
  space (U+202F, as `"81 °C"`); a word with a digit in it and a word after an
  ordinary space stay words.
- `"num"` — right-aligned tabular figures (counters); muted.
- `"meter"` — a share of something: ten blocks lit to the cell's `fill`
  (0–100) with `text` as the figure beside it (`"61 %"`). `variant` bands the
  lit blocks — `warning` marigold, `danger` crimson, anything else green — so
  the listing sets its own thresholds. A headroom (a temperature against the
  point the hardware protects itself) runs its bar to its protect limit and
  sends `warn_mark`, its warn limit as a percent of the bar: the blocks past
  it are shaded; 0 shades none. A cell that sends none draws a plain bar, so a
  share keeps the bar it has always had. A reading with no
  ceiling sets `no_track`: its `text` stands in the figure's place and no bar
  is drawn, never a bar against a guessed ceiling. A cell with no `text` is
  the faint dash.
- A `text` cell may carry a `chip`: the config value the word stands for
  (`"WPA3"` beside `sae`).
- `"pill"` — an enum value as a status pill; the cell carries `text` plus a
  `variant` from the badge vocabulary (`success`/`warning`/`danger`/`info`/
  neutral) — e.g. `accept`→success, `reject`→warning, `drop`→danger. An empty
  cell renders a faint dash, which is what keeps the pills meaningful. The
  SDK's `pill_cell(text, variant)` builds one.
- `"endpoint"` — one or more traffic endpoints; each is `{ "kind", "label" }`
  where kind is `"zone"` (sans + shield), `"device"` (mono address + screen),
  `"router"` (this device, accented), or `"any"` (muted globe).
- `"reorder"` — a drag handle, first column only; see **Reorderable rows** below.

In the Rust SDK, `text_cell(text)` is a plain value or, when there is none, the
quiet dash (`muted_cell(EM_DASH)`) every listing states a missing value with.

A row's `id` is its stable handle — use the UCI section name.

A column may fix its `width` at a measure named by what it holds, so a
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
`uci order` on that config, like every other write, so the Pending changes
page's apply is what makes it live. Your plugin ships no drag logic and never sees the
reorder POST; it renders the page again from the fresh snapshot.

A drop is a save. The reorderable table renders **the page's form** beside
itself, carrying the config and the row sequence as hidden fields, and the shell
posts it where it stands when a row lands, so the new order counts in the
rail's Pending changes row like any other change and shows on its page.
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
panel in place, seeded by the href. `close` is the lane's last word, a line of
phrase parts drawn after its last row in the meta step: what happens to what
none of its rows caught. It is no row of the listing, so nothing drags it and it
holds no place in the order.

```json
{ "group": { "key": "input_wan", "label": "WAN", "to": "Router", "close": [
  { "words": "Otherwise" },
  { "chip": { "label": "reject", "tone": "danger", "dot": true } },
  { "words": "the zone’s input policy", "href": "/plugins/firewall/zones?open=wan" } ] } }
```

**Phrase.** A `phrase` column names its rows with one line of parts, in order:
a `chip` (an entity reference), `words` (prose the shell translates) or a
`value` (a machine string, verbatim in mono). Words with an `href` lead to where
what they name is decided, in the line's own ink, underlined (the SDK's
`TablePart::Door`). It reads the way a person would
say the row and the way the device holds it at once: `[Allow-Ping] [target]
ACCEPT → DROP`. The cell's `text` is the line read plain. It is not drawn, and
it names the row to the controls in it. A `detail` line sits beneath, as a
name's does. The phrase is the row's name, so a muted row marks it over.

```json
{ "text": "Allow-Ping target ACCEPT to DROP", "parts": [
  { "chip": { "icon": "shield", "label": "Allow-Ping" } },
  { "chip": { "label": "target" } },
  { "value": "ACCEPT" }, { "words": "→" }, { "value": "DROP" } ] }
```

An individual icon in an `actions` cell can carry `confirm_title`,
`confirm` and `confirm_cancel`. Only that action opens the dialog; its neighboring toggle or edit
action keeps its usual behavior. The shell substitutes the row's name for `%s`
in the title after translation. On confirmation it posts the action's `name`
and `value` to the listing. Identify the removal target in that value rather
than the `open` query parameter, which may still name a different editor.

`confirm_cancel` names the way back after what it keeps — `"Keep session"`,
`"Keep rule"`. Left out, the shell says "Keep it". It is never "Cancel": the
alert speaks one hue, its question and both answers in crimson.

```json
{ "actions": [
  { "icon": "trash-2", "title": "Remove network",
    "name": "_remove", "value": "guest",
    "confirm_title": "Remove network “%s”?",
    "confirm": "Devices using this network will disconnect when the change is applied.",
    "confirm_cancel": "Keep network" }
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
own meaning. The panel's first section may set `flush:true` so the drawer
supplies the outer top inset instead of stacking two layers of padding.

Set `open:true` to render the panel already open — how a link from elsewhere in
the shell arrives with the row's panel already in front of the operator
(`/plugins/dnsdhcp/?open=lan` opens lan's DHCP server).

```json
{ "id": "lease-iphone", "cells": [ /* … */ ],
  "drawer": { "title": "iPhone — 10.0.0.23",
              "children": [ /* properties, callout, form */ ] } }
```

A panel's form posts to its listing's address, the same address a row's toggle
posts to while the panel is open, so the form names itself: the SDK's
`panel_form(submit, fields)` appends the hidden `_panel=1` (`PANEL_FIELD`),
and `from_panel(form)` reads it back. Read a submission without it as a row
act, never as a save. A confirmed removal names its subject in `_remove`
(`REMOVE_FIELD`). `text_field` and `select_field` draw a typed value and a
closed choice carrying the message `Errors` holds for them.

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
  its pause, and stands on the heading line beside the page's act: it reads `Live`,
  its spinner turning, while events flow, `Paused · N new` while they are
  held (its title says the act, Pause or Resume), `Connecting…` while the
  stream is down, and nothing on the page moves — not even the relative
  times — until it is pressed again.
- **Narrowing.** Every value in a streamed row is a control: click a verdict, a
  zone, or an address and the listing narrows to it, with what was clicked
  travelling to a shelf above the table wearing the treatment it had in the row.

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

**Editing rows are field rows.** A row with a `toggle`, or a `value` posted
under a `name`, is drawn as the same row a `field` or `switch` is: the `title`
is its label, the `code` is its key chip, and the `desc` stays in view under
it. The `code` is the option's key, so the shell marks the row `staged` while
that option waits in the form or section's `target`, exactly as it marks a
field. A row that only reads (pills, or a `value` with no `name`) keeps the
compact listing row and is never marked.

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
holds; the operator applies from the Pending changes page (ADR-010):

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
`_action=reshape` is the shell's own: it arrives only when a `reshapes` select
changed (see [field](#field--one-labelled-control)), and the shell strips it from
any form that posts it itself.

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
- `error`: an inline error to show under the field (you set this on a 422).
- `key`: the option this field writes, verbatim — `"ipaddr"`, `"leasetime"`. It
  rides beside the label as a mono chip, so someone who knows the config sees
  which line they are editing and someone who does not can ignore it.
- `unit`: what the number in the box is counted in — `"Mb/s"`, `"seconds"`. It
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
  set long enough to wrap belongs in the grid. Single-choice `select` fields
  use radios up to the shell's `RadioOptionLimit` (currently three choices),
  and native dropdowns above it, including when an older schema supplies this
  style. Plugins declare the option set; the shared shell chooses its control.
  Selected multi-select chips share the shell's Denim choice colour and white
  labels with the other selected form controls.
- `reshapes: true` on a `select`: its value decides which fields the form has
  at all — what kind of object a New drawer makes. Changing it posts the form
  with `_action=reshape`; answer with the form drawn for the value now chosen,
  keeping the posted values that still mean the same thing, refusing nothing
  and returning no `commit`. The shell swaps that form (in a drawer, the whole
  drawer, title included) into place, stages nothing even if a `commit` comes
  back, and returns focus to the choice. While the form is on its way the
  shell's waiting mark turns beside the choice. A choice that only shows or hides
  fields inside one shape is a `when`, which needs no round trip.
- `help`, `tip` and `source`: a field's description, written under its label,
  is its `help` and then its `tip`; the operator can fold every description on
  the page away at once, and a screen reader reads it either way. `help` says
  what to put in the control, or what changing it does (`"Takes effect
  immediately, for everyone who signs in to this router."`): nothing stands
  under the control but its refusal. `tip` answers "what even is this", for a label
  that is a term of art the operator did not choose — a DUID, an interface
  identifier — and is left off where the label is already the plain word for
  the thing. `source` names what reads the option and joins `key` in the chip
  beside the label (`duid · dhcp host`):

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

`verbatim: true` says the label is a machine string, such as a path: it is set
in mono and never translated. `error` refuses the state the switch was set to,
drawn exactly as a field's refusal is (the band under the label, the checkbox
marked invalid and pointing at it); never push a callout beside the switch. `style: "locked"` states a state nothing on the
page changes; it is drawn set or clear, inert, and posts nothing.

**Switch groups.** Switches alone in a labelled `form` grid are one setting
asked of several things, such as which included files load. The shell draws the
grid's `label` once, with the options the switches write named once in its
chip, then one checkbox row per switch, its `help` kept in view under it and its
own `staged` mark. In Rust:
`Widget::form_grid(1, switches).labelled("Files the config includes", "")`.

For object-level state, a section may place an inline switch in its heading with
`control`; set the switch's `style` to `"inline"`. The state label — its
`label` when on, "Disabled" when off — follows the switch and changes with it. This keeps identity and state
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

> **The schema's edges are a finding.** A single value repeats with `list`; a
> field-set appears on a toggle with [`conditional`](#conditional--a-field-set-behind-a-toggle); a value is
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

### ports — a router's panel

Draws physical ports as the panel a person recognises: a chassis with each
connector in order, its link LED lit while it has link and its traffic LED
blinking at its activity. You send the ports and their state; the shell draws
every pixel and keeps each port reachable by keyboard, with its detail on hover
or focus.

```json
{ "type": "ports", "legend": true, "ports": [
  { "kind": "sfp", "label": "wan", "verbatim": true, "iface": "wan", "role": "wan",
    "linked": true, "activity": 3, "speed": "2.5G", "addr": "203.0.113.7" },
  { "kind": "rj45", "label": "lan1", "verbatim": true, "iface": "lan1",
    "linked": false, "speed": "—" } ] }
```

- `kind` — `rj45` or `sfp`, the connector drawn.
- `label` — the name under the connector; `verbatim: true` when it is a kernel
  name the language must not touch.
- `iface` — the interface, which keys the port's live updates and its detail.
- `linked` — a cable has link: the link LED is lit.
- `activity` — how busy the traffic LED blinks, `0` (dark) to `4` (near solid):
  under 10, under 1,000, under 100,000 and from 100,000 packets a second. The
  boolean `active` is accepted in its place and reads as `2`; `activity` wins
  when both are sent.
- `speed` — the negotiated speed as labelled (`1G`), a dash without link.
- `role` — `wan` marks the port carrying the internet.
- `addr`, `note` — lines in the port's detail.
- `legend` — a key to the LEDs and connector kinds under the panel.

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

### slot — a place on your page

A slot has two uses.

**With no `kind`, it holds a block the shell owns.** You say where it stands, and the
shell says what it is. These ids are the shell's, a closed set, and a slot the shell
has no block for draws nothing.

- `appearance` is the Appearance card, where the operator picks the router's theme
  (ADR-024). The system plugin leaves it on General, between its router settings and
  Time.

```json
{ "type": "slot", "id": "appearance" }
```

**With a `kind` of `block`, `act` or `notice`, it is a place you open for other
plugins to fill** ([Filling slots](#filling-slots)). Its name is `<your id>.<id>`.

```json
{ "type": "slot", "id": "certificate.acts", "kind": "act" }
```

A facts slot is the `slot` field of a `properties` list instead.

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
page `title`, `pages` and manifest `nav` labels, and every
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
| `cidr` | an IPv4 or IPv6 address and its prefix length — `10.0.0.1/24`, `2001:db8::/64`; the length is required, and the address may carry host bits (a WireGuard interface's `addresses`, a peer's `allowed_ips`) |
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
6. Runs as a single procd instance with output/crashes visible in `logread`,
   as its own account `verso-plugin-<id>`, listening in a directory of its own.

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
`link` style `rail` and a live `code` preview in that column. Form sections
remain unboxed.

The Interfaces inventory uses `table` style `interfaces`: its six columns are
identity, device type, IPv4 address, MAC, operational state, and actions. Identity
cells use `lead_icon:"physical"` or `"software"` for the topology mark and `chips`
for logical network references. State remains plain text with a square mark.
`depth` describes a transport dependency (VLAN or PPP), not bridge membership.
Its envelope `act` is the add act at `new`: on the listing it carries no panel,
so the shell fetches a new network's drawer from that address and its form posts
back there; at `new` or an object's editor address it carries that panel open.
The new drawer opens on a `Type` select marked `reshapes` (Network, Internet
connection, Bridge, VLAN, Tunnel); changing it draws the drawer for that kind,
keeping the name, device, MTU and MAC override. The page has no control band.
Expanded details use `grid` styles
`facts` and `configurations`; an editor rail may hold an unlabeled live code
block.

`verso.networkState` brokers normalized netifd booleans and kernel device facts.
`interface-up`, `interface-down`, and `interface-restart` commands each require
the matching `network.interface` write grant and are checked against the
operator's concrete netifd object permissions. They affect runtime only; UCI
`auto` stays in the staged editor. The shell refreshes the inventory without
resetting its filter, expanded row, or the session's inactivity deadline.

### Hand-edited files

Some daemons read files no config section describes: dnsmasq's option files and
fw4's rule files. The helper stages them in families, each binding its files to
the config whose daemon reads them:

| Family | Files | Staged by a plugin with | Checked at Apply by | Read again by |
|---|---|---|---|---|
| dnsmasq | `/etc/dnsmasq.conf`, `/etc/dnsmasq.d/*.conf` | `dhcp` write | `dnsmasq --test` (also on save) | `dnsmasq reload` |
| fw4 | `/etc/nftables.d/*.nft` | `firewall` write | `fw4 check` of the whole ruleset | `fw4 restart` |

A staged file waits in the review under its family's config (`dhcp`,
`firewall`), and discarding that config discards that family's files alone.
fw4 is rebuilt with `restart`, not `reload`: its reload flushes the table, which
keeps chains a file no longer declares, so a rolled-back base chain would stay
hooked in. `verso.firewallFiles` brokers fw4's files as `verso.dnsState` does
dnsmasq's; each lists only its own family.

A page lists and edits its family with the SDK's `verso_plugin::files::FileSet`:
the Files band with its count and New file, a row per file (folder, name, lines
that set something, the pen that opens its editor), and an editor drawer whose
save is the `config-file-stage` command. DNS and Firewall declare only
their folder, suffix and words, so the two listings cannot drift apart.

### DHCP and DNS pages, and custom option files

The DNS/DHCP plugin serves two pages under Network, both over `/etc/config/dhcp`.
**DHCP** (`/plugins/dnsdhcp/`, also where any other sub-path lands) lists the
servers (one per network's `config dhcp`, each edited in its drawer: pool, lease
time, what devices are told, IPv6 announcement), the reservations (`config host`,
each in its drawer; New opens it blank, a lease's Reserve opens it filled from
the lease, and the device panel's Reserved address tab is the same form), the
live leases, and the `config dnsmasq` options about DHCP, edited on the page.
`?open=` names the open drawer: a network, a reservation's section or its
device's MAC, or `new` (with `reserve=<mac>` to fill it from a lease). **DNS**
(`/plugins/dnsdhcp/dns`) is the resolver's settings form; its record and
upstream editors live under `dns/records/` and `dns/servers/`, its custom option
files under `dns/files/`. The Interfaces plugin creates a default server with a
new network and renames it with the network; editing it is the DHCP page's.
`dhcp` sections of type `verso_defaults` hold the ordinary upstreams retained
while encryption is selected.

The shell brokers `verso.dnsState` for installed DNS capabilities and custom file
contents. The `config-file-stage` command takes `path`, `expected` (the version
returned by the read, opaque, or `NEW_FILE_VERSION` to create a file), and
`content`; its manifest must declare both
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


DHCP service visibility is shared by the Interfaces and DNS/DHCP plugins via
`verso_plugin::dhcp`. The SDK merges applied observations with each plugin's
staged snapshot, labels pending enable/disable changes explicitly, and links each
server to its drawer on the DHCP page (`/plugins/dnsdhcp/?open=<network>`),
which the interface listing opens over itself. The interface identity
carries one DHCP chip per configured network: success, warning or danger, with a
neutral disabled state. The DHCP broker obtains protected include directives through a bounded helper read; handwritten exclusions are included in its service assessment. Chip `title` is localized independently of its verbatim
identity label. Expanded network facts show the observed pool, lease duration and
active lease count. Physical bridge members do not inherit their bridge's chip.

Table style `live` uses the shell's existing ten-second inventory refresh to
replace only its rows. It preserves the surrounding settings form, skips hidden
pages and open dialogs, and shows a stale-data notice when a read fails. The DNS
network summary uses this style; configuration still belongs to the interface
editor and follows the normal Save/Apply flow.
