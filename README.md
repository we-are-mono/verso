# Verso

A web UI for OpenWrt. On devices with a real resource budget it is a **replacement** for
LuCI — a better-looking, plugin-extensible admin UI, not a sidecar to it. LuCI stays the
right choice for the constrained hardware Verso deliberately doesn't target; where Verso
runs, it's meant to be the one you use.

Verso's differentiator is its **plugin system**: third parties ship a page or set of pages
that register with the shell and look native — **without writing HTML or CSS**, and
**without being able to take the shell down**.

> **Status:** exploratory prototype, built to production quality (test-first). It serves a
> multi-page admin UI over live `ubus`/`uci` from a booted OpenWrt — login/session, staged
> config changes with device-side rollback, package/service management — styled with a
> design-token system and privilege-gated through rpcd (ADR-007). The plugin transport and
> a broad widget set are in place; the open question is still whether a *third-party* plugin
> renders native through the schema alone.

**Minimum target: 128 MB flash** (NAND-class). Flash is the binding constraint, not RAM:
the shell is a single, deliberately unconstrained Go binary, and it plus its plugins fit
128 MB with ample headroom — so the shell never fights for kilobytes. The resident memory
footprint sits well within what 128 MB-class hardware carries, so RAM is not the wall.
Below this floor, on the legacy NOR-flash tier, the shell binary simply doesn't fit, and
LuCI stays the right choice there — the same boundary the intro draws.

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

- **Language/runtime:** the shell is Go, a single statically-linked binary (~11 MB arm64),
  assets via `embed.FS`, **no CGo, no runtime dependencies.** Its privileged companion,
  `verso-rpcd`, is a small static Rust daemon (ADR-007).
- **Web layer:** stdlib `net/http` + `ServeMux` (routing table in `routes.go`, no
  framework), `html/template` (contextual auto-escaping — the safety net for
  plugin-supplied data), and **HTMX** for interactivity (no SPA, no npm/Node build).
- **Styling:** **Tailwind v4** standalone CLI (no Node) compiles `@theme` design tokens +
  component classes to an embedded stylesheet. Tokens are the consistency substrate; the
  generated CSS is committed so a bare `go build` stays self-contained.
- **Backend (native transport, rpcd-authorized):**
  - `internal/ubus` — a hand-written **pure-Go ubus client** speaking the native
    blob/blobmsg protocol over `/var/run/ubus/ubus.sock`. Wire format documented in
    `docs/ubus-protocol.md`.
  - `internal/openwrt` — the `Backend` interface (the ADR-003 test seam). Every method
    carries the operator's rpcd session id and acts through rpcd's **ACL-gated** ubus/uci
    objects — the shell reads and writes with no ambient root, so a restricted operator is
    bounded by their ACLs (ADR-007), not by Verso.
  - `verso-rpcd` — the persistent Rust root companion for the few actions rpcd's objects
    can't cover (system password, package verbs), each re-checked via `session.access`.
- **Rendering:** `internal/widget` decodes the JSON schema and renders it to auto-escaped,
  token-styled HTML through the closed widget set (`table`, `card`, `form`, `stat`, `chart`,
  `hero`, `badge`, `raw`, …).

## Repository layout

```
cmd/verso/            thin entrypoint (wire deps, serve)
internal/
  server/             HTTP shell: routes, handlers, page/nav, auth/session, Tailwind assets
  widget/             widget schema model + renderer (the closed widget set)
  openwrt/            Backend interface: rpcd-authorized ubus/uci, sid-carried (ADR-007)
  ubus/               pure-Go ubus blob/blobmsg client
  datatype/           declarative datatype validation (ADR-008)
  plugin/             plugin transport: unix-socket schema gateway (ADR-006)
  sysstat/ sensors/ telemetry/   host stat, sensor, and metric sources
  deviceicon/ version/   device-type icon by MAC OUI/hostname · build version stamp
verso-rpcd/           persistent privileged Rust companion (ADR-007): src/ + Cargo
docs/
  ADR/                architecture decision records (001–011)
  ubus-protocol.md    reverse-engineered ubus wire-format reference
docker/rootfs/        OpenWrt overlay: verso + verso-rpcd services, netfix, ACLs, config
Dockerfile, docker-compose.yml
scripts/dev.sh        hot-reload dev loop
sources/              reference clones (openwrt, luci, libubox, ubus) — gitignored
```

## Build & run

Requires Go 1.24+, `rustup` (for the `verso-rpcd` helper), Docker, and `make`. The Rust
toolchain and musl targets are provisioned automatically from `verso-rpcd/rust-toolchain.toml`;
the Tailwind CLI is auto-fetched (pinned) on first `make css`/`build`.

```sh
make test           # go test ./... + cargo test (unit-tested with fakes; no device needed)
make build          # cross-compiles both arches (compiles CSS first) ->
                    #   build/verso-{amd64,arm64}, build/verso-rpcd-{amd64,arm64}
make build-arm64    # just the device target (amd64 is the docker testbed's arch)
```

Run it against a real, booted OpenWrt in a container:

```sh
docker compose up -d --build      # boots OpenWrt (procd/ubus) + verso as a service
# -> http://<docker-host-ip>:8080   (LAN-reachable; the container hostname is verso-lab)
```

The container boots full OpenWrt (procd, netifd, dnsmasq, odhcpd, fw4) so verso talks to
live `ubus`/`uci`. `docker-compose.yml` wires it as a real router: a `wan-sim` ISP serves
it DHCP/DHCPv6-PD and a `lan-client` sits behind it, so Verso reads real WAN state, leases,
and routed traffic. It is **deliberately non-privileged with no `/dev/watchdog`** (a
privileged procd boot can grab the host watchdog and reboot the machine); `netfix` re-applies
the management IP so Docker port-publishing survives. Never add `privileged: true`.

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
| 006 | Plugin contract: manifest + unix-socket schema gateway + crash isolation |
| 007 | Privilege gating: act through rpcd ACLs with the session, not ambient root |
| 008 | Validation: shell enforces declarative datatypes, plugin owns semantic checks |
| 009 | Core navigation and the shell/plugin ownership boundary |
| 010 | Coordinated changes: prepare every owner, stage once, apply once |
| 011 | Plugin management: the shell's trust surface |

## Roadmap

Done: the ADRs through 011 · native rpcd-authorized ubus/uci backend · login/session ·
staged changes with device-side rollback · package/service management · privilege gating
and the `verso-rpcd` companion · the closed widget set and the plugin transport · Tailwind ·
OpenWrt-in-Docker router harness · hot-reload.

Open: the load-bearing bet — a *third-party* plugin rendering native through the schema
alone — and the widgets that bet still needs (e.g. live chart rendering).

## Caveats

This is an exploratory prototype: interfaces move, and not every page is built out. It is
**not** the old root-and-shell-out spike — privilege gating landed (ADR-007): the shell
runs unprivileged and every operation is authorized by rpcd's ACLs against the operator's
session. See `CONTRIBUTING.md` for the security model.

## License

GPL-2.0-only — the same license as OpenWrt itself, to ease upstreaming. See
`LICENSE`.
