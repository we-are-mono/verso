// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
//! One DNS settings page. Pools belong to Interfaces; reservations to Devices.
use crate::model::Options;
use std::collections::{BTreeMap, BTreeSet};
use std::net::IpAddr;
use verso_plugin::{
    commit, commit_new, json, CommitOp, Envelope, Field, Form, Grid, List, Request, SectionWidget,
    SelectOption, Switch, Value, Widget,
};

const FLAGS: &[(&str, &str, bool)] = &[
    ("expandhosts", "Answer for connected devices by name", false),
    (
        "rebind_protection",
        "Refuse answers that point into this network",
        true,
    ),
    ("dnssec", "Verify answers are signed", false),
    ("localservice", "Answer only devices on this network", true),
    ("logqueries", "Log every query", false),
    (
        "sequential_ip",
        "Hand out addresses in order, not at random",
        false,
    ),
];
const PROVIDERS: &[(&str, &str, &str)] = &[
    (
        "https://cloudflare-dns.com/dns-query",
        "Cloudflare · 1.1.1.1",
        "1.1.1.1,1.0.0.1",
    ),
    (
        "https://dns.quad9.net/dns-query",
        "Quad9 · 9.9.9.9",
        "9.9.9.9,149.112.112.112",
    ),
    (
        "https://dns.google/dns-query",
        "Google · 8.8.8.8",
        "8.8.8.8,8.8.4.4",
    ),
    (
        "https://dns.mullvad.net/dns-query",
        "Mullvad",
        "194.242.2.2",
    ),
];
type Errors = BTreeMap<String, String>;
struct Settings {
    daemon: Options,
    defaults: Options,
    proxy: Options,
    proxy_main: Options,
    adblock: Options,
    resolver: bool,
    doh: bool,
    blocking: bool,
    dnssec: bool,
    values: BTreeMap<String, String>,
    lists: BTreeMap<String, Vec<String>>,
}
fn section(r: &Request, config: &str, kind: &str) -> Options {
    r.snapshot
        .sections_of_type(config, kind)
        .first()
        .map(Options::read)
        .unwrap_or_default()
}
fn yes(v: bool) -> String {
    if v { "1" } else { "0" }.into()
}
impl Settings {
    fn read(r: &Request) -> Self {
        let daemon = section(r, "dhcp", "dnsmasq");
        let defaults = section(r, "dhcp", "verso_defaults");
        let proxy = section(r, "https-dns-proxy", "https-dns-proxy");
        let proxy_main = section(r, "https-dns-proxy", "main");
        let adblock = section(r, "adblock", "adblock");
        let state = r.ubus.get("dnsState");
        let capability = |key: &str, fallback| {
            state
                .and_then(|s| s.get(key))
                .and_then(Value::as_bool)
                .unwrap_or(fallback)
        };
        let resolver = capability("resolver", !daemon.section.is_empty());
        let doh = capability("doh", false);
        let blocking = capability("adblock", false);
        let dnssec = capability("dnssec", false);
        let mut values = BTreeMap::new();
        for &(key, _, fallback) in FLAGS {
            values.insert(key.into(), yes(daemon.flag(key, fallback)));
        }
        for key in ["domain", "cachesize"] {
            values.insert(key.into(), daemon.scalar(key).into());
        }
        let isp = !daemon.flag("noresolv", false)
            && r.snapshot
                .sections_of_type("network", "interface")
                .iter()
                .filter(|s| matches!(s.scalar("proto").as_str(), "dhcp" | "dhcpv6" | "pppoe"))
                .all(|s| s.scalar("peerdns") != "0");
        values.insert("peerdns".into(), yes(isp));
        values.insert(
            "reservations_only".into(),
            yes(defaults.scalar("dynamicdhcp") == "0"),
        );
        values.insert("adblock".into(), yes(adblock.flag("adb_enabled", false)));
        let proxy_server = proxy_address(&proxy);
        let encrypted = doh
            && (daemon.list("server").contains(&proxy_server)
                || daemon.list("doh_server").contains(&proxy_server));
        values.insert("encrypted".into(), yes(encrypted));
        let url = proxy.scalar("resolver_url");
        values.insert(
            "provider".into(),
            if url.is_empty() {
                PROVIDERS[0].0.into()
            } else if PROVIDERS.iter().any(|p| p.0 == url) {
                url.into()
            } else {
                "custom".into()
            },
        );
        values.insert("resolver_url".into(), url.into());
        let mut lists = BTreeMap::new();
        let servers = daemon.list("server");
        lists.insert(
            "upstream".into(),
            if encrypted {
                if defaults.section.is_empty() {
                    daemon
                        .list("doh_backup_server")
                        .into_iter()
                        .filter(|s| !s.starts_with('/'))
                        .collect()
                } else {
                    defaults.list("upstream")
                }
            } else {
                servers
                    .iter()
                    .filter(|s| !s.starts_with('/'))
                    .cloned()
                    .collect()
            },
        );
        lists.insert(
            "forwarding".into(),
            servers.into_iter().filter(|s| s.starts_with('/')).collect(),
        );
        lists.insert("address".into(), daemon.list("address"));
        if !resolver {
            let mut seen = BTreeSet::new();
            lists.insert(
                "upstream".into(),
                r.snapshot
                    .sections_of_type("network", "interface")
                    .iter()
                    .filter(|n| matches!(n.scalar("proto").as_str(), "dhcp" | "dhcpv6" | "pppoe"))
                    .flat_map(|n| n.list("dns"))
                    .filter(|ip| seen.insert(ip.clone()))
                    .collect(),
            );
        }
        if encrypted {
            values.insert(
                "peerdns".into(),
                yes(defaults.flag(
                    "peerdns",
                    if daemon.scalar("doh_backup_noresolv").is_empty() {
                        isp
                    } else {
                        daemon.scalar("doh_backup_noresolv") != "1"
                    },
                )),
            );
        }
        Self {
            daemon,
            defaults,
            proxy,
            proxy_main,
            adblock,
            resolver,
            doh,
            blocking,
            dnssec,
            values,
            lists,
        }
    }
    fn v(&self, key: &str) -> &str {
        self.values.get(key).map(String::as_str).unwrap_or("")
    }
    fn list(&self, key: &str) -> Vec<String> {
        self.lists.get(key).cloned().unwrap_or_default()
    }
    fn check(&self, key: &str, label: &str, option: &str) -> Widget {
        let mut w = Widget::switch_keyed(key, label, option, "", self.v(key) == "1");
        if let Widget::Switch(Switch { style, .. }) = &mut w {
            *style = "checkbox".into();
        }
        w
    }
    fn flag(&self, key: &str) -> Widget {
        self.check(key, FLAGS.iter().find(|f| f.0 == key).unwrap().1, key)
    }
    fn field(&self, e: &Errors, key: &str, label: &str, hint: &str) -> Widget {
        let mut w = Widget::field(key, label, self.v(key), "", "").writes(key);
        if let Widget::Field(Field {
            placeholder, error, ..
        }) = &mut w
        {
            *placeholder = hint.into();
            *error = e.get(key).cloned().unwrap_or_default();
        }
        w
    }
    fn rows(
        &self,
        e: &Errors,
        key: &str,
        label: &str,
        option: &str,
        hint: &str,
        datatype: &str,
    ) -> Widget {
        let mut w = Widget::list(key, label, datatype, &self.list(key), "").writes(option);
        if let Widget::List(List {
            style,
            prompt,
            errors,
            ..
        }) = &mut w
        {
            *style = "rows".into();
            *prompt = hint.into();
            if let Some(error) = e.get(key) {
                errors.insert("0".into(), error.clone());
            }
        }
        w
    }
}
fn proxy_address(proxy: &Options) -> String {
    let port = match proxy.scalar("listen_port") {
        "" => "5053",
        p => p,
    };
    format!("127.0.0.1#{port}")
}
// missing is a capability this page needs a package for: the fact, a sentence
// under it where it has one, and the act that installs the package — in the
// package's own drawer, over this page (the shell's package panel), so it is
// added where the need for it is and the page then shows the setting it adds.
fn missing(label: &str, desc: &str, package: &str) -> Widget {
    Widget::Link {
        label: label.into(),
        desc: desc.into(),
        href: format!("/system/packages/package?name={package}"),
        style: "status".into(),
        code: String::new(),
        icon: "download".into(),
        act: format!("Install {package}"),
        panel: true,
    }
}
fn part(title: &str, lede: &str, anchor: &str, fields: Vec<Widget>) -> Widget {
    Widget::section(title, lede, fields)
        .ruled()
        .addressed_as(anchor)
}
pub fn page(r: &Request) -> Envelope {
    render(r, &Settings::read(r), &Errors::new())
}
fn render(r: &Request, s: &Settings, e: &Errors) -> Envelope {
    let mut sections = vec![part(
        "",
        "",
        "upstream",
        vec![
            s.rows(
                e,
                "upstream",
                "Upstream servers",
                "server",
                "1.1.1.1",
                "dnsserver",
            ),
            s.check("peerdns", "Use the servers the ISP hands out", "peerdns"),
        ],
    )];
    sections.push(part(
        "Local names",
        "",
        "local-names",
        if s.resolver {
            vec![
                s.field(e, "domain", "Local domain", "lan"),
                s.flag("expandhosts"),
                s.rows(
                    e,
                    "address",
                    "Extra hostnames",
                    "address",
                    "/nas.lan/10.0.0.12",
                    "dnsaddress",
                ),
                s.flag("rebind_protection"),
            ]
        } else {
            vec![missing(
                "No local resolver",
                "Devices ask the upstream servers directly.",
                "dnsmasq",
            )]
        },
    ));
    let mut privacy = if s.doh && s.resolver {
        let provider = Widget::select(
            "provider",
            "Provider",
            s.v("provider"),
            PROVIDERS
                .iter()
                .map(|p| SelectOption::new(p.0, p.1))
                .chain([SelectOption::new("custom", "Custom URL…")])
                .collect(),
            e.get("provider").map(String::as_str).unwrap_or(""),
        )
        .writes("resolver_url");
        vec![
            s.check(
                "encrypted",
                "Encrypt queries to upstream",
                "https-dns-proxy",
            ),
            Widget::When {
                name: "encrypted".into(),
                value: "1".into(),
                active: s.v("encrypted") == "1",
                children: vec![
                    provider,
                    Widget::When {
                        name: "provider".into(),
                        value: "custom".into(),
                        active: s.v("provider") == "custom",
                        children: vec![s.field(
                            e,
                            "resolver_url",
                            "Resolver URL",
                            "https://dns.example/dns-query",
                        )],
                    },
                ],
            },
        ]
    } else {
        vec![missing(
            "Queries leave in plain text",
            "Your internet provider can read every name your devices look up. Installing adds encryption here, off until you turn it on.",
            "https-dns-proxy",
        )]
    };
    if s.resolver {
        let mut dnssec = s.flag("dnssec");
        if let (Widget::Switch(Switch { error, .. }), Some(refused)) =
            (&mut dnssec, e.get("dnssec"))
        {
            *error = refused.clone();
        }
        privacy.push(dnssec);
    }
    sections.push(part(
        "Privacy",
        "Whether queries leave the router readable, and whether answers are checked for tampering.",
        "privacy",
        privacy,
    ));
    if s.resolver {
        sections.push(part("Forwarding", "Domains that should be answered by a server other than the upstream ones — a VPN, an office.", "forwarding", vec![s.rows(e, "forwarding", "Send these domains to a specific server", "server", "/corp.example/10.1.0.53", "dnsforward")]));
        sections.push(part(
            "Blocking",
            "Names that are refused before any device sees them.",
            "blocking",
            if s.blocking {
                vec![s.check("adblock", "Block ads and trackers", "adblock")]
            } else {
                vec![missing(
                    "Nothing is blocked",
                    "Installing adds a blocklist of ads and trackers here, off until you turn it on. Once on, the router downloads the lists and keeps them in memory.",
                    "adblock",
                )]
            },
        ));
        sections.push(part(
            "Behaviour",
            "Who the resolver answers, how much it remembers, and whether it keeps a record.",
            "behaviour",
            vec![
                s.flag("localservice"),
                s.field(e, "cachesize", "Cache size", "150"),
                s.flag("logqueries"),
            ],
        ));
        sections.push(part("DHCP", "How addresses are handed out on every network; each network’s own pool lives in its editor.", "dhcp", vec![crate::servers::listing(r), s.flag("sequential_ip"), s.check("reservations_only", "New networks only serve devices with a reservation", "dynamicdhcp")]));
        sections.push(part(
            "Custom options",
            "Anything dnsmasq accepts that has no field above.",
            "custom-options",
            vec![crate::files::listing(r)],
        ));
    }
    let anchors = sections
        .iter()
        .filter_map(|w| {
            if let Widget::Section(SectionWidget { title, anchor, .. }) = w {
                Some(Widget::link(
                    if title.is_empty() { "Upstream" } else { title },
                    &format!("#{anchor}"),
                    "rail",
                ))
            } else {
                None
            }
        })
        .collect();
    Envelope::page(
        "DNS & DHCP",
        Widget::Grid(Grid {
            style: "settings".into(),
            columns: 2,
            children: vec![
                // Most of what this form writes is the daemon's own section, so
                // that is where its controls' options live unless they say
                // otherwise; an option of another config is simply not marked.
                Widget::Form {
                    style: "settings".into(),
                    submit: "Save settings".into(),
                    note: String::new(),
                    target: String::new(),
                    // Each refusal rides its own field; the shell's
                    // navigator counts them, so the form says nothing.
                    error: String::new(),
                    fields: sections,
                }
                .at("dhcp", &s.daemon.section),
                Widget::section("On this page", "", vec![Widget::stack(anchors).flush()])
                    .kicker()
                    .flush(),
            ],
            ..Default::default()
        }),
    )
    .with_width("wide")
}
fn unique(f: &Form, key: &str) -> Vec<String> {
    let mut seen = BTreeSet::new();
    f.all(key)
        .iter()
        .map(|s| s.trim().to_owned())
        .filter(|s| !s.is_empty() && seen.insert(s.clone()))
        .collect()
}
pub fn valid_server(v: &str) -> bool {
    let (ip, port) = v
        .split_once('#')
        .map(|(a, b)| (a, Some(b)))
        .unwrap_or((v, None));
    ip.parse::<IpAddr>().is_ok() && port.is_none_or(|p| p.parse::<u16>().is_ok_and(|n| n > 0))
}
fn domain(v: &str) -> bool {
    !v.is_empty()
        && v.len() <= 253
        && v.trim_end_matches('.').split('.').all(|l| {
            !l.is_empty()
                && l.len() <= 63
                && l.as_bytes().first().is_some_and(u8::is_ascii_alphanumeric)
                && l.as_bytes().last().is_some_and(u8::is_ascii_alphanumeric)
                && l.bytes()
                    .all(|b| b.is_ascii_alphanumeric() || b == b'-' || b == b'_')
        })
}
fn resolver_url(url: &str) -> bool {
    if url.len() > 2048
        || url.chars().any(char::is_whitespace)
        || url.chars().any(char::is_control)
        || url.contains('#')
    {
        return false;
    }
    let Some(rest) = url.strip_prefix("https://") else {
        return false;
    };
    let authority = rest.split(['/', '?']).next().unwrap_or("");
    if let Some(rest) = authority.strip_prefix('[') {
        let Some((ip, port)) = rest.split_once(']') else {
            return false;
        };
        return ip.parse::<std::net::Ipv6Addr>().is_ok()
            && (port.is_empty()
                || port
                    .strip_prefix(':')
                    .is_some_and(|p| p.parse::<u16>().is_ok_and(|n| n > 0)));
    }
    let (host, port) = authority
        .split_once(':')
        .map(|(h, p)| (h, Some(p)))
        .unwrap_or((authority, None));
    (domain(host) || host.parse::<IpAddr>().is_ok())
        && port.is_none_or(|p| p.parse::<u16>().is_ok_and(|n| n > 0))
}
fn scoped(v: &str, address: bool) -> bool {
    let Some((names, target)) = v.strip_prefix('/').and_then(|v| v.rsplit_once('/')) else {
        return false;
    };
    names.split('/').all(|n| n == "#" || domain(n))
        && if address {
            target.is_empty() || target == "#" || target.parse::<IpAddr>().is_ok()
        } else {
            target == "#" || valid_server(target)
        }
}
fn write(config: &str, kind: &str, old: &Options, values: Value, out: &mut Vec<CommitOp>) {
    let mut changed = json!({}).as_object().unwrap().clone();
    for (key, value) in values.as_object().unwrap() {
        let differs = if let Some(items) = value.as_array() {
            old.list(key)
                != items
                    .iter()
                    .filter_map(Value::as_str)
                    .map(String::from)
                    .collect::<Vec<_>>()
        } else if value.is_null() {
            !old.scalar(key).is_empty() || !old.list(key).is_empty()
        } else {
            old.scalar(key) != value.as_str().unwrap_or("")
        };
        if differs {
            changed.insert(key.clone(), value.clone());
        }
    }
    if !changed.is_empty() {
        out.push(if old.section.is_empty() {
            let mut op = commit_new(config, kind, json!(changed));
            if config == "https-dns-proxy" && kind == "main" {
                op.section = "config".into();
            }
            if config == "adblock" && kind == "adblock" {
                op.section = "global".into();
            }
            op
        } else {
            commit(config, &old.section, json!(changed))
        });
    }
}
pub fn post(r: &Request, f: &Form) -> Envelope {
    let mut s = Settings::read(r);
    let before = s.values.clone();
    let before_lists = s.lists.clone();
    for key in ["domain", "cachesize", "provider", "resolver_url"] {
        s.values.insert(key.into(), f.get(key).trim().into());
    }
    for key in
        FLAGS
            .iter()
            .map(|f| f.0)
            .chain(["peerdns", "reservations_only", "encrypted", "adblock"])
    {
        s.values
            .insert(key.into(), yes(matches!(f.get(key).as_str(), "1" | "on")));
    }
    if s.v("encrypted") != "1" {
        for key in ["provider", "resolver_url"] {
            s.values
                .insert(key.into(), before.get(key).cloned().unwrap_or_default());
        }
    }
    let mut e = Errors::new();
    for key in ["upstream", "forwarding", "address"] {
        let values = unique(f, key);
        if values.iter().any(|v| {
            !s.list(key).contains(v)
                && !(match key {
                    "upstream" => valid_server(v),
                    "forwarding" => scoped(v, false),
                    _ => scoped(v, true),
                })
        }) {
            e.insert(
                key.into(),
                "Enter a valid DNS address or domain/server entry.".into(),
            );
        }
        s.lists.insert(key.into(), values);
    }
    if !s.v("domain").is_empty() && !domain(s.v("domain")) {
        e.insert("domain".into(), "Enter a valid domain name.".into());
    }
    if !s.v("cachesize").is_empty() && s.v("cachesize").parse::<u32>().is_err() {
        e.insert(
            "cachesize".into(),
            "Enter a non-negative whole number.".into(),
        );
    }
    if s.v("dnssec") == "1" && !s.dnssec && before.get("dnssec").is_none_or(|v| v != "1") {
        e.insert(
            "dnssec".into(),
            "DNSSEC requires a dnsmasq build with DNSSEC support. Install dnsmasq-full first."
                .into(),
        );
    }
    let url = if s.v("provider") == "custom" {
        s.v("resolver_url").to_owned()
    } else {
        s.v("provider").to_owned()
    };
    if s.doh
        && s.v("encrypted") == "1"
        && (!resolver_url(&url)
            || (s.v("provider") != "custom" && !PROVIDERS.iter().any(|p| p.0 == url)))
    {
        e.insert("resolver_url".into(), "Enter an HTTPS resolver URL.".into());
    }
    if !e.is_empty() {
        return render(r, &s, &e);
    }
    let mut ops = vec![];
    let encrypted = s.resolver && s.doh && s.v("encrypted") == "1";
    let mut upstream = s.list("upstream");
    if encrypted {
        write(
            "https-dns-proxy",
            "https-dns-proxy",
            &s.proxy,
            json!({"resolver_url":url,"bootstrap_dns":PROVIDERS.iter().find(|p| p.0 == url).map(|p|p.2).unwrap_or("1.1.1.1,9.9.9.9"),"listen_addr":"127.0.0.1","listen_port": if s.proxy.scalar("listen_port").is_empty() { "5053" } else { s.proxy.scalar("listen_port") }}),
            &mut ops,
        );
        upstream = vec![proxy_address(&s.proxy)];
    }
    if s.doh && (encrypted || before.get("encrypted").is_some_and(|v| v == "1")) {
        // Manage forwarding explicitly so proxy reloads cannot overwrite split DNS.
        write(
            "https-dns-proxy",
            "main",
            &s.proxy_main,
            json!({"dnsmasq_config_update":"-","update_dnsmasq_config":"-"}),
            &mut ops,
        );
    }
    if s.resolver {
        upstream.extend(s.list("forwarding"));
        let mut values = json!({}).as_object().unwrap().clone();
        for &(key, _, _) in FLAGS {
            if before.get(key).is_none_or(|old| old != s.v(key)) {
                values.insert(key.into(), json!(s.v(key)));
            }
        }
        for key in ["domain", "cachesize"] {
            if before.get(key).is_some_and(|old| old == s.v(key)) {
                continue;
            }
            values.insert(
                key.into(),
                if s.v(key).is_empty() {
                    Value::Null
                } else {
                    json!(s.v(key))
                },
            );
        }
        if before.get("domain").is_none_or(|old| old != s.v("domain")) {
            values.insert(
                "local".into(),
                if s.v("domain").is_empty() {
                    Value::Null
                } else {
                    json!(format!("/{}/", s.v("domain")))
                },
            );
        }
        if before_lists.get("upstream") != s.lists.get("upstream")
            || before_lists.get("forwarding") != s.lists.get("forwarding")
            || before
                .get("encrypted")
                .is_none_or(|old| old != s.v("encrypted"))
        {
            values.insert(
                "server".into(),
                if upstream.is_empty() {
                    Value::Null
                } else {
                    json!(upstream)
                },
            );
        }
        values.insert(
            "address".into(),
            if s.list("address").is_empty() {
                Value::Null
            } else {
                json!(s.list("address"))
            },
        );
        if before
            .get("peerdns")
            .is_none_or(|old| old != s.v("peerdns"))
            || before
                .get("encrypted")
                .is_none_or(|old| old != s.v("encrypted"))
        {
            values.insert(
                "noresolv".into(),
                json!(yes(encrypted || s.v("peerdns") != "1")),
            );
        }
        if s.doh && (encrypted || before.get("encrypted").is_some_and(|v| v == "1")) {
            // Older proxy packages restore their backup when restarting. Once
            // forwarding is explicit, retire that backup in the same transaction.
            for key in ["doh_server", "doh_backup_server", "doh_backup_noresolv"] {
                values.insert(key.into(), Value::Null);
            }
        }
        write("dhcp", "dnsmasq", &s.daemon, json!(values), &mut ops);
        let mut defaults = json!({});
        if before
            .get("reservations_only")
            .is_none_or(|old| old != s.v("reservations_only"))
        {
            defaults["dynamicdhcp"] = json!(if s.v("reservations_only") == "1" {
                "0"
            } else {
                "1"
            });
        }
        if encrypted {
            defaults["upstream"] = json!(s.list("upstream"));
            defaults["peerdns"] = json!(s.v("peerdns"));
        }
        write("dhcp", "verso_defaults", &s.defaults, defaults, &mut ops);
        if s.blocking {
            write(
                "adblock",
                "adblock",
                &s.adblock,
                json!({"adb_enabled":s.v("adblock")}),
                &mut ops,
            );
        }
    }
    for net in r.snapshot.sections_of_type("network", "interface") {
        if !matches!(net.scalar("proto").as_str(), "dhcp" | "dhcpv6" | "pppoe") {
            continue;
        }
        let mut values = json!({});
        if before.get("peerdns").is_none_or(|v| v != s.v("peerdns")) {
            values["peerdns"] = json!(s.v("peerdns"));
        }
        if !s.resolver {
            values["dns"] = json!(s.list("upstream"));
        }
        write(
            "network",
            "interface",
            &Options::read(&net),
            values,
            &mut ops,
        );
    }
    render(r, &s, &e).with_commit(ops)
}

