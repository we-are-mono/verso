# Design

## Context

See proposal.md for why; the requirements are in `specs/overview/spec.md`.

What exists today:

- **Cards** are a fixed registry, `overviewCardSpecs()` in `internal/widget/overview_cards.go`: each spec has an id, a `head` and a `body`. `LiveStatus` walks the same registry, so a new spec's header reaches the stream's `overview` frame and the Arrange shelf without client changes.
- **The layout** (`internal/widget/overview_layout.go`) knows one fixed card set (`overviewDefault`) and maps the retired `hardware` and `firmware` ids onto `system` (`overviewMerged`). The parser has no idea which board it runs on.
- **The stream** (`internal/server/events.go`) ticks once a second per connection and skips any frame only hidden cards read. The `sensors` frame, the old Hardware card and `internal/server/sensors.go` were removed in `90fdc4d`. The client code that wrote `data-verso-prop` rows in place survives as `writeSpan`, which the `clock` and `wan` frames use.
- **Sensors** resolve through `sensors.ResolveAll(profile, reader)`, which grades each temperature against its own limits (`gradeTemp`, `tripLimits`) and gives each fan a `State` (`running` | `fault` | `absent`). Its accessors serve the Hardware page's layout, not profile order. `CuratedTemps` skips the `cpu` entry. `OtherPowers` sorts by draw. Power and fan readings come out in hwmon enumeration order, and unprofiled rails are mixed in.
- **The verdict** (`issues()` in `overview_status.go`) counts named fields (firewall, interfaces, `SysMetrics`), not card heads.
- **The profile** is looked up per request with `sensors.LoadProfile(profiles.FS, board.BoardName)` in three places.

## Goals / Non-Goals

**Goals:**
- The card set depends on the board: Hardware exists only where a profile lists sensors, and the layout parser knows it.
- One sensor read per five seconds for the whole router, shared by the page render and every stream connection.
- The card composes existing widgets and the existing live-row mechanism: no new widget, no new cell kind, no new client concept.

**Non-Goals:**
- Sharing the read with the history collector's `temperatures()` source. It keeps its own cadence.
- Where profiles come from. `board-profiles-runtime` changes the source. This change reads the profile through the one loaded value it introduces.
- Fibre modules and unprofiled sensors on the card. They stay on the Hardware page.

## Decisions

### 1. The Server resolves the board's profile once, and the card set follows it

`board_name` does not change while the router runs, so the Server loads the profile once at construction (`boardProfile`). It has a Hardware card when the profile lists at least one thermal, power or fan entry.

The layout always knows `hardware`: `overviewDefault` gains it at the end of the right column, and `overviewMerged` keeps only `firmware → system`. On a board without the card, `cookieLayout{hardware: false}` returns the parsed layout `Without("hardware")`, which takes the card out of its column, the closed cards and the shelf, leaving every other card in place. Because a card the cookie doesn't name is appended at a column's *end*, removing it afterwards moves nothing else, so this equals parsing against a smaller card set. `LiveStatus` sends a header only for the cards the layout holds.

*Alternative:* render the Hardware card everywhere and say "no profile" in its body. Rejected: the proposal keeps the card off boards that have nothing curated to show, and an empty card the operator has to hide by hand is noise.

*Alternative:* pass the card set into `DefaultOverviewLayout` and `ParseOverviewLayout`. Rejected: it changes two signatures and every test that calls them, while one `Without` at the layout source reaches the same result.

### 2. Profile order comes from the resolver, not from re-sorting at the edge

Each resolved temperature, power and fan reading carries `Entry`, one more than its index in its profile array, or `0` when no entry matched (so the zero value means unprofiled). A new accessor, `inv.Profiled()`, returns the three kinds, each holding only entries with `Entry > 0`, ordered by `Entry`. The `cpu` entry is included in Thermal at its place.

*Alternative:* match the profile again in the server. Rejected: matching is the resolver's job, and doing it twice risks the card and the Hardware page disagreeing on which reading an entry names. `Entry` is one fact, set where the match happens.

