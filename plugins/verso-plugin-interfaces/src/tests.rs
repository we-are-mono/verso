// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
use super::*;
use verso_plugin::{json, Ubus};
fn request(path: &str, query: &str) -> Request {
    Request {
        path: path.into(),
        query: Form::parse(query),
        snapshot: Snapshot::from_value(json!({
        "network":{
         "lan":{".name":"lan",".type":"interface","device":"br-lan","proto":"static","ipaddr":"192.168.1.1","netmask":"255.255.255.0","auto":"1"},
         "br":{".name":"br",".type":"device","name":"br-lan","type":"bridge","ports":["eth0"]},
         "uplink":{".name":"uplink",".type":"interface","device":"eth1","proto":"dhcp"}},
        "firewall":{"lan":{".name":"lan",".type":"zone","name":"lan","network":["lan"]},"wan":{".name":"wan",".type":"zone","name":"wan","network":["uplink"]}},
        "dhcp":{"lan":{".name":"lan",".type":"dhcp","interface":"lan","start":"100","limit":"150","leasetime":"12h"}},
        "wireless":{"radio":{".name":"radio",".type":"wifi-iface","network":"lan"}}
        })),
        ubus: Ubus::from_value(
            json!({"networkState":{"devices":{"eth0":{"up":true},"eth1":{"up":true},"eth2":{"up":false},"br-lan":{"up":true,"bridge":true}},"interfaces":[{"interface":"lan","l3_device":"br-lan","up":true,"ipv4-address":[{"address":"192.168.1.1","mask":24}]}]}}),
        ),
    }
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
    assert!(e.commit.iter().all(|o| o.config != "dhcp"), "{:?}", e.commit);
}
// A network that hands out addresses keeps a static one to hand out from.
#[test]
fn a_serving_network_keeps_its_static_address() {
    let e = post(
        &request("/edit", "network=lan"),
        &Form::parse("name=lan&device=br-lan&proto=dhcp&zone=lan&auto=1"),
    );
    assert!(e.commit.is_empty());
    assert!(serde_json::to_string(&e).unwrap().contains("Turn its DHCP server off"));
}
#[test]
fn invalid_posts_never_stage() {
    for body in ["name=lan&device=eth2&proto=dhcp","name=guest&device=missing&proto=dhcp","name=../bad&proto=dhcp","name=guest&device=eth2&proto=static&ipaddr=invalid&netmask=255.255.255.0","name=guest&device=eth2&proto=static&ipaddr=192.168.3.1&netmask=255.0.255.0","name=wan2&device=eth2&proto=pppoe&username=account"]{let e=post(&request("/new","kind=network"),&Form::parse(body));assert!(e.commit.is_empty(),"accepted {body}");assert!(serde_json::to_string(&e).unwrap().contains("\"error\":\""),"no refusal for {body}");}
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
        .any(|o| o["config"] == "dhcp"
            && o["values"] == json!({"interface": "home"})));
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
    assert_eq!(values, ["network", "wan", "bridge", "vlan", "tunnel"]);
    // A VPN is made on the tunnels' own page, and the drawer says where.
    assert!(drawer.to_string().contains(&format!("{ROOT}vpn")), "{drawer}");
    // Editing an interface is not choosing one: an existing object's kind is
    // what it is.
    let lan = open_drawer(&get(&request("/edit", "network=lan")));
    assert_ne!(the_type(&lan)["name"], "kind", "{lan}");
    // The drawer's title already names what it is; its opening fields stand
    // under it without a heading or a lede of their own.
    for d in [&drawer, &lan, &open_drawer(&get(&request("/edit", "network=&device=br-lan")))] {
        let opening = &d["children"][0]["fields"][0];
        let said = |k: &str| opening.get(k).and_then(|v| v.as_str()).unwrap_or_default().to_string();
        assert!(said("title").is_empty() && said("sub").is_empty(), "{opening}");
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
        &Form::parse("_action=reshape&kind=bridge&name=guest&device=eth2&mtu=1400&proto=static&ipaddr=10.0."),
    );
    assert!(e.commit.is_empty(), "a reshape stages nothing: {e:?}");
    let drawer = open_drawer(&e);
    assert_eq!(drawer["title"], "New bridge");
    assert_eq!(the_type(&drawer)["value"], "bridge");
    let j = drawer.to_string();
    assert!(!j.contains("\"error\":\""), "a reshape refuses nothing: {j}");
    assert!(j.contains("\"value\":\"guest\""), "the name carries over: {j}");
    assert!(j.contains("\"value\":\"1400\""), "the MTU carries over: {j}");
    assert!(!j.contains("\"value\":\"10.0.\""), "a network's address is no bridge's: {j}");
    // A line to the internet starts as one, whatever the network was set to.
    let wan = open_drawer(&post(
        &request("/new", ""),
        &Form::parse("_action=reshape&kind=wan&name=wan2&device=eth2&proto=static&zone=lan"),
    ));
    assert_eq!(wan["title"], "New internet connection");
    assert_eq!(value_of(&wan, "proto"), "dhcp", "{wan}");
    assert_eq!(value_of(&wan, "zone"), "wan", "{wan}");
    assert_eq!(value_of(&wan, "device"), "eth2", "the device it runs on carries over: {wan}");
}
// value_of is the value of the first control named name anywhere in v.
fn value_of(v: &serde_json::Value, name: &str) -> String {
    match v {
        serde_json::Value::Object(o) if o.get("name").and_then(|n| n.as_str()) == Some(name) => {
            o.get("value").and_then(|x| x.as_str()).unwrap_or_default().into()
        }
        serde_json::Value::Object(o) => o.values().map(|x| value_of(x, name)).find(|s| !s.is_empty()).unwrap_or_default(),
        serde_json::Value::Array(a) => a.iter().map(|x| value_of(x, name)).find(|s| !s.is_empty()).unwrap_or_default(),
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
    assert!(post(&request("/new", ""), &Form::parse("kind=vpn&name=x")).commit.is_empty());
}
#[test]
fn listing_uses_live_addresses_and_expandable_details() {
    let j = serde_json::to_string(&get(&request("/", ""))).unwrap();
    assert!(j.contains("192.168.1.1/24"));
    assert!(j.contains("expanded"));
    assert!(j.contains("Restart"));
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

fn inventory_row(m: &Model, id: &str) -> serde_json::Value {
    fn find(v: &serde_json::Value, id: &str) -> Option<serde_json::Value> {
        if v.get("id").and_then(serde_json::Value::as_str) == Some(id) && v.get("cells").is_some() {
            return Some(v.clone());
        }
        match v {
            serde_json::Value::Object(o) => o.values().find_map(|v| find(v, id)),
            serde_json::Value::Array(a) => a.iter().find_map(|v| find(v, id)),
            _ => None,
        }
    }
    find(&serde_json::to_value(page::listing(m)).unwrap(), id).unwrap()
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
    let row = inventory_row(&m, "br-lan");
    assert_eq!(row["cells"][4]["text"], "up");
    assert_eq!(row["cells"][5]["actions"][1]["name"], "down");
    m.live["interfaces"][0]["up"] = json!(false);
    assert_eq!(inventory_row(&m, "br-lan")["cells"][4]["text"], "down");
    m.live["interfaces"][0]["pending"] = json!(true);
    assert_eq!(inventory_row(&m, "br-lan")["cells"][4]["text"], "pending");
    m.live["interfaces"][0]["pending"] = json!(false);
    m.live["devices"]["br-lan"]["carrier"] = json!(false);
    assert_eq!(inventory_row(&m, "br-lan")["cells"][4]["text"], "no link");
}
#[test]
fn addresses_are_observed_and_ipv6_assignments_stay_in_details() {
    let mut m = Model::read(&request("/", ""));
    m.live["interfaces"][0]["ipv6-prefix-assignment"] =
        json!([{"local-address":{"address":"fd42:7ea:aa00::1","mask":60}}]);
    m.live["devices"]["br-lan"]["type"] = json!("bridge");
    let row = inventory_row(&m, "br-lan");
    assert_eq!(row["cells"][1]["text"], "bridge");
    assert_eq!(row["cells"][2]["text"], "192.168.1.1/24");
    assert!(row["expanded"].to_string().contains("fd42:7ea:aa00::1/60"));
    m.live["interfaces"][0]["ipv4-address"] = json!([]);
    assert_eq!(inventory_row(&m, "br-lan")["cells"][2]["text"], "—");
    m.live = serde_json::Value::Null;
    assert_eq!(
        inventory_row(&m, "br-lan")["cells"][4]["text"],
        "not reported"
    );
}
#[test]
fn physical_bridge_ports_are_roots_and_vlan_and_ppp_follow_their_transport() {
    let mut m = Model::read(&request("/", ""));
    m.live["devices"]["eth0.20"] = json!({"kind":"vlan","parent":"eth0","up":true});
    m.live["interfaces"]
        .as_array_mut()
        .unwrap()
        .push(json!({"interface":"uplink","l3_device":"pppoe-wan","up":true}));
    let parents = m.parents();
    assert!(!parents.contains_key("eth0"));
    assert_eq!(parents["eth0.20"], "eth0");
    assert_eq!(parents["pppoe-wan"], "eth1");
    assert_eq!(inventory_row(&m, "eth0.20")["depth"], 1);
    assert_eq!(
        inventory_row(&m, "pppoe-wan")["cells"][0]["chips"][0]["label"],
        "uplink"
    );
}

#[test]
fn home_router_tree_matches_the_reference_relationships() {
    let mut r = request("/", "");
    r.snapshot = Snapshot::from_value(json!({"network":{
        "lan_bridge":{".name":"lan_bridge",".type":"device","name":"br-lan","type":"bridge","ports":["eth0","wlan0"]},
        "iptv_bridge":{".name":"iptv_bridge",".type":"device","name":"br-iptv","type":"bridge","ports":["eth4.3999","br-lan.3999"]},
        "wan":{".name":"wan",".type":"interface","device":"eth4.3900","proto":"pppoe"}
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
    assert_eq!(inventory_row(&m, "pppoe-wan")["depth"], 2);
    assert_eq!(inventory_row(&m, "eth4.3999")["depth"], 1);
    for name in ["eth0", "wlan0", "br-lan", "br-iptv"] {
        assert!(
            inventory_row(&m, name).get("depth").is_none(),
            "{name} must be a root"
        );
    }
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
    assert_eq!(inventory_row(&m, "lan0")["depth"], 1);
    // An observed empty bridge overrides ports still in the UCI snapshot.
    m.live["devices"]["br-lan"]["bridge-members"] = json!([]);
    assert!(m.parents().is_empty());
    // A member reported only by netifd still belongs in the inventory.
    m.live["devices"]["br-lan"]["bridge-members"] = json!(["tap-only"]);
    assert!(m.names().contains("tap-only"));
}

// The interface's drawer states its DHCP server as it stands and links to its
// drawer on the DHCP page; it carries none of the server's own controls.
#[test]
fn the_interface_drawer_states_its_dhcp_server_and_leads_to_it() {
    let e = get(&request("/edit", "network=lan"));
    let body = serde_json::to_string(&e).unwrap();
    for gone in [
        "\"name\":\"start\"",
        "\"name\":\"leasetime\"",
        "\"name\":\"ra\"",
        "\"name\":\"dhcpv6\"",
    ] {
        assert!(!body.contains(gone), "{gone}");
    }
    let j = serde_json::to_value(&e).unwrap();
    fn find(v: &serde_json::Value, title: &str) -> Option<serde_json::Value> {
        if v["title"] == title && v["type"] == "section" {
            return Some(v.clone());
        }
        match v {
            serde_json::Value::Object(m) => m.values().find_map(|c| find(c, title)),
            serde_json::Value::Array(a) => a.iter().find_map(|c| find(c, title)),
            _ => None,
        }
    }
    let server = find(&j, "DHCP server").expect("the DHCP server part");
    assert_eq!(server["control"]["href"], "/plugins/dnsdhcp/?open=lan");
    assert!(server["control"].get("panel").is_none(), "{server}");
}
// groups are the parts an expanded row is made of: one per uci section it
// reads, each a section on its own ledger line.
fn groups(row: &serde_json::Value) -> Vec<serde_json::Value> {
    let outer = &row["expanded"][0];
    assert_eq!(outer["type"], "section", "one frame holds the parts: {row}");
    outer["children"].as_array().unwrap().clone()
}
// labels are a part's facts in reading order: its left column, then its
// right.
fn labels(group: &serde_json::Value) -> Vec<String> {
    group["children"][0]["children"]
        .as_array()
        .unwrap()
        .iter()
        .flat_map(|column| column["items"].as_array().unwrap().iter())
        .map(|p| p["label"].as_str().unwrap().to_string())
        .collect()
}
#[test]
fn an_expanded_row_reads_as_the_uci_sections_behind_it() {
    // A bridge carrying one network with a DHCP server: its network, its
    // server and its device, each named as the config names it, in that order.
    let m = Model::read(&request("/", ""));
    let parts = groups(&inventory_row(&m, "br-lan"));
    // Each on its ledger line with the glyph of its kind, so parts of
    // different kinds are told apart at a glance.
    let heads: Vec<_> = parts
        .iter()
        .map(|g| {
            (
                g["icon"].as_str().unwrap(),
                g["title"].as_str().unwrap(),
                g["meta"].as_str().unwrap(),
            )
        })
        .collect();
    assert_eq!(
        heads,
        [
            ("network", "Network", "lan"),
            ("server", "DHCP server", "lan"),
            ("git-merge", "Device", "br-lan")
        ]
    );
    // A line to the internet wears the globe.
    let uplink = groups(&inventory_row(&m, "eth1"));
    assert_eq!(uplink[0]["icon"], "globe", "{uplink:?}");
    // Each part reads as its facts alone, in two columns split evenly in
    // reading order: the config text is the drawer's.
    for part in &parts {
        let grid = &part["children"][0];
        assert_eq!(grid["style"], "facts", "{part}");
        let columns = grid["children"].as_array().unwrap();
        assert_eq!(columns.len(), 2, "{part}");
        assert!(columns.iter().all(|c| c["type"] == "properties"), "{part}");
        let (left, right) = (
            columns[0]["items"].as_array().unwrap().len(),
            columns[1]["items"].as_array().unwrap().len(),
        );
        assert!(left == right || left == right + 1, "{part}");
    }
    // A network's addresses stand on the left, how it stands on the right.
    assert_eq!(
        parts[0]["children"][0]["children"][1]["items"][0]["label"],
        "Protocol"
    );
    // Addresses first, then how the network is brought up and where it sits.
    assert_eq!(
        labels(&parts[0]),
        ["IPv4", "IPv6", "ULA", "Protocol", "Uptime", "Firewall zone"]
    );
    // The device behind the network reads as itself: a bridge's ports first.
    assert_eq!(labels(&parts[2])[0], "Ports");
    // The acts stand on the line of what they edit; the network's own edit
    // is the row's pencil, so its line carries none.
    assert!(parts[0].get("control").is_none(), "{}", parts[0]);
    for (part, label, href) in [
        (
            &parts[1],
            "Configure DHCP",
            "/plugins/dnsdhcp/?open=lan",
        ),
        (
            &parts[2],
            "Edit device",
            "/plugins/interfaces/edit?network=&device=br-lan",
        ),
    ] {
        assert_eq!(part["control"]["label"], label);
        assert_eq!(part["control"]["style"], "act");
        assert_eq!(part["control"]["href"], href);
    }
    // Nothing stands loose between the parts.
    assert!(parts.iter().all(|g| g["type"] == "section"));
}
#[test]
fn a_dhcp_server_reads_as_its_state_and_how_full_its_pool_is() {
    let mut r = request("/", "");
    r.ubus = Ubus::from_value(json!({
        "networkState":{"devices":{"eth0":{"up":true},"br-lan":{"up":true,"bridge":true}},"interfaces":[{"interface":"lan","l3_device":"br-lan","up":true,"ipv4-address":[{"address":"192.168.1.1","mask":24}]}]},
        "dhcpState":{"networks":{"lan":{"state":"running","enabled":true,"pool":"192.168.1.100–192.168.1.249","lease_time":"12h","leases":30}}}
    }));
    let m = Model::read(&r);
    let parts = groups(&inventory_row(&m, "br-lan"));
    let items = |column: usize| parts[1]["children"][0]["children"][column]["items"].clone();
    // The server's state is said once, with its mark.
    assert_eq!(items(0)[0]["label"], "Server");
    assert_eq!(items(0)[0]["value"], "Serving");
    assert_eq!(items(0)[0]["dot"], "success");
    // The pool is the stretch from its first address to its last, filled by
    // the share of its 150 addresses that are leased.
    let pool = items(0)[1].clone();
    assert_eq!(pool["label"], "Pool");
    assert_eq!(pool["span"]["from"], "192.168.1.100");
    assert_eq!(pool["span"]["to"], "192.168.1.249");
    assert_eq!(pool["span"]["at"], 20);
    assert_eq!(pool["span"]["tone"], "success");
    // Without a lease count there is no fill to draw: the range as words.
    r.ubus = Ubus::from_value(json!({
        "dhcpState":{"networks":{"lan":{"state":"running","enabled":true,"pool":"192.168.1.100–192.168.1.249"}}}
    }));
    let parts = groups(&inventory_row(&Model::read(&r), "br-lan"));
    let pool = parts[1]["children"][0]["children"][0]["items"][1].clone();
    assert_eq!(pool["value"], "192.168.1.100–192.168.1.249");
    assert!(pool.get("span").is_none(), "{pool}");
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
fn every_way_into_the_editor_opens_it_in_place() {
    let m = Model::read(&request("/", ""));
    // A row's pencil is its panel, so the shell opens it over the listing.
    for (id, href) in [
        ("br-lan", "/plugins/interfaces/edit?network=lan&device="),
        ("eth1", "/plugins/interfaces/edit?network=uplink&device="),
    ] {
        let row = inventory_row(&m, id);
        assert_eq!(row["panel"], href, "{row}");
        let pencil = row["cells"][5]["actions"][3].clone();
        assert_eq!(pencil["href"], href, "{row}");
    }
    // The acts on an expanded row's parts open the same drawer.
    let parts = groups(&inventory_row(&m, "br-lan"));
    for part in &parts[1..] {
        assert_eq!(part["control"]["style"], "act");
        assert_eq!(part["control"]["panel"], true, "{part}");
    }
}
#[test]
fn a_disabled_dhcp_server_says_so_once() {
    let mut r = request("/", "");
    r.snapshot = Snapshot::from_value(json!({
        "network":{"uplink":{".name":"uplink",".type":"interface","device":"eth1","proto":"dhcp"}},
        "dhcp":{"uplink":{".name":"uplink",".type":"dhcp","interface":"uplink","ignore":"1"}}
    }));
    let m = Model::read(&r);
    let parts = groups(&inventory_row(&m, "eth1"));
    let server = parts.iter().find(|g| g["title"] == "DHCP server").unwrap();
    assert_eq!(labels(server), ["Server"]);
    assert_eq!(
        server["children"][0]["children"][0]["items"][0]["value"],
        "Off"
    );
    // Disabled is still where it is turned on.
    assert_eq!(server["control"]["label"], "Configure DHCP");
}
#[test]
fn each_network_a_device_carries_is_its_own_part_with_its_edit() {
    let mut r = request("/", "");
    r.snapshot = Snapshot::from_value(json!({
        "network":{
         "uplink":{".name":"uplink",".type":"interface","device":"eth1","proto":"dhcp"},
         "uplink6":{".name":"uplink6",".type":"interface","device":"eth1","proto":"dhcpv6"}}
    }));
    let m = Model::read(&r);
    let parts = groups(&inventory_row(&m, "eth1"));
    let networks: Vec<_> = parts.iter().filter(|g| g["title"] == "Network").collect();
    assert_eq!(networks.len(), 2, "{parts:?}");
    for (part, name) in networks.iter().zip(["uplink", "uplink6"]) {
        assert_eq!(part["meta"], name);
        assert_eq!(labels(part)[0], "State");
        assert_eq!(part["control"]["label"], "Edit network");
        assert_eq!(
            part["control"]["href"],
            format!("/plugins/interfaces/edit?network={name}&device=")
        );
    }
}
#[test]
fn a_device_with_no_network_reads_as_its_device() {
    let m = Model::read(&request("/", ""));
    let parts = groups(&inventory_row(&m, "eth0"));
    assert_eq!(parts.len(), 1, "{parts:?}");
    assert_eq!(parts[0]["title"], "Device");
    assert_eq!(parts[0]["meta"], "eth0");
    // the row's pencil edits it
    assert!(parts[0].get("control").is_none());
}
// A device OpenVPN makes when it connects is the VPN's, not the network
// config's: its drawer says whose it is and leads there, rather than offering
// a name, an MTU and a MAC the tunnel would overwrite.
#[test]
fn a_device_run_by_openvpn_hands_off_to_its_vpn_panel() {
    let mut r = request("/edit", "device=tun0");
    r.ubus = Ubus::from_value(json!({
        "vpnState": {"tunnels": [{"device": "tun0", "kind": "openvpn", "instance": "proton", "up": true}]}
    }));
    let drawer = open_drawer(&get(&r));
    assert_eq!(drawer["title"], "tun0");
    let text = drawer.to_string();
    assert!(text.contains("/plugins/vpn/?open=proton"), "{text}");
    assert!(text.contains("proton"), "{text}");
    assert!(!text.contains("\"mtu\""), "{text}");
    assert!(!text.contains("macaddr"), "{text}");
}
