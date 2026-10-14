# Proposal

## Why

An operator with a profiled board has no way to see from the landing page whether the box itself is healthy: how hot it runs, what each power rail draws, whether a fan has stopped. Those readings sit on the Hardware page only. The board profile already names exactly the sensors worth showing, and where Linux reads each one, so the Overview can show them without guessing.

## What Changes

- The Overview gains a **Hardware** card on a board that has a profile: its Thermal, Power and Fans sections, each listing the profile's entries in the profile's order, under the names the profile gives them. A board with no profile has no Hardware card.
- Each reading is live. Temperatures show °C, power rails W, fans RPM. A reading the chip marks faulted shows "—"; a fan reading zero with no fault shows "—", quietly, as not connected.
- The card's header speaks only when a reading is out of band: a temperature past its own warn or critical limit (a thermal zone's trip points, or the chip's own limits), or a fan fault. Closed, it shows the CPU temperature (the entry flagged `cpu`).
- The Hardware card's status does not move the page's verdict, which speaks for the network. Hidden, the card takes no readings.
- The card's door opens the Hardware page, which keeps every reading the box exposes, profiled or not.
- The Overview's stream sends a `sensors` frame again, only while the Hardware card is shown, every 5 seconds: most of the profile's readings are i2c transactions behind a mux, and router CPU and the i2c bus are the budget.
- A saved layout that names `hardware` puts the card back where it stood on a profiled board. On a board with no profile, `hardware` is an unknown card: dropped, and not listed on the shelf.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `overview`: the Overview shows seven cards: Hardware joins Internet, Ports, Tunnels, Traffic, Resources and System, standing only on a profiled board; the Hardware card's sections, readings, header status and closed reading; the `sensors` frame is sent only while the Hardware card is shown.

## Impact

- **ADR-022 (Overview cards)** is amended in place: the board's sensors are on the Overview, in the Hardware card, on a profiled board only; the `sensors` frame and its cadence; a layout naming `hardware` restores the card; the Hardware card's status stays out of the verdict.
- **ADR-009 (shell boundary)**: the card is shell-owned, as the read-only status baseline is; no plugin is involved.
- `internal/widget/overview_cards.go` (the card's head and body), the overview stream (the `sensors` frame, gated by the layout), the layout parser (`hardware` as a known card id).
- `internal/sensors`: the profile's thermal, power and fan entries resolved for the card, graded exactly as the Hardware page grades them.
- `profiles/README.md`: the "homepage" paragraph states what the Hardware card shows.
- `DESIGN.md` "Overview cards" and the Slovenian catalog (`i18n/sl/base.json`) for the card's strings.
