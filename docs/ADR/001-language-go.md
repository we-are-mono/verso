# ADR-001 — Language & runtime: Go, single static binary

- **Status:** Accepted
- **Date:** 2026-08-23
- **Deciders:** tomaz@zaman.io

## Context

Verso is a web UI for OpenWrt — on capable hardware, a replacement for LuCI — for devices
at a deliberately high floor spec: **2 GB RAM, 8 GB eMMC, aarch64 or x86_64**. Two
constraints dominate the language choice:

1. **It must run without LuCI installed.** Verso brings its own HTTP server and talks to
   base OpenWrt (`ubusd`, `uci`, `rpcd`, `procd`) directly. It cannot assume LuCI's
   Lua/JS runtime, uhttpd, or cgi-io are present.
2. **Install/packaging must be trivial.** The eventual intent is a single OpenWrt package
   that drops onto a device with no dependency chain to resolve beyond base.

The high floor spec means binary size, resident memory, and a garbage collector are
**not** constraints — the tiny-footprint discipline LuCI lives under does not apply here.
We can trade footprint for developer velocity and safety.

The shipping architecture is out-of-process plugins over unix sockets, which is
**language-agnostic by design** — this decision governs the *shell*, not plugins.

## Decision

Write the Verso shell in **Go**, compiled to a **single statically-linked binary** with
assets embedded via `embed.FS`.

The shell reaches the backend through a **native, pure-Go `ubus` client** — it speaks the
binary blob/blobmsg protocol over the ubus socket directly (`internal/ubus`), with no CGo,
no `libubus` link, and no shelling out. The privileged root actions the shell cannot take
itself live in a separate Rust companion, `verso-rpcd` (ADR-007); this decision governs the
Go shell.

## Consequences

### Positive
- One self-contained artifact per arch; `GOOS=linux GOARCH=arm64|amd64` cross-compiles
  with no toolchain gymnastics. Matches the target floor spec exactly.
- No runtime dependencies on device → directly satisfies the no-LuCI constraint and the
  trivial-packaging goal.
- `embed.FS` bakes templates, the compiled CSS, and static assets into the binary —
  nothing to lay on the filesystem, nothing to version-skew.
- `html/template` gives contextual auto-escaping — important because the shell renders
  plugin-supplied schema data, which is an injection surface into the shell's own origin.
- Standard library (`net/http`, `encoding/json`, `net` for the ubus socket) covers the
  shell with no third-party framework.
- Precedent: AdGuard Home ships exactly this way (Go, single binary) on OpenWrt-class
  hardware.

### Costs / negatives (recorded honestly)
- **A native ubus client was the cost paid.** The ubus socket speaks binary blobmsg, not
  JSON, so talking to it natively meant reimplementing blob/blobmsg in pure Go rather than
  CGo-binding libubus. That work landed (`internal/ubus`) and kept the property intact: the
  shell stays a single static binary with no CGo and no runtime deps.
- Larger binary and a GC vs C/Lua — acceptable **only because** of the high floor spec.
  This decision is coupled to that floor spec; if the floor drops, revisit.
- Go is not an OpenWrt-native contributor language the way Lua/C/ucode are; contributors
  porting from LuCI meet a new language. Mitigated: plugins may be any language, so this
  cost lands only on shell contributors.

### Neutral
- Does not constrain plugin language (out-of-process contract).
- Server-rendered `html/template` + HTMX is a separate concern (future ADR).

## Alternatives considered

- **Rust** — also a single static binary, stronger compile-time safety. Rejected for the
  spike: slower iteration and heavier onboarding, with no clear win over Go for an
  I/O-bound web shell at this floor spec. Reconsider only under a hard memory-ceiling or
  real-time requirement (absent at 2 GB). Rust does appear in the system, but not as the
  shell: the privileged companion `verso-rpcd` is Rust — a small, memory-safe root daemon
  (ADR-007) — which leaves this shell-language choice intact.
- **C** — native to OpenWrt, tiny. Rejected: memory-unsafe and slow to build a web UI in;
  LuCI itself moved *away* from C for its UI layer.
- **PHP** — Rejected: needs a PHP interpreter on the device, and its natural CGI/FPM
  deployment reintroduces a separate web server — re-coupling to exactly the LuCI-style
  stack we are shedding. Its shared-nothing, process-per-request model also fights the
  long-lived server-side state Verso holds (the `sid`, persistent plugin sockets).
- **Ruby** — Rejected: ships an interpreter plus a gem tree whose native extensions must
  be cross-built per arch; there is no single-artifact story, and OpenWrt has effectively
  no Ruby presence — a non-starter both for upstreaming and for contributors.
- **Node.js** — Rejected: bundles the V8 runtime and a `node_modules` tree — the heaviest
  install of the three — with no first-class static-binary equivalent. Its one apparent
  edge, JS everywhere, is moot: Verso is server-rendered with no plugin JS ABI (a separate
  ADR), so Node's pull toward a client-side SPA is the very architecture we rejected.

All three interpreted options fail the defining property of this decision — a single
static binary with zero runtime deps and a trivial one-package install that runs without
LuCI. The generous floor spec relaxes size and memory, **not** the packaging and
supply-chain burden of shipping a language runtime and its dependency tree.
