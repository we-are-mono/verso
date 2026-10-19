# Spec Delta

## ADDED Requirements

### Requirement: acme announces its outcomes
While the `verso` section is enabled, acme SHALL announce each outcome of the request or the renewal for the section's domain, the domain as its argument: `issued`, `renewed`, `issue-failed` and `renew-failed`. Each SHALL be seen under the grant of acme's `state` read. A request starting, a catch-up, and an outcome for any other section or domain SHALL announce nothing.

#### Scenario: Issued while the operator waits
- **WHEN** the operator applies a first setup for `router.example.com` and stays on Access, and the request succeeds
- **THEN** the notification says the certificate was issued, and System → Events lists it

#### Scenario: A renewal at night
- **WHEN** acme renews the certificate at night with nobody signed in
- **THEN** System → Events lists the renewal at its time, and no notification says it to anyone who signs in the next morning

#### Scenario: Another section renews
- **WHEN** a second cert section for `nas.example.com` renews
- **THEN** acme announces nothing

#### Scenario: Nothing due
- **WHEN** the nightly renew runs 20 days after issue and acme.sh renews nothing
- **THEN** acme announces nothing

### Requirement: acme's outcomes say what happened
acme's outcomes SHALL read, with the domain in place of `%s`:
- `issued`: "A certificate for %s was issued.", in the success tone;
- `renewed`: "The certificate for %s was renewed.", in the success tone;
- `issue-failed`: "A certificate for %s could not be issued.", in the danger tone;
- `renew-failed`: "The certificate for %s could not be renewed.", in the danger tone.

#### Scenario: Issued
- **WHEN** acme announces `issued` for `router.example.com`
- **THEN** System → Events reads "A certificate for router.example.com was issued." in the success colour

### Requirement: acme's failures alert until a success ends them
`issue-failed` and `renew-failed` SHALL alert. `issued` SHALL end `issue-failed`, and `renewed` SHALL end `renew-failed`. Neither success SHALL alert on its own.

#### Scenario: A failed renewal alerts
- **WHEN** the alert address is set, `verso.alerts.kind` is absent, and the renewal fails
- **THEN** an alert says the certificate for `router.example.com` could not be renewed, and one more is sent once a later renewal succeeds

#### Scenario: A renewal with nothing failed before
- **WHEN** acme renews the certificate and no failure is open
- **THEN** no alert is sent
