# ADR-012 — Localization: the English source is the key

- **Status:** Accepted
- **Date:** 2026-08-31
- **Deciders:** tomaz@zaman.io
- **Relates to:** ADR-004 (the shell renders HTML; plugins ship no text-bearing
  JS), ADR-005 (text arrives as semantic props in the closed widget vocabulary),
  ADR-006 (plugins emit a schema, never rendered strings; catalogs ship as data
  packages like plugins do), ADR-008 (the translation walk mirrors the datatype
  validation walk over the decoded tree), ADR-009 (the shell owns the chrome its
  own strings live in).

## Context

Verso has no localization. Every user-facing string is English, inline in Go
source or an html/template, and there is no language configuration, no
per-session locale, and a hardcoded `<html lang="en">`. LuCI ships each language
as a separate data package (`luci-i18n-base-<code>`) that drops a compiled
catalog on disk; the operator installs the ones they want. We want the same
shape for Verso — Slovenian first — without a shell rebuild to add a language.

Three facts about how Verso already works decide most of this:

- **Text reaches the browser at one boundary as data, not markup.** A plugin
  returns a widget *schema*; the shell decodes it into Go structs whose text is
  plain `string` fields, then renders. ADR-008's datatype validation already
  walks exactly that decoded tree, mutating fields in place before render. The
  same walk can translate.
- **Plugins emit English source, never rendered HTML.** Two plugins by two
  strangers render identically through the shell's templates (ADR-005/006). So a
  plugin's strings can be translated *by the shell* at render time — the plugin
  stays monolingual and English.
- **Sessions are ephemeral** (in-memory, gone on restart — ADR-009's session
  store) and carry no preferences. There is nowhere to durably store a per-user
  language today, and no UI to set one.

## Decision

**The English source string is the catalog key.** Translation is a lookup of the
source text with the source as the fallback; a language is a flat
`{ "English source": "translation" }` map. A missing key, a missing language, or
English all render the source. Plugins and shell code keep their English strings
unchanged; localization is added around them, not woven through them.

1. **Catalogs are data packages discovered at runtime.** A
   `verso-i18n-base-<code>` apk drops `<code>.json` into `/usr/share/verso/i18n/`.
   The shell loads them with the same resilient, injected-`fs.FS` glob it uses for
   plugin manifests (ADR-006): a malformed catalog is skipped and reported, never
   fatal, and installing one triggers the existing manifest rescan — no restart.
   English ships in the binary as the source; it has no catalog. **Base language
   codes only** (`sl`, `de`) — a region or script variant (`pt-br`, `zh-hant`) is
   rejected with a reported problem rather than loaded into a slot negotiation
   could never select; carrying full BCP-47 tags is a later scope expansion.

2. **Language is negotiated per request from `Accept-Language`.** The shell picks
   the best *installed* catalog for the request header (quality values honored and
   clamped, matched on the base subtag), falling back to English. This is
   stateless — no session storage, no config schema, no settings UI — and
   per-browser, so one router serves each operator their own language. An explicit
   stored preference can layer on later; it is not required to ship.

3. **Translation happens at the render boundary, in one primitive.** A single
   `t(string) string` (catalog lookup, English fallback) is wired into the four
   places text reaches the page:
   - the decoded **widget tree** — a `translateSchema` walk mirroring ADR-008's
     validation walk, run inside the renderer so every widget render is covered
     once (it also translates the string defaults the renderer injects);
   - the **envelope chrome and nav labels** the shell copies out of the schema;
   - the **shell's own Go strings** (page copy, errors, notices, flashes),
     wrapped at the site;
   - the **template-baked chrome**, via a `{{ t }}` template function. Because
     html/template binds functions at parse time, each template set is parsed
     once *per installed language* and cached; the request selects its set.

4. **English is the source of truth and the guaranteed fallback.** Nothing is
   ever blank or a raw key: an untranslated string is simply English. This is
   what lets a partial catalog, an untranslated plugin, and an English browser
   all work with no special casing.

