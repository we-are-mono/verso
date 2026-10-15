// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
//! What the router's tunnels are doing, for the VPN page: each OpenVPN
//! instance's profile read out, whether it runs and on which device, what
//! OpenVPN last reported through its hooks, and every tunnel device as the
//! kernel holds it. Profiles are read only under /etc/openvpn, their key blocks
//! are folded away, and a sign-in file is never opened: nothing secret leaves
//! the helper. No state is read from the log, which forgets.
use crate::netlink::{self, Link};
use crate::Failure;
use serde_json::{json, Map, Value};
use std::collections::{BTreeMap, HashMap, VecDeque};
use std::fs;
use std::io::Read;
use std::os::unix::fs::MetadataExt;
use std::path::Path;
use std::process::Command;

const PROFILES: &str = "/etc/openvpn";
/// Where the hotplug hook the verso package ships beside this helper
/// (/etc/hotplug.d/openvpn/50-verso) records each instance's last transition.
const RECORDS: &str = "/var/run/verso-openvpn";
const LIMIT: usize = 64 * 1024;
/// Link types the kernel gives a tunnel: none (WireGuard, tun), ipip, ip6tnl,
/// sit, gre, ip6gre.
const TUNNEL_TYPES: [u64; 6] = [65534, 768, 769, 776, 778, 823];
/// The kernel makes these for itself when a tunnel module loads.
const KERNEL_TUNNELS: [&str; 9] = [
    "sit0", "ip6tnl0", "tunl0", "gre0", "gretap0", "erspan0", "ip6gre0", "ip_vti0", "ip6_vti0",
];
/// What one read of OpenVPN's takes of a profile line: its option buffer
/// outside an inline block, its inline buffer inside one, each a C string.
/// A longer line is read as several, each parsed on its own.
const OPTION_READ: usize = 256;
const INLINE_READ: usize = 255;
/// OpenVPN's longest token; a longer one fails the line.
const TOKEN_LIMIT: usize = 256;
const BOM: &[u8] = b"\xef\xbb\xbf";

pub fn state() -> Result<Value, Failure> {
    let sections = uci_sections(&run("/sbin/uci", &["-q", "show", "openvpn"]));
    let running = running();
    let links = netlink::links();
    let mut devices = BTreeMap::new();
    let mut instances = Map::new();
    for (name, config) in &sections {
        let process = running.get(name);
        let record = process.and_then(|p| read_record(Path::new(RECORDS), name, p));
        // The device the process holds open, or, where its descriptors are
        // not ours to read (no CAP_SYS_PTRACE), the one OpenVPN named.
        let device = process
            .and_then(|p| p.device.clone())
            .or_else(|| record.as_ref().map(|r| r.device.clone()).filter(|d| !d.is_empty()));
        let link = device.as_ref().and_then(|d| links.get(d));
        let mut instance = instance_state(process.is_some(), record.as_ref(), link);
        instance.insert("config".into(), json!(config));
        instance.insert("running".into(), json!(process.is_some()));
        if let Some(device) = device {
            devices.insert(device.clone(), name.clone());
            instance.insert("device".into(), json!(device));
        }
        if let Some(text) = read_profile(config) {
            instance.insert("profile".into(), profile(&text));
        }
        instances.insert(name.clone(), Value::Object(instance));
    }
    Ok(json!({"instances": instances, "tunnels": tunnels(&devices, &links)}))
}

fn run(program: &str, args: &[&str]) -> String {
    Command::new(program)
        .args(args)
        .output()
        .map(|o| String::from_utf8_lossy(&o.stdout).into_owned())
        .unwrap_or_default()
}

/// uci_sections is each `openvpn` section and the profile it names, from
/// `uci show openvpn`. A section with no profile file is left out: its options
/// live in uci, where the page reads them itself.
fn uci_sections(show: &str) -> BTreeMap<String, String> {
    let mut kinds = BTreeMap::new();
    let mut configs = BTreeMap::new();
    for line in show.lines() {
        let Some((key, value)) = line.split_once('=') else {
            continue;
        };
        let value = value.trim_matches('\'');
        let mut parts = key.splitn(3, '.');
        let (Some("openvpn"), Some(section)) = (parts.next(), parts.next()) else {
            continue;
        };
        match parts.next() {
            None => {
                kinds.insert(section.to_string(), value.to_string());
            }
            Some("config") => {
                configs.insert(section.to_string(), value.to_string());
            }
            _ => {}
        }
    }
    configs
        .into_iter()
        .filter(|(section, _)| kinds.get(section).is_some_and(|k| k == "openvpn"))
        .collect()
}

/// read_profile reads a profile only from under /etc/openvpn, after resolving
/// links, so a section cannot point the helper at any other root file.
fn read_profile(path: &str) -> Option<String> {
    let resolved = fs::canonicalize(path).ok()?;
    if !resolved.starts_with(PROFILES) {
        return None;
    }
    let file = fs::File::open(&resolved).ok()?;
    if !file.metadata().ok()?.is_file() {
        return None;
    }
    let mut text = String::new();
    file.take(LIMIT as u64 + 1).read_to_string(&mut text).ok()?;
    (text.len() <= LIMIT).then_some(text)
}

/// fold is a profile as it may be shown: every inline block one line, its
/// name and its length, so no key in it leaves the helper. A block left open
/// at the end of the file is folded too.
pub fn fold(text: &str) -> String {
    let lines: Vec<&str> = text.lines().collect();
    let mut shown = Vec::new();
    let mut at = 0;
    for span in spans(text) {
        shown.extend(lines[at..span.start].iter().map(|l| l.to_string()));
        shown.push(span.placeholder());
        at = span.end;
    }
    shown.extend(lines[at..].iter().map(|l| l.to_string()));
    let mut out = shown.join("\n");
    if text.ends_with('\n') {
        out.push('\n');
    }
    out
}

