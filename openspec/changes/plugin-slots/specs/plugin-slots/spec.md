# Spec Delta

## Purpose

Lets a plugin place what it has (a page block, an act and its drawer, a fact row, a notice, an Overview card) on any page that offers a named place for it. The shell and every other plugin stay unaware of who fills it.

## ADDED Requirements

### Requirement: Shell pages publish named slots
Every page the shell owns SHALL publish a `blocks` slot named `shell.<page>.blocks`, whose anchors are the page's own sections. The Overview SHALL also publish `shell.overview.cards`, whose anchors are its cards. The shell's slot names, their kinds and their anchors SHALL be listed in one published catalogue that the SDK's conformance tests read.

#### Scenario: Access offers its sections as anchors
- **WHEN** a plugin reads the catalogue
- **THEN** it finds `shell.access.blocks` of kind `block`, with anchors `password`, `sessions`, `web` and `luci` in page order

#### Scenario: The catalogue matches the shell
- **WHEN** the shell's tests run
- **THEN** every slot a shell page publishes is in the catalogue with the same kind and anchors, and the catalogue names no slot the shell does not publish

### Requirement: A slot's name says who opened it
A slot opened by a plugin's page SHALL be named `<plugin id>.<name>`, the shell adding the opener's id. A slot opened by the shell SHALL begin with `shell.`. The shell SHALL refuse a manifest whose id is `shell`. A plugin SHALL NOT open a slot under another plugin's id.

#### Scenario: A plugin opens a slot
- **WHEN** verso-plugin-system's certificate card holds a slot with id `certificate.acts`
- **THEN** the slot's name is `system.certificate.acts`, and a contribution naming that slot fills it

#### Scenario: A plugin tries another's namespace
- **WHEN** a plugin with id `foo` opens a slot with id `acme.certificate.acts`
- **THEN** the slot is named `foo.acme.certificate.acts`, and contributions to `acme.certificate.acts` do not fill it

#### Scenario: The reserved id
- **WHEN** a manifest has id `shell`
- **THEN** the shell loads none of it and logs the reason

### Requirement: A slot takes one kind of contribution
Every slot SHALL take exactly one kind: `block` (what a page holds, standing in a page's stack as one of its own blocks would), `act` (one act in an act row, which may open the contributor's drawer), `fact` (rows in a facts list), `notice` (a callout) or `overview-card`. The shell SHALL check each contribution's answer against its slot's kind and SHALL NOT draw an answer of another kind.

#### Scenario: A block where an act belongs
- **WHEN** a contribution to `system.certificate.acts` answers with a section
- **THEN** the act row draws without it, and the shell logs a compatibility warning naming the plugin, the contribution and the slot

#### Scenario: An act opens its drawer
- **WHEN** a contribution to an act slot answers with a link to its own path that opens a drawer
- **THEN** the act stands in the host's act row, and pressing it opens the contributor's page in a drawer over the host page

### Requirement: Plugins fill slots from their manifest
A manifest SHALL declare what it fills in `contributes`. Each entry has an `id` unique in the plugin (lowercase letters, digits and hyphens), a `slot`, a `path` on the plugin, and optionally a `position` and an `order`. The shell SHALL load a manifest whose only contributions are in `contributes`. The shell SHALL also load a manifest whose entries name slots it does not know.

#### Scenario: A contribution-only plugin
- **WHEN** a manifest has no `nav` and one `contributes` entry
- **THEN** the shell loads it, and the plugin has no sidebar row

#### Scenario: A malformed entry
- **WHEN** a `contributes` entry has no `path`, or its `id` repeats another entry's
- **THEN** the shell loads the rest of the manifest, skips that entry, and logs the reason

### Requirement: Contributions stand where their position puts them
A contribution's `position` SHALL be `first`, `last` (the default), `before:<anchor>` or `after:<anchor>`. An anchor is a slot's own item or another contribution, named `<plugin id>.<contribution id>`. Contributions at the same place SHALL be ordered by `order` (default 100), then plugin id, then contribution id.

