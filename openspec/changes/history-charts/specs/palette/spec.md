# Spec Delta

## MODIFIED Requirements

### Requirement: A chart's wash is a pair of palette colours
The wash under a chart series SHALL run between two palette colour tokens, where it starts at the line and where it fades to, for each series hue. By default the start SHALL be the series' own hue at its strength (16% for the traffic and temperature series, 24% for a role series) and the end SHALL be transparent at the plot's bottom border, so a theme that changes the hue changes its wash and a quiet line rides clean paper.

#### Scenario: The Traffic card under Sand light
- **WHEN** the Overview's Traffic card draws download and upload under Sand light
- **THEN** each series' wash runs from its hue at 16% to nothing at the bottom border

#### Scenario: A theme sets the wash
- **WHEN** the theme in force sets download's wash to start at `#6b9bd640`
- **THEN** download's wash starts at that colour, and the grid rules still show through it
