// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
use super::*;
use verso_plugin::Ubus;
fn request() -> Request {
    Request {
        path: "/".into(),
        query: Form::default(),
        snapshot: Snapshot::from_value(
            json!({"system":{"sys":{".name":"sys",".type":"system","hostname":"router","zonename":"UTC","timezone":"UTC"},"ntp":{".name":"ntp",".type":"timeserver","enabled":"1","server":["time.example.org"]}},"network":{"globals":{".name":"globals",".type":"globals","ula_prefix":"fd42:1:1::/48","packet_steering":"1"},"lan":{".name":"lan",".type":"interface"}},"dropbear":{"ssh":{".name":"ssh",".type":"dropbear","Port":"22","Interface":"lan","PasswordAuth":"on"}},"uhttpd":{"main":{".name":"main",".type":"uhttpd","listen_http":["0.0.0.0:80","[::]:80"],"listen_https":["0.0.0.0:443"]}}}),
        ),
        ubus: Ubus::from_value(json!({})),
    }
}
#[test]
fn general_saves_one_complete_validated_form() {
    let e=post(&request(),&Form::parse("hostname=router2&ula_prefix=fd42%3A2%3A%3A%2F48&packet_steering=1&zonename=Europe%2FLjubljana&ntp_enabled=1&server=time.example.org&enable_server=1"));
    assert_eq!(e.commit.len(), 3, "{:?}", e);
    let j = serde_json::to_value(&e).unwrap();
    assert_eq!(j["commit"][0]["values"]["hostname"], "router2");
    assert_eq!(j["commit"][1]["config"], "network");
    assert_eq!(j["commit"][2]["values"]["enable_server"], "1");
    assert!(e.commands.is_empty());
}
#[test]
fn general_refuses_bad_hostname_ula_timezone_and_servers() {
    for body in [
        "hostname=-bad&zonename=UTC",
        "hostname=router&zonename=madeup&original_zonename=madeup&original_timezone=UTC",
        "hostname=router&zonename=UTC&ula_prefix=2001%3Adb8%3A%3A%2F48",
        "hostname=router&zonename=UTC&ntp_enabled=1",
        "hostname=router&zonename=UTC&server=not%20a%20host",
    ] {
        assert!(
            post(&request(), &Form::parse(body)).commit.is_empty(),
            "{body}"
        );
    }
}
#[test]
fn ntp_target_is_read_from_snapshot() {
    let e = post(
        &request(),
        &Form::parse(
            "hostname=router&zonename=UTC&ntp_enabled=1&server=time.example.org&ntp_section=sys",
        ),
    );
    let j = serde_json::to_value(e).unwrap();
    assert_eq!(j["commit"][2]["section"], "ntp");
}
#[test]
fn computer_time_is_an_immediate_command() {
    let e = post(
        &request(),
        &Form::parse(
            "_action=clock&_client_time=2026-09-14T12%3A00%3A00&hostname=unsaved&zonename=UTC",
        ),
    );
    assert!(e.commit.is_empty());
    assert_eq!(e.commands.len(), 1);
    assert_eq!(e.commands[0].args["timezone"], "UTC");
}
#[test]
fn general_never_invents_time_servers() {
    let e = page(Facts::default(), &BTreeMap::new());
    let text = serde_json::to_string(&e).unwrap();
    assert!(!text.contains("pool.ntp.org"));
    assert!(text.contains("packet_steering"));
    assert!(text.contains("ula_prefix"));
    assert!(!text.contains("inline\":true"));
}
#[test]
fn timezone_catalog_has_unique_names_and_valid_rules() {
    let mut seen = std::collections::BTreeSet::new();
    for (name, tz) in ZONES {
        assert!(seen.insert(name));
        assert!(valid_posix_tz(tz), "{tz}");
    }
}
#[test]
fn date_and_prefix_validation() {
    assert!(valid_local_datetime("2024-02-29T23:59:59"));
    for s in [
        "2026-02-29T00:00:00",
        "2026-04-31T12:00:00",
        "2026-01-01T25:00:00",
        "2026-01-01T00:00:00;reboot",
    ] {
        assert!(!valid_local_datetime(s));
    }
    for p in ["fd42:1:1::/48", "fd00::/8", "fc00::/7"] {
        assert!(valid_ula(p), "{p}");
    }
    for p in ["fd42:1:1::1/48", "2001:db8::/48", "::/0"] {
        assert!(!valid_ula(p));
    }
}
#[test]
fn access_refuses_invalid_ports_and_stale_sections() {
    let mut r = request();
    r.path = "/access".into();
    for f in [
        "_access_config=ssh&section=ssh&Port=70000&Interface=lan",
        "_access_config=ssh&section=other&Port=22",
        "_access_config=web&section=main&redirect_https=1&listen_http=0.0.0.0%3A80",
        "_access_config=web&section=main&listen_http=bad",
    ] {
        assert!(post(&r, &Form::parse(f)).commit.is_empty(), "{f}");
    }
}
#[test]
fn access_writes_only_its_selected_service() {
    let mut r = request();
    r.path = "/access".into();
    let e = post(
        &r,
        &Form::parse("_access_config=ssh&section=ssh&Port=2222&Interface=lan&PasswordAuth=1"),
    );
    assert_eq!(e.commit.len(), 1);
    assert_eq!(e.commit[0].config, "dropbear");
    assert_eq!(e.commit[0].values["Port"], "2222");
    assert_eq!(e.commit[0].values["RootPasswordAuth"], "off");
}

#[test]
fn web_ports_preserve_all_bind_addresses() {
    let mut r = request();
    r.path = "/access".into();
    let e = post(&r, &Form::parse("_access_config=web&section=main&listen_http_port=8080&listen_https_port=8443&redirect_https=1"));
    assert_eq!(e.commit.len(), 1, "{e:?}");
    assert_eq!(
        e.commit[0].values["listen_http"],
        json!(["0.0.0.0:8080", "[::]:8080"])
    );
    assert_eq!(e.commit[0].values["listen_https"], json!(["0.0.0.0:8443"]));
    for body in [
        "listen_http_port=443&listen_https_port=443",
        "listen_http_port=70000&listen_https_port=443",
        "listen_http_port=80&redirect_https=1",
    ] {
        assert!(post(
            &r,
            &Form::parse(&format!("_access_config=web&section=main&{body}"))
        )
        .commit
        .is_empty());
    }
}
#[test]
fn unknown_access_page_is_contained() {
    let mut r = request();
    r.path = "/access/unknown".into();
    assert!(serde_json::to_string(&get(&r))
        .unwrap()
        .contains("This page does not exist."));
}
