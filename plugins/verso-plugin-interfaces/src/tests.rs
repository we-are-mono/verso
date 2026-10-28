// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
use super::*;
use verso_plugin::{json, Ubus};
fn request(path: &str, query: &str) -> Request {
    Request {
        path: path.into(),
        query: Form::parse(query),
        snapshot: Snapshot::from_value(fixture()),
        plugins: Default::default(),
        ubus: Ubus::from_value(
            json!({"networkState":{"devices":{"eth0":{"up":true,"physical":true},"eth1":{"up":true,"physical":true},"eth2":{"up":false,"physical":true},"br-lan":{"up":true,"bridge":true}},"interfaces":[{"interface":"lan","l3_device":"br-lan","up":true,"ipv4-address":[{"address":"192.168.1.1","mask":24}]}]}}),
        ),
    }
}
// fixture is the router's config the requests read: a LAN bridge with its
// DHCP server, a DHCP uplink and their zones.
fn fixture() -> serde_json::Value {
    json!({
        "network":{
         "lan":{".name":"lan",".type":"interface","device":"br-lan","proto":"static","ipaddr":"192.168.1.1","netmask":"255.255.255.0","auto":"1"},
         "br":{".name":"br",".type":"device","name":"br-lan","type":"bridge","ports":["eth0"]},
         "uplink":{".name":"uplink",".type":"interface","device":"eth1","proto":"dhcp"}},
        "firewall":{"lan":{".name":"lan",".type":"zone","name":"lan","network":["lan"]},"wan":{".name":"wan",".type":"zone","name":"wan","network":["uplink"]}},
        "dhcp":{"lan":{".name":"lan",".type":"dhcp","interface":"lan","start":"100","limit":"150","leasetime":"12h"}},
        "wireless":{"radio":{".name":"radio",".type":"wifi-iface","network":"lan"}}
    })
}
fn form(extra: &str) -> Form {
    Form::parse(&format!("name=guest&device=eth2&proto=static&ipaddr=192.168.20.1&netmask=255.255.255.0&zone=lan&auto=1&{extra}"))
}
#[test]
fn new_network_stages_named_section_and_references() {
    let r = request("/new", "kind=network");
    let e = post(&r, &form(""));
    assert!(!e.commit.is_empty(), "{:?}", e);
    let j = serde_json::to_value(&e).unwrap();
    assert_eq!(j["commit"][0]["type"], "interface");
    assert_eq!(j["commit"][0]["section"], "guest");
    assert!(e.commands.is_empty());
    assert_eq!(j["back"]["href"], ROOT);
}
// A new network starts with a DHCP server of the daemon's defaults, named after
// it; a line to the internet starts with none. Editing either is the DHCP
// page's.
#[test]
fn a_new_network_starts_with_a_default_dhcp_server() {
    let e = post(&request("/new", "kind=network"), &form(""));
    let j = serde_json::to_value(&e).unwrap();
    let pool = j["commit"]
        .as_array()
        .unwrap()
        .iter()
        .find(|o| o["config"] == "dhcp")
        .expect("a pool");
    assert_eq!(pool["section"], "guest");
    assert_eq!(pool["type"], "dhcp");
    assert_eq!(
        pool["values"],
        json!({"interface": "guest", "start": "100", "limit": "150", "leasetime": "12h"})
    );
    let wan = post(
        &request("/new", "kind=wan"),
        &Form::parse("name=wan2&device=eth2&proto=dhcp&auto=1"),
    );
    assert!(wan.commit.iter().all(|o| o.config != "dhcp"));
}
// Saving a network leaves its DHCP server alone: the server is edited on the
// DHCP page, and the editor posts nothing about it.
#[test]
fn saving_a_network_writes_nothing_to_its_dhcp_server() {
    let e = post(
        &request("/edit", "network=lan"),
        &Form::parse("name=lan&device=br-lan&proto=static&ipaddr=192.168.1.1&netmask=255.255.255.0&zone=lan&auto=1&mtu=1400"),
    );
    assert!(!e.commit.is_empty());
    assert!(
        e.commit.iter().all(|o| o.config != "dhcp"),
        "{:?}",
        e.commit
    );
}
// A network that hands out addresses keeps a static one to hand out from.
#[test]
fn a_serving_network_keeps_its_static_address() {
    let e = post(
        &request("/edit", "network=lan"),
        &Form::parse("name=lan&device=br-lan&proto=dhcp&zone=lan&auto=1"),
    );
    assert!(e.commit.is_empty());
    assert!(serde_json::to_string(&e)
        .unwrap()
        .contains("Turn its DHCP server off"));
}
#[test]
fn invalid_posts_never_stage() {
    for body in [
        "name=lan&device=eth2&proto=dhcp",
        "name=guest&device=missing&proto=dhcp",
        "name=../bad&proto=dhcp",
        "name=guest&device=eth2&proto=static&ipaddr=invalid&netmask=255.255.255.0",
        "name=guest&device=eth2&proto=static&ipaddr=192.168.3.1&netmask=255.0.255.0",
        "name=wan2&device=eth2&proto=pppoe&username=account",
    ] {
        let e = post(&request("/new", "kind=network"), &Form::parse(body));
        assert!(e.commit.is_empty(), "accepted {body}");
        assert!(
            serde_json::to_string(&e).unwrap().contains("\"error\":\""),
            "no refusal for {body}"
        );
    }
}
#[test]
fn device_kinds_write_device_sections() {
    for (kind, body, typ) in [
        ("bridge", "name=br_guest&ports=eth2&stp=1", "bridge"),
        (
            "vlan",
            "name=eth2.20&device=eth2&vid=20&vlan_protocol=8021q",
            "8021q",
        ),
    ] {
        let e = post(
            &request("/new", &format!("kind={kind}")),
            &Form::parse(body),
        );
        let j = serde_json::to_value(&e).unwrap();
        assert_eq!(j["commit"][0]["type"], "device", "{j}");
        assert_eq!(j["commit"][0]["values"]["type"], typ);
    }
}
#[test]
fn rejects_bridge_and_vlan_conflicts() {
    for (kind, body) in [
        ("bridge", "name=br_guest&ports=eth0"),
        (
            "vlan",
            "name=eth2.9999&device=eth2&vid=9999&vlan_protocol=8021q",
        ),
    ] {
        assert!(post(
            &request("/new", &format!("kind={kind}")),
            &Form::parse(body)
        )
        .commit
        .is_empty());
    }
}
#[test]
fn network_rename_updates_all_owners() {
    let e=post(&request("/edit","network=lan"),&Form::parse("name=home&device=br-lan&proto=static&ipaddr=192.168.1.1&netmask=255.255.255.0&zone=lan&auto=1"));
    let j = serde_json::to_value(&e).unwrap();
    assert!(!e.commit.is_empty(), "{j}");
    let ops = &j["commit"];
    assert!(ops
        .as_array()
        .unwrap()
        .iter()
        .any(|o| o["config"] == "wireless" && o["values"]["network"] == json!(["home"])));
    assert!(ops
        .as_array()
        .unwrap()
        .iter()
        .any(|o| o["config"] == "dhcp" && o["values"] == json!({"interface": "home"})));
    assert!(ops
        .as_array()
        .unwrap()
        .iter()
        .any(|o| o["delete"] == true && o["section"] == "lan"));
}
#[test]
fn deletion_refuses_devices_still_in_use() {
    assert!(post(
        &request("/delete", "device=br-lan"),
        &Form::parse("delete=1")
    )
    .commit
    .is_empty());
}
// the_type is the drawer's Type choice: the one control a new interface opens
// with, which decides which fields the rest of the form has.
fn the_type(drawer: &serde_json::Value) -> serde_json::Value {
    let form = &drawer["children"][0];
    form["fields"][0]["children"][0].clone()
}
#[test]
fn adding_an_interface_opens_a_new_network_with_its_type_to_change() {
    let listing = serde_json::to_value(get(&request("/", ""))).unwrap();
    let act = &listing["act"];
    assert_eq!(act["label"], "Add interface");
    assert_eq!(act["href"], format!("{ROOT}new"));
    // The listing carries no drawer for it: the shell fetches the drawer from
    // the act's address, so the drawer's form posts to the address that makes
    // an interface rather than to the listing.
    assert_eq!(act["opens_panel"], true, "{act}");
    assert!(act.get("drawer").is_none(), "{act}");
    let opened = get(&request("/new", ""));
    assert!(opened.pages.is_empty());
    let drawer = open_drawer(&opened);
    assert_eq!(drawer["title"], "New network");
    let kind = the_type(&drawer);
    assert_eq!(kind["name"], "kind", "{drawer}");
    assert_eq!(kind["kind"], "select");
    assert_eq!(kind["value"], "network");
    assert_eq!(kind["reshapes"], true);
    let values: Vec<&str> = kind["options"]
        .as_array()
        .unwrap()
        .iter()
        .map(|o| o["value"].as_str().unwrap())
        .collect();
    // A network, a line or a tunnel; a bridge or a VLAN is a device, added
    // from the Devices card, whose chooser offers those alone.
    assert_eq!(values, ["network", "wan", "tunnel"]);
    let device = the_type(&open_drawer(&get(&request("/new", "kind=bridge"))));
    let kinds: Vec<&str> = device["options"]
        .as_array()
        .unwrap()
        .iter()
        .map(|o| o["value"].as_str().unwrap())
        .collect();
    assert_eq!(kinds, ["bridge", "vlan"]);
    // A VPN is made on the tunnels' own page, and the drawer says where.
    assert!(
        drawer.to_string().contains(&format!("{ROOT}vpn")),
        "{drawer}"
    );
    // Editing an interface is not choosing one: an existing object's kind is
    // what it is.
    let lan = open_drawer(&get(&request("/edit", "network=lan")));
    assert_ne!(the_type(&lan)["name"], "kind", "{lan}");
    // The drawer's title already names what it is; its opening fields stand
    // under it without a heading or a lede of their own.
    for d in [
        &drawer,
        &lan,
        &open_drawer(&get(&request("/edit", "network=&device=br-lan"))),
    ] {
        // The Status card, where there is one, comes first; the opening
        // fields after it.
        let opening = &cards(d)
            .into_iter()
            .find(|c| c["title"] != "Status")
            .unwrap();
        let said = |k: &str| {
            opening
                .get(k)
                .and_then(|v| v.as_str())
                .unwrap_or_default()
                .to_string()
        };
        assert!(
            said("title").is_empty() && said("sub").is_empty(),
            "{opening}"
        );
        // It is one of the drawer's subjects like the rest: ruled, so it keeps
        // its air before the next section's rule (the first one draws none
        // above itself), never a bare block that adds none.
        assert_eq!(opening["hairline"], true, "{opening}");
        assert!(opening.get("flush").is_none(), "{opening}");
    }
}
#[test]
fn changing_the_type_reshapes_the_drawer_and_keeps_what_still_applies() {
    let e = post(
        &request("/new", ""),
        &Form::parse(
            "_action=reshape&kind=bridge&name=guest&device=eth2&mtu=1400&proto=static&ipaddr=10.0.",
        ),
    );
    assert!(e.commit.is_empty(), "a reshape stages nothing: {e:?}");
    let drawer = open_drawer(&e);
    assert_eq!(drawer["title"], "New bridge");
    assert_eq!(the_type(&drawer)["value"], "bridge");
    let j = drawer.to_string();
    assert!(
        !j.contains("\"error\":\""),
        "a reshape refuses nothing: {j}"
    );
    assert!(
        j.contains("\"value\":\"guest\""),
        "the name carries over: {j}"
    );
    assert!(
        j.contains("\"value\":\"1400\""),
        "the MTU carries over: {j}"
    );
    assert!(
        !j.contains("\"value\":\"10.0.\""),
        "a network's address is no bridge's: {j}"
    );
    // A line to the internet starts as one, whatever the network was set to.
    let wan = open_drawer(&post(
        &request("/new", ""),
        &Form::parse("_action=reshape&kind=wan&name=wan2&device=eth2&proto=static&zone=lan"),
    ));
    assert_eq!(wan["title"], "New internet connection");
    assert_eq!(value_of(&wan, "proto"), "dhcp", "{wan}");
    assert_eq!(value_of(&wan, "zone"), "wan", "{wan}");
    assert_eq!(
        value_of(&wan, "device"),
        "eth2",
        "the device it runs on carries over: {wan}"
    );
}
// value_of is the value of the first control named name anywhere in v.
fn value_of(v: &serde_json::Value, name: &str) -> String {
    match v {
        serde_json::Value::Object(o) if o.get("name").and_then(|n| n.as_str()) == Some(name) => o
            .get("value")
            .and_then(|x| x.as_str())
            .unwrap_or_default()
            .into(),
        serde_json::Value::Object(o) => o
            .values()
            .map(|x| value_of(x, name))
            .find(|s| !s.is_empty())
            .unwrap_or_default(),
        serde_json::Value::Array(a) => a
            .iter()
            .map(|x| value_of(x, name))
            .find(|s| !s.is_empty())
            .unwrap_or_default(),
        _ => String::new(),
    }
}
#[test]
fn a_new_interface_saves_as_the_type_chosen() {
    let e = post(
        &request("/new", ""),
        &Form::parse("kind=bridge&name=br_guest&ports=eth2&stp=1"),
    );
    let j = serde_json::to_value(&e).unwrap();
    assert_eq!(j["commit"][0]["type"], "device", "{j}");
    assert_eq!(j["commit"][0]["values"]["type"], "bridge");
    assert!(post(&request("/new", ""), &Form::parse("kind=vpn&name=x"))
        .commit
        .is_empty());
}
// lab is the router in the dev container: the LAN on a bridge over one port,
// a management port, and an internet line with its DHCPv6 half.
fn lab() -> Model {
    Model::read(&lab_at("/", ""))
}
// lab_at is a request to the lab router at path.
fn lab_at(path: &str, query: &str) -> Request {
    let mut r = request(path, query);
    let mut snapshot = fixture();
    snapshot["network"] = json!({
        "loopback":{".name":"loopback",".type":"interface",".index":0,"device":"lo","proto":"static"},
        "mgmt":{".name":"mgmt",".type":"interface",".index":1,"device":"eth0","proto":"static","ipaddr":"172.20.0.10","netmask":"255.255.0.0"},
        "br":{".name":"br",".type":"device",".index":2,"name":"br-lan","type":"bridge","ports":["lan0"]},
        "lan":{".name":"lan",".type":"interface",".index":3,"device":"br-lan","proto":"static","ipaddr":"192.168.77.1","netmask":"255.255.255.0"},
        "wan":{".name":"wan",".type":"interface",".index":4,"device":"wan0","proto":"dhcp"},
        "wan6":{".name":"wan6",".type":"interface",".index":5,"device":"wan0","proto":"dhcpv6","reqaddress":"try","reqprefix":"56"}
    });
    snapshot["firewall"]["wan"]["network"] = json!(["wan", "wan6"]);
    snapshot["firewall"]["wan2"] = json!({".name":"wan2",".type":"zone","name":"wan2"});
    r.snapshot = Snapshot::from_value(snapshot);
    r.ubus = Ubus::from_value(json!({"networkState":{
    "devices":{
        "br-lan":{"up":true,"carrier":true,"type":"bridge","bridge-members":["lan0"]},
        "eth0":{"up":true,"carrier":true,"physical":false,"kind":"virtual"},
        "lan0":{"up":true,"carrier":true,"physical":true,"kind":"port","speed":"1000F"},
        "wan0":{"up":true,"carrier":true,"physical":false,"kind":"virtual"},
        "sit0":{"up":false,"physical":false,"kind":"tunnel"},
        "ip6tnl0":{"up":false,"physical":false,"kind":"tunnel"}
    },
    "interfaces":[
        {"interface":"mgmt","l3_device":"eth0","up":true,"proto":"static","ipv4-address":[{"address":"172.20.0.10","mask":16}]},
        {"interface":"lan","l3_device":"br-lan","up":true,"proto":"static","ipv4-address":[{"address":"192.168.77.1","mask":24}]},
        {"interface":"wan","l3_device":"wan0","up":true,"proto":"dhcp","ipv4-address":[{"address":"172.30.1.171","mask":24}]},
        {"interface":"wan6","l3_device":"wan0","up":true,"proto":"dhcpv6"}
    ]}}));
    r
}
// card is the listing's card titled title: its table's rows and columns.
fn card(m: &Model, title: &str) -> serde_json::Value {
    fn find(v: &serde_json::Value, title: &str) -> Option<serde_json::Value> {
        if v.get("type") == Some(&json!("section")) && v["title"] == title {
            return Some(v.clone());
        }
        match v {
            serde_json::Value::Object(o) => o.values().find_map(|v| find(v, title)),
            serde_json::Value::Array(a) => a.iter().find_map(|v| find(v, title)),
            _ => None,
        }
    }
    find(&serde_json::to_value(page::listing(m)).unwrap(), title).expect(title)
}
fn ids(card: &serde_json::Value) -> Vec<String> {
    card["children"][0]["rows"]
        .as_array()
        .unwrap()
        .iter()
        .map(|r| r["id"].as_str().unwrap().to_string())
        .collect()
}
fn columns(card: &serde_json::Value) -> Vec<String> {
    card["children"][0]["columns"]
        .as_array()
        .unwrap()
        .iter()
        .map(|c| c["label"].as_str().unwrap_or_default().to_string())
        .collect()
}
fn network_row(m: &Model, id: &str) -> serde_json::Value {
    row_in(&card(m, "Networks"), id)
}
fn device_row(m: &Model, id: &str) -> serde_json::Value {
    row_in(&card(m, "Devices"), id)
}
fn row_in(card: &serde_json::Value, id: &str) -> serde_json::Value {
    card["children"][0]["rows"]
        .as_array()
        .unwrap()
        .iter()
        .find(|r| r["id"] == id)
        .cloned()
        .unwrap_or_else(|| panic!("no row {id} in {card}"))
}
// texts are a row's cells as read, a chip cell by its chips' labels.
fn texts(row: &serde_json::Value) -> Vec<String> {
    row["cells"]
        .as_array()
        .unwrap()
        .iter()
        .map(|c| {
            let mut said = c["text"].as_str().unwrap_or_default().to_string();
            for chip in c["chips"].as_array().into_iter().flatten() {
                said.push_str(&format!(" [{}]", chip["label"].as_str().unwrap()));
            }
            said.trim().to_string()
        })
        .collect()
}
// acts are a row's acts as (title, glyph), the slots it has none for left out.
fn acts(row: &serde_json::Value) -> Vec<(String, String)> {
    row["cells"].as_array().unwrap().last().unwrap()["actions"]
        .as_array()
        .unwrap()
        .iter()
        .filter(|a| a["icon"].as_str().is_some_and(|i| !i.is_empty()))
        .map(|a| {
            (
                a["title"].as_str().unwrap().to_string(),
                a["icon"].as_str().unwrap().to_string(),
            )
        })
        .collect()
}
#[test]
fn the_page_holds_networks_then_devices() {
    let m = lab();
    let listing = serde_json::to_value(page::listing(&m)).unwrap();
    let titles: Vec<_> = listing["widget"]["children"]
        .as_array()
        .unwrap()
        .iter()
        .map(|s| s["title"].as_str().unwrap().to_string())
        .collect();
    assert_eq!(titles, ["Networks", "Devices"], "{listing}");
    // Both are live: the shell refreshes their rows as the router changes.
    for title in ["Networks", "Devices"] {
        assert_eq!(card(&m, title)["children"][0]["style"], "live");
    }
    // The networks in the order the config holds them, loopback and the
    // internet line's DHCPv6 half left out.
    assert_eq!(ids(&card(&m, "Networks")), ["mgmt", "lan", "wan"]);
    let devices = ids(&card(&m, "Devices"));
    for name in ["br-lan", "eth0", "lan0", "wan0"] {
        assert!(devices.contains(&name.to_string()), "{devices:?}");
    }
    assert!(!devices.contains(&"lo".to_string()), "{devices:?}");
    // A network netifd reports not active is still listed.
    let mut m = lab();
    m.networks.push(model::Record {
        id: "guest".into(),
        values: json!({"device":"eth2","proto":"static"})
            .as_object()
            .unwrap()
            .clone(),
        ..Default::default()
    });
    assert_eq!(texts(&network_row(&m, "guest"))[5], "not active");
}
#[test]
fn networks_columns_read_name_protocol_address_device_zone_state() {
    let m = lab();
    assert_eq!(
        columns(&card(&m, "Networks")),
        ["Network", "Protocol", "Address", "Device", "Zone", "State", ""]
    );
    assert_eq!(
        texts(&network_row(&m, "lan"))[..6],
        [
            "lan [DHCP]",
            "static",
            "192.168.77.1/24",
            "br-lan",
            "lan",
            "up"
        ]
    );
    assert_eq!(network_row(&m, "lan")["cells"][2]["copy"], true);
    assert_eq!(
        texts(&network_row(&m, "wan"))[..6],
        ["wan", "dhcp", "172.30.1.171/24", "wan0", "wan", "up"]
    );
}
#[test]
fn a_rows_dhcp_chip_stands_only_for_a_network_with_a_server() {
    let m = Model::read(&request("/", ""));
    assert!(texts(&network_row(&m, "lan"))[0].contains("[DHCP]"));
    // The uplink's section says not to serve there: no server, no chip.
    let mut r = request("/", "");
    r.snapshot = Snapshot::from_value(json!({
        "network":{"uplink":{".name":"uplink",".type":"interface","device":"eth1","proto":"dhcp"}},
        "dhcp":{"wan":{".name":"wan",".type":"dhcp","interface":"uplink","ignore":"1"}}
    }));
    let m = Model::read(&r);
    let uplink = serde_json::to_string(&network_row(&m, "uplink")).unwrap();
    assert!(!uplink.contains("\"DHCP\""), "{uplink}");
}
#[test]
fn devices_columns_read_device_type_used_by_state() {
    let mut m = lab();
    m.devices.push(model::Record {
        id: "vlan20".into(),
        values: json!({"name":"lan0.20","type":"8021q","ifname":"lan0","vid":"20"})
            .as_object()
            .unwrap()
            .clone(),
        ..Default::default()
    });
    assert_eq!(
        columns(&card(&m, "Devices")),
        ["Device", "Type", "Used by", "State", ""]
    );
    assert_eq!(
        texts(&device_row(&m, "br-lan"))[..4],
        ["br-lan", "bridge", "[lan]", "up"]
    );
    let lan0 = device_row(&m, "lan0");
    assert_eq!(
        texts(&lan0)[..4],
        ["lan0", "port", "[br-lan] [lan0.20]", "up"]
    );
    assert_eq!(lan0["cells"][1]["rate"], 1e9);
    assert_eq!(lan0["cells"][2]["chips"][0]["icon"], "git-merge");
    assert_eq!(lan0["cells"][2]["chips"][1]["icon"], "tag");
    // The internet line's device names the line, not its DHCPv6 half.
    assert_eq!(texts(&device_row(&m, "wan0"))[2], "[wan]");
    assert_eq!(
        device_row(&m, "wan0")["cells"][2]["chips"][0]["kind"],
        "network"
    );
}
#[test]
fn kernel_made_devices_without_a_use_have_no_row() {
    let m = lab();
    let devices = ids(&card(&m, "Devices"));
    assert!(!devices.contains(&"sit0".to_string()), "{devices:?}");
    assert!(!devices.contains(&"ip6tnl0".to_string()), "{devices:?}");
    // An unused port is still the router's, and listed.
    let mut m = lab();
    m.live["devices"]["eth2"] = json!({"up":false,"physical":true,"kind":"port"});
    assert_eq!(texts(&device_row(&m, "eth2"))[3], "unused");
    // A tunnel base put to use is listed.
    m.networks.push(model::Record {
        id: "six".into(),
        values: json!({"device":"sit0","proto":"6in4"})
            .as_object()
            .unwrap()
            .clone(),
        ..Default::default()
    });
    assert!(ids(&card(&m, "Devices")).contains(&"sit0".to_string()));
}
#[test]
fn rows_open_their_drawer_from_the_pencil() {
    let mut m = lab();
    m.devices.push(model::Record {
        id: "vlan20".into(),
        values: json!({"name":"eth1.20","type":"8021q","ifname":"eth1","vid":"20"})
            .as_object()
            .unwrap()
            .clone(),
        ..Default::default()
    });
    let pair = |t: &str, i: &str| (t.to_string(), i.to_string());
    // The state first, what acts on it, the way in, the removal last.
    assert_eq!(
        acts(&network_row(&m, "lan")),
        [
            pair("Bring down", "power"),
            pair("Restart", "refresh-cw"),
            pair("Edit", "square-pen"),
            pair("Delete", "trash-2"),
        ]
    );
    // A device in use is edited, never deleted; one nothing uses can go.
    assert_eq!(acts(&device_row(&m, "lan0")), [pair("Edit", "square-pen")]);
    assert_eq!(
        acts(&device_row(&m, "eth1.20")),
        [pair("Edit", "square-pen"), pair("Delete", "trash-2")]
    );
    // The pencil is the row's way in: its panel, at the drawer's address.
    let lan = network_row(&m, "lan");
    assert_eq!(lan["panel"], "/plugins/interfaces/edit?network=lan");
    assert_eq!(lan["cells"][6]["actions"][2]["href"], lan["panel"]);
    assert_eq!(
        device_row(&m, "br-lan")["panel"],
        "/plugins/interfaces/edit?device=br-lan"
    );
    // No row unfolds, and none carries an info act.
    let listing = serde_json::to_string(&page::listing(&m)).unwrap();
    assert!(!listing.contains("\"expanded\""), "{listing}");
    assert!(!listing.contains("\"info\""), "{listing}");
}
#[test]
fn a_networks_row_reads_and_acts_on_its_companion() {
    let mut m = lab();
    m.live["interfaces"][3]["up"] = json!(false);
    assert_eq!(texts(&network_row(&m, "wan"))[5], "partial");
    // Restarting the line restarts both halves.
    let line = |path: &str, query: &str| {
        let mut r = request(path, query);
        r.snapshot = Snapshot::from_value(json!({
            "network":{
                "wan":{".name":"wan",".type":"interface","device":"wan0","proto":"dhcp"},
                "wan6":{".name":"wan6",".type":"interface","device":"wan0","proto":"dhcpv6"}},
            "firewall":{"wan":{".name":"wan",".type":"zone","name":"wan","network":["wan","wan6"]}}
        }));
        r.ubus = Ubus::from_value(json!({"networkState":{"devices":{},"interfaces":[
            {"interface":"wan","l3_device":"wan0","up":true},
            {"interface":"wan6","l3_device":"wan0","up":true}]}}));
        r
    };
    for action in ["restart", "down"] {
        let e = post(&line("/", ""), &Form::parse(&format!("{action}=wan")));
        let named: Vec<_> = e
            .commands
            .iter()
            .map(|c| (c.name.clone(), c.args["interface"].clone()))
            .collect();
        assert_eq!(
            named,
            [
                (format!("interface-{action}"), "wan".to_string()),
                (format!("interface-{action}"), "wan6".to_string()),
            ]
        );
    }
    // Deleting the line names its half and removes both.
    let d = line("/delete", "network=wan");
    let confirm = serde_json::to_string(&get(&d)).unwrap();
    assert!(confirm.contains("wan6"), "{confirm}");
    let e = post(&d, &Form::parse("delete=1"));
    let removed: Vec<_> = e
        .commit
        .iter()
        .filter(|o| o.delete && o.config == "network")
        .map(|o| o.section.clone())
        .collect();
    assert_eq!(removed, ["wan", "wan6"]);
    let zone = e.commit.iter().find(|o| o.config == "firewall").unwrap();
    assert_eq!(zone.values["network"], serde_json::Value::Null);
}
#[test]
fn tunnel_writes_real_protocol_options() {
    let e = post(
        &request("/new", "kind=tunnel"),
        &Form::parse("name=remote&proto=vxlan&peeraddr=203.0.113.8&vid=100&port=4789&auto=1"),
    );
    let j = serde_json::to_value(e).unwrap();
    assert_eq!(j["commit"][0]["values"]["proto"], "vxlan", "{j}");
    assert_eq!(j["commit"][0]["values"]["peeraddr"], "203.0.113.8");
}

