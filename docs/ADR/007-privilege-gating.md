# ADR-007 — Privilege gating: act through rpcd's ACLs with the session, not as ambient root

- **Status:** Accepted
- **Date:** 2026-08-23
- **Deciders:** tomaz@zaman.io
- **Relates to:** ADR-001 (single static binary; supersedes its "runs as root,
  acts freely" spike posture for the privileged surface), ADR-003 (the `Backend`
  seam, where this swap lands), ADR-006 (plugin contract; adds manifest ACL
  declarations and a read-snapshot channel), ADR-009 (shell-owned platform
  administration and recovery), ADR-011 (package and service management). Closes
  the privilege-gating gap
  from the prototype security review: the session is authenticated but never used
  to authorize anything.

## Context

Verso authenticates via rpcd's `session.login` and holds the returned session id
(`sid`) server-side, then **never uses it again**: it reads live state over the
native ubus socket and writes uci directly via go-uci — all as **root**, with no
authorization check. So any account rpcd accepts, *including a deliberately
read-only operator*, obtains full root through Verso. That gap is what this ADR
closes.

Two distinct ACL systems exist in OpenWrt, and the distinction is load-bearing:

- **ubusd's socket ACL is uid-based and exempts root.** ubusd stamps each
  client's uid from the kernel (`SO_PEERCRED`) and checks calls against
  `/usr/share/acl.d/*`, but returns "allow" immediately for uid 0
  (`sources/ubus/ubusd_acl.c:108`). Verso-as-root therefore bypasses it — by
  ubusd's design, not by accident. This is **not** the ACL that governs LuCI.
- **rpcd's RBAC is sid-based**, defined in `/usr/share/rpcd/acl.d/*.json` (named
  groups → `read`/`write` → scope → object → functions). This is the model that
  actually gates privileged operations.

The enforcement point for rpcd's ACLs is a **userspace bridge, not ubusd**.
LuCI's `admin/ubus` handler probes `session.access{sid, scope, object, function}`
*before* forwarding a call, then talks to ubus as root
(`sources/luci/modules/luci-base/ucode/controller/admin/index.uc:34-41,62-71`);
uci writes go through rpcd's `uci` object carrying the sid
(`.../admin/uci.uc:76-85`). Verso replaced that bridge with its native backend
and never rebuilt the check. **The gap is a missing enforcement point, not a
broken socket.**

## Decision

1. **Verso acts as a credential-presenting client, not ambient root.** Privileged
   and ACL-gated operations — config writes, and any ubus call a restricted
   operator might not be permitted — are performed by calling rpcd's ACL-gated
   ubus objects (`uci`, `rc`, and the sid-gated read objects) with the session's
   `sid` in the payload. rpcd
   is the single component that both **authorizes and executes**; Verso never
   decides its own privilege.

2. **The `Backend` seam absorbs the swap (ADR-003).** The interface stays; its
   implementation moves from go-uci-direct + no-argument native invoke to
   rpcd-object calls (`uci.get/set/commit/apply`, `rc.init`, …) carrying the sid. The
   sid is plumbed from the session into backend calls (via context or an explicit
   parameter). Tests, which use fakes, stay valid — the seam pays off again.

3. **The endgame is de-privileging the shell.** Acting through rpcd is the
   on-ramp; the target is a Verso process that *cannot* bypass rpcd (dropped
   capabilities, no ambient write path), so a compromised or buggy shell is
   bounded by the operator's ACL rather than by root. This supersedes ADR-001's
   spike posture for the privileged surface.

