// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"strings"
	"testing"

	"github.com/we-are-mono/verso/internal/openwrt"
)

// The lines below are the shapes the kernel really writes. nf_log_syslog emits
// `<level><prefix>IN=%s OUT=%s ` and then the family's own fields; logd strips
// the `<level>` and keeps the rest, kernel uptime stamp included where the
// kernel was built with CONFIG_PRINTK_TIME. fw4 supplies the prefix: a rule's
// is `"<name>: "`, a zone policy's is `"<verdict> <zone> <direction>: "`.
const (
	fwLineRuleDrop  = `[ 1730.874112] Log-WAN-probes: IN=wan0 OUT= MAC=02:42:ac:1e:01:ab:02:42:ac:1e:01:02:08:00 SRC=203.0.113.9 DST=192.0.2.10 LEN=60 TOS=0x00 PREC=0x00 TTL=52 ID=54321 DF PROTO=TCP SPT=51422 DPT=445 WINDOW=64240 RES=0x00 SYN URGP=0`
	fwLineForward   = `[ 1731.004000] Minecraft server: IN=wan0 OUT=br-lan MAC=02:42:ac:1e:01:ab SRC=198.51.100.7 DST=10.0.0.30 LEN=52 TOS=0x00 PREC=0x00 TTL=51 ID=0 DF PROTO=TCP SPT=61000 DPT=25565 WINDOW=65535 RES=0x00 SYN URGP=0`
	fwLineZonePol   = `[ 1732.118004] reject wan in: IN=wan0 OUT= MAC=02:42:ac:1e:01:ab SRC=45.148.10.77 DST=192.0.2.10 LEN=44 TOS=0x00 PREC=0x00 TTL=44 ID=1234 PROTO=TCP SPT=40000 DPT=22 WINDOW=1024 RES=0x00 SYN URGP=0`
	fwLineICMP      = `[ 1733.220001] Allow-Ping: IN=br-lan OUT= MAC=02:42:ac:1e:01:ab SRC=10.0.0.41 DST=10.0.0.1 LEN=84 TOS=0x00 PREC=0x00 TTL=64 ID=9999 DF PROTO=ICMP TYPE=8 CODE=0 ID=4242 SEQ=1`
	fwLineIPv6      = `[ 1734.330002] Log-WAN-probes: IN=wan0 OUT= MAC=33:33:00:00:00:01 SRC=2001:0db8:0000:0000:0000:0000:0000:0009 DST=2001:0db8:0000:0000:0000:0000:0000:0001 LEN=64 TC=0 HOPLIMIT=58 FLOWLBL=0 PROTO=TCP SPT=39000 DPT=8443 WINDOW=64800 RES=0x00 SYN URGP=0`
	fwLineNoStamp   = `Log-WAN-probes: IN=wan0 OUT= SRC=203.0.113.9 DST=192.0.2.10 PROTO=UDP SPT=5060 DPT=5060 LEN=20`
	fwLineNoProto   = `[ 1735.000003] Log-WAN-probes: IN=wan0 OUT= SRC=203.0.113.9 DST=192.0.2.10 LEN=40`
	fwLineNumProto  = `[ 1736.000004] Log-WAN-probes: IN=wan0 OUT= SRC=203.0.113.9 DST=192.0.2.10 LEN=40 PROTO=47`
	fwLineOutbound  = `[ 1737.000005] Log-egress: IN= OUT=wan0 SRC=192.0.2.10 DST=8.8.8.8 LEN=60 PROTO=UDP SPT=33333 DPT=53 LEN=8`
	fwLineNotOurs   = `verso[147852]: 2026/09/01 20:20:54 verso listening on :8080`
	fwLineTruncated = `[ 1738.000006] Log-WAN-probes: IN=wan0 OUT= SRC=203.0.113.9 DST=`
)

