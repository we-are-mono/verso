# Spec Delta

## ADDED Requirements

### Requirement: A card's door into a plugin is that plugin's claim
The Internet and Ports cards SHALL open the door to networks, and the Tunnels card the door to tunnels, else the door to networks. Each such door SHALL lead where the claiming plugin's door leads and say "Open" with that plugin's word for the page. A card whose door no running plugin claims SHALL draw no Go link. The System and Hardware cards SHALL keep opening the shell's own pages in the shell's words.

#### Scenario: Interfaces and VPN running
- **WHEN** the interfaces plugin claims `network` and the VPN plugin claims `tunnel`, both answering
- **THEN** Internet and Ports say "Open Interfaces" and lead to the Interfaces page, and Tunnels says "Open VPN" and leads to the VPN page

#### Scenario: No VPN plugin
- **WHEN** no running plugin claims `tunnel` and the interfaces plugin claims `network`
- **THEN** the Tunnels card says "Open Interfaces" and leads to the Interfaces page

#### Scenario: No interfaces plugin
- **WHEN** no running plugin claims `network` or `tunnel`
- **THEN** Internet, Ports and Tunnels draw no Go link and show their readings as before, while Hardware still opens the Hardware page

#### Scenario: A renamed page
- **WHEN** the VPN plugin's nav entry is labelled Tunnels
- **THEN** the Tunnels card says "Open Tunnels"