/// unfold is an edited profile with the keys put back: each placeholder line
/// left as fold wrote it takes the original block it stood for, in order. A
/// block pasted in whole stays as pasted; a placeholder for a block the
/// original never had stays a line OpenVPN will refuse. None is an edit that
/// puts a line of a block back where more of it leaves the helper than did
/// in the original (exposure): where it stands, OpenVPN no longer reads the
/// block inline (an opener moved off the first line loses its BOM's pass, or
/// a block the editor opened closes inside it), so fold would show it, or it
/// sits among a <connection> block's options, read out with them.
pub fn unfold(edited: &str, original: &str) -> Option<String> {
    let lines: Vec<&str> = original.lines().collect();
    // How far each original line leaves, read from its lines as they go back.
    let mut rejoined = lines.join("\n");
    if original.ends_with('\n') {
        rejoined.push('\n');
    }
    let was = exposure(&rejoined);
    // Each placeholder's blocks, in the order they stood.
    let mut blocks: HashMap<String, VecDeque<(usize, usize)>> = HashMap::new();
    for span in spans(original) {
        blocks.entry(span.placeholder()).or_default().push_back((span.start, span.end));
    }
    let mut out = Vec::new();
    // Each restored line: where it lands in out, and how far it left before.
    let mut restored = Vec::new();
    for line in edited.lines() {
        match blocks.get_mut(line.trim()).and_then(VecDeque::pop_front) {
            Some((start, end)) => {
                restored.extend((start..end).map(|from| (out.len() + from - start, was[from])));
                out.extend_from_slice(&lines[start..end]);
            }
            None => out.push(line),
        }
    }
    let mut out = out.join("\n");
    if edited.ends_with('\n') {
        out.push('\n');
    }
    // A blank last line the join leaves out holds nothing to show.
    let now = exposure(&out);
    restored
        .iter()
        .all(|(at, was)| now.get(*at).copied().unwrap_or(Exposure::Hidden) <= *was)
        .then_some(out)
}

/// Exposure is how much of a profile line leaves the helper, least first.
#[derive(Clone, Copy, Debug, PartialEq, PartialOrd)]
enum Exposure {
    /// Folded away inside an inline block.
    Hidden,
    /// One of a <connection> block's own options: profile reads out the
    /// values of its remote and proto lines alone (connection_options).
    ReadOut,
    /// Outside every inline block, shown as it is.
    Shown,
}

/// exposure is, for each of a profile's lines, how much of it leaves the
/// helper.
fn exposure(text: &str) -> Vec<Exposure> {
    let lines: Vec<&str> = text.lines().collect();
    let raw: Vec<&str> = text.split_inclusive('\n').collect();
    let mut out = vec![Exposure::Shown; raw.len()];
    for span in spans(text) {
        out[span.start..span.end].fill(Exposure::Hidden);
        for at in connection_options(&raw, &lines, &span).unwrap_or_default() {
            out[at] = Exposure::ReadOut;
        }
    }
    out
}

/// Span is a run of a profile's lines OpenVPN reads as inline content: the
/// line that opens a block through the line that closes it, by index into
/// the profile's lines. A line OpenVPN reads in several pieces can close one
/// block and open the next, so a span names every block it holds.
struct Span {
    start: usize,
    end: usize,
    names: Vec<String>,
    closed: bool,
}

impl Span {
    /// placeholder is the one line a span is shown as: its first block's name
    /// and the lines between its tags.
    fn placeholder(&self) -> String {
        let tags = 1 + usize::from(self.closed);
        let lines = (self.end - self.start).saturating_sub(tags);
        let unit = if lines == 1 { "line" } else { "lines" };
        let name = &self.names[0];
        format!("<{name}> … {lines} {unit} … </{name}>")
    }
}

/// spans is every inline block in a profile, found as OpenVPN's
/// read_config_file finds them, so whatever OpenVPN reads inline is folded,
/// whatever its name: a read whose only token is `<name>` (a leading `--`
/// dropped) opens one, and the first read inside it that starts with
/// `</name>`, past C whitespace, closes it.
fn spans(text: &str) -> Vec<Span> {
    read_spans(text, true)
}

/// read_spans is the inline blocks in text read as OpenVPN reads it, `bom`
/// whether a BOM before the first line is dropped: read_config_file drops
/// one from a profile's first line, and a <connection> block's content,
/// read apart from the profile's lines, has none dropped.
fn read_spans(text: &str, bom: bool) -> Vec<Span> {
    let mut spans = Vec::new();
    let mut current: Option<Span> = None;
    let mut closing: Option<Vec<u8>> = None;
    let mut first = bom;
    for (at, line) in text.split_inclusive('\n').enumerate() {
        let mut rest = line.as_bytes();
        while !rest.is_empty() {
            let size = if closing.is_some() { INLINE_READ } else { OPTION_READ };
            let (read, tail) = rest.split_at(rest.len().min(size));
            rest = tail;
            match &closing {
                Some(tag) => {
                    if read[read.iter().take_while(|b| space(**b)).count()..].starts_with(tag) {
                        closing = None;
                    }
                }
                None => {
                    let read = if first { read.strip_prefix(BOM).unwrap_or(read) } else { read };
                    if let Some(name) = opens(read) {
                        closing = Some(format!("</{name}>").into_bytes());
                        let span = current.get_or_insert(Span { start: at, end: at, names: Vec::new(), closed: false });
                        span.names.push(name);
                    }
                }
            }
            first = false;
        }
        if let Some(span) = current.as_mut() {
            span.end = at + 1;
        }
        if closing.is_none() {
            if let Some(span) = current.take() {
                spans.push(Span { closed: true, ..span });
            }
        }
    }
    spans.extend(current);
    spans
}

/// opens is the block one read of a profile opens, if any: its only token,
/// a leading `--` dropped as OpenVPN drops it, is `<name>`.
fn opens(read: &[u8]) -> Option<String> {
    let [token] = tokens(read)?.try_into().ok()?;
    let token = match token.strip_prefix(b"--") {
        Some(rest) if token.len() >= 3 => rest,
        _ => &token[..],
    };
    let name = token.strip_prefix(b"<")?.strip_suffix(b">")?;
    Some(String::from_utf8_lossy(name).into_owned())
}

