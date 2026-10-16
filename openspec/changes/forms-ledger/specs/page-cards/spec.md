# Spec Delta

## MODIFIED Requirements

### Requirement: Each ruled section of a page is a card
Every top-level section of a page that has a title SHALL render as a card: Paper, a single Strong Hairline frame with 2px corners, a header holding the section's glyph when it has one, its title and its lede, and its content behind a Strong Hairline, the frame's own ink. Cards stand 24px apart in one column at the page's measure, and the blocks inside a card stand 24px apart; a titled part inside a card starts 40px on.

#### Scenario: Firewall settings
- **WHEN** an operator opens Firewall settings
- **THEN** "When no zone applies", "Protection", "Speed", "TCP and helpers" and "Custom rules" each stand in their own card, in that order, 24px apart

#### Scenario: A section's lede
- **WHEN** a section carries a lede ("Restoring replaces the saved files and reboots the router.")
- **THEN** the lede stands in the card's header on the line under the title, at weight 400
- **AND** the header's hairline stands under the lede

#### Scenario: The header's hairline
- **WHEN** an operator opens the Protection card on Firewall settings
- **THEN** the hairline under "Protection" is the frame's ink
- **AND** the hairlines between the card's fields are the lighter Hairline

#### Scenario: Blocks in a card
- **WHEN** an operator opens Maintenance
- **THEN** the Firmware card's ledger, its upload act, the packages notice and the automatic-check switch stand 24px apart

#### Scenario: A part inside a section
- **WHEN** a section holds a part (SSH's "Authorized keys")
- **THEN** the part stands inside the section's card on its ledger line, not as a card of its own
