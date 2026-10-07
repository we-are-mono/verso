// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! Per-device usage (ADR-018): the live bytes each address has moved, read from
//! conntrack's accounting while someone watches; each day's bytes per device,
//! read from nlbwmon; and the watchdog that moves aside a day nlbwmon can no
//! longer read, so one torn write does not stop the counting.

use serde_json::{json, Map, Value};
use std::collections::{HashMap, HashSet};
use std::fs;
use std::net::IpAddr;
use std::os::unix::net::UnixStream;
use std::path::Path;
use std::process::Command;
use std::sync::Mutex;
use std::time::Duration;

const CONNTRACK: &str = "/proc/net/nf_conntrack";
const NLBW_SOCKET: &str = "/var/run/nlbwmon.sock";
/// The days the shell may ask for at once: this month and the last, whole.
const WINDOW_DAYS: usize = 62;
/// At most this many addresses in one live reading: a roster's worth, bounded.
const MAX_ADDRESSES: usize = 1024;

/// One tracked connection: its key, and each direction's source with the bytes
/// it sent (none while the kernel keeps no accounting).
#[derive(Debug, PartialEq)]
struct Conn {
    key: String,
    orig_src: IpAddr,
    orig_bytes: Option<u64>,
    reply_src: IpAddr,
    reply_bytes: Option<u64>,
}

/// parse_line reads one `/proc/net/nf_conntrack` line. The original direction
/// comes first (`src=`, then its ports and counters), the reply after the second
/// `src=`. The key is the protocol and the original direction's identity, which
/// stay the same while the state, timeout and counters move.
fn parse_line(line: &str) -> Option<Conn> {
    let mut fields = line.split_whitespace();
    let family = fields.next()?;
    let protocol = fields.nth(1)?;
    let mut key = format!("{family} {protocol}");
    let (mut orig_src, mut reply_src) = (None, None);
    let (mut orig_bytes, mut reply_bytes) = (None, None);
    let mut direction = 0;
    for field in fields {
        let Some((name, value)) = field.split_once('=') else {
            continue;
        };
        match name {
            "src" => {
                direction += 1;
                let address = value.parse::<IpAddr>().ok()?;
                match direction {
                    1 => orig_src = Some(address),
                    2 => reply_src = Some(address),
                    _ => return None,
                }
            }
            "bytes" if direction == 1 => orig_bytes = value.parse().ok(),
            "bytes" if direction == 2 => reply_bytes = value.parse().ok(),
            "packets" | "bytes" => {}
            _ if direction == 1 || name == "zone" => {
                key.push(' ');
                key.push_str(field);
            }
            _ => {}
        }
    }
    Some(Conn {
        key,
        orig_src: orig_src?,
        orig_bytes,
        reply_src: reply_src?,
        reply_bytes,
    })
}

