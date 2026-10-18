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
is about: its kind, from a closed set the shell draws a chip for (`rule`,
`port-forward`, `zone`, `crossing`, `network`, `route`, `device`, `wifi`,
`dhcp`, `service`, `setting`), and the name a person set, or its section where
it has none. A change no description covers, or one whose sentence names no
subject, takes its config and section as its subject.

A row in the listing reads from its subjects:

- one change of one option reads as the edit: the subject's chip, the option's
  key chip, then the old value and the new (`Allow-Ping` `target` ACCEPT → DROP);
- otherwise the first two subjects, each with what happened to it (added,
  changed, removed), then "and N more";
- a relation follows in Meta: "undo of 14:02", "back to 9 Oct, 16:40",
  "again, from 11:47".

The Pending changes page's Apply carries an optional **Note** field. It says
what it is for (why the changes are made, kept with them here) and its hint
says only that it is optional, never words that read as a note already
written. A note appears in the entry's drawer, never in place of the row's
subjects.

### Changes made outside Verso

`verso-rpcd` watches `/etc/config` with inotify for as long as it runs. `uci
commit` writes a temporary file and renames it over the config
(`IN_MOVED_TO`); an editor or a script writes it in place (`IN_CLOSE_WRITE`).
Either way the watch sees it, from the `uci` command line, `vi`, a script,
LuCI or a restored backup alike. Writes to one config within two seconds of
each other are one edit.

When an edit settles, the helper reads the config as committed (`uci -n
export` on its path, the reading that gives an apply its old values) and
replaces the value of every secret option (below) with a **fingerprint**: an
HMAC-SHA-256 under a key made once and kept readable by root alone
(`/etc/verso/fingerprint.key`). The same value always gives the same
fingerprint and a changed one another, so a changed password still shows, and
nothing gives the value back. The reading, the time of the last write and the
config go to the spool, `/var/run/verso/changes`: one file per config, renamed
into place over any reading of it not yet taken. The init script makes the
directory, owned by the shell's account; the helper refuses one that account
does not own, or a link to one. No secret leaves the helper, and no session is
asked for: the shell, which faces the network before anyone signs in, never
holds a secret without one (ADR-014).

The helper's own commits (the web listeners, a package that just arrived) are
Verso's acts, not edits made outside it: the helper marks the config before it
commits, and spools that reading as its own.

The shell keeps the last reading of every config the plugins declare in
`history.db`, as the **current state**, and takes the spool every two seconds.
While an apply's rollback window is open it takes nothing: a reading may be
the apply's write or its undoing. Once the apply is recorded, kept or rolled
back, a reading of a config it wrote, made between the apply and ten seconds
after it was recorded, is the apply's own. The first reading of a config is
where its history starts, and is kept as it is, as is Verso's own; any other
reading that differs is an entry of its own: by no one, at the time it was
written, its rows the changes `uci changes` would have listed had it been
staged, described by the plugin that writes the config, with what each
replaced. History therefore stays continuous whatever wrote to `/etc/config`,
with nobody signed in.

procd's `config.change` event is not the source: it reaches procd only from
rpcd and `reload_config`, so a plain `uci commit` over SSH never raises it.

An edit made while `verso-rpcd` or the shell is not running (a config restored
before boot, an edit during an upgrade) is found when the helper starts: it
spools every config, and the shell compares each with the reading it kept.
A secret the kept reading holds only as not kept, as a scrub leaves an option
named a secret after it was read, is nothing to compare: a fingerprint read
in its place is no edit, so an upgrade that names a secret records none.

Who made an outside edit is not known. inotify names no process, and the
`uci` process that wrote the file has exited before it could be traced to a
login. The row's By says **Edited directly**, and the entry's drawer says it was
written straight to the config: over SSH, by a script or by another tool. The
words name the way the edit arrived, never the product: the interface carries
no logo, so a reader may not know what "Verso" is.

### Secrets are not kept

