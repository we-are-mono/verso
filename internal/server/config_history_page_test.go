// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/we-are-mono/verso/internal/history"
	"github.com/we-are-mono/verso/internal/plugin"
	"github.com/we-are-mono/verso/internal/widget"
)

// turnedOff is Allow-Ping's target moved from ACCEPT to DROP.
func turnedOff(at time.Time) history.Change {
	return history.Change{Time: at, Status: history.ChangeApplied, User: "root", Address: "10.0.0.12", Items: []history.ChangeItem{{
		Page: "Firewall", Plugin: "firewall", Verb: "changed", Words: "Edited the rule “Allow-Ping”.",
		SubjectKind: "rule", SubjectName: "Allow-Ping",
		Ops: []history.ChangeOp{{Config: "firewall", Section: "cfg0792bd", Op: "set", Option: "target", Value: text("DROP"), Old: json.RawMessage(`"ACCEPT"`)}},
	}}}
}

// phraseOf is a phrase read back as its parts: [chip icon label], words and
// `values`.
func phraseOf(parts []widget.TablePart) []string {
	var out []string
	for _, p := range parts {
		switch {
		case p.Chip != nil:
			out = append(out, fmt.Sprintf("[%s %s]", p.Chip.Icon, p.Chip.Label))
		case p.Value != "":
			out = append(out, "`"+p.Value+"`")
		default:
			out = append(out, p.Words)
		}
	}
	return out
}

// TestAChangeOfOneOptionReadsAsTheEdit: the subject's chip, the option's,
// and the value it held and holds.
func TestAChangeOfOneOptionReadsAsTheEdit(t *testing.T) {
	parts, plain := changePhrase(turnedOff(time.Now()), identity)
	want := []string{"[shield Allow-Ping]", "[ target]", "`ACCEPT`", "→", "`DROP`"}
	if got := phraseOf(parts); !reflect.DeepEqual(got, want) {
		t.Fatalf("phrase = %q, want %q", got, want)
	}
	if plain != "Allow-Ping target ACCEPT → DROP" {
		t.Errorf("plain = %q", plain)
	}
}

// TestAnEditReadsWhatWasNotThere: an option that was not set, or is cleared,
// says so; an empty value is uci's ”; a list gains or loses one value.
func TestAnEditReadsWhatWasNotThere(t *testing.T) {
	edit := func(op history.ChangeOp) []string {
		c := history.Change{OldRead: true, Items: []history.ChangeItem{{Verb: "changed", SubjectKind: "network", SubjectName: "lan", Ops: []history.ChangeOp{op}}}}
		parts, _ := changePhrase(c, identity)
		return phraseOf(parts)
	}
	for _, tc := range []struct {
		op   history.ChangeOp
		want []string
	}{
		{history.ChangeOp{Op: "set", Option: "mtu", Value: text("1400")}, []string{"[network lan]", "[ mtu]", "not set", "→", "`1400`"}},
		{history.ChangeOp{Op: "remove-option", Option: "mtu", Old: json.RawMessage(`"1400"`)}, []string{"[network lan]", "[ mtu]", "`1400`", "→", "not set"}},
		{history.ChangeOp{Op: "set", Option: "dns", Value: text(""), Old: json.RawMessage(`"1.1.1.1"`)}, []string{"[network lan]", "[ dns]", "`1.1.1.1`", "→", "`''`"}},
		{history.ChangeOp{Op: "list-add", Option: "ports", Value: text("lan3")}, []string{"[network lan]", "[ ports]", "added", "`lan3`"}},
		{history.ChangeOp{Op: "list-del", Option: "ports", Value: text("lan1")}, []string{"[network lan]", "[ ports]", "removed", "`lan1`"}},
		{history.ChangeOp{Op: "set", Option: "password", Secret: true}, []string{"[network lan]", "[ password]", "changed"}},
	} {
		if got := edit(tc.op); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s %s = %q, want %q", tc.op.Op, tc.op.Option, got, tc.want)
		}
	}
}