/// tokens is one read of a profile split as OpenVPN's parse_line splits it:
/// C whitespace separates, double and single quotes group, a backslash
/// escapes outside single quotes, `;` or `#` where a token would start ends
/// the line, and the string ends at a NUL. None is a line parse_line refuses
/// (an unclosed quote, a stray backslash, an overlong token).
fn tokens(read: &[u8]) -> Option<Vec<Vec<u8>>> {
    #[derive(PartialEq)]
    enum State {
        Between,
        Bare,
        Quoted,
        SingleQuoted,
    }
    // parse_line's own space(), which counts the NUL that ends the string.
    let blank = |c: u8| c == 0 || space(c);
    let end = read.iter().position(|b| *b == 0).unwrap_or(read.len());
    let mut out = Vec::new();
    let mut token = Vec::new();
    let mut state = State::Between;
    let mut backslash = false;
    // parse_line reads that NUL too, which ends a bare token.
    for c in read[..end].iter().copied().chain([0]) {
        if !backslash && c == b'\\' && state != State::SingleQuoted {
            backslash = true;
            continue;
        }
        let mut kept = None;
        match state {
            State::Between if !blank(c) => {
                if c == b';' || c == b'#' {
                    break;
                }
                if !backslash && c == b'"' {
                    state = State::Quoted;
                } else if !backslash && c == b'\'' {
                    state = State::SingleQuoted;
                } else {
                    kept = Some(c);
                    state = State::Bare;
                }
            }
            State::Between => {}
            State::Bare if !backslash && blank(c) => {
                out.push(std::mem::take(&mut token));
                state = State::Between;
            }
            State::Quoted if !backslash && c == b'"' => {
                out.push(std::mem::take(&mut token));
                state = State::Between;
            }
            State::SingleQuoted if c == b'\'' => {
                out.push(std::mem::take(&mut token));
                state = State::Between;
            }
            State::Bare | State::Quoted | State::SingleQuoted => kept = Some(c),
        }
        let kept = kept.filter(|c| *c != 0);
        if let Some(c) = kept {
            if backslash && !(c == b'\\' || c == b'"' || blank(c)) {
                return None;
            }
            if token.len() >= TOKEN_LIMIT {
                return None;
            }
            token.push(c);
        }
        backslash = false;
    }
    (state == State::Between).then_some(out)
}

/// space is whitespace as OpenVPN reads a profile: C's isspace, never
/// Unicode's, so a non-breaking space neither splits a tag from its line nor
/// closes a block before OpenVPN does.
fn space(c: u8) -> bool {
    matches!(c, b' ' | b'\t' | b'\n' | b'\x0b' | b'\x0c' | b'\r')
}

/// profile reads what a profile says, with every inline block folded to its
/// name and length, so the text can be shown and nothing in it is a secret.
/// The facts come from the lines outside the blocks, and the servers also
/// from each <connection> block's own options (connection_options).
fn profile(text: &str) -> Value {
    let mut remotes = Vec::new();
    let mut facts = Map::new();
    let mut proto = String::new();
    let mut connection_proto = String::new();
    let spans = spans(text);
    let blocks: Vec<&String> = spans.iter().flat_map(|s| &s.names).collect();
    let lines: Vec<&str> = text.lines().collect();
    let raw: Vec<&str> = text.split_inclusive('\n').collect();
    for span in &spans {
        let Some(options) = connection_options(&raw, &lines, span) else {
            continue;
        };
        let mut own = String::new();
        let mut found = Vec::new();
        for (directive, rest) in options.iter().filter_map(|at| words(lines[*at])) {
            match directive {
                "proto" => own = rest.first().unwrap_or(&"").to_string(),
                "remote" => found.extend(remote(&rest)),
                _ => {}
            }
        }
        for remote in &mut found {
            if remote["proto"] == "" {
                remote["proto"] = json!(own);
            }
        }
        if connection_proto.is_empty() {
            connection_proto = own;
        }
        remotes.push((span.start, found));
    }
    for (at, line) in lines.iter().enumerate() {
        if spans.iter().any(|s| (s.start..s.end).contains(&at)) {
            continue;
        }
        let Some((directive, rest)) = words(line) else {
            continue;
        };
        match directive {
            "client" | "tls-client" => {
                facts.insert("client".into(), json!(true));
            }
            "dev" => {
                facts.insert("dev".into(), json!(rest.first().unwrap_or(&"")));
            }
            "proto" => proto = rest.first().unwrap_or(&"").to_string(),
            "remote" => remotes.push((at, remote(&rest).into_iter().collect())),
            "remote-random" => {
                facts.insert("random".into(), json!(true));
            }
            "data-ciphers" | "cipher" => {
                facts
                    .entry("cipher")
                    .or_insert_with(|| json!(rest.first().unwrap_or(&"")));
            }
            "auth-user-pass" => {
                // Whether the profile asks for a sign-in, and where it keeps
                // one. The file itself is never read.
                facts.insert("auth".into(), json!(rest.first().unwrap_or(&"")));
                facts.insert("asks_sign_in".into(), json!(true));
            }
            "remote-cert-tls" => {
                facts.insert("verify".into(), json!(rest.first().unwrap_or(&"")));
            }
            "redirect-gateway" => {
                facts.insert("redirect".into(), json!(true));
            }
            "route-nopull" => {
                facts.insert("nopull".into(), json!(true));
            }
            _ => {}
        }
    }
    if blocks.iter().any(|b| *b == "auth-user-pass") {
        facts.insert("asks_sign_in".into(), json!(false));
    }
    if proto.is_empty() {
        proto = connection_proto;
    }
    // Servers in the order the profile names them.
    remotes.sort_by_key(|(at, _)| *at);
    let remotes: Vec<Value> = remotes.into_iter().flat_map(|(_, found)| found).collect();
    facts.insert("proto".into(), json!(proto));
    facts.insert("remotes".into(), json!(remotes));
    facts.insert("blocks".into(), json!(blocks));
    facts.insert("text".into(), json!(fold(text)));
    Value::Object(facts)
}

/// words is a profile line's directive and its arguments, as profile reads
/// them; None for a comment or a blank line.
fn words(line: &str) -> Option<(&str, Vec<&str>)> {
    let trimmed = line.trim();
    if trimmed.starts_with('#') || trimmed.starts_with(';') {
        return None;
    }
    let mut words = trimmed.split_whitespace();
    Some((words.next()?, words.collect()))
}

/// remote is the server a `remote` line names.
fn remote(rest: &[&str]) -> Option<Value> {
    let host = rest.first()?;
    Some(json!({
        "host": host,
        "port": rest.get(1).unwrap_or(&""),
        "proto": rest.get(2).unwrap_or(&""),
    }))
}

