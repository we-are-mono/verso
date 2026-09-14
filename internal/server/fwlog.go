// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package server

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/we-are-mono/verso/internal/openwrt"
)

// The firewall's live activity: turning the kernel's own account of a verdict
// into a row a person can read.
//
// firewall4 logs only what is asked of it. A rule with "Log matching packets"
// and a zone with logging on both hand the kernel a *prefix*, and the kernel
// writes that prefix in front of its netfilter key=value dump of the packet:
//
//	[ 1730.874112] Log-WAN-probes: IN=wan0 OUT= MAC=… SRC=203.0.113.9 \
//	  DST=192.0.2.10 LEN=60 TOS=0x00 PREC=0x00 TTL=52 ID=54321 DF \
//	  PROTO=TCP SPT=51422 DPT=445 WINDOW=64240 RES=0x00 SYN URGP=0
//
// The bracketed stamp is the kernel's uptime clock (CONFIG_PRINTK_TIME; absent
// on a kernel built without it), stripped by logd only for its debug ring, so
// it is still in front of the message logd hands over. The prefix is everything
// between it and the netfilter fields, and it is the only thing that says which
// decision this was: fw4 writes `"<rule name>: "` for a rule and
// `"<verdict> <zone> <direction>: "` for a zone's own policy.
//
// Every value below the prefix is untrusted input — a rule name is whatever a
// person typed, a hostname whatever a device claims. Nothing here interpolates
// into markup: the parse yields data, the stream carries JSON, and the browser
// writes it as text.

// fwLogRuleEditor and fwLogRedirectEditor are where a logged verdict's rule
// opens. The stream is named for the firewall and so is the door it opens: a
// row states which rule decided it, and the only useful thing to do with that
// is to go and read it.
const (
	fwLogRuleEditor     = "/plugins/firewall/rules/"
	fwLogRedirectEditor = "/plugins/firewall/port-forwards/"
)

// fwEvent is one firewall verdict as the activity stream carries it. Every
// field is a string or a number the browser renders as text — never markup, and
// never a colour: the verdict is a word, and the shell's own table maps it to a
// tone (ADR-005).
type fwEvent struct {
	ID       int64  `json:"id"`
	At       int64  `json:"at"`      // seconds since the epoch, from logd's own stamp
	Verdict  string `json:"verdict"` // "accept" | "reject" | "drop"; empty when the rule only marks
	From     string `json:"from"`    // the ingress zone, or the kernel device no zone owns
	Src      string `json:"src"`
	SPort    string `json:"sport,omitempty"` // the source port, where the protocol has one
	ToKind   string `json:"to_kind"`         // "router" (the packet was for this device) | "device"
	ToZone   string `json:"to_zone,omitempty"`
	To       string `json:"to"`
	Proto    string `json:"proto"`
	Port     string `json:"port"` // the destination port; empty where the protocol has none
	Rule     string `json:"rule"` // empty for a zone-policy verdict: no rule decided it
	RuleHref string `json:"rule_href,omitempty"`
	// Tags are the coarse cuts a listing's bar can narrow this line by. One
	// question is asked of a firewall log often enough to be a cut of its own —
	// was this traffic stopped or let through — and the verdict already answers
	// it, so the stream says so rather than making the browser reason about
	// verdict names it should not know.
	Tags []string `json:"tags,omitempty"`
}

// fwLogLine is one parsed kernel log line: the fw4 prefix that names the
// decision, and the netfilter fields that describe the packet.
type fwLogLine struct {
	Prefix string
	Fields map[string]string
}

// fwNetfilterHead matches the signature every netfilter log line carries — the
// interface pair the kernel always writes first — and captures everything
// before it as the log prefix. Anchoring on the pair rather than on "IN="
// alone keeps a rule whose *name* contains "IN=" from cutting the prefix short.
var fwNetfilterHead = regexp.MustCompile(`(?:^|\s)IN=(\S*) OUT=(\S*)(?:\s|$)`)