// TestAChangeOfManyReadsAsItsFirstTwoSubjects: each subject once, with what
// happened to it, and how many more there are.
func TestAChangeOfManyReadsAsItsFirstTwoSubjects(t *testing.T) {
	item := func(verb, kind, name string) history.ChangeItem {
		return history.ChangeItem{Verb: verb, SubjectKind: kind, SubjectName: name,
			Ops: []history.ChangeOp{{Config: "c", Section: "s", Op: "set", Option: "o", Value: text("v")}}}
	}
	c := history.Change{Items: []history.ChangeItem{
		item("changed", "rule", "Allow-Ping"),
		item("added", "zone", "guest"),
		item("changed", "rule", "Allow-Ping"),
		item("added", "network", "guest"),
		item("removed", "route", "10.20.0.0/16"),
	}}
	parts, plain := changePhrase(c, identity)
	want := []string{"[shield Allow-Ping]", "changed,", "[zone guest]", "added", "and 2 more"}
	if got := phraseOf(parts); !reflect.DeepEqual(got, want) {
		t.Fatalf("phrase = %q, want %q", got, want)
	}
	if plain != "Allow-Ping changed, guest added and 2 more" {
		t.Errorf("plain = %q", plain)
	}

	// A subject that was added and then edited in the same apply was added.
	c.Items = []history.ChangeItem{item("added", "rule", "SSH"), item("changed", "rule", "SSH"), item("changed", "zone", "wan")}
	parts, _ = changePhrase(c, identity)
	if got := phraseOf(parts); !reflect.DeepEqual(got, []string{"[shield SSH]", "added,", "[zone wan]", "changed"}) {
		t.Fatalf("phrase = %q", got)
	}
}

// TestAnUndescribedChangeIsNamedByItsSection: no plugin named a subject, so
// the config and section stand for it.
func TestAnUndescribedChangeIsNamedByItsSection(t *testing.T) {
	c := history.Change{Items: []history.ChangeItem{{Verb: "changed", Verbatim: true, Words: "system: main.hostname = edge",
		Ops: []history.ChangeOp{{Config: "system", Section: "main", Op: "set", Option: "hostname", Value: text("edge"), Old: json.RawMessage(`"OpenWrt"`)}}}}}
	parts, _ := changePhrase(c, identity)
	want := []string{"[ system.main]", "[ hostname]", "`OpenWrt`", "→", "`edge`"}
	if got := phraseOf(parts); !reflect.DeepEqual(got, want) {
		t.Fatalf("phrase = %q, want %q", got, want)
	}
}

// TestADescribedChangeWithNoSubjectReadsAsItsSentence: a plugin that said
// what a change did but named no subject has still said it in words, so the
// entry and the Pending changes row read those words — never the anonymous section
// uci calls it by (firewall.cfg0792bd).
func TestADescribedChangeWithNoSubjectReadsAsItsSentence(t *testing.T) {
	items := []history.ChangeItem{{Page: "Firewall", Plugin: "firewall", Verb: "changed", Words: "Turned off the rule “Allow-Ping”.",
		Ops: []history.ChangeOp{{Config: "firewall", Section: "cfg0792bd", Op: "set", Option: "enabled", Value: text("0")}}}}
	parts, plain := changePhrase(history.Change{OldRead: true, Items: items}, identity)
	if got := phraseOf(parts); !reflect.DeepEqual(got, []string{"Turned off the rule “Allow-Ping”."}) {
		t.Fatalf("phrase = %q", got)
	}
	if plain != "Turned off the rule “Allow-Ping”." {
		t.Errorf("plain = %q", plain)
	}
	setting := []history.ChangeItem{{Page: "DNS", Plugin: "dnsdhcp", Verb: "changed", Words: "Log every query: on",
		Ops: []history.ChangeOp{{Config: "dhcp", Section: "cfg01411c", Op: "set", Option: "logqueries", Value: text("1")}}}}
	if _, got := phrase(history.Change{Items: setting}, identity, false); got != "Log every query: on" {
		t.Errorf("the Pending changes row's words = %q, want the setting's own words", got)
	}
}

// TestAnEditWhoseOldValueWasNeverReadClaimsNone: an entry kept before
// config history read what an option held does not know it, so it says only
// what the option became — never "not set → not set".
func TestAnEditWhoseOldValueWasNeverReadClaimsNone(t *testing.T) {
	c := history.Change{Items: []history.ChangeItem{{Verb: "changed", SubjectKind: "rule", SubjectName: "Allow-Ping",
		Ops: []history.ChangeOp{{Config: "firewall", Section: "cfg0792bd", Op: "remove-option", Option: "enabled"}}}}}
	parts, _ := changePhrase(c, identity)
	if got, want := phraseOf(parts), []string{"[shield Allow-Ping]", "[ enabled]", "→", "not set"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("phrase = %q, want %q", got, want)
	}
}