4. **Plugins present credentials too, for reads and writes (extends ADR-006).** A
   plugin holds no session and touches config in neither direction directly. It
   declares the ACL scopes it needs in its `manifest.json` (the Verso analog of
   LuCI's per-plugin `acl.d`) — `acl.read` for what it renders, `acl.write`
   for the configs it changes. The shell, which holds the sid, brokers **both**
   directions through rpcd on the plugin's behalf: it **reads** each declared
   config with the sid and hands the plugin a snapshot to render from, and it
   **writes** the plugin's declarative `commit` intent — refusing anything outside
   the plugin's declared, session-authorized scopes. A read scope also reaches past
   config: a `ubus` scope on object `verso` names one of the helper's read functions
   (`wirelessState`), and a `verso-plugin` scope names a read a plugin's registry
   declares (the firewall's nftables counters, ADR-027), so a page can show live
   kernel state that neither the plugin nor the de-privileged shell may read for
   itself. Plugins
   gain a *declared* privilege surface instead of ambient root, and never read
   `/etc/config` or hold a write path.

5. **A first-party persistent helper performs privileged root actions that are not uci
   config.** The operations the shell owns but must not perform itself include
   setting the root password, setting the kernel clock, apk package operations (index refresh,
   search, list-installed, install, remove), privileged reads such as listing fw4's
   nftables ruleset for its per-rule counters, and network diagnostics (ping,
   traceroute, a DNS lookup) — fixed programs whose arguments the helper builds from a
   validated tool, target, device and IP family, each run a job only the session that
   started it can read back by cursor or stop. procd service lifecycle is *not* one of them —
   it rides rpcd's native `rc` object, sid-gated like `uci` (the `rc` grant in the shell's
   `acl.d`). Rather than depend
   on `rpcd-mod-luci` (a LuCI component this shell
   exists to replace), Verso ships `verso-rpcd`: a small, single-responsibility
   Rust daemon kept **separate from the shell** so the root-run process carries
   minimal code. It stays resident instead of making rpcd spawn a fresh helper
   for every browser call, and accepts requests only on a `root:verso` Unix
   socket. The helper **self-gates** every request through native ubus
   `session.access` for `{ubus, verso, <method>}` and refuses, fail-closed, a
   missing or invalid sid. The shell calls it through the `Backend` seam carrying
   the operator's sid, exactly as it calls `uci`. Extending it is adding a method
   plus one rpcd-ACL grant; gating and protocol are shared. Each method accepts only
   the structured arguments required for that operation and invokes the corresponding
   native OpenWrt mechanism; the protocol exposes no command string. Its one general
   path is bound to a registry root wrote: `pluginCommand` runs a command a plugin
   package registered in `/usr/share/verso/plugins/<id>/commands.json`, only under the
   session's grant `verso-plugin <id> <command>`, only from root-owned files, with
   arguments checked against the registered kinds, no shell, a clean environment and a
   timeout (ADR-027). So the helper holds the root work core owns, and a plugin's root
   work ships with the plugin. Firmware validation, target compatibility, and destructive
   operation checks are enforced again at this privileged boundary rather than relying
   only on browser or shell validation.

   Explicit confirmation belongs to the shell interaction; the sid check at the helper is
   mandatory regardless of any UI confirmation.

6. **Enforcement is unified at rpcd.** The privilege model lives behind the `Backend`
   seam and the manifest: the shell holds no ambient root, and every privileged action —
   uci writes, service control through the `rc` object, and the helper's own methods — is
   authorized by rpcd against the operator's sid. A Verso compromise is bounded by that
   operator's ACLs.

7. **The operator's rpcd session is kept alive for as long as the Verso session
   lives, and torn down with it.** The `sid` from `session.login` is rpcd's own
   session, on rpcd's inactivity clock (`RPC_DEFAULT_SESSION_TIMEOUT`, 300 s) — far
   shorter than a Verso session (30 min idle, 12 h absolute). Verso therefore treats
   the `sid` as a resource its session owns: a background renewer touches each live
   session's `sid` (`session.access`, which rpcd renews the session on) well inside
   the 300 s window, and a session that ends — by idle expiry, absolute cap,
   logout, a revoke on Access, a password change, or signing in again — has its
   `sid` destroyed. A password change ends every other session of the account
   whose password it replaced, keeping only the session that made the change, so
   no browser signed in with the old password outlives it; another account's
   sessions stand, an rpcd login that signs in with a system account's password
   (`$p$root`) among them. Signing in again from a browser that presents a live
   session ends that session as the new one starts. Nothing else keeps the `sid`
   warm: an idle operator's session renews on the renewer's clock, not on request
   traffic, so the `sid` is ready the instant they act again and gone once their
   session is. Verso never re-mints a `sid` on its own — it holds no credential to
   do so (it keeps the `sid`, never the password) — so if rpcd reports a live
   session's `sid` unknown (rpcd's own restart, say), Verso ends that Verso session
   and the operator signs in again, rather than presenting a half-working shell
   whose every gated read fails.

