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
RUN CGO_ENABLED=0 go build -trimpath -o /out/verso ./cmd/verso
# verso-rpcd is the privileged root helper rpcd runs (ADR-007); a separate,
# single-responsibility binary, not folded into the shell.
RUN CGO_ENABLED=0 go build -trimpath -o /out/verso-rpcd ./cmd/verso-rpcd

# --- runtime: full OpenWrt (procd init), verso as a procd service ----------
FROM openwrt/rootfs:x86-64-24.10.2
COPY --from=build /out/verso /usr/bin/verso
# Installed as `verso` so rpcd names the ubus object "verso".
COPY --from=build /out/verso-rpcd /usr/libexec/rpcd/verso
COPY docker/rootfs/ /
# Disable OpenWrt's network stack, firewall, and DHCP/DNS: netifd flushes eth0's
# Docker-assigned IP and fw4's default input policy is "drop" — both break Docker
# port publishing — and dnsmasq crash-loops under a non-privileged ujail. procd,
# ubusd and rpcd stay up, so `system info` and uci still work.
# Create the non-root `verso` user/group the service drops to (ADR-007). In a
# real .apk this is the package's USERID; here it is baked into the image.
RUN echo 'verso:x:6000:6000:verso:/var/run/verso:/bin/false' >> /etc/passwd \
 && echo 'verso:x:6000:' >> /etc/group \
 # ubusd skips any acl.d file that is group/world-writable or not root-owned
 # (ubusd_acl.c:579-586); git tracks only the exec bit, so normalize here.
 && chmod 0644 /usr/share/acl.d/verso.json /etc/capabilities/verso.json /usr/share/rpcd/acl.d/verso-helper.json \
 && chmod 0755 /usr/libexec/rpcd/verso \
 && chmod +x /etc/init.d/verso /etc/init.d/netfix \
 && rm -f /etc/rc.d/S*firewall /etc/rc.d/S*network /etc/rc.d/S*dnsmasq /etc/rc.d/S*odhcpd* \
 && ( /etc/init.d/verso enable || ln -sf ../init.d/verso /etc/rc.d/S95verso ) \
 && ln -sf ../init.d/netfix /etc/rc.d/S91netfix ; true
EXPOSE 8080
# Boots OpenWrt via procd so verso can reach live ubus/uci. MUST run
# non-privileged with NO /dev/watchdog (see docker-compose.yml): procd then
# cannot grab a host watchdog, so it cannot reboot the host.
ENTRYPOINT ["/sbin/init"]
