# ADR-021 — Config history

- **Status:** Accepted
- **Date:** 2026-10-12
- **Relates to:** ADR-005 (entity panels, the closed widget set), ADR-006 (the
  describe hook and the plugin manifest), ADR-007 (every read and write through
  rpcd), ADR-010 (staged changes, apply and rollback), ADR-019 (history.db and
  its directory), ADR-020 (the rollback alert)

## Context

ADR-010 protects an apply for 30 seconds: a change that cuts the operator off is
rolled back by the router. Once an apply is confirmed, nothing records what it
did. ADR-019's events say that changes were applied and on which pages, not
which settings moved or what they held before.

The first question after something breaks is "what changed since it last
worked?" Verso cannot answer it, and neither can LuCI. Careful administrators
answer it with `git` over `/etc/config` from a shell: a task that sends the
operator back to SSH, which `PRODUCT.md` counts as a gap in Verso.

An apply has no name. It is whatever was staged when Apply was pressed: one
firewall rule, or a guest network across four configs. A name asked for on
every apply is friction on Verso's most repeated act, and what people type
under that friction ("fix", "test") reads worse than no name. Rollbacks and
edits made outside Verso arrive with no one to ask.

## Decision

### One entry per apply

When an apply is confirmed, the shell records it as an entry: every change it
carried, the value each changed option held before, and who applied it. An
apply that is rolled back is recorded too, as rolled back; it is the one most
likely to be tried again.

An entry holds:

| Field | What it is |
|---|---|
| time | when the apply was confirmed, or when its rollback window closed |
| status | `applied`, `rolledback`, or `undone` once a later entry reverses it |
| by | the rpcd session's user and the browser's address; empty for an edit made outside Verso |
| changes | each change as `uci changes` reported it (`op`, config, section, option, value), with the value it replaced |
| files | the daemon option files the apply staged (the helper's config-file families, `docs/plugins.md`), as their text before and after |
| words | each describe hook's sentences and subjects (below), as the plugin wrote them at apply time |
| relation | the entry this one undoes, returns to, or stages again, if any |
| note | the operator's optional note |

The words are kept, never asked for again. A plugin asked later describes the
config as it is then, in which a removed rule no longer has a name.

The by field records the operator's address for the apply only. It is the
administrator's own act on the router, unlike the traffic ADR-019 declines to
record.

### An entry names itself from what it touched

The describe hook (ADR-006) returns, beside each sentence, the **subject** it
is about: the entity kind (`rule`, `zone`, `network`, `device`, `wifi`, `dhcp`,
`service`, the kinds entity chips already draw) and the name a person set. A
change no description covers takes its config and section as its subject.

A row in the listing reads from its subjects:

- one change of one option reads as the edit: the subject's chip, the option's
  key chip, then the old value and the new (`Allow-Ping` `target` ACCEPT → DROP);
- otherwise the first two subjects, each with what happened to it (added,
  changed, removed), then "and N more";
- a relation follows in Meta: "undo of 14:02", "back to 9 Oct, 16:40",
  "again, from 11:47".

The review drawer's Apply has an optional **Note** field. Its placeholder is
the summary the row will show. A note appears in the entry's drawer, never in
place of the row's subjects.

### Changes made outside Verso

After each confirmed apply the shell keeps the committed contents of every
config the plugins declare, as the **current state**.

`verso-rpcd` watches `/etc/config` with inotify for as long as it runs. `uci
commit` writes a temporary file and renames it over the config
(`IN_MOVED_TO`); an editor or a script writes it in place (`IN_CLOSE_WRITE`).
Either way the watch sees it, from the `uci` command line, `vi`, a script,
LuCI or a restored backup alike. Writes to one config within two seconds of
each other are one edit. When an edit settles, the helper reads the config,
which it can do whatever the file's mode, and hands the shell its contents and
the time of the last write.

The shell compares them with the current state. While an apply's rollback
window is open, edits to the configs it carries wait until it closes, then
compare with the state it left: the new one once confirmed, the old one once
rolled back. A config that matches is Verso's own apply and is dropped; any
other is recorded as its own entry: by no one, named from its subjects as any
entry is, at the time it was written.
History therefore stays continuous whatever wrote to `/etc/config`, with nobody
signed in.

procd's `config.change` event is not the source: it reaches procd only from
rpcd and `reload_config`, so a plain `uci commit` over SSH never raises it.

An edit made while `verso-rpcd` is not running (a config restored before boot,
an edit during an upgrade) is found when the shell starts: it reads every
declared config through the helper, compares each with the current state, and
records a difference at that time.

Who made an outside edit is not known. inotify names no process, and the
`uci` process that wrote the file has exited before it could be traced to a
login. The row's By says **Edited directly**, and the entry's drawer says it was
written straight to the config: over SSH, by a script or by another tool. The
words name the way the edit arrived, never the product: the interface carries
no logo, so a reader may not know what "Verso" is.

### Secrets are not kept

A plugin's manifest lists the options that hold secrets (`wireless.*.key`,
`network.*.password`). History records that such an option changed, never its
value, before or after. Undoing an entry that changed a secret stages
everything else and leaves the secret as it is, and the drawer says so before
the act.

