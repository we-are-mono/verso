# Design

## Context

See proposal.md for why. The current state this design starts from:

- **The helper.** `verso-rpcd` gates every verb by asking rpcd's `session.access` for
  `{ubus, verso, <method>}` with the caller's sid (`main.rs` `dispatch`). It runs up to
  16 requests at a time, one thread each, and already runs child processes with a
  timeout and capped output (`main.rs` ~905). `grants.rs` re-derives a session's grants
  for any rpcd scope, not only `ubus` and `uci`.
- **The shell's broker.** `brokeredUbusRead` (`gateway.go` ~1221) maps twelve function
  names to backend methods and skips anything else. `runPluginCommands`
  (`plugin_commands.go`) is a closed switch. `fileConfig` hard-codes which uci config
  owns each file path.
- **What plugins read.** Plugins receive brokered reads in the `X-Verso-Ubus` header,
  keyed by function name.
- **Which plugins live where.** firewall, interfaces and system are built in this tree
  and packaged inside `verso`. acme, vpn, ddns, dnsdhcp, wireless and qos are their own
  repositories and packages.
- **File families.** `config_files.rs` holds three families (dnsmasq, fw4, openvpn) as
  a Rust enum, with per-family places, limits, words, check, reload and the OpenVPN
  fold. The staging journal is in `/tmp/run/verso-config-files` and is keyed by path.
- **What a POST may do.** A POST may carry one `commands` entry, run at once. It is
  refused beside a uci commit unless it is the file stage (`gateway.go:543-569`).
  `docs/plugins.md` instead documents an `apply` array that runs after the UCI apply,
  which the code never did.

## Goals / Non-Goals

**Goals:**
- One generic, root-authored path from an operator's session to a plugin's root code,
  gated by rpcd and checked at the helper.
- First-party plugins hold all their own root code. Core holds the rule for running it,
  never the code.
- Each piece testable without root and without a router, through seams (ADR-003).

**Non-Goals:**
- Long-running plugin jobs (start, read by cursor, stop). No current command needs
  them. When one does, it reuses the Diagnostics job pattern.
- Plugin state files written by root halves as a contract. A root half may keep files
  under `/var/run` for its own use (acme and vpn do), but plugins reach that state only
  through a read.
- Core events beyond `verso-certificate` (the events theme).

## Decisions

### 1. The registry is a file per plugin, read on every call

`/usr/share/verso/plugins/<id>/commands.json`, beside the manifest:

```json
{
  "schema_version": 1,
  "commands": [
    { "name": "state", "kind": "read", "version": 1,
      "exec": "/usr/bin/verso-plugin-ddns", "argv": ["root"] },
    { "name": "update", "kind": "act", "timeout": 30,
      "exec": "/usr/bin/verso-plugin-ddns", "argv": ["root"],
      "args": { "section": { "kind": "uci-name" } } },
    { "name": "fold", "kind": "transform",
      "exec": "/usr/bin/verso-plugin-vpn", "argv": ["root"] }
  ],
  "families": [ … decision 9 … ],
  "certificates": { "state": ["certificate"] }
}
```

The helper reads and checks the file each time a command is asked for. The file is a
few hundred bytes and the call already starts a process, so a cache would buy nothing.
It would also have to notice an `apk del`, so reading every time is what makes a
removal take effect at once.

`transform` commands are for the helper's own root-to-root use (decision 9). They are
never reachable through the session verb.

*Alternative: a directory of one file per command.* Rejected: a plugin's commands are
one unit, installed and removed together, and one file is one ownership check.

### 2. One verb, gated by a plugin scope

`pluginCommand {plugin, name, args}`. For this verb, `dispatch` does not check
`{ubus, verso, pluginCommand}`. It checks `{verso-plugin, <plugin>, <name>}` through
the same `ubus::access`. rpcd scopes are free-form, and `grants.rs` already copies any
scope, so a plugin installed during a session is granted without a new sign-in
(ADR-007 §8).

A plugin's rpcd ACL puts reads under `read` and acts under `write`, so a read-only
login gets reads and no acts. The zero session matches nothing in
`zero_session_grants`, so the registry is unreachable unattended.

*Alternative: rpcd's own exec plugins (`/usr/libexec/rpcd/*`).* These are OpenWrt's
native "root executable behind an rpcd ACL", which LuCI uses. rpcd discovers them when
it starts, so an install needs an rpcd reload. That reload keeps sessions (ADR-007), so
it is a small cost and not a reason to reject them. They are rejected because:
- rpcd checks argument types only (string, int, bool), so every root half would
  validate its own input, and one mistake in any plugin, a third party's included,
  gives root;
- rpcd runs any executable in that directory with no ownership check;
- its timeout is one global setting, not a per-command bound;
- the shell, a non-root ubus client, would need a ubusd ACL file per plugin object.

The registry keeps one tested validator, one ownership check and one bounded runner
for all plugins.

*Alternative: a long-lived root daemon per plugin.* Rejected for memory (2 GB unit,
base ~450 MB) and because it multiplies root processes.

### 3. The root half is the plugin's own binary

