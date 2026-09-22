# ADR-016 — Firewall packet log isolation

- **Status:** Accepted
- **Date:** 2026-09-14
- **Relates to:** ADR-007 (privileged reads), ADR-010 (staged configuration)

## Context

Stock firewall4 emits nftables `log prefix …` statements. These use the kernel
logger, so packet matches enter the kernel ring, logd, remote system logging,
and potentially the serial console. Filtering System Logs in the browser cannot
prevent that traffic from overwhelming diagnostics visible through SSH or UART.

## Decision

Use NFLOG group **4242** for fw4-managed packet logging. The persistent root
helper owns the netlink socket and an independent RAM buffer. Firewall Activity
reads that buffer through the existing SID-checked helper protocol. System Logs
continues to read logd, retaining service errors and normal kernel diagnostics.

### Generate the correct transport before installing rules

The supported firewall4 release has no UCI option for an NFLOG group. Verso
therefore checks NFLOG kernel support, then augments the transport clause in the installed fw4 logging templates:
`log prefix` becomes `log group 4242 snaplen 256 queue-threshold 1 prefix`.
This changes neither UCI rule settings nor matches, verdicts, counters, prefixes,
or rate limits. Upstream templates are edited in place rather than forked into
the package; the inverse edit preserves other upstream and local changes.

`verso-rpcd` starts at **18**, before firewall at **19**. Its local root setup
prepares the templates before fw4 can generate logging rules. A named fw4 script
include also repairs the templates following package replacement and migrates
existing plain log statements in `inet fw4` through a single nft transaction.
Explicit NFLOG groups belonging to other collectors are left alone. Group 4242
is reserved for Verso. Migration preserves counter snapshots; packets arriving
between reading and replacing a counter may be absent from that snapshot.

Setup serializes against fw4's reload lock. The post-reload include runs under
that lock already and never invokes another reload. Its UCI include is installed
using a private configuration/package name, so an operator's staged changes are
neither committed nor discarded. Package removal removes this include, reverses
only our transport clause and restores live group-4242 statements to the normal
OpenWrt logging transport. Stopping the collector alone does **not** restore
kernel logging.

The package requires `firewall4` and `kmod-nfnetlink-log`. Docker uses its host's
NFLOG module and needs only its existing `NET_ADMIN` capability; no kernel-log
sysctl or privileged container is required.

### Bound packet history and expose failures

The collector retains at most **2,048 entries**, each capped at **1,024 bytes**.
It copies at most 256 bytes per packet and retains only parsed IP/transport
header metadata, interface names, prefix and timestamp. It never persists packet
history or sends it to syslog. Oldest entries are evicted when the buffer fills;
a helper restart clears history. Reads return at most 500 entries.

Readers have independent cursors, including a process generation to distinguish
restarts even when the new event ID has already overtaken the old one. Lost
history and netlink receive overruns are reported. The browser displays an
unavailable state if the collector cannot bind or the helper cannot be reached.
The collector retries binding without changing logging back to the kernel.

Routing failures are recorded in a root-owned runtime marker and reported as
unavailable, without disabling unrelated helper operations. Successful setup or
reload clears the marker. Unsupported kernels fail setup before template changes.

The `firewallLog` helper read requires the caller's rpcd SID and corresponding
read ACL. It does not widen the zero-SID automation grant. Template/routing
commands are local-root CLI operations, with no remote write method.

### Keep diagnostics separate

System Logs omits recognized legacy kernel packet entries by default while
advancing its logd cursor across them. This hides entries already in the old
ring; it is additional to source isolation. Existing kernel/logd history is not
erased and remains accessible through SSH until normal rotation removes it.

“Include firewall traffic” merges NFLOG history into the browser's System Logs
view with source `firewall`, separate event IDs and chronological ordering.
It does not reconfigure logging, persist history or forward packets into logd.
Downloads contain the visible rows.

## Consequences and limits

- This integration couples Verso to fw4's logging template syntax. Required
  templates are validated, and the real OpenWrt integration test exercises
  generation, delivery, reload, restart and removal. Future fw4 releases must
  be tested before being supported. The integration test currently uses OpenWrt
  25.12.4 with firewall4 `2025.03.17~b6e51575-r2`.
- Updating `firewall4` independently can overwrite the modified templates.
  The post-reload include repairs them and migrates live rules, but that first
  reload can briefly install stock logging statements. For continuous isolation
  during such an upgrade, reapply `/usr/libexec/verso/firewall-logging-setup`
  before its first reload; package scripts that reload first can create that
  window. A native
  upstream fw4 NFLOG option would remove this integration and upgrade caveat.
- Handwritten nft includes with plain `log` statements bypass template
  generation. Use `log group 4242 snaplen 256 queue-threshold 1` directly in those
  includes to avoid a post-reload migration window. Tables outside `inet fw4`
  and logging from other software are outside this integration.
- A stopped or overwhelmed collector can lose packet history. It never falls
  back to the kernel logger and never changes packet acceptance or rejection.
- Firewall packet events no longer reach an existing remote **system** syslog
  destination. Dedicated remote packet-log export is a separate future feature.

## Verification

`make test`, `make build`, and `make i18n-audit` cover the protocol, permission
gate, bounded parser/history, separate server streams and localization.
`python3 scripts/test-firewall-logging.py` exercises real fw4/NFLOG in a disposable
OpenWrt container with no host network changes. `scripts/dev-livelog-feed.sh`
sends real probes from the testbed's WAN simulator instead of injecting syslog.


## References

- [nftables packet logging and NFLOG](https://wiki.nftables.org/wiki-nftables/index.php/Logging_traffic)
- [firewall4 rule template](https://github.com/openwrt/firewall4/blob/master/root/usr/share/firewall4/templates/rule.uc)
