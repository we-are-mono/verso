# Design

## Context

See proposal.md for why. What exists today:

- **Events.** The shell writes events into history.db's `events` table (`time, kind, subject, detail`), through `Store.Record`. Four callers write them, all core: wan, boot, changes and storage.
  - Kinds are bare strings, matched in four places: `eventRow` (`history_events.go`), `eventGrants` and `eventsView.show` (`config_history_grants.go`), and `Alerter.Observe` (`internal/alerts/alerts.go`).
  - An unknown kind shows its raw string, and only to a session that may read the whole of config history.
- **Alerts** read the table with a rowid cursor (`EventsAfter`, `outbox.go`) every ten seconds. Their kinds are fixed at four (`DefaultKinds`). The incident logic is written for WAN down and up alone.
- **The config spool** (`/var/run/verso/changes`) is how root tells the shell something changed. verso-rpcd renames JSON readings in, and the shell takes them every two seconds (`watchSpool`). It keeps one reading per config and pauses while an apply's rollback window is open, so it cannot carry events as it stands.
- **The top-right notification** comes from the request's own answer, or from a one-slot session flash taken by the next render (`session.go`). No stream carries it. `window.versoOutcome.show` (`verso-page.js`) is how a page says one.
- **Plugins.**
  - A plugin's page process is resident, run by procd under its own account.
  - Its root half is the same binary run as root: by the helper for a registered command, and by its hotplug and init scripts for unattended work.
  - Reactions today use OpenWrt's own channels:
    - `procd_add_config_trigger "config.change"` (acme, dnsdhcp);
    - `hotplug-call verso-certificate` with `ACTION=installed`, raised by the helper after a hand install, which it waits for (`certificate_event.rs`).
- **Certificates.** acme renews through acme-common's cron and acme-notify, which runs `hotplug-call acme` and then sends a procd service event (`acme.renew`, `acme.issue`). acme's `/etc/hotplug.d/acme/50-verso` runs `root deliver`. That records the outcome in `/var/run/verso-acme/verso`, hands the pair to `verso certificate install`, and sends the shell a SIGHUP. The SIGHUP reloads the certificate along with everything else.
- **OpenVPN** runs its hooks as the profile's user once the profile drops privileges (`user nobody`). The VPN plugin's hook only overwrites a status file. ADR-019's "VPN tunnel up, down" row was never built.

## Goals / Non-Goals

**Goals:**
- One path from "root saw it happen" to the Events page, the operator's screen, an alert and other plugins' reactions. No plugin is named in core, and no resident process is added.
- Events survive a restart of the shell and are never recorded twice.
- The kinds core and plugins record read alike everywhere.

**Non-Goals:**
- **VPN tunnel events.** An announcement is root's, and OpenVPN's later hooks may run unprivileged. The VPN plugin needs a root-side source for tunnel state before it can announce.
- **Saying the router's own events as notifications.** The internet going down, a boot and an apply keep their present surfaces.
- **Telling a plugin's page process anything.** It reads the router fresh on every request, so it has nothing to react to.
- **Refreshing a part of a page when an event arrives.** The stream carries notifications only. A drawer that refreshes itself while a request runs is a widget behaviour, and it is the owner's call.
- **Translating alerts.** They stay in English, as ADR-020 has them.
- **The certificate reload.** It stays on SIGHUP (decision 10).

## Decisions

### 1. Store and forward, no bus

Each hop is a pattern the router already runs:

| Hop | Carrier | Order and durability |
|---|---|---|
| Root → shell | the events spool, `/var/run/verso/events`: one file per announcement, named `<nanoseconds:020>-<owner>.<event>.json`, renamed into place | taken oldest first; kept across a shell restart (tmpfs, so not across a reboot); at most 256 wait |
| Within the shell | history.db's `events` table, with one writer | each reader keeps its own rowid cursor, as alerts already do |
| Shell → a session | a queue per session, at most 8 events, each kept a minute | said by the session's first open stream |
| Root → other plugins | a procd service event `verso.<owner>.<event>` | procd runs each matching trigger, with its delay |