#### Scenario: Before a shell section
- **WHEN** a contribution to `shell.access.blocks` has position `before:web`
- **THEN** its block stands after every Access section before `web` and directly before `web`

#### Scenario: Two at one place
- **WHEN** two plugins both contribute `after:sessions` with the same order
- **THEN** the one whose plugin id sorts first stands first, on every render

#### Scenario: After another plugin's contribution
- **WHEN** plugin `b` contributes `after:a.status` and plugin `a`'s `status` contribution stands in the slot
- **THEN** `b`'s contribution stands directly after `a`'s

### Requirement: An unknown slot or anchor affects only its contribution
A contribution to a slot that is never opened SHALL draw nowhere, and one whose anchor is not in its slot SHALL stand at `last`. A shell slot missing from the catalogue, or a plugin slot whose opener is not installed, SHALL be logged once per load of the plugins as a compatibility warning naming the plugin, the contribution and what was missing. The rest of the page SHALL draw as it would without it.

#### Scenario: An anchor that is gone
- **WHEN** a contribution names `after:a.status` and plugin `a` is not installed
- **THEN** the contribution stands last in its slot, and the shell logs the missing anchor

#### Scenario: A slot that is gone
- **WHEN** a contribution names `shell.access.footer`, which the catalogue does not hold, or `system.certificate.acts` while verso-plugin-system is not installed
- **THEN** nothing of it is drawn anywhere, and the shell logs the missing slot

#### Scenario: An opener that no longer opens it
- **WHEN** a contribution names `system.certificate.acts` and the installed system plugin's card holds no such slot
- **THEN** nothing of it is drawn, and the rest of the card draws as it would without it

### Requirement: Only the slots being drawn are fetched
The shell SHALL fetch only the contributions to slots that stand on the page, drawer or Overview it is drawing, and it SHALL fetch them in parallel. Each fetch SHALL be bounded by the plugin transport's timeout. A drawer's slots SHALL be fetched when the drawer opens, not with the page under it.

#### Scenario: A drawer's notices
- **WHEN** Access is drawn
- **THEN** the shell does not fetch contributions to the Install drawer's notice slot until the Install drawer opens

#### Scenario: Several plugins on one page
- **WHEN** three plugins contribute to slots on one page and each answers in 200 ms
- **THEN** the page is drawn after about 200 ms, not 600 ms

### Requirement: A contribution can have nothing to show
A contribution SHALL be able to answer that it has nothing to show at this moment. The shell SHALL then draw nothing in its place, list no warning, and keep it in the slot's order for the next render.

#### Scenario: No trusted certificate set up
- **WHEN** acme's Source row contribution answers that it has nothing to show because the `verso` section is disabled
- **THEN** the certificate card's facts end with the system plugin's own rows, and the shell logs nothing about it

### Requirement: A failing contribution is contained
A contribution whose plugin is not running SHALL leave no placeholder. A running plugin's block or notice that times out, fails or cannot be decoded SHALL be drawn as the shell's unavailable state in its place. Such an act or fact SHALL be left out and logged, as no notice fits an act row or facts list. The rest of the page SHALL draw, and it SHALL answer 200.

#### Scenario: The plugin is stopped
- **WHEN** verso-plugin-acme's service is stopped
- **THEN** the certificate card shows no Source row from acme and no "Use your domain" act, and draws no placeholder

#### Scenario: The plugin hangs
- **WHEN** a plugin whose block stands on Access accepts the request and never answers
- **THEN** after the transport's timeout the block's place shows the unavailable state, and the rest of the page is drawn

#### Scenario: A failing act
- **WHEN** acme is running and its "Use your domain" contribution fails
- **THEN** the certificate card's act row draws its own acts alone, and the shell logs the failure