// fwKernelStamp is the kernel's uptime prefix, present when the kernel was
// built with CONFIG_PRINTK_TIME and absent when it was not.
var fwKernelStamp = regexp.MustCompile(`^\[\s*\d+\.\d+\]\s*`)

// fwZonePolicyPrefix matches the prefixes fw4 generates for a zone's own
// policy — `accept lan in: `, `reject wan forward: `, `drop guest out: ` — and
// for the invalid-conntrack drop, `drop wan invalid ct state: `. These name a
// verdict nobody wrote a rule for, which is why such a row has no door.
var fwZonePolicyPrefix = regexp.MustCompile(`^(accept|reject|drop) (\S+) (in|out|forward|invalid ct state)$`)

// parseFWLogLine reads one message from the log ring. It reports whether the
// line is a netfilter verdict at all — the ring carries everything the device
// says, and all but a few lines of it are somebody else's business.
//
// The parse never fails and never panics: a line is either the shape or it is
// not, and every field it yields is whatever the kernel wrote, unexamined.
func parseFWLogLine(msg string) (fwLogLine, bool) {
	loc := fwNetfilterHead.FindStringIndex(msg)
	if loc == nil {
		return fwLogLine{}, false
	}
	head := msg[:loc[0]]
	head = fwKernelStamp.ReplaceAllString(head, "")
	// A line that reached logd through remote syslog rather than the kernel
	// ring wears the "kernel:" tag logread prints; the prefix is behind it.
	head = strings.TrimPrefix(head, "kernel:")
	// fw4 ends every prefix with ": ", so exactly one colon is punctuation —
	// never part of the name a rule was given. A person who named a rule
	// "Ports::web" keeps every colon they typed, which is what lets that rule be
	// found in the config at all.
	prefix := strings.TrimSuffix(strings.TrimSpace(head), ":")

	fields := make(map[string]string, 16)
	// The interface pair is consumed by the head match; start the field walk at
	// it so IN= and OUT= land in the map like every other key.
	for _, token := range strings.Fields(msg[loc[0]:]) {
		key, value, ok := strings.Cut(token, "=")
		if !ok || key == "" {
			continue // a bare flag (DF, SYN, URGP): the packet's shape, not its identity
		}
		if _, seen := fields[key]; !seen {
			fields[key] = value
		}
	}
	return fwLogLine{Prefix: strings.TrimSpace(prefix), Fields: fields}, true
}

// fwRule is one logging section of the firewall config, as the resolver needs
// it: the prefix the kernel will write for it, the verdict its target decides,
// and the address of its editor.
type fwRule struct {
	Name    string
	Verdict string
	Href    string
}

// fwTargets maps a firewall section's target onto the verdict vocabulary the
// stream speaks. A target that decides nothing (MARK, NOTRACK, a helper) is
// absent on purpose: such a rule can log, but it did not decide, and claiming
// a verdict it never reached would be a lie the colour would repeat.
var fwTargets = map[string]string{
	"ACCEPT": "accept",
	"REJECT": "reject",
	"DROP":   "drop",
	// A port forward that logs is a permission granted: the packet was let in
	// and rewritten to its destination. The same holds for the source rewrite a
	// `config nat` section performs. Masquerade is deliberately absent: it
	// rewrites an address and decides nothing about the packet's fate.
	"DNAT": "accept",
	"SNAT": "accept",
}

// fwResolver turns parsed lines into events. It holds the two mappings a log
// line cannot carry: which rule a prefix belongs to, and which zone owns the
// kernel device the packet arrived on. Both come from configuration and netifd,
// so they are refreshed on their own slow clock, not once per line.
type fwResolver struct {
	rules map[string]fwRule // log prefix → the section that wrote it
	zones map[string]string // kernel device → zone name
}