// changesServer is a signed-in server keeping history.db in a temporary
// directory.
func changesServer(t *testing.T) (*Server, *history.Store) {
	t.Helper()
	s := newServer(t, fakeBackend{access: true})
	store, err := history.Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	s.historyStore = store
	return s, store
}

func record(t *testing.T, store *history.Store, c history.Change) int64 {
	t.Helper()
	id, err := store.RecordChange(t.Context(), c)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// TestChangesPageListsChangesByDayNewestFirst: lanes by day, each row named
// from what it touched, and one no longer in force marked over.
func TestChangesPageListsChangesByDayNewestFirst(t *testing.T) {
	s, store := changesServer(t)
	now := time.Now()
	earlier := turnedOff(now.AddDate(0, 0, -3))
	earlier.Status = history.ChangeRolledBack
	old := record(t, store, earlier)
	recent := record(t, store, turnedOff(now))

	status, body := getSignedIn(t, s, changesPath)
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	today, before := strings.Index(body, ">Today<"), strings.Index(body, ">"+now.AddDate(0, 0, -3).Format("Mon 2 Jan")+"<")
	if today < 0 || before < today {
		t.Fatalf("lanes out of place (%d, %d):\n%s", today, before, body)
	}
	newer, older := strings.Index(body, changesPath+"/"+fmt.Sprint(recent)), strings.Index(body, changesPath+"/"+fmt.Sprint(old))
	if newer < 0 || older < newer {
		t.Fatalf("rows out of order (%d, %d)", newer, older)
	}
	for _, want := range []string{">Allow-Ping</span>", ">target</span>", "Rolled back", "Applied"} {
		if !strings.Contains(body, want) {
			t.Errorf("page missing %q", want)
		}
	}
	if n := len(regexp.MustCompile(`data-verso-row-id="\d+" data-verso-off`).FindAllString(body, -1)); n != 1 {
		t.Errorf("%d rows marked over, want the rolled-back one", n)
	}
}

// TestChangesPageSaysWhenNothingChanged.
func TestChangesPageSaysWhenNothingChanged(t *testing.T) {
	s, _ := changesServer(t)
	if _, body := getSignedIn(t, s, changesPath); !strings.Contains(body, "Nothing has been applied yet.") {
		t.Errorf("empty page:\n%s", body)
	}
}

// TestChangesPageWithoutHistory.
func TestChangesPageWithoutHistory(t *testing.T) {
	s := newServer(t, fakeBackend{})
	if status, _ := getSignedIn(t, s, changesPath); status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
}

// TestChangesIsASystemPage: it sits beside Events.
func TestChangesIsASystemPage(t *testing.T) {
	s, _ := changesServer(t)
	_, body := getSignedIn(t, s, changesPath)
	events, changes := strings.Index(body, `href="`+eventsPath+`"`), strings.Index(body, `href="`+changesPath+`"`)
	if events < 0 || changes < events {
		t.Fatalf("Changes should follow Events in System's pages (%d, %d)", events, changes)
	}
}

// TestAChangeOpensOnWhatItChanged: its drawer holds what its row cannot, the
// note, then every uci change in lanes by page with its plugin's words, each
// with what it held and holds. Who, from where and how it stands are the
// row's, and not said twice.
func TestAChangeOpensOnWhatItChanged(t *testing.T) {
	s, store := changesServer(t)
	c := turnedOff(time.Now())
	c.Note = "Ping off for the audit"
	id := record(t, store, c)

	rec := getPanel(t, s, fmt.Sprintf("%s/%d", changesPath, id))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"Ping off for the audit", "Firewall", "Edited the rule “Allow-Ping”.",
		firewallTargetLine, ">ACCEPT<", ">DROP<"} {
		if !strings.Contains(body, want) {
			t.Errorf("drawer missing %q:\n%s", want, body)
		}
	}
	for _, twice := range []string{"10.0.0.12", ">Applied<", "data-verso-expand-row"} {
		if strings.Contains(body, twice) {
			t.Errorf("drawer should not carry %q:\n%s", twice, body)
		}
	}
}

// TestAChangeNeverRecordedHasNoDrawer.
func TestAChangeNeverRecordedHasNoDrawer(t *testing.T) {
	s, _ := changesServer(t)
	for _, path := range []string{changesPath + "/7", changesPath + "/x"} {
		if rec := getPanel(t, s, path); rec.Code != http.StatusNotFound {
			t.Errorf("%s = %d, want 404", path, rec.Code)
		}
	}
}