## Consequences

- Adding a language is a data change (an apk), never a code change. A plugin
  author writes only English and is localized for free.
- Partial and stale catalogs degrade gracefully to English, per key.
- **Editing English copy silently orphans its translation** (the key changed, so
  the lookup misses and reverts to English). This is the accepted cost of keyless
  catalogs, and it is bounded — the string universe is small, closed, and
  centrally authored. It is made *visible* rather than silent by the extraction
  step below.
- **Homographs collapse.** One English "Free" or "Order" is one key and one forced
  translation; a language that distinguishes the senses will be wrong in one of
  them. Acceptable at Verso's scale (one copy author, a closed vocabulary);
  disambiguation would require message context, which this model has no seam for.
- The render path gains a per-request negotiation and per-language template
  parsing (cheap: a handful of languages, parsed once at startup and on rescan).
- Machine and structured text (a UCI `config.section = value` change line, a
  device name, an address) stays English/verbatim by design — it is not prose.

## Non-goals (for this decision)

- **Plurals.** Slovenian has four plural forms; a flat source→string map with a
  `%d`-formatted count cannot select among them. Count strings render one form
  until an *additive* `tn(one, other, n)` ngettext-style helper (with a catalog
  value that carries the forms) is added — a new primitive beside `t`, not a
  breaking change to it. Few strings are affected.
- **Interpolation.** A sentence built by concatenation is not one translatable
  unit; those few become single format strings with placeholders as they are
  localized. No general message-formatting framework is adopted here.
- **Region/script variants and a stored per-user language + picker UI.** Deferred;
  base codes and `Accept-Language` ship the feature first.

## Implementation notes

- **Catalog values are untrusted data (a separate apk supply chain) and must
  reach the browser only through html/template's contextual autoescaping — never
  as template *source* and never wrapped as `template.HTML`.** The `{{ t }}` func
  returns a plain string (autoescaped at the action) and the walk sets plain
  struct fields (rendered escaped, like the English they replace); neither injects
  a value into template text. A `%`-bearing value must never flow unchecked into a
  later `Sprintf`. This invariant is asserted at the boundary so the wiring stays
  escape-correct rather than retrofitted.
- **Reload is an atomic snapshot swap.** A `*Bundle` is immutable after load, so
  runtime rescan builds a fresh one and swaps the pointer *atomically*
  (`atomic.Pointer`); request handlers read the current snapshot without a lock.
- **Orphan/context drift is surfaced, not hidden.** A small extraction step emits
  the current source set (the strings under `t(...)` and `{{ t }}`) so a catalog
  can be diffed against reality — dead keys and missing keys become visible in
  review rather than silent-forever.

## Alternatives considered

- **Message IDs / keyed catalogs (`system.password.title`).** Stable keys
  decouple translations from English wording, so copy edits don't orphan anything.
  Rejected for now because it demands keying *every* string across the shell and
  every plugin up front, and a wrong or missing key renders a raw id — worse than
  English. Source-as-key gets a working, partial-tolerant system with near-zero
  churn; the drift cost is real but visible and bounded.
- **`golang.org/x/text` (message, language.Matcher).** Correct plural and BCP-47
  matching (including script fallback) out of the box. Rejected to keep the
  dependency surface minimal when a flat map plus a small, tested `Accept-Language`
  parser and per-language templates cover the need; revisit if plurals, region
  variants, or richer matching become load-bearing.
- **A language setting in UCI or the session.** Deterministic and user-controlled
  even on an English browser, but it needs a config source, a reader, and a
  settings surface, and the session store is ephemeral. `Accept-Language` ships
  the same outcome for most operators with none of that.
- **Client-side translation (JS).** Rejected: the shell renders HTML under a
  strict CSP, plugins emit schema rather than text, and shipping catalogs to the
  browser would duplicate them and leak untranslated frames.