#[test]
fn editing_bridge_keeps_its_existing_ports() {
    let e = post(
        &request("/edit", "device=br-lan"),
        &Form::parse("name=br-lan&ports=eth0&stp=1"),
    );
    assert!(!e.commit.is_empty(), "{e:?}");
    assert_eq!(e.commit[0].values["ports"], json!(["eth0"]));
}
#[test]
fn runtime_actions_do_not_change_autostart() {
    for action in ["restart", "up", "down"] {
        let e = post(&request("/", ""), &Form::parse(&format!("{action}=lan")));
        assert!(e.commit.is_empty());
        assert_eq!(e.commands[0].name, format!("interface-{action}"));
        assert_eq!(e.commands[0].args["interface"], "lan");
        assert_eq!(e.back.unwrap().href, ROOT);
    }
    assert!(post(&request("/", ""), &Form::parse("down=loopback"))
        .commands
        .is_empty());
}

#[test]
fn preview_uses_the_values_that_save_stages() {
    let e = post(&request("/new", "kind=network"), &form(""));
    let value = serde_json::to_value(&e).unwrap();
    fn preview(v: &serde_json::Value) -> Option<&str> {
        if v.get("live") == Some(&json!(true)) && v.get("type") == Some(&json!("code")) {
            return v["value"].as_str();
        }
        match v {
            serde_json::Value::Object(m) => m.values().find_map(preview),
            serde_json::Value::Array(a) => a.iter().find_map(preview),
            _ => None,
        }
    }
    let text = preview(&value).expect("editor must carry a live preview");
    assert!(text.contains("config interface 'guest'"));
    for (key, val) in e.commit[0].values.as_object().unwrap() {
        if let Some(value) = val.as_str() {
            assert!(
                text.contains(&format!("option {key} '{value}'")),
                "missing {key}"
            );
        }
    }
}

