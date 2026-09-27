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
// The page as drawn, posted back: what the router already has.
const GENERAL_AS_IS: &str = "hostname=router&ula_prefix=fd42%3A1%3A1%3A%3A%2F48&packet_steering=1&zonename=UTC&ntp_enabled=1&server=time.example.org";
#[test]
fn general_stages_only_what_differs() {
    // Nothing changed: nothing to stage.
    let e = post(&request(), &Form::parse(GENERAL_AS_IS));
    assert!(e.commit.is_empty(), "{:?}", e.commit);
    // One switch flipped: that option, and only it, in its own section.
    let e = post(
        &request(),
        &Form::parse(&GENERAL_AS_IS.replace("&packet_steering=1", "")),
    );
    let j = serde_json::to_value(&e).unwrap();
    assert_eq!(e.commit.len(), 1, "{:?}", e.commit);
    assert_eq!(j["commit"][0]["config"], "network");
    assert_eq!(j["commit"][0]["values"], json!({"packet_steering": "0"}));
    // A time server added: the list, and nothing else about time.
    let e = post(
        &request(),
        &Form::parse(&format!("{GENERAL_AS_IS}&server=pool.example.org")),
    );
    let j = serde_json::to_value(&e).unwrap();
    assert_eq!(e.commit.len(), 1, "{:?}", e.commit);
    assert_eq!(
        j["commit"][0]["values"],
        json!({"server": ["time.example.org", "pool.example.org"]})
    );
}
#[test]
fn access_stages_only_what_differs() {
    let staged = |body: &str| {
        let e = post(&access_request(), &Form::parse(body));
        serde_json::to_value(&e).unwrap()["commit"].clone()
    };
    // Either form posted back as drawn: nothing to stage, the defaults of
    // options the router leaves unset included.
    let ssh =
        "_access_config=ssh&section=ssh&Port=22&Interface=lan&PasswordAuth=1&RootPasswordAuth=1";
    let web = "_access_config=web&section=main&listen_http_port=80&listen_https_port=443";
    assert!(
        staged(ssh).as_array().is_none_or(Vec::is_empty),
        "{}",
        staged(ssh)
    );
    assert!(
        staged(web).as_array().is_none_or(Vec::is_empty),
        "{}",
        staged(web)
    );
    // One switch flipped: that option alone.
    assert_eq!(
        staged(&ssh.replace("&PasswordAuth=1", ""))[0]["values"],
        json!({"PasswordAuth": "off"})
    );
    assert_eq!(
        staged(&format!("{web}&redirect_https=1"))[0]["values"],
        json!({"redirect_https": "1"})
    );
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
        &Form::parse(&format!("{GENERAL_AS_IS}&enable_server=1&ntp_section=sys")),
    );
    let j = serde_json::to_value(e).unwrap();
    let ntp = j["commit"]
        .as_array()
        .unwrap()
        .iter()
        .find(|op| op["values"].get("enable_server").is_some())
        .expect("the timeserver change is staged");
    assert_eq!(ntp["section"], "ntp");
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
fn general_names_what_it_saves() {
    let page = serde_json::to_value(get(&request())).unwrap();
    assert_eq!(page["widget"]["submit"], "Save settings", "{page}");
}
#[test]
fn general_says_where_each_option_lives() {
    // The shell marks a control whose option waits on the stage by its full
    // address, so every control names the config and section it writes.
    let page = serde_json::to_value(get(&request())).unwrap();
    let text = page.to_string();
    let find = |name: &str| -> serde_json::Value {
        fn walk(v: &serde_json::Value, name: &str) -> Option<serde_json::Value> {
            if v.get("name").and_then(|n| n.as_str()) == Some(name) && v.get("key").is_some() {
                return Some(v.clone());
            }
            match v {
                serde_json::Value::Array(a) => a.iter().find_map(|x| walk(x, name)),
                serde_json::Value::Object(o) => o.values().find_map(|x| walk(x, name)),
                _ => None,
            }
        }
        walk(&page, name).unwrap_or_else(|| panic!("{name} not on the page: {text}"))
    };
    assert_eq!(find("hostname")["target"], "system.sys");
    assert_eq!(find("zonename")["target"], "system.sys");
    assert_eq!(find("ula_prefix")["target"], "network.globals");
    assert_eq!(find("packet_steering")["target"], "network.globals");
    // the time section's own options live in the timeserver section
    assert!(text.contains("\"target\":\"system.ntp\""), "{text}");
}
#[test]
fn access_forms_say_where_their_options_live() {
    let page = access_page(json!([]));
    assert!(page.contains("\"target\":\"dropbear.ssh\""), "{page}");
    assert!(page.contains("\"target\":\"uhttpd.main\""), "{page}");
}
#[test]
fn computer_time_is_an_act_on_the_clock_dressed_as_the_certificates() {
    let text = serde_json::to_string(&page(Facts::default(), &BTreeMap::new())).unwrap();
    assert!(
        text.contains(
            "\"type\":\"button\",\"label\":\"Use my computer's time\",\"icon\":\"clock\",\"style\":\"act\""
        ),
        "{text}"
    );
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

// A refusal is said once, where it belongs: a port's under its field, a
// switch's under the switch, and only one no control owns above the form.
#[test]
fn a_refusal_is_said_once() {
    let mut r = request();
    r.path = "/access".into();
    let times = |body: &str, said: &str| {
        let e = post(&r, &Form::parse(body));
        serde_json::to_string(&e).unwrap().matches(said).count()
    };
    let web = "_access_config=web&section=main&listen_http_port=80";
    let port = "Enter a port from 1 to 65535.";
    assert_eq!(times(&format!("{web}&listen_https_port=99999"), port), 1);
    assert_eq!(times("_access_config=ssh&section=ssh&Port=99999", port), 1);
    let redirect = "Add an HTTPS listener before enabling redirection.";
    let unsecured = format!("{web}&redirect_https=1");
    assert_eq!(times(&unsecured, redirect), 1);
    let j = serde_json::to_value(post(&r, &Form::parse(&unsecured)))
        .unwrap()
        .to_string();
    let on_switch = format!(r#""error":"{redirect}","key":"redirect_https""#);
    assert!(j.contains(&on_switch), "the switch carries it: {j}");
    let hostname = "Use 1–63 letters, numbers or hyphens, without a leading or trailing hyphen.";
    let bad = GENERAL_AS_IS.replace("hostname=router", "hostname=-bad");
    let e = post(&request(), &Form::parse(&bad));
    assert_eq!(
        serde_json::to_string(&e).unwrap().matches(hostname).count(),
        1
    );
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
fn access_with(keys: serde_json::Value, certificate: serde_json::Value) -> String {
    let mut r = request();
    r.path = "/access".into();
    r.ubus = Ubus::from_value(json!({"accessCredentials":{"keys":keys,"certificate":certificate}}));
    serde_json::to_string(&get(&r)).unwrap()
}
fn self_signed() -> serde_json::Value {
    json!({"file":"/etc/uhttpd.crt","subject":"OpenWrt","issuer":"OpenWrt","from":"2026-08-31",
        "until":"2027-10-02","fingerprint":"9D 4C 7A","self_signed":"1",
        "days_total":"397","days_left":"376"})
}
fn access_request() -> Request {
    let mut r = request();
    r.path = "/access".into();
    r
}
fn access_page(keys: serde_json::Value) -> String {
    access_with(keys, self_signed())
}
fn property<'a>(page: &'a serde_json::Value, label: &str) -> &'a serde_json::Value {
    fn find<'a>(v: &'a serde_json::Value, label: &str) -> Option<&'a serde_json::Value> {
        match v {
            serde_json::Value::Object(m)
                if m.get("label").and_then(|l| l.as_str()) == Some(label)
                    && m.contains_key("value") =>
            {
                Some(v)
            }
            serde_json::Value::Object(m) => m.values().find_map(|c| find(c, label)),
            serde_json::Value::Array(a) => a.iter().find_map(|c| find(c, label)),
            _ => None,
        }
    }
    find(page, label).unwrap_or_else(|| panic!("no {label} row in {page}"))
}
#[test]
fn certificate_reads_as_a_document_the_browser_can_be_checked_against() {
    let page: serde_json::Value = serde_json::from_str(&access_page(json!([]))).unwrap();
    let signed = property(&page, "Signed by");
    assert_eq!(signed["value"], "The router itself");
    assert_eq!(signed["dot"], "warning");
    assert!(signed["help"]
        .as_str()
        .unwrap()
        .starts_with("Browsers warn"));
    let valid = property(&page, "Valid");
    assert_eq!(
        valid["span"],
        json!({"from":"2026-08-31","to":"2027-10-02","at":5,"tone":"success"})
    );
    let left = property(&page, "Time left");
    assert_eq!(
        (left["value"].as_str(), left["dot"].as_str()),
        (Some("376 d"), Some("success"))
    );
    let print = property(&page, "Fingerprint");
    assert_eq!(print["value"], "9D 4C 7A");
    assert!(print["help"].as_str().unwrap().contains("your browser"));
    let text = page.to_string();
    for said in [
        "\"title\":\"HTTPS certificate\"",
        "\"subtitle\":\"What this router shows browsers that connect over HTTPS.\"",
    ] {
        assert!(text.contains(said), "the card says what it holds: {text}");
    }
    assert!(text.contains("\"align\":\"left\""), "{text}");
    assert!(
        !text.contains("\"markdown\":\"Self-signed"),
        "the warning belongs to its fact"
    );
    assert!(!text.contains("Valid until"));
}
#[test]
fn sections_commit_their_settings_before_what_they_hold() {
    let page: serde_json::Value = serde_json::from_str(&access_page(json!([]))).unwrap();
    let sections = page["widget"]["children"].as_array().unwrap();
    let (ssh, web) = (&sections[0]["children"], &sections[1]["children"]);
    assert_eq!(ssh[0]["type"], "form", "{ssh}");
    // Each form's button names what it saves, never a bare "Save".
    assert_eq!(ssh[0]["submit"], "Save SSH settings");
    assert_eq!(web[0]["submit"], "Save web interface settings");
    assert!(
        !ssh[0].to_string().contains("Authorized keys"),
        "keys are not a setting"
    );
    assert_eq!(
        (ssh[1]["type"].as_str(), ssh[1]["title"].as_str()),
        (Some("section"), Some("Authorized keys"))
    );
    assert_eq!(web[0]["type"], "form", "{web}");
    assert_eq!(
        (web[1]["type"].as_str(), web[1]["title"].as_str()),
        (Some("section"), Some("Certificates"))
    );
    let held = &web[1]["children"][0];
    assert_eq!(
        held["type"], "stack",
        "the certificate and its acts are one group: {web}"
    );
    assert_eq!(held["children"][0]["style"], "artifact");
    assert_eq!(held["children"][1]["inline"], true);
    let keys = &ssh[1]["children"];
    assert_eq!(
        keys.as_array().unwrap().len(),
        1,
        "the keys and their act are one group: {keys}"
    );
}
#[test]
fn certificate_life_turns_as_it_runs_out() {
    for (left, tone, value) in [
        ("30", "warning", "30 d"),
        ("7", "danger", "7 d"),
        ("0", "danger", "Expired"),
    ] {
        let mut cert = self_signed();
        cert["days_left"] = json!(left);
        if left == "0" {
            cert["expired"] = json!("1");
        }
        let page: serde_json::Value = serde_json::from_str(&access_with(json!([]), cert)).unwrap();
        let row = property(&page, "Time left");
        assert_eq!(
            (row["value"].as_str(), row["dot"].as_str()),
            (Some(value), Some(tone)),
            "{left}"
        );
        assert_eq!(property(&page, "Valid")["span"]["tone"], tone);
    }
}
#[test]
fn a_certificate_someone_else_signed_names_its_signer() {
    let mut cert = self_signed();
    cert["issuer"] = json!("R11");
    cert.as_object_mut().unwrap().remove("self_signed");
    let page: serde_json::Value = serde_json::from_str(&access_with(json!([]), cert)).unwrap();
    let signed = property(&page, "Signed by");
    assert_eq!(
        (signed["value"].as_str(), signed["mono"].as_bool()),
        (Some("R11"), Some(true))
    );
    assert_eq!(signed["dot"], "success");
    assert!(signed.get("help").is_none());
}
#[test]
fn access_names_the_rebinding_guard_for_what_it_does() {
    let page = access_page(json!([]));
    assert!(page.contains("Block DNS rebinding"), "{page}");
    assert!(!page.contains("Refuse requests from the internet"));
}
#[test]
fn access_spells_authorized_keys_one_way() {
    let page = access_page(json!([]));
    assert!(page.contains("Authorized keys"), "{page}");
    assert!(!page.contains("Authorised"));
}
#[test]
fn access_says_when_no_key_is_authorized() {
    let none = keys_listing(json!([]));
    assert_eq!(none["empty"], "No keys are authorized.");
    assert!(none["items"].as_array().unwrap().is_empty());
}
fn keys_listing(keys: serde_json::Value) -> serde_json::Value {
    let page: serde_json::Value = serde_json::from_str(&access_page(keys)).unwrap();
    keys_of(&page)
}
fn keys_of(page: &serde_json::Value) -> serde_json::Value {
    page["widget"]["children"][0]["children"][1]["children"][0].clone()
}
const PASTED: &str =
    "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIG8/UqksrqlP7SLQWgj8xqnjV3e6cdgzDyzcNwOcT5+K demo@laptop";
