# WAN discovery

This document defines how Verso discovers WAN interfaces from OpenWrt and Linux
runtime state. It complements ADR-009's shell-owned status baseline and ADR-010's
staged network configuration: configuration may be pending while runtime routing
remains the displayed truth.

## Context

OpenWrt has three related but distinct network identities:

- a **logical interface**, which is a UCI `config interface` and a netifd ubus
  object such as `network.interface.upstream`;
- its configured **transport device**, reported by netifd as `device`, such as a
  physical port or VLAN (`eth4.3900`);
- its active **Layer 3 device**, reported by netifd as `l3_device`, such as the
  PPP interface created above that transport (`pppoe-upstream`).

None of their names carries a guaranteed role. `wan` and `wan6` are OpenWrt
conventions, not reserved names. A valid installation may call its uplink
`upstream`, `isp`, or anything else. Protocol-created netdev names are likewise
implementation details and may be configured.

Treating a logical interface named `wan` as authoritative and propagating its
role through the parent chain is invalid. On a PPPoE connection over a VLAN that
produces three individually accurate but semantically confusing labels:

```text
eth4 -> eth4.3900 -> pppoe-wan
WAN     WAN          WAN
```

Only the final device is routing the connection. The VLAN and physical port are
transports. Their counters and topology remain useful, but they do not share the
same role.

OpenWrt itself resolves this without name matching:

- `package/base-files/files/lib/functions/network.sh` implements
  `network_find_wan()` and `network_find_wan6()` by locating logical interfaces
  with active IPv4 or IPv6 default routes.
- `modules/luci-base/htdocs/luci-static/resources/network.js` implements
  `getWANNetworks()` and `getWAN6Networks()` the same way, returning every match
  and ordering matches by metric.
- netifd's `ubus.c` publishes `l3_device` only for an interface that is up and
  keeps the configured transport separately as `device`.
- firewall4's `root/usr/share/ucode/fw4.uc` maps a logical network to
  `l3_device` first and uses `device` only as a fallback.

The kernel adds cases that netifd alone cannot completely describe. mwan3,
policy-based routing, VRFs, routing daemons, and other software may install
default routes in custom tables. A route may be selected according to source,
mark, incoming interface, user, protocol, or other FIB-rule selectors. Equal-cost
multipath may select several output devices. In such systems there is no truthful
single global WAN.

## Decision

**WAN is a runtime routing role attached to an exact Layer 3 netdev. Verso will
discover zero, one, or many WAN devices from active default routes, without
depending on logical-interface names, kernel-interface names, firewall-zone
names, address ranges, NAT, or physical ancestry.**

### 1. Terminology

- **WAN device:** a distinct Layer 3 netdev participating in an active, reachable
  default route for IPv4, IPv6, or both.
- **Main WAN:** a WAN device participating in the preferred default route in the
  main routing table for an address family. There may be several under ECMP.
- **Policy WAN:** a WAN device reachable only through a non-main routing table and
  an active FIB rule.
- **Transport:** a lower device on which an L3 device depends. A transport is not
  WAN merely because WAN traffic passes through it.
- **Logical owner:** the netifd logical interface whose runtime `l3_device`
  matches the WAN netdev.

“WAN” describes routing topology, not verified Internet reachability. A default
route can exist while the provider or Internet is unavailable. Connectivity and
health are separate states, optionally enriched by a feature such as mwan3.

### 2. Stock OpenWrt discovery

The baseline discovery source is one `network.interface dump` response.

For each logical-interface entry:

1. Consider its active `route` array; never use `inactive.route` to assign a live
   WAN badge.
2. Recognize IPv4 default routes as `target = 0.0.0.0`, `mask = 0` and IPv6
   default routes as `target = ::`, `mask = 0`.
3. Treat a route with no explicit `table` as a main-table route, matching netifd
   and LuCI semantics.
4. Require the logical interface to be up and to expose a non-empty `l3_device`.
5. Attach the WAN role to that exact `l3_device`.
6. Deduplicate devices. IPv4 and IPv6 logical interfaces may share one L3 device,
   as `wan` and a dynamically-created DHCPv6 companion commonly share a PPP
   netdev.