// TestParseFWLogLineReadsTheRealShapes: the prefix is everything between the
// kernel's stamp and the interface pair, and every key=value below it is a
// field. A line that is not a netfilter verdict is not one.
func TestParseFWLogLineReadsTheRealShapes(t *testing.T) {
	line, ok := parseFWLogLine(fwLineRuleDrop)
	if !ok {
		t.Fatal("a rule's drop is a netfilter line")
	}
	if line.Prefix != "Log-WAN-probes" {
		t.Errorf("prefix = %q, want the rule's name", line.Prefix)
	}
	for key, want := range map[string]string{
		"IN": "wan0", "OUT": "", "SRC": "203.0.113.9", "DST": "192.0.2.10",
		"PROTO": "TCP", "SPT": "51422", "DPT": "445",
	} {
		if got := line.Fields[key]; got != want {
			t.Errorf("field %s = %q, want %q", key, got, want)
		}
	}
	// A bare flag is the packet's shape, not a field.
	if _, ok := line.Fields["DF"]; ok {
		t.Error("a bare flag must not become a field")
	}
	// ICMP's own ID must not overwrite the IP header's, which came first.
	icmp, _ := parseFWLogLine(fwLineICMP)
	if icmp.Fields["ID"] != "9999" {
		t.Errorf("repeated key = %q, want the first the kernel wrote", icmp.Fields["ID"])
	}
	if icmp.Fields["TYPE"] != "8" || icmp.Fields["DPT"] != "" {
		t.Errorf("icmp fields wrong: %+v", icmp.Fields)
	}
	// A kernel built without CONFIG_PRINTK_TIME writes no stamp.
	plain, ok := parseFWLogLine(fwLineNoStamp)
	if !ok || plain.Prefix != "Log-WAN-probes" || plain.Fields["DPT"] != "5060" {
		t.Errorf("unstamped line not parsed: %+v", plain)
	}
	if _, ok := parseFWLogLine(fwLineNotOurs); ok {
		t.Error("an ordinary syslog line is not a firewall verdict")
	}
	// fw4 appends exactly one ": " — so exactly one colon is punctuation. A rule
	// a person named "Ports::web" keeps every colon they typed, and so can be
	// found in the config.
	colons, ok := parseFWLogLine(`[ 1.0] Ports::web:: IN=wan0 OUT= SRC=1.2.3.4 DST=5.6.7.8`)
	if !ok || colons.Prefix != "Ports::web:" {
		t.Errorf("prefix = %q, want the name with only fw4's own colon removed", colons.Prefix)
	}
}

// TestParseFWLogLineSurvivesHostileContent: a rule's name is whatever a person
// typed and a log line is untrusted input. Nothing here may panic, and a name
// that imitates the kernel's own fields may at worst cost its own row's rule.
func TestParseFWLogLineSurvivesHostileContent(t *testing.T) {
	for _, msg := range []string{
		"",
		"IN=",
		"IN= OUT=",
		`[ 1.000000] <script>alert(1)</script>: IN=wan0 OUT= SRC=1.2.3.4`,
		`[ 1.000000] a "quoted: rule" name: IN=wan0 OUT= SRC=1.2.3.4 DST=5.6.7.8`,
		`[ 1.000000] rule=with=equals: IN=wan0 OUT= SRC=1.2.3.4`,
		`[ 1.000000] ` + strings.Repeat("x", 4000) + `: IN=wan0 OUT= SRC=1.2.3.4`,
		fwLineTruncated,
		"=====",
		"IN=a OUT=b =novalue = ==",
	} {
		line, ok := parseFWLogLine(msg)
		if !ok {
			continue
		}
		// Whatever came back is data: it is never interpreted, only carried.
		_ = line.Prefix
		_ = line.Fields["SRC"]
	}
	// A prefix carrying the kernel's own signature cuts the prefix short but
	// still yields a row — the rule simply does not resolve.
	line, ok := parseFWLogLine(`[ 1.000000] IN=fake OUT=fake pretend: IN=wan0 OUT= SRC=1.2.3.4`)
	if !ok {
		t.Fatal("a line with a hostile prefix is still a line")
	}
	if line.Prefix != "" {
		t.Logf("hostile prefix resolved to %q — a rule that cannot match", line.Prefix)
	}
}