`verso-plugin-<id> root` reads one JSON request on stdin and dispatches by command
name. The plugin's hotplug and init scripts call named subcommands directly (for
example `verso-plugin-acme root deliver renewed`). The SDK provides
`verso_plugin_sdk::root::run(handlers)`, which:
- refuses unless the effective uid is 0;
- parses the request;
- maps `Ok(value)` to exit 0, `Refusal(text)` to exit 3 with `{"refusal": …}`, and any
  other error to exit 1.

*Alternative: a second, root-only binary per plugin.* Rejected for flash: a static
musl Rust binary is about 1 MB (`verso-rpcd` on aarch64 is 1.04 MB), and the SDK and
serde would be duplicated in every package. Privilege comes from who starts the
process, not from the file. The page server runs as `verso-plugin-<id>` and never
takes the `root` path. The binary is root-owned and not writable by the plugin's
account.

### 4. Reads are commands; the result key is `<plugin>.<name>`

The shell's broker keeps its core functions (`dhcpLeases`, `networkState`,
`wirelessState`, `accessCredentials`, `configFiles`). For each manifest `acl.read`
entry with scope `verso-plugin`, it calls `pluginCommand` and puts
`{"version": <declared>, "result": …}` in the same `X-Verso-Ubus` map under
`<plugin>.<name>`. A plugin reading vpn's state reads `vpn.state`, wherever it is
installed from.

The SDK's accessor takes the key and the versions the caller knows, and returns
nothing (logging once) for any other version. A read's version rises only on an
incompatible change: a removed or retyped field. Added fields keep it. This matters
most for cross-plugin reads, because the producer and the reader ship from different
repositories and upgrade apart. A reader that meets an unknown version draws without
it rather than misreading it.

*Alternative: no versions, readers tolerate whatever arrives.* Rejected: a renamed
field reads as a missing one, and the page quietly shows wrong state (a tunnel listed
as down) instead of no state. Reads for one render run in parallel (today's loop
is sequential), so a page with three reads waits for the slowest, not their sum.

*Cost:* one process start per read per page render. Brokered reads run per page and
per entity panel, never per stream tick, so this is bounded by operator clicks. Task
3.4 measures it on the DK.

### 5. Argument kinds are a closed, shared set

The kinds are `uci-name`, `hostname`, `ipv4`, `ipv6`, `ip`, `port`,
`int{min,max}`, `enum[…]`, `bool` and `text{max ≤ 65536}`. Text holds no NUL and must
be valid UTF-8. Every declared argument is required unless marked `optional`, and
unknown ones are refused.

The helper validates in Rust. `testdata/argument-kinds.json` in the SDK lists accepted
and refused samples per kind. Both the helper's and the SDK's tests read it, and the
Go `internal/datatype` tests read it for the kinds the shell also checks, so the three
cannot drift. The shell does not re-validate acts: the helper is the boundary. The
shell checks only that the plugin declared the act.

### 6. Running a command: one bounded runner behind a seam

A `Runner` trait (real: `std::process::Command`; tests: a fake recording argv, stdin
and env) runs:
- `exec` with `argv` and `env_clear()` plus `PATH=/usr/sbin:/usr/bin:/sbin:/bin`;
- in `current_dir("/")` and its own process group;
- with the request as stdin;
- capturing stdout up to 1 MiB + 1 and stderr up to 64 KiB, logged only;
- killing the group at the timeout (default 10 s, ceiling 120 s, the existing
  `CHILD_TIMEOUT`).

It generalizes the existing child runner rather than adding a second one.

### 7. Ownership is checked on the path, not trusted from the package

An `Owner` seam (real: `symlink_metadata`; tests: a table) walks the registry file and
the executable and every ancestor to `/`. Each must be uid 0, have no group or other
write bit, and not be a link; the executable must be a regular file. This is the same
rule ubusd applies to its ACL files. `config_files.rs`'s `check_path` folds into it.

### 8. Certificates leave root as facts, through a shell transform

acme's root half returns its certificate as PEM (it never reads the key). The
registry's `certificates` map names that field, and the shell replaces it with
`certificateFacts`, the same function the HTTPS certificate card uses. This
generalizes `readACMEState` without naming acme.

*Alternative: the SDK parses X.509 in Rust.* Rejected: two implementations of the
same facts would drift (day rounding, fingerprint format, self-signed detection), and
every plugin would carry an X.509 crate.

### 9. File families come from the registries

`families` entries carry:
- `name`, `config`, `files` and `dir`;
- `mode` (`0644` or `0600`);
- `words` (`too_large`, `invalid`, `over_limit`), translated through the owning
  plugin's catalogue;
- `check` and `reload` as fixed argv, never templated;
- an optional `transform` naming one of the plugin's `transform` commands.

The helper builds its family table from all registries for each operation, under the
existing journal lock. The bounds in the spec (under `/etc`, the protected list, no
shared name or place) are checked when the table is built. A bad family is dropped and
logged, and the others stand.

The transform protocol has two ops:
- `list`, given `{path, content, pending}` per file, answers `{content}` or
  `{withheld: reason}` per file;
