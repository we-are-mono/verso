// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! Per-device usage (ADR-018): the live bytes each address has moved, read from
//! conntrack's accounting while someone watches; each day's bytes per device,
//! read from nlbwmon; and the watchdog that moves aside a day nlbwmon can no
//! longer read, so one torn write does not stop the counting.
//!
//! Conntrack is dumped over ctnetlink, not read from /proc/net/nf_conntrack:
//! the proc file walks the table again from its start for every page it hands
//! out, so a busy router's table of ten thousand connections costs a second of
//! kernel time to read, every second someone watches.

use crate::netlink::{self, Message, NETLINK_NETFILTER};
use serde_json::{json, Map, Value};
use std::collections::{HashMap, HashSet};
use std::fs;
use std::net::{IpAddr, Ipv4Addr, Ipv6Addr};
use std::os::unix::net::UnixStream;
use std::path::Path;
use std::process::Command;
use std::sync::Mutex;
use std::time::Duration;

/// ctnetlink's dump request (NFNL_SUBSYS_CTNETLINK << 8 | IPCTNL_MSG_CT_GET)
/// and each connection in its answer (… | IPCTNL_MSG_CT_NEW).
const CT_GET: u16 = 1 << 8 | 1;
const CT_NEW: u16 = 1 << 8;
/// struct nfgenmsg: AF_UNSPEC for every family, NFNETLINK_V0, no resource.
const CT_HEADER: [u8; 4] = [0; 4];
const CTA_TUPLE_ORIG: u16 = 1;
const CTA_TUPLE_REPLY: u16 = 2;
const CTA_COUNTERS_ORIG: u16 = 9;
const CTA_COUNTERS_REPLY: u16 = 10;
const CTA_ZONE: u16 = 18;
const CTA_TUPLE_IP: u16 = 1;
const CTA_IP_V4_SRC: u16 = 1;
const CTA_IP_V6_SRC: u16 = 3;
const CTA_COUNTERS_BYTES: u16 = 2;
const CTA_COUNTERS32_BYTES: u16 = 4;

const NLBW_SOCKET: &str = "/var/run/nlbwmon.sock";
/// The days the shell may ask for at once: this month and the last, whole.
const WINDOW_DAYS: usize = 62;
/// At most this many addresses in one live reading: a roster's worth, bounded.
const MAX_ADDRESSES: usize = 1024;

/// One tracked connection: its key, and each direction's source with the bytes
/// it sent (none while the kernel keeps no accounting).
#[derive(Debug, PartialEq)]
struct Conn {
    key: Vec<u8>,
    orig_src: IpAddr,
    orig_bytes: Option<u64>,
    reply_src: IpAddr,
    reply_bytes: Option<u64>,
}

/// parse_conntrack reads one connection of a ctnetlink dump: struct nfgenmsg,
/// then its attributes. Each direction's tuple names its source, and its
/// counters (absent while the kernel keeps no accounting) the bytes that
/// source sent. The key is the original tuple and the zone, as the kernel
/// encodes them, which stay the same while the state, timeout and counters
/// move.
fn parse_conntrack(message: &Message) -> Option<Conn> {
    if message.kind != CT_NEW || message.body.len() < CT_HEADER.len() {
        return None;
    }
    let (mut key, mut zone) = (None, &[][..]);
    let (mut orig_src, mut reply_src) = (None, None);
    let (mut orig_bytes, mut reply_bytes) = (None, None);
    for (kind, value) in netlink::attributes(&message.body[CT_HEADER.len()..]) {
        match kind {
            CTA_TUPLE_ORIG => {
                key = Some(value);
                orig_src = tuple_source(value);
            }
            CTA_TUPLE_REPLY => reply_src = tuple_source(value),
            CTA_COUNTERS_ORIG => orig_bytes = counted_bytes(value),
            CTA_COUNTERS_REPLY => reply_bytes = counted_bytes(value),
            CTA_ZONE => zone = value,
            _ => {}
        }
    }
    let mut key = key?.to_vec();
    key.extend_from_slice(zone);
    Some(Conn {
        key,
        orig_src: orig_src?,
        orig_bytes,
        reply_src: reply_src?,
        reply_bytes,
    })
}