// newFWResolver builds the mappings from the firewall config and netifd's view
// of which device each logical interface currently owns. A miss on either read
// is not fatal: an unresolved rule is a row with no door, an unresolved device
// is a row that names the kernel's own device — both honest, both still rows.
func newFWResolver(config map[string]any, ifaces []openwrt.NetIface) *fwResolver {
	r := &fwResolver{rules: map[string]fwRule{}, zones: map[string]string{}}
	// interface name → the kernel device netifd has it on right now.
	devices := make(map[string]string, len(ifaces))
	for _, i := range ifaces {
		if i.Device != "" {
			devices[i.Name] = i.Device
		}
	}
	// fw4 names an anonymous section positionally — @rule[0], @redirect[1] —
	// counting the sections of that type in the order the file holds them, named
	// ones included. Walking the snapshot in that order is what arrives at the
	// same numbers; a snapshot carrying no `.index` sorts by id, which is
	// deterministic rather than right, and only a config rpcd did not write can
	// be in that state.
	seen := make(map[string]int, 4)
	for _, id := range sectionOrder(config) {
		section, ok := config[id].(map[string]any)
		if !ok {
			continue
		}
		secType := uciString(section[".type"])
		position := seen[secType]
		seen[secType]++
		switch secType {
		case "rule":
			r.addRule(id, section, secType, position, fwLogRuleEditor)
		case "redirect":
			r.addRule(id, section, secType, position, fwLogRedirectEditor)
		case "nat":
			// A source-rewrite section logs with the same `<name>: ` prefix, and
			// nothing in this shell edits one — so the row names the decision and
			// offers no door. An empty href renders the name as plain text.
			r.addRule(id, section, secType, position, "")
		case "zone":
			r.addZone(section, devices)
		}
	}
	return r
}

// addRule records the prefix fw4 will write for one logging section. fw4 names a
// section by its `name` option and falls back to `section_id`, and the prefix is
// that name followed by ": " — so the name is the whole key. `section_id` is the
// uci section name where a config states one (`config rule 'block_telnet'`,
// which is also the id rpcd hands back) and `@<type>[<position>]` where it does
// not. rpcd's config id for an anonymous section — cfg13dc81 — is never a prefix
// fw4 writes, so it is never a key here.
func (r *fwResolver) addRule(id string, section map[string]any, secType string, position int, editor string) {
	name := uciString(section["name"])
	if name == "" {
		name = fwSectionID(id, section, secType, position)
	}
	if _, taken := r.rules[name]; taken {
		return // two sections sharing a name: the first is as good an answer as the second
	}
	rule := fwRule{
		Name:    name,
		Verdict: fwTargets[strings.ToUpper(uciString(section["target"]))],
	}
	if editor != "" {
		rule.Href = editor + id
	}
	r.rules[name] = rule
}

// fwSectionID is fw4's own `section_id`: an anonymous section is known by where
// it sits among the sections of its type, and a named one by its name.
func fwSectionID(id string, section map[string]any, secType string, position int) string {
	if anonymous, _ := section[".anonymous"].(bool); anonymous {
		return "@" + secType + "[" + strconv.Itoa(position) + "]"
	}
	return id
}

// addZone records which kernel devices a zone covers: the devices of the
// logical interfaces it lists, plus any device it names outright.
func (r *fwResolver) addZone(section map[string]any, devices map[string]string) {
	zone := uciString(section["name"])
	if zone == "" {
		return
	}
	for _, network := range uciWords(section["network"]) {
		if device := devices[network]; device != "" {
			r.zones[device] = zone
		}
	}
	for _, device := range uciWords(section["device"]) {
		// A zone may name a wildcard device family (eth+); the kernel writes
		// the exact device, so a pattern cannot be joined and is left alone.
		if device != "" && !strings.ContainsAny(device, "+*") {
			r.zones[device] = zone
		}
	}
}

