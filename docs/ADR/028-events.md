# ADR-028 — Events: root announces what happened, the shell records and says it

- **Status:** Accepted
- **Date:** 2026-10-19
- **Deciders:** tomaz@zaman.io
- **Relates to:**
  - ADR-006 §4: an outcome that comes after its request reaches the notification as an event.
  - ADR-019: announced events join the router's history, through one table of kinds.
  - ADR-020: a plugin's alerting events are an alert kind; incidents come from what ends them.
  - ADR-021: an event is said in the vocabulary of a commit's cause, an English sentence and its arguments.
  - ADR-027: the registry declares a plugin's events; `root::announce` stands beside `own_write`.
  - ADR-017 §6: the web certificate's replacement is an event.

## Context

A plugin says how an act went in the answer to its request. Outcomes that come later
have nowhere to go: a certificate renewed at night, one that failed to renew, a request
that finishes a minute after the operator applied it. The operator learns of them only
by opening the page that shows them. Nothing tells another plugin that they happened.

The work that sees such an outcome is a root half (ADR-027), started by a hotplug
script, a cron line or an init script. It holds no session and no connection to the
shell. The shell is the only writer of the router's history (ADR-019) and the only
place that can say anything to an operator.

## Decision

1. **Store and forward, no bus.** Each hop is one the router already runs:

   | Hop | Carrier |
   |---|---|
   | Root → shell | the events spool, `/var/run/verso/events`: one file an announcement, taken oldest first |
   | Within the shell | history.db's `events` table, one writer, each reader keeping its own cursor |
   | Shell → a browser | each session's events of the last minute, sent to the browser's one stream |
   | Root → other plugins | a procd service event, `verso.<owner>.<event>` |

   The spool keeps an announcement across a restart of the shell, at most 256 waiting.
   The helper's init makes it at boot, before any plugin's init or hotplug script can
   announce, the shell's account's, mode 0700, so no plugin's account may enter it, and
   keeps a sound one across its own restarts.

2. **One writer: `verso announce`.** The shell's own binary, run as root, writes the
   spool: `verso announce <owner>.<event> [--subject <device>] [<arg>…]`. The SDK's
   `root::announce` runs it, as may a plugin's shell script. It refuses a caller that
   is not root, an event its owner does not declare, arguments that do not fill the
   declared sentence, an argument with a control character, a subject that is not a
   device name, and a full spool. It opens the spool once, checks the open descriptor
   (a directory, the shell's account's, mode 0700) and reaches every name through it,
   so a link the shell's account stands at the spool's name takes root's write nowhere
   else. An announcement is `0644`, under a name of its time, its writer's pid and a
   random tail.

3. **Events are declared in the registry.** `commands.json` declares each event: its
   name, its English sentence with `%s` for each argument, its tone, the registered read
   whose grant lets a session see it, whether it is said (`notify`, off unless
   declared), whether it alerts, and the events of its own it ends. The sentence is
   declared and the arguments announced, so a root half says nothing its catalog has
   not seen. An event's name is its contract: the sentence may be reworded, but a
   changed meaning, or a change in its arguments' count or order, is a new name. Only
   the shell reads an event, so there is no version to negotiate.

4. **Core's names are reserved.** A plugin's id names its events and its alert kind, so
   a plugin may not take the name of one of core's event owners or alert kinds (`wan`,
   `boot`, `changes`, `rollback`, `storage`, `certificate`) or of ADR-019's events not
   yet recorded (`firmware`, `device`, `dhcp`). Of core's own kinds root announces only
   the certificate's replacement.

5. **One table of kinds.** Every kind of event, the router's own and each installed
   plugin's, is one entry: its words, its tone, its grant, whether root announces it,
   whether it is said, its alert kind and the kinds it ends. Every reader of events
   reads them through it. A plugin's event keeps the sentence and tone it was recorded
   with, so it still reads, in English, once the plugin is gone.

6. **Recorded once.** The shell takes the spool every two seconds, oldest first, and
   records each announcement unless an event of its time and kind already carries its
   spool name; then it lets the file go. A crash between the two records nothing twice.
   A kind the table lacks is looked for in its plugin's registry on disk before it is
   given up, since a plugin's upgrade may announce a new event before the rescan its
   package asks for.

7. **Said to the operators looking.** A recorded event declared `notify` is said as the
   top-right notification to each signed-in session rpcd lets see it: once per browser,
   on a page someone is looking at, as it is recorded or when a page comes into view
   within a minute. One stream per browser carries it, held by one tab and passed to the
   others. An act's own outcome keeps its place and is said at once; each event is a
   notification of its own, stacked beneath it. The router's own events are not said
   this way. Nothing matches an outcome to the act that caused it: an unattended root
   half carries no session (ADR-021 §3).

8. **Reactions run through procd.** `verso announce` sends each event to procd as the
   service event `verso.<owner>.<event>`, its data the subject and the arguments. A
   plugin reacts in its init script with `procd_add_raw_trigger`, as it reacts to
   `config.change`, procd's delay its debounce. procd hands the command it runs none of
   the event's data and runs a pending command once for several events, so a reaction
   is a command that may run any number of times and reads the router's state itself;
   the data only chooses which trigger fires. A plugin's page process is told nothing:
   it reads the router as it is on every request.

## Consequences

### Positive
- An outcome that comes at any time reaches the operator, the router's history, an
  alert and other plugins, and core names no plugin to carry it.
- No process is added: the spool, the history, procd and the browser's stream exist
  for other reasons or cost a goroutine an open browser.
- A plugin adds an event in its own release, declared beside its commands.

### Costs / negatives (honest)
- An announcement starts the shell's binary, a few tens of milliseconds of root's time
  per event.
- A plugin that declares a chatty event as said interrupts every operator allowed to
  see it. `notify` is off unless declared, and at most three events stand at once.
- Over plain HTTP, the browser's one stream is one of its six connections to the router.

## Alternatives considered

- **The shell listens on ubus (`ubus listen`).** ubus events are not buffered: one sent
  while the shell restarts is lost. The record needs the spool; procd is told as well,
  for reactions.
- **A resident event daemon.** Rejected for memory, as ADR-027 rejected a resident root
  half.
- **The shell polls plugins' reads for changes.** It starts a process per read on a
  timer, and derives events instead of being told them.
- **The SDK or the helper writes the spool.** The format would live in two or three
  codebases; the zero session reaches no helper verb a root half could use (ADR-014 §3).
- **Undeclared events, everything in the announcement.** Alerts could not list a
  plugin's kinds before one happened, visibility would be whatever each announcement
  said, and a misspelt name would be a new kind.
- **A hotplug call per event (`hotplug-call verso-<owner>`).** It forks a shell per event,
  has no debounce, and would be a second reaction mechanism beside procd's triggers.
  `hotplug-call verso-certificate` stays for the one reaction a caller waits on: a hand
  install answers only once acme has turned itself off.
- **Saying an event only to the session whose act caused it.** A root half carries no
  session and must not; saying it to everyone allowed to see it is true and needs no
  matching.
- **One notification at a time, events queued.** A danger outcome stays until it is
  closed, so an event behind one could wait without end.
