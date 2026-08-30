# SPDX-License-Identifier: GPL-2.0-only
# SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
# syntax=docker/dockerfile:1

# --- build: compile the static Verso binary --------------------------------
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY profiles/ ./profiles/
RUN CGO_ENABLED=0 go build -trimpath -o /out/verso ./cmd/verso

# The privileged companion is a small persistent Rust daemon. Unlike an rpcd
# exec plugin it is not respawned for every method invocation.
FROM rust:1.98-alpine AS helper-build
WORKDIR /src
COPY verso-rpcd/Cargo.toml verso-rpcd/Cargo.lock ./
COPY verso-rpcd/src/ ./src/
RUN cargo build --locked --release

# --- runtime: full OpenWrt (procd init), verso as a procd service ----------
# 25.12 = the apk-based series the Mono image targets (25.12.5); the hub's
# newest published rootfs tag is .4 — bump when .5 lands.
FROM openwrt/rootfs:x86-64-25.12.4
COPY --from=build /out/verso /usr/bin/verso
COPY --from=helper-build /src/target/release/verso-rpcd /usr/sbin/verso-rpcd
COPY docker/rootfs/ /
# The full network stack runs (netifd, dnsmasq, odhcpd, fw4): /etc/config/network
# declares eth0's Docker address as the static mgmt interface and the firewall's
# mgmt zone keeps input open there, so port publishing survives — while wan0 and
# br-lan (the testbed networks in docker-compose.yml) behave like a real
# router's ports, IPv6 included (DHCPv6-PD in, RA + DHCPv6 out).
# Create the non-root `verso` user/group the service drops to (ADR-007). In a
# real .apk this is the package's USERID; here it is baked into the image.
RUN echo 'verso:x:6000:6000:verso:/var/run/verso:/bin/false' >> /etc/passwd \
 && echo 'verso:x:6000:' >> /etc/group \
 # ubusd skips any acl.d file that is group/world-writable or not root-owned
 # (ubusd_acl.c:579-586); git tracks only the exec bit, so normalize here.
 && chmod 0644 /usr/share/acl.d/verso.json /etc/capabilities/verso.json /usr/share/rpcd/acl.d/verso-helper.json /usr/share/rpcd/acl.d/verso-shell.json \
 && chmod 0755 /usr/sbin/verso-rpcd \
 && chmod +x /etc/init.d/verso /etc/init.d/verso-rpcd /etc/init.d/netfix \
 # No ujail in an unprivileged container: it cannot clone namespaces (EPERM),
 # which turns jailed services (dnsmasq) into crash loops. Without the binary,
 # procd runs every instance plain — the same skip verso's own jail params
 # already get here (see /etc/init.d/verso). Jails apply on real hardware.
 && rm -f /sbin/ujail \
 && ( /etc/init.d/verso-rpcd enable || ln -sf ../init.d/verso-rpcd /etc/rc.d/S94verso-rpcd ) \
 && ( /etc/init.d/verso enable || ln -sf ../init.d/verso /etc/rc.d/S95verso ) \
 && ln -sf ../init.d/netfix /etc/rc.d/S91netfix ; true
EXPOSE 8080
# Boots OpenWrt via procd so verso can reach live ubus/uci. MUST run
# non-privileged with NO /dev/watchdog (see docker-compose.yml): procd then
# cannot grab a host watchdog, so it cannot reboot the host.
ENTRYPOINT ["/sbin/init"]
