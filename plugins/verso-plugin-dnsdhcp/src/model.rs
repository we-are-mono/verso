// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! `/etc/config/dhcp` as this plugin understands it, plus the network config the
//! DHCP pools are measured against.
//!
//! One file, two daemons: dnsmasq answers names and hands out IPv4 addresses,
//! odhcpd announces the network and hands out IPv6. They read the same sections,
//! which is why one plugin owns them — and why the pages are organized by what
//! an operator came to do rather than by which daemon consumes the option.
//!
//! Most of this config is options on one section, and a settings block is a list
//! of option names, so a section is held as a bag of values rather than a field
//! per option: the catalogue in `options` is the contract, and adding a row to a
//! page is adding an entry to it. The typed shapes here are the ones the pages
//! reason about rather than merely display — a record's kind, an upstream's
//! domain scope, an interface's subnet.

use std::collections::BTreeMap;

use verso_plugin::{Section, Snapshot};

/// CONFIG is the uci config this plugin reads and writes.
pub const CONFIG: &str = "dhcp";

/// NETWORK_CONFIG holds the interfaces the pools live on; an address range is
/// derived from it, never from the dhcp config, which names interfaces only.
pub const NETWORK_CONFIG: &str = "network";

/// DAEMON_TYPE is the section every daemon-wide option lives on.
pub const DAEMON_TYPE: &str = "dnsmasq";

/// Options is one uci section as this plugin holds it: its handle and its
/// values, scalars and lists alike, by option name.
#[derive(Default, Clone)]
pub struct Options {
    pub section: String,
    scalars: BTreeMap<String, String>,
    lists: BTreeMap<String, Vec<String>>,
}

impl Options {
    /// read takes every option a section carries, so a page can render an option
    /// this plugin has no opinion about and a save can compare against what is
    /// really written.
    pub fn read(section: &Section) -> Options {
        let mut options = Options {
            section: section.name(),
            ..Options::default()
        };
        for (option, value) in section.entries() {
            match value {
                verso_plugin::Value::Array(items) => {
                    let items = items
                        .iter()
                        .filter_map(|item| item.as_str().map(String::from))
                        .collect();
                    options.lists.insert(option.clone(), items);
                }
                verso_plugin::Value::String(text) => {
                    options.scalars.insert(option.clone(), text.clone());
                }
                _ => {}
            }
        }
        options
    }

    /// scalar reads an option as a string, or "" when the config states none.
    pub fn scalar(&self, option: &str) -> &str {
        self.scalars.get(option).map(String::as_str).unwrap_or("")
    }

    /// list reads an option that may be written either as a uci list or as one
    /// whitespace-separated string — uci accepts both, and configs use both. An
    /// empty entry is no value (a package's `doh_backup_server=''`), so it is
    /// left out rather than read as one blank item.
    pub fn list(&self, option: &str) -> Vec<String> {
        if let Some(items) = self.lists.get(option) {
            return items
                .iter()
                .filter(|item| !item.trim().is_empty())
                .cloned()
                .collect();
        }
        self.scalar(option)
            .split_whitespace()
            .map(String::from)
            .collect()
    }

    /// flag reads a boolean option in uci's vocabulary, falling back to what the
    /// daemon does when the option is absent.
    pub fn flag(&self, option: &str, fallback: bool) -> bool {
        boolean(self.scalar(option)).unwrap_or(fallback)
    }

    /// set writes a value into the model so a page can render the change it just
    /// accepted; None clears the option, the way a `null` clears it on disk.
    pub fn set(&mut self, option: &str, value: Option<&str>) {
        self.lists.remove(option);
        match value {
            Some(value) => {
                self.scalars.insert(option.to_string(), value.to_string());
            }
            None => {
                self.scalars.remove(option);
            }
        }
    }

    /// set_list writes a list option into the model; an empty list clears it.
    pub fn set_list(&mut self, option: &str, values: Vec<String>) {
        self.scalars.remove(option);
        match values.is_empty() {
            true => self.lists.remove(option),
            false => self.lists.insert(option.to_string(), values),
        };
    }
}

/// boolean is uci's reading of a written truth value.
pub fn boolean(value: &str) -> Option<bool> {
    match value {
        "1" | "on" | "true" | "yes" | "enabled" => Some(true),
        "0" | "off" | "false" | "no" | "disabled" => Some(false),
        _ => None,
    }
}

/// RecordKind is which extra name a record adds. dnsmasq spells the four in four
/// section types, and splits A from AAAA by nothing but the address family of
/// the address written — so the kind is what the operator means, and the section
/// type plus the value is how it is stored.
#[derive(Clone, Copy, PartialEq, Eq, Debug)]
pub enum RecordKind {
    A,
    Aaaa,
    Cname,
    Srv,
    Mx,
}