### Undo, Return and Stage again only stage

Every act on history writes onto the stage, through the same broker a Save
uses, and goes live only from the review drawer's Apply, with its rollback:

- **Undo** stages the reverse of one entry.
- **Return to this point** stages the reverse of every entry since, so the
  configs read as they did just after that entry.
- **Stage again** stages a rolled-back or undone entry's changes as they were.

When a later entry changed the same option, Undo says which entry and that
undoing writes over it. It never refuses (warn, don't block).

The apply that results is an entry like any other, with its relation set. An
undo makes the entry it reverses `undone`; a return makes every entry it
reverses `undone`.

### Where it is shown

- **System → Changes**, a page of its own beside Events. Entries newest first,
  in lanes by day. Columns: When, Changed, By, Status. A row whose entry is not
  in force (rolled back, undone) is marked over with the sand marker, as a
  switched-off rule is. There is no search: the listing is still, and the
  browser's find reads it.
- **An entry's drawer** has two tabs. *What changed* lists the entry's changes
  as the review drawer listed them, by page, each opening on its uci lines.
  *Since then* lists what is set differently now, net of anything later undone
  or rolled back, and carries Return to this point.
- **Entity panels** carry a History tab drawn by the shell: the entries that
  touched the panel's section, its rows reading as edits without the subject,
  since the panel is the subject.
- **Charts** mark each entry with a dashed Denim rule at its time, beside
  ADR-019's outage and boot marks; its tip opens the entry.
- **System → Events** keeps its applied and rolled-back events, and each opens
  its entry.

### Where it lives, and for how long

Entries are a `changes` table in ADR-019's `history.db`, in
`verso.history.directory` (`/data/verso` on a Mono router, which an A/B flash
keeps). ADR-019's handling of a directory that cannot be written covers it:
entries wait in memory and are written once writing works again.

The last **1000 entries** are kept, whatever their age. A config that last
changed a year ago keeps its record: config history is read when something
breaks, which can be long after the change. Beside them the table keeps the
**starting point**, the committed contents of every declared config before
the oldest kept entry. When entry 1001 is recorded, the oldest entry folds into
the starting point. On a fresh history, the starting point is the config as
Verso first finds it.

An entry is a few kilobytes; a full history stays within a few megabytes.
System → Maintenance's "History is kept in" row states the count beside the
file's size.

### Factory reset erases it by default

`firstboot` clears the overlay and leaves `/data`, so history outlives a reset.
The Factory reset form carries **Also erase the router's history**, checked by
default: `verso-rpcd` empties the history directory (changes, events, charts
and per-device usage) before `firstboot` runs. A router handed to someone else
leaves nothing of its past behind. Unchecked, the reset is recorded as an entry
made outside Verso on the next start, and Return to a point before it is
offered like any other.

## Consequences

- The describe hook gains a subject per sentence, and the plugin manifest a
  list of secret options; `docs/plugins.md` states both.
- `verso-rpcd` gains a long-running inotify watch on `/etc/config` and hands
  the shell config contents, secrets included, which the shell drops before
  anything is stored. Reading the declared configs joins the helper's
  zero-session grant (ADR-014), since the shell starts with no one signed in.
- The shell reads the committed contents of every declared config after each
  apply.
- A history that holds config also holds addresses, hostnames and PPPoE user
  names; the directory is the `verso` user's alone.
- The words of an entry stay in the language of the session that applied it.
- Entries recorded while history cannot be written are lost at a reboot, as
  ADR-019's events are.