/// tuple_source is a tuple's source address (CTA_TUPLE_IP's v4 or v6 source).
fn tuple_source(tuple: &[u8]) -> Option<IpAddr> {
    let (_, ip) = netlink::attributes(tuple).find(|(kind, _)| *kind == CTA_TUPLE_IP)?;
    netlink::attributes(ip).find_map(|(kind, value)| match kind {
        CTA_IP_V4_SRC => Some(IpAddr::from(Ipv4Addr::from(
            <[u8; 4]>::try_from(value).ok()?,
        ))),
        CTA_IP_V6_SRC => Some(IpAddr::from(Ipv6Addr::from(
            <[u8; 16]>::try_from(value).ok()?,
        ))),
        _ => None,
    })
}

/// counted_bytes is a direction's byte counter, in network byte order: 64
/// bits, or 32 from a kernel that keeps them that wide.
fn counted_bytes(counters: &[u8]) -> Option<u64> {
    netlink::attributes(counters).find_map(|(kind, value)| match kind {
        CTA_COUNTERS_BYTES => Some(u64::from_be_bytes(value.try_into().ok()?)),
        CTA_COUNTERS32_BYTES => Some(u32::from_be_bytes(value.try_into().ok()?).into()),
        _ => None,
    })
}

/// Live is the running byte totals per address since the helper started. They
/// only grow, so any reader turns two readings into a rate (ADR-018).
#[derive(Default)]
pub struct Live {
    /// Each connection's bytes at the last reading, original then reply.
    last: HashMap<Vec<u8>, (u64, u64)>,
    /// Each address's bytes sent and received.
    totals: HashMap<IpAddr, (u64, u64)>,
}

impl Live {
    /// absorb adds what each connection moved since the last reading: the
    /// original direction's bytes as sent by its source and received by the
    /// reply's source, the reply direction's the other way. A connection first
    /// seen counts in full, and one whose counters went back is a new one on the
    /// same tuple. False when the kernel keeps no accounting.
    fn absorb(&mut self, conns: Vec<Conn>) -> bool {
        let mut accounted = false;
        let mut seen = HashMap::with_capacity(conns.len());
        for conn in conns {
            let (Some(orig), Some(reply)) = (conn.orig_bytes, conn.reply_bytes) else {
                continue;
            };
            accounted = true;
            let (was_orig, was_reply) = self.last.get(&conn.key).copied().unwrap_or((0, 0));
            let (new_orig, new_reply) = if orig < was_orig || reply < was_reply {
                (orig, reply)
            } else {
                (orig - was_orig, reply - was_reply)
            };
            let source = self.totals.entry(conn.orig_src).or_default();
            source.0 += new_orig;
            source.1 += new_reply;
            let answer = self.totals.entry(conn.reply_src).or_default();
            answer.0 += new_reply;
            answer.1 += new_orig;
            seen.insert(conn.key, (orig, reply));
        }
        self.last = seen;
        accounted
    }

    /// report is the totals of the addresses asked for, zero for one not seen.
    fn report(&self, addresses: &[IpAddr]) -> Value {
        let mut out = Map::new();
        for address in addresses {
            let (sent, received) = self.totals.get(address).copied().unwrap_or((0, 0));
            out.insert(
                address.to_string(),
                json!({"sent": sent, "received": received}),
            );
        }
        Value::Object(out)
    }
}

/// Reader is the live totals behind the helper's lock.
#[derive(Default)]
pub struct Reader(Mutex<Live>);

impl Reader {
    /// read takes a reading of conntrack and answers the totals of the listed
    /// addresses (comma-separated), with whether the kernel accounts at all.
    pub fn read(&self, addresses: &str) -> Result<Value, String> {
        let addresses = addresses
            .split(',')
            .filter(|a| !a.is_empty())
            .map(|a| {
                a.parse::<IpAddr>()
                    .map_err(|_| format!("invalid address {a:?}"))
            })
            .collect::<Result<Vec<_>, _>>()?;
        if addresses.len() > MAX_ADDRESSES {
            return Err("too many addresses".into());
        }
        let conns: Vec<Conn> = netlink::dump(NETLINK_NETFILTER, CT_GET, &CT_HEADER)
            .map_err(|e| format!("conntrack: {e}"))?
            .iter()
            .filter_map(parse_conntrack)
            .collect();
        let empty = conns.is_empty();
        let mut live = self
            .0
            .lock()
            .map_err(|_| "usage lock poisoned".to_string())?;
        let accounted = live.absorb(conns) || empty;
        Ok(json!({"accounting": accounted, "addresses": live.report(&addresses)}))
    }
}

