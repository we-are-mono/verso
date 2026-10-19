# Design

## Context

See proposal.md for why. What exists today:

- **Events.** The shell writes events into history.db's `events` table (`time, kind, subject, detail`), through `Store.Record`. Four callers write them, all core: wan, boot, changes and storage.
  - Kinds are bare strings, matched in four places: `eventRow` (`history_events.go`), `eventGrants` and `eventsView.show` (`config_history_grants.go`), and `Alerter.Observe` (`internal/alerts/alerts.go`).
  - An unknown kind shows its raw string, and only to a session that may read the whole of config history.
- **Alerts** read the table with a rowid cursor (`EventsAfter`, `outbox.go`) every ten seconds.
  - Their kinds are a package global of four (`DefaultKinds`).
  - The incident logic is written for WAN alone, and its state lives only in memory: `Begin` skips what was recorded before the alerter started, so a restart forgets an open outage.
- **The config spool** (`/var/run/verso/changes`) is how root tells the shell something changed. verso-rpcd checks that the directory is the shell's and not a link, then renames `0644` JSON readings into it. The shell takes them every two seconds (`watchSpool`). It keeps one reading per config and pauses while an apply's rollback window is open, so it cannot carry events as it stands.
- **The top-right notification** comes from the request's own answer, or from a one-slot session flash taken by the next render (`session.go`). No stream carries it. The layer (`verso-page.js`):
  - shows one outcome at a time, a newer one replacing the one showing;
  - keeps a danger or warning outcome until it is closed.
- **Plugins.**
  - A plugin's page process is resident, run by procd under its own account (init START=93).
  - Its root half is the same binary run as root: by the helper for a registered command, and by its hotplug and init scripts for unattended work.
  - Reactions today use OpenWrt's own channels:
    - `procd_add_config_trigger "config.change"` (acme, dnsdhcp);
    - `hotplug-call verso-certificate` with `ACTION=installed`, raised by the helper after a hand install, which it waits for (`certificate_event.rs`).
  - procd runs a trigger's command with no access to the event's data: `rule_handle_var` answers nothing, the command's output goes to `/dev/null`, and a pending run with the same argv absorbs a new one (`procd/service/trigger.c`).
- **Certificates.** acme renews through acme-common's cron and acme-notify, which runs `hotplug-call acme` and then sends a procd service event (`acme.renew`, `acme.issue`). acme's `/etc/hotplug.d/acme/50-verso` runs `root deliver`. That records the outcome in `/var/run/verso-acme/verso`, hands the pair to `verso certificate install`, and sends the shell a SIGHUP. The SIGHUP reloads the certificate along with everything else.
  - The helper's hand install writes the pair, sends the SIGHUP, and then runs `verso-certificate installed` (`main.rs`, `access.rs`).
  - acme's init runs the same delivery as a catch-up at boot, at START=93, before the shell (START=95).
- **OpenVPN** runs its hooks as the profile's user once the profile drops privileges (`user nobody`). The VPN plugin's hook only overwrites a status file.

## Goals / Non-Goals

**Goals:**
- One path from "root saw it happen" to the Events page, the operator's screen, an alert and other plugins' reactions. No plugin is named in core, and no resident process is added.
- Events survive a restart of the shell and are never recorded twice. Alert incidents survive one too.
- The kinds core and plugins record read alike everywhere.
- A contract shape that does not change when the next producer arrives: `ends` is a list, `notify` a field, and an event's name its contract.

