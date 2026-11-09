# Configuration

Verso keeps its own settings in one uci config, `/etc/config/verso` ([ADR-013](ADR/013-verso-settings.md)). Every option is optional: an absent option means the default in the tables below.

Most settings have a place in the web interface, named under each section. To change one by hand, edit it with `uci`, then restart the shell:

```sh
uci commit verso && /etc/init.d/verso restart
```

`uci show verso` prints what is set.

Plugins keep their own configs, which their pages edit. This page covers the shell's.

## web — where Verso answers

**System → Access**, Web interface. Changing the ports there opens the new ones beside the old, sends the browser over, and closes the old ones only once it arrives ([ADR-017](ADR/017-tls-and-the-web-certificate.md)).

| Option | Type | Default | Meaning |
|---|---|---|---|
| `listen_https` | list | `0.0.0.0:443` `[::]:443` | HTTPS listeners, each `address:port`, or a bare port for every address |
| `listen_http` | list | `0.0.0.0:80` `[::]:80` | HTTP listeners, the same form |
| `redirect_https` | boolean | `1` | HTTP answers with a redirect to HTTPS; any value but `0` is on |

With neither list set, Verso takes 443 and 80. If another web server holds them, such as LuCI's uhttpd, Verso answers on HTTPS alone at 8443. Listeners that are set are bound as written.

```sh
uci set verso.web=web
uci add_list verso.web.listen_https='0.0.0.0:9443'
uci add_list verso.web.listen_https='[::]:9443'
uci add_list verso.web.listen_http='9080'
uci set verso.web.redirect_https='1'
```

## updates — checking for updates

**System → Maintenance** ([ADR-014](ADR/014-unattended-update-checks.md)).

| Option | Type | Default | Meaning |
|---|---|---|---|
| `autocheck` | boolean | `1` on first install | The router checks for updates once a day; only `1` is on |

A check only looks; nothing installs on its own. The first install writes `1`, and the Maintenance toggle writes `1` or `0`, so a `0` survives every upgrade.

```sh
uci set verso.updates=updates
uci set verso.updates.autocheck='0'
```

## history — what the router records

No page; set by hand ([ADR-019](ADR/019-history-and-events.md), [ADR-021](ADR/021-config-history.md)).

| Option | Type | Default | Meaning |
|---|---|---|---|
| `directory` | path | `/data/verso` where `/data` is a mount, else `/usr/share/verso/history` | Where `history.db` is kept |
| `changes` | whole number | `1000` | How many entries config history keeps; anything not a whole number is ignored |
| `probe` | list | none | Internet addresses the uplink is probed at, beside its gateway |

A new `directory` takes effect at the next start, which moves the history there from the old one. The router reaches no address on the internet unless `probe` lists it.

```sh
uci set verso.history=history
uci set verso.history.directory='/mnt/usb/verso'
uci set verso.history.changes='5000'
uci add_list verso.history.probe='9.9.9.9'
uci add_list verso.history.probe='2620:fe::fe'
```

## alerts — where alerts go

**System → Events**, Alerts ([ADR-020](ADR/020-alerts.md)).

| Option | Type | Default | Meaning |
|---|---|---|---|
| `url` | URL | none | Where alerts are posted, `http://` or `https://`; nothing is sent without it |
| `heartbeat` | URL | none | Sent a GET every five minutes, so an outside monitor notices when the router goes quiet |
| `kind` | list | every kind | Which kinds alert |

The kinds are `wan` (the internet going down and coming back), `boot` (a start after losing power or crashing), `rollback` (applied changes the router undid because nobody confirmed them), `storage` (the history not being saved), and the id of each installed plugin that declares an alerting event.

```sh
uci set verso.alerts=alerts
uci set verso.alerts.url='https://ntfy.example.com/router'
uci add_list verso.alerts.kind='wan'
uci add_list verso.alerts.kind='rollback'
```

## ui — how Verso looks

**System → General**, Appearance ([ADR-024](ADR/024-themes.md)).

| Option | Type | Default | Meaning |
|---|---|---|---|
| `theme` | theme id | `sand` | The theme for everyone who signs in |

`sand` follows the browser's light or dark preference, `sand-light` and `sand-dark` hold one, and every other theme is named by its file in `/usr/share/verso/themes` without `.json`, such as `tokyo-night-storm`. A name no theme answers draws as `sand`.

```sh
uci set verso.ui=ui
uci set verso.ui.theme='catppuccin-mocha'
```