#[test]
fn a_new_network_cannot_have_an_empty_name() {
    let e = post(
        &request("/new", "kind=wan"),
        &Form::parse("name=&device=eth2&proto=dhcp&ra=disabled&dhcpv6=disabled&auto=1"),
    );
    assert!(e.commit.is_empty());
    assert!(serde_json::to_string(&e)
        .unwrap()
        .contains("starting with a letter"));
}

#[test]
fn runtime_flags_win_over_staged_autostart_and_device_state() {
    let mut m = Model::read(&request("/", ""));
    m.networks
        .iter_mut()
        .find(|n| n.id == "lan")
        .unwrap()
        .values
        .insert("auto".into(), json!("0"));
    m.live["interfaces"][0]["up"] = json!(true);
    m.live["devices"]["br-lan"]["up"] = json!(true);
    m.live["devices"]["br-lan"]["carrier"] = json!(true);
    let state = |m: &Model| texts(&network_row(m, "lan"))[5].clone();
    let row = network_row(&m, "lan");
    assert_eq!(state(&m), "up");
    // The state's act leads the row's acts, as a rule's power glyph does.
    assert_eq!(row["cells"][6]["actions"][0]["name"], "down");
    let icons: Vec<&str> = row["cells"][6]["actions"]
        .as_array()
        .expect("acts")
        .iter()
        .map(|a| a["icon"].as_str().unwrap_or_default())
        .collect();
    assert_eq!(icons, ["power", "refresh-cw", "square-pen", "trash-2"]);
    m.live["interfaces"][0]["up"] = json!(false);
    assert_eq!(state(&m), "down");
    m.live["interfaces"][0]["pending"] = json!(true);
    assert_eq!(state(&m), "pending");
    m.live["interfaces"][0]["pending"] = json!(false);
    m.live["devices"]["br-lan"]["carrier"] = json!(false);
    assert_eq!(state(&m), "no link");
}
#[test]
fn addresses_are_observed_and_ipv6_assignments_stay_in_details() {
    let mut m = Model::read(&request("/", ""));
    m.live["interfaces"][0]["ipv6-prefix-assignment"] =
        json!([{"local-address":{"address":"fd42:7ea:aa00::1","mask":60}}]);
    m.live["devices"]["br-lan"]["type"] = json!("bridge");
    assert_eq!(texts(&device_row(&m, "br-lan"))[1], "bridge");
    assert_eq!(texts(&network_row(&m, "lan"))[2], "192.168.1.1/24");
    assert!(cards(&drawer_of(&m, "lan", ""))[0]
        .to_string()
        .contains("fd42:7ea:aa00::1/60"));
    m.live["interfaces"][0]["ipv4-address"] = json!([]);
    assert_eq!(texts(&network_row(&m, "lan"))[2], "—");
    m.live = serde_json::Value::Null;
    assert_eq!(texts(&network_row(&m, "lan"))[5], "not reported");
}
#[test]
fn physical_bridge_ports_are_roots_and_vlan_and_ppp_follow_their_transport() {
    let mut m = Model::read(&request("/", ""));
    m.live["devices"]["eth0.20"] = json!({"kind":"vlan","parent":"eth0","up":true});
    m.live["interfaces"]
        .as_array_mut()
        .unwrap()
        .push(json!({"interface":"uplink","l3_device":"pppoe-wan","up":true}));
    m.networks.push(model::Record {
        id: "iot".into(),
        values: json!({"device":"eth0.20","proto":"static"})
            .as_object()
            .unwrap()
            .clone(),
        ..Default::default()
    });
    let parents = m.parents();
    assert!(!parents.contains_key("eth0"));
    assert_eq!(parents["eth0.20"], "eth0");
    assert_eq!(parents["pppoe-wan"], "eth1");
    assert!(follows(&m, "eth0.20", "eth0"));
    assert_eq!(texts(&device_row(&m, "pppoe-wan"))[2], "[uplink]");
}

