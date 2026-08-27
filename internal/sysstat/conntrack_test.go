// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package sysstat

import "testing"

const testConntrack = "ipv4     2 tcp      6 431999 ESTABLISHED src=192.168.77.195 dst=104.16.132.229 sport=51234 dport=443 packets=100 bytes=12000 src=104.16.132.229 dst=172.30.1.171 sport=443 dport=51234 packets=200 bytes=250000 [ASSURED] mark=0 zone=0 use=2\n" +
	"ipv4     2 udp      17 30 src=192.168.77.195 dst=192.168.77.1 sport=40000 dport=53 packets=1 bytes=70 src=192.168.77.1 dst=192.168.77.195 sport=53 dport=40000 packets=1 bytes=150 mark=0 zone=0 use=2\n" +
	"ipv6    10 tcp      6 300 ESTABLISHED src=fd42:07ea:aa00:0000:5075:3bff:fe72:32b7 dst=fd42:07ea:0001:0000:0000:0000:0000:0002 sport=41000 dport=8000 packets=10 bytes=900 src=fd42:07ea:0001:0000:0000:0000:0000:0002 dst=fd42:07ea:aa00:0000:5075:3bff:fe72:32b7 sport=8000 dport=41000 packets=20 bytes=30000 [ASSURED] mark=0 zone=0 use=2\n" +
	"ipv4     2 tcp      6 100 ESTABLISHED src=10.0.0.229 dst=172.20.0.10 sport=61657 dport=8080 src=172.20.0.10 dst=10.0.0.229 sport=8080 dport=61657 [ASSURED] mark=0 zone=0 use=2\n"

// TestParseConntrack: origin tuple carries the initiator; the first bytes=
// is what it sent, the second what came back. A line without accounting
// (no bytes=) still parses, zeroed.
func TestParseConntrack(t *testing.T) {
	entries := ParseConntrack([]byte(testConntrack))
	if len(entries) != 4 {
		t.Fatalf("got %d entries, want 4: %+v", len(entries), entries)
	}
	tcp := entries[0]
	if tcp.Proto != "tcp" || tcp.Src != "192.168.77.195" || tcp.Dst != "104.16.132.229" ||
		tcp.TxBytes != 12000 || tcp.RxBytes != 250000 {
		t.Errorf("tcp entry = %+v", tcp)
	}
	v6 := entries[2]
	if v6.Src != "fd42:07ea:aa00:0000:5075:3bff:fe72:32b7" || v6.TxBytes != 900 || v6.RxBytes != 30000 {
		t.Errorf("v6 entry = %+v", v6)
	}
	if noacct := entries[3]; noacct.TxBytes != 0 || noacct.RxBytes != 0 {
		t.Errorf("unaccounted entry = %+v, want zero bytes", noacct)
	}
}

// TestAggregateTraffic: a device's addresses (both families) fold to one
// total; a connection initiated *toward* an address mirrors directions;
// unrelated flows (the shell's own mgmt traffic) don't count.
func TestAggregateTraffic(t *testing.T) {
	entries := ParseConntrack([]byte(testConntrack))
	totals := AggregateTraffic(entries, map[string]string{
		"192.168.77.195": "a2:ba:a6:35:2b:b8",
		"fd42:07ea:aa00:0000:5075:3bff:fe72:32b7": "a2:ba:a6:35:2b:b8",
	})
	if len(totals) != 1 {
		t.Fatalf("totals = %+v, want one device", totals)
	}
	dev := totals["a2:ba:a6:35:2b:b8"]
	if dev.Conns != 3 || dev.TxBytes != 12000+70+900 || dev.RxBytes != 250000+150+30000 {
		t.Errorf("device totals = %+v", dev)
	}
}
