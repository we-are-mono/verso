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
use std::collections::BTreeMap;
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
/// Inline blocks a profile may carry. Every one is folded; their names are
/// all the page needs.
const BLOCKS: [&str; 9] = [
    "ca", "cert", "key", "tls-auth", "tls-crypt", "tls-crypt-v2", "secret", "pkcs12",
    "auth-user-pass",
];

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

/// profile reads what a profile says, with every inline block folded to its
/// name and length, so the text can be shown and nothing in it is a secret.
/// fold is a profile as it may be shown: every inline block one line, its
/// name and its length, so no key in it leaves the helper. A block left open
/// at the end of the file is folded too.
pub fn fold(text: &str) -> String {
    let mut shown = Vec::new();
    let mut open: Option<(&str, usize)> = None;
    for line in text.lines() {
        let trimmed = line.trim();
        if let Some((name, count)) = open.as_mut() {
            if trimmed == format!("</{name}>") {
                shown.push(placeholder(name, *count));
                open = None;
            } else {
                *count += 1;
            }
            continue;
        }
        match block_name(trimmed) {
            Some(name) => open = Some((name, 0)),
            None => shown.push(line.to_string()),
        }
    }
    if let Some((name, count)) = open {
        shown.push(placeholder(name, count));
    }
    let mut out = shown.join("\n");
    if text.ends_with('\n') {
        out.push('\n');
    }
    out
}

/// unfold is an edited profile with the keys put back: each placeholder line
/// left as fold wrote it takes the original block of that name, in order. A
/// block pasted in whole stays as pasted; a placeholder for a block the
/// original never had stays a line OpenVPN will refuse.
pub fn unfold(edited: &str, original: &str) -> String {
    let mut blocks: Vec<(&str, Vec<&str>)> = Vec::new();
    let mut open: Option<(&str, Vec<&str>)> = None;
    for line in original.lines() {
        if let Some((name, lines)) = open.as_mut() {
            lines.push(line);
            if line.trim() == format!("</{name}>") {
                blocks.push(open.take().unwrap());
            }
            continue;
        }
        if let Some(name) = block_name(line.trim()) {
            open = Some((name, vec![line]));
        }
    }
    let mut out = Vec::new();
    for line in edited.lines() {
        let restored = blocks.iter().position(|(name, lines)| {
            line.trim() == placeholder(name, lines.len().saturating_sub(2))
        });
        match restored {
            Some(at) => out.extend(blocks.remove(at).1),
            None => out.push(line),
        }
    }
    let mut out = out.join("\n");
    if edited.ends_with('\n') {
        out.push('\n');
    }
    out
}

fn block_name(trimmed: &str) -> Option<&'static str> {
    let name = trimmed.strip_prefix('<')?.strip_suffix('>')?;
    BLOCKS.iter().copied().find(|b| *b == name)
}

fn placeholder(name: &str, lines: usize) -> String {
    let unit = if lines == 1 { "line" } else { "lines" };
    format!("<{name}> … {lines} {unit} … </{name}>")
}

fn profile(text: &str) -> Value {
    let mut remotes = Vec::new();
    let mut blocks = Vec::new();
    let mut facts = Map::new();
    let mut open: Option<String> = None;
    let mut proto = String::new();
    for line in text.lines() {
        let trimmed = line.trim();
        if let Some(name) = &open {
            if trimmed == format!("</{name}>") {
                open = None;
            }
            continue;
        }
        if let Some(name) = block_name(trimmed) {
            blocks.push(name.to_string());
            open = Some(name.to_string());
            continue;
        }
        if trimmed.starts_with('#') || trimmed.starts_with(';') {
            continue;
        }
        let mut words = trimmed.split_whitespace();
        let Some(directive) = words.next() else {
            continue;
        };
        let rest: Vec<&str> = words.collect();
        match directive {
            "client" | "tls-client" => {
                facts.insert("client".into(), json!(true));
            }
            "dev" => {
                facts.insert("dev".into(), json!(rest.first().unwrap_or(&"")));
            }
            "proto" => proto = rest.first().unwrap_or(&"").to_string(),
            "remote" => {
                if let Some(host) = rest.first() {
                    remotes.push(json!({
                        "host": host,
                        "port": rest.get(1).unwrap_or(&""),
                        "proto": rest.get(2).unwrap_or(&""),
                    }));
                }
            }
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
    if blocks.iter().any(|b| b == "auth-user-pass") {
        facts.insert("asks_sign_in".into(), json!(false));
    }
    facts.insert("proto".into(), json!(proto));
    facts.insert("remotes".into(), json!(remotes));
    facts.insert("blocks".into(), json!(blocks));
    facts.insert("text".into(), json!(fold(text)));
    Value::Object(facts)
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
        let dir = scratch("hook");
        let hook = fs::read_to_string("../docker/rootfs/etc/hotplug.d/openvpn/50-verso")
            .expect("hook")
            .replace("/var/run/verso-openvpn", dir.to_str().expect("utf-8"));
        let script = dir.join("hook.sh");
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
        assert!(!dir.join("../x").exists());
        let _ = fs::remove_dir_all(&dir);
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
        assert_eq!(unfold(&edited, PROFILE), PROFILE.replace("remote a 1194", "remote b 443"));
    }

    #[test]
    fn a_block_pasted_whole_replaces_the_old_one() {
        let edited = fold(PROFILE).replace(
            "<tls-crypt> … 1 line … </tls-crypt>",
            "<tls-crypt>\nNEWKEY\n</tls-crypt>",
        );
        let saved = unfold(&edited, PROFILE);
        assert!(saved.contains("NEWKEY") && !saved.contains("\nKEY\n"), "{saved}");
        assert!(saved.contains("CERT1"), "{saved}");
    }

    #[test]
    fn a_placeholder_taken_out_takes_its_key_with_it() {
        let edited = fold(PROFILE).replace("<ca> … 2 lines … </ca>\n", "");
        assert!(!unfold(&edited, PROFILE).contains("CERT1"));
    }
}