impl RecordKind {
    /// KINDS is the closed set a record's type select offers, in the order it
    /// offers them.
    pub const KINDS: [RecordKind; 5] = [
        RecordKind::A,
        RecordKind::Aaaa,
        RecordKind::Cname,
        RecordKind::Srv,
        RecordKind::Mx,
    ];

    /// label is what the type reads as, which is also the value its select posts.
    pub fn label(self) -> &'static str {
        match self {
            RecordKind::A => "A",
            RecordKind::Aaaa => "AAAA",
            RecordKind::Cname => "CNAME",
            RecordKind::Srv => "SRV",
            RecordKind::Mx => "MX",
        }
    }

    /// section_type is the uci section a record of this kind is written as.
    pub fn section_type(self) -> &'static str {
        match self {
            RecordKind::A | RecordKind::Aaaa => "domain",
            RecordKind::Cname => "cname",
            RecordKind::Srv => "srvhost",
            RecordKind::Mx => "mxhost",
        }
    }

    /// of reads a kind back from the value its select posted.
    pub fn of(label: &str) -> Option<RecordKind> {
        RecordKind::KINDS
            .into_iter()
            .find(|kind| kind.label().eq_ignore_ascii_case(label))
    }

    /// name_option and target_option are where this kind's two everyday values
    /// live: the name being answered, and what it answers with.
    pub fn name_option(self) -> &'static str {
        match self {
            RecordKind::A | RecordKind::Aaaa => "name",
            RecordKind::Cname => "cname",
            RecordKind::Srv => "srv",
            RecordKind::Mx => "domain",
        }
    }

    pub fn target_option(self) -> &'static str {
        match self {
            RecordKind::A | RecordKind::Aaaa => "ip",
            RecordKind::Cname | RecordKind::Srv => "target",
            RecordKind::Mx => "relay",
        }
    }

    /// priority_option is the rank a resolver picks by: SRV writes it as `class`
    /// (uci's name for the record's priority field), MX as `pref`.
    pub fn priority_option(self) -> Option<&'static str> {
        match self {
            RecordKind::Srv => Some("class"),
            RecordKind::Mx => Some("pref"),
            _ => None,
        }
    }
}

/// Record is one extra name this router answers.
pub struct Record {
    pub section: String,
    pub kind: RecordKind,
    pub name: String,
    pub target: String,
    pub port: String,
    pub priority: String,
    pub weight: String,
}

impl Record {
    /// blank is the record a new-record page starts from: an A record — the
    /// everyday kind — with nothing filled in.
    pub fn blank() -> Record {
        Record {
            section: String::new(),
            kind: RecordKind::A,
            name: String::new(),
            target: String::new(),
            port: String::new(),
            priority: String::new(),
            weight: String::new(),
        }
    }

    fn read(kind: RecordKind, options: &Options) -> Record {
        let kind = match kind {
            // A and AAAA are the same section type; the address says which.
            RecordKind::A if options.scalar("ip").contains(':') => RecordKind::Aaaa,
            kind => kind,
        };
        Record {
            section: options.section.clone(),
            kind,
            name: options.scalar(kind.name_option()).to_string(),
            target: options.scalar(kind.target_option()).to_string(),
            port: options.scalar("port").to_string(),
            priority: kind
                .priority_option()
                .map(|option| options.scalar(option))
                .unwrap_or("")
                .to_string(),
            weight: options.scalar("weight").to_string(),
        }
    }
}

/// Upstream is one entry of the daemon's `list server`: where a question goes,
/// and the domain it is limited to. dnsmasq writes a domain-limited upstream as
/// `/domain/server`, so the two are one list item on disk and two controls in
/// the editor.
pub struct Upstream {
    pub index: usize,
    pub server: String,
    pub domain: String,
}

impl Upstream {
    /// read splits one written entry.
    pub fn read(index: usize, written: &str) -> Upstream {
        let written = written.trim();
        let Some(rest) = written.strip_prefix('/') else {
            return Upstream {
                index,
                server: written.to_string(),
                domain: String::new(),
            };
        };
        // Everything up to the last separator is the domain scope (dnsmasq
        // accepts several domains in one entry); the tail is the server.
        match rest.rsplit_once('/') {
            Some((domain, server)) => Upstream {
                index,
                server: server.trim().to_string(),
                domain: domain.trim().to_string(),
            },
            None => Upstream {
                index,
                server: String::new(),
                domain: rest.trim().to_string(),
            },
        }
    }

    /// written recomposes the entry the way dnsmasq reads it.
    pub fn written(server: &str, domain: &str) -> String {
        match domain.is_empty() {
            true => server.to_string(),
            false => format!("/{domain}/{server}"),
        }
    }
}