// TestEachUCIChangeIsARow: the option, what it held, what it holds and what
// happened to it; a removed section with each option it held; a secret with
// no value.
func TestEachUCIChangeIsARow(t *testing.T) {
	ops := []history.ChangeOp{
		{Config: "network", Section: "guest", Op: "add-section", Option: "interface"},
		{Config: "network", Section: "guest", Op: "set", Option: "proto", Value: text("static")},
		{Config: "network", Section: "lan", Op: "list-add", Option: "ports", Value: text("lan3")},
		{Config: "network", Section: "lan", Op: "list-del", Option: "ports", Value: text("lan1")},
		{Config: "network", Section: "lan", Op: "remove-option", Option: "mtu", Old: json.RawMessage(`"1400"`)},
		{Config: "network", Section: "wan6", Op: "remove-section", Old: json.RawMessage(`{".type":"interface","proto":"dhcpv6","dns":["1.1.1.1","9.9.9.9"]}`)},
		{Config: "network", Section: "wan", Op: "set", Option: "password", Secret: true},
		{Config: "system", Section: "main", Op: "set", Option: "hostname", Value: text(""), Old: json.RawMessage(`"OpenWrt"`)},
	}
	var got []string
	for _, op := range ops {
		for _, line := range opLines(op, identity) {
			got = append(got, plainOf(line))
		}
	}
	// Each reads as the listing reads an edit: the section's chip and the
	// option's, then what it held and holds.
	want := []string{
		"network.guest added interface",
		"network.guest proto not set → static",
		"network.lan ports added lan3",
		"network.lan ports removed lan1",
		"network.lan mtu 1400 → not set",
		"network.wan6 removed interface",
		"network.wan6 dns 1.1.1.1 9.9.9.9 → not set",
		"network.wan6 proto dhcpv6 → not set",
		"network.wan password changed",
		"system.main hostname OpenWrt → ''",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("rows\n%v\nwant\n%v", got, want)
	}
}

// TestTheDrawerReadsInItsWidth: changes a plugin left undescribed, one after
// another on one page, stand under one band; one it described keeps a band
// of its own with its words; and each change is one line, read as the
// listing reads an edit, in one column that wraps in the drawer's width.
func TestTheDrawerReadsInItsWidth(t *testing.T) {
	raw := func(option string) history.ChangeItem {
		return history.ChangeItem{Page: "DNS and DHCP", Plugin: "dnsdhcp", Verbatim: true, Ops: []history.ChangeOp{
			{Config: "dhcp", Section: "cfg01411c", Op: "remove-option", Option: option, Old: json.RawMessage(`"1"`)},
		}}
	}
	described := history.ChangeItem{Page: "DNS and DHCP", Plugin: "dnsdhcp", Words: "Turned on logging", Ops: []history.ChangeOp{
		{Config: "dhcp", Section: "cfg01411c", Op: "set", Option: "logqueries", Value: text("1")},
	}}
	table := changeOpsTable(history.Change{Items: []history.ChangeItem{raw("noresolv"), raw("doh_server"), described, raw("server")}}, identity)
	var bands []string
	for _, row := range table.Rows {
		if row.Group != nil {
			bands = append(bands, row.Group.Label+"|"+row.Group.Tally)
		}
	}
	want := []string{"DNS and DHCP|", "DNS and DHCP|Turned on logging", "DNS and DHCP|"}
	if !reflect.DeepEqual(bands, want) {
		t.Errorf("bands %v, want %v", bands, want)
	}
	// One line a change, in one column that wraps and needs no heading.
	if len(table.Columns) != 1 || table.Columns[0].Kind != "phrase" || !table.Columns[0].Wrap || table.Columns[0].Label != "" {
		t.Errorf("columns = %+v, want one unlabelled phrase column that wraps", table.Columns)
	}
	var lines []string
	for _, row := range table.Rows {
		lines = append(lines, plainOf(row.Cells[0].Parts))
	}
	if want := []string{"dhcp.cfg01411c noresolv 1 → not set", "dhcp.cfg01411c doh_server 1 → not set",
		"dhcp.cfg01411c logqueries not set → 1", "dhcp.cfg01411c server 1 → not set"}; !reflect.DeepEqual(lines, want) {
		t.Errorf("lines\n%v\nwant\n%v", lines, want)
	}
}