// fwTestConfig is a firewall config in the shape rpcd returns it: section id →
// its options, with the section type under ".type" and uci's own meta beside it.
// `.anonymous` and `.index` are part of that shape and are what fw4 names a
// section by, so a fixture without them would be testing a config rpcd never
// returns.
func fwTestConfig() map[string]any {
	return map[string]any{
		"cfg01dc81": map[string]any{".type": "zone", ".anonymous": true, ".index": int64(0), "name": "lan", "network": []any{"lan"}, "input": "ACCEPT"},
		"cfg02dc81": map[string]any{".type": "zone", ".anonymous": true, ".index": int64(1), "name": "wan", "network": []any{"wan", "wan6"}, "masq": "1"},
		"cfg03dc81": map[string]any{".type": "zone", ".anonymous": true, ".index": int64(2), "name": "mgmt", "device": "eth0"},
		"cfg11dc81": map[string]any{".type": "rule", ".anonymous": true, ".index": int64(3), "name": "Log-WAN-probes", "src": "wan", "target": "DROP", "log": "1"},
		"cfg12dc81": map[string]any{".type": "rule", ".anonymous": true, ".index": int64(4), "name": "Allow-Ping", "src": "lan", "target": "ACCEPT", "log": "1"},
		"cfg13dc81": map[string]any{".type": "rule", ".anonymous": true, ".index": int64(5), "src": "wan", "target": "REJECT", "log": "1"},
		"cfg14dc81": map[string]any{".type": "rule", ".anonymous": true, ".index": int64(6), "name": "Mark-VoIP", "src": "lan", "target": "MARK", "log": "1"},
		// A named section: `config rule 'block_telnet'` carries its name as its
		// section id, which is what fw4 calls it when no name option overrides.
		"block_telnet": map[string]any{".type": "rule", ".anonymous": false, ".index": int64(7), "src": "wan", "target": "DROP", "log": "1"},
		// Anonymous again, after a named one — fw4's counter walks every section
		// of the type, so this is @rule[5] and not @rule[4].
		"cfg15dc81": map[string]any{".type": "rule", ".anonymous": true, ".index": int64(8), "src": "lan", "target": "ACCEPT", "log": "1"},
		"cfg21dc81": map[string]any{".type": "redirect", ".anonymous": true, ".index": int64(9), "name": "Minecraft server", "target": "DNAT", "log": "1"},
		"cfg22dc81": map[string]any{".type": "redirect", ".anonymous": true, ".index": int64(10), "target": "DNAT", "log": "1"},
		"cfg31dc81": map[string]any{".type": "nat", ".anonymous": true, ".index": int64(11), "src": "wan", "target": "SNAT", "log": "1"},
	}
}

func fwTestIfaces() []openwrt.NetIface {
	return []openwrt.NetIface{
		{Name: "lan", Device: "br-lan", Up: true},
		{Name: "wan", Device: "wan0", Up: true},
		{Name: "wan6", Device: "wan0", Up: true},
		{Name: "mgmt", Device: "eth0"},
	}
}

// TestFWResolverNamesTheDecision: a prefix is a rule's name or a zone policy's
// sentence, and the two resolve differently — one opens its editor, the other
// has nothing to open because nobody wrote it.
func TestFWResolverNamesTheDecision(t *testing.T) {
	r := newFWResolver(fwTestConfig(), fwTestIfaces())

	line, _ := parseFWLogLine(fwLineRuleDrop)
	e := r.event(4211, 1788294054, line)
	if e.Verdict != "drop" || e.Rule != "Log-WAN-probes" {
		t.Errorf("rule verdict wrong: %+v", e)
	}
	if e.RuleHref != "/plugins/firewall/rules/cfg11dc81" {
		t.Errorf("rule href = %q, want its editor", e.RuleHref)
	}
	if e.From != "wan" {
		t.Errorf("from = %q, want the zone that owns wan0", e.From)
	}
	if e.ToKind != "router" || e.To != "router" {
		t.Errorf("a packet with no departure device was for this device: %+v", e)
	}
	if e.Src != "203.0.113.9" || e.Proto != "tcp" || e.Port != "445" {
		t.Errorf("packet facts wrong: %+v", e)
	}
	if e.ID != 4211 || e.At != 1788294054 {
		t.Errorf("cursor/stamp not carried: %+v", e)
	}

	fwd, _ := parseFWLogLine(fwLineForward)
	if e := r.event(1, 1, fwd); e.Verdict != "accept" || e.ToKind != "device" || e.To != "10.0.0.30" ||
		e.RuleHref != "/plugins/firewall/port-forwards/cfg21dc81" {
		t.Errorf("a logged port forward is a permission granted to a device: %+v", e)
	}

	pol, _ := parseFWLogLine(fwLineZonePol)
	if e := r.event(2, 2, pol); e.Verdict != "reject" || e.Rule != "" || e.RuleHref != "" || e.From != "wan" {
		t.Errorf("a zone policy has no rule to open: %+v", e)
	}

	out, _ := parseFWLogLine(fwLineOutbound)
	if e := r.event(3, 3, out); e.From != "router" || e.ToKind != "device" || e.To != "8.8.8.8" {
		t.Errorf("a packet this device sent comes from the router: %+v", e)
	}
}

