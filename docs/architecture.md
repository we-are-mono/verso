# How Verso is built

For contributors and plugin authors. To install Verso, see the [README](../README.md).

Verso is a web UI for OpenWrt that replaces LuCI on routers with room for it. Its
difference is its **plugin system**: third parties ship pages that register with the
shell and look native, without writing HTML or CSS, and without being able to take the
shell down.

**Minimum target: 128 MB flash** (NAND-class). Flash is the binding constraint, not RAM:
the shell is a single, deliberately unconstrained Go binary, and it plus its plugins fit
128 MB with ample headroom. Below this floor, on the legacy NOR-flash tier, the binary
doesn't fit, and LuCI stays the right choice there.

## The core idea

The question Verso exists to answer:

> Does the plugin contract work when someone *other than the shell author* writes the
> plugin?

The bet:

- **Plugins are out-of-process** HTTP services on unix sockets: crash isolation is the
  whole point, and it makes plugins language-agnostic.
- **Plugins return a widget *schema* (JSON); the shell renders it.** Plugins never emit
  markup or CSS. This is the risky, load-bearing bet.
- **Consistency by construction:** plugins compose a *closed set* of Verso-defined
  widgets using *semantic* props (`variant: "success"`), never presentational ones
  (colors, spacing). Two plugins by two strangers render identically.
- **A governed "raw" bridge:** when no widget fits, a plugin may emit `raw`, display-only
  Markdown rendered through Verso's own tokens (never arbitrary HTML/CSS). Raw usage is
  the demand signal for the next widget, and authors migrate off it.

See [ADR-005](ADR/005-ui-consistency-contract.md) for the full model, and
[plugins.md](plugins.md) for the contract plugin authors write against.

## Architecture

- **Language/runtime:** the shell is Go, a single statically-linked binary, assets via
  `embed.FS`, no CGo, no runtime dependencies. Its privileged companion, `verso-rpcd`, is
  a small static Rust daemon ([ADR-007](ADR/007-privilege-gating.md)).
- **Plugins:** the bundled plugins are static Rust binaries built on the plugin SDK
  (`plugins/verso-plugin-sdk`), each serving its pages on its own unix socket. The
  contract itself is language-agnostic JSON.
- **Web layer:** stdlib `net/http` + `ServeMux` (routing table in `routes.go`, no
  framework), `html/template` (contextual auto-escaping, the safety net for
  plugin-supplied data), **HTMX** for server round-trips and **Alpine** (its CSP build)
  for in-page behaviour. No SPA, no npm/Node build, no inline script.
- **Styling:** the **Tailwind v4** standalone CLI (no Node) compiles `@theme` design
  tokens and component classes to an embedded stylesheet. The generated CSS is committed
  so a bare `go build` stays self-contained.
- **Backend (native transport, rpcd-authorized):**
  - `internal/ubus`: a hand-written pure-Go ubus client speaking the native
    blob/blobmsg protocol over `/var/run/ubus/ubus.sock` ([ubus-protocol.md](ubus-protocol.md)).
  - `internal/openwrt`: the `Backend` interface. Every method carries the operator's rpcd
    session id and acts through rpcd's ACL-gated ubus/uci objects, so the shell holds no
    ambient root and a restricted operator is bounded by their ACLs, not by Verso.
  - `verso-rpcd`: the persistent Rust root companion for the few actions rpcd's objects
    can't cover (system password, package verbs), each re-checked via `session.access`.
- **Rendering:** `internal/widget` decodes the JSON schema and renders it to
  auto-escaped, token-styled HTML through the closed widget set.

## Repository layout

```
cmd/verso/            thin entrypoint (wire deps, serve)
internal/
  server/             HTTP shell: routes, handlers, page/nav, auth/session, assets
  widget/             widget schema model + renderer (the closed widget set)
  openwrt/            Backend interface: rpcd-authorized ubus/uci, sid-carried
  ubus/               pure-Go ubus blob/blobmsg client
  datatype/           declarative datatype validation
  plugin/             plugin transport: unix-socket schema gateway
  i18n/               localization, the English source as the key
  units/              how every rate and amount of data is written
  history/ usage/     the router's history and per-device usage
  updatecheck/        unattended package and firmware update checks
  sysstat/ sensors/ telemetry/   host stat, sensor and metric sources
plugins/
  verso-plugin-sdk/   the Rust plugin SDK: envelope, widgets, forms, serving
  verso-plugin-*/     core plugins shipped in the verso package: interfaces, system,
                      firewall. Optional plugins are repositories of their own
                      (we-are-mono/verso-plugin-dnsdhcp, -qos, -vpn, -ddns), checked
                      out here for `make dev` to deploy them too
verso-rpcd/           persistent privileged Rust companion
i18n/                 the shell's translation catalogs
profiles/             hardware profiles: sensors and board art per device
packaging/apk/        package install and removal scripts
docs/
  ADR/                architecture decision records, one per decision
  building.md         building, packaging, versions, and installing on a router
  plugins.md          the plugin contract for plugin authors
  ubus-protocol.md    ubus wire-format reference
openspec/             behaviour specs and change proposals
DESIGN.md, PRODUCT.md the design system and the product it serves
docker/rootfs/        OpenWrt overlay for the dev container
scripts/              dev.sh (hot-reload dev loop), version.sh (the build's version)
```

## Build, test and run

Requires Go 1.27+, `rustup` (for `verso-rpcd` and the plugins), Docker and `make`. The
Rust toolchain and musl targets are provisioned from `verso-rpcd/rust-toolchain.toml`;
the Tailwind CLI is fetched (pinned) on first build.

```sh
make test           # go test ./... + cargo test (no device needed)
make build          # cross-compiles amd64 and arm64 -> build/
make apk            # the signed router package (APK_GOARCH=amd64 for x86_64)
make version        # what this commit builds as
```

A build's version is the latest `vX.Y.Z` tag and the commits on it: the tagged commit
builds `0.5.0-r0`, the third commit after it `0.5.0-r3`. Packaging and publishing are in
[building.md](building.md).

### The dev container

```sh
docker compose up -d --build      # boots OpenWrt (procd/ubus) + verso as a service
make dev                          # watch sources -> hot-swap what changed
```

The container boots full OpenWrt (procd, netifd, dnsmasq, odhcpd, fw4), wired as a real
router: a `wan-sim` ISP serves it DHCP/DHCPv6-PD and a `lan-client` sits behind it. It is
**deliberately non-privileged with no `/dev/watchdog`**: a privileged procd boot can grab
the host's watchdog and reboot the machine. Never add `privileged: true`.

`make dev` hot-swaps CSS without a restart; the shell, `verso-rpcd`, plugins and ACLs
reload independently. In the container, `procd` may log `failed adding instance cgroup`:
Docker exposes a read-only cgroup tree, and the service still starts.

## Firewall packet logs

Firewall packet matches use a separate, bounded NFLOG buffer. They don't enter `logread`,
`dmesg` or the serial console. See [ADR-016](ADR/016-firewall-packet-log-isolation.md).