// TestAReturnReadsEachOptionFromNowToThen: a return's option reads from what
// it holds now to what it held then; a whole section it puts back or takes
// away reads as added or removed with its type, never as a value.
func TestAReturnReadsEachOptionFromNowToThen(t *testing.T) {
	for _, c := range []struct {
		row  planRow
		want string
	}{
		{planRow{Option: "firewall.cfg0792bd.target", Now: "DROP", Then: "ACCEPT"}, "firewall.cfg0792bd target DROP → ACCEPT"},
		{planRow{Option: "firewall.cfg0792bd.target", Now: none, Then: "ACCEPT"}, "firewall.cfg0792bd target not set → ACCEPT"},
		{planRow{Option: "https-dns-proxy.cfg032d79", Now: none, Then: "https-dns-proxy"}, "https-dns-proxy.cfg032d79 added https-dns-proxy"},
		{planRow{Option: "network.guest", Now: "interface", Then: none}, "network.guest removed interface"},
	} {
		if got := plainOf(returnLine(c.row, identity)); got != c.want {
			t.Errorf("%+v reads %q, want %q", c.row, got, c.want)
		}
	}
}

// chipsLine is how a drawer's line begins in the page: the section's chip,
// then the option's.
func chipsLine(section, option string) string {
	return `>` + section + `</span><span class="verso-chip" data-verso-verbatim>` + option + `<`
}

// firewallTargetLine is the line the fixtures' rule target change begins.
var firewallTargetLine = chipsLine("firewall.cfg0792bd", "target")

// plainOf is a line's parts as read text.
func plainOf(parts []widget.TablePart) string {
	words := make([]string, len(parts))
	for i, p := range parts {
		switch {
		case p.Chip != nil:
			words[i] = p.Chip.Label
		case p.Value != "":
			words[i] = p.Value
		default:
			words[i] = p.Words
		}
	}
	return strings.Join(words, " ")
}

// TestEverySubjectKindHasAChip: each kind a plugin can name draws with its
// own glyph.
func TestEverySubjectKindHasAChip(t *testing.T) {
	fallback := widget.Icon("no-such-glyph", "")
	for kind := range plugin.SubjectKinds {
		icon, ok := subjectIcons[kind]
		if !ok || (icon != "device" && widget.Icon(icon, "") == fallback) {
			t.Errorf("subject kind %q draws no glyph of its own (%q)", kind, icon)
		}
	}
}

// TestAChangeOpensFromItsIcon: a row opens on the info glyph at its
// trailing edge, never on a "Details" link.
func TestAChangeOpensFromItsIcon(t *testing.T) {
	s, store := changesServer(t)
	record(t, store, turnedOff(time.Now()))
	_, body := getSignedIn(t, s, changesPath)
	if !strings.Contains(body, string(widget.Icon("info", "size-4"))) {
		t.Errorf("the row should open from the info glyph:\n%s", body)
	}
	if strings.Contains(body, ">Details</a>") || strings.Contains(body, ">Details</button>") {
		t.Errorf("the row should not carry a Details link:\n%s", body)
	}
}

// TestAnAppliedEventOpensItsChange: Events' row for an apply opens the
// entry config history keeps for it.
func TestAnAppliedEventOpensItsChange(t *testing.T) {
	row := coreRow(history.Event{Kind: "changes.applied", Detail: `{"pages":["Firewall"],"change":42}`}, time.Now(), identity)
	if row.Panel != changesPath+"/42" {
		t.Errorf("panel = %q, want the change's drawer", row.Panel)
	}
	if row := coreRow(history.Event{Kind: "changes.applied", Detail: `{"pages":["Firewall"]}`}, time.Now(), identity); row.Panel != "" {
		t.Errorf("an apply history did not keep opens %q", row.Panel)
	}
}