8. **A plugin installed while the operator is signed in is granted to their session as
   signing in would grant it.** rpcd works out a session's grants once, at
   `session.login`, from the `acl.d` files on disk then; a plugin package installed
   afterwards ships a group the session never received, and its pages read nothing
   until the operator signs in again. Whenever the shell rescans the plugins (an
   install or remove on Packages, or the SIGHUP a plugin package's scripts send), it
   asks the helper's `refreshGrants` for each live session, with that session's own
   sid. The helper re-derives the session's grants exactly as rpcd's login does —
   the first `config login` in `/etc/config/rpcd` for the session's username; for
   every group in every `/usr/share/rpcd/acl.d/*.json`, its `read` and `write` parts
   that the login's `read`/`write` lists allow (`!` patterns first, then the others,
   matched with the C library's `fnmatch` as rpcd does, `write` implying `read`);
   each table entry one `(scope, object, function)`, each list entry one
   `(scope, object, permission)`, plus the group in `access-group` — and adds them
   with `session.grant`. It only ever grants what signing in now would, never
   revokes, and touches only the sessions the shell holds. rpcd still evaluates every
   call; this replicates only how a login fills a session, not the evaluation
   Alternative C rejects.

9. **What the shell reads with its own privilege reaches a session only under the
   grant that guards the same fact through rpcd.** Some readings never pass rpcd: a
   file or kernel table the shell opens itself, a cache of a helper's answer, the
   shell's own stores. For those the shell asks rpcd `session.access` with the
   reading session's sid, once per grant for each page it renders and again each time
   a stream reads afresh, and shows the reading only on
   a yes; a denial and an unreachable rpcd both withhold it. The grant is the one an
   rpcd read of the same fact needs, never one the shell invents:

   | Reading | Grant |
   |---|---|
   | dnsmasq's lease file and the kernel neighbour table (the Devices roster, a device's panel, the brokered `dhcpLeases` read, the Overview's count of online devices) | `ubus dhcp ipv6leases`, the DHCPv6 lease read connected devices are declared under |
   | The held per-device usage days (the roster's month, a device's periods, the usage stream) | `ubus verso usageDays` |
   | The held per-device rates and the minute of them a device's graph opens on | `ubus verso usageLive` |
   | How the last alert went (ADR-020), whose words could name the address, and a test alert | `uci verso read`; a test also `uci verso write` |
   | Config history (ADR-021): each entry's changes, the starting point, the current readings | `uci <config> read` for each config; an act on it also `uci <config> write` |
   | System → Events: the internet going down and coming back; the router starting; an apply | `ubus network.interface dump`; `ubus system info`; its config history entry's grants |
   | The Overview's past Internet graph: the interface's traffic, the gateway's loss and latency, and its outage and boot marks | `ubus network.interface dump`; its marks as on System → Events |
   | The held temperature history (System → Hardware's graph) | `ubus system info`, the grant the router's own state is shown under |
   | Access's signed-in sessions of another rpcd account, and ending one | `ubus session list`; `ubus session destroy` — rpcd's own session object's grants, which no page's group carries and `verso-admin` does. A session's own account's sessions need none |

   A session refused the grant sees the reading's absence said plainly (an empty
   roster that says the account may not see devices), and a plugin receives no entry
   for the brokered read, as for any read rpcd refuses. A reading whose grant rpcd
   does not answer is withheld the same way, and the page says the permissions
   could not be checked rather than that the account may not see it. A page asks
   each grant once however many of its parts need it: a device's panel asks the
   lease grant once for its facts and every contributing plugin's lease read.

## Consequences

### Positive
- Authorization and enforcement are **unified at rpcd**, the platform's audited
  authority — no separate check for Verso's code to get wrong or forget on a new
  path.
- Verso **inherits OpenWrt's ACL model as it evolves** (new scopes, objects,
  grants) for free; there is no hand-maintained operation→ACL mapping to keep in
  sync.
- The **trust boundary moves out of Verso's code**. Combined with de-privileging,
  a Verso RCE cannot exceed the operator's grants — genuine defense in depth.
