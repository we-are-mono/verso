# ADR-004 — Frontend stack: stdlib net/http, html/template, HTMX

- **Status:** Accepted
- **Date:** 2026-08-23
- **Deciders:** tomaz@zaman.io
- **Relates to:** ADR-001 (single static binary, minimal deps), ADR-003 (testability)

## Context

Verso is server-rendered. Coming from a heavier framework (Rails), the instinct is
to reach for a web framework, a template engine, and a JavaScript build. Verso's
constraints point the other way: Go's standard library already covers routing,
rendering and testing, and the plugin model keeps the interactive/JS surface
deliberately tiny (plugins emit a widget schema, not markup or JS).

## Decision

1. **HTTP + routing: stdlib `net/http` with `ServeMux`** (Go 1.22+ method/pattern
   routing). No web framework. The routing table lives in one scannable `routes.go`
   per server; handlers live in their feature files. Middleware (session, logging,
   recovery) is plain `http.Handler` wrappers.
2. **Rendering: `html/template`** (stdlib), assets via `embed.FS`. Its contextual
   auto-escaping is load-bearing — it is what makes rendering plugin-supplied schema
   data safe.
3. **Interactivity: HTMX**, vendored as a single file in `embed.FS` — HTML over the
   wire, no SPA, no npm/Node build, no client-side plugin ABI. (The Hotwire/Turbo
   equivalent for a Rails reader.)
4. **No JS framework, no bundler, no `node_modules`.**

## Consequences

### Positive
- Lowest contribution barrier for *shell* code: plain Go + stdlib, no framework idioms
  to learn, and the compiler checks that every route references a real handler.
- `net/http/httptest` makes handlers unit-testable with no server and no device
  (ADR-003) — already in use.
- No build step beyond `go build`; consistent with the single static binary (ADR-001).
- HTMX keeps contributors out of JavaScript; the interactive surface stays small
  because plugins emit schema, not markup.

### Costs / negatives
- `html/template` is terser and less ergonomic than ERB/JSX; the `.`-rebinding inside
  `range`/`with` (use `$` for the root) is a known newcomer trap.
- Stdlib routing has no built-in route groups or middleware chains; if the surface
  grows we add small helpers, not a framework.

### Neutral
- Applies to the shell only. Plugins never touch this stack — they emit widget schema
  (its own ADR).

## Alternatives considered

- **A web framework** (chi, gin, echo, Fiber) — nicer route-group / middleware
  ergonomics. Rejected: a dependency plus framework idioms for a small, mostly-static
  routing surface, when stdlib `ServeMux` (1.22+) already handles method routing, path
  params and precedence. `chi` stays the minimal, stdlib-compatible escape hatch if we
  ever genuinely need groups.
- **`templ`** (type-safe Go components, compiled) — better DX and compile-time safety.
  Rejected for now: a codegen build step + dependency + new syntax for a small, closed
  template set. Named as a deferred upgrade if the shell's view layer grows.
- **An SPA** (React / Vue / Svelte) — rejected outright: a JS build and a client ABI,
  the opposite of server-rendered/minimal-deps, and it would push plugins toward the
  JavaScript contract we deliberately avoid.