### 3. One shared sampler with a five-second freshness

A `sensorSampler` on the Server holds the last `Inventory` and when it was read. `Read()` returns the cached inventory if it is under five seconds old. Otherwise it calls `ResolveAll` under a mutex, so concurrent callers wait for one read rather than starting their own. The page render and every stream connection read through it.

*Alternative:* each stream connection reads on every fifth tick. Rejected: N browsers make N reads, and most entries are i2c transactions behind the PCA9545. The mutex-guarded cache bounds the bus to one read per five seconds, whatever is open.

*Alternative:* a background goroutine sampling every five seconds. Rejected: it reads while no one is looking. The on-demand cache reads only while a Hardware card is shown somewhere.

### 4. The card is fact lists built from a view the server formats

The server turns the sampler's inventory into `Overview.Hardware`: three sections, each a list of `{Key, Name, Value, Level}`, where `Value` is already formatted ("52 °C", "9.4 W", "4 210 rpm", "—"). It uses the Hardware page's formatters (`celsiusWhole`, `watts`, `rpmGroup`), so the two pages print a reading the same way. `hardwareBody` renders each section as a `Properties` with a `Head` naming the section, the same way the Internet card renders IPv4 and IPv6. Each row's `Key` is `hw-<kind>-<entry>`.

This mirrors how `SysMetrics` reaches the Resources card: the widget package renders, and the server formats.

### 5. The stream sends a `sensors` frame keyed by row

While `layout.Shows("hardware")`, each tick asks the sampler for the inventory (a cached read for up to five seconds), updates `liveOverview.Hardware`, and writes `sensors` as `{"rows": {"hw-thermal-1": "52 °C", …}}`. The `overview` frame that follows carries the Hardware head. The client writes each row's text in place and skips rows whose text hasn't changed. No reading is taken while Hardware is hidden. Sending the cached rows every second, rather than only after a fresh read, costs a few hundred bytes on a LAN, and the client's text check keeps the browser's work flat.

### 6. The header ranks temperatures before fans

`hardwareHead` walks Thermal for the worst `Level`: `critical` is danger and `warn` is warning, naming it "%s is hot". If no temperature is out of band, it looks for a fan whose `State` is `fault`: warning, "%s stopped". The summary is the `cpu` entry's value whenever it resolves, so the line never switches readings. A profile with no entry that answered reads "Unavailable", neutral. `issues()` is untouched: Hardware's readings never feed it.

### 7. The door is the shell's own page

The spec gets `Opens: "Hardware"` with a static `/system/hardware` href. The Hardware page is shell-owned and always mounted, so there is no registry lookup to fail.

## Risks / Trade-offs

- [A read that blocks a stream tick] `ResolveAll` reads every hwmon and zone, fibres included, not just the profile's entries. On a cache miss, one connection's tick waits for it. → A task measures `ResolveAll` on the DK. Above 100 ms, the sampler refreshes in a goroutine and serves the previous inventory meanwhile. Reading everything keeps grading identical to the Hardware page's.
- [Cards jump on upgrade] Existing cookies don't name `hardware`, so on a profiled board it arrives open at the end of the shorter column, which may be the left one. → That's the spec's existing rule for a card a cookie doesn't name. Reset layout puts it back in the default place.
- [The shelf is quiet about a hidden hot board] A hidden Hardware card reads nothing, so its shelf chip is neutral even when the board runs hot. → That's intended (spec: "Hardware hidden"). The verdict doesn't count it either, so nothing claims a status it didn't read.
- [Startup profile lookup] A profile loaded at construction won't notice a `board_name` that appears late. → `/tmp/sysinfo/board_name` is written before procd starts services, and the history collector already reads it the same way.

## Migration Plan

Nothing on the router changes: the cookie stays version 1, and no file or UCI option moves. Downgrading to a build that still maps `hardware → system` reads a cookie naming Hardware as System, which that build already handles.
