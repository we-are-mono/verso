# ADR-003 — Testing strategy: test-first, quality-first

- **Status:** Accepted
- **Date:** 2026-08-23
- **Deciders:** tomaz@zaman.io
- **Relates to:** ADR-001 (the ubus/uci backend is the testability seam)

## Context

Verso is currently an experiment, and the kickoff framed it as a throwaway spike
("optimize for learning speed, not durability"). We **amend that in one dimension:**
scope stays small, but **code quality is not cut.** Rationale: if the experiment pans
out we continue on *this* code — retrofitting tests onto an untested codebase is the
wrong order of work and pure technical debt, and after-the-fact tests tend to rubber-
stamp whatever the code already happens to do.

Operating principle: **cut scope, not quality.** Fewer features, each built test-first
and clean — the discovery loop stays fast because there is *less* code, not because the
code is sloppy.

This is genuinely an ADR (not just a `CONTRIBUTING` note) because the load-bearing part
is **architectural**: code is only testable-without-retrofitting if its side-effecting
dependencies sit behind seams from day one. That seam is a structural decision.

## Decision

1. **Test-first (TDD) is the default** for all first-party code we intend to keep:
   red → green → refactor. Production code is written to satisfy a failing test.

2. **Design for testability via dependency seams — the architectural core.**
   Side-effecting dependencies sit behind small Go interfaces, injected into the
   components that use them; never bare package-level side effects:
   - the **backend** (`ubus`/`uci` access — a native pure-Go ubus client per ADR-001)
     behind a `Backend` interface;
   - the **plugin transport** (unix-socket dial + schema fetch) behind an interface;
   - clock / filesystem wherever they would otherwise make a test non-hermetic.

   Real implementations in production, fakes in tests. The `Backend` interface is the
   *same* seam that keeps the native pure-Go ubus client (ADR-001) behind an abstraction —
   testability and the native-ubus client are one seam, so the indirection is not wasted.

3. **Test at the right level:**
   - **Pure logic** (validation datatypes, schema model, renderer): fast, table-driven
     unit tests; the renderer is checked with inline exact-string and substring
     assertions. **Golden files** cover the ubus wire format (`internal/ubus`).
   - **Plugin contract**: a **conformance suite** — one that runs a plugin through the
     contract and asserts it conforms, doubling as *executable `PLUGIN.md`* and a kit
     third-party authors self-check against — is not built. It is recorded in
     `docs/BACKLOG.md`.
   - **Wiring** (HTTP handlers, session, transport adapters): integration tests against
     the fakes; keep adapters thin.

4. **Hermetic + CI-ready.** `go test ./...` runs fast and needs **no real OpenWrt box**
   (the seams make that possible). A CI pipeline itself is out of weekend scope; the
   suite is written to drop into one.

5. **Coverage is an outcome, not a target.** TDD yields coverage as a side effect; we do
   not chase a percentage or write tests to game a number.

6. **The privileged companion is Rust, tested with `cargo test`.** `verso-rpcd/` is a
   separate Rust binary with its own `#[test]` suite across
   `verso-rpcd/src/{packages,ubus,main}.rs`. The seam-and-fake philosophy above governs
   the Go shell; the Rust companion is tested in its own toolchain.

## Consequences

### Positive
- No retrofit debt: a successful experiment continues on trustworthy code instead of a
  rewrite — the whole point.
- The native ubus client (ADR-001) sits behind the `Backend` interface, with the existing
  tests still valid.
- Fakes mean the suite runs on any dev machine — no device in the loop.

### Costs / negatives (honest)
- Interface seams add a little upfront indirection. Accepted — it is also the exact seam
  the native-ubus client uses, so it pays for itself.
- TDD over a still-forming schema means editing tests as the contract changes. Accepted:
  the test **is** the spec, not a frozen regression. Redesign = re-express the test first,
  then the code — never "add tests afterward." (This answers the usual "tests calcify an
  exploratory design" objection: they don't, if you edit them as spec.)

### Neutral
- Applies to the first-party shell and plugins. Third-party plugins own their tests.

## Alternatives considered
- **Test-after / retrofit once it pans out** — rejected: wrong order, incurs debt, and
  the code is often un-testable by then because the seams were never built.
- **No tests (pure throwaway spike)** — rejected: a promising result would need a rewrite
  to be trustworthy, slower overall than doing it right at small scope.
- **Mandated 100% coverage** — not adopted: coverage is an outcome of TDD, not a goal;
  a percentage target invites low-value tests.
