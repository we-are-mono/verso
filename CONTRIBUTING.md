# Contributing to Verso

Guidance for everyone working in this repo. Read `docs/architecture.md` and the ADRs in `docs/ADR/`
first: they hold the decisions and rationale. This file is the short "how we work."

## Principles (non-negotiable)

- **Test-first.** Write the failing test, then the code. Design for testability: every
  side-effecting dependency (the ubus/uci backend, the plugin transport, clock, fs) sits
  behind an injected interface, so units test with fakes and **no device is needed**
  (ADR-003). `go test ./...` must stay hermetic.
- **Quality over scope.** This is an exploratory prototype, but code quality is not cut.
  When something is too big, **cut scope, not quality** — fewer features, each fully built
  and tested.
- **SOLID.** Small, focused types; small, role-specific interfaces; depend on interfaces,
  not concretions.
- **Minimal dependencies, static binaries.** The Go shell is stdlib-first — **no CGo, no
  runtime deps, no Node/npm** — and ships as one static binary; the privileged companion
  (`verso-rpcd`) is a small static Rust daemon (ADR-007). A new dependency needs a real
  justification and a GPL-2.0-compatible license (MIT/BSD/ISC yes; **Apache-2.0 is
  incompatible** with GPL-2.0-only).

## Architecture (short version)

- **The shell** is a Go static binary. `net/http` + `ServeMux` (routes in
  `internal/server/routes.go`, no framework); `html/template` + HTMX; Tailwind v4
  (build-time, embedded CSS) under the block contract of ADR-025 (see Styling). It runs
  **unprivileged** (ADR-007).
- **Backend, sid-gated.** `internal/openwrt` is the `Backend` interface; every method
  carries the operator's rpcd session id (`sid`) and acts through rpcd's ACL-gated objects
  — rpcd, not Verso, authorizes and executes. `internal/ubus` is the pure-Go ubus
  blob/blobmsg client underneath. The shell holds no ambient root: a restricted operator is
  limited to exactly what their ACLs allow.
- **The privileged companion.** `verso-rpcd` (Rust, at repo root) is a persistent root
  daemon on a group-protected Unix socket, owning the few actions rpcd's `uci`/`session`
  objects can't cover (system password, package verbs). Every request re-verifies the
  operator's `sid` via native ubus `session.access` before acting (ADR-007, ADR-011).
- `internal/widget` renders a JSON widget schema to auto-escaped, token-styled HTML.
- The plugin/consistency model — closed widget set, **semantic** props (never colors),
  design tokens as the substrate, and the governed **`raw`** bridge — is **ADR-005**. Read
  it before touching widgets or the schema.

## Conventions

- **ADRs:** architecturally-significant decisions go in `docs/ADR/NNN-slug.md` (3-digit
  number); follow the existing shape. `FINDINGS.md`, if present, is a temporary scratch
  file — never reference it from durable docs.
- **Licensing:** GPL-2.0-only. Every source file starts with an SPDX header:
  ```
  // SPDX-License-Identifier: GPL-2.0-only
  // SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
  ```
  (`#`, `<!-- -->`, `/* */` comment styles per file type; Markdown docs are exempt.) LuCI
  is a **behavioral spec only** — never copy its Apache-2.0 source into this tree.
- **Commits:** first line `context: summary because why`; imperative mood, **no temporal
  words** ("now/today"); keep messages short (≤ 25 lines); make small, logically-ordered
  commits that each build.
