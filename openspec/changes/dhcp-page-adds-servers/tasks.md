# Tasks

## 1. Shell: a drawer-only row

- [x] 1.1 Write a shell test in `internal/widget/table_test.go`: a listing with two rows plus one cell-less row carrying an open `RowDrawer` renders two `<tr>` rows and the drawer, with no empty row and no lane band. Make it pass (in `internal/widget/table.go` / `table.html.tmpl` if the shell lacks the rule), keep the SDK Files editor's drawer-only answer test green, and verify with `make lint` and `make test`
- [ ] 1.2 Write a shell render test showing that a page card's section `control` holding a `Link` with `panel: true` stands on the card header and opens its panel. Make it pass, document the drawer-only row and the header add in DESIGN.md's table and card entries, and verify with `make lint` and `make test`

## 2. DHCP page: which servers are listed

- [ ] 2.1 Write tests in `plugins/verso-plugin-dnsdhcp/src/dhcp.rs`: a static network with no section is not listed; the stock `wan` section (`proto dhcp`, `ignore 1`) is not listed; `lan` with `ignore 1` is listed with `muted: true`. Make them pass by filtering the page's list (not `verso_plugin::dhcp::servers`), and verify with `make test` in the plugin repo
- [ ] 2.2 Update the empty text of the Servers table for a router with no servers, pointing to "Add DHCP server" or to Interfaces when no network can take one. Cover it with a test, and verify with `make lint` and `make test` in the plugin repo

## 3. DHCP page: adding a server

- [ ] 3.1 Write tests: the page's act is "Add DHCP server" opening `?open=new-server` while a free static network exists, and there is no act when none exists. Make them pass with a `new_server` heading act and a `NEW_SERVER` key in `page.rs`, and verify with `make test` in the plugin repo
- [ ] 3.2 Write tests: `?open=new-server` opens "New DHCP server" with a `network` select listing only the free static networks; `?open=guest` for a free network opens it with `guest` chosen; `?open=nosuch` opens nothing. Make them pass by giving `server_drawer` an optional network, and verify with `make test` in the plugin repo
- [ ] 3.3 Write tests: saving the blank drawer with `guest` stages `config dhcp 'guest'` with `option interface 'guest'` and the default pool, and a submission naming `lan` (already served) stays in the drawer with a refusal on `network`. Make them pass through `save_drawer`, reusing `validate()` and `write()`, and verify with `make test` in the plugin repo

## 4. DHCP page: removing a server

- [ ] 4.1 Write tests: each server row carries "Remove DHCP server" under `_remove_server` with the confirm words from design.md; posting it stages `commit_delete(dhcp, <section>)`; posting a section that is gone answers with the "isn't in the config" notice. Make them pass, and verify with `make test` in the plugin repo

## 5. DHCP page: adding a reservation from its card

- [ ] 5.1 Write tests: the Reservations section's `control` is a `Link` "Add reservation" with the plus glyph, `?open=new` and `panel: true`; the page's act is no longer "Add reservation". Update `new_reservation_is_the_pages_act_and_carries_the_blank_one` to match. Make them pass, and verify with `make test` in the plugin repo
- [ ] 5.2 Write tests: `?open=new` (and `?open=new&reserve=<mac>`) puts the blank reservation drawer on a drawer-only row of the Reservations table, including when the page has no heading act; saving it still stages the reservation. Make them pass, and verify with `make test` in the plugin repo

## 6. Words, docs and packaging

- [ ] 6.1 Add every new string ("Add DHCP server", "New DHCP server", "Network", "Remove DHCP server", the confirm, "Keep DHCP server", the empty text) to `i18n/sl.json`, drop the strings no longer used, and verify with the plugin's catalog test and `make lint`
- [ ] 6.2 Update `docs/plugins.md` (the DHCP page's `?open=` keys, `new-server`, and how a server is added and removed) and `docs/BACKLOG.md`, and verify that the documented addresses match the tests in 3.2 and 5.2
- [ ] 6.3 If task 1.1 changed the shell, raise the plugin's apk dependency on verso to that release and the `PLUGIN_FLOORS` entry in the shell's Makefile. Verify with `make test` in both repos and an apk build of the plugin

## 7. Integration

- [ ] 7.1 With `make dev` running, add a server on a fresh static network, switch it off, remove it, add a reservation from the card and one from a lease, and check that each stages as specified and that `/plugins/dnsdhcp/?open=lan` from the Interfaces page still opens lan's drawer. Save screenshots to ~/Share/screenshots

## Workflow follow-up

- Archive the change once the user has reviewed it in the browser, confirming docs/BACKLOG.md and DESIGN.md reflect what landed.
- Next change: Interfaces' expanding rows become an info drawer.
