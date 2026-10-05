// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
//! What the router's tunnels are doing, for the VPN page: each OpenVPN
//! instance's profile read out, whether it runs and on which device, what its
//! log last said, and every tunnel device's counters. Profiles are read only
//! under /etc/openvpn, their key blocks are folded away, and a sign-in file is
//! never opened: nothing secret leaves the helper.
use crate::Failure;
use serde_json::{json, Map, Value};
use std::collections::BTreeMap;
use std::fs;
use std::io::Read;
use std::path::Path;
use std::process::Command;

const PROFILES: &str = "/etc/openvpn";
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
    let log = run("/sbin/logread", &["-t", "-e", "openvpn"]);
    let addresses = addresses(&run("/sbin/ip", &["-o", "addr", "show"]));
    let mut devices = BTreeMap::new();
    let mut instances = Map::new();
    for (name, config) in &sections {
        let mut instance = log_state(&log, name);
        instance.insert("config".into(), json!(config));
        let process = running.get(name);
        instance.insert("running".into(), json!(process.is_some()));
        if process.is_none() {
            instance.insert("state".into(), json!("stopped"));
        }
        if let Some(device) = process.and_then(|p| p.device.clone()) {
            devices.insert(device.clone(), name.clone());
            instance.insert("device".into(), json!(device));
        }
        if let Some(text) = read_profile(config) {
            instance.insert("profile".into(), profile(&text));
        }
        instances.insert(name.clone(), Value::Object(instance));
    }
    Ok(json!({"instances": instances, "tunnels": tunnels(&devices, &addresses)}))
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
fn profile(text: &str) -> Value {
    let mut shown = Vec::new();
    let mut remotes = Vec::new();
    let mut blocks = Vec::new();
    let mut facts = Map::new();
    let mut open: Option<(String, usize)> = None;
    let mut proto = String::new();
    for line in text.lines() {
        let trimmed = line.trim();
        if let Some((name, count)) = open.as_mut() {
            if trimmed == format!("</{name}>") {
                shown.push(format!("<{name}> … {count} lines … </{name}>"));
                open = None;
            } else {
                *count += 1;
            }
            continue;
        }
        if let Some(name) = trimmed.strip_prefix('<').and_then(|t| t.strip_suffix('>')) {
            if BLOCKS.contains(&name) {
                blocks.push(name.to_string());
                open = Some((name.to_string(), 0));
                continue;
            }
        }
        shown.push(line.to_string());
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
    facts.insert("text".into(), json!(shown.join("\n")));
    Value::Object(facts)
}

/// log_state is what an instance's log last said: connected (and since when,
/// to which server, at which tunnel address), still trying, or turned away
/// for its sign-in.
fn log_state(log: &str, name: &str) -> Map<String, Value> {
    let tag = format!(" openvpn({name})[");
    let mut out = Map::new();
    let mut state = "";
    let mut since = 0u64;
    for line in log.lines().filter(|l| l.contains(&tag)) {
        let Some(message) = line.split_once("]: ").map(|(_, m)| m) else {
            continue;
        };
        let at = line
            .split_once(" [")
            .and_then(|(_, rest)| rest.split(['.', ']']).next())
            .and_then(|epoch| epoch.parse::<u64>().ok())
            .unwrap_or(0);
        if message.contains("Initialization Sequence Completed") {
            state = "connected";
            since = at;
        } else if message.contains("AUTH_FAILED") {
            state = "auth-failed";
        } else if message.contains("SIGUSR1")
            || message.contains("Restart pause")
            || message.contains("Connection reset")
            || message.contains("Inactivity timeout")
            || message.contains("TLS Error")
        {
            if state != "auth-failed" {
                state = "connecting";
            }
        } else if message.contains("process exiting") {
            state = "stopped";
        } else if let Some(peer) = message.split_once("Peer Connection Initiated with ") {
            let server = peer.1.split_whitespace().next().unwrap_or("");
            let server = server.split_once(']').map_or(server, |(_, s)| s);
            out.insert("server".into(), json!(server));
            if state != "connected" {
                state = "connecting";
            }
        } else if let Some(address) = message.split_once("net_addr_v4_add: ") {
            let address = address.1.split_whitespace().next().unwrap_or("");
            out.insert("address".into(), json!(address));
        }
    }
    out.insert("state".into(), json!(if state.is_empty() { "connecting" } else { state }));
    if state == "connected" {
        out.insert("since".into(), json!(since));
    }
    out
}

struct Process {
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
            out.insert(name, Process { device: tun_device(pid) });
        }
    }
    out
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

/// addresses is each device's addresses from `ip -o addr show`.
fn addresses(output: &str) -> BTreeMap<String, Vec<String>> {
    let mut out: BTreeMap<String, Vec<String>> = BTreeMap::new();
    for line in output.lines() {
        let words: Vec<&str> = line.split_whitespace().collect();
        if let [_, device, family, address, ..] = words.as_slice() {
            if *family == "inet" || *family == "inet6" {
                out.entry(device.trim_end_matches(':').to_string())
                    .or_default()
                    .push(address.to_string());
            }
        }
    }
    out
}