// TestFWResolverNamesAnonymousSectionsThePositionalWayFW4Does pins the naming
// rule the whole activity page rests on. fw4's section_id gives an anonymous
// section `@<type>[N]` — N counting the sections of that type in config order,
// named ones included — and that string, not the rpcd config id, is what the
// kernel writes in front of the packet. A section written with a name of its own
// (`config rule 'block_telnet'`) keeps that name.
func TestFWResolverNamesAnonymousSectionsThePositionalWayFW4Does(t *testing.T) {
	r := newFWResolver(fwTestConfig(), fwTestIfaces())

	positional, _ := parseFWLogLine(`[ 1.0] @rule[2]: IN=wan0 OUT= SRC=1.2.3.4 DST=5.6.7.8 PROTO=TCP DPT=23`)
	e := r.event(1, 1, positional)
	if e.Rule != "@rule[2]" || e.Verdict != "reject" {
		t.Errorf("an anonymous rule logs under its positional name: %+v", e)
	}
	if e.RuleHref != "/plugins/firewall/rules/cfg13dc81" {
		t.Errorf("href = %q, want the editor of the section that sits third", e.RuleHref)
	}

	// The config id is not a name fw4 ever writes, so it resolves to nothing.
	byID, _ := parseFWLogLine(`[ 1.0] cfg13dc81: IN=wan0 OUT= SRC=1.2.3.4 DST=5.6.7.8`)
	if e := r.event(1, 1, byID); e.Rule != "" || e.Verdict != "" {
		t.Errorf("rpcd's config id is not a log prefix: %+v", e)
	}

	// The counter walks every section of the type, so the anonymous rule after a
	// named one is @rule[5] — not @rule[4].
	after, _ := parseFWLogLine(`[ 1.0] @rule[5]: IN=br-lan OUT= SRC=10.0.0.5 DST=10.0.0.1`)
	if e := r.event(1, 1, after); e.Verdict != "accept" || e.RuleHref != "/plugins/firewall/rules/cfg15dc81" {
		t.Errorf("a named section still advances fw4's counter: %+v", e)
	}

	// A section written with a name of its own logs under it.
	named, _ := parseFWLogLine(`[ 1.0] block_telnet: IN=wan0 OUT= SRC=1.2.3.4 DST=5.6.7.8 PROTO=TCP DPT=23`)
	if e := r.event(1, 1, named); e.Rule != "block_telnet" || e.Verdict != "drop" ||
		e.RuleHref != "/plugins/firewall/rules/block_telnet" {
		t.Errorf("a named section logs under its name: %+v", e)
	}

	// The name option, where a section carries one, is what fw4 uses instead.
	overridden, _ := parseFWLogLine(`[ 1.0] @rule[0]: IN=wan0 OUT= SRC=1.2.3.4 DST=5.6.7.8`)
	if e := r.event(1, 1, overridden); e.Rule != "" {
		t.Errorf("a section with a name option never logs positionally: %+v", e)
	}

	// Redirects are counted in their own type's sequence.
	redirect, _ := parseFWLogLine(`[ 1.0] @redirect[1]: IN=wan0 OUT=br-lan SRC=1.2.3.4 DST=10.0.0.30`)
	if e := r.event(1, 1, redirect); e.Verdict != "accept" ||
		e.RuleHref != "/plugins/firewall/port-forwards/cfg22dc81" {
		t.Errorf("an anonymous redirect logs as @redirect[N]: %+v", e)
	}
}