#[test]
fn home_router_tree_matches_the_reference_relationships() {
    let mut r = request("/", "");
    r.snapshot = Snapshot::from_value(json!({"network":{
        "lan_bridge":{".name":"lan_bridge",".type":"device","name":"br-lan","type":"bridge","ports":["eth0","wlan0"]},
        "iptv_bridge":{".name":"iptv_bridge",".type":"device","name":"br-iptv","type":"bridge","ports":["eth4.3999","br-lan.3999"]},
        "wan":{".name":"wan",".type":"interface","device":"eth4.3900","proto":"pppoe"},
        "iot":{".name":"iot",".type":"interface","device":"br-lan.10","proto":"static"}
    }}));
    r.ubus = Ubus::from_value(json!({"networkState":{
        "devices":{
            "eth0":{"physical":true,"kind":"port","parent":"eth4"},
            "eth4":{"physical":true,"kind":"port"},
            "wlan0":{"physical":false,"kind":"wifi"},
            "br-lan":{"kind":"bridge","parent":"eth0"},
            "br-iptv":{"kind":"bridge","parent":"eth4.3999"},
            "eth4.3900":{"kind":"vlan","parent":"eth4"},
            "eth4.3999":{"kind":"vlan","parent":"eth4"},
            "br-lan.3999":{"kind":"vlan","parent":"br-lan"},
            "br-lan.10":{"kind":"vlan","parent":"br-lan"},
            "br-lan.40":{"kind":"vlan","parent":"br-lan"}
        },
        "interfaces":[{"interface":"wan","device":"eth4.3900","l3_device":"pppoe-wan","up":true}]
    }}));
    let m = Model::read(&r);
    assert_eq!(
        m.parents(),
        std::collections::BTreeMap::from([
            ("eth4.3900".into(), "eth4".into()),
            ("pppoe-wan".into(), "eth4.3900".into()),
            ("eth4.3999".into(), "br-iptv".into()),
            ("br-lan.3999".into(), "br-iptv".into()),
            ("br-lan.10".into(), "br-lan".into()),
            ("br-lan.40".into(), "br-lan".into()),
        ])
    );
    // What a device carries follows it down the listing.
    assert!(follows(&m, "pppoe-wan", "eth4.3900"));
    assert!(follows(&m, "eth4.3900", "eth4"));
    assert!(follows(&m, "eth4.3999", "br-iptv"));
    assert!(follows(&m, "br-lan.10", "br-lan"));
}
// follows says the Devices card reads `child` after `parent`, as everything
// a device carries comes after it.
fn follows(m: &Model, child: &str, parent: &str) -> bool {
    let ids = ids(&card(m, "Devices"));
    let at = |id: &str| ids.iter().position(|x| x == id);
    matches!((at(parent), at(child)), (Some(p), Some(c)) if c > p)
}

