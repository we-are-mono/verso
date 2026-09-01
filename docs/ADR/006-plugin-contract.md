# ADR-006 — Mechanical plugin contract: manifest, unix-socket transport, schema gateway

- **Status:** Accepted
- **Date:** 2026-08-23
- **Deciders:** tomaz@zaman.io
- **Relates to:** ADR-001 (single static binary, crash isolation motive), ADR-003
  (the transport is a test seam), ADR-004 (stdlib `net/http`), ADR-005 (the
  *visual* contract — this ADR is its mechanical half: how schema gets from a
  plugin to the renderer), ADR-015 (the reader mode `nav` and `pages` entries
  declare visibility for).

## Context

ADR-005 fixed *what* a plugin emits (a widget schema, never markup) and *why*
(consistency by construction). It deliberately left the *mechanics* open: how a
plugin is discovered, how it ships, how the shell reaches it, what happens when
it dies. This ADR fixes that.

The one question the prototype exists to answer is whether the plugin contract
works when someone *other than the shell author* writes the plugin. That forces
two properties the mechanics must guarantee:

- **Crash isolation.** A third-party plugin must not be able to take the shell
  down — not by crashing, hanging, panicking, or returning garbage. LuCI's
  answer ("ship browser JS in the shell's origin") fails this: plugin code can
  hijack the session and wedge the page. Isolation is the
  whole reason to pay the cost of a separate process.
- **Language-agnosticism.** The plugin author should not be forced into Go, or
  into linking the shell. Anyone who can serve HTTP can write a plugin.

Both point at the same mechanism: **an out-of-process service the shell talks to
over a local socket**, exchanging *data*, not markup.

## Decision

1. **A plugin is an out-of-process HTTP service on a unix domain socket.** It is
   its own program, its own lifecycle (supervised by procd in production), its
   own address space. The shell never loads plugin code. HTTP/1.1 is the wire
   protocol because it is universal (any language serves it), debuggable
   (`curl --unix-socket …`), and gives us methods, paths and status codes for
   free. A unix socket (not TCP) keeps the surface local and filesystem-permissioned.

2. **A plugin ships a `manifest.json`.** The shell discovers plugins by globbing
   a plugins directory (`$VERSO_PLUGINS_DIR`, default
   `/usr/share/verso/plugins/*/manifest.json`). The manifest is the shell's only
   static knowledge of a plugin:

   ```json
   {
     "manifest_version": 1,
     "id": "system",
     "name": "System — General",
     "socket": "/var/run/verso/system.sock",
     "schema_version": 1,
     "nav": [ { "section": "System", "label": "General", "path": "/" } ],
     "acl": { "write": [ { "scope": "uci", "object": "system", "function": "write" } ] }
   }
   ```

   - `id` — stable, URL-safe; namespaces the `/plugins/<id>/` mount and the
     plugin's socket. Two manifests with the same `id` is a load error, not a
     merge.
   - `socket` — absolute path the plugin listens on. Explicit, so the shell
     never guesses and the author owns placement/permissions.
   - `schema_version` — the widget-schema vocabulary the plugin speaks (see 6).
   - `nav` — a **list** of `{section, label, path}` entries placing the plugin's
     pages in the shell's navigation; `path` is relative to the plugin mount. A
     plugin may contribute several entries (each auto-grouped under its section),
     and the shell builds the nav tree from manifests, so a plugin appears in the
     chrome without touching shell code. An entry may carry `mode`
     (`"basic"` | `"advanced"`, ADR-015): the shell renders it only in that
     reader mode; absent means both.
   - `manifest_version` — the manifest *format* version, distinct from
     `schema_version`; lets the manifest shape evolve independently of the widget
     vocabulary.
   - `acl` — the rpcd access scopes the plugin's writes need, as a `write` list of
     `{scope, object, function}` triples mirroring `session.access` (added for
     ADR-007, see Neutral below). Optional; a display-only plugin declares none.

3. **The shell is a schema gateway, not a reverse proxy.** For a browser request
   to `/plugins/<id>/<rest>`, the shell dials the plugin's socket, issues the
   corresponding HTTP request (`<rest>`, method, form body, query preserved), and
   expects a **widget-schema JSON envelope** in reply — never HTML. It decodes
   that through `internal/widget` and renders it through the *shell's* page chrome
   and styling (ADR-005). The plugin's bytes are **data the shell renders**,
   never bytes streamed to the browser. Transparent reverse-proxying is rejected
   outright: it would let a plugin put arbitrary HTML/CSS/JS into the shell's
   origin, destroying both the consistency contract (ADR-005) and the XSS
   boundary (ADR-004's `html/template` auto-escaping is only load-bearing if the
   shell, not the plugin, emits the markup).

