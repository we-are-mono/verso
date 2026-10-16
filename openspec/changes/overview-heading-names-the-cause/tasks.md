# Tasks

## 1. Each card says its cause

- [ ] 1.1 Test first in `internal/widget/overview_test.go`: a card head out of band carries a stand-alone cause (Ports "1 port without link", Tunnels "1 of 2 tunnels down", Resources "Storage is high", Hardware "cluster is hot"); in band it carries none. Then `cardHead.Cause`, set in each `*Head`. Verify: `go test ./internal/widget/`.

## 2. The heading names the cause

- [ ] 2.1 Test first in `internal/widget/overview_test.go`: offline beats every cause; one cause is the heading in its tone; two are "2 things need attention", danger when one is; a hidden card's cause counts; Hardware counts; nothing wrong reads online. Then `causes()` and `verdict()`. Verify: `go test ./internal/widget/`.
- [ ] 2.2 The cause sentences and the count in `i18n/sl/base.json`. Verify: `make i18n-audit` shows no new misses.

## 3. Hidden Hardware still reads

- [ ] 3.1 Test first in `internal/server`: with Hardware hidden, the page's verdict names a hot board and the stream sends no sensor frame. Then read the sensors in `server.go` and the stream's sampler whenever the board has them, and send the frame only while Hardware is shown. Verify: `go test ./internal/server/`.

## 4. Docs

- [ ] 4.1 ADR-022's verdict paragraph says the heading names the cause and every card's problem counts, Hardware included. Verify: it matches the spec.
