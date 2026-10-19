# Spec Delta

## Purpose

Lets a plugin's root side, and core's, announce what happened on the router: the shell records it with the router's other events, says it to the operators looking at the router, alerts on it where the operator asked, and lets other plugins react to it through procd, so an outcome that comes after its request still reaches the operator.

## ADDED Requirements

### Requirement: Root announces an event through one command
`verso announce <owner>.<event> [--subject <device>] [<arg>…]` SHALL be the one way to announce an event; the certificate's install and `verso certificate announce` SHALL announce through it. It SHALL refuse unless run as root. It SHALL leave the event for the shell, with the time it was announced. It SHALL refuse an event its owner does not declare, and arguments that do not fill the declared sentence. A refused announcement SHALL exit non-zero, say why on stderr, and leave nothing behind.

#### Scenario: A root half announces
- **WHEN** acme's root half runs `verso announce acme.renewed router.example.com` as root, and acme declares `renewed` with the sentence "The certificate for %s was renewed."
- **THEN** the command exits 0 and the shell records the event within a few seconds

#### Scenario: Not root
- **WHEN** the shell's account or a plugin's account runs `verso announce acme.renewed router.example.com`
- **THEN** the command exits non-zero and nothing is announced

#### Scenario: An undeclared event
- **WHEN** a root half announces `acme.expired`, which acme's `commands.json` does not declare
- **THEN** the command exits non-zero naming the undeclared event, and nothing is announced

#### Scenario: Arguments that do not fill the sentence
- **WHEN** a root half announces `acme.renewed` with no argument, and its sentence holds one `%s`
- **THEN** the command exits non-zero and nothing is announced

### Requirement: Announcing never waits on the shell
An announcement SHALL be kept until the shell takes it, across a restart of the shell, up to 256 announcements waiting. Past that, `verso announce` SHALL refuse with exit status non-zero, and the announcements already waiting SHALL be kept. The announcing process SHALL never wait on the shell or on a reaction.

#### Scenario: The shell restarting
- **WHEN** acme announces a renewal while procd is restarting the shell
- **THEN** the started shell records the renewal

#### Scenario: The shell stopped
- **WHEN** 256 announcements wait because the shell is stopped, and a root half announces another
- **THEN** that announcement is refused, and the 256 waiting are recorded once the shell starts

### Requirement: A plugin declares its events
A plugin's `commands.json` SHALL declare each event it announces in `events`: its name; its sentence, in English, with `%s` for each argument; its tone (`success`, `info`, `warning` or `danger`); the registered read whose grant lets a session see it; whether it alerts; and, optionally, the event of its own that it ends. A declaration naming a read the registry lacks, or an unknown tone, SHALL be ignored with a logged reason, and that plugin's other declarations kept.

#### Scenario: A declaration with no such read
- **WHEN** a plugin declares an event whose read is `status` and its registry registers no read `status`
- **THEN** the shell logs that the event is ignored, announcing it is refused, and the plugin's other events are recorded

#### Scenario: A registry from before events
- **WHEN** a plugin's `commands.json` has no `events`
- **THEN** the plugin's commands work as before, and announcing any event as that plugin is refused

### Requirement: Core's owners are reserved
The owners `wan`, `boot`, `changes`, `storage` and `certificate` SHALL be core's. A plugin with one of those ids SHALL have its event declarations ignored, with a logged reason.

#### Scenario: A plugin named for a core owner
- **WHEN** a plugin with the id `certificate` declares an event `replaced`
- **THEN** its declaration is ignored, and `certificate.replaced` stays core's

### Requirement: Each event is recorded once, in the order announced
The shell SHALL take waiting announcements every two seconds, oldest first, and record each as an event of the router's history, at the time it was announced, kind `<owner>.<event>`. An announcement the shell has recorded SHALL NOT be recorded again, even when the shell stopped before it let the announcement go. An announcement not written by root, or whose owner no longer declares it, SHALL be dropped unrecorded with a logged reason.

#### Scenario: A crash between recording and letting go
- **WHEN** the shell records an announcement and is killed before it removes the announcement
- **THEN** the started shell finds it, records nothing new for it, and removes it

#### Scenario: Two announcements in one pass
- **WHEN** acme announces `renew-failed` and then `renewed` within two seconds
- **THEN** System → Events lists `renewed` above `renew-failed`, each at the time it was announced

#### Scenario: A plugin removed with an announcement waiting
- **WHEN** acme announces an event and is removed before the shell takes it
- **THEN** the event is dropped and the log names it

### Requirement: The Events page says a plugin's event in the reader's language
System → Events SHALL list a plugin's event with the router's others: its time, its subject in the Interface column, and its sentence in the reader's language, translated through the plugin's catalog with the arguments filled in as written, coloured by its tone. An event whose plugin has been removed SHALL keep its row, its sentence in English.

#### Scenario: Read in Slovenian
- **WHEN** an operator reading in Slovenian opens System → Events after acme announced `renewed` for `router.example.com`
- **THEN** the row reads acme's Slovenian sentence with `router.example.com` in it, in the success colour

#### Scenario: A sentence no catalog holds
- **WHEN** the reader's language has no translation of the sentence
- **THEN** the row reads the English sentence with its arguments filled in

#### Scenario: The plugin removed
- **WHEN** acme is removed after announcing `renewed`
- **THEN** System → Events still lists the row, in English

### Requirement: A plugin's event shows under its read's grant
A plugin's event SHALL show, on System → Events and as a notification, only to a session that rpcd grants the read its declaration names, scope `verso-plugin`. An event whose plugin has been removed SHALL show only to a session that may read the whole of config history, as an event of an unknown kind does.