4. **The response envelope is a widget tree, plus optional page metadata.** A
   plugin replies (to both GET and POST) with:

   ```json
   {
     "schema_version": 1,
     "title": "General",
     "widget": { "type": "card", "title": "…", "children": [ … ] }
   }
   ```

   `widget` is a single root widget (typically a `card` composing the page).
   The shell renders `widget` into the content region and uses `title` for the
   page heading. There is exactly one rendering path: the shell always just
   renders the tree it is handed.

   Beyond `widget`, the envelope carries optional page chrome the shell owns and
   the plugin only requests: `kicker` (an eyebrow above the heading) with
   `kicker_status` and `live` (a state label and a pulsing dot beside it),
   `subheading` (a lede under the heading), `width` (`narrow` | `normal` | `wide`),
   and `immediate` (the page's actions apply at once, so the staging capsule is
   omitted — ADR-010). `pages` is the **third navigation tier**: a domain's subpages
   rendered as the shell's top bar (sidebar → domain, top bar → kind of visit). Each
   entry is `{label, path}` relative to the plugin's mount, plus an optional `mode`
   filtered exactly like a manifest `nav` entry (ADR-015); the shell builds the href
   and marks the active tab, so a plugin cannot aim the bar outside itself. `banner`
   is a full-width `{variant, title, body}` notice the shell renders at the navigation
   seam — standing page state, visible above the heading. `notice` is the *outcome*
   of the action this render answers — `{level, text}`, level in the tone vocabulary
   (`success` | `warning` | `danger` | `info`) — rendered in the shell's flash slot,
   exactly where and how the shell's own confirmations appear. An outcome is stated
   as intent, never composed as widgets, so a plugin cannot get it wrong. Every
   field is optional: a plugin that sends only `schema_version`, `title`,
   and `widget` gets a plain page. A successful POST may also carry a closed-set
   `apply` intent for a typed non-UCI operation that follows the UCI apply; the
   shell accepts it only when the plugin declared its exact rpcd scope.

5. **The plugin is authoritative for its own writes and validation, and says so
   in-band.** A POST that fails validation returns HTTP **422** with the *same*
   envelope — the form re-rendered, each offending field carrying its `error` and
   its submitted `value`. A POST that succeeds returns **200** with the
   re-rendered page, its outcome stated in the envelope's `notice` (the
   live-apply gap is the author's to surface in the notice's text). The shell
   does not interpret field semantics; it renders
   whatever tree comes back. This keeps tier-3 (server-side, authoritative)
   validation entirely inside the plugin, where it belongs, while tier-1
   (declarative `datatype`) validation is expressed in the schema and can be
   checked on both ends.

6. **Two independent version numbers.** `schema_version` (widget vocabulary) and
   `manifest_version` (manifest format) evolve separately. `schema_version` is the
   enforced gate: a version the shell does not support yields a **degradation card
   in the chrome** ("plugin needs a newer Verso"), never a load failure or a 500.
   `manifest_version` is carried for the same purpose as the manifest shape grows.
   Additive vocabulary changes bump a minor; removals/renames bump a major.

7. **Crash isolation is a contract, not a hope.** Any transport failure — dial
   refused, timeout, non-JSON body, a schema the shell can't decode, a 5xx from
   the plugin — resolves to the shell rendering **"plugin unavailable" in its own
   chrome and returning 200 to the browser.** The shell never 500s, never exits,
   never blocks unbounded (the transport carries a timeout) on account of a
   plugin. This is the property that justifies the whole out-of-process design;
   it is verified hermetically (a fake transport that fails), not only by a live
   kill.

8. **The transport is an injected seam (ADR-003).** The shell depends on a
   `Transport` interface (dial a plugin socket, exchange a request for a schema
   envelope), with the real unix-socket/HTTP implementation in production and a
   fake in tests. The shell is therefore fully unit-testable — discovery, nav,
   gateway, and the isolation contract above — with no plugin process and no
   device.