/// parse_day reads `nlbw -c json -g mac`: each MAC's bytes received (download)
/// and sent (upload) in that period.
fn parse_day(text: &str) -> Result<Map<String, Value>, String> {
    let value: Value =
        serde_json::from_str(text).map_err(|e| format!("nlbw returned invalid JSON: {e}"))?;
    let columns: Vec<&str> = value["columns"]
        .as_array()
        .ok_or("nlbw returned no columns")?
        .iter()
        .filter_map(Value::as_str)
        .collect();
    let at = |name: &str| {
        columns
            .iter()
            .position(|c| *c == name)
            .ok_or_else(|| format!("nlbw returned no {name} column"))
    };
    let (mac, received, sent) = (at("mac")?, at("rx_bytes")?, at("tx_bytes")?);
    let mut out = Map::new();
    for row in value["data"].as_array().ok_or("nlbw returned no data")? {
        let Some(name) = row[mac].as_str() else {
            continue;
        };
        out.insert(
            name.to_lowercase(),
            json!({"received": row[received].as_u64().unwrap_or(0), "sent": row[sent].as_u64().unwrap_or(0)}),
        );
    }
    Ok(out)
}

/// civil_day is a `YYYY-MM-DD` date's day number, so dates subtract.
fn civil_day(date: &str) -> Option<i64> {
    let mut parts = date.splitn(3, '-').map(|p| p.parse::<i64>().ok());
    let (y, m, d) = (parts.next()??, parts.next()??, parts.next()??);
    let y = if m <= 2 { y - 1 } else { y };
    let era = y.div_euclid(400);
    let yoe = y - era * 400;
    let doy = (153 * (if m > 2 { m - 3 } else { m + 9 }) + 2) / 5 + d - 1;
    Some(era * 146_097 + yoe * 365 + yoe / 4 - yoe / 100 + doy)
}

/// days_to_read is today, always read since it still grows, and every earlier
/// day on disk inside the window the caller does not already hold, newest
/// first. Days come from the files themselves, not from `nlbw -c list`, which
/// walks back from today and stops at the first day without a file: today's is
/// written only at its first commit, so after midnight it lists nothing, and a
/// day the router was off would hide every day before it.
fn days_to_read(
    today: &str,
    archived: &[String],
    known: &HashSet<String>,
    window: usize,
) -> Vec<String> {
    let Some(newest) = civil_day(today) else {
        return Vec::new();
    };
    let mut days: Vec<&String> = archived
        .iter()
        .filter(|d| d.as_str() != today && !known.contains(d.as_str()))
        .filter(|d| civil_day(d).is_some_and(|n| (0..window as i64).contains(&(newest - n))))
        .collect();
    days.sort_by(|a, b| b.cmp(a));
    std::iter::once(today.to_string())
        .chain(days.into_iter().cloned())
        .collect()
}

/// archived_day is the date a file in nlbwmon's directory holds, when it is a
/// day's file: `YYYYMMDD.db`, or `.db.gz`.
fn archived_day(name: &str) -> Option<String> {
    let stamp = name
        .strip_suffix(".db.gz")
        .or_else(|| name.strip_suffix(".db"))?;
    if stamp.len() != 8 || !stamp.bytes().all(|b| b.is_ascii_digit()) {
        return None;
    }
    Some(format!("{}-{}-{}", &stamp[..4], &stamp[4..6], &stamp[6..]))
}

/// nlbw_option is one of nlbwmon's options as uci holds it.
fn nlbw_option(option: &str) -> String {
    Command::new("uci")
        .args(["-q", "get", &format!("nlbwmon.@nlbwmon[0].{option}")])
        .output()
        .ok()
        .map(|o| String::from_utf8_lossy(&o.stdout).trim().to_string())
        .unwrap_or_default()
}

