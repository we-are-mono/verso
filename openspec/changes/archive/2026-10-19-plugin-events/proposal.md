# Proposal

## Why

An outcome that comes later has nowhere to go. Examples are acme renewing the router's certificate at night, failing to, or issuing one a minute after the operator applied its setup. The operator learns of any of them only by opening Access. There is no notification, no row on System → Events and no alert. A plugin can say an outcome only in the answer to a request (`notice`). Nothing lets it say "this happened" later, and nothing lets another plugin react when it does. This is the last part of the plugin contract, after slots (ADR-026) and the command registry (ADR-027).

## What Changes

- **The operator hears of it.** An outcome a plugin announces is said as the top-right notification on the open pages of every signed-in operator allowed to see it. It is a row on System → Events, and it can alert.
- **One way to announce.**
  - `verso announce <owner>.<event> [--subject <device>] [<arg>…]`, run as root, is the one writer.
  - Callers: a plugin's root half (through the SDK's `root::announce`), the privileged helper, and the router's certificate install.
  - It leaves one file in the shell's events spool, `/var/run/verso/events`.
  - It sends the event to procd as the service event `verso.<owner>.<event>`.
- **Events are declared.** A plugin's `commands.json` gains `events`. Each event has:
  - its name;
  - its sentence: English, with `%s` for its arguments, and a key of the plugin's catalog;
  - its tone;
  - the registered read whose grant lets a session see it;
  - whether it is said to the operators looking;
  - whether it alerts;
  - the events of its own it ends.

  The shell records an announced event only when its owner declares it. An event's name is its contract: a changed meaning is a new name.
- **Recorded once.**
  - The shell takes the spool every two seconds, oldest first, and records each event in history.db.
  - A crash between recording and removing never records an event twice.
  - Core kinds and plugin kinds become one table. The Events page, the events' grants and alerts all read it.
- **Said on the page someone is looking at.**
  - One stream per browser carries each recorded event to the sessions allowed to see it.
  - It is said once per browser, on a visible page, as it is recorded or when a page comes into view within a minute.
  - An act's own outcome is said at once in its place, as today. Each event is a notification of its own, stacked under it, at most three at a time.
- **Alerts.** Each installed plugin that declares an alerting event becomes a kind under System → Events → Alerts. An event that ends another closes its incident, as the internet coming back closes an outage. Incidents survive a restart of the shell, the internet's included.
- **Reactions run through procd.** A plugin reacts with `procd_add_raw_trigger "verso.<owner>.<event>"` in its init script, the way it reacts to `config.change`. A plugin's page process is never told anything: it reads the router as it is on every request.
- **The web certificate is an event.** Replacing the served certificate, by hand or by acme, records `certificate.replaced` and sends `verso.certificate.replaced`. `hotplug-call verso-certificate` with `ACTION=installed` stays as it is.
- **verso-plugin-acme announces** a certificate issued or renewed, and a failure of either.
- **VPN tunnel events are not announced.** Once a profile drops privileges, OpenVPN runs its hooks as the profile's user, and only root may announce. ADR-019's "VPN tunnel up, down" row names the VPN plugin's daemon hooks as its source, which cannot announce; it names the VPN plugin's announced events instead.
- **Not in this change.**
  - The places where the shell still names a plugin. They move through the registry and the manifest in `shell-names-no-plugin`.
  - A "config applied" event: every apply already sends OpenWrt's `config.change`, which `procd_add_config_trigger` answers.

## Capabilities

### New Capabilities
- `plugin-events`: how an event is announced, declared, recorded, shown, said, alerted and reacted to.

### Modified Capabilities
- `trusted-certificate`: acme announces its outcomes.

## Impact

- **ADRs.**
  - New ADR-028, "Events".
  - ADR-019 amended in place: event sources gain a plugin's announced events and the certificate's replacement; the VPN row's source becomes the VPN plugin's announced events; kinds become one table.
  - ADR-020 amended: a plugin's alerting events are a kind; incidents come from `ends`.
  - ADR-027 amended: the registry's `events`, and `announce` beside `own_write`.
  - ADR-017 §6 amended: `certificate.replaced`.
  - ADR-006 §4: an outcome that comes later reaches the notification as an event.
- **Shell.**
  - `cmd/verso`: the `announce` subcommand. `certificate install` announces too.
  - `internal/plugin`: the registry's `events`; core's names reserved in `Manifest.validate`.
  - `internal/events` (new): the kind table, the spool's writer and reader.
  - `internal/history`: an event recorded once by its spool name.
  - `internal/server`: the spool taker, Events rows and grants, each session's last minute of events, the `/events/stream` stream, the alert kinds.
  - `internal/alerts`: incidents for any kind that ends another, rebuilt from history at start; kinds passed through `Configure`.
  - `verso-page.js`: one stream per browser, and a notification layer that stacks events under an act's outcome.
- **DESIGN.md.** The notification's stacking rule.
- **Helper.** `access.rs` announces the hand install and the made certificate. Its init script makes the spool.
- **SDK.** `root::announce`, the registry builder's `events`, and `registry::check_catalog` for plugins' tests.
- **verso-plugin-acme.** Its declarations, its announces in `deliver`, its catalog's sentences, and a release that depends on this verso.
- **Compatibility.**
  - Older shells and helpers ignore `events` in `commands.json`.
  - A plugin that announces nothing keeps working.
  - No `PLUGIN_FLOORS` entry is needed.
  - No history.db migration: an event's words travel in its detail.
  - With `verso.alerts.kind` absent, an installed plugin's alerting events alert; the release notes say so.