- It is LuCI's decade-proven, OpenWrt-blessed model, and it **extends uniformly to
  plugins**.

### Costs / negatives (honest)
- go-uci is **dropped entirely** — from the shell's read/write/commit and from the
  plugins' reads (its last use). Every uci operation is re-expressed as an rpcd ubus
  call carrying the sid, and rpcd becomes a hard runtime dependency for reads and
  writes (it already is for login).
- A per-operation authorization **round-trip** to rpcd remains. The root helper
  itself is persistent, so this no longer includes an exec-plugin process spawn.
- The native-ubus investment (`internal/ubus`) is **not** wasted — it is exactly
  what carries the sid-bearing calls — but the go-uci write path built for the
  spike is superseded.
- **De-privileging the shell is real work** (which capabilities remain necessary,
  socket permissions for a non-root client) and is the harder half of the endgame.

### Neutral
- The `Backend` interface gains session-awareness; this is behind the seam and
  invisible to the widget renderer.
- Governs privilege only; the visual contract (ADR-005) is unchanged. The
  mechanical plugin contract (ADR-006) gains two things: the manifest
  `acl.read`/`acl.write` declarations, and two read channels the shell injects into
  the plugin request — the uci snapshot (`X-Verso-UCI`) and the brokered helper
  reads (`X-Verso-Ubus`).

## Alternatives considered

- **A — Verso as the enforcement point:** probe `session.access` itself, then act
  as root via go-uci. Rejected for the long term: it **splits authorization (rpcd
  decides) from enforcement (Verso acts)** — a confused-deputy shape whose safety
  rests on Verso's own discipline, and it needs a hand-maintained
  operation→`(scope,object,function)` mapping that drifts as OpenWrt's ACL model
  evolves. It is the correct low-cost **interim**, not the destination.
- **C — reimplement rpcd's ACL evaluation in Go** (read `acl.d`, resolve
  sid→groups, evaluate). Rejected: a permanent sync burden that re-derives trust
  Verso can simply ask rpcd for, and duplicates session→group resolution Verso
  does not own. Justified only if rpcd were absent — which it is not; rpcd is base
  OpenWrt and Verso already depends on it for login.
- **For root actions (Decision 5): depend on `rpcd-mod-luci`'s `setPassword`, or
  fold the action into the shell.** The former pulls a LuCI component into a shell
  built to replace it; the latter re-grants the shell an ambient write path (e.g.
  `CAP_DAC_OVERRIDE` to write `/etc/shadow`) that de-privileging removes. Also
  rejected: a *shared* `verso-plugin` uid instead of per-plugin — it fixes the
  shell/bus boundary but lets a compromised plugin `ptrace` a peer. The self-gated
  `verso-rpcd` helper plus per-plugin uids keep root actions in a minimal,
  session-authorized, mutually-isolated surface.
- **For the sid lifecycle (Decision 7): re-mint the sid lazily when a gated call
  is refused, or widen rpcd's session timeout to match Verso's.** Lazy re-mint is
  impossible by construction — Verso keeps only the sid, never the password (the
  point of holding it server-side), so it has no credential to log in again with.
  Widening rpcd's timeout only moves the mismatch: it leaves two independent clocks
  to keep in sync by hand, and silently regresses the moment either changes. The
  renewer ties the sid's life to the session's by construction, with no second
  number to maintain.

## Implementation notes

On OpenWrt the shell runs as a dedicated non-root user (`verso`) via procd's
`user`/`group`. It then holds no write access to `/etc/config` and does not get
ubusd's uid-0 ACL exemption, so every backend call is bounded by rpcd plus a ubusd
`acl.d` grant. Two constraints are load-bearing:

- **A non-root uid is required; dropping capabilities from root is not enough.**
  ubusd exempts uid 0 by uid, not by capability (`ubusd_acl.c`), so a
  capability-stripped root still bypasses ubusd entirely. Only a non-root uid is
  gated.