#[cfg(test)]
mod tests {
    use super::*;
    use verso_plugin::{Snapshot, Ubus};
    fn request() -> Request {
        Request {
            path: "/".into(),
            query: Form::default(),
            snapshot: Snapshot::from_value(json!({
                "dhcp":{"main":{".name":"main",".type":"dnsmasq","domain":"lan","local":"/lan/","server":["1.1.1.1","/office.example/10.1.0.53"],"address":["/nas.lan/192.168.1.2"],"logqueries":"0","filter_aaaa":"1"},"lan":{".name":"lan",".type":"dhcp","interface":"lan","dynamicdhcp":"1"}},
                "network":{"wan":{".name":"wan",".type":"interface","proto":"dhcp","peerdns":"1"}},
                "https-dns-proxy":{"config":{".name":"config",".type":"main"},"resolver":{".name":"resolver",".type":"https-dns-proxy","listen_port":"5053","resolver_url":"https://dns.quad9.net/dns-query"}}
            })),
            ubus: Ubus::from_value(
                json!({"dnsState":{"resolver":true,"dnssec":false,"doh":true,"adblock":false}}),
            ),
        }
    }
    // Every status row the page draws, wherever it sits in the tree.
    fn statuses(v: &Value) -> Vec<Value> {
        match v {
            Value::Object(m) if m.get("style").and_then(Value::as_str) == Some("status") => {
                vec![v.clone()]
            }
            Value::Object(m) => m.values().flat_map(statuses).collect(),
            Value::Array(a) => a.iter().flat_map(statuses).collect(),
            _ => vec![],
        }
    }