fn post_access(body: &str) -> Envelope {
    let mut r = request();
    r.path = "/access".into();
    r.ubus = Ubus::from_value(
        json!({"accessCredentials":{"keys":[{"comment":"me@laptop","fingerprint":"SHA256:a+b/c"}],"certificate":self_signed()}}),
    );
    post(&r, &Form::parse(body))
}
#[test]
fn keys_are_a_set_kept_in_place() {
    let set = keys_listing(json!([
        {"comment":"me@laptop","fingerprint":"SHA256:a+b/c"},
        {"comment":"","fingerprint":"SHA256:e"}
    ]));
    assert_eq!(set["type"], "collection", "{set}");
    let item = &set["items"][0];
    assert_eq!(
        (item["title"].as_str(), item["detail"].as_str()),
        (Some("me@laptop"), Some("SHA256:a+b/c"))
    );
    // a key nobody named is known by its fingerprint alone
    assert_eq!(set["items"][1]["title"], "SHA256:e");
    assert!(set["items"][1].get("detail").is_none());
    let remove = &item["remove"];
    assert_eq!(
        (remove["name"].as_str(), remove["value"].as_str()),
        (Some("_key_remove"), Some("SHA256:a+b/c"))
    );
    assert_eq!(remove["confirm"]["icon"], "trash-2");
    let add = &set["add"];
    assert_eq!(
        (add["name"].as_str(), add["submit"].as_str()),
        (Some("authorized_key"), Some("Add key"))
    );
    assert_eq!(add["preview"]["live"], true);
    assert_eq!(add["preview"]["value"], "", "nothing typed, nothing read");
}
#[test]
fn a_key_is_removed_by_its_fingerprint() {
    let e = post_access("_key_remove=SHA256%3Aa%2Bb%2Fc");
    assert!(e.commit.is_empty());
    assert_eq!(e.commands.len(), 1);
    assert_eq!(e.commands[0].name, "ssh-key-remove");
    assert_eq!(e.commands[0].args["fingerprint"], "SHA256:a+b/c");
}
#[test]
fn a_pasted_key_is_read_as_it_is_typed_and_added_as_pasted() {
    let body = format!(
        "authorized_key={}",
        PASTED
            .replace('+', "%2B")
            .replace('/', "%2F")
            .replace(' ', "+")
    );
    let e = post_access(&body);
    assert_eq!(e.commands.len(), 1, "{e:?}");
    assert_eq!(e.commands[0].name, "ssh-key-add");
    assert_eq!(e.commands[0].args["key"], PASTED);
    let page = serde_json::to_value(&e).unwrap();
    assert_eq!(
        keys_of(&page)["add"]["preview"]["value"],
        "256 SHA256:UPedI7axeQlxL8dlMkeSROduLmrflVzNxJkm5UNiHxw demo@laptop (ED25519)",
        "the reading a pasted key gets while it is typed"
    );
}
#[test]
fn a_refused_paste_comes_back_kept_with_its_reason() {
    for body in ["authorized_key=", "authorized_key=ssh-ed25519+nope"] {
        let e = post_access(body);
        assert!(e.commands.is_empty(), "{body}");
        let add = &keys_of(&serde_json::to_value(&e).unwrap())["add"];
        assert_eq!(add["error"], "Paste one complete SSH public key.", "{body}");
    }
    let add =
        &keys_of(&serde_json::to_value(post_access("authorized_key=ssh-ed25519+nope")).unwrap())
            ["add"];
    assert_eq!(add["value"], "ssh-ed25519 nope");
}
#[test]
fn the_key_pages_are_gone_now_keys_are_kept_in_place() {
    let mut r = request();
    for path in ["/access/key/new", "/access/key/remove"] {
        r.path = path.into();
        assert!(serde_json::to_string(&get(&r))
            .unwrap()
            .contains("This page does not exist."));
    }
}
#[test]
fn access_forms_wait_for_a_change_before_saving() {
    let page = access_page(json!([]));
    assert_eq!(page.matches("\"style\":\"settings\"").count(), 2, "{page}");
    assert!(!page.contains("\"style\":\"page\""));
}
#[test]
fn the_certificate_is_a_part_of_the_web_section_named_certificates() {
    // Like the keys under SSH, the certificate is a part of its section with
    // a subheading of its own over the card and its acts.
    let page: serde_json::Value = serde_json::from_str(&access_page(json!([]))).unwrap();
    fn find(v: &serde_json::Value) -> Option<&serde_json::Value> {
        if v.get("type").and_then(|t| t.as_str()) == Some("section")
            && v.get("title").and_then(|t| t.as_str()) == Some("Certificates")
        {
            return Some(v);
        }
        match v {
            serde_json::Value::Array(a) => a.iter().find_map(find),
            serde_json::Value::Object(o) => o.values().find_map(find),
            _ => None,
        }
    }
    let part = find(&page).expect("a Certificates part on Access");
    let text = part.to_string();
    assert!(
        text.contains("HTTPS certificate") && text.contains("Install a certificate"),
        "{text}"
    );
}
#[test]
fn access_certificate_acts_lead_somewhere_and_spend_no_denim() {
    let page = access_page(json!([]));
    assert!(page.contains("Install a certificate"), "{page}");
    assert!(!page.contains("certificate/trusted"));
    assert!(!page.contains("\"style\":\"button\""));
    // the certificate's acts are acts on a part of the section, dressed as
    // the key set's add is
    assert_eq!(page.matches("\"style\":\"act\"").count(), 3, "{page}");
    // and every one of them leads with the glyph of what it does
    for icon in [
        "\"icon\":\"upload\"",
        "\"icon\":\"refresh-cw\"",
        "\"icon\":\"download\"",
    ] {
        assert!(page.contains(icon), "{icon} in {page}");
    }
    let mut r = request();
    r.path = "/access/certificate/trusted".into();
    assert!(serde_json::to_string(&get(&r))
        .unwrap()
        .contains("This page does not exist."));
}
// The first drawer a tree holds, open or not.
fn drawer_of(v: &serde_json::Value) -> Option<&serde_json::Value> {
    if let Some(d) = v.get("drawer").filter(|d| d.is_object()) {
        return Some(d);
    }
    match v {
        serde_json::Value::Array(a) => a.iter().find_map(drawer_of),
        serde_json::Value::Object(o) => o.values().find_map(drawer_of),
        _ => None,
    }
}
fn certificate_act(path: &str, form: Option<&str>) -> (Envelope, serde_json::Value) {
    let mut r = request();
    r.path = path.into();
    let e = match form {
        Some(f) => post(&r, &Form::parse(f)),
        None => get(&r),
    };
    let j = serde_json::to_value(&e).unwrap();
    (e, j)
}
#[test]
fn certificate_acts_that_replace_it_open_in_a_drawer_over_access() {
    // Install and Make a new one open their forms over Access; Download is a
    // file, and leaves as one.
    let page: serde_json::Value = serde_json::from_str(&access_page(json!([]))).unwrap();
    fn links(v: &serde_json::Value, out: &mut Vec<serde_json::Value>) {
        match v {
            serde_json::Value::Object(o) => {
                if o.get("type").and_then(|t| t.as_str()) == Some("link") {
                    out.push(v.clone());
                }
                o.values().for_each(|c| links(c, out));
            }
            serde_json::Value::Array(a) => a.iter().for_each(|c| links(c, out)),
            _ => {}
        }
    }
    let mut found = vec![];
    links(&page, &mut found);
    for (label, panel) in [
        ("Install a certificate", true),
        ("Make a new one", true),
        ("Download", false),
    ] {
        let link = found
            .iter()
            .find(|l| l["label"] == label)
            .unwrap_or_else(|| panic!("{label} in {page}"));
        assert_eq!(link["panel"].as_bool().unwrap_or(false), panel, "{link}");
    }
}
#[test]
fn a_certificate_act_is_a_drawer_that_closes_back_on_access() {
    for (path, title, submit) in [
        (
            "/access/certificate/install",
            "Install a certificate",
            "Install certificate",
        ),
        (
            "/access/certificate/new",
            "Make a new certificate",
            "Make certificate",
        ),
    ] {
        let (_, j) = certificate_act(path, None);
        let drawer = drawer_of(&j["widget"]).unwrap_or_else(|| panic!("a drawer at {path}: {j}"));
        assert_eq!(drawer["open"], true, "{drawer}");
        assert_eq!(drawer["title"], title);
        assert_eq!(drawer["closed"], "/system/access");
        // Its act runs at once, so it is named for what it does, not "Save",
        // which stages.
        assert_eq!(drawer["children"][0]["submit"], submit, "{drawer}");
        assert_eq!(j["back"]["href"], "/system/access");
    }
}
#[test]
fn a_certificate_act_runs_its_command_and_keeps_its_drawer_for_a_refusal() {
    // A valid submission runs the act; the drawer stays open in the answer,
    // so a refusal from the router is said in it.
    let (e, j) = certificate_act("/access/certificate/new", Some("hostname=router.lan"));
    assert_eq!(e.commands.len(), 1);
    assert_eq!(e.commands[0].name, "certificate-generate");
    assert_eq!(drawer_of(&j["widget"]).unwrap()["open"], true);
    // An invalid one runs nothing and says why, in the drawer.
    let (e, j) = certificate_act("/access/certificate/new", Some("hostname=not%20a%20host"));
    assert!(e.commands.is_empty());
    let drawer = drawer_of(&j["widget"]).unwrap();
    assert_eq!(
        drawer["children"][0]["error"],
        "Enter a valid hostname or IP address."
    );
    let (e, j) = certificate_act("/access/certificate/install", Some("certificate=x&key=y"));
    assert!(e.commands.is_empty());
    let drawer = drawer_of(&j["widget"]).unwrap();
    assert_eq!(
        drawer["children"][0]["error"],
        "Paste a PEM certificate and its private key."
    );
}