- **`make build` cross-compiles both targets** — arm64 and amd64, the two arches every
  release ships (amd64 is also the docker testbed's) — for the Go shell and the Rust
  helper alike. Narrow it with `make build-arm64` or `make build ARCHES=arm64`.

## Localization (ADR-012)

- **English is the source and the key.** Write user-facing prose once, in English,
  exactly as rendered — translation is a per-key catalog lookup with English
  fallback. Never pre-translate widget prose: the shell's render walk localizes
  every widget tree, shell-composed and plugin envelopes alike.
- **Compose through `tr`.** A string that mixes prose with data (a count, a
  timestamp, a limit) translates its *format* at the point of composition —
  `fmt.Sprintf(tr("%d pending changes"), n)` — because the walk matches only
  whole catalog keys. Plurals are flat: two keys, `"1 thing"` / `"%d things"`.
- **Machine strings never translate.** Declare data as data — machine column
  kinds on table cells; `mono`, `chip`, or `verbatim` on values — so an identity
  (an interface named `dev`, a device named "Online") can never come back as
  words. The typography contract and the translation walk share these
  declarations.
- **Client-side strings go through `T`.** `verso.js` has no translator; every
  string it writes into the page reads the `#verso-i18n` blob via `T(...)`, and a
  new one joins the key list in `renderPage`'s `jsCatalog`.
- **`make i18n-audit` is the check.** It renders every page reachable from `/`
  in each installed language and reports, exactly, the strings that fell back to
  English and the catalog keys nothing requested. A change that adds UI strings
  lands together with its `i18n/sl/base.json` entries and a clean audit.
- **A plugin's catalog travels with the plugin** — `i18n/<code>.json` beside its
  `manifest.json`, in this repo under `plugins/verso-plugin-*/i18n/`. Guidance
  for third-party authors is the Translations section of `docs/plugins.md`.

## Styling (ADR-025)

- **A look drawn in more than one place is a block.** Name it `verso-<block>`, its parts
  `verso-<block>__<element>`, its variants and states `data-verso-*` on the root, and put
  its rules in `internal/server/assets/blocks/<block>.css`, every selector starting from
  the block. Templates and client scripts name the block; they never repeat its utilities.
- **Text takes a type role.** Apply a `type-<role>` from `internal/server/assets/type.css`
  (`@apply` in a block, or the class on a one-off) and set the ink beside it; never write a
  size, weight, line height, tracking or font family. A machine-written value carries
  `data-verso-verbatim` and a count or duration `data-verso-figures`, and the role draws
  them. A text no role fits goes to DESIGN.md › Typography as a new role, with its reason.
- **Utilities are layout glue and one-offs.** At most eight on an element, no `[&…]:` or
  `!` utilities in markup; a selector belongs in a block.
- **A block adjusts another only through the custom properties that block exposes**,
  registered with `@property … inherits: false`.
- **Check what a change moved.** With `make dev` running, `make layout-snapshot
  LAYOUT_OUT=build/layout/before`, change, snapshot to `…/after`, then `make layout-diff
  FROM=build/layout/before TO=build/layout/after` names every element whose style changed.

## Build · test · run

A fresh checkout needs only Go 1.27+, `rustup`, Docker, and `make` — no host cross-gcc.
`rustup` provisions the Rust toolchain + musl targets from `verso-rpcd/rust-toolchain.toml`
on first build, and `rust-lld` (bundled with rustc) links every arch; the Tailwind CLI is
auto-fetched (pinned). Nothing machine-specific is baked in.

```
make test     # go test ./... + cargo test — hermetic, no device needed
make vuln     # govulncheck + osv-scanner against the advisory databases (needs the network)
make build    # cross-compiles verso + verso-rpcd for amd64 and arm64 (CSS first)
              #   -> build/verso-{amd64,arm64}, build/verso-rpcd-{amd64,arm64}
make dev      # hot-reload loop against the running container
docker compose up -d --build     # boots OpenWrt + verso; serves on :8080
```

### Packaging for a real router (apk)

```
make apk          # build + sign a router .apk  -> build/apk/verso-<version>.apk
make apk-publish  # make vuln, then copy it into the local dev repo + rebuild the signed index
```

`make apk` is machine-agnostic: the one input that varies per host is the OpenWrt
buildroot, from which it derives the `apk` tool and the signing key. It's auto-detected
at `~/Mono/Gateway/openwrt/source`; point it elsewhere with `OPENWRT_DIR=/path/...` on
the command line or in a gitignored `local.mk`. Root ownership of the packaged files
(which ubusd requires) is recorded via `fakeroot` — no `sudo`. The version is the
latest `vX.Y.Z` tag and the commits on it (`make version` prints it). Packages arm64
into `build/apk/aarch64_generic/` by default, `APK_GOARCH=amd64` into
`build/apk/x86_64/`; a release ships both. `make apk-publish` is the only
dev-box-specific step — it writes to `VERSO_REPO_DIR` (default `/srv/verso`). See
[docs/building.md](docs/building.md) for the full runbook and the router-side install.

## Adding a board profile

A board profile teaches the shell one device: which sensor is its CPU temperature, which
order its ports sit in, what its rear panel looks like and what the board itself looks
like. It is data, not code: one folder in `profiles/`, picked up by the build with no code
change. A board without one still works (the shell shows whatever the kernel exposes); a
profile makes it read the way its maker would name it.

The folder holds up to three files:

1. **`profile.json` (required)** — the board's identity and its sensor and port map.
   Its `id` is the board's `board_name` exactly as `ubus call system board` prints it
   (`mono,gateway-dk`; on x86 the DMI-derived `supermicro-h13sae-mf`), and `name` is what
   the Hardware page titles the board with. `processor`, `ports`, `thermal`, `power` and
   `fans` are optional; [`profiles/README.md`](profiles/README.md) documents each key and
   how to read the right values off the device.
2. **`back.svg` (optional)** — the rear panel the Overview's Ports card draws and lights
   live: one group per port with its outline and two LEDs, drawn at its own size on the
   8px grid. The contract (the groups, classes and sizes the shell relies on) is in
   [`profiles/README.md` § Back panel](profiles/README.md#back-panel-optional);
   `profiles/mono_gateway-dk/back.svg` is a worked example.
3. **`board.png` (optional)** — a photograph of the board, shown on System › Hardware
   beside what the box is made of. Make it:
   - the board alone, on a **transparent background**, cropped to its edges;
   - about **800 px wide** (it shows at up to a third of a page, sharp on a high-density
     screen);
   - small: it ships inside the firmware, so reduce it to a **256-colour palette** (a
     board photo stays sharp and lands near 150 KB);
   - a photograph you have the right to share under the project's licence (GPL-2.0).

   Without one the page says the board has no photo and links here.

To contribute one:

1. Fork the repository and add `profiles/<a-plain-folder-name>/` with the files above
   (the folder name is only storage; `id` is what matches the board).
2. Run `make test`: the profile tests check the panel art's contract and that every
   profile and photo loads.
3. Open a pull request with a screenshot of System › Hardware and the Overview's Ports
   card on the board.

## Running OpenWrt in Docker — safety

The dev container boots real OpenWrt (procd) so the shell can reach live ubus/uci.
**Always run it non-privileged with no `/dev/watchdog` exposed** — `cap_add: [NET_ADMIN]`
+ `tmpfs: [/tmp]`, no `devices:`, and never `privileged: true`. A privileged procd boot
can grab the host's hardware watchdog and **hard-reboot the host** when the container
stops. The committed `docker-compose.yml` is already set up correctly; do not loosen it.

## Security model (ADR-007)

Verso does not act as ambient root. The Go shell runs as the non-root `verso` user with
`no_new_privs` and a single capability (`CAP_NET_BIND_SERVICE`, to bind :80/:443), so it
gets no ubusd uid-0 ACL exemption and cannot write `/etc/config` directly. Every backend
operation carries the operator's rpcd session and is authorized by rpcd's sid-based ACLs —
an authenticated but read-only operator stays read-only through Verso. The few genuine root
actions live in the `verso-rpcd` companion, which re-checks the session via `session.access`
before each one. Read ADR-007 before touching auth, the `Backend` seam, or the ACL files
under `docker/rootfs/usr/share/`.