*Alternatives:*
- **The shell listens on ubus (`ubus listen verso.*`).** Rejected for the record: ubus events are not buffered, so one sent while the shell restarts is lost. A resident ubus connection in the shell also needs its own ACL and reconnects.
- **A resident event daemon.** Rejected for memory, as ADR-027 rejected a resident root half.
- **The shell polls each plugin's reads for changes.** Rejected: that starts a process per read on a timer, and it derives events instead of being told them.

### 2. One writer: `verso announce`

The shell's own binary, run as root, writes the spool:
- `verso announce <owner>.<event> [--subject <device>] [<arg>…]` is the command.
- The SDK's `root::announce(plugin, event, subject, args)` runs `/usr/bin/verso announce` with a bound of 10 s.
- A plugin's shell script may run it directly.
- The helper runs `verso certificate announce` (decision 10).

The writer:
1. Refuses unless the effective uid is 0.
2. Reads the owner's declaration from `/usr/share/verso/plugins/<owner>/commands.json`, or core's table for core owners. It refuses an undeclared event, or arguments whose count is not the sentence's count of `%s`.
3. Checks the subject is a device name or empty, and each argument is one line of at most 256 bytes, eight at most.
4. Refuses when 256 announcements already wait.
5. Writes `{"v": 1, "time": <ns>, "kind": "<owner>.<event>", "subject": "…", "args": […]}` to a temporary file made `O_EXCL`, mode `0600`, then renames it into place.
6. Sends `ubus call service event` (decision 9).

A failed ubus send is said on stderr and still exits 0, because the event is recorded.

The spool is made by the shell's init script, root's step before it drops to `verso`: a directory `verso:verso 0700`. A sound directory is kept across restarts. Anything else at that name is removed and made again, as the helper makes `changes`: `rm` first, then a plain `mkdir` that fails on a link. Plugins run as their own accounts, so they cannot write in it. Root writes past the mode.

*Alternatives:*
- **The SDK writes the file itself.** Rejected: the format would live in Go, in the SDK's Rust and in the helper's Rust. The writer belongs beside the reader, in one codebase, with one test of the round trip.
- **A helper verb.** Rejected: unattended root halves hold no session, and the zero session cannot reach the registry (ADR-014 §3). Root needs no broker to write a file.

### 3. Events are declared in the registry; the sentence is declared, the arguments announced

`commands.json` gains `events`:

```json
"events": [
  { "name": "renewed", "sentence": "The certificate for %s was renewed.",
    "tone": "success", "read": "state", "alert": false, "ends": "renew-failed" },
  { "name": "renew-failed", "sentence": "The certificate for %s could not be renewed.",
    "tone": "danger", "read": "state", "alert": true }
]
```

- `name` matches `[a-z0-9-]{1,64}`.
- `tone` is one of `success`, `info`, `warning` or `danger`.
- `read` names a read the same registry registers.
- `ends` names another of the plugin's events.

The Go registry (`internal/plugin/registry.go`) parses `events` and drops, with a logged reason, a declaration that breaks a rule. The SDK's registry builder gains `event(…)` and refuses one the shell would drop. The helper ignores the field, as serde ignores unknown fields.

- **Why the registry, not the manifest.** The manifest is the page process's contract. Events are root's: root announces them, and the registry is the file root authored for that.
- **Why the sentence is declared.** The words are fixed per event, so the plugin's catalog and its audit hold every one. A root half cannot announce text no catalog has seen. The tone and the grant are static facts of the event, not of one announcement.
- **The vocabulary is ADR-021's.** It is the same shape as a commit's cause: an English sentence and its arguments, stored in English and translated when shown, the arguments as written.

*Alternative: no declaration, everything in the announcement.* Rejected:
- Alerts could not list a plugin's kinds before one happened.
- Visibility would be whatever each announcement said.
- A misspelt event name would become a new kind.

### 4. Core owners are reserved

`wan`, `boot`, `changes`, `storage` and `certificate` are core's. The registry parser ignores the event declarations of a plugin with one of those ids, so a plugin cannot speak as the router.

### 5. One kind table

A new package, `internal/events`, holds the `Kind`:
- its name;
- its words: core's in Go, translated through the shell's catalog; a plugin's from its declaration, translated through its catalog;
- its tone;
- its grant;
- whether it is said;
- its alert kind;
- the kind it ends.

The table is core's kinds plus each installed plugin's declared events, rebuilt on every rescan. Its consumers:
- the Events page's row;
- `eventGrants` and `eventsView.show`;
- the spool taker;
- the alerter;
- the stream.

