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
  plugin's strings are translated *by the shell* at render time — the plugin binary
  stays monolingual, English, and i18n-unaware. Its translations are a separate
  data package that ships and is removed with it, so the shell applies them without
  ever owning or curating them.
- **Sessions are ephemeral** (in-memory, gone on restart — ADR-009's session
  store) and carry no preferences. There is nowhere to durably store a per-user
  language today, and no UI to set one.

## Decision

**The English source string is the catalog key.** Translation is a lookup of the
source text with the source as the fallback; a language is a flat
`{ "English source": "translation" }` map. A missing key, a missing language, or
English all render the source. Plugins and shell code keep their English strings
unchanged; localization is added around them, not woven through them.

1. **Catalogs are per-component data packages discovered at runtime.** A component
   is the shell or one plugin, and each owns its own catalog of the same shape:
   `verso-i18n-base-<code>` carries the shell's strings, `verso-i18n-<plugin>-<code>`
   carries one plugin's strings. Each apk drops a JSON map into a per-language
   directory keyed by component — `/usr/share/verso/i18n/<code>/base.json`,
   `/usr/share/verso/i18n/<code>/<plugin-id>.json`. The shell loads them all with
   the same resilient, injected-`fs.FS` glob it uses for plugin manifests (ADR-006):
   a malformed catalog is skipped and reported, never fatal, and installing one
   triggers the existing manifest rescan — no restart. English ships in each binary
   as the source; it has no catalog. **Base language codes only** (`sl`, `de`) — a
   region or script variant (`pt-br`, `zh-hant`) is rejected with a reported problem
   rather than loaded into a slot negotiation could never select; carrying full
   BCP-47 tags is a later scope expansion.

   The split is by **ownership**: a plugin's translations live with the plugin,
   authored by its author or a translator, installed and removed with it. The shell
   never curates a plugin's strings into its own catalog. It *applies* the right
   catalog per render (§5) but *owns* only `base`.

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

5. **The shell owns the render, so the shell applies the catalog — choosing it by
   what it renders.** A plugin emits English schema and declares English manifest
   metadata; nothing in the plugin is locale-aware. The translator the render uses
   is picked from the string's owner:
   - the shell's own chrome and pages → `base`;
   - a plugin's page (the widget walk of §3) and that plugin's manifest labels
     (`name`, each `nav[].label`, and `nav[].section` for display) →
     `base ⊕ <plugin>` — the plugin's catalog overlaid on the shell's, so
     shell-owned widget defaults (Save, Details, Close) still resolve while the
     plugin's own text resolves from its catalog. A key collision resolves to the
     plugin's value.
   The manifest stays English and declarative; only its known label fields are
   translated, and a `nav[].section` keeps its English value as the grouping and
   ordering key while its display is localized. There is no guessing over arbitrary
   plugin data — the manifest's translatable surface is a fixed, named set.

## Consequences

- Adding a language is a data change (an apk), never a code change. A plugin
  author writes only English and is localized for free — its language is a
  `verso-i18n-<plugin>-<code>` package anyone can author, ship, and remove
  independently of the shell and of every other plugin.
- Partial and stale catalogs degrade gracefully to English, per key. A plugin with
  no catalog for the negotiated language renders English inside an otherwise
  localized page, with no special casing.
- **Editing English copy silently orphans its translation** (the key changed, so
  the lookup misses and reverts to English). This is the accepted cost of keyless
  catalogs, and it is bounded per component — each catalog's string universe is
  small, closed, and authored beside the code that emits it. It is made *visible*
  rather than silent by the extraction step below.
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
- **Reload is an atomic snapshot swap.** A `*Bundle` holds every component's
  catalog for every installed language and is immutable after load, so a runtime
  rescan builds a fresh one and swaps the pointer *atomically* (`atomic.Pointer`);
  request handlers read the current snapshot without a lock. A render composes its
  translator from that snapshot — `base` alone for the shell's own content, `base`
  overlaid with the plugin's catalog for a plugin's — so the composition is a cheap
  per-request read, never a mutation of shared state.
- **Orphan/context drift is surfaced, not hidden.** A small extraction step emits a
  component's current source set (its strings under `t(...)` and `{{ t }}`, and a
  plugin's manifest label fields) so its catalog can be diffed against reality —
  dead keys and missing keys become visible in review rather than silent-forever.

## Alternatives considered

- **A single shell-owned catalog for every string.** One `verso-i18n-base-<code>`
  carrying the shell's *and* every plugin's strings is simpler to load — one file,
  no per-component merge. Rejected because it couples ownership to the shell: a
  plugin author cannot extend a catalog the shell ships, plugin strings pile into a
  file no one component owns, and installing or removing a plugin leaves its
  translations orphaned in the base catalog. Per-component catalogs cost only a
  keyed load and a per-render overlay, and keep each plugin's translations
  self-contained and disposable with it.
- **The plugin translates its own schema.** Pass the negotiated language into the
  plugin request and have each plugin look up its own strings (via an SDK helper)
  before emitting schema. It removes the shell's render-time walk over plugin
  content, but it makes every plugin locale-aware, needs an SDK primitive and a
  request field, and splits one page's translation across two processes for no
  gain the per-component catalog does not already give. The shell already renders
  the page; letting it apply the plugin's own catalog keeps the plugin a pure,
  English, i18n-unaware emitter.
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
