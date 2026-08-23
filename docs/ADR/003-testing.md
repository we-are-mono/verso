# ADR-003 — Testing strategy: test-first, quality-first

- **Status:** Accepted
- **Date:** 2026-08-23
- **Deciders:** tomaz@zaman.io
- **Relates to:** ADR-001 (the shell-out backend becomes the testability seam)

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
   - the **backend** (`ubus`/`uci` access — shelling out per ADR-001) behind a `Backend`
     interface;
   - the **plugin transport** (unix-socket dial + schema fetch) behind an interface;
   - clock / filesystem wherever they would otherwise make a test non-hermetic.

   Real implementations in production, fakes in tests. **Bonus:** the `Backend` interface
   is the *same* seam ADR-001 needs to later swap shell-out for a native Go ubus client —
   testability and the future-native-ubus swap are one seam, so the indirection is not
   wasted.

3. **Test at the right level:**
   - **Pure logic** (validation datatypes, schema model, renderer): fast, table-driven
     unit tests; **golden files** for schema-JSON → HTML.
   - **Plugin contract**: a **conformance suite** that runs a plugin through the contract
     and asserts it conforms. It doubles as *executable `PLUGIN.md`* and the mechanical
     form of the definition of done ("a second dev builds a plugin from the docs"), and
     ships as a kit third-party authors self-check against.
   - **Wiring** (HTTP handlers, session, transport adapters): integration tests against
     the fakes; keep adapters thin.

4. **Hermetic + CI-ready.** `go test ./...` runs fast and needs **no real OpenWrt box**
   (the seams make that possible). A CI pipeline itself is out of weekend scope; the
   suite is written to drop into one.

5. **Coverage is an outcome, not a target.** TDD yields coverage as a side effect; we do
   not chase a percentage or write tests to game a number.

## Consequences

### Positive
- No retrofit debt: a successful experiment continues on trustworthy code instead of a
  rewrite — the whole point.
- The native-ubus swap (ADR-001) becomes a drop-in behind the `Backend` interface, with
  existing tests still valid.
- The conformance suite directly serves the definition of done and gives plugin authors a
  self-check.
- Fakes mean the suite runs on any dev machine — no device in the loop.

### Costs / negatives (honest)
- Interface seams add a little upfront indirection. Accepted — it is also the exact seam
  the native-ubus swap needs, so it pays for itself.
- TDD over a still-forming schema means editing tests as the contract changes. Accepted:
  the test **is** the spec, not a frozen regression. Redesign = re-express the test first,
  then the code — never "add tests afterward." (This answers the usual "tests calcify an
  exploratory design" objection: they don't, if you edit them as spec.)

### Neutral
- Applies to the first-party shell and plugins. Third-party plugins own their tests; the
  conformance kit is offered.

## Alternatives considered
- **Test-after / retrofit once it pans out** — rejected: wrong order, incurs debt, and
  the code is often un-testable by then because the seams were never built.
- **No tests (pure throwaway spike)** — rejected: a promising result would need a rewrite
  to be trustworthy, slower overall than doing it right at small scope.
- **Mandated 100% coverage** — not adopted: coverage is an outcome of TDD, not a goal;
  a percentage target invites low-value tests.