7. Rank otherwise equivalent main routes using the route-specific metric when
   present, then the logical interface's metric, then its name only as a stable
   presentation tie-breaker. A name never changes classification.

This baseline deliberately follows LuCI's plural model: every active default-route
owner is returned. It does not force the first result to be the only WAN.

### 3. Kernel routing extension

Netifd remains the authoritative mapping from logical networks to their active L3
devices, protocols, addresses, and uptime. The installed kernel FIB is the
authoritative source for routes installed outside netifd.

Policy routing and external route managers require rtnetlink discovery in
addition to the baseline:

1. Dump IPv4 and IPv6 routes from all tables with `RTM_GETROUTE`.
2. Retain active unicast default routes only. Exclude blackhole, unreachable,
   prohibit, throw, and other non-forwarding route types.
3. Resolve ordinary output interfaces and every nexthop of a multipath route.
4. Dump IPv4 and IPv6 FIB rules with `RTM_GETRULE`.
5. Treat the main table as reachable by the normal rule chain. Treat a custom
   table as reachable only when an active rule can select it, including fwmark,
   source-routing, and VRF/L3-master rules.
6. Mark each distinct output L3 netdev as a WAN device. A custom-table-only
   result is a policy WAN, not a main WAN.
7. Join the kernel netdev back to all netifd entries whose `l3_device` matches.
   An unmanaged netdev remains a valid WAN even when no logical owner can be
   found; it simply has no UCI network or firewall-zone metadata.

The discovery model must retain the address family, table, metric/priority,
route kind, and logical owner internally. Collapsing discovery immediately into
a single device string would discard information required for split-family,
multi-WAN, and policy-routing installations.

### 4. Interface-table presentation

WAN status is never propagated through `Parent`, bridge membership, VLAN
ancestry, or any other topology relationship.

Logical metadata follows runtime L3 ownership:

- network name, protocol, subnet, and firewall zone attach to the logical
  interface's active `l3_device`;
- when `l3_device` differs from configured `device`, that metadata moves to the
  L3 device rather than being copied to both;
- the configured device retains its independently observed kind, state, counters,
  VLAN identity, physical-port identity, and topology relationships;
- several logical owners sharing one L3 device are merged and deduplicated on
  that row.

For the motivating PPPoE-over-VLAN case the table therefore means:

| Kernel netdev | Presentation role |
|---|---|
| `pppoe-wan` | WAN, logical network and firewall zone |
| `eth4.3900` | VLAN transport |
| `eth4` | Physical port |

No “via” text or duplicated WAN badge is required. The existing topology fields
already preserve the structural relationship for an advanced user.

### 5. Multiple and unusual uplinks

The same rules intentionally produce different cardinalities:

| Configuration | Result |
|---|---|
| PPPoE over a VLAN | PPP L3 device only |
| DHCP over a VLAN | VLAN netdev only |
| DHCP directly on a port | Physical netdev only |
| Full-tunnel VPN | Tunnel device when it owns the active default route |
| Split IPv4 and IPv6 uplinks | One family-specific WAN device for each side |
| Failover routes | Every installed default-route L3 device; metric identifies preference |
| ECMP | Every active nexthop device is a main WAN |
| mwan3 or source/mark policy routing | Every device reached through an active default-route table is a WAN; custom-table-only devices are policy WANs |
| No active default route | No live WAN device |

A single graph or Internet summary must not blindly sum every discovered WAN.
Distinct L3 devices may still be stacked—for example, a tunnel and its provider
underlay can both be reachable through different tables—so summing them can count
the same traffic twice. WAN discovery supplies candidates and routing roles;
each telemetry presentation must choose an explicit main device, present separate
series, or apply a topology-aware aggregation. It must never sum parent and child
interfaces merely because both participate in routing.

### 6. Down state

netifd exposes `l3_device` only while a logical interface is up. A protocol-created
netdev may disappear entirely when disconnected. Therefore:

- an inactive or absent default route does not produce a live WAN badge;
- the configured `device` must not inherit the badge as a down-state fallback;
- `inactive.route` may identify a configured uplink for a separate “configured”
  or “disconnected” status, but it does not identify a live kernel WAN device;
- the Internet summary may report that configured uplinks are down without
  relabelling their transports as WAN.