/// Live is the running byte totals per address since the helper started. They
/// only grow, so any reader turns two readings into a rate (ADR-018).
#[derive(Default)]
pub struct Live {
    /// Each connection's bytes at the last reading, original then reply.
    last: HashMap<String, (u64, u64)>,
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
        let text = fs::read_to_string(CONNTRACK).map_err(|e| format!("{CONNTRACK}: {e}"))?;
        let conns: Vec<Conn> = text.lines().filter_map(parse_line).collect();
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

/// days_to_read is today (the newest day nlbwmon lists, always read since it
/// still grows) and every earlier listed day inside the window the caller does
/// not already hold, newest first.
fn days_to_read(listed: &[String], known: &HashSet<String>, window: usize) -> Vec<String> {
    let mut days: Vec<&String> = listed.iter().filter(|d| civil_day(d).is_some()).collect();
    days.sort_by(|a, b| b.cmp(a));
    let Some(today) = days.first().map(|d| d.to_string()) else {
        return Vec::new();
    };
    let newest = civil_day(&today).unwrap_or_default();
    let mut out = vec![today.clone()];
    for day in days.into_iter().skip(1) {
        let age = newest - civil_day(day).unwrap_or_default();
        if age < window as i64 && !known.contains(day.as_str()) {
            out.push(day.clone());
        }
    }
    out
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
    let listed: Vec<String> = nlbw(&["-c", "list"])?.lines().map(str::to_string).collect();
    let known: HashSet<String> = known
        .split(',')
        .filter(|d| !d.is_empty())
        .map(str::to_string)
        .collect();
    let wanted = days_to_read(&listed, &known, WINDOW_DAYS);
    let mut out = Map::new();
    for day in &wanted {
        out.insert(
            day.clone(),
            Value::Object(parse_day(&nlbw(&["-c", "json", "-g", "mac", "-t", day])?)?),
        );
    }
    Ok(json!({"enabled": true, "today": wanted.first(), "days": out}))
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
    let uci = |option: &str| {
        Command::new("uci")
            .args(["-q", "get", &format!("nlbwmon.@nlbwmon[0].{option}")])
            .output()
            .ok()
            .map(|o| String::from_utf8_lossy(&o.stdout).trim().to_string())
            .unwrap_or_default()
    };
    let dir = uci("database_directory");
    let compressed = uci("database_compress") != "0";
    let stamp = Command::new("date").arg("+%Y%m%d").output().ok()?;
    let stamp = String::from_utf8_lossy(&stamp.stdout).trim().to_string();
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

    const OUTBOUND: &str = "ipv4     2 tcp      6 431999 ESTABLISHED src=192.168.77.20 dst=93.184.215.14 sport=51544 dport=443 packets=12 bytes=1500 src=93.184.215.14 dst=172.30.1.10 sport=443 dport=51544 packets=40 bytes=52000 [ASSURED] mark=0 zone=0 use=2";
    const FORWARDED: &str = "ipv4     2 tcp      6 300 ESTABLISHED src=203.0.113.9 dst=172.30.1.10 sport=40000 dport=8080 packets=3 bytes=300 src=192.168.77.20 dst=203.0.113.9 sport=80 dport=40000 packets=9 bytes=9000 [ASSURED] mark=0 zone=0 use=2";
    const V6: &str = "ipv6     10 udp      17 25 src=fd42:07ea:0001:0000:0000:0000:0000:0020 dst=2001:0db8:0000:0000:0000:0000:0000:0001 sport=5353 dport=53 packets=1 bytes=80 src=2001:0db8:0000:0000:0000:0000:0000:0001 dst=fd42:07ea:0001:0000:0000:0000:0000:0020 sport=53 dport=5353 packets=1 bytes=120 mark=0 zone=0 use=2";
    const UNACCOUNTED: &str = "ipv4     2 tcp      6 87 TIME_WAIT src=10.0.0.232 dst=172.20.0.10 sport=38694 dport=8443 src=172.20.0.10 dst=10.0.0.232 sport=8443 dport=38694 [ASSURED] mark=0 zone=0 use=2";

    fn ip(text: &str) -> IpAddr {
        text.parse().unwrap()
    }

    // A connection is its original source and the source of the reply, each
    // with the bytes it sent; the key names the connection whatever its
    // state or timeout reads now, so a later reading finds it again.
    #[test]
    fn a_conntrack_line_is_both_directions_and_what_each_sent() {
        let conn = parse_line(OUTBOUND).expect("parsed");
        assert_eq!(conn.orig_src, ip("192.168.77.20"));
        assert_eq!(conn.orig_bytes, Some(1500));
        assert_eq!(conn.reply_src, ip("93.184.215.14"));
        assert_eq!(conn.reply_bytes, Some(52000));
        let later = OUTBOUND
            .replace("431999 ESTABLISHED", "120 FIN_WAIT")
            .replace("bytes=52000", "bytes=99000");
        assert_eq!(parse_line(&later).expect("parsed").key, conn.key);

        let v6 = parse_line(V6).expect("parsed");
        assert_eq!(v6.orig_src, ip("fd42:7ea:1::20"));

        let unaccounted = parse_line(UNACCOUNTED).expect("parsed");
        assert_eq!(unaccounted.orig_bytes, None);
        assert_eq!(parse_line("garbage"), None);
    }

    // The running totals only grow: what a connection moved since the last
    // reading is added once, a connection first seen counts in full, and a
    // forwarded connection counts for the LAN host that answers it.
    #[test]
    fn totals_grow_by_what_is_new_and_credit_both_ends() {
        let mut live = Live::default();
        assert!(live.absorb(vec![
            parse_line(OUTBOUND).unwrap(),
            parse_line(FORWARDED).unwrap()
        ]));
        let host = ip("192.168.77.20");
        assert_eq!(live.totals[&host], (1500 + 9000, 52000 + 300));
        assert_eq!(live.totals[&ip("93.184.215.14")], (52000, 1500));

        let grown = OUTBOUND.replace("bytes=52000", "bytes=60000");
        live.absorb(vec![
            parse_line(&grown).unwrap(),
            parse_line(FORWARDED).unwrap(),
        ]);
        assert_eq!(live.totals[&host], (1500 + 9000, 60000 + 300));

        // Gone, then back with fewer bytes: a new connection on the same tuple.
        live.absorb(vec![]);
        let reused = OUTBOUND
            .replace("bytes=52000", "bytes=100")
            .replace("bytes=1500", "bytes=10");
        live.absorb(vec![parse_line(&reused).unwrap()]);
        assert_eq!(live.totals[&host], (1500 + 9000 + 10, 60000 + 300 + 100));

        // Without accounting nothing is counted, and the reading says so.
        let mut off = Live::default();
        assert!(!off.absorb(vec![parse_line(UNACCOUNTED).unwrap()]));
        assert!(off.totals.is_empty());
    }

    #[test]
    fn a_report_names_only_the_addresses_asked_for() {
        let mut live = Live::default();
        live.absorb(vec![parse_line(OUTBOUND).unwrap()]);
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

    // The newest listed day is today, always read; earlier days inside the
    // window are read once, then the shell keeps them.
    #[test]
    fn days_read_are_today_and_the_unknown_days_in_the_window() {
        let listed: Vec<String> = ["2026-10-05", "2026-10-07", "2026-08-01", "2026-10-06"]
            .iter()
            .map(|d| d.to_string())
            .collect();
        let known: HashSet<String> = ["2026-10-06".to_string(), "2026-10-07".to_string()].into();
        assert_eq!(
            days_to_read(&listed, &known, 3),
            vec!["2026-10-07", "2026-10-05"]
        );
        assert!(days_to_read(&[], &known, 3).is_empty());
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
