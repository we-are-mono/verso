# SPDX-License-Identifier: GPL-2.0-only
# SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
# syntax=docker/dockerfile:1

# --- build: compile the static Verso binary --------------------------------
FROM golang:1.27.2-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY profiles/ ./profiles/
RUN CGO_ENABLED=0 go build -trimpath -o /out/verso ./cmd/verso

# --- runtime: full OpenWrt (procd init), verso as a procd service ----------
# 25.12 = the apk-based series the Mono image targets (25.12.5); the hub's
# newest published rootfs tag is .4 — bump when .5 lands.
# verso-rpcd and the plugins are not baked in: `make dev` builds, deploys and
# enables each one on the running container.
FROM openwrt/rootfs:x86-64-25.12.4
COPY --from=build /out/verso /usr/bin/verso
COPY docker/rootfs/ /
# The full network stack runs (netifd, dnsmasq, odhcpd, fw4): /etc/config/network
# declares eth0's Docker address as the static mgmt interface and the firewall's
# mgmt zone keeps input open there, so port publishing survives — while wan0 and
# br-lan (the testbed networks in docker-compose.yml) behave like a real
# router's ports, IPv6 included (DHCPv6-PD in, RA + DHCPv6 out).
# Create the non-root `verso` user/group the service drops to (ADR-007). In a
# real .apk this is the package's USERID; here it is baked into the image.
# The minimal OpenWrt rootfs omits its CA bundle even though the distribution
# feeds use HTTPS. Bootstrap the signed ca-bundle package with transport
# verification disabled for this one transaction; apk still verifies the
# repository and package signatures against OpenWrt's bundled signing keys.
RUN apk add --no-check-certificate ca-bundle \
 # The package depends on nlbwmon for usage history (ADR-018).
 && apk add nlbwmon \
 # What verso-plugin-ddns depends on, for `make dev` to deploy it beside the rest.
 && apk add ddns-scripts ddns-scripts-cloudflare ddns-scripts-noip \
 && echo 'verso:x:6000:6000:verso:/var/run/verso:/bin/false' >> /etc/passwd \
 && echo 'verso:x:6000:' >> /etc/group \
 # ubusd skips any acl.d file that is group/world-writable or not root-owned
 # (ubusd_acl.c:579-586); git tracks only the exec bit, so normalize here.
 && chmod 0644 /usr/share/acl.d/verso.json /etc/capabilities/verso.json /usr/share/rpcd/acl.d/verso-helper.json /usr/share/rpcd/acl.d/verso-shell.json /usr/share/rpcd/acl.d/verso-admin.json \
 && chmod 0644 /etc/config/verso \
 # The daily update check ships as package content in a real install (the script,
 # the crontab line, and crond enabled); the dev image carries the same pieces so
 # the unattended path is exercised here before it ships. A real install's
 # post-install seeds the opt-in (ADR-014 §2); mirror that here so `make dev`
 # runs with the switch on, as a fresh install ships.
 && uci -q set verso.updates=updates \
 && uci -q set verso.updates.autocheck=1 \
 # The dev image runs LuCI's uhttpd on 80/443 as the reference UI, so the shell
 # answers beside it: the shell over HTTP on 8080 as `make dev` serves it, and
 # over HTTPS on 8443.
 && uci -q set verso.web=web \
 && uci -q add_list verso.web.listen_http=0.0.0.0:8080 \
 && uci -q add_list verso.web.listen_https=0.0.0.0:8443 \
 && uci -q set verso.web.redirect_https=0 \
 && uci -q commit verso \
 && chmod 0755 /usr/libexec/verso/update-check /usr/libexec/verso/web-owner /usr/libexec/verso/usage-setup \
 && /usr/libexec/verso/usage-setup \
 && chmod 0600 /etc/crontabs/root \
 && chmod 0755 /usr/libexec/verso/firewall-logging /usr/libexec/verso/firewall-logging-setup \
 && chmod +x /etc/init.d/verso /etc/init.d/verso-rpcd /etc/init.d/netfix \
 # No ujail in an unprivileged container: it cannot clone namespaces (EPERM),
 # which turns jailed services (dnsmasq) into crash loops. Without the binary,
 # procd runs every instance plain — the same skip verso's own jail params
 # already get here (see /etc/init.d/verso). Jails apply on real hardware.
 && rm -f /sbin/ujail \
 && ( /etc/init.d/verso enable || ln -sf ../init.d/verso /etc/rc.d/S95verso ) \
 && ( /etc/init.d/cron enable || ln -sf ../init.d/cron /etc/rc.d/S50cron ) \
 && ( /etc/init.d/nlbwmon enable || ln -sf ../init.d/nlbwmon /etc/rc.d/S60nlbwmon ) \
 && ln -sf ../init.d/netfix /etc/rc.d/S91netfix
EXPOSE 8080
# Boots OpenWrt via procd so verso can reach live ubus/uci. MUST run
# non-privileged with NO /dev/watchdog (see docker-compose.yml): procd then
# cannot grab a host watchdog, so it cannot reboot the host.
ENTRYPOINT ["/sbin/init"]