### 7. Explicit role override

Routing state cannot infer administrative intent in every topology. A dumb access
point may have its management default route through `br-lan`; operationally that
bridge is the router's upstream, while its owner may not consider it WAN. A lab
may deliberately install default routes that should not appear in the ordinary
interface summary.

The design therefore requires an explicit role override keyed by **logical
network**, not by transient kernel-device name. The override can force or suppress
WAN classification. Automatic route-based discovery remains the default. The
override changes presentation intent; it does not alter routes or firewall policy.

### 8. Signals that must not classify WAN

The following may enrich presentation but are never sufficient evidence of a WAN
role:

- a logical or kernel name containing `wan`, `upstream`, `ppp`, or a protocol;
- membership in a firewall zone named `wan`;
- masquerading or MSS-fix configuration;
- a public, private, or link-local address;
- being a physical port, VLAN, tunnel, or point-to-point device;
- being a parent or child of a discovered WAN device;
- having Internet connectivity in an active probe.

Names and firewall policies are arbitrary, address classes do not express traffic
direction, and connectivity probes measure health rather than topology.

## Consequences

### Positive

- Arbitrarily named OpenWrt interfaces work without configuration.
- PPPoE, VLAN, tunnel, and physical layers retain accurate individual counters
  without receiving duplicate semantic roles.
- IPv4 and IPv6 can select different uplinks honestly.
- Dual-WAN, failover, ECMP, mwan3, PBR, VRF, and routing-daemon installations fit
  the same data model instead of adding protocol-specific branches.
- Firewall-zone and logical-network chips follow the same L3 ownership model as
  firewall4.
- The shell remains independent of mwan3, PBR, and other optional packages;
  their plugins may enrich health without owning base classification.

### Costs and limitations

- Backend WAN state is a collection retaining logical owner, L3 device, families,
  route table, metric, and main/policy classification, never one device string.
- Full policy-routing support requires route and rule netlink parsing in addition
  to the netifd dump.
- There is no universal “primary” under source-, mark-, UID-, VRF-, or
  multipath-dependent routing. UI copy must not imply one when the kernel does not
  have one.
- Automatic classification cannot resolve human intent in the management-uplink
  case; the explicit override is the honest escape hatch.
- Route ownership does not prove Internet reachability. Health remains separate.

## Alternatives considered

- **Hardcode `network.interface.wan` and `wan6`.** Rejected because logical
  interface names are arbitrary and split or multi-WAN installations need more
  than those two objects.
- **Match names such as `wan*`, `pppoe-*`, or `upstream`.** Rejected as a naming
  convention disguised as discovery.
- **Use the configured UCI `device`.** Rejected because it identifies the
  transport for protocols such as PPPoE, not the device that owns addresses and
  routes.
- **Mark the complete parent path.** Rejected because it conflates routing role
  with topology and creates the original duplicate badges.
- **Use firewall-zone membership or masquerading.** Rejected because zone names
  and policies are arbitrary, IPv6 uplinks commonly do not use NAT, and internal
  or VPN zones may masquerade too.
- **Use only the main kernel routing table.** Rejected as the complete model
  because policy-routed uplinks would disappear. Retained as the stock OpenWrt
  baseline and the meaning of “main WAN.”
- **Force exactly one WAN.** Rejected because split-family, failover, ECMP, and
  policy-routing systems legitimately have several.
- **Use an Internet probe as discovery.** Rejected because it introduces traffic,
  latency, privacy and captive-portal ambiguity, and still cannot identify every
  valid but temporarily unhealthy uplink.

## Implementation requirements

Discovery belongs in the Verso shell process and uses the existing ubus connection
plus rtnetlink. It requires neither a helper daemon nor per-request command
execution. The implementation combines plural, default-route-based discovery from
`network.interface dump`, exact `l3_device` assignment, L3-owned logical/firewall
metadata, and kernel route and FIB-rule discovery for custom tables, multipath,
unmanaged netdevs, and external route managers.

Tests cover arbitrary logical names, PPPoE over VLAN, direct DHCP, shared
IPv4/IPv6 L3 devices, split families, metrics, multiple defaults, down state,
custom tables, multipath, and the absence of any default route.
