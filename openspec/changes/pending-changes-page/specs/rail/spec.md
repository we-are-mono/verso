# Spec Delta

## ADDED Requirements

### Requirement: The stage's row stands first in the rail
While the stage holds a change, the rail SHALL show a Pending changes row above every destination. It SHALL belong to no group, SHALL NOT open or close any group, and SHALL carry the rail's marker while the Pending changes page is open. Under the pointer it SHALL slide as every rail row does. While the stage is clean, the row SHALL be absent and the rail SHALL start with Overview.

#### Scenario: Changes waiting
- **WHEN** the stage holds changes and the operator is on Firewall › Rules
- **THEN** the rail's first row is Pending changes, Firewall's group is open below it, and Rules carries the marker

#### Scenario: On the Pending changes page
- **WHEN** the operator opens the Pending changes page
- **THEN** the Pending changes row carries the marker and every group is closed

#### Scenario: A clean stage
- **WHEN** nothing is pending, even while a form holds an unsaved edit
- **THEN** the rail's first row is Overview

### Requirement: The phone's menu button carries the stage's count
On a layout where the rail sits behind the menu button, the menu button SHALL show the pending count beside its icon while the stage holds a change. An unsaved edit SHALL NOT change it.

#### Scenario: A phone with changes waiting
- **WHEN** the rail is behind the menu button and 12 changes are pending
- **THEN** the menu button reads its icon and 12

#### Scenario: Editing on a phone
- **WHEN** 12 changes are pending and the operator edits a rule without saving
- **THEN** the menu button still reads 12
