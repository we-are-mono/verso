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
- **`commit` has no section add or delete.** A `commit` op is only `uci set` (+
  commit): it cannot create a section (blocks adding a WireGuard peer or a new
  tunnel) or unset an option (so an emptied optional field is left untouched, not
  cleared). Adding/removing repeating sections needs a section add/delete op or a
  round-trip. Governed by ADR-006/ADR-007; related to the behavioural-pages item.

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

## Research

- **Behavioural pages (F3) — the escape hatch.** The WireGuard plugin
  (`verso-plugin-wireguard`) is the vehicle: it renders and edits tunnels + peers
  and makes the break concrete — there is no way to add or remove a peer with
  today's widgets. Decide and land the escape hatch (shell-owned behavioural
  widgets such as a `repeater`, and/or a declarative round-trip) that adds this
  while preserving crash isolation and central theming. Plugins never ship code.
  Governed by ADR-005 and ADR-006.
