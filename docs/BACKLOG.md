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

## Product / definition-of-done

- **Timezone acceptance plugin.** A second plugin written only from
  `docs/plugins.md`, with no shell changes — the "someone else can write one"
  proof.
- **Conformance kit.** An executable check of the plugin contract
  (`docs/plugins.md` carries the checklist).
- **`verso` uci config.** A uci config for durable shell preferences (e.g. a
  reorderable set of dashboard cards).

## Research

- **Behavioural pages (F3).** Build one genuinely behavioural page (a proto-switch
  that swaps its field-set, or WireGuard-style repeating peers), find where the
  static widget schema breaks, and land the escape hatch — shell-owned behavioural
  widgets plus a declarative round-trip — that preserves crash isolation and
  central theming. Plugins never ship code. Governed by ADR-005 and ADR-006.
