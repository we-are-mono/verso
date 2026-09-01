# Contributing to Verso

Guidance for everyone working in this repo. Read `README.md` and the ADRs in `docs/ADR/`
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
  (build-time, embedded CSS). It runs **unprivileged** (ADR-007).
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
- **`make build` cross-compiles both targets** — arm64 (the device) and amd64 (the docker
  testbed) — for the Go shell and the Rust helper alike. Narrow it with `make build-arm64`
  or `make build ARCHES=arm64`.

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

## Build · test · run

A fresh checkout needs only Go 1.24+, `rustup`, Docker, and `make` — no host cross-gcc.
`rustup` provisions the Rust toolchain + musl targets from `verso-rpcd/rust-toolchain.toml`
on first build, and `rust-lld` (bundled with rustc) links every arch; the Tailwind CLI is
auto-fetched (pinned). Nothing machine-specific is baked in.

```
make test     # go test ./... + cargo test — hermetic, no device needed
make build    # cross-compiles verso + verso-rpcd for amd64 and arm64 (CSS first)
              #   -> build/verso-{amd64,arm64}, build/verso-rpcd-{amd64,arm64}
make dev      # hot-reload loop against the running container
docker compose up -d --build     # boots OpenWrt + verso; serves on :8080
```

### Packaging for a real router (apk)

```
make apk          # build + sign a router .apk  -> build/apk/verso-<version>.apk
make apk-publish  # also copy it into the local dev repo + rebuild the signed index
```

`make apk` is machine-agnostic: the one input that varies per host is the OpenWrt
buildroot, from which it derives the `apk` tool and the signing key. It's auto-detected
at `~/Mono/Gateway/openwrt/source`; point it elsewhere with `OPENWRT_DIR=/path/...` on
the command line or in a gitignored `local.mk`. Root ownership of the packaged files
(which ubusd requires) is recorded via `fakeroot` — no `sudo`. The version comes from
the committed `VERSION` file; pass `REVISION=2` to repackage the same version. Ships
arm64 by default (`APK_GOARCH=amd64` for the other). `make apk-publish` is the only
dev-box-specific step — it writes to `VERSO_REPO_DIR` (default `/srv/verso`). See
[docs/building.md](docs/building.md) for the full runbook and the router-side install.

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