**Non-Goals:**
- **VPN tunnel events.** An announcement is root's, and OpenVPN's hooks after a privilege drop run as the profile's user. A root-side source that could feed them is the kernel's `net` hotplug for the tunnel's device, or netifd's `iface` hotplug for an interface built on it. Choosing one is the VPN plugin's own change.
- **The places where the shell still names a plugin** (the backlog's list). Events remove none of them. They move through the registry and the manifest in a change of their own, `shell-names-no-plugin`.
- **A "config applied" event.** Every apply already sends OpenWrt's `config.change`, which a plugin reacts to with `procd_add_config_trigger`, as acme and dnsdhcp do. Verso adds nothing beside it.
- **Saying the router's own events as notifications.** The internet going down, a boot and an apply keep their present surfaces. The kind table's notify field is where that would change.
- **Telling a plugin's page process anything.** It reads the router fresh on every request, so it has nothing to react to.
- **Refreshing a part of a page when an event arrives.** The stream carries notifications only. A drawer that refreshes itself while a request runs is a widget behaviour, and it is the owner's call.
- **Translating alerts.** They stay in English, as ADR-020 has them.

## Decisions

### 1. Store and forward, no bus

Each hop is a pattern the router already runs:

| Hop | Carrier | Order and durability |
|---|---|---|
| Root → shell | the events spool, `/var/run/verso/events`: one `0644` file per announcement in a directory only the shell's account and root may enter, renamed into place | taken oldest first; kept across a shell restart (tmpfs, so not across a reboot); at most 256 wait |
| Within the shell | history.db's `events` table, with one writer | each reader keeps its own rowid cursor, as alerts already do; alert incidents are rebuilt from it |
| Shell → a browser | the session's events of the last minute, at most 8 | sent with their rowid as the event id to the browser's one stream, which shows each once |
| Root → other plugins | a procd service event `verso.<owner>.<event>` | procd runs each matching trigger's command, with its delay |

*Alternatives:*
- **The shell listens on ubus (`ubus listen verso.*`).** Rejected for the record: ubus events are not buffered, so one sent while the shell restarts is lost. A resident ubus connection in the shell also needs its own ACL and reconnects.
- **A resident event daemon.** Rejected for memory, as ADR-027 rejected a resident root half.
- **The shell polls each plugin's reads for changes.** Rejected: that starts a process per read on a timer, and it derives events instead of being told them.

### 2. One writer: `verso announce`

The shell's own binary, run as root, writes the spool:
- `verso announce <owner>.<event> [--subject <device>] [<arg>…]` is the command.
- The SDK's `root::announce(plugin, event, subject, args)` runs `/usr/bin/verso announce` with a bound of 10 s.
- A plugin's shell script may run it directly.
- `verso certificate install` and `verso certificate announce` call the same writer in process (decision 10).

The writer:
1. Refuses unless the effective uid is 0.
2. Takes the declaration from `/usr/share/verso/plugins/<owner>/commands.json`. Of core's kinds it takes only those marked announced, which is `certificate.replaced` alone. It refuses an undeclared event, and arguments whose count is not the sentence's count of `%s`.
3. Checks the subject is a device name or empty, and each argument is at most 256 bytes with no control character, eight at most. Arguments reach an alert's `Title` header, so no control character may ride along.
4. Opens the spool with `O_DIRECTORY|O_NOFOLLOW` and checks the open descriptor: a directory owned by the shell's account, mode `0700`. A link, another owner or another mode refuses.
5. Refuses when 256 announcements already wait.
6. Writes `{"v": 1, "time": <ns>, "kind": "<owner>.<event>", "subject": "…", "args": […]}` through the descriptor (`openat`, `O_EXCL|O_NOFOLLOW`, mode `0644`). It then `renameat`s it to `<ns:020>-<pid>-<random>.json`, so two announcements in one nanosecond never meet.
7. Sends `ubus call service event` (decision 9).

Every path the writer touches is reached through the descriptor it checked, never by name again. The shell's account owns the spool, and could swap a link in at that name; this stops that from making root write elsewhere. A failed ubus send is said on stderr and still exits 0, because the event is recorded.

The spool is made by the helper's init script (START=18), before any plugin's init or hotplug can announce, beside `changes` and `state`. Those two are emptied at every helper start. This one is kept when it is sound: a directory, not a link, owned by the shell's account, mode `0700`. Anything else at that name is removed and the directory made again with a plain `mkdir`, which fails on a link, as `changes` is made. Plugins run as their own accounts, so they cannot enter it. Root writes past the mode.

*Alternatives:*
- **The SDK writes the file itself.** Rejected: the format would live in Go, in the SDK's Rust and in the helper's Rust. The writer belongs beside the reader, in one codebase, with one test of the round trip.
- **A helper verb.** Rejected: unattended root halves hold no session, and the zero session cannot reach the registry (ADR-014 §3). Root needs no broker to write a file.

### 3. Events are declared in the registry; the sentence is declared, the arguments announced

`commands.json` gains `events`:

```json
"events": [
  { "name": "renewed", "sentence": "The certificate for %s was renewed.",
    "tone": "success", "read": "state", "notify": true, "alert": false,
    "ends": ["renew-failed", "issue-failed"] },
  { "name": "renew-failed", "sentence": "The certificate for %s could not be renewed.",
    "tone": "danger", "read": "state", "notify": true, "alert": true }
]
```

- `name` matches `[a-z0-9-]{1,64}`.
- `tone` is one of `success`, `info`, `warning` or `danger`.
- `read` names a read the same registry registers.
- `notify` says whether the event is said to the operators looking. Absent means no: an event is a record first.
- `alert` says whether it can alert.
- `ends` lists other events of the plugin's that it closes.

The Go registry (`internal/plugin/registry.go`) parses `events` and drops, with a logged reason, a declaration that breaks a rule. The SDK's registry builder gains `event(…)` and refuses one the shell would drop. The helper ignores the field, as serde ignores unknown fields.

- **An event's name is its contract.** Its sentence's words may be reworded, which only changes what new rows read. Its meaning, its arguments' count and their order never change under one name: a changed meaning is a new event. Nothing but the shell reads an event, so there is no version to negotiate.
- **Why the registry, not the manifest.** The manifest is the page process's contract. Events are root's: root announces them, and the registry is the file root authored for that.
- **Why the sentence is declared.** The words are fixed per event, so the plugin's catalog and its audit hold every one. A root half cannot announce text no catalog has seen. The tone, the grant and whether it is said are static facts of the event, not of one announcement.
- **The vocabulary is ADR-021's.** It is the same shape as a commit's cause: an English sentence and its arguments, stored in English and translated when shown, the arguments as written.
- **A catalog check.** The SDK gains `registry::check_catalog`, which a plugin's tests call: every declared sentence has a translation in each catalog the plugin ships, with the same count of `%s`.

*Alternative: no declaration, everything in the announcement.* Rejected:
- Alerts could not list a plugin's kinds before one happened.
- Visibility would be whatever each announcement said.
- A misspelt event name would become a new kind.

### 4. Core's names are reserved

These names are core's: the event owners `wan`, `boot`, `changes`, `storage` and `certificate`; the alert kinds `rollback` and the rest; and `firmware`, `device` and `dhcp`, the owners of ADR-019's events that are not yet recorded.

`Manifest.validate` refuses a plugin with one of these ids, as it refuses `shell` (`manifest.go`). A plugin cannot speak as the router, and its alert kind cannot merge with one of core's.

### 5. One kind table

A new package, `internal/events`, holds the `Kind`:
- its name;
- its words: core's in Go, translated through the shell's catalog; a plugin's from its declaration, translated through its catalog;
- its tone;
- its grant;
- whether it is announced (taken from the spool);
- whether it is said;
- its alert kind;
- the kinds it ends.

The table is core's kinds plus each installed plugin's declared events, rebuilt on every rescan. Its consumers:
- the Events page's row;
- `eventGrants` and `eventsView.show`;
- the spool taker;
- the alerter;
- the stream.

| Kind | Grant | Announced | Said | Alert kind | Ends |
|---|---|---|---|---|---|
| `wan.down` | `ubus network.interface dump` | no | no | `wan` | — |
| `wan.up` | `ubus network.interface dump` | no | no | `wan` | `wan.down` |
| `boot` | `ubus system info` | no | no | `boot` (unclean only) | — |
| `changes.applied`, `changes.rolledback` | its config history entry | no | no | `rollback` (rolled back only) | — |
| `storage.*` | none | no | no | `storage` (sent at once) | — |
| `certificate.replaced` | `ubus verso accessCredentials` | yes | no | — | — |
| `<plugin>.<event>` | `verso-plugin <plugin> <read>` | yes | its `notify` | `<plugin>` when it alerts | its `ends` |

The new probe, `accessCredentials`, joins `shellProbes`, so root's grant of it is held by the same test as the others.

A recorded plugin event's `detail` carries `{"spool", "plugin", "sentence", "args", "tone", "read"}`. The row still reads, in English, after the plugin is removed. Its visibility then falls back to the rule for unknown kinds. Chart marks stay core's (`eventMarks`).

### 6. Recorded once

The spool taker is its own loop beside `watchSpool`, started with the history (`history.go`), every two seconds. It does not pause for an apply's rollback window: an event is not a reading of config. For each file, oldest name first:

1. Check it: a regular file, not a link (`O_NOFOLLOW`), owned by uid 0, at most 8 KiB, of format `v` 1.
2. Check its kind is announced in the table.
   - For a plugin's kind the table does not have, re-read that plugin's `commands.json` from disk before deciding. An event a plugin's upgrade declares can arrive before the rescan its package's SIGHUP starts. The helper reads the registry on each request for the same reason.
3. Record it with `Store.RecordOnce`, which inserts it unless an event with the same time and kind already carries that spool name. The time is indexed, so the check reads a row or two.
4. Remove the file.

A file failing a check is removed and logged. A record that fails, because history is not being saved, leaves the file for the next pass. A crash between steps 3 and 4 is caught by step 3 on the next pass, so no migration is needed.

### 7. Saying: the session's last minute, one stream per browser

1. **Server.** When the taker records a said kind, it adds the event to each signed-in session whose grant probe answers yes (`grantsFor(sid)`).
   - `Sessions` keeps each session's said events of the last minute of their recording, at most 8, beside the Overview's cards. A buffered wake channel per session tells that session's streams.
   - Nothing is taken: each stream sends what is there.
2. **Stream.** `GET /events/stream` is an event stream that wakes on the channel, not on a tick.
   - On connect it sends the session's events that are newer than the `Last-Event-ID` the browser gives, then each new one. Each is sent as the shell's own `verso-flash` fragment, rendered in the request's language, with its rowid as the event id.
   - It re-checks the session every 30 s with a comment line, without touching the idle clock, as `serveSSE` does. That sibling is `serveSSEOn`.
   - When the session ends, the reconnect lands on the sign-in redirect and the EventSource closes, as the Overview's does.
3. **Browser.** One stream per browser.
   - The tab holding the Web Lock `verso-events` opens the stream and passes each event to every tab on a BroadcastChannel. When that tab closes, another takes the lock and the stream.
   - A tab shows an event only while it is visible. It first claims the event under the Web Lock `verso-event-<id>` and records the id as shown in `localStorage`.
   - A hidden tab keeps an unshown event until it becomes visible, while the event is still fresh.

   So each event is said once per browser, on a page someone is looking at, and a browser holds one stream however many tabs are open. Draining on connect is what says an event on the next page opened within its minute.
4. **The layer stacks.**
   - An act's own outcome keeps one place at the top, and the next act's outcome replaces it, as today: an operator's newer outcome supersedes their older one.
   - Each event is a notification of its own, stacked under that place, newest nearest it. Each leaves on its own timer, a danger or warning one when it is closed, and holds while the pointer or focus is on it.
   - At most three events stand. A fourth pushes out the oldest, which System → Events still lists.
   - Inside an open drawer the stack stands where the notification stands today. Each notification is read aloud through the page's announcer as it arrives.

   *Alternative: one at a time, an event waiting until the layer is free.* Rejected: a danger outcome stays until closed, so an event behind one could wait without end, and the queue would hide what happened instead of saying it.

*Alternatives:*
- **The flash slot.** Rejected: it holds one message, and a redirect's own outcome would overwrite an event or be overwritten by one.
- **Taking each event from the queue on first send.** Rejected: an event sent to a page that is unloading, or to a hidden tab, would be lost.
- **One stream per tab.** Rejected: over plain HTTP (the HTTPS redirect off, or `make dev`), a page already holding the Overview's or a table's stream would hold two of the browser's six connections to the router.
- **A polling endpoint.** Rejected: timer work on the router and in every tab, for something that happens a few times a day.
- **Saying an event only to the session whose act caused it.** Rejected: an unattended root half carries no session, and must not (ADR-021 decision 3). Saying it to everyone allowed to see it is true and needs no matching.

**Cost.** One goroutine and one stream per signed-in browser, which wakes only for an event or every 30 s.

### 8. Alerts

- **Kinds.**
  - Each installed plugin that declares an event with `alert: true` is an alert kind named by its id, labelled by its manifest `name` through its catalog.
  - The kinds are the core four plus those plugin kinds, handed to the alerter through `Configure`, never kept in a package global. `verso.alerts.kind` absent means every kind.
  - `stageAlerts` compares the operator's choice with that set. A plugin id left in the option after its removal matches nothing and does no harm.
- **Incidents** generalise WAN's.
  - An incident is keyed by its opening kind and subject.
  - A kind that another kind's `ends` lists opens one when it alerts. Any kind whose `ends` lists it closes it.
  - ADR-020's rules are unchanged: held a minute, rewritten into one message if it ends sooner, reopened within five minutes, and a recovery message once a sent alert's incident closes.
  - The words come from the kind. WAN keeps ADR-020's titles ("the internet is down", "was down", "is back"). A plugin's incident is titled with its opening sentence, and its recovery with the closing event's sentence, the body giving the times and the duration.
  - An alerting kind that nothing ends is one message each time, titled `<host>: <sentence>`, its body the time.
- **Incidents survive a restart.**
  - `Begin` rebuilds the open incidents from the events table: each opening event of a chosen kind with no closing event after it for its subject, within the 400 days events are kept. A rebuilt incident counts as alerted, so a repeat sends nothing and its close sends the recovery.
  - The one loss: an opening alert still within its minute's hold at the restart is sent as it stands, not rewritten into one message with its end.
  - This holds WAN to the same rule, which today forgets an outage across a restart.
- **Boot, rollback and storage** keep their own paths.
- **Tests.** The existing alert tests stay as they are, and they hold WAN to the generalised code.

### 9. Reactions through procd

After the spool write, `verso announce` runs `ubus call service event '{"type":"verso.<owner>.<event>","data":{"subject":"…","args":[…]}}'`, bounded to 5 s. A plugin reacts in its init script's `service_triggers`:

```sh
procd_add_raw_trigger "verso.certificate.replaced" 1000 /usr/bin/verso-plugin-x root reload-tls
```

That is where it already reacts to `config.change`, with procd's delay as the debounce. It is acme-notify's own idiom: `hotplug-call` for what must act before the caller goes on, and then a service event for services to react.

**What a reaction gets.** procd hands the command none of the event's data, sends its output to `/dev/null`, and runs a pending command once for several events. So:
- a reaction is a command that can run any number of times and reads the router's state itself;
- it logs through `logger`;
- the event's data can only choose which trigger fires, through a trigger rule's `if` conditions.

`docs/plugins.md` says so.

`hotplug-call verso-certificate` with `ACTION=installed` stays the helper's, raised and waited for before a hand install answers. acme's hand-over must be done by then.

*Alternatives:*
- **Raise `hotplug-call verso-<owner>` for every event.** Rejected: it forks a shell and scans a directory per event, has no debounce, and is a second reaction mechanism beside the trigger a plugin already writes.
- **The shell calls each plugin's socket with the event.** Rejected: the page process is stateless, and a reaction that changes the router must run as root anyway.

### 10. The certificate's replacement

- `verso certificate install` announces `certificate.replaced` after it writes the pair, in process, with the leaf's common name and its expiry day (`2006-01-02`) as arguments. An announcement that fails is said on stderr, and the install still succeeds.
- A new `verso certificate announce DIR` reads the pair in DIR and announces the same.
- The helper's `setWebCertificate` and `makeWebCertificate` run, in order:
  1. write the pair;
  2. send the SIGHUP (as today);
  3. run `verso-certificate installed` and wait for it (as today);
  4. run `verso certificate announce`.
- `generate`, run by the init script on a first boot, announces nothing: nothing was replaced.
- The core sentence is "The web interface's certificate was replaced, for %s until %s." It is not said. A hand install's own act says its outcome, and an acme renewal is said by acme's `renewed`.

The reload stays on SIGHUP (ADR-017 §4):
- acme releases in the field already send it after their install;
- serving a new pair must not wait on a spool taken every two seconds, or fail with it.

### 11. verso-plugin-acme

- **Declarations.** `commands.json` declares `issued`, `renewed`, `issue-failed` and `renew-failed` (spec: trusted-certificate), each under its `state` read and each said.
  - `issued` and `renewed` each end both failures, so a failed renewal closes when the operator's re-applied setup issues a certificate.
- **Announcing.** `deliver` announces a success after the certificate is installed and the shell signalled, and a failure as it records it, with the section's domain as the argument. A success whose install fails announces nothing: Access's Source row already says the certificate is held but not served. A failed announcement is logged on stderr and changes nothing else.
- **Catalog.** `i18n/sl.json` gains the four sentences, held by the SDK's catalog check.
- **Package.** The release depends on this verso, which carries `verso announce`.

## Risks / Trade-offs

- **A chatty plugin interrupts every operator.**
  - Mitigation: `notify` is off unless declared, and `docs/plugins.md` says an event is said only when it is worth interrupting for.
  - Mitigation: at most three events stand at once, and System → Events holds them all.
- **The browser's one stream depends on Web Locks and BroadcastChannel.** Every browser the shell supports has both. Where either is missing, the tab opens a stream of its own and shows only while visible.
- **An event recorded twice by two shells.** Only one shell runs; procd's named instance sees to that.
- **Clock.** An event carries the root half's clock at announcement, like every event on a router without NTP. Freshness is reckoned from the shell's record time.
- **A root process announces as another plugin.** Root is trusted, as with `own_write`'s plugin id. Core's names stay core's (decision 4).
- **rpcd probes per event and session.** A few sessions and a few events a day.
- **An older shell with this acme.** apk refuses it by acme's dependency. Forced, `verso announce` is missing: acme logs it, and Access still shows the outcome.
- **A downgrade.** An older shell lists a plugin event by its raw kind, to whole-history readers. Rows stay.
- **Plugin alerts on by default.** With `verso.alerts.kind` absent, an installed plugin's alerting events alert. The release notes say so.

## Migration Plan

- Nothing on the router moves or is renamed: the spool is new and in tmpfs, and the events and alerts tables keep their shape.
- Ship verso, then verso-plugin-acme, which depends on it.
- Roll back by reinstalling the earlier verso and acme. The recorded rows stay.