/// Interface is one `config interface` of the network config, reduced to the
/// address a pool is measured against. An interface that learns its address at
/// runtime states neither, which is exactly what makes it an uplink here.
pub struct Interface {
    pub name: String,
    pub ipaddr: String,
    pub netmask: String,
}

impl Interface {
    fn read(section: &Section) -> Interface {
        Interface {
            name: section.name(),
            ipaddr: first_value(section, "ipaddr"),
            netmask: first_value(section, "netmask"),
        }
    }
}

/// Dnsdhcp is the whole readable state of one render.
pub struct Dnsdhcp {
    pub daemon: Options,
    pub records: Vec<Record>,
    pub upstreams: Vec<Upstream>,
    pub networks: Vec<Options>,
    pub hosts: Vec<Options>,
    pub interfaces: Vec<Interface>,
    /// How many `config relay` and `config boot` sections the config carries.
    /// Both are rare enough to be stated rather than edited, and stating them
    /// truthfully needs only the count.
    pub relays: usize,
    pub boots: usize,
}

impl Dnsdhcp {
    /// read maps the brokered snapshot onto the model, keeping config order.
    pub fn read(snapshot: &Snapshot) -> Dnsdhcp {
        let daemon = snapshot
            .sections_of_type(CONFIG, DAEMON_TYPE)
            .first()
            .map(Options::read)
            .unwrap_or_default();
        Dnsdhcp {
            records: records(snapshot),
            upstreams: daemon
                .list("server")
                .iter()
                .enumerate()
                .map(|(index, written)| Upstream::read(index, written))
                .collect(),
            networks: read_type(snapshot, "dhcp"),
            hosts: read_type(snapshot, "host"),
            interfaces: snapshot
                .sections_of_type(NETWORK_CONFIG, "interface")
                .iter()
                .map(Interface::read)
                .collect(),
            relays: snapshot.sections_of_type(CONFIG, "relay").len(),
            boots: snapshot.sections_of_type(CONFIG, "boot").len(),
            daemon,
        }
    }

    /// interface returns the network interface a pool names.
    pub fn interface(&self, name: &str) -> Option<&Interface> {
        self.interfaces
            .iter()
            .find(|interface| interface.name == name)
    }

    /// host_index and record_index find one object by its uci section name. They
    /// answer with a position rather than a reference because that is what a
    /// save needs: the same lookup finds the object, applies the accepted change
    /// to it, and states which section the write is against.
    pub fn host_index(&self, section: &str) -> Option<usize> {
        self.hosts.iter().position(|host| host.section == section)
    }

    pub fn record_index(&self, section: &str) -> Option<usize> {
        self.records
            .iter()
            .position(|record| record.section == section)
    }

    /// reserved reports whether a MAC already has a reservation — the config, not
    /// the lease table, is what makes an address permanent.
    pub fn reserved(&self, mac: &str) -> bool {
        self.hosts.iter().any(|host| {
            host.list("mac")
                .iter()
                .any(|written| same_mac(written, mac))
        })
    }
}

/// same_mac compares two written MACs as addresses rather than as text: case and
/// the separator are spelling, not identity.
pub fn same_mac(one: &str, other: &str) -> bool {
    let normalize = |mac: &str| {
        mac.chars()
            .filter(|c| c.is_ascii_hexdigit())
            .map(|c| c.to_ascii_lowercase())
            .collect::<String>()
    };
    let one = normalize(one);
    !one.is_empty() && one == normalize(other)
}

fn read_type(snapshot: &Snapshot, typ: &str) -> Vec<Options> {
    snapshot
        .sections_of_type(CONFIG, typ)
        .iter()
        .map(Options::read)
        .collect()
}

/// records reads the four section types that add a name, in one list ordered the
/// way the flat table shows them: by kind, then by config order within a kind.
fn records(snapshot: &Snapshot) -> Vec<Record> {
    let mut out = Vec::new();
    for kind in [
        RecordKind::A,
        RecordKind::Cname,
        RecordKind::Srv,
        RecordKind::Mx,
    ] {
        for section in snapshot.sections_of_type(CONFIG, kind.section_type()) {
            out.push(Record::read(kind, &Options::read(&section)));
        }
    }
    out
}

