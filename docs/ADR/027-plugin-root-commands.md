# ADR-027 — Plugin root commands: a plugin carries its own root work

- **Status:** Accepted
- **Date:** 2026-10-18
- **Deciders:** tomaz@zaman.io
- **Relates to:**
  - ADR-007 §5: the helper gains one generic path, bound to a root-authored registry.
  - ADR-006: the plugin contract gains `commands.json`, registered reads and acts.
  - ADR-014 §3: the registry is unreachable with the zero session.
  - ADR-017 §6: the trusted certificate's root work ships with its plugin.
  - ADR-026: slots placed plugins on the screen; this places their root work.
  - ADR-021: a root half's commits are recorded as the router's acts, with their cause.

## Context

A plugin runs as its own account and never holds root (ADR-007). Root work a page
needed was therefore written into the privileged helper, `verso-rpcd`, one verb per
need, and the shell brokered each verb by name:

- OpenVPN's tunnel state, profile folding and the hotplug recorder that feeds them;
- DDNS's state and its update;
- dnsmasq's state, features and option-file check;
- the firewall's counters and rule files;
- the trusted certificate's delivery, state, catch-up and the hand-install that turns
  acme off.

Core carried all of it on a router without those plugins. A plugin could not fix its
own root half without a core release, and a plugin core did not know could have no
root half at all.

## Decision

1. **A plugin registers its root commands in one file.** The package installs
   `/usr/share/verso/plugins/<id>/commands.json`, beside its manifest. Each command
   has a name, a kind (`read`, `act` or `transform`), an absolute executable with fixed
   arguments, an optional timeout, and, for an act, a schema of its arguments. The
   helper reads the file each time a command is asked for, so an install or a removal
   takes effect at once; the file is small, and the call starts a process anyway.

2. **One verb, gated by a plugin scope.** `pluginCommand {plugin, name, args}` is
   granted not as `ubus verso pluginCommand` but per command, as
   `verso-plugin <plugin> <name>`: reads under the grant's `read`, acts under its
   `write`. rpcd scopes are free-form, and the grant refresh already copies any scope
   (ADR-007 §8), so a plugin installed during a session is granted without a new
   sign-in. The zero session holds no such grant (ADR-014 §3). A `transform` is the
   helper's own and is never reachable through the verb.

3. **The root half is the plugin's own binary.** `verso-plugin-<id> root` reads one
   JSON request on stdin; the SDK's dispatcher refuses unless the effective uid is
   root. The plugin's hotplug and init scripts start the same binary for unattended
   work, under subcommands of the plugin's own, as root already. Privilege comes from
   who starts the process, not from the file: a second, root-only binary per plugin
   would cost about a megabyte of flash each and duplicate the SDK.

4. **Reads are commands, versioned.** The shell runs each read a plugin declares in
   `acl.read` when the page renders, in parallel, and hands it on as
   `{"version", "result"}` under `<plugin>.<name>`. A read's version rises only on an
   incompatible change. A plugin uses a result only at a version it knows, so a
   producer and a reader shipped from different repositories upgrade apart without a
   page misreading state. A plugin may hold and declare another plugin's read; while
   that plugin is absent, the read gives no entry.

5. **Arguments have a closed set of kinds.** `uci-name`, `hostname`, `ipv4`, `ipv6`,
   `ip`, `port`, bounded `int`, `enum`, `bool` and bounded `text`, every value text as a
   form posts it. The helper checks every argument before anything runs and refuses an
   unknown or missing one. The SDK, the helper and the shell's datatypes are held to
   one file of samples, so the three checks answer alike. The shell checks only that a
   plugin declared the act; the helper is the boundary.

6. **A command runs bounded, without a shell.** The helper starts the executable
   directly, with `PATH` alone in its environment, in `/`, its request on stdin, in a
   process group of its own. It stops the group at the command's timeout (10 s unless
   declared, never more than 120 s) and refuses output over 1 MiB or output that is
   not one JSON object. Exit 0 is a result; exit 3 with `{"refusal"}` is a refusal the
   operator reads, translated by the plugin's catalogue; any other end is a failure,
   logged and never shown.

7. **Ownership is checked on the path.** The registry file, each executable and every
   directory above them must be owned by root, writable by root alone, and not links,
   and the executable must be a regular file: ubusd's rule for its ACL files. A group
   write counts as root's alone when the group is root's own. A registry that fails
   any check, or does not parse, refuses that plugin's commands and no other's.

8. **Certificates leave root as facts.** A read may name fields of its result that
   hold a PEM certificate; the shell turns each into the facts the HTTPS certificate
   card shows, with the function that card uses, so no PEM reaches a plugin and the
   facts have one implementation.

9. **A root half's commit says what caused it.** Before a root half commits a config
   (`uci commit`), it marks it with the SDK's `root::own_write(plugin, config, cause)`.
   The mark is a file in `/tmp/run/verso-own`, a directory root alone may write, and
   names the plugin and one English sentence, a key of the plugin's catalog. Config
   history records the commit as the router's own act with that cause (ADR-021). A
   root half's mark never names an operator: only the helper's commits made for an
   operator's act do.

Core keeps the root work the shell itself renders (the firewall's status and log, DHCP
leases, network state) and what guards access to the router (credentials, SSH keys,
the web certificate, the clock, the password), with packages, firmware, backups,
diagnostics, usage and hardware.

## Consequences

### Positive
- A router carries a plugin's root code only while the plugin is installed, and
  `apk del` removes it with the registry and the grants.
- A plugin fixes or extends its root half in its own release.
- One validator, one ownership walk and one bounded runner serve every plugin, so a
  plugin author's mistake in input checking cannot reach root.

### Costs / negatives (honest)
- A process starts for each read on each render. Reads run in parallel and only per
  render, never per stream tick.
- The browser gains a session-gated path to root executables. The package installing
  them is root already; the grant, the schema, the ownership walk, the clean
  environment and the timeout bound what the path adds.
- Core and the plugins whose verbs left it ship together. `verso` conflicts with their
  versions from before the registry, and apk moves them in one transaction or refuses
  both.

## Alternatives considered

- **rpcd's own exec plugins (`/usr/libexec/rpcd/*`)**, OpenWrt's native root
  executable behind an rpcd ACL, which LuCI uses. rpcd discovers them when it starts,
  and the reload an install needs keeps sessions, so that is no obstacle. Rejected
  because rpcd checks argument types only, so each plugin's root half would validate
  its own input and one mistake in any of them gives root; it runs any executable in
  its directory with no ownership check; its timeout is one global setting; and the
  shell, a non-root ubus client, would need a ubusd ACL file per plugin object.
- **A long-lived root daemon per plugin.** Rejected for memory on a 2 GB unit and for
  multiplying root processes.
- **Plugin state files as the contract**, written by root halves and handed out by one
  read verb. Rejected because most state is worked out from live system state when it
  is asked for; a file would be stale. A root half may keep files for itself, as
  acme's records and OpenVPN's hotplug records are.
- **Readers tolerate whatever a read returns, unversioned.** Rejected: a renamed field
  reads as a missing one, and a page shows wrong state instead of none.
