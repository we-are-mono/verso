# ADR-002 — License: GPL-2.0-only, with per-file SPDX headers

- **Status:** Accepted
- **Date:** 2026-08-23
- **Deciders:** tomaz@zaman.io

## Context

Verso's long-term intent is to be upstreamed into OpenWrt. OpenWrt core — and the Linux
kernel it runs on — is **GPL-2.0-only**. Matching the platform's license removes friction
for upstreaming and lets us reuse and contribute back OpenWrt code freely.

(LuCI itself is Apache-2.0, but Verso deliberately stands apart from LuCI and follows
OpenWrt **core**, not LuCI.)

Two sub-decisions:
- **`-only` vs `-or-later`.** OpenWrt core and the kernel are GPL-2.0-**only**. Matching
  them keeps relicensing control and avoids automatically inheriting future GPL versions.
- **SPDX tags over pasted boilerplate.** The kernel/OpenWrt convention is a single
  machine-readable `SPDX-License-Identifier` line per file, not a copied license header.
  (`GPL-2.0` bare is deprecated in SPDX; the current identifier is `GPL-2.0-only`.)

## Decision

1. License the entire Verso codebase under **GPL-2.0-only**. Ship the full text as
   `LICENSE` at the repo root — verbatim GPL-2.0, identical to OpenWrt's `COPYING`.
2. **Every source file** starts with a two-line SPDX header — license tag first,
   copyright second:
   - Go: `// SPDX-License-Identifier: GPL-2.0-only`
   - Shell / Makefile / `#`-comment: `# SPDX-License-Identifier: GPL-2.0-only`
   - HTML template: `<!-- SPDX-License-Identifier: GPL-2.0-only -->`
   - CSS: `/* SPDX-License-Identifier: GPL-2.0-only */`

   …immediately followed by the copyright line in the same comment style. Go example:

   ```go
   // SPDX-License-Identifier: GPL-2.0-only
   // SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
   ```

   Copyright holder for all first-party files: **Mono Technologies Inc.**
3. **Scope of "source file":** Go, shell, Makefiles, HTML templates, CSS, and any other
   compiled/executed/served code. **Exempt:** Markdown docs (ADRs, `PLUGIN.md`),
   config/data files (`.gitignore`, plugin `manifest.json`), and generated files.

## Consequences

### Positive
- Frictionless upstreaming into the OpenWrt tree; free two-way reuse of GPL-2.0 code.
- **Out-of-process plugins keep license freedom.** Plugins are separate programs talking
  to the shell over an arms-length unix-socket protocol — not linked into it — so they
  are generally *not* derivative works of the GPL-2.0 shell. Third-party plugins may
  therefore carry any license, reinforcing the language-agnostic, independent-plugin
  design. *(Not legal advice; the arms-length IPC boundary is the basis.)*
- SPDX tags are machine-readable → license scanners, `reuse` tooling, and SBOM
  generation work with no extra metadata.

### Costs / negatives
- Every new file needs the tag — easy to forget. Mitigate later with a one-line CI/
  pre-commit grep for the SPDX line (out of weekend scope, noted).
- **Apache-2.0 → GPL-2.0 is a one-way incompatibility.** We therefore **cannot copy LuCI
  source into Verso.** LuCI is a *behavioral spec* only — reuse datatype names and
  semantics (not copyrightable as such) and reimplement clean in Go; never paste its code.
- Today Verso is pure Go stdlib (BSD — ADR-001), so no dependency conflict. If we later
  CGo-link OpenWrt libs (the native-ubus path in ADR-001), verify `libubox`/`libubus`
  licensing before linking. Shelling out (the MVP path) sidesteps this.

### Neutral
- Applies to the shell and first-party plugins; third-party plugins choose their own
  license (see above).

## Alternatives considered

- **GPL-2.0-or-later** — allows adopting future GPL versions. Rejected: OpenWrt core and
  the kernel are `-only`; matching keeps relicensing control and avoids version drift.
- **Apache-2.0** (LuCI's license) — permissive, patent grant. Rejected: Verso follows
  OpenWrt core, not LuCI; GPL-2.0 eases upstreaming, and Apache-2.0 is one-way-incompatible
  with linking the GPL-2.0 code we may want to reuse.
- **MIT / BSD** — maximally permissive. Rejected for the same upstreaming-alignment reason.