// TestARelationIsANoteBesideTheChange: an undo says what it undid in the
// Notes column, on the row's one line, and the operator's note follows it.
func TestARelationIsANoteBesideTheChange(t *testing.T) {
	now := time.Now()
	first := turnedOff(now.Add(-time.Minute))
	first.ID = 1
	undo := turnedOff(now)
	undo.ID, undo.Relation, undo.Related, undo.Note = 2, "undo", 1, "Ping back on"
	table := changesTable([]history.Change{undo, first}, time.Time{}, now, "", identity, func(string) func(string) string { return identity })
	if table.Columns[2].Label != "Notes" || table.Columns[2].Kind != "comment" {
		t.Fatalf("third column = %+v, want Notes", table.Columns[2])
	}
	notes, changed := table.Rows[0].Cells[2], table.Rows[0].Cells[1]
	want := "undo of " + eventTime(first.Time, now) + " · Ping back on"
	if notes.Text != want || changed.Detail != "" {
		t.Errorf("notes %q, changed detail %q; want %q on the notes", notes.Text, changed.Detail, want)
	}
	if table.Rows[1].Cells[2].Text != "" {
		t.Errorf("a change with no relation or note has notes %q", table.Rows[1].Cells[2].Text)
	}
}

// TestTheRoutersOwnChangeNamesItselfAndItsCause: a change the router made
// itself reads "The router" in By, never edited directly; Notes read the
// relation, then the cause, then the note; a cause reads in the viewer's
// language from the catalog of the plugin that wrote it, the shell's own from
// the shell's, and one no catalog holds as written (ADR-021, ADR-012).
func TestTheRoutersOwnChangeNamesItselfAndItsCause(t *testing.T) {
	now := time.Now()
	first := turnedOff(now.Add(-2 * time.Minute))
	first.ID = 1
	handOver := turnedOff(now.Add(-time.Minute))
	handOver.ID, handOver.User, handOver.Address, handOver.Own = 2, "", "", true
	handOver.Cause, handOver.CausePlugin = "A certificate was installed by hand.", "acme"
	handOver.Relation, handOver.Related, handOver.Note = "undo", 1, "kept"
	theme := turnedOff(now)
	theme.ID, theme.Own, theme.Cause = 3, true, "The theme was changed."
	older := turnedOff(now)
	older.ID, older.User, older.Address, older.Own = 4, "", "", true
	unknown := turnedOff(now)
	unknown.ID, unknown.User, unknown.Address, unknown.Own, unknown.Cause, unknown.CausePlugin = 5, "", "", true, "Something else.", "acme"

	slovenian := map[string]string{"The theme was changed.": "Tema je bila spremenjena.", "The router": "Usmerjevalnik"}
	tr := func(s string) string {
		if t, ok := slovenian[s]; ok {
			return t
		}
		return s
	}
	pluginTr := func(id string) func(string) string {
		return func(s string) string {
			if id == "acme" && s == "A certificate was installed by hand." {
				return "Potrdilo je bilo nameščeno ročno."
			}
			return tr(s)
		}
	}
	table := changesTable([]history.Change{unknown, older, theme, handOver, first}, time.Time{}, now, "", tr, pluginTr)
	row := func(id string) widget.TableRow {
		for _, r := range table.Rows {
			if r.ID == id {
				return r
			}
		}
		t.Fatalf("no row %s", id)
		return widget.TableRow{}
	}
	if got := row("2").Cells[3].Text; got != "Usmerjevalnik" {
		t.Errorf("the router's By = %q", got)
	}
	want := "undo of " + eventTime(first.Time, now) + " · Potrdilo je bilo nameščeno ročno. · kept"
	if got := row("2").Cells[2].Text; got != want {
		t.Errorf("notes %q, want %q", got, want)
	}
	if got := row("3").Cells[2].Text; got != "Tema je bila spremenjena." {
		t.Errorf("the shell's cause reads %q", got)
	}
	if got := row("3").Cells[3].Text; got != theme.User {
		t.Errorf("an operator's act names %q in By, want %q", got, theme.User)
	}
	if by, notes := row("4").Cells[3].Text, row("4").Cells[2].Text; by != "Usmerjevalnik" || notes != "" {
		t.Errorf("an older plugin's commit: By %q, notes %q", by, notes)
	}
	if got := row("5").Cells[2].Text; got != "Something else." {
		t.Errorf("a cause no catalog holds reads %q", got)
	}
	if got := row("1").Cells[3].Text; got == "Usmerjevalnik" || got == "Edited directly" {
		t.Errorf("an apply's By = %q", got)
	}
}