/// tunnels is every tunnel device someone brought up: its kind as far as the
/// kernel says, which OpenVPN instance holds it, its state and counters.
fn tunnels(devices: &BTreeMap<String, String>, addresses: &BTreeMap<String, Vec<String>>) -> Value {
    let mut out = Vec::new();
    let Ok(entries) = fs::read_dir("/sys/class/net") else {
        return json!(out);
    };
    let mut names: Vec<String> = entries
        .flatten()
        .map(|e| e.file_name().to_string_lossy().into_owned())
        .collect();
    names.sort();
    for name in names {
        let base = Path::new("/sys/class/net").join(&name);
        let read = |file: &str| fs::read_to_string(base.join(file)).unwrap_or_default();
        let link_type = read("type").trim().parse::<u64>().unwrap_or(0);
        let tun = base.join("tun_flags").exists();
        if KERNEL_TUNNELS.contains(&name.as_str()) || !(tun || TUNNEL_TYPES.contains(&link_type)) {
            continue;
        }
        let kind = if devices.contains_key(&name) {
            "openvpn"
        } else if read("uevent").lines().any(|l| l == "DEVTYPE=wireguard") {
            "wireguard"
        } else if name.starts_with("tailscale") {
            "tailscale"
        } else {
            "tunnel"
        };
        let counter = |file: &str| read(&format!("statistics/{file}")).trim().parse::<u64>().unwrap_or(0);
        out.push(json!({
            "device": name,
            "kind": kind,
            "instance": devices.get(&name),
            "up": read("operstate").trim() != "down",
            "rx": counter("rx_bytes"),
            "tx": counter("tx_bytes"),
            "addresses": addresses.get(&name).cloned().unwrap_or_default(),
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

    #[test]
    fn the_log_says_connected_since_when_and_to_which_server() {
        let log = "Mon Oct  6 14:01:58 2026 [1791295318.100] daemon.notice openvpn(proton)[812]: TCP/UDP: Preserving recently used remote address: [AF_INET]185.107.56.234:1194\n\
Mon Oct  6 14:02:00 2026 [1791295320.200] daemon.notice openvpn(proton)[812]: [node-nl-05.protonvpn.net] Peer Connection Initiated with [AF_INET]185.107.56.234:1194\n\
Mon Oct  6 14:02:01 2026 [1791295321.300] daemon.notice openvpn(proton)[812]: net_addr_v4_add: 10.96.0.14/16 dev tun0\n\
Mon Oct  6 14:02:01 2026 [1791295321.400] daemon.notice openvpn(proton)[812]: Initialization Sequence Completed\n\
Mon Oct  6 14:03:00 2026 [1791295380.000] daemon.notice openvpn(other)[900]: AUTH_FAILED\n";
        let s = log_state(log, "proton");
        assert_eq!(s["state"], "connected");
        assert_eq!(s["since"], 1791295321);
        assert_eq!(s["server"], "185.107.56.234:1194");
        assert_eq!(s["address"], "10.96.0.14/16");
        assert_eq!(log_state(log, "other")["state"], "auth-failed");
    }

    #[test]
    fn a_dropped_connection_reads_as_trying_again() {
        let log = "x [1.0] daemon.notice openvpn(proton)[1]: Initialization Sequence Completed\n\
x [2.0] daemon.notice openvpn(proton)[1]: Inactivity timeout (--ping-restart), restarting\n\
x [3.0] daemon.notice openvpn(proton)[1]: SIGUSR1[soft,ping-restart] received, process restarting\n";
        let s = log_state(log, "proton");
        assert_eq!(s["state"], "connecting");
        assert!(s.get("since").is_none());
    }

    #[test]
    fn the_init_names_the_instance_on_the_command_line() {
        let cmdline = b"/usr/sbin/openvpn\0--syslog\0openvpn(proton)\0--status\0/var/run/openvpn.proton.status\0--cd\0/etc/openvpn\0--config\0proton.ovpn\0";
        assert_eq!(instance_name(cmdline).as_deref(), Some("proton"));
        assert_eq!(instance_name(b"/usr/sbin/dnsmasq\0--syslog\0openvpn(x)\0"), None);
    }

    #[test]
    fn ip_lists_each_devices_addresses() {
        let out = "1: lo    inet 127.0.0.1/8 scope host lo\\       valid_lft forever\n9: tun0    inet 10.96.0.14/16 scope global tun0\\       valid_lft forever\n9: tun0    inet6 fe80::1/64 scope link \\       valid_lft forever\n";
        let got = addresses(out);
        assert_eq!(got["tun0"], vec!["10.96.0.14/16", "fe80::1/64"]);
    }
}
