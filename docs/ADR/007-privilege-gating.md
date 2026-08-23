# ADR-007 — Privilege gating: act through rpcd's ACLs with the session, not as ambient root

- **Status:** Accepted
- **Date:** 2026-08-23
- **Deciders:** tomaz@zaman.io
- **Relates to:** ADR-001 (single static binary; supersedes its "runs as root,
  acts freely" spike posture for the privileged surface), ADR-003 (the `Backend`
  seam, where this swap lands), ADR-006 (plugin contract; adds a manifest ACL
  declaration). Closes the privilege-gating gap
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
   ubus objects (`uci`, `file`, …) with the session's `sid` in the payload. rpcd
   is the single component that both **authorizes and executes**; Verso never
   decides its own privilege.

2. **The `Backend` seam absorbs the swap (ADR-003).** The interface stays; its
   implementation moves from go-uci-direct + no-argument native invoke to
   rpcd-object calls (`uci.get/set/commit/apply`, `file.*`) carrying the sid. The
   sid is plumbed from the session into backend calls (via context or an explicit
   parameter). Tests, which use fakes, stay valid — the seam pays off again.

3. **The endgame is de-privileging the shell.** Acting through rpcd is the
   on-ramp; the target is a Verso process that *cannot* bypass rpcd (dropped
   capabilities, no ambient write path), so a compromised or buggy shell is
   bounded by the operator's ACL rather than by root. This supersedes ADR-001's
   spike posture for the privileged surface.

4. **Plugins present credentials too (extends ADR-006).** A plugin declares the
   ACL scopes it needs in its `manifest.json` (the Verso analog of LuCI's
   per-plugin `acl.d`). The shell, which holds the sid, brokers privileged
   operations through rpcd on the plugin's behalf and refuses anything outside the
   plugin's declared, session-authorized scopes. Plugins gain a *declared*
   privilege surface instead of ambient root.

5. **Staged, not big-bang.** This is the target architecture, implemented
   incrementally behind the `Backend` seam and the manifest — not in one cut. The
   security review rates this a device blocker; shipping to real hardware waits
   on it.

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
- go-uci's direct read/write/commit is **dropped for the gated surface**; uci
  operations are re-expressed as rpcd ubus calls, and rpcd becomes a hard runtime
  dependency for writes (it already is for login).
- A per-operation authorization/execution **round-trip** (cacheable per sid
  within its lifetime).
- The native-ubus investment (`internal/ubus`) is **not** wasted — it is exactly
  what carries the sid-bearing calls — but the go-uci write path built for the
  spike is superseded.
- **De-privileging the shell is real work** (which capabilities remain necessary,
  socket permissions for a non-root client) and is the harder half of the endgame.

### Neutral
- The `Backend` interface gains session-awareness; this is behind the seam and
  invisible to the widget renderer.
- Governs privilege only; the visual contract (ADR-005) and the mechanical plugin
  contract (ADR-006) are unchanged except for the added manifest ACL field.

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
  for the objects it brokers (`session`, `uci`, `system`), because ubusd
  ACL-checks non-root callers; rpcd still applies the per-operator sid gating on
  top. That file must be root-owned and not group/world-writable, or ubusd skips
  it (`ubusd_acl.c` `ubusd_acl_load`). The shell keeps only `CAP_NET_BIND_SERVICE`
  (to bind :80/:443) and `no_new_privs`.

Plugins are confined the same way: a plugin runs as the same non-root `verso`
user, never writes config, and holds no session. It returns a declarative
`commit` intent (ADR-006) and the shell executes the write through rpcd, refusing
any op outside the plugin's declared configs and with rpcd re-checking the
operator's sid. A compromised plugin therefore cannot write `/etc/config`, cannot
broker a write outside what it declared, and cannot exceed the operator's ACL.
Sharing the shell's uid does not isolate a plugin from the shell process itself
(same-uid signal/ptrace).