9. **The wire contract is the API; the SDK is its typed mirror.** A plugin may
   be written in any language by hand-writing envelope JSON — nothing the
   contract guarantees may depend on the SDK. The Rust SDK exists to make the
   right widget as easy to emit as any other: a typed `Widget` model mirroring
   the vocabulary (the editor's completion is the catalog), never an
   escape-hatch builder set that makes `raw` the one-liner. The SDK's
   serialization is pinned to the shell's decoder by conformance fixtures — the
   SDK's tests serialize each widget kind to `testdata/`, the shell's tests
   decode every fixture — so the two catalogs cannot drift apart silently.

10. **Naming and filesystem conventions.**
   - **Program / repository:** a plugin is distributed as `verso-plugin-<name>`
     (e.g. `verso-plugin-system`) and is **its own repository and module**,
     independent of the shell's build — crash isolation extends to build
     isolation, so no plugin can break the shell's compile any more than its
     runtime. First-party plugin *sources* live in the shell repo's gitignored
     `./plugins/` directory purely for dev convenience; they are built and
     installed on their own.
   - **Manifest `id`:** the short `<name>` (e.g. `system`), **without** the
     `verso-plugin-` prefix. The id namespaces the `/plugins/<id>/` mount and the
     socket; the prefix lives on the program name, not the id.
   - **Socket:** by convention `/var/run/verso/<id>.sock` (the manifest states it
     explicitly regardless).
   - **Runtime discovery:** `/usr/share/verso/plugins/<id>/manifest.json`,
     overridable via `$VERSO_PLUGINS_DIR`. This install path is distinct from the
     `./plugins/` source directory above.

## Consequences

### Positive
- Crash isolation is structural: a separate process cannot corrupt the shell's
  memory or origin, and the transport contract (7) turns *any* plugin misbehavior
  into a contained, styled "unavailable" state.
- Language-agnostic: the plugin ABI is "serve this JSON over a unix socket,"
  reachable from Go, C, Rust, a shell script with `socat`, anything.
- The consistency contract (ADR-005) holds by construction — the shell owns every
  pixel because it owns the only rendering path (3).
- The whole shell side is hermetically testable through the transport seam (8);
  the live Docker kill is a confirming demo, not the proof.

### Costs / negatives
- A process and a socket per plugin — more moving parts than in-process, and
  procd (or equivalent) must supervise plugin lifecycle. Accepted: it is the
  price of isolation, and OpenWrt already runs procd.
- Per-request dial + HTTP has overhead a function call would not. Acceptable at
  admin-UI request rates; a connection pool is a later optimization behind the
  same seam.
- The schema gateway can only render what the widget vocabulary can express.
  Where a plugin's need outruns the vocabulary it must fall to `raw` (ADR-005) —
  and where even `raw` (display-only) cannot reach, that is a *finding* about the
  bet's limits, which is the point of the prototype.

### Neutral
- Per-plugin **privilege** is part of the contract via the manifest `acl` field
  (ADR-007), with two enforcement points. Before dispatching a state-changing
  request the shell — which holds the operator's session — probes `session.access`
  for the plugin's declared `acl.write` scopes and refuses (403, the plugin never
  dialed) a session that lacks the grant, failing closed (403 with no declared
  scopes, 503 when rpcd is unreachable). And the plugin no longer writes config
  itself: it returns a declarative `commit` intent in its envelope, and the shell
  executes it through rpcd with the operator's sid, refusing any op outside the
  plugin's declared configs. An op takes one of three shapes — set a section's
  options; create a section of a named `type` (empty `section`) and set them;
  `delete` a section outright — and a `null` among an op's `values` clears that
  one option, since an empty string is a value and unsetting is not. A typed `apply` intent is bounded the same way for
  the small closed set of non-UCI operations the shell brokers. §5 narrows accordingly — the plugin stays
  authoritative for *validation* and for *deciding* what to write, but the
  privileged write is the shell's, so a de-privileged (non-root) plugin holds no
  write access and no session credential of its own.
- Governs mechanics only; the visual/semantic rules remain ADR-005.

## Alternatives considered

- **In-process plugins (Go plugins / linked modules)** — no socket, no second
  process, a direct call. Rejected: it forfeits crash isolation (a panic or a
  wedged goroutine takes the shell with it) and language-agnosticism (author must
  write Go and match the shell's build), which are the two properties the whole
  exercise exists to prove.
- **Transparent reverse proxy** (stream the plugin's HTTP response to the
  browser) — simplest gateway. Rejected: it hands the shell's origin to plugin
  markup/JS, breaking ADR-005 consistency and reopening the XSS/session-hijack
  hole ADR-004 closes. The schema gateway is the deliberate opposite.
- **Raw framed JSON over the socket** (our own length-prefixed protocol) instead
  of HTTP — marginally leaner. Rejected: HTTP is universal and debuggable, gives
  methods/paths/status for multi-page plugins for free, and Go's stdlib speaks it
  over a unix socket with a one-line custom dialer. Not worth reinventing.
- **TCP sockets / a port per plugin** — network-reachable, needs port
  allocation and firewalling, and widens the surface past the local host.
  Rejected in favor of filesystem-permissioned unix sockets.
