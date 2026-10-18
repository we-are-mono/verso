# Release notes (draft)

Drafts for the GitHub releases of the tags that carry this change: Verso's section, then verso-plugin-acme's for its own tag.

## Verso

### Changes to your router

- **History shows every change the router makes by itself.** Installing a certificate by hand, a package arriving switched off, choosing a theme and moving the web interface's ports each leave a row in System › Changes. Before, the router took these in without a row, so history could show a setting the router no longer held.
- **History names who caused each change.** A change you started (a theme, the ports, a package from Packages) reads as yours, from your browser. One the router made on its own reads "The router". Notes say what caused it, for example "A certificate was installed by hand."
- **What a package changes as it installs is part of the install.** A package's own scripts often change settings as it installs, is removed or is upgraded. https-dns-proxy, for one, takes over dnsmasq's servers as it starts. When you do this from Packages, those changes read as yours, with Notes naming the package: "Changed by https-dns-proxy as it was installed."
- **history.db takes three steps (10 to 12) at the first start.** Every earlier row is kept. A Verso from before this release refuses the newer file and keeps its history in memory, so a downgrade never deletes it.

### For plugin authors

- **BREAKING (SDK):** `root::own_write(config)` is now `root::own_write(plugin, config, cause)`. `cause` is one English sentence of at most 512 bytes, and a key of your plugin's catalog; history shows it translated. A plugin built on the earlier SDK keeps working: its commits read as the router's, with no cause.

### Upgrade

```sh
apk update && apk upgrade
```

Upgrade verso and verso-plugin-acme together.

## verso-plugin-acme

When a certificate is installed by hand, the plugin turns its renewal off, and config history now shows that change as the router's, with "A certificate was installed by hand." Build it with the SDK of the Verso released beside it.
