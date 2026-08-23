# ADR-006 — Mechanical plugin contract: manifest, unix-socket transport, schema gateway

- **Status:** Accepted
- **Date:** 2026-08-23
- **Deciders:** tomaz@zaman.io
- **Relates to:** ADR-001 (single static binary, crash isolation motive), ADR-003
  (the transport is a test seam), ADR-004 (stdlib `net/http`), ADR-005 (the
  *visual* contract — this ADR is its mechanical half: how schema gets from a
  plugin to the renderer).

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
     "id": "hostname",
     "name": "System — General",
     "socket": "/var/run/verso/hostname.sock",
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
     chrome without touching shell code.
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
   and design tokens (ADR-005). The plugin's bytes are **data the shell renders**,
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

5. **The plugin is authoritative for its own writes and validation, and says so
   in-band.** A POST that fails validation returns HTTP **422** with the *same*
   envelope — the form re-rendered, each offending field carrying its `error` and
   its submitted `value`. A POST that succeeds returns **200** with the
   re-rendered page (a success `badge`/`raw` note; the live-apply gap is the
   author's to surface). The shell does not interpret field semantics; it renders
   whatever tree comes back. This keeps tier-3 (server-side, authoritative)
   validation entirely inside the plugin, where it belongs, while tier-1
   (declarative `datatype`) validation is expressed in the schema and can be
   checked on both ends.

6. **Two independent version numbers, both refuse-don't-crash.** `schema_version`
   (widget vocabulary) and `manifest_version` (manifest format) evolve
   separately. The shell supports a known set of each; a value it does not
   support yields a **degradation card in the chrome** ("plugin needs a newer
   Verso"), never a load failure or a 500. Additive vocabulary changes bump a
   minor; removals/renames bump a major.

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

9. **Naming and filesystem conventions.**
   - **Program / repository:** a plugin is distributed as `verso-plugin-<name>`
     (e.g. `verso-plugin-hostname`) and is **its own repository and Go module**,
     independent of the shell's build — crash isolation extends to build
     isolation, so no plugin can break the shell's compile any more than its
     runtime. First-party plugin *sources* live in the shell repo's gitignored
     `./plugins/` directory purely for dev convenience; they are built and
     installed on their own.
   - **Manifest `id`:** the short `<name>` (e.g. `hostname`), **without** the
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
  plugin's declared configs. §5 narrows accordingly — the plugin stays
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