#[test]
fn docker_bridge_connects_only_its_observed_virtual_member() {
    let mut m = Model::read(&request("/", ""));
    m.live["devices"] = json!({
        "br-lan":{"kind":"bridge","bridge-members":["lan0"],"up":true},
        "lan0":{"kind":"virtual","physical":false,"up":true},
        "wan0":{"kind":"virtual","physical":false,"up":true},
        "eth0":{"kind":"virtual","physical":false,"up":true}
    });
    assert_eq!(
        m.parents(),
        std::collections::BTreeMap::from([("lan0".into(), "br-lan".into())])
    );
    assert!(follows(&m, "lan0", "br-lan"));
    // An observed empty bridge overrides ports still in the UCI snapshot.
    m.live["devices"]["br-lan"]["bridge-members"] = json!([]);
    assert!(m.parents().is_empty());
    // A member reported only by netifd still belongs in the inventory.
    m.live["devices"]["br-lan"]["bridge-members"] = json!(["tap-only"]);
    assert!(m.names().contains("tap-only"));
}

// A network's DHCP server is the DHCP page's: the interface's editor and its
// drawer say nothing of it, so a server is read and changed in one place.
#[test]
fn the_interface_leaves_its_dhcp_server_to_the_dhcp_page() {
    let mut r = request("/", "");
    let mut snapshot = fixture();
    snapshot["network"]["guest"] = json!({".name":"guest",".type":"interface","device":"eth2",
        "proto":"static","ipaddr":"192.168.20.1","netmask":"255.255.255.0"});
    r.snapshot = Snapshot::from_value(snapshot);
    let m = Model::read(&r);
    for (what, body) in [
        // The listing behind keeps the row's DHCP chip; the drawer itself
        // says nothing of the server.
        ("lan's drawer", drawer_of(&m, "lan", "").to_string()),
        ("guest's drawer", drawer_of(&m, "guest", "").to_string()),
        ("br-lan's drawer", drawer_of(&m, "", "br-lan").to_string()),
    ] {
        for gone in [
            "DHCP server",
            "Configure DHCP",
            "Add DHCP server",
            "/plugins/dnsdhcp/",
            "\"name\":\"leasetime\"",
            "\"name\":\"dhcpv6\"",
        ] {
            assert!(!body.contains(gone), "the {what} carries {gone}: {body}");
        }
    }
}
// drawer_of is the drawer of a network or a device, as its pencil opens it.
fn drawer_of(m: &Model, network: &str, device: &str) -> serde_json::Value {
    open_drawer(&editor::edit(m, network, device))
}
// cards are a drawer's cards in order: the editor form's sections, the
// preview at its foot left out.
fn cards(drawer: &serde_json::Value) -> Vec<serde_json::Value> {
    drawer["children"][0]["fields"]
        .as_array()
        .unwrap()
        .iter()
        .filter(|f| f["type"] == "section")
        .cloned()
        .collect()
}
fn titles(drawer: &serde_json::Value) -> Vec<String> {
    cards(drawer)
        .iter()
        .map(|c| c["title"].as_str().unwrap_or_default().to_string())
        .collect()
}
// fact is the value a card states under label.
fn fact(card: &serde_json::Value, label: &str) -> serde_json::Value {
    card["children"][0]["items"]
        .as_array()
        .unwrap()
        .iter()
        .find(|p| p["label"] == label)
        .cloned()
        .unwrap_or_else(|| panic!("no {label} in {card}"))
}
// names are the controls a card holds, by name, in order.
fn names(card: &serde_json::Value) -> Vec<String> {
    fn walk(v: &serde_json::Value, out: &mut Vec<String>) {
        if let Some(n) = v.get("name").and_then(|n| n.as_str()) {
            if v.get("type").is_some() && v.get("kind").is_none_or(|k| k != "hidden") {
                out.push(n.into());
            }
        }
        match v {
            serde_json::Value::Object(o) => o.values().for_each(|x| walk(x, out)),
            serde_json::Value::Array(a) => a.iter().for_each(|x| walk(x, out)),
            _ => {}
        }
    }
    let mut out = vec![];
    walk(&card["children"], &mut out);
    out.dedup();
    out
}
#[test]
fn status_card_states_live_facts_only() {
    let mut m = lab();
    m.live["devices"]["br-lan"]["macaddr"] = json!("52:54:00:12:34:56");
    m.live["devices"]["br-lan"]["statistics"] = json!({"rx_errors":0,"tx_errors":2});
    m.live["interfaces"][1]["uptime"] = json!(3600);
    m.live["interfaces"][3]["ipv6-address"] = json!([{"address":"2001:db8::5","mask":128}]);
    let lan = cards(&drawer_of(&m, "lan", ""))[0].clone();
    assert_eq!(lan["title"], "Status");
    assert_eq!(lan["icon"], "network");
    assert_eq!(
        labels(&lan),
        ["State", "IPv4", "IPv6", "ULA", "Uptime"],
        "nothing the form sets"
    );
    assert_eq!(fact(&lan, "State")["value"], "up");
    assert_eq!(fact(&lan, "IPv4")["value"], "192.168.77.1/24");
    assert_eq!(fact(&lan, "IPv4")["copy"], true);
    assert_eq!(fact(&lan, "Uptime")["duration"], 3600);
    // The line's DHCPv6 half's address is the line's.
    let wan = cards(&drawer_of(&m, "wan", ""))[0].clone();
    assert_eq!(wan["icon"], "globe");
    assert_eq!(fact(&wan, "IPv6")["value"], "2001:db8::5/128");
    // A device reads as itself: its MAC address, a bridge's live ports, its
    // error counts; a port its link and driver.
    let bridge = cards(&drawer_of(&m, "", "br-lan"))[0].clone();
    assert_eq!(bridge["title"], "Status");
    assert_eq!(bridge["icon"], "git-merge");
    assert_eq!(labels(&bridge), ["State", "MAC address", "Ports", "Errors"]);
    assert_eq!(fact(&bridge, "MAC address")["value"], "52:54:00:12:34:56");
    assert_eq!(fact(&bridge, "MAC address")["copy"], true);
    assert_eq!(fact(&bridge, "Ports")["value"], "lan0");
    assert_eq!(fact(&bridge, "Errors")["value"], "RX 0 · TX 2");
    let port = cards(&drawer_of(&m, "", "lan0"))[0].clone();
    assert_eq!(
        labels(&port),
        ["State", "MAC address", "Link", "Driver", "Errors"]
    );
    assert_eq!(fact(&port, "Link")["rate"], 1e9);
    // netifd's own refusal stands in the card, as it says it.
    m.live["interfaces"][1]["errors"] = json!([{"code":"NO_DEVICE"}]);
    let refused = cards(&drawer_of(&m, "lan", ""))[0].to_string();
    assert!(refused.contains("NO_DEVICE"), "{refused}");
}
#[test]
fn a_network_drawer_reads_status_general_ipv4_ipv6_advanced() {
    let m = lab();
    let lan = drawer_of(&m, "lan", "");
    assert_eq!(lan["title"], "lan");
    assert_eq!(titles(&lan), ["Status", "", "IPv4", "IPv6", "Advanced"]);
    let general = &cards(&lan)[1];
    assert_eq!(names(general), ["auto", "name", "device", "zone"]);
    assert_eq!(names(&cards(&lan)[2])[0], "proto");
    assert_eq!(names(&cards(&lan)[4]), ["mtu", "metric", "macaddr"]);
    // A new one has nothing to report yet.
    let fresh = open_drawer(&editor::new(&m, "network"));
    assert_eq!(titles(&fresh), ["", "IPv4", "IPv6", "Advanced"]);
}
#[test]
fn a_device_drawer_reads_status_kind_advanced() {
    let m = lab();
    let bridge = drawer_of(&m, "", "br-lan");
    assert_eq!(titles(&bridge), ["Status", "", "Advanced"]);
    assert_eq!(names(&cards(&bridge)[1]), ["ports", "stp", "igmp_snooping"]);
    assert_eq!(names(&cards(&bridge)[2]), ["mtu", "macaddr"]);
    // A device keeps its name: the drawer's title says it, and it is posted
    // back as it is.
    assert!(
        bridge.to_string().contains("\"kind\":\"hidden\""),
        "{bridge}"
    );
    // A plain port has no settings of its kind.
    let port = drawer_of(&m, "", "eth0");
    assert_eq!(titles(&port), ["Status", "Advanced"]);
    // A new bridge is named as it is made.
    let fresh = open_drawer(&editor::new(&m, "bridge"));
    assert!(names(&cards(&fresh)[0]).contains(&"name".to_string()));
}
#[test]
fn status_acts_run_and_return_to_the_listing() {
    let line = |query: &str| {
        let mut r = request("/edit", query);
        r.snapshot = Snapshot::from_value(json!({
            "network":{
                "lan":{".name":"lan",".type":"interface","device":"br-lan","proto":"static","ipaddr":"192.168.1.1","netmask":"255.255.255.0"},
                "wan":{".name":"wan",".type":"interface","device":"wan0","proto":"dhcp"},
                "wan6":{".name":"wan6",".type":"interface","device":"wan0","proto":"dhcpv6"}}
        }));
        r.ubus = Ubus::from_value(json!({"networkState":{"devices":{},"interfaces":[
            {"interface":"lan","l3_device":"br-lan","up":true},
            {"interface":"wan","l3_device":"wan0","up":true},
            {"interface":"wan6","l3_device":"wan0","up":true}]}}));
        r
    };
    // The card's header carries the acts, each posting the editor's form.
    let lan = open_drawer(&get(&line("network=lan")));
    let acts: Vec<_> = cards(&lan)[0]["control"]["children"]
        .as_array()
        .unwrap()
        .iter()
        .map(|b| (b["label"].clone(), b["name"].clone(), b["value"].clone()))
        .collect();
    assert_eq!(
        acts,
        [
            (json!("Restart"), json!("restart"), json!("lan")),
            (json!("Bring down"), json!("down"), json!("lan")),
        ]
    );
    assert_eq!(cards(&lan)[0]["control"]["inline"], true);
    // Pressing one runs it, stages nothing of what was typed, and returns to
    // the live listing, where the state it changes is read.
    let e = post(
        &line("network=lan"),
        &Form::parse("restart=lan&name=lan&device=br-lan&proto=static&ipaddr=192.168.1.1&netmask=255.255.255.0&mtu=1400"),
    );
    assert!(e.commit.is_empty());
    let run: Vec<_> = e
        .commands
        .iter()
        .map(|c| (c.name.as_str(), c.args["interface"].as_str()))
        .collect();
    assert_eq!(run, [("interface-restart", "lan")]);
    assert_eq!(e.back.as_ref().unwrap().href, ROOT);
    assert!(e.notice.is_some());
    // The line restarts with its DHCPv6 half.
    let e = post(&line("network=wan"), &Form::parse("restart=wan&name=wan"));
    let run: Vec<_> = e
        .commands
        .iter()
        .map(|c| c.args["interface"].as_str())
        .collect();
    assert_eq!(run, ["wan", "wan6"]);
}
#[test]
fn drawer_addresses() {
    let m = lab();
    let at = |path: &str, query: &str| {
        let mut r = request(path, query);
        r.snapshot = Snapshot::from_value(json!({"network":{
            "br":{".name":"br",".type":"device","name":"br-lan","type":"bridge","ports":["lan0"]},
            "lan":{".name":"lan",".type":"interface","device":"br-lan","proto":"static"},
            "wan":{".name":"wan",".type":"interface","device":"wan0","proto":"dhcp"},
            "wan6":{".name":"wan6",".type":"interface","device":"wan0","proto":"dhcpv6"}}}));
        r.ubus = Ubus::from_value(json!({"networkState": m.live.clone()}));
        get(&r)
    };
    let lan = at("/edit", "network=lan");
    assert_eq!(open_drawer(&lan)["title"], "lan");
    assert!(
        serde_json::to_string(&lan)
            .unwrap()
            .contains("\"id\":\"wan\""),
        "the listing stands behind it"
    );
    assert_eq!(open_drawer(&lan)["closed"], ROOT);
    // An old details address opens the device's drawer.
    let bridge = open_drawer(&at("/details", "device=br-lan"));
    assert_eq!(bridge["title"], "br-lan");
    assert!(
        bridge.to_string().contains("\"name\":\"ports\""),
        "{bridge}"
    );
    // A companion's address is its line's.
    assert_eq!(open_drawer(&at("/edit", "network=wan6"))["title"], "wan");
    // A name the router no longer has says so and leads back.
    for (path, query) in [("/edit", "network=nosuch"), ("/details", "device=nosuch")] {
        let gone = serde_json::to_value(at(path, query)).unwrap();
        assert_eq!(
            gone["notice"]["text"],
            "This interface no longer exists. Return to Interfaces and try again."
        );
    }
}
#[test]
fn the_devices_card_carries_add_device() {
    let control = card(&lab(), "Devices")["control"].clone();
    assert_eq!(control["label"], "Add device");
    assert_eq!(control["icon"], "plus");
    assert_eq!(control["href"], format!("{ROOT}new?kind=bridge"));
    assert_eq!(control["panel"], true);
    // Its drawer opens as a new bridge, and a VLAN saves as a device.
    assert_eq!(
        open_drawer(&get(&request("/new", "kind=bridge")))["title"],
        "New bridge"
    );
    let e = post(
        &request("/new", "kind=vlan"),
        &Form::parse("kind=vlan&name=eth2.20&device=eth2&vid=20&vlan_protocol=8021q"),
    );
    assert_eq!(e.commit[0].section_type, "device", "{e:?}");
}
// wan_form is wan's drawer posted as it stands, extra overriding it (a form
// reads a field's first value).
fn wan_form(extra: &str) -> Form {
    Form::parse(&format!(
        "{extra}&name=wan&device=wan0&proto=dhcp&zone=wan&auto=1&peerdns=1&defaultroute=1\
         &ip6_reqaddress=try&ip6_reqprefix=56&ip6_peerdns=1"
    ))
}
#[test]
fn wans_ipv6_card_edits_wan6() {
    let m = lab();
    let wan = drawer_of(&m, "wan", "");
    let ipv6 = cards(&wan)
        .into_iter()
        .find(|c| c["title"] == "IPv6")
        .unwrap();
    // The card names the section it writes.
    assert_eq!(ipv6["meta"], "wan6");
    assert_eq!(
        names(&ipv6),
        ["ip6_reqaddress", "ip6_reqprefix", "ip6_peerdns"]
    );
    assert_eq!(value_of(&ipv6, "ip6_reqaddress"), "try");
    assert_eq!(value_of(&ipv6, "ip6_reqprefix"), "56");
    // Each control's key chip is the option it writes in wan6.
    assert!(ipv6.to_string().contains("\"key\":\"reqprefix\""), "{ipv6}");
    // The LAN hands out a prefix of its own.
    let lan = drawer_of(&m, "lan", "");
    let ipv6 = cards(&lan)
        .into_iter()
        .find(|c| c["title"] == "IPv6")
        .unwrap();
    assert_eq!(names(&ipv6), ["ip6assign"]);
}
#[test]
fn one_save_stages_the_network_and_its_companion() {
    let e = post(
        &lab_at("/edit", "network=wan"),
        &wan_form("ip6_reqprefix=60"),
    );
    let ops: Vec<_> = e
        .commit
        .iter()
        .filter(|o| o.config == "network")
        .map(|o| (o.section.clone(), o.values.clone()))
        .collect();
    let sections: Vec<_> = ops.iter().map(|(s, _)| s.as_str()).collect();
    assert_eq!(sections, ["wan", "wan6"], "{e:?}");
    assert_eq!(ops[1].1["reqprefix"], "60");
    assert_eq!(ops[1].1["reqaddress"], "try");
    assert_eq!(ops[1].1["peerdns"], "1");
    // The preview shows both sections.
    let drawer = open_drawer(&e).to_string();
    assert!(drawer.contains("config interface 'wan6'"), "{drawer}");
    // A refusal lands on the companion's own field.
    let refused = post(
        &lab_at("/edit", "network=wan"),
        &wan_form("ip6_reqprefix=33"),
    );
    assert!(refused.commit.is_empty());
    let ipv6 = cards(&open_drawer(&refused))
        .into_iter()
        .find(|c| c["title"] == "IPv6")
        .unwrap()
        .to_string();
    assert!(ipv6.contains("\"error\":\"Choose"), "{ipv6}");
}
#[test]
fn zone_and_rename_carry_the_companion() {
    let e = post(&lab_at("/edit", "network=wan"), &wan_form("zone=wan2"));
    let zone = |name: &str| {
        e.commit
            .iter()
            .find(|o| o.config == "firewall" && o.section == name)
            .map(|o| o.values["network"].clone())
    };
    assert_eq!(zone("wan"), Some(serde_json::Value::Null));
    assert_eq!(zone("wan2"), Some(json!(["wan", "wan6"])));
    // Under PPPoE the half rides the line by name, and follows it.
    let mut r = lab_at("/edit", "network=wan");
    let mut snapshot = fixture();
    snapshot["network"] = json!({
        "wan":{".name":"wan",".type":"interface","device":"wan0","proto":"pppoe","username":"u","password":"p"},
        "wan6":{".name":"wan6",".type":"interface","device":"@wan","proto":"dhcpv6"}
    });
    r.snapshot = Snapshot::from_value(snapshot);
    let e = post(
        &r,
        &Form::parse("name=isp&device=wan0&proto=pppoe&username=u&zone=wan&auto=1&ip6_reqaddress=try&ip6_reqprefix=auto&ip6_peerdns=1"),
    );
    let half = e
        .commit
        .iter()
        .find(|o| o.config == "network" && o.section == "wan6")
        .unwrap_or_else(|| panic!("{e:?}"));
    assert_eq!(half.values["device"], "@isp");
}
// labels are a card's facts in reading order.
fn labels(card: &serde_json::Value) -> Vec<String> {
    card["children"][0]["items"]
        .as_array()
        .unwrap()
        .iter()
        .map(|p| p["label"].as_str().unwrap().to_string())
        .collect()
}
#[test]
fn a_dhcp_read_at_an_unknown_version_still_draws_the_row() {
    // The DNS and DHCP plugin's read at a version this plugin does not know,
    // or none at all, still draws the row, its DHCP chip taking no state from
    // it.
    let mut r = request("/", "");
    for ubus in [
        json!({"dnsdhcp.dhcp-state":{"version":2,"result":{"networks":{"lan":{"state":"running","enabled":true,"pool":"192.168.1.100–192.168.1.249"}}}}}),
        json!({}),
    ] {
        r.ubus = Ubus::from_value(ubus);
        let row = network_row(&Model::read(&r), "lan");
        let chip = row["cells"][0]["chips"]
            .as_array()
            .unwrap()
            .iter()
            .find(|c| c["label"] == "DHCP")
            .cloned()
            .expect("lan still has its server");
        assert_ne!(chip["tone"], "success", "{chip}");
    }
}
// open_drawer is the one panel the page's act says is open.
fn open_drawer(e: &Envelope) -> serde_json::Value {
    let j = serde_json::to_value(e).unwrap();
    let act = j["act"].clone();
    assert_eq!(act["drawer"]["open"], true, "{j}");
    act["drawer"].clone()
}
#[test]
fn an_object_is_edited_in_a_drawer_over_the_listing() {
    let m = Model::read(&request("/", ""));
    // The editor's address shows the listing with the object's drawer open:
    // the form measure, closed on the listing's own address, the form's
    // preview at its foot.
    for (query, title) in [("network=lan", "lan"), ("network=&device=br-lan", "br-lan")] {
        let e = editor::edit(
            &m,
            &Form::parse(query).get("network"),
            &Form::parse(query).get("device"),
        );
        let drawer = open_drawer(&e);
        assert_eq!(drawer["title"], title);
        assert_eq!(drawer["closed"], ROOT);
        let form = &drawer["children"][0];
        assert_eq!(form["type"], "form", "{drawer}");
        assert_eq!(form["submit"], "Save interface", "{drawer}");
        let last = form["fields"].as_array().unwrap().last().unwrap();
        assert_eq!(last["type"], "code");
        assert_eq!(last["live"], true);
        assert_eq!(last["grammar"], "uci");
        assert_eq!(last["label"], "/etc/config/network");
        // The listing stands behind it.
        let j = serde_json::to_string(&e).unwrap();
        assert!(j.contains("\"id\":\"br-lan\""), "{j}");
    }
    // So does a new one, from the kind chosen.
    assert_eq!(
        open_drawer(&editor::new(&m, "bridge"))["title"],
        "New bridge"
    );
    // A refused save comes back as the drawer, still open.
    let refused = post(
        &request("/edit", "network=lan"),
        &Form::parse("name=lan&device=br-lan&proto=static&ipaddr=invalid"),
    );
    assert!(refused.commit.is_empty());
    let drawer = open_drawer(&refused).to_string();
    assert!(
        drawer.contains("\"error\":\""),
        "the refused field says why: {drawer}"
    );
    assert!(!drawer.contains("Check the highlighted fields."));
}
#[test]
fn the_editors_switches_read_as_netifd_reads_them() {
    fn switch(v: &serde_json::Value, name: &str) -> Option<bool> {
        match v {
            serde_json::Value::Object(o)
                if o.get("type") == Some(&json!("switch"))
                    && o.get("name") == Some(&json!(name)) =>
            {
                Some(o.get("on") == Some(&json!(true)))
            }
            serde_json::Value::Object(o) => o.values().find_map(|x| switch(x, name)),
            serde_json::Value::Array(a) => a.iter().find_map(|x| switch(x, name)),
            _ => None,
        }
    }
    let m = Model::read(&Request {
        path: "/".into(),
        query: Form::default(),
        snapshot: Snapshot::from_value(json!({"network": {
            "uplink": {".name": "uplink", ".type": "interface", "device": "eth1", "proto": "dhcp",
                "auto": "false", "peerdns": "0", "defaultroute": "no"},
            "br": {".name": "br", ".type": "device", "name": "br-lan", "type": "bridge",
                "ports": ["eth0"], "stp": "yes", "igmp_snooping": "true"}
        }})),
        ubus: Ubus::from_value(json!({})),
        plugins: Default::default(),
    });
    // netifd's uci reader takes 1/true and 0/false; any other spelling leaves
    // the option at netifd's default.
    let uplink = serde_json::to_value(editor::edit(&m, "uplink", "")).unwrap();
    assert_eq!(switch(&uplink, "auto"), Some(false));
    assert_eq!(switch(&uplink, "peerdns"), Some(false));
    assert_eq!(switch(&uplink, "defaultroute"), Some(true));
    let bridge = serde_json::to_value(editor::edit(&m, "", "br-lan")).unwrap();
    assert_eq!(switch(&bridge, "stp"), Some(false));
    assert_eq!(switch(&bridge, "igmp_snooping"), Some(true));
}
// A device OpenVPN makes when it connects is the VPN's, not the network
// config's: its drawer says whose it is and leads there, rather than offering
// a name, an MTU and a MAC the tunnel would overwrite.
#[test]
fn a_device_run_by_openvpn_hands_off_to_its_vpn_panel() {
    let mut r = request("/edit", "device=tun0");
    r.ubus = Ubus::from_value(json!({
        "vpn.state": {"version": 1, "result": {"tunnels": [{"device": "tun0", "kind": "openvpn", "instance": "proton", "up": true}]}}
    }));
    let drawer = open_drawer(&get(&r));
    assert_eq!(drawer["title"], "tun0");
    let text = drawer.to_string();
    assert!(text.contains("/plugins/vpn/?open=proton"), "{text}");
    assert!(text.contains("proton"), "{text}");
    assert!(!text.contains("\"mtu\""), "{text}");
    assert!(!text.contains("macaddr"), "{text}");
}
// The VPN plugin's read at a version this plugin does not know is no read: the
// device is never handed to a panel it may misname.
#[test]
fn a_vpn_read_at_an_unknown_version_hands_nothing_off() {
    let mut r = request("/edit", "device=tun0");
    r.ubus = Ubus::from_value(json!({
        "vpn.state": {"version": 2, "result": {"tunnels": [{"device": "tun0", "kind": "openvpn", "instance": "proton", "up": true}]}}
    }));
    let text = serde_json::to_string(&get(&r)).unwrap();
    assert!(!text.contains("/plugins/vpn/?open=proton"), "{text}");
}
/// A change to an interface reads as its settings and is about that
/// interface, by its name; one to anything else in network (a bridge, the
/// globals) names no subject.
#[test]
fn an_interface_change_is_about_that_interface() {
    let snapshot = Snapshot::from_value(json!({"network": {
        "lan": {".name": "lan", ".type": "interface", "proto": "static"},
        "br": {".name": "br", ".type": "device", "name": "br-lan"}
    }}));
    let change = |section: &str, option: &str, value: &str| Change {
        config: "network".into(),
        op: "set".into(),
        section: section.into(),
        option: option.into(),
        value: value.into(),
    };
    let out = describe(
        &[
            change("lan", "ipaddr", "192.168.2.1"),
            change("br", "mtu", "1400"),
        ],
        &snapshot,
    );
    let read: Vec<_> = out
        .iter()
        .map(|d| {
            (
                d.plain.as_str(),
                d.subject.clone().map(|s| (s.kind, s.name)),
            )
        })
        .collect();
    assert_eq!(
        read,
        vec![
            (
                "Changed interface settings",
                Some((SubjectKind::Network, "lan".to_string()))
            ),
            ("Changed interface settings", None),
        ]
    );
}