/// connection_options is the option lines of a span that is one closed
/// <connection> block, by index into the profile's lines, which OpenVPN
/// reads as options of their own, with any block inline in them (a
/// tls-crypt key, say) left out. OpenVPN reads that content apart from the
/// profile's lines, so a span with a line it would read in pieces, or a NUL
/// that ends its reading, is not read at all.
fn connection_options(raw: &[&str], lines: &[&str], span: &Span) -> Option<Vec<usize>> {
    if !span.closed || span.names != ["connection"] {
        return None;
    }
    if raw[span.start..span.end].iter().any(|l| l.len() >= INLINE_READ || l.contains('\0')) {
        return None;
    }
    let inner = span.start + 1..span.end - 1;
    let mut nested = vec![false; inner.len()];
    for block in read_spans(&lines[inner.clone()].join("\n"), false) {
        nested[block.start..block.end].fill(true);
    }
    Some(inner.filter(|at| !nested[at - span.start - 1]).collect())
}

/// Record is what the hotplug hook last wrote for an instance: OpenVPN's own
/// word on it (connected at route-up, connecting from up or route-pre-down,
/// stopped at down), since when it connected, its tunnel address, the server
/// it reached and the device it named, from the OpenVPN process that wrote it.
#[derive(Debug, Default, PartialEq)]
struct Record {
    pid: u32,
    state: String,
    since: Option<u64>,
    address: String,
    server: String,
    device: String,
}

fn parse_record(text: &str) -> Record {
    let mut record = Record::default();
    for (key, value) in text.lines().filter_map(|l| l.split_once('=')) {
        match key {
            "pid" => record.pid = value.parse().unwrap_or(0),
            "state" => record.state = value.to_string(),
            "since" => record.since = value.parse().ok(),
            "address" => record.address = value.to_string(),
            "server" => record.server = value.to_string(),
            "device" => record.device = value.to_string(),
            _ => {}
        }
    }
    record
}

/// read_record is an instance's record, trusted only as the running process's
/// own: a file the process's user wrote (anyone may write in the directory, no
/// one may replace another's) carrying that process's PID, so a record from
/// an earlier run, or one planted by someone else, is not read as this one.
fn read_record(dir: &Path, name: &str, process: &Process) -> Option<Record> {
    if name.is_empty() || !name.bytes().all(|b| b.is_ascii_alphanumeric() || b == b'_' || b == b'-') {
        return None;
    }
    let path = dir.join(name);
    let meta = fs::symlink_metadata(&path).ok()?;
    if !meta.is_file() || meta.uid() != process.uid {
        return None;
    }
    let record = parse_record(&fs::read_to_string(path).ok()?);
    (record.pid == process.pid).then_some(record)
}

/// instance_state is what the page is told of an instance: what OpenVPN last
/// reported through its hooks; or, where it reported nothing this run (the
/// hook arrived after the tunnel came up, or the profile turns scripts off),
/// what the kernel proves — its device up with an address ("up"), or not yet.
/// The address is the device's while it has one.
fn instance_state(running: bool, record: Option<&Record>, link: Option<&Link>) -> Map<String, Value> {
    let mut out = Map::new();
    let ipv4 = link
        .filter(|l| l.up && l.carrier)
        .and_then(|l| l.addresses.iter().find(|a| !a.contains(':')))
        .cloned();
    let state = match (running, record) {
        (false, _) => "stopped".to_string(),
        (true, Some(record)) => record.state.clone(),
        (true, None) if ipv4.is_some() => "up".to_string(),
        (true, None) => "connecting".to_string(),
    };
    if let Some(record) = record.filter(|_| running) {
        if !record.server.is_empty() {
            out.insert("server".into(), json!(record.server));
        }
        if let (Some(since), "connected") = (record.since, state.as_str()) {
            out.insert("since".into(), json!(since));
        }
    }
    let address = ipv4.or_else(|| record.filter(|r| running && !r.address.is_empty()).map(|r| r.address.clone()));
    if let Some(address) = address {
        out.insert("address".into(), json!(address));
    }
    out.insert("state".into(), json!(state));
    out
}

struct Process {
    pid: u32,
    /// The effective user it runs as, which a profile's `user` changes.
    uid: u32,
    device: Option<String>,
}

/// running is each OpenVPN instance with a process, by the name OpenWrt's init
/// passes it (`--syslog openvpn(<name>)`), and the tun device it holds open.
fn running() -> BTreeMap<String, Process> {
    let mut out = BTreeMap::new();
    let Ok(entries) = fs::read_dir("/proc") else {
        return out;
    };
    for entry in entries.flatten() {
        let pid = entry.file_name();
        let Some(pid) = pid.to_str().filter(|p| p.bytes().all(|b| b.is_ascii_digit())) else {
            continue;
        };
        let Ok(cmdline) = fs::read(format!("/proc/{pid}/cmdline")) else {
            continue;
        };
        if let Some(name) = instance_name(&cmdline) {
            let uid = fs::read_to_string(format!("/proc/{pid}/status"))
                .ok()
                .and_then(|s| effective_uid(&s))
                .unwrap_or(u32::MAX);
            out.insert(name, Process { pid: pid.parse().unwrap_or(0), uid, device: tun_device(pid) });
        }
    }
    out
}

/// effective_uid is the second field of a process status's `Uid:` line.
fn effective_uid(status: &str) -> Option<u32> {
    let line = status.lines().find_map(|l| l.strip_prefix("Uid:"))?;
    line.split_whitespace().nth(1)?.parse().ok()
}

fn instance_name(cmdline: &[u8]) -> Option<String> {
    let args: Vec<&[u8]> = cmdline.split(|b| *b == 0).collect();
    if !args.first()?.ends_with(b"openvpn") {
        return None;
    }
    let at = args.iter().position(|a| *a == b"--syslog")?;
    let tag = std::str::from_utf8(args.get(at + 1)?).ok()?;
    tag.strip_prefix("openvpn(")?.strip_suffix(')').map(str::to_string)
}