- **The shell has its own ubusd `acl.d` grant** (`/usr/share/acl.d/verso.json`)
  for the objects it brokers (`session`, `uci`, `system`, and related read
  objects), because ubusd
  ACL-checks non-root callers; rpcd still applies the per-operator sid gating on
  top. That file must be root-owned and not group/world-writable, or ubusd skips
  it (`ubusd_acl.c` `ubusd_acl_load`). The shell keeps only `CAP_NET_BIND_SERVICE`
  (to bind :80/:443) and `no_new_privs`. A separate rpcd grant,
  `/usr/share/rpcd/acl.d/verso-shell.json`, gives the operator's *session* the reads the
  overview needs (network interfaces, devices, dhcp) — a sid grant, distinct from the ubusd
  `verso.json` and the helper's `verso-helper.json`. A third,
  `/usr/share/rpcd/acl.d/verso-admin.json`, holds what an administrator's session
  needs beyond any page's reads: `uci *` read and write, for config history to show
  and act on whatever config it names (ADR-021), and `ubus session list` and
  `destroy`, for Access to list and end other accounts' sessions. It declares all
  of them under `write` alone, as table entries (`"uci": {"*": ["read",
  "write"]}`, `"ubus": {"session": ["list", "destroy"]}`) so each keeps its
  function: a login takes the group up only through its `write` list, and one
  that reads `*` but writes less takes none of it — `session list` answers every
  session's `ubus_rpc_session`, which LuCI's `/ubus` beside Verso would let it
  call as that session. A login whose `write` list names `*`, as root's does,
  takes it up with every other group, so root holds every grant §9 asks without
  LuCI's groups beside Verso's. A pattern takes up every group it matches, so a
  restricted login lists the groups it may use by name and leaves `verso-admin`
  out; a `write` pattern such as `verso-*` matches it and hands over the whole
  group, unless a `!verso-admin` beside it denies it first.