// event resolves one parsed line into the row the stream carries. id and at
// come from the log record itself — logd's monotonic counter is the cursor a
// reader advances, and its wall stamp is when the packet was seen.
func (r *fwResolver) event(id, at int64, line fwLogLine) fwEvent {
	in, out := line.Fields["IN"], line.Fields["OUT"]
	e := fwEvent{
		ID:    id,
		At:    at,
		Src:   line.Fields["SRC"],
		SPort: line.Fields["SPT"],
		Proto: fwProto(line.Fields["PROTO"]),
		Port:  line.Fields["DPT"],
	}
	// Where the packet came from: the zone that owns the arrival device, the
	// device itself when no zone claims it, and this router when it has no
	// arrival device at all (a packet this device sent).
	switch {
	case in == "":
		e.From = "router"
	case r.zones[in] != "":
		e.From = r.zones[in]
	default:
		e.From = in
	}
	// Where it was going: a packet with no departure device was for this
	// device; anything else was on its way to something else on the network,
	// and the zone that owns the departure device names where that is.
	if out == "" {
		e.ToKind, e.To = "router", "router"
	} else {
		e.ToKind, e.To = "device", line.Fields["DST"]
		if zone := r.zones[out]; zone != "" {
			e.ToZone = zone
		}
	}
	// Which decision this was. A configured rule's name is ground truth and is
	// tried first: a person may well name a rule "drop wan in", and their rule
	// is what the prefix meant.
	if rule, ok := r.rules[line.Prefix]; ok {
		e.Rule, e.Verdict, e.RuleHref = rule.Name, rule.Verdict, rule.Href
		return e.tagged()
	}
	if m := fwZonePolicyPrefix.FindStringSubmatch(line.Prefix); m != nil {
		// The zone's own policy decided this: there is no rule to open, and
		// the zone the policy belongs to is a better "from" than a device the
		// configuration did not join.
		e.Verdict = m[1]
		if e.From == "" || e.From == in {
			e.From = m[2]
		}
		return e.tagged()
	}
	// A prefix nothing explains: the verdict is unknown, and saying so (an
	// empty pill reads as a quiet dash) beats guessing one.
	return e
}

// tagged states whether the traffic got through. A verdict nobody could resolve
// carries no tag: it is neither, and a cut for either would be a claim.
func (e fwEvent) tagged() fwEvent {
	switch e.Verdict {
	case "":
		return e
	case "accept":
		e.Tags = []string{fwTagAllowed}
	default:
		e.Tags = []string{fwTagBlocked}
	}
	return e
}

// The cuts a firewall log is narrowed by, named once for the stream that writes
// them and the page that offers them.
const (
	fwTagAllowed = "allowed"
	fwTagBlocked = "blocked"
)

// fwProto reads the protocol the way the row states it: the closed vocabulary
// in lower case, a numeric protocol as its number. The kernel writes TCP, UDP,
// ICMP, ICMPv6, AH, ESP by name and everything else as a number.
func fwProto(proto string) string {
	if proto == "" {
		return ""
	}
	if _, err := strconv.Atoi(proto); err == nil {
		return proto
	}
	return strings.ToLower(proto)
}

// uciString reads a uci option that should be a scalar. rpcd hands back
// strings, and a list option arrives as an array — a caller asking for one
// value from a list gets the first.
func uciString(v any) string {
	switch v := v.(type) {
	case string:
		return v
	case []any:
		if len(v) > 0 {
			return uciString(v[0])
		}
	}
	return ""
}

// uciWords reads a uci option as the set of names it holds. uci keeps a list of
// one as a plain string, and a config written by hand may put several names in
// one option separated by spaces — fw4 reads both that way, so this does too.
func uciWords(v any) []string {
	var out []string
	for _, entry := range uciList(v) {
		out = append(out, strings.Fields(entry)...)
	}
	return out
}

// Whether anything is configured to log at all is the activity page's own
// question — the honest empty state rests on it — and the firewall plugin,
// which holds the config and owns that page, is where it is answered.
