# Backlog

**This file exists to be deleted.** It is a transient queue of open, deferred work
— nothing here is a durable record. Delete a done item's line outright — do not
mark it done, strike it through, or keep a "done" section; git history is the
record. When the last item is gone, delete the file.

Durable records live elsewhere: decisions in `docs/ADR/`, current behaviour in the
code and `docs/`. This is the one place forward-looking "deferred / next / TODO"
language belongs — keep it out of ADRs and code comments. Each item stands on its
own and points at the decision that governs it; the rationale lives there, not here.

## Security (open audit items)

- **VS-05 — HTTPS + `Secure` cookie.** Serve over TLS and set the session cookie
  `Secure`. The dev container is plain HTTP, so the flag is conditional on `r.TLS`
  today.
- **VS-08 — require a root password before serving.** When root has an empty
  password, refuse to serve (or force a password to be set) rather than only
  showing the warning banner.
- **VS-10 — bind LAN-only.** `VERSO_ADDR` is configurable; default it to the LAN
  interface and keep the shell off the WAN (bind address + firewall).

## Hardening

- **Per-plugin uid.** Plugins share the shell's `verso` uid, so a plugin is not
  isolated from the shell *process* (same-uid signal/ptrace). A dedicated uid per
  plugin isolates them. Governed by ADR-007.
- **Verify capabilities on real hardware.** `CAP_NET_BIND_SERVICE` and
  `no_new_privs` are applied through procd's ujail, which the non-privileged dev
  container cannot run — confirm they take effect on device. See ADR-007 and
  `docker/rootfs/etc/init.d/verso`.
- **Package the `verso` user.** The `.apk` must create the `verso` user/group
  (OpenWrt `USERID`); `apk add` does not. It is baked into the dev image only.
  Governed by ADR-007.

## Correctness

- **Clear-all NTP servers.** Whether an empty list in a brokered `uci.set` clears
  the option or leaves it stale is unverified (`internal/openwrt`).
- **`list` widget add/remove.** The `list` widget renders a fixed set of rows with
  no client-side add/remove. Blocked on the behavioural-pages item below.
- **`commit` cannot unset an option.** A `commit` op is `uci set`, so an emptied
  optional field is left untouched rather than cleared; the WireGuard plugin omits
  empty scalars from its commit for this reason. Clearing a value needs a delete op.
  Governed by ADR-007. (Adding and removing repeating sections is the repeater's
  job — ADR-005 §7 — not the commit's.)

## Validation (ADR-008)

- **A `cidr` datatype.** WireGuard addresses and allowed-IPs are CIDR
  (`10.0.0.1/24`); `internal/datatype` has `ipaddr` (a bare address) but no CIDR
  type, so the WireGuard plugin declares no datatype on those fields. Add a `cidr`
  (v4/v6 `addr/len`) datatype so they validate. Governed by ADR-008.
- **Optional fields can't declare a datatype.** The shell validates every field
  carrying a `datatype` against its value on POST, and every datatype rejects the
  empty string — so an optional field (a WireGuard `listen_port`, `endpoint_host`)
  can't declare one without a blank value failing the save. Needs a
  validate-if-present notion (skip empty, or a `required` flag). Governed by ADR-008.

## Product / definition-of-done

- **Timezone acceptance plugin.** A second plugin written only from
  `docs/plugins.md`, with no shell changes — the "someone else can write one"
  proof.
- **Conformance kit.** An executable check of the plugin contract
  (`docs/plugins.md` carries the checklist).
- **`verso` uci config.** A uci config for durable shell preferences (e.g. a
  reorderable set of dashboard cards).

## Per-device usage (ADR-018)

- **First Devices visit on the router.** The first history read after the
  shell starts runs `nlbw` once per day of the last 62; time it on the DK.
- **nlbwmon writes in place.** Send a temp-file-and-rename save upstream and
  carry it in the Mono feed; the helper's watchdog covers stock installs.
- **SELinux: label `/data/nlbwmon`** before the policy goes enforcing.
- **Overview usage.** The sentence naming the heavy user and the household's
  month tile (agreed, not built).
- **Destinations.** What a device talks to needs a source for names that is
  not logs, and a privacy decision.

## Tooling

- **Router pull-watcher.** A small procd service on a dev router that polls a
  version stamp on the apk repo and runs the upgrade line in
  `docs/building.md` §5 when it changes, so publishing from the dev box is the
  whole loop and the router updates itself, with no SSH into it. Dev-only;
  ships disabled.

## Research

- **Behavioural pages — the remaining edge.** Three realizations landed in the
  WireGuard plugin under the declare-intent/shell-realizes rule (ADR-005 §7):
  `repeater` (add/remove peers, rpcd round-trip), `conditional` (pre-shared-key
  toggle, pure CSS), and a form `action` (generate keypair, plugin-computed
  round-trip). The remaining edge is a field-set gated on a multi-value `select` (a
  proto switch): as a radio group it stays pure-CSS, but as a dropdown it would be
  the first case needing JavaScript. A candidate next behavioural widget. Governed
  by ADR-005.
