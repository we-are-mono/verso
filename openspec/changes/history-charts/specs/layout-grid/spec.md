# Spec Delta

## RENAMED Requirements

- FROM: `### Requirement: Chart plots are ruled every 16px`
- TO: `### Requirement: Chart plots are ruled in squares`

## MODIFIED Requirements

### Requirement: Chart plots are ruled in squares
A chart's plot SHALL be ruled in square cells, four cells tall, so each of its four scale labels stands on a rule, and its width SHALL be whole quarters of cells. The cell SHALL be 32px on a page's card (a 128px plot) and 24px in an Overview card or a drawer (a 96px plot). Every rule stands on an 8px grid line.

#### Scenario: A full-size graph
- **WHEN** a page shows a graph in one of its cards
- **THEN** the plot is 128px tall with a rule every 32px across and down

#### Scenario: The Overview's traffic graph
- **WHEN** an operator opens the Overview with the Traffic card open
- **THEN** each lane's plot is 96px tall with a rule every 24px across and down
- **AND** the scale labels stand on the rules at 24, 48 and 72px down and at the top

#### Scenario: A drawer's graph
- **WHEN** an operator opens a device's drawer with its Usage graph
- **THEN** each lane's plot is 96px tall with a rule every 24px across and down