| Kind | Grant | Said | Alert kind | Ends |
|---|---|---|---|---|
| `wan.down` | `ubus network.interface dump` | no | `wan` | — |
| `wan.up` | `ubus network.interface dump` | no | `wan` | `wan.down` |
| `boot` | `ubus system info` | no | `boot` (unclean only) | — |
| `changes.applied`, `changes.rolledback` | its config history entry | no | `rollback` (rolled back only) | — |
| `storage.*` | none | no | `storage` (sent at once) | — |
| `certificate.replaced` | `ubus verso accessCredentials` | no | — | — |
| `<plugin>.<event>` | `verso-plugin <plugin> <read>` | yes | `<plugin>` when it alerts | its `ends` |

The new probe, `accessCredentials`, joins `shellProbes`, so root's grant of it is held by the same test as the others.

A recorded plugin event's `detail` carries `{"spool", "plugin", "sentence", "args", "tone", "read"}`. The row still reads, in English, after the plugin is removed. Its visibility then falls back to the rule for unknown kinds. Chart marks stay core's (`eventMarks`).

### 6. Recorded once

The spool taker is its own loop beside `watchSpool`, started with the history (`history.go`), every two seconds. It does not pause for an apply's rollback window: an event is not a reading of config. For each file, oldest name first:

1. Check it: a regular file, not a link, owned by uid 0, at most 8 KiB, of format `v` 1.
2. Check the table declares its kind.
3. Record it with `Store.RecordOnce`, which inserts it unless an event with the same time and kind already carries that spool name. The time is indexed, so the check reads a row or two.
4. Remove the file.

A file failing a check is removed and logged. A record that fails, because history is not being saved, leaves the file for the next pass. A crash between steps 3 and 4 is caught by step 3 on the next pass, so no migration is needed.

### 7. Saying: a queue per session and one stream per page

1. **Queue.** When the taker records a said kind, it queues the event for each signed-in session. Each enqueue runs only after that session's grant probe answers yes (`grantsFor(sid)`).
   - The queue lives in `Sessions`, beside the Overview's cards: at most 8 events, each dropped a minute after it was recorded.
   - A buffered wake channel per session tells that session's streams.
2. **Stream.** Every signed-in page opens `GET /events/stream`, an event stream that wakes on the channel, not on a tick.
   - On connect and on each wake it takes the session's queue and sends each event as the shell's own `verso-flash` fragment, rendered in the request's language.
   - It re-checks the session every 30 s with a comment line, without touching the idle clock, as `serveSSE` does. That sibling is `serveSSEOn`.
   - When the session ends, the reconnect lands on the sign-in redirect and the page's EventSource closes, as the Overview's does.
3. **Browser.** `verso-page.js` hands each frame to `versoOutcome.show`. The layer says one notification at a time: one that arrives while another is shown waits until it leaves. A page's own outcome is therefore not cut short by an event arriving with it.

Draining on connect is what says an event on the next page opened within a minute. One path serves both an open page and one opened soon after, and the first stream to take the queue is the only one that says it.

*Alternatives:*
- **The flash slot.** Rejected: it holds one message, and a redirect's own outcome would overwrite an event or be overwritten by one.
- **A polling endpoint per tab.** Rejected: timer work on the router and in every tab, for something that happens a few times a day.
- **Saying an event only to the session whose act caused it.** Rejected: an unattended root half carries no session, and must not (ADR-021 decision 3). Saying it to everyone allowed to see it is true and needs no matching.

**Cost.** One goroutine and one HTTP/2 stream per open tab, which wakes only for an event or every 30 s. Over plain HTTP (the HTTPS redirect turned off) it is one of the browser's six connections to the router.

### 8. Alerts

- **Kinds.** Each installed plugin that declares an event with `alert: true` is an alert kind named by its id, labelled by its manifest `name` through its catalog.
  - `DefaultKinds` becomes the core four plus those plugin kinds, so `verso.alerts.kind` absent means every kind.
  - `stageAlerts` compares the operator's choice with that set. A plugin id left in the option after its removal matches nothing and does no harm.