/// router_date is today as the router dates it, in a format of date's own; the
/// helper runs under procd as nlbwmon does, so both read the same clock and zone.
fn router_date(format: &str) -> Option<String> {
    let out = Command::new("date").arg(format).output().ok()?;
    let date = String::from_utf8_lossy(&out.stdout).trim().to_string();
    (!date.is_empty()).then_some(date)
}

fn nlbw(args: &[&str]) -> Result<String, String> {
    let output = Command::new("nlbw")
        .args(args)
        .output()
        .map_err(|e| format!("nlbw: {e}"))?;
    if !output.status.success() {
        return Err(format!(
            "nlbw: {}",
            String::from_utf8_lossy(&output.stderr).trim()
        ));
    }
    String::from_utf8(output.stdout).map_err(|e| format!("nlbw: {e}"))
}

fn enabled() -> bool {
    fs::read_dir("/etc/rc.d").is_ok_and(|entries| {
        entries.filter_map(Result::ok).any(|e| {
            e.file_name().to_string_lossy().ends_with("nlbwmon")
                && e.file_name().to_string_lossy().starts_with('S')
        })
    })
}

/// days answers whether usage is on, nlbwmon's today, and each day's per-MAC
/// bytes for today and the window's days the caller does not list as known.
pub fn days(known: &str) -> Result<Value, String> {
    if !enabled() {
        return Ok(json!({"enabled": false}));
    }
    let today = router_date("+%Y-%m-%d").ok_or("the router's date could not be read")?;
    let archived: Vec<String> = fs::read_dir(nlbw_option("database_directory"))
        .map(|entries| {
            entries
                .filter_map(Result::ok)
                .filter_map(|e| archived_day(&e.file_name().to_string_lossy()))
                .collect()
        })
        .unwrap_or_default();
    let known: HashSet<String> = known
        .split(',')
        .filter(|d| !d.is_empty())
        .map(str::to_string)
        .collect();
    let wanted = days_to_read(&today, &archived, &known, WINDOW_DAYS);
    let mut out = Map::new();
    for (i, day) in wanted.iter().enumerate() {
        out.insert(
            day.clone(),
            Value::Object(parse_day(&nlbw(&day_args(day, i == 0))?)?),
        );
    }
    Ok(json!({"enabled": true, "today": wanted.first(), "days": out}))
}

/// day_args asks nlbw for one day's per-device totals. Today is asked for with
/// no date: named by its date, nlbwmon answers from the file it last wrote,
/// not the counts it holds now.
fn day_args(day: &str, today: bool) -> Vec<&str> {
    let mut args = vec!["-c", "json", "-g", "mac"];
    if !today {
        args.extend(["-t", day]);
    }
    args
}

/// torn is the watchdog's whole decision: nlbwmon should be running, its socket
/// does not answer, and its current day is on disk but cannot be read. A day
/// that is not there yet, or reads fine, is someone else's problem.
fn torn(enabled: bool, answering: bool, readable: Option<bool>) -> bool {
    enabled && !answering && readable == Some(false)
}

/// current_day is nlbwmon's file for today, and whether it reads: None when
/// there is none. A compressed day must decompress to something.
fn current_day() -> Option<(String, bool)> {
    let dir = nlbw_option("database_directory");
    let compressed = nlbw_option("database_compress") != "0";
    let stamp = router_date("+%Y%m%d")?;
    if dir.is_empty() || stamp.len() != 8 {
        return None;
    }
    let path = format!("{dir}/{stamp}.db{}", if compressed { ".gz" } else { "" });
    let size = fs::metadata(&path).ok()?.len();
    let readable = if compressed {
        Command::new("gzip")
            .args(["-dc", &path])
            .output()
            .is_ok_and(|o| o.status.success() && !o.stdout.is_empty())
    } else {
        size > 0
    };
    Some((path, readable))
}