/// first_value reads a scalar option a config may also have written as a
/// single-entry list (uci allows both for an interface address).
fn first_value(section: &Section, option: &str) -> String {
    let scalar = section.scalar(option);
    if !scalar.is_empty() {
        return scalar;
    }
    section.list(option).into_iter().next().unwrap_or_default()
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::fixture;

    // An empty entry in a list is no value: https-dns-proxy leaves
    // `doh_backup_server=''` behind, and reading it as one blank server drew
    // an empty upstream with a remove beside it.
    #[test]
    fn a_list_drops_its_empty_entries() {
        let snapshot = verso_plugin::Snapshot::from_value(serde_json::json!({"dhcp": {"cfg01": {
            ".name": "cfg01", ".type": "dnsmasq",
            "doh_backup_server": [""],
            "server": ["", "1.1.1.1", " "]
        }}}));
        let daemon = Options::read(&snapshot.sections_of_type("dhcp", "dnsmasq")[0]);
        assert!(daemon.list("doh_backup_server").is_empty());
        assert_eq!(daemon.list("server"), vec!["1.1.1.1".to_string()]);
    }

    #[test]
    fn a_section_reads_scalars_and_lists_the_same_way() {
        let model = fixture::dnsdhcp();
        assert_eq!(model.daemon.scalar("domain"), "lan");
        assert!(model.daemon.flag("expandhosts", false));
        // An option written as one whitespace-separated string reads as a list.
        assert_eq!(
            model.daemon.list("rebind_domain"),
            vec!["plex.direct".to_string()]
        );
        // An absent boolean falls back to what the daemon does without it.
        assert!(model.daemon.flag("dnsseccheckunsigned", true));
        assert!(!model.daemon.flag("dnssec", false));
    }

    #[test]
    fn the_four_record_types_read_as_five_kinds() {
        let model = fixture::dnsdhcp();
        let kinds: Vec<&str> = model
            .records
            .iter()
            .map(|record| record.kind.label())
            .collect();
        assert_eq!(kinds, vec!["A", "AAAA", "CNAME", "CNAME", "SRV", "MX"]);

        let aaaa = &model.records[1];
        assert_eq!(aaaa.name, "backup.lan");
        assert_eq!(aaaa.target, "2a00:ee2:2d00:2e00::31");

        let srv = model
            .records
            .iter()
            .find(|record| record.kind == RecordKind::Srv)
            .expect("srv");
        assert_eq!(srv.name, "_matrix._tcp.lan");
        assert_eq!(srv.target, "nas.lan");
        assert_eq!(srv.port, "8448");
        assert_eq!(srv.priority, "10");
        assert_eq!(srv.weight, "5");

        let mx = model
            .records
            .iter()
            .find(|record| record.kind == RecordKind::Mx)
            .expect("mx");
        assert_eq!(
            (mx.name.as_str(), mx.target.as_str(), mx.priority.as_str()),
            ("lan", "nas.lan", "10")
        );
    }

    #[test]
    fn a_domain_limited_upstream_comes_apart_and_back_together() {
        let model = fixture::dnsdhcp();
        let entries: Vec<(&str, &str)> = model
            .upstreams
            .iter()
            .map(|up| (up.server.as_str(), up.domain.as_str()))
            .collect();
        assert_eq!(
            entries,
            vec![
                ("1.1.1.1", ""),
                ("9.9.9.9", ""),
                ("10.66.0.53", "corp.example.com"),
            ]
        );
        assert_eq!(Upstream::written("1.1.1.1", ""), "1.1.1.1");
        assert_eq!(
            Upstream::written("10.66.0.53", "corp.example.com"),
            "/corp.example.com/10.66.0.53"
        );
        // dnsmasq's "answer this domain from nowhere" entry keeps its shape.
        let blocked = Upstream::read(0, "/ads.example.com/");
        assert_eq!(
            (blocked.domain.as_str(), blocked.server.as_str()),
            ("ads.example.com", "")
        );
    }

    #[test]
    fn pools_hosts_and_interfaces_keep_config_order() {
        let model = fixture::dnsdhcp();
        let pools: Vec<&str> = model
            .networks
            .iter()
            .map(|net| net.scalar("interface"))
            .collect();
        assert_eq!(pools, vec!["lan", "guest", "wan"]);
        assert!(model.networks[2].flag("ignore", false));

        let hosts: Vec<&str> = model.hosts.iter().map(|host| host.scalar("name")).collect();
        assert_eq!(hosts, vec!["nas", "thermo-attic"]);
        assert_eq!(model.host_index("host_thermo"), Some(1));
        assert_eq!(model.record_index("srv_matrix"), Some(4));
        assert_eq!(model.host_index("no_such_host"), None);

        let lan = model.interface("lan").expect("lan");
        assert_eq!(
            (lan.ipaddr.as_str(), lan.netmask.as_str()),
            ("10.0.0.1", "255.255.255.0")
        );
        // An uplink learns its address at runtime, so it states none.
        assert_eq!(model.interface("wan").expect("wan").ipaddr, "");
    }

    #[test]
    fn a_reservation_is_found_however_its_mac_was_spelled() {
        let model = fixture::dnsdhcp();
        assert!(model.reserved("30:9c:23:5e:88:01"));
        assert!(model.reserved("30-9C-23-5E-88-01"));
        assert!(!model.reserved("42:e6:ad:ff:b7:af"));
        assert!(!model.reserved(""));
    }
}