- `stage`, given `{path, edited, previous}`, answers `{content}` or a refusal.

dnsdhcp's `stage` runs `dnsmasq --test` on the one file, which the core
`validate_dnsmasq` does today. vpn's ops are `openvpn::fold`, `unfold` and the
profile-path withholding, moved as they are.

The shell learns each family's owning config from `configFiles`, which gains a
`families` list (`name`, `config`, `plugin`). `fileConfig` disappears. A plugin's file
read is the core `configFiles` read, narrowed to the families of configs the plugin
reads. The per-family verbs and the extra flags they carried move to the plugins'
reads: `fw4` installed goes to firewall's `state`, dnsmasq's features to dnsdhcp's
`dns-state`.

*Alternative: keep the three families in core.* Rejected: OpenVPN's fold (most of
`openvpn.rs`) is plugin code by any measure, and leaving it would keep core naming
vpn.

### 10. `hand_over` becomes an announcement

After `setWebCertificate` or `makeWebCertificate` writes the pair, the helper runs
`hotplug-call verso-certificate` with `ACTION=installed` and waits for it, so acme's
section is off before the call answers. acme ships
`/etc/hotplug.d/verso-certificate/50-acme`, which runs
`verso-plugin-acme root hand-over`. That is the existing `hand_over` logic: `uci set
acme.verso.enabled=0` and a commit through root's own delta directory.

The other hotplug and init lines change their target only:
- `/etc/hotplug.d/acme/50-verso` → `verso-plugin-acme root deliver "$ACTION"`;
- the init `catch_up` → `verso-plugin-acme root deliver sync`.

Delivery still installs through `/usr/bin/verso certificate install`.

### 11. The `apply` drift: the docs follow the code

Acts run at once on their own POST. Running them after the UCI apply would tie a
"Sync with this computer" press to an unrelated apply, and the code has always refused
an act beside a commit. ADR-006 and `docs/plugins.md` drop the `apply` array and
describe `commands`:
- one act per POST, run before the answer;
- never beside staged changes;
- the file stage as the one command that joins the stage.

A test pins the refusal of an act beside a commit.

### 12. Names of the moved commands

| Plugin | Read | Act | Transform | Family |
|---|---|---|---|---|
| vpn | `state` | — | `files` | `openvpn` |
| ddns | `state` | `update` | — | — |
| dnsdhcp | `dhcp-state`, `dns-state` | — | `files` | `dnsmasq` |
| firewall | `state` (counters, `fw4`) | — | — | `fw4` |
| acme | `state` | — | — | — |

interfaces' ACL holds `verso-plugin` grants on `vpn state` and `dnsdhcp dhcp-state`.
Family names stay as today, so a journal written before the upgrade still resolves.

## Risks / Trade-offs

- **[apk may not support a versioned conflict on OpenWrt's apk-tools]** → Task 1.1
  checks `!verso-plugin-vpn<V` on the DK before anything else is built. apk's
  dependency syntax has both `!` conflicts and version constraints; whether OpenWrt's
  apk-tools honours them combined is what the task finds out. A no stops
  the change at task 1.1 and goes to the owner: the spec requirement "Core and plugins
  built before the registry are not mixed" would need rewording, so it is not guessed
  at here.
- **[A process start per read costs router CPU]** → Reads run in parallel and only per
  render. Task 3.4 measures VPN and Interfaces renders on the DK. If a render's added
  time is visible, the remedy is a per-render memo shared by all panels, not a cache
  across renders.
- **[A registry gives the browser a path to root executables]** → The package was
  already root. What the registry adds is bounded by the grant, the schema, the
  ownership walk, the clean environment and the timeout. ADR-027 records this, and
  ADR-007 §5 is amended to say it.
- **[A third-party family could claim another plugin's directory]** → Both families
  are refused and logged (spec). The operator sees the file editor go empty, not a
  file taken over.
- **[A transform that fails hides a family's files]** → They are listed by path alone,
  never unfolded. Failing closed is the point of the transform.
- **[The hand-over hotplug fails]** → The install still succeeds, and acme's section
  stays on, which the next catch-up would undo. acme's script logs the failure. The
  Install and Make drawers' notice already told the operator. The spec scenario
  "Turned off before the next catch-up" is the acceptance test on the DK.

## Migration Plan

1. Ship one release train: `verso` R (registry, families from registries, verbs
   removed, firewall/interfaces/system in-tree) and acme, vpn, ddns and dnsdhcp, each
   depending on `verso>=R`.
2. Runtime paths stay put (`/var/run/verso-acme`, `/var/run/verso-openvpn`, the
   journal, `/etc/verso`'s version key). An upgrade needs no migration step, and a
   connected tunnel keeps its record.
3. vpn's package takes `/etc/hotplug.d/openvpn/50-verso` with `replaces:verso`.
   `verso` drops its `replaces:verso-plugin-vpn`.
4. Rollback: downgrade `verso` and the four plugins together to the previous train.
   Nothing on disk changed shape, so the older helper reads the same records and
   journal.