/// tun_device is the device a process's tun file is bound to, which the kernel
/// writes into that file's fdinfo as `iff:`.
fn tun_device(pid: &str) -> Option<String> {
    for entry in fs::read_dir(format!("/proc/{pid}/fd")).ok()?.flatten() {
        let target = fs::read_link(entry.path()).ok()?;
        if target != Path::new("/dev/net/tun") {
            continue;
        }
        let info = fs::read_to_string(format!(
            "/proc/{pid}/fdinfo/{}",
            entry.file_name().to_string_lossy()
        ))
        .ok()?;
        return info
            .lines()
            .find_map(|l| l.strip_prefix("iff:"))
            .map(|d| d.trim().to_string());
    }
    None
}

/// tunnels is every tunnel device someone brought up, as the kernel holds it:
/// its kind, which OpenVPN instance holds it, whether it carries (up, and for
/// a tun device a process holding it), its counters and addresses.
fn tunnels(devices: &BTreeMap<String, String>, links: &BTreeMap<String, Link>) -> Value {
    let mut out = Vec::new();
    for (name, link) in links {
        let tun = link.kind == "tun";
        if KERNEL_TUNNELS.contains(&name.as_str()) || !(tun || TUNNEL_TYPES.contains(&u64::from(link.link_type))) {
            continue;
        }
        let kind = if devices.contains_key(name) {
            "openvpn"
        } else if link.kind == "wireguard" {
            "wireguard"
        } else if name.starts_with("tailscale") {
            "tailscale"
        } else {
            "tunnel"
        };
        out.push(json!({
            "device": name,
            "kind": kind,
            "instance": devices.get(name),
            "up": link.up && link.carrier,
            "rx": link.rx,
            "tx": link.tx,
            "addresses": link.addresses,
        }));
    }
    json!(out)
}

#[cfg(test)]
mod tests {
    use super::*;

    const PROTON: &str = "client\ndev tun\nproto udp\n\nremote 185.107.56.234 1194\nremote 185.107.56.234 80\nremote-random\nresolv-retry infinite\ndata-ciphers AES-256-GCM\nremote-cert-tls server\nauth-user-pass\nredirect-gateway def1\n<ca>\n-----BEGIN CERTIFICATE-----\nMIIF\n-----END CERTIFICATE-----\n</ca>\n<tls-crypt>\nsecret line one\nsecret line two\n</tls-crypt>\n";

    #[test]
    fn a_profile_reads_out_and_folds_every_key_block() {
        let p = profile(PROTON);
        assert_eq!(p["client"], true);
        assert_eq!(p["dev"], "tun");
        assert_eq!(p["proto"], "udp");
        assert_eq!(p["remotes"].as_array().unwrap().len(), 2);
        assert_eq!(p["remotes"][0]["host"], "185.107.56.234");
        assert_eq!(p["remotes"][0]["port"], "1194");
        assert_eq!(p["random"], true);
        assert_eq!(p["cipher"], "AES-256-GCM");
        assert_eq!(p["verify"], "server");
        assert_eq!(p["asks_sign_in"], true);
        assert_eq!(p["redirect"], true);
        assert_eq!(p["blocks"], json!(["ca", "tls-crypt"]));
        let text = p["text"].as_str().unwrap();
        assert!(text.contains("<ca> … 3 lines … </ca>"), "{text}");
        assert!(text.contains("<tls-crypt> … 2 lines … </tls-crypt>"), "{text}");
        assert!(!text.contains("secret line"), "a key block leaked: {text}");
        assert!(!text.contains("MIIF"), "a certificate block leaked: {text}");
    }

    #[test]
    fn a_profile_that_carries_its_own_sign_in_does_not_ask_for_one() {
        let p = profile("client\nauth-user-pass\n<auth-user-pass>\nme\nhunter2\n</auth-user-pass>\n");
        assert_eq!(p["asks_sign_in"], false);
        assert!(!p["text"].as_str().unwrap().contains("hunter2"));
    }

    #[test]
    fn uci_names_each_instance_and_its_profile() {
        let show = "openvpn.proton=openvpn\nopenvpn.proton.enabled='1'\nopenvpn.proton.config='/etc/openvpn/proton.ovpn'\nopenvpn.inline=openvpn\nopenvpn.inline.dev='tun'\n";
        let got = uci_sections(show);
        assert_eq!(got.len(), 1);
        assert_eq!(got["proton"], "/etc/openvpn/proton.ovpn");
    }

    #[test]
    fn a_profile_outside_etc_openvpn_is_never_read() {
        assert_eq!(read_profile("/etc/shadow"), None);
        assert_eq!(read_profile("/etc/openvpn/../shadow"), None);
    }

    fn tun(addresses: &[&str]) -> Link {
        Link {
            name: "tun0".into(),
            link_type: 65534,
            kind: "tun".into(),
            up: true,
            carrier: true,
            addresses: addresses.iter().map(|a| a.to_string()).collect(),
            ..Link::default()
        }
    }

    fn connected() -> Record {
        Record {
            pid: 812,
            state: "connected".into(),
            since: Some(1791295321),
            address: "10.96.0.14".into(),
            server: "185.107.56.234:1194".into(),
            device: "tun0".into(),
        }
    }

    #[test]
    fn a_record_reads_back_what_the_hook_wrote() {
        let text = "pid=812\nstate=connected\nsince=1791295321\naddress=10.96.0.14\nserver=185.107.56.234:1194\ndevice=tun0\n";
        assert_eq!(parse_record(text), connected());
    }

    #[test]
    fn what_openvpn_reported_is_the_state_with_the_devices_address() {
        let s = instance_state(true, Some(&connected()), Some(&tun(&["10.96.0.14/16", "fe80::1/64"])));
        assert_eq!(s["state"], "connected");
        assert_eq!(s["since"], 1791295321);
        assert_eq!(s["server"], "185.107.56.234:1194");
        assert_eq!(s["address"], "10.96.0.14/16");
    }

    /// Under persist-tun the device keeps its address while OpenVPN
    /// reconnects; the hook's word wins over what the device still shows.
    #[test]
    fn a_reconnect_reads_as_connecting_though_the_device_holds_on() {
        let record = Record { state: "connecting".into(), ..connected() };
        let s = instance_state(true, Some(&record), Some(&tun(&["10.96.0.14/16"])));
        assert_eq!(s["state"], "connecting");
        assert!(s.get("since").is_none());
    }

