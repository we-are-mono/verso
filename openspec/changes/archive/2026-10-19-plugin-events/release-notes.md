# Release notes (draft)

Drafts for the GitHub releases of the tags that carry this change: Verso's section, then verso-plugin-acme's for its own tag.

## Verso

### New

- **What a plugin does on its own reaches you.** A plugin can now tell the router when something happens long after you asked for it, such as a certificate renewed at night or a request that finishes a minute after you applied it. The router lists it on System › Events with everything else that happened, in your language.
- **You see it where you are looking.** When such an event happens while you are signed in, it appears at the top right of the page you are looking at, once per browser however many tabs you have open. A tab in the background shows it when you bring it forward within a minute; after that, System › Events has it.
- **Notifications stack.** The outcome of your own action keeps its place at the top, and events stand under it, each leaving on its own: a success after a few seconds, a failure when you close it. At most three events stand at once.
- **A plugin's failures can alert you.** Each installed plugin that reports failures is a kind of its own under System › Events › Alerts, by its name. A failure alerts once and once more when it is resolved, even across a restart of the router's web interface.
- **Replacing the web certificate is recorded.** Every replacement of the certificate the web interface serves, by hand, made on Access, or renewed, is listed on System › Events with the name it is for and the day it expires.

### Changes to your router

- **Plugin alerts are on unless you chose otherwise.** If you never picked which kinds alert, every kind alerts, an installed plugin's failures included. If you picked kinds before, nothing changes until you tick the plugin's box.
- **Alerts survive a restart.** An outage or a failure that is still going on when the web interface restarts is remembered: you are not alerted for it twice, and you hear when it ends.

### For plugin authors

- **Declare and announce events.** Declare each event in your `commands.json` (`events`: name, sentence, tone, read, notify, alert, ends) and announce it from your root half with the SDK's `root::announce(plugin, event, subject, args)`, or `verso announce <plugin>.<event>` from a script, as root. See "Events" in docs/plugins.md.
- **React to any event through procd.** `procd_add_raw_trigger "verso.<owner>.<event>"` in your init script runs your command as root when it happens, as `config.change` does. The command gets none of the event's data, so read the router's state yourself.
- **Reserved plugin ids.** A plugin may not take the id `wan`, `boot`, `changes`, `rollback`, `storage`, `certificate`, `firmware`, `device` or `dhcp`.

### Upgrade

```sh
apk update && apk upgrade
```

## verso-plugin-acme

The certificate's outcomes reach you as they happen: issued, renewed, or a failure of either is listed on System › Events and appears at the top right of the page you are on. A failed renewal can alert you under "Trusted certificate" in System › Events › Alerts, once, and once more when a later renewal succeeds. It needs the Verso released beside it.
