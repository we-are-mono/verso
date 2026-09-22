// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

use super::*;
use serde_json::json;

#[test]
fn generator_transport_change_is_idempotent_and_reversible() {
    let source="limit rate {{ rule.log_limit.rate }}/second log prefix {{ fw4.quote(rule.log, true) }}\n{%+ if (rule.log): -%} log prefix \"drop {{ zone.name }} in: \" {%+ endif -%}\n";
    let changed = route::template(source, true);
    assert_eq!(changed.matches("group 4242").count(), 2);
    assert!(!changed.contains("log prefix "));
    assert_eq!(route::template(&changed, true), changed);
    assert_eq!(route::template(&changed, false), source);
}

#[test]
fn routing_preserves_matches_verdicts_limits_and_counters() {
    let rule = json!({"family":"inet","table":"fw4","chain":"input_wan","handle":12,
        "comment":"!fw4: reject wan", "expr":[
        {"match":{"op":"==","left":{"meta":{"key":"l4proto"}},"right":"tcp"}},
        {"limit":{"rate":10,"per":"second"}}, {"counter":{"packets":42,"bytes":2048}},
        {"log":{"prefix":"reject wan in: ","level":"warn"}}, {"reject":null}]});
    let plan = route::plan(&json!({"nftables":[{"rule":rule.clone()}]}), true).unwrap();
    let replacement = &plan["nftables"][0]["replace"]["rule"];
    let mut expected = rule;
    expected["expr"][3] = json!({"log":{"prefix":"reject wan in: ","group":GROUP,"snaplen":SNAPLEN,"queue-threshold":1}});
    assert_eq!(replacement, &expected);
    assert!(
        route::plan(&json!({"nftables":[{"rule":replacement}]}), true).unwrap()["nftables"]
            .as_array()
            .unwrap()
            .is_empty()
    );
}

#[test]
fn routing_leaves_other_tables_and_explicit_nflog_groups_alone() {
    let rule = |table: &str, log| json!({"rule":{"family":"inet","table":table,"chain":"input","handle":1,"expr":[{"log":log},{"accept":null}]}});
    let rules = json!({"nftables":[rule("custom",json!({})),rule("fw4",json!({"group":0})),rule("fw4",json!({"group":42}))]});
    assert_eq!(route::plan(&rules, true).unwrap(), json!({"nftables":[]}));
    assert!(route::plan(&json!({}), true).is_err());
    assert_eq!(route::plan(&rules, false).unwrap(), json!({"nftables":[]}));
    let managed = rule("fw4", json!({"group":GROUP,"prefix":"custom prefix"}));
    let restored = route::plan(&json!({"nftables":[managed]}), false).unwrap();
    assert_eq!(
        restored["nftables"][0]["replace"]["rule"]["expr"][0],
        json!({"log":{"prefix":"custom prefix"}})
    );
}

#[test]
fn buffer_is_bounded_and_readers_do_not_consume_each_others_history() {
    let mut buffer = Buffer::new(3);
    buffer.ready = true;
    for i in 0..5 {
        buffer.push(i, format!("packet {i}"));
    }
    let read = buffer.read("", -1, 100);
    assert_eq!(read["entries"].as_array().unwrap().len(), 3);
    assert_eq!(read["entries"][0]["id"], 3);
    assert_eq!(buffer.read("", -1, 100), read);
    assert_eq!(
        buffer.read(&buffer.generation, 4, 100)["entries"]
            .as_array()
            .unwrap()
            .len(),
        1
    );
    assert!(buffer.read("previous-process", 999, 100)["reset"]
        .as_bool()
        .unwrap());
    assert_eq!(
        buffer.read("previous-process", 999, 100)["entries"]
            .as_array()
            .unwrap()
            .len(),
        3
    );
    assert_eq!(buffer.read(&buffer.generation, 1, 100)["lost"], true);
    assert_eq!(buffer.read(&buffer.generation, 3, 1)["lost"], true);
    assert_eq!(buffer.read(&buffer.generation, 4, 1)["lost"], false);
    assert_eq!(buffer.read(&buffer.generation, i64::MAX, 1)["lost"], false);
    buffer.push(6, "ž".repeat(MAX_MESSAGE));
    assert_eq!(buffer.entries.back().unwrap().message.len(), MAX_MESSAGE);
    buffer.ready = false;
    assert_eq!(buffer.read(&buffer.generation, 0, 100)["available"], false);
}