// TestFWResolverReadsSNATSections: `config nat` logs with the same
// `<name>: ` prefix every other section does, so a masquerade or SNAT rule that
// logs is a row like any other. Nothing in this shell renders an editor for one,
// so the row states the decision and offers no door.
func TestFWResolverReadsSNATSections(t *testing.T) {
	r := newFWResolver(fwTestConfig(), fwTestIfaces())

	snat, _ := parseFWLogLine(`[ 1.0] @nat[0]: IN= OUT=wan0 SRC=10.0.0.5 DST=1.1.1.1 PROTO=UDP DPT=53`)
	e := r.event(1, 1, snat)
	if e.Rule != "@nat[0]" || e.Verdict != "accept" {
		t.Errorf("a logged snat section is a decision with a name: %+v", e)
	}
	if e.RuleHref != "" {
		t.Errorf("href = %q, want none: no page in this shell edits a nat section", e.RuleHref)
	}
}

// TestFWResolverStaysHonestWhereItCannotResolve: a rule that only marks reached
// no verdict, an unknown prefix claims none, and a device no zone owns is named
// as the kernel named it.
func TestFWResolverStaysHonestWhereItCannotResolve(t *testing.T) {
	r := newFWResolver(fwTestConfig(), fwTestIfaces())

	marked, _ := parseFWLogLine(`[ 1.0] Mark-VoIP: IN=br-lan OUT=wan0 SRC=10.0.0.5 DST=1.1.1.1 PROTO=UDP DPT=5060`)
	if e := r.event(1, 1, marked); e.Verdict != "" || e.Rule != "Mark-VoIP" {
		t.Errorf("a rule that only marks decided nothing: %+v", e)
	}

	unknown, _ := parseFWLogLine(`[ 1.0] Something-Else: IN=eth9 OUT= SRC=1.2.3.4 DST=5.6.7.8`)
	e := r.event(1, 1, unknown)
	if e.Verdict != "" || e.Rule != "" {
		t.Errorf("a prefix nothing explains claims nothing: %+v", e)
	}
	if e.From != "eth9" {
		t.Errorf("a device no zone owns is named as the kernel named it: %q", e.From)
	}

	// A zone named by device rather than by network still joins.
	mgmt, _ := parseFWLogLine(`[ 1.0] Log-WAN-probes: IN=eth0 OUT= SRC=1.2.3.4 DST=5.6.7.8`)
	if e := r.event(1, 1, mgmt); e.From != "mgmt" {
		t.Errorf("a zone that names a device outright still owns it: %q", e.From)
	}
}

// TestFWProtoSpeaksTheColumnVocabulary: the protocol column is a closed set of
// lower-case words; a protocol the kernel could only number stays a number.
func TestFWProtoSpeaksTheColumnVocabulary(t *testing.T) {
	for in, want := range map[string]string{
		"TCP": "tcp", "UDP": "udp", "ICMP": "icmp", "ICMPv6": "icmpv6", "47": "47", "": "",
	} {
		if got := fwProto(in); got != want {
			t.Errorf("fwProto(%q) = %q, want %q", in, got, want)
		}
	}
	num, _ := parseFWLogLine(fwLineNumProto)
	if e := (&fwResolver{}).event(1, 1, num); e.Proto != "47" {
		t.Errorf("numeric protocol = %q", e.Proto)
	}
	none, _ := parseFWLogLine(fwLineNoProto)
	if e := (&fwResolver{}).event(1, 1, none); e.Proto != "" || e.Port != "" {
		t.Errorf("a line with no protocol states none: %+v", e)
	}
	v6, _ := parseFWLogLine(fwLineIPv6)
	if e := (&fwResolver{}).event(1, 1, v6); e.Src != "2001:0db8:0000:0000:0000:0000:0000:0009" || e.Port != "8443" {
		t.Errorf("an IPv6 verdict reads the same way: %+v", e)
	}
}
