# Notes

## 8.1 The router's own commits on `make dev`

Checked 2026-10-19 in the lab container at v0.9.0-r74 with verso-plugin-acme 0.1.0-r12, through headless Chromium. Screenshots are in `~/Share/screenshots/history-own`. System › Changes read, newest first:

| Act | Changed | Notes | By |
|---|---|---|---|
| A theme chosen on General's Appearance card | `verso.ui theme not set → sand-light` | The theme was changed. | root, this browser |
| A certificate installed by hand on Access, with `acme.verso` on | Turned off the trusted certificate for router.example.com | A certificate was installed by hand. | The router |
| https-dns-proxy installed from Packages | `https-dns-proxy.config changed, https-dns-proxy.cfg032d79 removed` | The package arrived switched off, so it changes nothing until it is turned on. | root, this browser |

The `acme.verso` turn-on before the hand install was made with the command-line `uci commit`, and read "Edited directly", as it should.

**Seen beside the arrival.** A `dhcp` row, "Edited directly", landed in the same minute as the https-dns-proxy row. https-dns-proxy's own init script rewrites dnsmasq's servers when the arrival stops and starts the service, and nothing marks that commit. It reads as made outside Verso, which is true of the package's script.

**Restored afterwards.** The lab's own certificate and theme were put back, and https-dns-proxy was removed.

## On a router in service

On 2026-10-19 a Mono Gateway in daily use was upgraded with local apks, from verso 0.9.0-r65 and verso-plugin-acme 0.1.0-r11 to 0.9.0-r83 and 0.1.0-r12, in one `apk add`. apk upgraded acme first and verso last.

- **history.db.** Changes still listed all six earlier rows and the starting point, so history.db took steps 10 to 12 and kept what it held.
- **The theme.** The owner chose a theme on General's Appearance card and set it back. Each change read "The theme was changed.", by root from the owner's own address.

Not run on that router: the hand install (it would turn off ACME for the owner's real domain) and a package install. Both ran in the lab (8.1, 9.5), and the hand install's reboot waits for the DUT (8.2).

## 9.5 What a package's scripts commit during an install

The `dhcp` row seen beside the arrival is a package operation's own (design decision 7). The check was repeated at v0.9.0-r76 on `make dev`:

| Changed | Notes | By |
|---|---|---|
| `https-dns-proxy.config changed, https-dns-proxy.cfg032d79 removed` | The package arrived switched off, so it changes nothing until it is turned on. | root, this browser |
| `dhcp.cfg01411c changed` | Changed by https-dns-proxy as it was installed. | root, this browser |
| `dhcp.cfg01411c changed` | Changed by https-dns-proxy as it was installed. | root, this browser |

The two `dhcp` rows are the package's own service taking dnsmasq over as it started, and handing it back when the arrival stopped it. The arrival's commit kept its own cause.

Removing the package from Packages answered "https-dns-proxy removed." and left no row. With the arrival's settings, the package's stop leaves dnsmasq alone, so nothing declared changed. The removal's cause is held by the unit tests. Screenshot: `package-removal.png`.
