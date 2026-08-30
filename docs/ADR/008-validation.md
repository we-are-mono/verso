# ADR-008 — Validation: the shell enforces declarative datatypes, the plugin owns semantic checks

- **Status:** Accepted
- **Date:** 2026-08-23
- **Deciders:** tomaz@zaman.io
- **Relates to:** ADR-005 (the visual contract — every error renders through the
  shell), ADR-006 (splits §5, "the plugin is authoritative for its own
  validation"), ADR-007 (the shell as the enforcement point — the same principle
  applied to writes).

## Context

Two kinds of validation exist, and they belong in different places:

- **Declarative** — is this value a hostname, an IP, a port? Context-free: it
  depends only on the value and its declared datatype. The shell already carries
  these as `internal/datatype` (the LuCI-named set), and a `field`/`list` already
  declares its `datatype` in the schema — but nothing enforces it. Each plugin
  re-implements the same checks in its own handler, and the shell's copy is dead.
- **Semantic** — is this port already in use, do these CIDRs overlap, is this peer
  name unique? These need domain knowledge and live state that only the plugin has.

ADR-006 §5 placed all validation in the plugin. That leaves the declarative layer
advisory (the shell renders a datatype as a hint but does not check it) and
duplicated (every plugin re-writes hostname/ip/port checks with its own wording).

## Decision

1. **The shell enforces declarative (datatype) validation.** For a state-changing
   request, the shell validates every `field`/`list` value in the plugin's
   returned schema against its declared `datatype` (via `internal/datatype`). A
   failure annotates the widget with an inline error and **blocks the write** —
   the commit does not run and the form re-renders as 422. One implementation,
   applied uniformly to every plugin.

2. **The plugin owns semantic validation.** Cross-field, uniqueness, and stateful
   rules stay in the plugin's handler, which returns 422 with its own inline
   errors. The shell cannot know these.

3. **One error model, one renderer.** Errors live in the schema — `field.error`, a
   `list`'s index-keyed `errors`, and a form-level `error` for a message that
   pertains to no single field. The shell is the only renderer (ADR-005); the
   plugin and the shell both write into the same slots, so which side produced a
   given message is invisible to the operator. The shell merges its datatype errors
   with any the plugin set, so all problems show at once, and it does not overwrite
   a plugin's more specific message on a field.

4. **This is not a defence against a malicious plugin.** The shell enforces the
   datatype the plugin *declares*; a hostile plugin declares none. The integrity of
   what is written is bounded by ADR-007 (a plugin may only write its declared
   configs, and rpcd re-checks the operator) and by the service that consumes the
   value. Shell-side validation buys consistency, deduplication, and bug-catching
   for honest plugins.

5. **The vocabulary is LuCI's datatype names, kept in `internal/datatype`; no
   validation dependency.** Plugin authors never call the validator — they declare
   a datatype *name* in their JSON schema, in any language, and the shell
   validates. So the barrier to entry is the *name vocabulary*, not a library, and
   the vocabulary OpenWrt authors already know is LuCI's (`hostname`, `fqdn`,
   `ipaddr`, `ip4addr`, `ip6addr`, `host`, `port`). `internal/datatype` implements
   that set, backed by stdlib (`net/netip` plus small label checks). A general validation library is rejected:
   it is invisible to plugin authors (who see only the declared name), its names do
   not match LuCI's, its accept/reject rules drift from LuCI's, and it adds a
   dependency for a small, closed set the stdlib already covers — the glue is less
   total code than a dependency plus the LuCI-name mapping it would still need. If
   the set later grows large or exotic, that trade-off is worth revisiting.

## Consequences

### Positive
- Declarative validation is uniform across every plugin and rendered identically,
  with no per-plugin code. A plugin declares a `datatype` and gets enforcement and
  error UX for free.
- Plugin handlers shrink to the semantic rules they alone can check; the hostname
  plugin's datatype validation disappears entirely.
- The datatype vocabulary is one shared, tested set, reused by the shell now and
  available to a client-side tier later.

### Costs
- The shell walks the returned schema on every state-changing request. The trees
  are small (an admin form), so the cost is negligible.
- The datatype set is the shell's to maintain; a plugin needing a datatype the
  shell lacks falls back to a semantic check in its own handler until the shell
  gains it.

### Neutral
- Amends ADR-006 §5: the plugin stays authoritative for *validation it alone can
  perform* and for *deciding what to write*, while the declarative layer moves to
  the shell. The mechanical contract (schema in, schema out) is unchanged.

## Alternatives considered

- **All validation in the plugin (the prior contract).** Rejected: it leaves the
  declarative layer advisory and duplicated, and every plugin re-implements the
  same hostname/ip/port checks with its own error wording.
- **All validation in the shell, including semantic.** Rejected as impossible: the
  shell does not interpret field semantics (ADR-006), so it cannot know whether a
  port is free or a name is unique. Those checks require the plugin's domain.
- **A general validation library** (e.g. struct-tag validators). Rejected: a
  dependency whose vocabulary does not match LuCI's datatype names, for a small
  closed set the stdlib already covers.
