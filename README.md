# Verso

A web UI for OpenWrt. On devices with a real resource budget it is a **replacement** for
LuCI — a better-looking, plugin-extensible admin UI, not a sidecar to it. LuCI stays the
right choice for the constrained hardware Verso deliberately doesn't target; where Verso
runs, it's meant to be the one you use.

Verso's differentiator is its **plugin system**: third parties ship a page or set of pages
that register with the shell and look native — **without writing HTML or CSS**, and
**without being able to take the shell down**.

> **Status:** exploratory prototype, built to production quality (test-first). Today it
> serves a live system-status page reading real `ubus`/`uci` from a booted OpenWrt, styled
> with a design-token system. The plugin transport and most widgets are still ahead.

**Target floor spec (deliberately high):** 2 GB RAM, 8 GB eMMC, aarch64 or x86_64. That
lets Verso be relaxed about resident memory and binary size where LuCI cannot.

## The core idea

The one question the prototype exists to answer:

> Does the plugin contract work when someone *other than the shell author* writes the
> plugin?

The bet:

- **Plugins are out-of-process** HTTP services on unix sockets — crash isolation is the
  whole point, and it makes plugins language-agnostic.
- **Plugins return a widget *schema* (JSON); the shell renders it.** Plugins never emit
  markup or CSS. This is the risky, load-bearing bet.
- **Consistency by construction:** plugins compose a *closed set* of Verso-defined
  widgets using *semantic* props (`variant: "success"`), never presentational ones
  (colors, spacing). Two plugins by two strangers render identically.
- **A governed "raw" bridge:** when no widget fits, a plugin may emit `raw` — display-only
  Markdown, rendered through Verso's own tokens (never arbitrary HTML/CSS). It's
  instrumented so it stays a *bridge*: raw usage is the demand signal for the next widget,
  and authors migrate off it. (This is the deliberate fix for the escape hatch that eroded
  LuCI's consistency.)

See `docs/ADR/005-ui-consistency-contract.md` for the full model.

## Architecture

- **Language/runtime:** Go, a single statically-linked binary (~11 MB arm64), assets via
  `embed.FS`, **no CGo, no runtime dependencies.**
- **Web layer:** stdlib `net/http` + `ServeMux` (routing table in `routes.go`, no
  framework), `html/template` (contextual auto-escaping — the safety net for
  plugin-supplied data), and **HTMX** for interactivity (no SPA, no npm/Node build).
- **Styling:** **Tailwind v4** standalone CLI (no Node) compiles `@theme` design tokens +
  component classes to an embedded stylesheet. Tokens are the consistency substrate; the
  generated CSS is committed so a bare `go build` stays self-contained.
- **Backend (native, no subprocess):**
  - `internal/ubus` — a hand-written **pure-Go ubus client** speaking the native
    blob/blobmsg protocol over `/var/run/ubus/ubus.sock` (live state, e.g. `system info`).
    Wire format documented in `docs/ubus-protocol.md`.
  - `internal/openwrt` — the `Backend` interface (the ADR-003 test seam); config is read
    straight from `/etc/config` via `go-uci`, live state via the ubus client.
- **Rendering:** `internal/widget` decodes the JSON schema and renders it to auto-escaped,
  token-styled HTML. `table` exists today.

## Repository layout

```
cmd/verso/            thin entrypoint (wire deps, serve)
internal/
  server/             HTTP shell: routes.go, handlers, page template, Tailwind assets
  widget/             widget schema model + renderer (table)
  openwrt/            Backend interface: go-uci (config) + ubus client (state)
  ubus/               pure-Go ubus blob/blobmsg client
docs/
  ADR/                architecture decision records (001–005)
  ubus-protocol.md    reverse-engineered ubus wire-format reference
docker/rootfs/        OpenWrt overlay: verso init.d service, netfix, default config
Dockerfile, docker-compose.yml
scripts/dev.sh        hot-reload dev loop
sources/              reference clones (openwrt, luci, libubox, ubus) — gitignored
```

## Build & run

Requires Go 1.24+, Docker, and `make`. The Tailwind CLI is auto-fetched (pinned) on first
`make css`/`build`.

```sh
make test           # go test ./... (unit-tested with fakes; no device needed)
make build          # static arm64 binary -> build/verso  (compiles CSS first)
```

Run it against a real, booted OpenWrt in a container:

```sh
docker compose up -d --build      # boots OpenWrt (procd/ubus) + verso as a service
# -> http://<docker-host-ip>:8080   (LAN-reachable; the container hostname is verso-lab)
```

The container runs full OpenWrt so verso talks to live `ubus`/`uci`. It is **deliberately
non-privileged with no `/dev/watchdog`** (a privileged procd boot can grab the host
watchdog and reboot the machine), OpenWrt's network stack/firewall are stripped and a
static IP is re-applied (`netfix`) so Docker port-publishing survives, and a default
`/etc/config/system` is injected. Never add `privileged: true`.

Fast inner loop:

```sh
make dev            # watch sources -> rebuild + hot-swap the binary into the container
```
Edit a `.go`/`.tmpl`/`.css`, save, refresh the browser (~3 s). No image rebuild.

## Decisions (ADRs)

| ADR | Decision |
|-----|----------|
| 001 | Language & runtime: **Go**, single static binary |
| 002 | License: **GPL-2.0-only**, per-file SPDX headers |
| 003 | **Test-first**, dependency seams so units are testable without a device |
| 004 | Frontend stack: stdlib `net/http` + `routes.go`, `html/template`, HTMX |
| 005 | UI consistency: design tokens + closed widget set + governed `raw` bridge |

## Roadmap

Done: language/license/testing/stack/consistency ADRs · native ubus/uci backend · live
system-status page · `table` widget · Tailwind · OpenWrt-in-Docker harness · hot-reload.

Next: `card` (container — proves **nesting**, still the unexercised core of the schema
bet) · `badge`/`form`/`raw` widgets · the mechanical plugin contract (manifest + socket +
schema versioning) and the first out-of-process plugin · login/session against
`session.login`.

## Caveats

This is a prototype. It currently runs **as root and shells out freely with no ACLs** —
a known, deliberate spike shortcut that must be replaced before any real deployment. The
`Backend` interface and rpcd's ACL model are where privilege gating will land.

## License

GPL-2.0-only — the same license as OpenWrt itself, to ease eventual upstreaming. See
`LICENSE`.