/// watchdog checks once a minute that a torn write has not stopped nlbwmon, and
/// moves the day it cannot read aside so it starts again (ADR-018).
pub fn watchdog() {
    std::thread::spawn(|| loop {
        std::thread::sleep(Duration::from_secs(60));
        let answering = Path::new(NLBW_SOCKET).exists() && UnixStream::connect(NLBW_SOCKET).is_ok();
        if !enabled() || answering {
            continue;
        }
        let Some((path, readable)) = current_day() else {
            continue;
        };
        if !torn(true, answering, Some(readable)) {
            continue;
        }
        let aside = format!("{path}.broken");
        match fs::rename(&path, &aside) {
            Ok(()) => {
                eprintln!("verso-rpcd: nlbwmon could not read {path}; moved it to {aside}");
                let _ = Command::new("/etc/init.d/nlbwmon").arg("restart").status();
            }
            Err(e) => eprintln!("verso-rpcd: move {path} aside: {e}"),
        }
    });
}

#[cfg(test)]
mod tests {
    use super::*;

    /// One direction of a connection: source, destination, ports, and the
    /// bytes its source sent (None while the kernel keeps no accounting).
    struct Side(&'static str, &'static str, u16, u16, Option<u64>);

    const OUTBOUND: [Side; 2] = [
        Side("192.168.77.20", "93.184.215.14", 51544, 443, Some(1500)),
        Side("93.184.215.14", "172.30.1.10", 443, 51544, Some(52000)),
    ];
    const FORWARDED: [Side; 2] = [
        Side("203.0.113.9", "172.30.1.10", 40000, 8080, Some(300)),
        Side("192.168.77.20", "203.0.113.9", 80, 40000, Some(9000)),
    ];
    const V6: [Side; 2] = [
        Side("fd42:7ea:1::20", "2001:db8::1", 5353, 53, Some(80)),
        Side("2001:db8::1", "fd42:7ea:1::20", 53, 5353, Some(120)),
    ];
    const UNACCOUNTED: [Side; 2] = [
        Side("10.0.0.232", "172.20.0.10", 38694, 8443, None),
        Side("172.20.0.10", "10.0.0.232", 8443, 38694, None),
    ];
    /// NLA_F_NESTED, which the kernel sets on every nested attribute.
    const NESTED: u16 = 0x8000;

    fn attribute(kind: u16, value: &[u8]) -> Vec<u8> {
        let mut out = ((4 + value.len()) as u16).to_ne_bytes().to_vec();
        out.extend_from_slice(&kind.to_ne_bytes());
        out.extend_from_slice(value);
        out.resize((out.len() + 3) & !3, 0);
        out
    }

    fn tuple(side: &Side) -> Vec<u8> {
        let ip = match (ip(side.0), ip(side.1)) {
            (IpAddr::V4(s), IpAddr::V4(d)) => {
                [attribute(1, &s.octets()), attribute(2, &d.octets())].concat()
            }
            (IpAddr::V6(s), IpAddr::V6(d)) => {
                [attribute(3, &s.octets()), attribute(4, &d.octets())].concat()
            }
            _ => unreachable!("one family per tuple"),
        };
        let proto = [
            attribute(1, &[6]),
            attribute(2, &side.2.to_be_bytes()),
            attribute(3, &side.3.to_be_bytes()),
        ]
        .concat();
        [
            attribute(CTA_TUPLE_IP | NESTED, &ip),
            attribute(2 | NESTED, &proto),
        ]
        .concat()
    }

    fn counters(bytes: u64) -> Vec<u8> {
        [
            attribute(1, &(bytes / 1000 + 1).to_be_bytes()),
            attribute(CTA_COUNTERS_BYTES, &bytes.to_be_bytes()),
        ]
        .concat()
    }

    /// conntrack is one connection as a ctnetlink dump sends it, with the
    /// timeout it has left.
    fn conntrack([orig, reply]: &[Side; 2], timeout: u32) -> Message {
        let mut body = CT_HEADER.to_vec();
        body.extend(attribute(CTA_TUPLE_ORIG | NESTED, &tuple(orig)));
        body.extend(attribute(CTA_TUPLE_REPLY | NESTED, &tuple(reply)));
        body.extend(attribute(7, &timeout.to_be_bytes()));
        if let (Some(o), Some(r)) = (orig.4, reply.4) {
            body.extend(attribute(CTA_COUNTERS_ORIG | NESTED, &counters(o)));
            body.extend(attribute(CTA_COUNTERS_REPLY | NESTED, &counters(r)));
        }
        body.extend(attribute(CTA_ZONE, &0u16.to_be_bytes()));
        Message { kind: CT_NEW, body }
    }

