# Spec Delta

## Purpose

Lets an operator edit a daemon's hand-written files (dnsmasq options, fw4 rule files,
OpenVPN profiles) through the same stage, apply, rollback and history as uci, with each
file family declared by the plugin that owns the daemon rather than built into core.

## ADDED Requirements

### Requirement: A plugin declares the file families it owns
A plugin SHALL declare each family of hand-edited files in its `commands.json`. A family has:
- a name;
- the uci config that owns it;
- its fixed files and at most one directory;
- its file mode;
- the words its refusals use;
- a check program and a reload program;
- optionally, a transform command of the same plugin.

Core SHALL hold no family of its own.

#### Scenario: The firewall's rule files
- **WHEN** verso-plugin-firewall is installed
- **THEN** a family `fw4` exists, owned by the `firewall` config, placed in `/etc/nftables.d`, checked by `fw4 check` and reloaded by `fw4 -q restart`

#### Scenario: Without the plugin
- **WHEN** verso-plugin-vpn is not installed
- **THEN** no family places files in `/etc/openvpn`, and the helper neither lists nor stages any file there

### Requirement: A family's places are bounded
A family's files and directory SHALL lie under `/etc`. The helper SHALL refuse a family whose places include or lie inside `/etc/config`, `/etc/verso`, `/etc/passwd`, `/etc/shadow`, `/etc/group`, `/etc/rpcd`, `/etc/dropbear`, `/etc/ssl`, `/etc/init.d`, `/etc/rc.d`, `/etc/hotplug.d`, `/etc/crontabs` or `/etc/uci-defaults`. It SHALL refuse two families that share a name or a place, and log each refusal.

#### Scenario: A family that names the shadow file
- **WHEN** a registry declares a family whose fixed file is `/etc/shadow`
- **THEN** that family is refused with a logged reason, and nothing lists or stages `/etc/shadow`

#### Scenario: Two plugins claim one directory
- **WHEN** two installed plugins each declare a family in `/etc/openvpn`
- **THEN** both families are refused, the log names both plugins, and every other family still works

### Requirement: A plugin reads the files of configs it reads
The shell SHALL give a plugin that declares the files read the files of each family whose owning config the plugin may read, and no other family's.

#### Scenario: The firewall page
- **WHEN** the firewall plugin, reading the `firewall` and `dhcp` configs, asks for files
- **THEN** it receives the `fw4` and `dnsmasq` families' files and none of OpenVPN's

#### Scenario: A page lists its own family
- **WHEN** a page's file set is handed the files of more than one family
- **THEN** it lists, opens and checks new names against its own family's files alone, so the firewall's rule files never list `/etc/dnsmasq.conf`

### Requirement: A plugin stages the files of configs it writes
The shell SHALL stage a family's file only for a plugin that declares file staging and may write the family's owning config. The helper SHALL refuse a stage when:
- the path belongs to no family;
- the file changed since it was read;
- the content is over 32 KiB or holds a NUL;
- the family's transform refuses it.

A staged file SHALL change nothing that runs until it is applied.

#### Scenario: A stale save
- **WHEN** the operator saves a rule file that another browser saved since this one opened it
- **THEN** the save is refused with "This file changed. Reopen it before saving." and nothing is staged

#### Scenario: A plugin outside the owning config
- **WHEN** a plugin that may write only `qos` asks to stage `/etc/nftables.d/10-custom.nft`
- **THEN** the shell refuses it without asking the helper

### Requirement: A family's transform shapes what leaves root
When a family names a transform, the helper SHALL pass each listed file through it. The transform SHALL:
- fold the content shown;
- withhold, by path alone, a file that is not to be opened;
- put held material back into a save and refuse a save it cannot place.

No content the transform folds or withholds SHALL leave the helper.

#### Scenario: An OpenVPN profile
- **WHEN** the VPN page lists `/etc/openvpn/proton.ovpn`, which carries an inline `<tls-crypt>` block
- **THEN** the content shows the block folded to one line, and saving an edit above it puts the original block back

#### Scenario: A sign-in file
- **WHEN** `/etc/openvpn/proton.auth` is in the directory and no instance reads it as a profile
- **THEN** it is listed by path alone, with no content or version

#### Scenario: A transform that fails
- **WHEN** the family's transform command fails or times out while files are listed
- **THEN** that family's files are listed by path alone, with no content, and no other family is affected

### Requirement: Apply checks, reloads and rolls back each family
Applying staged files SHALL first run each touched family's check with the files in place. On a refusal the apply SHALL restore the previous files and say the check's complaint. After a kept apply, each touched family's reload SHALL run. On a rollback, the previous files SHALL be restored and each touched family reloaded.

#### Scenario: A rule file fw4 rejects
- **WHEN** the operator applies a rule file with a syntax error
- **THEN** the apply is refused naming fw4's complaint, and the previous rule files are in place

#### Scenario: A rolled-back apply
- **WHEN** an apply that changed a dnsmasq options file is not confirmed in time
- **THEN** the previous file is back in place and dnsmasq has reloaded it

### Requirement: Config history records files as listed
Config history SHALL record each applied file's text before and after, as the family's listing gives it. Folded content SHALL be recorded folded, and a withheld file by path alone.

#### Scenario: Applying a profile edit
- **WHEN** the operator applies an edit to `/etc/openvpn/proton.ovpn`
- **THEN** its config history entry shows the edit with the key blocks folded, and the history database holds no key material

### Requirement: Staged files survive the move of their family
An upgrade that moves a family's declaration from core to its plugin SHALL keep the family's name, places and file versions. A file staged before the upgrade SHALL still be listed as pending, and still apply or discard, afterwards.

#### Scenario: A staged rule file across the upgrade
- **WHEN** a rule file is staged, then verso and verso-plugin-firewall upgrade before it is applied
- **THEN** the Pending changes page still lists it, and applying it puts it in place