    // A capability this page needs a package for is a settings row whose act
    // installs the package in its own drawer over the page — never a link that
    // leaves for Packages and lands on an empty Installed list. It says what
    // installing adds, and that it arrives off: installing changes nothing.
    #[test]
    fn a_missing_package_installs_in_its_own_drawer_over_the_page() {
        let mut r = request();
        r.ubus =
            Ubus::from_value(json!({"dnsState":{"resolver":true,"doh":false,"adblock":false}}));
        let rows = statuses(&serde_json::to_value(page(&r)).expect("serialize"));
        for (label, package) in [
            ("Queries leave in plain text", "https-dns-proxy"),
            ("Nothing is blocked", "adblock"),
        ] {
            let row = rows
                .iter()
                .find(|w| w["label"] == label)
                .unwrap_or_else(|| panic!("no row for {label}"));
            let desc = row["desc"].as_str().unwrap_or_default();
            assert!(desc.contains("Installing adds"), "{label}: {desc}");
            assert!(desc.contains("off until you turn it on"), "{label}: {desc}");
            assert_eq!(row["act"], format!("Install {package}"), "{label}");
            assert_eq!(
                row["href"],
                format!("/system/packages/package?name={package}"),
                "{label}"
            );
            assert_eq!(row["icon"], "download", "{label}");
            assert_eq!(row["panel"], true, "{label}");
        }
    }