- **Incidents** generalise WAN's.
  - An `incident` is keyed by its opening kind and subject.
  - A kind that another declares as its end opens one when it alerts. The ending kind closes it, with ADR-020's rules unchanged: held a minute, rewritten into one message if it ends sooner, reopened within five minutes, and a recovery message once a sent alert's incident closes.
  - The words come from the kind. WAN keeps ADR-020's titles ("the internet is down", "was down", "is back"). A plugin's incident is titled with its opening sentence, and its recovery with its ending sentence, the body giving the times and the duration.
  - An alerting kind nothing ends is one message each time, titled `<host>: <sentence>`, its body the time.
- **Boot, rollback and storage** keep their own paths.
- **Tests.** The existing alert tests stay as they are, and they hold WAN to the generalised code.

### 9. Reactions through procd

After the spool write, `verso announce` runs `ubus call service event '{"type":"verso.<owner>.<event>","data":{"subject":"…","args":[…]}}'`, bounded to 5 s. A plugin reacts in its init script's `service_triggers`:

```sh
procd_add_raw_trigger "verso.certificate.replaced" 1000 /usr/bin/verso-plugin-x root reload-tls
```

That is where it already reacts to `config.change`, with procd's delay as the debounce. It is acme-notify's own idiom: `hotplug-call` for what must act before the caller goes on, and then a service event for services to react.

`hotplug-call verso-certificate` with `ACTION=installed` stays the helper's, raised and waited for before a hand install answers. acme's hand-over must be done by then.

*Alternatives:*
- **Raise `hotplug-call verso-<owner>` for every event.** Rejected: it forks a shell and scans a directory per event, has no debounce, and is a second reaction mechanism beside the trigger a plugin already writes.
- **The shell calls each plugin's socket with the event.** Rejected: the page process is stateless, and a reaction that changes the router must run as root anyway.

### 10. The certificate's replacement

- `verso certificate install` announces `certificate.replaced` after it writes the pair, in process, with the leaf's common name and its expiry day (`2006-01-02`) as arguments.
- A new `verso certificate announce DIR` reads the pair in DIR and announces the same.
- The helper's `setWebCertificate` and `makeWebCertificate` run it after the pair is written and `verso-certificate installed` has been answered.
- `generate`, run by the init script on a first boot, announces nothing: nothing was replaced.
- The core sentence is "The web interface's certificate was replaced, for %s until %s." It is not said. A hand install's own act says its outcome, and an acme renewal is said by acme's `renewed`.

The reload stays on SIGHUP (ADR-017 §4). Serving the new pair must not depend on the history being on, and the spool taker runs only with it. The SIGHUP also rescans plugins and grants, which costs little at a renewal every couple of months.

### 11. verso-plugin-acme

- **Declarations.** `commands.json` declares `issued`, `renewed`, `issue-failed` and `renew-failed` (spec: trusted-certificate), each under its `state` read. The catalog `i18n/sl.json` gains the four sentences.
- **Announcing.** `deliver` announces at the moment it records an outcome, before it installs, with the section's domain as the argument. An announcement that fails is logged on stderr and changes nothing else: the outcome is still recorded for Access.
- **Package.** The release depends on this verso, which carries `verso announce`.

## Risks / Trade-offs

- **A chatty plugin interrupts every operator.**
  - Mitigation: a plugin declares only outcomes worth interrupting for, which `docs/plugins.md` says.
  - Mitigation: the layer says one at a time, and System → Events holds them all.
- **An event recorded twice by two shells.** Only one shell runs; procd's named instance sees to that.
- **Clock.** An event carries the root half's clock at announcement, like every event on a router without NTP.
- **A root process announces as another plugin.** Root is trusted, as with `own_write`'s plugin id. Core owners stay core's (decision 4).
- **rpcd probes per event and session.** A few sessions and a few events a day.
- **Plain HTTP costs a connection per tab.** HTTPS with the redirect is the default and serves HTTP/2.
- **An older shell with this acme.** apk refuses it by acme's dependency. Forced, `verso announce` is missing: acme logs it, and Access still shows the outcome.
- **A downgrade.** An older shell lists a plugin event by its raw kind, to whole-history readers. Rows stay.

## Migration Plan

- Nothing on the router moves or is renamed: the spool is new and in tmpfs, and the events table keeps its shape.
- Ship verso, then verso-plugin-acme, which depends on it.
- Roll back by reinstalling the earlier verso and acme. The recorded rows stay.
