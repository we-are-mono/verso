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
- **Minimal dependencies, single static binary.** Stdlib first; **no CGo, no runtime
  deps, no Node/npm.** A new dependency needs a real justification and a GPL-2.0-compatible
  license (MIT/BSD/ISC yes; **Apache-2.0 is incompatible** with GPL-2.0-only).

## Architecture (short version)

- Go, one static binary. `net/http` + `ServeMux` (routes in `internal/server/routes.go`,
  no framework); `html/template` + HTMX; Tailwind v4 (build-time, embedded CSS).
- Backend is native, no subprocess: `internal/ubus` (pure-Go ubus blob/blobmsg client),
  `internal/openwrt` (the `Backend` interface — go-uci for config, ubus for live state).
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
- **Prototype build target is arm64** (`make build`); override `GOARCH` for other targets.

## Build · test · run

```
make test     # go test ./...   — hermetic, no device needed
make build    # static binary; compiles CSS via the pinned Tailwind CLI first
make dev      # hot-reload loop against the running container
docker compose up -d --build     # boots OpenWrt + verso; serves on :8080
```

## Running OpenWrt in Docker — safety

The dev container boots real OpenWrt (procd) so the shell can reach live ubus/uci.
**Always run it non-privileged with no `/dev/watchdog` exposed** — `cap_add: [NET_ADMIN]`
+ `tmpfs: [/tmp]`, no `devices:`, and never `privileged: true`. A privileged procd boot
can grab the host's hardware watchdog and **hard-reboot the host** when the container
stops. The committed `docker-compose.yml` is already set up correctly; do not loosen it.

## Security status

The prototype runs as root and shells out with no ACLs — a deliberate spike shortcut (see
README). Don't build anything real on that assumption; privilege gating will land behind
the `Backend` interface and rpcd's ACL model.