// TestACausesArgumentsFillItsSentence: the sentence is translated, then its
// %s filled with the arguments as written; a sentence whose %s count differs
// from its arguments reads as written, in whichever language fills.
func TestACausesArgumentsFillItsSentence(t *testing.T) {
	slovenian := map[string]string{
		"Changed by %s as it was installed.": "Spremenil %s ob namestitvi.",
		"Changed by %s as it was removed.":   "Spremenjeno ob odstranitvi.",
	}
	tr := func(s string) string {
		if t, ok := slovenian[s]; ok {
			return t
		}
		return s
	}
	noPlugins := func(string) func(string) string { return tr }
	for _, c := range []struct {
		cause string
		args  []string
		want  string
	}{
		{"Changed by %s as it was installed.", []string{"https-dns-proxy"}, "Spremenil https-dns-proxy ob namestitvi."},
		// The translation lost its %s: the English sentence fills instead.
		{"Changed by %s as it was removed.", []string{"adblock"}, "Changed by adblock as it was removed."},
		{"Changed by %s as it was upgraded.", nil, "Changed by %s as it was upgraded."},
		{"Changed as packages were upgraded.", []string{"x"}, "Changed as packages were upgraded."},
		// With nothing to fill, a translation reads as it is, a % in it too.
		{"Changed by 100% of it.", nil, "Changed by 100% of it."},
	} {
		got := changeCause(history.Change{Own: true, Cause: c.cause, CauseArgs: c.args}, tr, noPlugins)
		if got != c.want {
			t.Errorf("%q with %v reads %q, want %q", c.cause, c.args, got, c.want)
		}
	}
}

// TestTheDrawerSaysWhatCausedTheChange: the drawer holds the cause the row's
// one line may cut short, before the note.
func TestTheDrawerSaysWhatCausedTheChange(t *testing.T) {
	s, store := changesServer(t)
	c := turnedOff(time.Now())
	c.User, c.Address, c.Own, c.Cause, c.Note = "", "", true, "The theme was changed.", "a note"
	id := record(t, store, c)
	body := getPanel(t, s, fmt.Sprintf("%s/%d", changesPath, id)).Body.String()
	cause, note := strings.Index(body, "The theme was changed."), strings.Index(body, "a note")
	if cause < 0 || note < 0 || cause > note {
		t.Errorf("the drawer should hold the cause before the note:\n%s", body)
	}
}

// TestTheCauseAndTheNoteCaptionTheLines: what caused a change and the
// operator's note are the caption under its lines, one quiet line in the
// listing's order, never paragraphs of their own above them.
func TestTheCauseAndTheNoteCaptionTheLines(t *testing.T) {
	c := turnedOff(time.Now())
	c.ID, c.OldRead, c.Note = 9, true, "kept for the audit"
	children := whatChangedTab(c, []history.Change{c}, changesPath+"/9", time.Now(), false, "The theme was changed.", identity)
	var table *widget.Table
	for _, child := range children {
		switch w := child.(type) {
		case *widget.Text:
			t.Errorf("a paragraph stands above the lines: %q", w.Markdown)
		case *widget.Table:
			table = w
		}
	}
	if table == nil || table.Note != "The theme was changed. · kept for the audit" {
		t.Fatalf("caption = %+v", table)
	}
	since := returnBody(actPlan{Rows: []planRow{{Option: "firewall.cfg0792bd.target", Now: "DROP", Then: "ACCEPT"}}}, changesPath+"/9", "Return to this point", "Each option, from what it holds now to what it held at this point.", false, identity)
	for _, child := range since {
		if w, ok := child.(*widget.Table); ok && w.Note != "Each option, from what it holds now to what it held at this point." {
			t.Errorf("the return's caption = %q", w.Note)
		}
		if w, ok := child.(*widget.Text); ok {
			t.Errorf("a paragraph stands above the return's lines: %q", w.Markdown)
		}
	}
}

// TestANoteReadsAsItWasWritten: the drawer holds the note itself, with no
// label, and nothing in it is read as formatting.
func TestANoteReadsAsItWasWritten(t *testing.T) {
	s, store := changesServer(t)
	c := turnedOff(time.Now())
	c.Note = "*not bold* #412 <b>x</b>"
	id := record(t, store, c)
	body := getPanel(t, s, fmt.Sprintf("%s/%d", changesPath, id)).Body.String()
	if !strings.Contains(body, "*not bold* #412 &lt;b&gt;x&lt;/b&gt;") {
		t.Errorf("the note should read literally:\n%s", body)
	}
	if strings.Contains(body, ">Note<") {
		t.Errorf("the note should stand without a label:\n%s", body)
	}
}
