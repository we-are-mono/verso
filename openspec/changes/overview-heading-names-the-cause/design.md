# Design

## Context

See proposal.md for why. `Overview.issues()` counts what needs attention (a firewall not fully loaded, a port in use or a tunnel without link, a Resources meter out of band) and `verdict()` turns the count into one of four fixed sentences. Each card's header status comes from its own `*Head` function in `overview_cards.go`. Hardware's status is left out of the count, and a hidden Hardware card stops the sensor reads in both the page render (`server.go`) and the stream (`events.go`).

## Goals / Non-Goals

**Goals:**
- One rule for the heading, with no card excepted, so a card added later joins the verdict by saying its cause.
- The heading's words for a cause are the card's own, said so they stand without the card.

**Non-Goals:**
- Linking the heading to the card. The card wears the same tone in its header; the words name it.
- Changing a card's header status. A header keeps the words that read in context ("1 of 2 down").

## Decisions

### A card says its cause beside its status

`cardHead` gains `Cause`: the sentence the heading uses when the card is the one thing wrong, set wherever the head's tone is warning or danger. Ports and Resources and Hardware already read on their own, so their cause is their status. Tunnels' status "1 of 2 down" names no tunnel without the card, so its cause is "1 of 2 tunnels down". The firewall has no card; its cause is "The firewall is not running" or "The firewall is only partly loaded".

The alternative, the heading composing a sentence from a card's title and status, puts words together the catalog cannot translate whole.

### The verdict collects causes, then picks one sentence

`causes()` replaces `issues()` and returns each cause with its tone, Internet aside (the uplink is the verdict's own first question). `verdict()`:

1. uplink known and down: offline, danger;
2. one cause: its sentence, its tone;
3. several: the count, through the plural helper, danger when any cause is danger and warning otherwise;
4. uplink up: online, success;
5. otherwise: at a glance, neutral.

The live status and the first paint read the same `verdict()`, so nothing else changes in the stream.

### Hidden Hardware is read, not streamed

The sensors are read whenever the board has them, in the render and once per sample in the stream, because the verdict needs them. The stream sends its sensor frame only while Hardware is shown. A sysfs read every five seconds is the cost.

## Risks / Trade-offs

- A board's sensors now move the heading on a healthy network. Hardware grades only against the zone's or chip's own limits, never a guessed one, so a warm board that is within its limits says nothing.
