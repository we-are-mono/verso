# Proposal

## Why

The Overview's heading reads "Your network needs attention" whenever anything is out of band, and says nothing of what. The reader has to hunt the cards for it, the cause may sit below the fold or in a hidden card, and it is often not the network at all: a full disk or a hot CPU reads as a network problem. A heading that names a vague problem is the page's loudest sentence saying the least. After this change the heading is the most important true sentence on the page: online, offline, the one thing wrong in its own words, or how many things are wrong.

## What Changes

- The verdict is one rule, with no card excepted:
  - the uplink down: "Your network is offline" (danger), whatever else is wrong;
  - exactly one cause: that cause, as a sentence that stands on its own ("Storage is high", "Tunnel wg0 is down", "cluster is hot"), in its tone;
  - two or more: "N things need attention" (warning, or danger when any cause is);
  - nothing wrong and the uplink up: "Your network is online" (success);
  - nothing known: "Your network at a glance" (neutral).
- A cause is a firewall not fully loaded, a port in use or a tunnel without link, a Resources reading out of band, or a Hardware reading out of band. Hardware joins the verdict: the heading no longer claims to speak for the network alone, and Hardware grades only against the board's own limits.
- Hidden cards still count. A hidden Hardware card's sensors are still read for the verdict; only its frames stop.
- Each card that can raise a cause says it as a stand-alone sentence for the heading, beside the status its header shows in context.
- The live stream carries the heading as it changes, as it does today.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `overview`: "One verdict heads the page" names the cause; "The Hardware card stays out of the verdict" is removed; "Hidden cards are neither rendered nor streamed" reads sensors for the verdict while Hardware is hidden.

## Impact

- **ADR-022 (Overview cards)**: its verdict paragraph says the page names the cause, and that every card's problem counts, Hardware included. Amended in place.
- Shell: `internal/widget/overview_status.go` (causes and the verdict), `internal/widget/overview_cards.go` (each card head's cause sentence), `internal/server/server.go` and `internal/server/events.go` (sensors read for the verdict while Hardware is hidden).
- Catalog: the cause sentences and the count of things that need attention in `i18n/sl/base.json`.
- No on-device state moves, so there is no migration.