    fn encoded(s: &str) -> String {
        s.bytes().map(|b| format!("%{b:02X}")).collect()
    }
    fn form(r: &Request, overrides: &[(&str, &[&str])]) -> Form {
        let s = Settings::read(r);
        let mut fields: BTreeMap<String, Vec<String>> =
            s.values.into_iter().map(|(k, v)| (k, vec![v])).collect();
        fields.extend(s.lists);
        for (k, vs) in overrides {
            fields.insert((*k).into(), vs.iter().map(|v| (*v).into()).collect());
        }
        Form::parse(
            &fields
                .into_iter()
                .flat_map(|(k, vs)| {
                    vs.into_iter()
                        .map(move |v| format!("{}={}", encoded(&k), encoded(&v)))
                })
                .collect::<Vec<_>>()
                .join("&"),
        )
    }
    #[test]
    fn the_settings_form_says_the_daemon_section_holds_its_options() {
        // The shell marks a control whose option waits on the stage by its
        // full address; the form names the daemon's section once.
        let page = serde_json::to_string(&page(&request())).unwrap();
        assert!(page.contains("\"target\":\"dhcp.main\""), "{page}");
    }
    #[test]
    fn unchanged_settings_do_not_stage_defaults() {
        let r = request();
        assert!(post(&r, &form(&r, &[])).commit.is_empty());
    }
    #[test]
    fn lists_and_defaults_leave_existing_pools_and_expert_options_alone() {
        let r = request();
        let e = post(
            &r,
            &form(
                &r,
                &[
                    ("upstream", &["9.9.9.9", "1.1.1.1"]),
                    ("reservations_only", &["1"]),
                ],
            ),
        );
        let j = serde_json::to_value(e).unwrap();
        let ops = j["commit"].as_array().unwrap();
        let daemon = ops.iter().find(|o| o["section"] == "main").unwrap();
        assert_eq!(
            daemon["values"]["server"],
            json!(["9.9.9.9", "1.1.1.1", "/office.example/10.1.0.53"])
        );
        assert!(daemon["values"].get("filter_aaaa").is_none());
        assert!(ops.iter().all(|o| o["section"] != "lan"));
        assert!(ops
            .iter()
            .any(|o| o["type"] == "verso_defaults" && o["values"]["dynamicdhcp"] == "0"));
    }
    #[test]
    fn invalid_values_refuse_the_entire_save() {
        let r = request();
        for changes in [
            vec![("upstream", &["1.2.3.999"][..])],
            vec![("forwarding", &["/office.example/10.1.0.53#65536"][..])],
            vec![("cachesize", &["-1"][..])],
            vec![("dnssec", &["1"][..])],
            vec![
                ("encrypted", &["1"][..]),
                ("provider", &["custom"][..]),
                ("resolver_url", &["http://insecure.example/"][..]),
            ],
        ] {
            let e = post(&r, &form(&r, &changes));
            assert!(e.commit.is_empty());
            let v = serde_json::to_string(&e).unwrap();
            // The refusal rides the refused field; the form says nothing.
            assert!(
                v.contains("\"error\":\"") || v.contains("\"errors\":{"),
                "{v}"
            );
            assert!(!v.contains("Check the highlighted fields."));
        }
    }
    // A refused DNSSEC switch is refused as a field is — on the switch, drawn
    // under its label — never as a notice pushed in beside it.
    #[test]
    fn a_refused_dnssec_switch_carries_its_own_refusal() {
        let r = request();
        let e = post(&r, &form(&r, &[("dnssec", &["1"])]));
        let body = serde_json::to_value(&e).unwrap();
        let switch =
            find(&body, &|v| v["type"] == "switch" && v["name"] == "dnssec").expect("switch");
        assert!(switch["error"]
            .as_str()
            .is_some_and(|m| m.starts_with("DNSSEC requires")));
        assert!(
            find(&body, &|v| v["type"] == "callout"
                && v["body"]
                    .as_str()
                    .is_some_and(|b| b.starts_with("DNSSEC requires")))
            .is_none(),
            "no callout repeats the refusal"
        );
    }
    fn find(v: &Value, wanted: &dyn Fn(&Value) -> bool) -> Option<Value> {
        if wanted(v) {
            return Some(v.clone());
        }
        match v {
            Value::Object(m) => m.values().find_map(|c| find(c, wanted)),
            Value::Array(a) => a.iter().find_map(|c| find(c, wanted)),
            _ => None,
        }
    }
    #[test]
    fn encryption_routes_only_general_queries_through_the_proxy() {
        let r = request();
        let e = post(&r, &form(&r, &[("encrypted", &["1"])]));
        let ops = serde_json::to_value(e.commit).unwrap();
        let ops = ops.as_array().unwrap();
        let dns = ops.iter().find(|o| o["section"] == "main").unwrap();
        assert_eq!(
            dns["values"]["server"],
            json!(["127.0.0.1#5053", "/office.example/10.1.0.53"])
        );
        assert_eq!(dns["values"]["noresolv"], "1");
        let defaults = ops.iter().find(|o| o["type"] == "verso_defaults").unwrap();
        assert_eq!(defaults["values"]["upstream"], json!(["1.1.1.1"]));
        assert_eq!(defaults["values"]["peerdns"], "1");
        assert!(ops.iter().any(
            |o| o["config"] == "https-dns-proxy" && o["values"]["dnsmasq_config_update"] == "-"
        ));
    }
    #[test]
    fn without_a_resolver_upstreams_belong_to_the_uplink() {
        let mut r = request();
        r.ubus = Ubus::from_value(json!({"dnsState":{"resolver":false}}));
        let e = post(&r, &form(&r, &[("upstream", &["9.9.9.9"])]));
        assert_eq!(e.commit.len(), 1);
        assert_eq!(e.commit[0].config, "network");
        assert_eq!(e.commit[0].values["dns"], json!(["9.9.9.9"]));
    }
}