#[test]
fn netlink_decodes_metadata_and_rejects_malformed_messages() {
    fn attribute(message: &mut Vec<u8>, kind: u16, data: &[u8]) {
        message.extend_from_slice(&((data.len() + 4) as u16).to_ne_bytes());
        message.extend_from_slice(&kind.to_ne_bytes());
        message.extend_from_slice(data);
        while !message.len().is_multiple_of(4) {
            message.push(0);
        }
    }
    let mut message = vec![0u8; 20];
    message[4..6].copy_from_slice(&0x400u16.to_ne_bytes());
    message[18..20].copy_from_slice(&GROUP.to_be_bytes());
    attribute(&mut message, 10, b"custom: \n\0ignored");
    let mut timestamp = 1700000000u64.to_be_bytes().to_vec();
    timestamp.extend_from_slice(&123000u64.to_be_bytes());
    attribute(&mut message, 3, &timestamp);
    let mut packet = vec![0u8; 28];
    packet[0] = 0x45;
    packet[2..4].copy_from_slice(&28u16.to_be_bytes());
    packet[9] = 17;
    packet[12..16].copy_from_slice(&[192, 0, 2, 1]);
    packet[16..20].copy_from_slice(&[192, 0, 2, 2]);
    packet[20..24].copy_from_slice(&[0, 53, 0, 123]);
    attribute(&mut message, 9, &packet);
    let len = message.len() as u32;
    message[..4].copy_from_slice(&len.to_ne_bytes());
    let decoded = netlink::decode(&message);
    assert_eq!(
        decoded,
        vec![(
            1700000000123,
            "custom:  IN= OUT= SRC=192.0.2.1 DST=192.0.2.2 PROTO=UDP SPT=53 DPT=123".into()
        )]
    );
    for end in 0..message.len() {
        assert!(netlink::decode(&message[..end]).is_empty());
    }
    let mut wrong_group = message.clone();
    wrong_group[18..20].copy_from_slice(&42u16.to_be_bytes());
    assert!(netlink::decode(&wrong_group).is_empty());
    for length in [0, 1, 3, u16::MAX] {
        let mut malformed = message.clone();
        malformed[20..22].copy_from_slice(&length.to_ne_bytes());
        assert!(netlink::decode(&malformed).is_empty());
    }
}

#[test]
fn packet_decoder_handles_ipv4_ipv6_and_truncation_without_reading_payload() {
    let mut v4 = vec![0u8; 28];
    v4[0] = 0x45;
    v4[2..4].copy_from_slice(&28u16.to_be_bytes());
    v4[9] = 17;
    v4[12..16].copy_from_slice(&[192, 0, 2, 7]);
    v4[16..20].copy_from_slice(&[198, 51, 100, 1]);
    v4[20..24].copy_from_slice(&[0x12, 0x34, 0, 53]);
    let parsed = netlink::packet_fields(&v4).unwrap();
    assert!(parsed.contains("SRC=192.0.2.7 DST=198.51.100.1"));
    assert!(parsed.contains("PROTO=UDP SPT=4660 DPT=53"));
    for end in 0..20 {
        assert!(netlink::packet_fields(&v4[..end]).is_none());
    }
    // A non-first fragment's payload must never become made-up ports.
    v4[7] = 1;
    assert!(!netlink::packet_fields(&v4).unwrap().contains("DPT="));
    let mut v6 = vec![0u8; 52];
    v6[0] = 0x60;
    v6[6] = 0;
    v6[23] = 1;
    v6[39] = 2;
    v6[40] = 6;
    v6[48..52].copy_from_slice(&[0x12, 0x34, 1, 0xbb]);
    let parsed = netlink::packet_fields(&v6).unwrap();
    assert!(parsed.contains("SRC=::1 DST=::2"));
    assert!(parsed.contains("PROTO=TCP SPT=4660 DPT=443"));
    assert!(netlink::packet_fields(&[]).is_none());
}
