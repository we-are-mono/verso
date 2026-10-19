# Notes

## 3.2 — the Wi-Fi read in the lab (2026-10-19)

- The wireless repo's `deploy-dev.sh` installed `commands.json` root-owned, and the helper ran `wireless.state` for a session granted `verso-plugin wireless state`: the grant and the ownership walk pass.
- The container has no radio, so netifd has no `network.wireless` object: the read fails ("ubus call network.wireless status ended with exit status: 4"), the helper logs it, and the page shows what the config says. The Go read failed the same way there.
- With a stand-in `network.wireless` object (an rpcd exec plugin answering radio0 up with `default_radio0` on `phy0-ap0`) and radio0 enabled in the config, the helper answered `{"radios":{"radio0":{"up":true}},"networks":{"default_radio0":{"up":true}}}`. The page drew the network "up" and radio0 "on"; channel numbers stayed dashes, since iwinfo has no such device. The stand-in and the config change were removed afterwards.
- Screenshot: `~/Share/screenshots/shell-names-no-plugin-3.2-wifi-live-read.png`.
- A router with a radio is the check that the numbers (channel, power, busy share, clients) arrive.