### Requirement: A contribution's form posts to its own plugin
A contributed block that holds a form SHALL post to the page it stands on, naming the contribution. The shell SHALL pass the post only to that contribution's plugin, behind that plugin's write gate. It SHALL read every other contribution afresh as on a GET. A post that stages a change or runs a command SHALL be answered by reading the page again with the outcome as the top-right notification.

#### Scenario: Saving a contributed block
- **WHEN** an operator saves a form inside plugin `a`'s block on Access while plugin `b` also contributes there
- **THEN** only `a` receives the post, `b` is asked for its block as on a GET, and Access is read again with `a`'s notice

#### Scenario: A session without the grant
- **WHEN** the operator's session lacks `a`'s declared write grant and they post `a`'s form
- **THEN** the shell refuses the post without dialing `a`

### Requirement: Contributed facts and acts join the host's list
A properties list and an act row in a plugin's widget tree SHALL each be able to name a slot. The shell SHALL draw contributed fact rows as rows of that same list, after its own, and contributed acts as acts of that same row, after its own.

#### Scenario: A source fact
- **WHEN** the certificate card's facts name the slot `certificate.source` and acme contributes a Source row
- **THEN** Source is a row of the card's one facts list, read in the same column as Issued to and Expires

### Requirement: Slots are filled two levels deep
The shell SHALL fill slots in a page's or drawer's own widget tree, and slots in a contribution standing in them. It SHALL NOT fill a slot inside a contribution that itself stands in a contribution. A drawer that a contributed act opens is the contributor's own page, and its slots SHALL be filled.

#### Scenario: A slot in a contribution
- **WHEN** verso-plugin-system's part of Access, a contribution to `shell.access.blocks`, opens `certificate.acts`, and acme contributes "Use your domain" to it
- **THEN** the act stands in the certificate card's act row

#### Scenario: A slot two contributions deep
- **WHEN** acme's contribution to `system.certificate.acts` holds a slot that a third plugin fills
- **THEN** the slot draws nothing, and the shell makes no fetch for it

#### Scenario: A slot in a contributed drawer
- **WHEN** acme's drawer, opened from "Use your domain", holds a slot that another plugin fills
- **THEN** the drawer draws that plugin's contribution

### Requirement: Contributions speak the reader's language
The shell SHALL ask each contribution in the reader's language and translate it with the contributing plugin's catalogue, as it does that plugin's pages.

#### Scenario: Slovenian
- **WHEN** a reader whose language is Slovenian opens Access, and acme ships a Slovenian catalogue
- **THEN** acme's Source row and act read in Slovenian, translated from acme's catalogue

### Requirement: The shell names the contract features it speaks
On every request to a plugin, the shell SHALL send `X-Verso-Features`: the contract features it speaks, comma-separated, including `slots`. A feature the shell does not speak SHALL NOT be named.

#### Scenario: A plugin checks for slots
- **WHEN** a plugin receives a request whose `X-Verso-Features` includes `slots`
- **THEN** the plugin can rely on its opened slots being filled and its contributions being drawn

### Requirement: The Access part key is read as a contribution
A manifest's `system_access` SHALL be read as a contribution with id `access`, slot `shell.access.blocks`, position `before:web` and its path. A manifest that declares both `system_access` and a contribution to `shell.access.blocks` SHALL be read from `contributes` alone.

#### Scenario: An installed plugin from before slots
- **WHEN** a plugin whose manifest declares only `"system_access": "/access"` is installed
- **THEN** its part stands on Access before the web interface's section, and its forms save as before

### Requirement: A removed plugin leaves no trace
Removing a plugin's package SHALL remove every one of its contributions and every slot it opens from every page at the next render. The router SHALL keep no record of where the plugin's contributions stood.

#### Scenario: Removing acme
- **WHEN** the operator removes verso-plugin-acme on Packages
- **THEN** Access's certificate card shows no Source row from acme and no "Use your domain" act, the Overview shows no Certificate card, and nothing on the router names acme's contributions