    fn conn(sides: &[Side; 2]) -> Conn {
        parse_conntrack(&conntrack(sides, 300)).expect("parsed")
    }

    fn ip(text: &str) -> IpAddr {
        text.parse().unwrap()
    }

    // A connection is its original source and the source of the reply, each
    // with the bytes it sent; the key names the connection whatever its
    // timeout and counters read now, so a later reading finds it again.
    #[test]
    fn a_conntrack_message_is_both_directions_and_what_each_sent() {
        let first = conn(&OUTBOUND);
        assert_eq!(first.orig_src, ip("192.168.77.20"));
        assert_eq!(first.orig_bytes, Some(1500));
        assert_eq!(first.reply_src, ip("93.184.215.14"));
        assert_eq!(first.reply_bytes, Some(52000));
        let [orig, Side(s, d, sp, dp, _)] = OUTBOUND;
        let later = parse_conntrack(&conntrack(&[orig, Side(s, d, sp, dp, Some(99000))], 120))
            .expect("parsed");
        assert_eq!(later.reply_bytes, Some(99000));
        assert_eq!(later.key, first.key);
        assert_ne!(conn(&FORWARDED).key, first.key);

        assert_eq!(conn(&V6).orig_src, ip("fd42:7ea:1::20"));

        let unaccounted = conn(&UNACCOUNTED);
        assert_eq!(
            (unaccounted.orig_bytes, unaccounted.reply_bytes),
            (None, None)
        );
    }

    // A kernel that keeps 32-bit counters still counts; anything that is not
    // a connection, or is cut short, is passed over.
    #[test]
    fn narrow_counters_count_and_other_messages_do_not() {
        let mut message = conntrack(&UNACCOUNTED, 300);
        message.body.extend(attribute(
            CTA_COUNTERS_ORIG | NESTED,
            &attribute(CTA_COUNTERS32_BYTES, &700u32.to_be_bytes()),
        ));
        assert_eq!(
            parse_conntrack(&message).expect("parsed").orig_bytes,
            Some(700)
        );

        let other = Message {
            kind: CT_NEW + 2,
            body: conntrack(&OUTBOUND, 300).body,
        };
        assert_eq!(parse_conntrack(&other), None);
        let mut cut = conntrack(&OUTBOUND, 300);
        cut.body.truncate(CT_HEADER.len() + 8);
        assert_eq!(parse_conntrack(&cut), None);
    }

    // The running totals only grow: what a connection moved since the last
    // reading is added once, a connection first seen counts in full, and a
    // forwarded connection counts for the LAN host that answers it.
    #[test]
    fn totals_grow_by_what_is_new_and_credit_both_ends() {
        // OUTBOUND with each direction's bytes as given.
        let outbound = |sent, received| {
            let [Side(a, b, c, d, _), Side(e, f, g, h, _)] = OUTBOUND;
            conn(&[
                Side(a, b, c, d, Some(sent)),
                Side(e, f, g, h, Some(received)),
            ])
        };
        let mut live = Live::default();
        assert!(live.absorb(vec![conn(&OUTBOUND), conn(&FORWARDED)]));
        let host = ip("192.168.77.20");
        assert_eq!(live.totals[&host], (1500 + 9000, 52000 + 300));
        assert_eq!(live.totals[&ip("93.184.215.14")], (52000, 1500));

        live.absorb(vec![outbound(1500, 60000), conn(&FORWARDED)]);
        assert_eq!(live.totals[&host], (1500 + 9000, 60000 + 300));

        // Gone, then back with fewer bytes: a new connection on the same tuple.
        live.absorb(vec![]);
        live.absorb(vec![outbound(10, 100)]);
        assert_eq!(live.totals[&host], (1500 + 9000 + 10, 60000 + 300 + 100));

        // Without accounting nothing is counted, and the reading says so.
        let mut off = Live::default();
        assert!(!off.absorb(vec![conn(&UNACCOUNTED)]));
        assert!(off.totals.is_empty());
    }