// A port's link speed travels as a number, so the shell writes it in the
// reader's language; a port that reports none carries no rate.
#[test]
fn a_ports_link_speed_is_a_rate_the_shell_writes() {
    let mut m = Model::read(&request("/", ""));
    m.live["devices"]["eth2"] = json!({"physical":true,"kind":"port","speed":"2500F"});
    m.live["devices"]["eth1"] = json!({"physical":true,"kind":"port","up":true});
    let row = device_row(&m, "eth2");
    assert_eq!(row["cells"][1]["text"], "port");
    assert_eq!(row["cells"][1]["rate"], 2.5e9);
    assert!(device_row(&m, "eth1")["cells"][1].get("rate").is_none());
}

// A network's uptime is handed over in seconds, which the shell writes as
// every duration is written; a network netifd reports no uptime for says so.
#[test]
fn a_networks_uptime_is_seconds_for_the_shell_to_write() {
    let up = status::uptime(Some(&json!({"uptime": 4_523_580})));
    assert_eq!(up.label, "Uptime");
    assert_eq!(up.duration, Some(4_523_580));
    let unknown = status::uptime(None);
    assert_eq!((unknown.duration, unknown.value.as_str()), (None, "—"));
}

// with_networks is the base request with its network config replaced.
fn with_networks(network: serde_json::Value) -> Model {
    let mut r = request("/", "");
    let mut snapshot = fixture();
    snapshot["network"] = network;
    r.snapshot = Snapshot::from_value(snapshot);
    Model::read(&r)
}
// An internet line's DHCPv6 half is its companion: on the same device, or
// riding the line by name as it does under PPPoE.
#[test]
fn a_dhcpv6_network_on_the_same_device_is_its_networks_companion() {
    let m = with_networks(json!({
        "wan":{".name":"wan",".type":"interface","device":"wan0","proto":"dhcp"},
        "wan6":{".name":"wan6",".type":"interface","device":"wan0","proto":"dhcpv6"},
        "lan":{".name":"lan",".type":"interface","device":"br-lan","proto":"static"}
    }));
    assert_eq!(m.companion("wan").map(|r| r.id.as_str()), Some("wan6"));
    assert_eq!(m.companion_of("wan6"), Some("wan"));
    assert!(m.companion("lan").is_none());
    assert!(m.companion_of("wan").is_none());
    let pppoe = with_networks(json!({
        "wan":{".name":"wan",".type":"interface","device":"eth1","proto":"pppoe"},
        "wan6":{".name":"wan6",".type":"interface","device":"@wan","proto":"dhcpv6"}
    }));
    assert_eq!(pppoe.companion("wan").map(|r| r.id.as_str()), Some("wan6"));
    assert_eq!(pppoe.companion_of("wan6"), Some("wan"));
}
// Where it is not plain whose half a DHCPv6 network is, it is no one's.
#[test]
fn contended_dhcpv6_networks_stay_their_own() {
    let two_halves = with_networks(json!({
        "wan":{".name":"wan",".type":"interface","device":"wan0","proto":"dhcp"},
        "wan6":{".name":"wan6",".type":"interface","device":"wan0","proto":"dhcpv6"},
        "wan6b":{".name":"wan6b",".type":"interface","device":"@wan","proto":"dhcpv6"}
    }));
    assert!(two_halves.companion("wan").is_none());
    assert!(two_halves.companion_of("wan6").is_none());
    assert!(two_halves.companion_of("wan6b").is_none());
    let two_lines = with_networks(json!({
        "wan":{".name":"wan",".type":"interface","device":"wan0","proto":"dhcp"},
        "wanb":{".name":"wanb",".type":"interface","device":"wan0","proto":"static"},
        "wan6":{".name":"wan6",".type":"interface","device":"wan0","proto":"dhcpv6"}
    }));
    assert!(two_lines.companion_of("wan6").is_none());
    let alone = with_networks(json!({
        "wan6":{".name":"wan6",".type":"interface","device":"wan0","proto":"dhcpv6"}
    }));
    assert!(alone.companion_of("wan6").is_none());
}