#### Scenario: A session without the grant
- **WHEN** a login whose groups do not grant `verso-plugin acme state` opens System → Events
- **THEN** no acme event is listed, and none is said to that session

### Requirement: An event is said to the operators looking at the router
Every plugin event the shell records SHALL be said as the top-right notification to each signed-in session allowed to see it: on one of the session's open pages as it is recorded, or, with no page open, on the next page the session opens within a minute of the event. It SHALL be said once per session, in the session's language, in the tone it declares. The router's own events (`wan`, `boot`, `changes`, `storage`, `certificate`) SHALL NOT be said this way.

#### Scenario: A page open
- **WHEN** an operator has Access open and acme's request for the domain succeeds
- **THEN** the notification says the certificate was issued, without the operator doing anything

#### Scenario: Two pages open
- **WHEN** the same session has two pages open as the event is recorded
- **THEN** one of them says it, and the other does not

#### Scenario: Back within a minute
- **WHEN** an operator has no page open as the event is recorded, and opens one 30 seconds later
- **THEN** the page says the event as it loads

#### Scenario: Back later
- **WHEN** an operator opens a page five minutes after the event
- **THEN** no notification says it, and System → Events lists it

#### Scenario: The internet went down
- **WHEN** the router records the internet going down while an operator has a page open
- **THEN** no notification says it, as before this change

### Requirement: A page listens only while its sign-in lasts
A page's stream of events SHALL end when its session ends, and holding it open SHALL NOT keep the session alive or move its idle clock.

#### Scenario: An idle page
- **WHEN** a page stays open with nobody using it until the session's idle limit passes
- **THEN** the session ends at its limit, the stream ends with it, and the page reconnects to the sign-in page

### Requirement: A plugin's alerting events are an alert kind
System → Events → Alerts SHALL list each installed plugin that declares an alerting event as a kind of its own, by the plugin's name, beside the router's four. With `verso.alerts.kind` absent, every kind SHALL alert, the plugins' included. An alert SHALL be titled with the router's hostname and the event's sentence in English, its arguments filled in, and say when it happened.

#### Scenario: Chosen kinds without acme
- **WHEN** `verso.alerts.kind` lists `wan` and `boot`, and acme announces `renew-failed`, which alerts
- **THEN** no alert is sent

#### Scenario: No kinds chosen
- **WHEN** `verso.alerts.kind` is absent and acme announces `renew-failed`
- **THEN** an alert titled "home-router: The certificate for router.example.com could not be renewed." is sent, saying at what time

#### Scenario: An event that does not alert
- **WHEN** acme announces `renewed`, which it declares as not alerting, and opens no incident
- **THEN** no alert is sent

### Requirement: An event that ends another closes its incident
An alerting event that another event is declared to end SHALL open an incident for its kind and subject, and that event SHALL close it, as the internet coming back closes an outage (ADR-020). The same opening event while its incident is open SHALL send nothing more. A closed incident whose alert was sent SHALL send one more message, naming how long it lasted.

#### Scenario: A failure that lasts
- **WHEN** acme announces `renew-failed` on three nights in a row, then `renewed`
- **THEN** one alert is sent for the failure, none for the second and third nights, and one when it is renewed, naming how long it lasted

### Requirement: An incident's first alert waits a minute
An incident's first alert SHALL wait a minute before it is sent, so an incident closed within it sends one message saying when it began and when it ended.

#### Scenario: A failure that recovers within the minute
- **WHEN** acme announces `renew-failed`, then `renewed` 30 seconds later
- **THEN** one alert is sent, saying when the failure began and when it ended

### Requirement: An alerting event nothing ends alerts each time
An alerting event that no event is declared to end SHALL send one alert each time it is recorded.

#### Scenario: Two alike in a day
- **WHEN** a plugin announces, twice in a day, an alerting event no event of its ends
- **THEN** two alerts are sent

### Requirement: Plugins react through procd
An announcement SHALL also be sent to procd as the service event `verso.<owner>.<event>`, its data the subject and the arguments, so a plugin's init script reacts with `procd_add_raw_trigger`, as it reacts to `config.change`. The shell SHALL send nothing to a plugin's socket for an event.

#### Scenario: A plugin reacts
- **WHEN** a plugin's init script declares `procd_add_raw_trigger "verso.certificate.replaced" 1000 /usr/bin/verso-plugin-x root reload-tls` and the web certificate is replaced
- **THEN** procd runs that command as root about a second later

#### Scenario: Nobody reacts
- **WHEN** no installed service declares a trigger for the event
- **THEN** the event is recorded and said as usual, and nothing else runs

### Requirement: The web certificate's replacement is an event
Replacing the certificate the router serves SHALL record `certificate.replaced`, naming the certificate's name and the day it expires, and send `verso.certificate.replaced` to procd, whether it was installed by hand, made on Access, or installed for acme by the router's install command. A hand install or a made certificate SHALL still announce `hotplug-call verso-certificate` with `ACTION=installed` first, waiting for its scripts, as before.

#### Scenario: Installed by hand
- **WHEN** the operator installs a certificate for `router.example.com` valid until 2027-01-17 through Access
- **THEN** System → Events lists that the web interface's certificate was replaced, for `router.example.com` until 2027-01-17, and acme's section is already off when Install answers

#### Scenario: Renewed by acme
- **WHEN** acme renews the certificate and hands it to the router's install command
- **THEN** System → Events lists acme's renewal and the certificate's replacement, and the next handshake serves the new certificate

#### Scenario: Seen by an operator who may not see the certificate
- **WHEN** a session rpcd does not grant `ubus verso accessCredentials` opens System → Events
- **THEN** the certificate's replacement is not listed
