# verso-plugin-system

System › General (the router's name, time and theme), and the router's own part of
System › Access: SSH and the HTTPS certificate.

## Slots it opens

Other plugins fill these from their manifest's `contributes` (see `docs/plugins.md`,
"Filling slots"). A certificate source, such as verso-plugin-acme, uses all three.

| slot | kind | where |
|---|---|---|
| `system.certificate.source` | fact | the HTTPS certificate card's facts, after its own rows |
| `system.certificate.acts` | act | the card's act row, after Install, Make a new one and Download |
| `system.certificate.notices` | notice | over the form of the Install and Make drawers |

The plugin itself fills `shell.access.blocks` with its part of Access, before the web
interface's section, at order 10, so its subjects lead the plugins' parts.