The options OpenWrt's own packages keep a secret in (`network.password`,
`wireless.key`, `wireless.sae_password`, `ddns.password` and the like), each
under every name uci knows it by (`wireless.dae_secret` and
`wireless.radius_das_secret` alike), and the shell's own alert and heartbeat
addresses (`verso.url`, `verso.heartbeat`), which let whoever holds one post
to the operator's channel or keep a heartbeat alive, are
listed once, in `internal/secrets/secrets.json`, compiled into the shell and
the helper alike. A test holds the Wi-Fi network's options to OpenWrt's
`wifi-iface` schema: each whose name says it may hold a secret (secret, key,
pin, pwd, passwd, password, kh, psk) is listed, or named in the test with what
it holds instead (a key file's path, a lifetime, a switch);
`wireless.hostapd_bss_options`, whose raw hostapd lines may carry
`wpa_passphrase=` or `sae_password=`, is listed though its name does not say
so. A plugin's manifest
names any other secret its config holds, as
`config.option` (`"secrets": ["vpn.auth_pass"]`), and both add those. Shell
and helper alike take a plugin's secret only in a config the plugin's acl
writes with scope `uci`: a plugin names the secrets of the configs it writes,
never of one it only reads or another's, whose values a scrub would erase
from the whole history. Both match a manifest's keys as the shell's JSON
decoder does, in any case, and neither reads a manifest that spells a key
twice in one object, in the same case or another, whose fields the decoder
would take the last of, or merge, nor one the helper's reader refuses and the
decoder reads: a value nested deeper than 127, or a `\u` escape of half a
surrogate pair; both read the same test cases, so the
options the helper fingerprints are the ones the shell scrubs. History
records that such an option changed, never its value, before or after; a
plugin that forgets to name one of OpenWrt's still leaks nothing. A test fails
the build when a core plugin's password field writes an option neither the list
nor its manifest names.

No copy keeps the value either. A section removed is kept whole but for its
secrets, each kept as `{"secret": true}`: the option was there, what it held
is not, and the drawer lists it as not kept. The raw uci line an undescribed
row is kept as masks the value of every option the list or a manifest names,
as the review drawer's line does, besides any whose name says it is a
password or a key. A config's reading keeps a secret only as its fingerprint.

The shell scrubs `history.db` of every secret it knows when it opens the file
and each time it reads the plugins again, one scrub at a time. A rescan's
scrub runs in the background, as it answers an operator's install or remove
and a rewrite takes as long as the file is large; one asked for while
another runs is run once more after it, with the secrets known then. It has
two minutes, and as the shell stops it is cut short and none starts after,
since procd gives the shell seconds to stop. A
removed section's secrets are
kept as not kept, a change to a named option is marked a secret's and loses
its values, a verbatim row for one is worded with the value masked, and each
config's current state and starting point keeps no value of one. A history
written before a secret was named, by an older shell or before the plugin
naming it arrived, is so scrubbed once the name is known. SQLite keeps what a
row held in the pages it frees and in its write-ahead log, so a scrub that
wrote any row then rewrites the file (`VACUUM`) and empties the log. The
rows commit first, and with them a mark in the file that the rewrite is owed
(`rewrite_owed`); the mark goes only once the rewrite and the emptied log
both succeed. A rewrite that fails or never finishes (a full disk, the power
going, the scrub's time running out) leaves the mark, and every later scrub
makes the rewrite, though it finds no row to write, until one succeeds; the
start's scrub has no deadline. A rewrite that fails is told: the storage
alert (ADR-020) goes out at the first failure after the shell starts, or
after a scrub made the rewrite, not at each failure after it, so once each
start while the rewrite keeps failing; the Changes page says so, and why,
until a scrub makes it. A scrub cut short, by a rescan's
deadline or the shell stopping, failed at nothing and is only logged: what it
left undone the next scrub does, the start's at the latest, and a rewrite
that start's scrub cannot make is told. Once the rewrite is made, no byte of
the file or its log holds a secret a scrub took out. Scrubbing what is
scrubbed, with no rewrite
owed, writes nothing, the file and its log included. `secure_delete` stays
off: nothing written after a scrub holds a secret, so it would only add flash
writes.

Undoing an entry that changed a secret stages everything else and leaves the
secret as it is, and the drawer says so before the act. Undoing the removal of
a section makes it again without its secrets, which stay unset.

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

### A session reads only the configs rpcd lets it read

`history.db` is the shell's own store, read with no rpcd call, so each session
reads of it only what its rpcd ACL lets it read (ADR-007): the shell asks
`session.access` for `uci <config> read`, once per config per page. An uci
change to any other config is left out, a row left with none goes, and an
entry left with none is not there for the session, who applied it and from
where with it; its drawer answers as one no longer kept. The starting point and
the current readings hold only readable configs, and the starting point's row is
listed, and its drawer opens, only when one is left. Where rpcd does not answer,
history is withheld the same way, and the page says the permissions could not
be checked. Verso's own `verso-admin` group grants `uci *` read and write to a
login whose `write` list names `*`, as root's does, so root reads history whole
and may act on all of it whatever else is installed, LuCI or not (ADR-007); a
login that reads `*` but writes less reads the configs the other groups declare.

An act is offered, and staged, only on what the session sees whole: Undo and
Stage again when every change of the entry is in a readable config, Return when
every entry it reverses is, Return to the starting point when every config it
writes is. Otherwise the drawer says part of it is in settings the account may
not read, and a posted act is refused with the same words. An act the session
sees whole is refused before anything is staged when rpcd will not let the
session write one of its configs (`uci <config> write`); rpcd checks each write
again as it is staged.

On System → Events, an applied or rolled-back event follows its entry, naming
only the pages of the rows the session reads; one whose entry history no longer
keeps shows only to a session that reads every config history names. The other
events follow the read their fact comes from: the internet going down and coming
back `ubus network.interface dump`, the router starting `ubus system info`; the
history's own storage events need none.

### Where it lives, and for how long

Entries are a `changes` table in ADR-019's `history.db`, in
`verso.history.directory` (`/data/verso` on a Mono router, which an A/B flash
keeps). ADR-019's handling of a directory that cannot be written covers it:
entries wait in memory and are written once writing works again.

The last **1000 entries** are kept, whatever their age; `verso.history.changes`
sets another count, for a router whose storage holds more. A config that last
changed a year ago keeps its record: config history is read when something
breaks, which can be long after the change. Beside them the table keeps the
**starting point**, the committed contents of every declared config before
the oldest kept entry. When one more entry than the count is recorded, the
oldest folds into the starting point: what it did is applied to it, unless it
was rolled back, and it leaves history. On a fresh history, the starting point
is the config as Verso first reads it.

An entry is a few kilobytes; a full history of 1000 stays within a few
megabytes.
System → Maintenance's "History is kept in" row states the count beside the
file's size.

### Factory reset erases it by default

`firstboot` clears the overlay and leaves `/data`, so history outlives a reset.
The Factory reset form carries **Also erase the router's history**, checked by
default: `verso-rpcd` empties every place the history may be in (changes,
events, charts and per-device usage) before `firstboot` runs. Those are the
defaults, `/data/verso`, `/usr/share/verso/history`, `/data/nlbwmon` and
`/usr/share/nlbwmon/db`, which the shell and nlbwmon open again once
`firstboot` puts their configs back, whatever directory was chosen before; and
the chosen directories, the one set and the one the running shell was started
with (an applied change reaches the shell as a SIGHUP, not a restart). Each
is reached as the shell reaches it: made clean as Go's `filepath.Clean` makes
it, from `/` where it is relative (procd starts the shell there), then
followed through its links. A default that is a directory of its own, the
`verso` user's or root's in a directory only root may change, reached through
no link and no mount's root (by its device, or its mount, which tells a bind
mount too), holds the history alone and is emptied whole; a filesystem or a
bind mount inside it, told by its mount as the walk holds it, is left with all
it holds. Everywhere else the history
shares its directory with whatever its owner keeps there, so only its
writer's files go, each a file by its name, never following a link or going
into a directory, whoever owns the directory: the shell's `history.db`, the
files SQLite keeps beside it and the `stopped` mark, or nlbwmon's `<day>.db`
and `<day>.db.gz`, the names nlbwmon itself prunes, and either with `.broken`
after it, a day the usage watchdog moved aside (ADR-018). That holds for a
default of another account's, a stick mounted at a default, a default linked
to a stick by hand, `/var` linked to `/tmp`, `/data` or a stick's root chosen
(`block` mounts a stick it has no mount point for at `/mnt/<device>`), a
directory on a stick whose root any account may change (`ntfs3` shows a
volume's root as `0777`, and a FAT stick shared over Samba is mounted
`umask=000`), and a directory of another account's the shell writes in all
the same (a FAT or NTFS stick mounted with `quiet` or `noacsrules` lets the
shell's `chmod` succeed without effect). The option is the `verso` user's to
set, so naming a directory never has root remove more there than those
files. A path the shell's open fails on, a name absent or too long, a file
where a directory must be or too many links, holds nothing.

Root empties the directory it reached and no other: it holds each one open,
never through a link at its last name, and goes on only if it is the
directory reached, so a directory replaced since, through a link or in a
directory others may change, is left as it is and its place answered as a
failure; one gone from its place since leaves nothing there to empty. Each
directory inside a default emptied whole is held open the same way as it is
emptied, one at a time, so a tree of any depth is emptied; the walk climbs
back through `..` only into the directory it came down from. A chosen directory inside a default emptied whole is gone with it;
one a link there leads to is emptied where it leads.

The helper empties the history before it answers, reaching every place
before emptying any, so a place it cannot reach or empty is answered as a
failure naming the place and why, shown as the notification on Maintenance,
and the router is not reset. Once the answer is out it stops the shell and
nlbwmon, which write as they stop, and waits until their processes are gone,
killing any still there after ten seconds: procd signals and returns, and
what is written after the second emptying would outlive it. It then empties
again every place it can, logging any it cannot, and runs `firstboot`. A
`firstboot` that returns has not restarted the router: the helper logs what
came of it and starts again those of the shell and nlbwmon that ran before,
or whose processes procd could not list.

A router handed to someone else keeps no history in those places. A history
left in a directory no longer chosen or running (changing the option moves
nothing, ADR-019) is outside them and the operator's to remove. So is
nlbwmon's usage in a `database_directory` the owner set themselves, which the
package's usage-setup keeps (ADR-018): the reset empties nlbwmon's defaults
only, and leaves that directory as it is. Unchecked, the reset is recorded as
an entry made outside Verso on the next start, and Return to a point before
it is offered like any other.

## Consequences

- The describe hook gains a subject per sentence, and the plugin manifest a
  list of secret options; `docs/plugins.md` states both.
- `verso-rpcd` gains a long-running inotify watch on `/etc/config` and writes
  the spool, so it reads every plugin manifest for its secrets and keeps the
  fingerprint key. The helper's zero-session grant does not grow: the spool is
  a file, not a call.
- The shell reads the configs an apply carries before it applies them, and
  keeps the last reading of every declared config.
- An anonymous section is named by its place in its config (`cfg0792bd`), so
  one added ahead of it, outside Verso, reads as sections removed and made.
- A history that holds config also holds addresses, hostnames and PPPoE user
  names, and copies of configs only root may read: the directory is `0700`,
  unless a mount or `/data` itself, and `history.db` with its `-wal` and
  `-shm` files `0600`, the `verso` user's alone whatever the umask, made so
  each time the shell opens the file (ADR-019).
- The words of an entry stay in the language of the session that applied it.
- Entries recorded while history cannot be written are lost at a reboot, as
  ADR-019's events are.