    /// Nothing reported this run: the kernel's proof is all there is.
    #[test]
    fn with_no_record_the_device_says_up_or_not_yet() {
        let up = instance_state(true, None, Some(&tun(&["10.96.0.112/16"])));
        assert_eq!(up["state"], "up");
        assert_eq!(up["address"], "10.96.0.112/16");
        assert_eq!(instance_state(true, None, Some(&tun(&[])))["state"], "connecting");
        let unplugged = Link { carrier: false, ..tun(&["10.96.0.112/16"]) };
        assert_eq!(instance_state(true, None, Some(&unplugged))["state"], "connecting");
        assert_eq!(instance_state(true, None, None)["state"], "connecting");
    }

    #[test]
    fn an_instance_with_no_process_is_stopped_whatever_was_recorded() {
        let s = instance_state(false, Some(&connected()), None);
        assert_eq!(s["state"], "stopped");
        assert!(s.get("server").is_none() && s.get("address").is_none());
    }

    fn scratch(name: &str) -> std::path::PathBuf {
        let dir = std::env::temp_dir().join(format!("verso-openvpn-{name}-{}", std::process::id()));
        let _ = fs::remove_dir_all(&dir);
        fs::create_dir_all(&dir).expect("scratch dir");
        dir
    }

    fn me(pid: u32) -> Process {
        let uid = effective_uid(&fs::read_to_string("/proc/self/status").expect("status")).expect("uid");
        Process { pid, uid, device: None }
    }

    #[test]
    fn a_record_is_trusted_only_as_the_running_processs_own() {
        let dir = scratch("trust");
        fs::write(dir.join("proton"), "pid=812\nstate=connected\n").expect("record");
        assert_eq!(read_record(&dir, "proton", &me(812)).map(|r| r.state), Some("connected".into()));
        // an earlier run's record
        assert_eq!(read_record(&dir, "proton", &me(900)), None);
        // a record some other user wrote
        assert_eq!(read_record(&dir, "proton", &Process { uid: u32::MAX - 1, ..me(812) }), None);
        // a name that would leave the directory
        assert_eq!(read_record(&dir, "../proton", &me(812)), None);
        // a link to somewhere else
        std::os::unix::fs::symlink(dir.join("proton"), dir.join("other")).expect("symlink");
        assert_eq!(read_record(&dir, "other", &me(812)), None);
        let _ = fs::remove_dir_all(&dir);
    }

    #[test]
    fn effective_uid_is_the_second_field() {
        assert_eq!(effective_uid("Name:\topenvpn\nUid:\t0\t65534\t65534\t65534\n"), Some(65534));
    }

    /// The hook itself, run as OpenWrt's hotplug-call runs it, against a
    /// scratch directory: route-up records the session, ipchange keeps it,
    /// route-pre-down turns it to connecting, and the record carries the PID
    /// of the process that ran it (this test, standing in for OpenVPN).
    #[test]
    fn the_hotplug_hook_records_each_transition() {
        // The records stand one level inside this test's own directory, so a
        // name that climbs out of them lands where only this test writes.
        let root = scratch("hook");
        let dir = root.join("records");
        fs::create_dir_all(&dir).expect("record dir");
        let hook = fs::read_to_string("../docker/rootfs/etc/hotplug.d/openvpn/50-verso")
            .expect("hook")
            .replace("/var/run/verso-openvpn", dir.to_str().expect("utf-8"));
        let script = root.join("hook.sh");
        fs::write(&script, hook).expect("script");
        let run = |action: &str, env: &[(&str, &str)]| {
            let status = Command::new("sh")
                .arg(&script)
                .env("ACTION", action)
                .env("INSTANCE", "proton")
                .envs(env.iter().copied())
                .status()
                .expect("sh");
            assert!(status.success());
            parse_record(&fs::read_to_string(dir.join("proton")).expect("record"))
        };
        let up = run(
            "route-up",
            &[("dev", "tun0"), ("ifconfig_local", "10.96.0.112"), ("trusted_ip", "79.127.144.158"), ("trusted_port", "5060")],
        );
        assert_eq!(up.pid, std::process::id());
        assert_eq!((up.state.as_str(), up.address.as_str(), up.server.as_str()), ("connected", "10.96.0.112", "79.127.144.158:5060"));
        assert_eq!(up.device, "tun0");
        assert!(up.since.is_some());
        let moved = run("ipchange", &[("trusted_ip", "79.127.144.159"), ("trusted_port", "5060")]);
        assert_eq!((moved.state.as_str(), moved.server.as_str(), moved.since), ("connected", "79.127.144.159:5060", up.since));
        let going = run("route-pre-down", &[]);
        assert_eq!((going.state.as_str(), going.address.as_str()), ("connecting", "10.96.0.112"));
        assert_eq!(run("down", &[]).state, "stopped");
        // a name the hook will not write under
        Command::new("sh").arg(&script).env("ACTION", "route-up").env("INSTANCE", "../x").status().expect("sh");
        assert!(!root.join("x").exists());
        let _ = fs::remove_dir_all(&root);
    }

    #[test]
    fn the_init_names_the_instance_on_the_command_line() {
        let cmdline = b"/usr/sbin/openvpn\0--syslog\0openvpn(proton)\0--status\0/var/run/openvpn.proton.status\0--cd\0/etc/openvpn\0--config\0proton.ovpn\0";
        assert_eq!(instance_name(cmdline).as_deref(), Some("proton"));
        assert_eq!(instance_name(b"/usr/sbin/dnsmasq\0--syslog\0openvpn(x)\0"), None);
    }
}

#[cfg(test)]
mod folding {
    use super::*;

    const PROFILE: &str = "client\nremote a 1194\n<ca>\nCERT1\nCERT2\n</ca>\n<tls-crypt>\nKEY\n</tls-crypt>\n";

    #[test]
    fn a_folded_profile_shows_no_key() {
        let shown = fold(PROFILE);
        assert_eq!(
            shown,
            "client\nremote a 1194\n<ca> … 2 lines … </ca>\n<tls-crypt> … 1 line … </tls-crypt>\n"
        );
        assert_eq!(
            fold("client\n<key>\nSECRET\n"),
            "client\n<key> … 1 line … </key>\n"
        );
    }