    #[test]
    fn a_report_names_only_the_addresses_asked_for() {
        let mut live = Live::default();
        live.absorb(vec![conn(&OUTBOUND)]);
        let report = live.report(&[ip("192.168.77.20"), ip("192.168.77.99")]);
        assert_eq!(
            report["192.168.77.20"],
            json!({"sent": 1500, "received": 52000})
        );
        assert_eq!(report["192.168.77.99"], json!({"sent": 0, "received": 0}));
        assert!(report.get("93.184.215.14").is_none());
    }

    // nlbw's JSON is columns and rows; a day is each MAC's bytes received
    // (download) and sent (upload), whatever order the columns come in.
    #[test]
    fn a_day_is_each_macs_received_and_sent_bytes() {
        let text = r#"{"columns":["mac","conns","rx_bytes","rx_pkts","tx_bytes","tx_pkts"],"data":[["02:42:7e:a0:00:01",4,5000,9,700,3],["9c:48:fb:00:00:01",1,0,0,0,0]]}"#;
        let day = parse_day(text).unwrap();
        assert_eq!(
            day["02:42:7e:a0:00:01"],
            json!({"received": 5000, "sent": 700})
        );
        assert_eq!(day.len(), 2);
        assert!(parse_day(r#"{"columns":["mac"],"data":[]}"#).is_err());
        assert!(parse_day("not json").is_err());
    }

    // Today is the router's date, always read, whether or not nlbwmon has
    // written its file yet; earlier days are the files on disk inside the
    // window the caller does not hold, newest first, a day without a file
    // hiding none before it.
    #[test]
    fn days_read_are_today_and_the_unknown_days_in_the_window() {
        let archived: Vec<String> = ["2026-10-05", "2026-10-03", "2026-08-01", "2026-10-06"]
            .iter()
            .map(|d| d.to_string())
            .collect();
        let known: HashSet<String> = ["2026-10-06".to_string()].into();
        assert_eq!(
            days_to_read("2026-10-07", &archived, &known, 5),
            vec!["2026-10-07", "2026-10-05", "2026-10-03"]
        );
        assert_eq!(
            days_to_read("2026-10-07", &[], &known, 5),
            vec!["2026-10-07"]
        );
        // Today's own file, once written, is still today, read live.
        assert_eq!(
            days_to_read("2026-10-07", &["2026-10-07".to_string()], &known, 5),
            vec!["2026-10-07"]
        );
    }

    // nlbwmon names a day's file by its date; a file moved aside, or anything
    // else in the directory, is not a day.
    #[test]
    fn a_day_is_a_file_named_by_its_date() {
        assert_eq!(
            archived_day("20261007.db.gz"),
            Some("2026-10-07".to_string())
        );
        assert_eq!(archived_day("20261007.db"), Some("2026-10-07".to_string()));
        for not in [
            "20261007.db.gz.broken",
            "0.db",
            "notes.txt",
            "2026100.db.gz",
            "2026xx07.db",
        ] {
            assert_eq!(archived_day(not), None, "{not}");
        }
    }

    // Today is the counts nlbwmon holds now, so it is asked for undated; an
    // earlier day is its file, by date.
    #[test]
    fn today_is_read_live_and_an_earlier_day_by_its_date() {
        assert_eq!(day_args("2026-10-07", true), ["-c", "json", "-g", "mac"]);
        assert_eq!(
            day_args("2026-10-06", false),
            ["-c", "json", "-g", "mac", "-t", "2026-10-06"]
        );
    }

    // A day is moved aside only when nlbwmon should run, does not answer, and
    // its current file is there but cannot be read.
    #[test]
    fn a_torn_day_is_one_nlbwmon_cannot_start_on() {
        assert!(torn(true, false, Some(false)));
        assert!(!torn(true, false, Some(true)));
        assert!(!torn(true, false, None));
        assert!(!torn(true, true, Some(false)));
        assert!(!torn(false, false, Some(false)));
    }
}
