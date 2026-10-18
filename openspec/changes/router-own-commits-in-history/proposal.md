# Proposal

## Why

Config history leaves out every change the router commits by itself. Installing a certificate by hand turns ACME's renewal off (`acme.verso.enabled` 1 → 0). A package that arrives switched off rewrites its own config. Choosing a theme, or moving the web interface's ports, commits `verso` through the helper. None of these leaves a row.

History then shows a state the router no longer holds, with nothing to say why. In the lab, history showed `acme.verso` turned on twice in a row and never off. The cause is that the shell takes a reading marked as the router's own into its baseline and records nothing (`config_history_direct.go`). The fix must land now: it changes the plugin SDK's `root::own_write`, which has one caller today (verso-plugin-acme) and no verso tag pinning it yet.

## What Changes

- Every commit the router makes by itself is a row in config history, never skipped. It reads as the router's act, never "Edited directly", and Notes says what caused it.
- A commit the shell made on an operator's behalf through the helper reads as that operator's, from their browser, as an apply does. Examples: the theme, the web interface's ports, a package installed from the Packages page.
- The mark that tells the helper a commit is the router's own carries its cause: a short sentence, and the plugin whose words it is.
  - **BREAKING (SDK):** `verso_plugin::root::own_write(config)` becomes `own_write(plugin, config, cause)`. verso-plugin-acme is its only caller.
  - A mark from an older plugin (an empty file) still counts. Its row has no cause.
- The helper's own writes give their causes: theme, web ports, a package arriving switched off.
- History keeps the cause in English and shows it in the viewer's language, from the shell's catalog or from the plugin's (ADR-012).
- What a package's own scripts commit while Verso installs, removes or upgrades it is part of that operation, by that operator. Notes name the package, for example "Changed by https-dns-proxy as it was installed." A cause can carry arguments, such as a package's name, which stay as written when the rest is translated.
- history.db gains the cause in migrations 010–012 (the cause, the router's own flag, the cause's arguments).
- verso-plugin-acme's hand-over gives its cause: "A certificate was installed by hand."
- Readings inside an apply's window stay the apply's, as today.

## Capabilities

### New Capabilities
- `config-history`: what config history records of each change to a config the plugins declare (applies, edits made outside Verso, and the router's own commits), and who and what each row names.

### Modified Capabilities

## Impact

- **ADRs.** ADR-021 is amended in place: the router's own commits are recorded as its acts with their causes, not taken in silently. ADR-027 names the mark's cause. ADR-012 already covers translating a plugin's words.
- **verso-rpcd.**
  - `config_watch.rs`: `own_write(config, cause)`; the mark file's cause and plugin; the spool record gains `cause` and `plugin`.
  - `access.rs` (theme, web ports) and `arrival.rs` (packages) give their causes.
- **Shell.**
  - `internal/server/config_history_direct.go`: an own reading becomes a recorded change.
  - The authored expectation for helper acts made for an operator.
  - `config_history_page.go`: the By cell for the router, and the cause in Notes.
  - `internal/history`: migration `010`, and `Change.Cause` and `CausePlugin`.
- **SDK.** `verso_plugin::root::own_write` gets its new signature, with its tests. `docs/plugins.md` changes its row.
- **verso-plugin-acme.** Its hand-over call, the cause sentence in its catalog (`i18n/sl.json`), and a release with the SDK.
- **Packaging.** Builds of verso-plugin-acme from before the change keep working: their empty marks still count. No conflict floor is needed.