Plugins are confined the same way, and the isolation model separates each by uid: **each
plugin gets its own non-root uid** (`verso-plugin-<name>`), sharing only the `verso` group.
The current image runs the shell alone under uid `6000`; the per-plugin uids are how a
packaged plugin is isolated, not testbed state. A plugin holds no session and
touches config in **neither** direction directly. For
**reads**, the shell reads each config the plugin declares in `acl.read` through
rpcd's `uci get` carrying the operator's sid — one whole-config call — and injects
the result into the plugin's request as the `X-Verso-UCI` header (JSON); the plugin
renders from that snapshot and links no uci library. A declared `ubus` read scope on
object `verso` is brokered the same way through the privileged helper, and rides its
own `X-Verso-Ubus` header keyed by function name. The shell brokers only the core
functions it knows and the reads a plugin's registry declares (`verso-plugin <id>
<name>`, keyed `<id>.<name>`, ADR-027), so a manifest cannot name its way to an
arbitrary helper verb, and the helper re-checks the operator's sid before it acts. For **writes**, the plugin
returns a declarative `commit` intent (ADR-006) and the shell executes it through
rpcd, refusing any op outside the plugin's declared configs. The two gates are
deliberately shaped by rpcd: a write is probe-and-refused up front
(`session.access` per declared scope → 403 before the plugin runs), while a read is
simply sid-scoped at the `uci get` — an operator who may not read a config gets an
empty snapshot, not an error, so `acl.read` names *what* the shell fetches rather
than adding a second gate. rpcd re-checks the operator's sid on every call. A
compromised plugin therefore cannot read a config the operator can't, cannot write
`/etc/config`, cannot act outside what it declared, and cannot exceed the operator's
ACL.

The distinct per-plugin uid is the isolation boundary. `ptrace` and signals are
uid-scoped, so a plugin can reach neither the shell nor a peer's memory. ubusd
grants ACLs per username, so a plugin's uid — having no `acl.d` file — has **zero**
bus access: it cannot reach
the shell's brokered objects (`uci`, `system`, `session`) nor the privileged
`verso-rpcd` root-action socket (Decision 5). Plugins need none — the shell brokers every
read and write. The shared `verso` group serves one purpose only: the shell (in
that group) reaches each plugin's socket (`0660`). It is never what keeps a secret
of the shell's: the web certificate's key is owned by the shell's uid, `0600`
(ADR-017 §3), so a plugin in the group cannot read it.

A socket's path proves nothing about who answers there: whoever binds it while
it is free does, and a plugin restart (an upgrade, a crash and respawn) frees it
between the old socket's removal and the new bind. So the shell takes the kernel's
word instead. Before it writes a byte to a plugin — no request line, no
`X-Verso-UCI` snapshot read with the operator's sid, no `X-Verso-Ubus` read, no
form — the transport reads `SO_PEERCRED` from the connected socket and goes on only
when the listener's uid is the plugin's own account, `verso-plugin-<id>`, looked
up by the manifest's id in `/etc/passwd` (which root writes and no plugin can).
A plugin with no such account is not dialled at all. Any other listener is
refused and the page renders "plugin unavailable", so another plugin holding the
path receives none of the operator's reads or submissions, and nothing it answers
— a commit, a command such as `ssh-key-add` — runs under the plugin's grants.
The liveness probe, which decides whether a plugin reads as running and whether
its pages and panel tabs are offered, makes the same check and writes nothing, so
a listener of another account cannot make a plugin that is down look up.

As defense in depth, a bundled plugin's socket lives in a directory of its own,
`/var/run/verso-plugin-<id>/` (`0750`, owned by the plugin's account, group
`verso`), which its init script makes as root and points the plugin at through
`VERSO_<ID>_SOCKET`. `/var/run` is root's, so no plugin can stand where the
directory belongs before it exists, and no other plugin can create a name inside
it, so a sibling cannot keep the plugin from binding its path. A bundled plugin
that once bound in the shared directory leaves that socket file, its own
account's, in place: while it stands no sibling can bind the old name for a shell
that still dials it during an upgrade. A manifest may name a socket in the shared
`/var/run/verso` (`1770` sticky, group-writable), the SDK's default, where a
plugin whose init script makes no directory of its own binds; there the peer
check alone holds the line, and a sibling that takes the path during a restart
can keep the plugin down but cannot answer as it. The plugin SDK stops a start
whose path it cannot clear (anything but "nothing there") rather than binding
past it.

The one remaining cross-plugin path — a plugin connecting to a peer's socket over
the shared group, an HTTP-only surface with no memory or `ptrace` access — is a
lesser residual left to tighten (a per-plugin socket group) if a plugin ever
handles a secret a peer must never reach.

The sid renewer (Decision 7) lives with the session store: one goroutine that, on
its own interval (well under rpcd's 300 s), calls `session.access` for each live
session's sid — rpcd touches the session on that call — and destroys the sid of any
session the store has swept or a logout has ended. rpcd answers `session.access` for
an unknown sid with `UBUS_STATUS_NOT_FOUND`, distinct from a transport error, so the
renewer tells a genuinely dead session (evict the Verso session, so the next request
lands on the login page with the expiry notice) from a momentary rpcd blip (leave it,
retry next tick). Renewal reads the session store without sliding its idle clock, so a
page left open still signs out on inactivity — the renewer keeps the sid ready for a
live session, it does not keep the session alive.

The root-action helper (Decision 5) installs at `/usr/sbin/verso-rpcd` and runs
under procd. Its socket is `/var/run/verso-rpcd/verso-rpcd.sock`, in a
directory of its own (`0750 root:verso`) rather than the shared `1770` one, so no
plugin can create a name there and bind the helper's path while a restart has
it free; the socket (`0660`) admits only root and the `verso` group. The daemon
uses Linux `SO_PEERCRED` to accept only root or the shell's uid 6000, keeping
plugins that merely share the group out. The check runs both ways: before
writing a byte, the shell (and the root `verso update-check` run, through the
same client) reads `SO_PEERCRED` from its side and talks only to a listener
the kernel reports as root, so the operator's sid, a new password or any other
argument reaches the helper or no one, and no other account's answer is taken
for the helper's. The rpcd **sid** grant
(`/usr/share/rpcd/acl.d/verso-helper.json`) names the group that authorizes each
method, which the operator's session must hold; the Rust daemon probes that ACL
over native ubus before every action. A denial reaches the shell as
`ErrAccessDenied`, which the calling page renders as a notice. `passwd` is fed
the new secret on stdin only (never argv/env). Deploying an ACL edit needs an
`rpcd reload` (session-preserving); deploying the daemon restarts only
`verso-rpcd`, not rpcd or the browser sessions — `make dev` automates both.