    #[test]
    fn an_edit_of_the_directives_keeps_every_key() {
        let edited = fold(PROFILE).replace("remote a 1194", "remote b 443");
        assert_eq!(unfold(&edited, PROFILE), Some(PROFILE.replace("remote a 1194", "remote b 443")));
    }

    #[test]
    fn a_block_pasted_whole_replaces_the_old_one() {
        let edited = fold(PROFILE).replace(
            "<tls-crypt> … 1 line … </tls-crypt>",
            "<tls-crypt>\nNEWKEY\n</tls-crypt>",
        );
        let saved = unfold(&edited, PROFILE).unwrap();
        assert!(saved.contains("NEWKEY") && !saved.contains("\nKEY\n"), "{saved}");
        assert!(saved.contains("CERT1"), "{saved}");
    }

    #[test]
    fn a_placeholder_taken_out_takes_its_key_with_it() {
        let edited = fold(PROFILE).replace("<ca> … 2 lines … </ca>\n", "");
        assert!(!unfold(&edited, PROFILE).unwrap().contains("CERT1"));
    }

    // Blocks fold shows alike take their keys back in the order they stood.
    #[test]
    fn placeholders_alike_take_their_blocks_in_order() {
        let original = "client\n<ca>\nFIRST\n</ca>\n<ca>\nSECOND\n</ca>\n";
        let shown = fold(original);
        assert_eq!(shown, "client\n<ca> … 1 line … </ca>\n<ca> … 1 line … </ca>\n");
        assert_eq!(
            unfold(&shown.replace("client\n", "client\nverb 3\n"), original),
            Some(original.replace("client\n", "client\nverb 3\n"))
        );
        assert_eq!(
            unfold("client\n<ca> … 1 line … </ca>\n", original),
            Some("client\n<ca>\nFIRST\n</ca>\n".to_owned())
        );
    }

    /// Every way OpenVPN 2.7 opens an inline block: any option's name, the
    /// line's only token once quotes, a trailing comment and a leading `--`
    /// are read as OpenVPN reads them.
    const OPENED: [(&str, &str); 8] = [
        (
            "http-proxy-user-pass",
            "client\nhttp-proxy proxy.example 3128\n<http-proxy-user-pass>\nproxyuser\nSECRET\n</http-proxy-user-pass>\nverb 3\n",
        ),
        (
            "auth-gen-token-secret",
            "server 10.8.0.0 255.255.255.0\nauth-gen-token\n<auth-gen-token-secret>\n-----BEGIN OpenVPN auth-token server key-----\nSECRET\n-----END OpenVPN auth-token server key-----\n</auth-gen-token-secret>\nverb 3\n",
        ),
        ("key", "client\n<key> # client key\nSECRET\n</key>\nverb 3\n"),
        ("tls-crypt", "client\n\"<tls-crypt>\"\nSECRET\n</tls-crypt>\nverb 3\n"),
        ("key", "client\n'<key>'\nSECRET\n</key>\nverb 3\n"),
        ("key", "client\n--<key>\nSECRET\n</key>\nverb 3\n"),
        ("key", "client\n\t<key>\t;\r\nSECRET\r\n  </key> trailing\r\nverb 3\r\n"),
        ("", "client\n<>\nSECRET\n</>\nverb 3\n"),
    ];

    #[test]
    fn every_block_openvpn_reads_inline_is_folded() {
        for (name, text) in OPENED {
            let shown = fold(text);
            assert!(!shown.contains("SECRET"), "{name}: {shown}");
            let folded = format!("<{name}> … {} lines … </{name}>", hidden_lines(text));
            assert!(shown.contains(&folded.replace(" 1 lines", " 1 line")), "{name}: {shown}");
            assert!(shown.ends_with("verb 3\n"), "{name}: the block ends where OpenVPN ends it: {shown}");
            let p = profile(text);
            assert_eq!(p["text"], shown, "{name}");
            assert_eq!(p["blocks"], json!([name]), "{name}");
        }
    }

    /// The lines OpenVPN reads between a test profile's opener and closer.
    fn hidden_lines(text: &str) -> usize {
        let lines: Vec<&str> = text.lines().collect();
        let opener = lines.iter().position(|l| l.contains('<')).unwrap();
        let closer = lines.iter().position(|l| l.contains("</")).unwrap();
        closer - opener - 1
    }

    #[test]
    fn a_line_that_is_more_than_its_tag_opens_nothing() {
        for text in [
            "client\n<key> extra\nshown\n",
            "client\n\\<key>\nshown\n",
            "client\n# <key>\nshown\n",
            "client\n<key\nshown\n",
            "client\n\"<key>\nshown\n",
            "client\n\u{a0}<key>\nshown\n",
        ] {
            assert_eq!(fold(text), text);
        }
    }

    /// OpenVPN's whitespace is C's isspace, never Unicode's: a non-breaking
    /// space before the closing tag leaves the block open, as it does there.
    #[test]
    fn a_non_breaking_space_does_not_close_a_block() {
        let text = "client\n<key>\nKEY1\n\u{a0}</key>\nSECRET\n</key>\nverb 3\n";
        assert_eq!(fold(text), "client\n<key> … 3 lines … </key>\nverb 3\n");
    }

    /// OpenVPN reads a profile 256 bytes at a time outside a block and 255
    /// inside one, so a longer line is read as several, each on its own.
    #[test]
    fn a_tag_past_openvpns_line_buffer_is_read_where_openvpn_reads_it() {
        let opened_late = format!("client\n{}<key>\nSECRET\n</key>\nverb 3\n", " ".repeat(256));
        assert_eq!(fold(&opened_late), "client\n<key> … 1 line … </key>\nverb 3\n");
        // the block closes mid-line and the line's rest opens another
        let reopened = format!(
            "client\n<key>\n{}</key>{}<tls-crypt>\n</key>\nSECRET\n</tls-crypt>\nverb 3\n",
            "A".repeat(255),
            " ".repeat(249),
        );
        let shown = fold(&reopened);
        assert!(!shown.contains("SECRET"), "{shown}");
        assert!(shown.ends_with("\nverb 3\n"), "{shown}");
        assert_eq!(profile(&reopened)["blocks"], json!(["key", "tls-crypt"]));
    }

    #[test]
    fn a_profile_folded_and_saved_untouched_keeps_every_byte() {
        let long = format!("client\n{}<key>\nSECRET\n</key>\n", " ".repeat(256));
        let open_at_end = "client\n<key>\nSECRET\n";
        let blank_at_end = "client\n<key>\nSECRET\n\n";
        for text in OPENED.iter().map(|(_, t)| *t).chain([PROFILE, CONNECTIONS, long.as_str(), open_at_end, blank_at_end]) {
            let text = text.replace("\r\n", "\n");
            assert_eq!(unfold(&fold(&text), &text), Some(text));
        }
        // its lines go back without their CRs, so a <connection> block left
        // unread for a line only its CR made OpenVPN's whole buffer is read
        // as it goes back, and refuses nothing
        let crlf = format!("client\r\n<connection>\r\nremote a.example 1194\r\n#{}\r\n</connection>\r\n", "x".repeat(252));
        assert_eq!(profile(&crlf)["remotes"], json!([]));
        assert_eq!(unfold(&fold(&crlf), &crlf), Some(crlf.replace("\r\n", "\n")));
    }

    /// OpenVPN drops a BOM on the first line alone, so a block whose opener
    /// carries one opens only there. Its placeholder moved down a line would
    /// put back an opener that opens nothing, its key shown on the next fold:
    /// that save is refused, and one that leaves the block in place is not.
    #[test]
    fn a_block_put_back_where_it_no_longer_opens_is_refused() {
        let original = "\u{feff}<key>\nSECRET\n</key>\nclient\n";
        let shown = fold(original);
        assert_eq!(shown, "<key> … 1 line … </key>\nclient\n");
        let moved = "client\n<key> … 1 line … </key>\n";
        assert_eq!(unfold(moved, original), None);
        assert_eq!(
            unfold(&shown.replace("client", "dev tun"), original).as_deref(),
            Some("\u{feff}<key>\nSECRET\n</key>\ndev tun\n")
        );
        // moved into a block the editor opened, it stays out of sight
        let inside = "client\n<ca>\n<key> … 1 line … </key>\n";
        assert!(!fold(&unfold(inside, original).unwrap()).contains("SECRET"));
    }

    /// A <connection> block's own options are read out as the profile's
    /// servers, folded or not. A block put back among them, where OpenVPN
    /// would not read it as a block, would have its lines read out with
    /// them: that save is refused, wherever among them it lands.
    #[test]
    fn a_block_put_back_among_a_connections_options_is_refused() {
        let original = "\u{feff}<auth-user-pass>\nremote SECRET1 1194\nproto SECRET2\n</auth-user-pass>\nclient\ndev tun\n";
        let placeholder = "<auth-user-pass> … 2 lines … </auth-user-pass>";
        assert_eq!(fold(original), format!("{placeholder}\nclient\ndev tun\n"));
        for moved in [
            format!("client\n<connection>\nremote x 1\n{placeholder}\n</connection>\ndev tun\n"),
            format!("client\n<connection>\nremote x 1\n{placeholder}\nremote y 2\n</connection>\ndev tun\n"),
        ] {
            assert_eq!(unfold(&moved, original), None, "{moved}");
        }
    }

    /// Of a <connection> block's options only its remote and proto values
    /// leave the helper; fold hides the rest (a proxy's sign-in header, say).
    /// A block the editor opens above the placeholder and a line inside it
    /// closes would put those options back where fold shows them: refused.
    #[test]
    fn a_connections_options_put_back_where_fold_shows_them_are_refused() {
        let original = "client\n<connection>\nremote a 1\n<tls-crypt>\nKEY\n</tls-crypt>\nhttp-proxy-option CUSTOM-HEADER Proxy-Authorization SECRETBASIC\n</connection>\n";
        let placeholder = "<connection> … 5 lines … </connection>";
        assert_eq!(fold(original), format!("client\n{placeholder}\n"));
        assert!(!profile(original).to_string().contains("SECRETBASIC"));
        let split = format!("<tls-crypt>\n{placeholder}\nclient\n");
        assert_eq!(unfold(&split, original), None);
        // moved whole, its options stay where they were read
        let moved = format!("dev tun\n{placeholder}\nclient\n");
        let saved = unfold(&moved, original).unwrap();
        assert!(!fold(&saved).contains("SECRETBASIC"), "{saved}");
    }

    const CONNECTIONS: &str = "client\n<connection>\nremote a.example 1194\nproto udp\n</connection>\nremote c.example 1194 udp\n<connection>\nremote b.example 443 tcp\n<tls-crypt>\nremote hidden.example 1\n</tls-crypt>\n</connection>\n<key>\nremote secret.example 2\n</key>\n";

    /// OpenVPN reads a <connection> block's content as options of their own:
    /// its servers are the profile's. The block stays folded; its remote and
    /// proto lines are read out, and a key inline in it is not.
    #[test]
    fn a_connection_blocks_servers_are_read_out_while_it_stays_folded() {
        let p = profile(CONNECTIONS);
        assert_eq!(
            p["remotes"],
            json!([
                {"host": "a.example", "port": "1194", "proto": "udp"},
                {"host": "c.example", "port": "1194", "proto": "udp"},
                {"host": "b.example", "port": "443", "proto": "tcp"},
            ])
        );
        assert_eq!(p["proto"], "udp");
        assert_eq!(p["blocks"], json!(["connection", "connection", "key"]));
        let shown = p["text"].as_str().unwrap();
        for folded in ["a.example", "b.example", "hidden", "secret"] {
            assert!(!shown.contains(folded), "{shown}");
        }
        // a line OpenVPN would read in pieces leaves the block unread
        let long = format!("<connection>\nremote a.example 1194\n#{}\n</connection>\n", "x".repeat(300));
        assert_eq!(profile(&long)["remotes"], json!([]));
        // read apart, its content drops no BOM: a BOM-led tag opens nothing
        let bom = "client\n<connection>\n\u{feff}<ca>\nremote a.example 1194\n</ca>\n</connection>\n";
        assert_eq!(profile(bom)["remotes"], json!([{"host": "a.example", "port": "1194", "proto": ""}]));
        assert_eq!(fold(bom), "client\n<connection> … 3 lines … </connection>\n");
    }
}
