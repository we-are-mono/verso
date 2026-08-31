// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! The Verso firewall plugin: three listings over `/etc/config/firewall`, and a
//! page apiece for editing one rule and one port forward.
//!
//! Every request is answered from the reads the shell brokers with it — the uci
//! snapshot and firewall4's kernel hit counters — so the plugin holds no state
//! between requests and reaches nothing itself (ADR-007).
//!
//! A listing's only change is flipping a switch, and a switch outside a form is
//! a complete instruction: it posts to the page it sits on and that page answers
//! with itself, re-rendered. The snapshot the answer is built from was read
//! before the change was staged, so the accepted change is applied to the model
//! and the page renders from the model — what the operator flipped is what they
//! see, and the commit intent beside it is what the shell stages. The editor
//! answers its own submissions the same way, from the values it was sent.

use verso_plugin::{commit, commit_new, json, serve, CommitOp, Envelope, Form, Request, Tone};

mod conditions;
mod counters;
mod editor;
mod format;
mod model;
mod page;
mod redirect_editor;
mod redirects;
mod rule_form;
mod rules;
mod zones;

#[cfg(test)]
mod fixture;

use counters::Counters;
use model::{Firewall, CONFIG};

fn main() {
    serve("firewall", get, post);
}

fn get(request: &Request) -> Envelope {
    let model = Firewall::read(&request.snapshot);
    let counters = Counters::read(&request.ubus);
    match Route::of(&request.path) {
        Route::Listing(listing) => render(listing, &model, &counters),
        Route::NewRule => editor::blank(&model),
        Route::EditRule(section) => editor::edit(&request.snapshot, &model, &section)
            .unwrap_or_else(|| editor::missing(&model, &counters)),
        Route::NewRedirect => redirect_editor::blank(&model),
        Route::EditRedirect(section) => {
            redirect_editor::edit(&request.snapshot, &model, &section)
                .unwrap_or_else(|| redirect_editor::missing(&model, &counters))
        }
    }
}

fn post(request: &Request, form: &Form) -> Envelope {
    let mut model = Firewall::read(&request.snapshot);
    let counters = Counters::read(&request.ubus);
    match Route::of(&request.path) {
        Route::NewRule => editor::create(&model, form),
        Route::EditRule(section) => editor::save(&mut model, &counters, &section, form)
            .unwrap_or_else(|| editor::missing(&model, &counters)),
        Route::NewRedirect => redirect_editor::create(&model, form),
        Route::EditRedirect(section) => {
            redirect_editor::save(&mut model, &counters, &section, form)
                .unwrap_or_else(|| redirect_editor::missing(&model, &counters))
        }
        Route::Listing(listing) => flip(&mut model, &counters, listing, form),
    }
}

/// flip answers a listing's switch. A body a listing did not draw changes
/// nothing and says so.
fn flip(model: &mut Firewall, counters: &Counters, listing: Listing, form: &Form) -> Envelope {
    let Some(switch) = Switch::read(model, listing, form) else {
        return render(listing, model, counters).with_notice(
            Tone::Danger,
            "Verso couldn’t tell what that change was, so nothing was saved.",
        );
    };
    let operation = switch.apply(model);
    render(listing, model, counters).with_commit(vec![operation])
}

/// Route is what a request's sub-path asks for. A sub-path the plugin does not
/// publish answers with the listing it leads with, so a stale link lands
/// somewhere real; a sub-path below a listing that names no section there is
/// that editor's to answer, because only it knows what it can edit.
enum Route {
    Listing(Listing),
    NewRule,
    EditRule(String),
    NewRedirect,
    EditRedirect(String),
}

/// Listing is which of the three listings a request is for.
#[derive(Clone, Copy)]
enum Listing {
    Rules,
    PortForwards,
    Zones,
}

impl Route {
    fn of(path: &str) -> Route {
        let path = path.trim_matches('/');
        if let Some(rest) = path.strip_prefix(page::RULE_EDITOR) {
            match rest.trim_start_matches('/') {
                "" => return Route::Listing(Listing::Rules),
                page::NEW => return Route::NewRule,
                section => return Route::EditRule(section.to_string()),
            }
        }
        if let Some(rest) = path.strip_prefix(page::PORT_FORWARDS) {
            match rest.trim_start_matches('/') {
                "" => return Route::Listing(Listing::PortForwards),
                page::NEW => return Route::NewRedirect,
                section => return Route::EditRedirect(section.to_string()),
            }
        }
        match path {
            page::ZONES => Route::Listing(Listing::Zones),
            _ => Route::Listing(Listing::Rules),
        }
    }
}

fn render(listing: Listing, model: &Firewall, counters: &Counters) -> Envelope {
    match listing {
        Listing::Rules => rules::page(model, counters),
        Listing::PortForwards => redirects::page(model, counters),
        Listing::Zones => zones::page(model),
    }
}

/// Switch is one accepted flip: what it turns on or off, and where.
struct Switch {
    subject: Subject,
    on: bool,
}

/// Subject is the thing a switch belongs to. Sections are held by index rather
/// than by name so applying the change and stating it are the same lookup.
enum Subject {
    Rule(usize),
    Redirect(usize),
    Default(String),
}

impl Switch {
    /// read finds the flip a submission carries. A page accepts only the switches
    /// it rendered, and only one of them: the browser posts a single flip, so a
    /// body naming several is not something this plugin drew.
    fn read(model: &Firewall, listing: Listing, form: &Form) -> Option<Switch> {
        let mut named: Vec<Switch> = Vec::new();
        for (name, subject) in switches(model, listing) {
            let value = form.get(&name);
            if value.is_empty() {
                continue;
            }
            let on = match value.as_str() {
                "on" => true,
                "off" => false,
                _ => return None,
            };
            named.push(Switch { subject, on });
        }
        match named.len() {
            1 => named.pop(),
            _ => None,
        }
    }

    /// apply reflects the flip in the model and states it as the write the shell
    /// stages. `enabled` is written explicitly in both directions: an absent
    /// option means enabled, so turning a section off has to say so.
    fn apply(&self, model: &mut Firewall) -> CommitOp {
        let value = if self.on { "1" } else { "0" };
        match &self.subject {
            Subject::Rule(index) => {
                let rule = &mut model.rules[*index];
                rule.enabled = self.on;
                commit(CONFIG, &rule.section, json!({ "enabled": value }))
            }
            Subject::Redirect(index) => {
                let redirect = &mut model.redirects[*index];
                redirect.enabled = self.on;
                commit(CONFIG, &redirect.section, json!({ "enabled": value }))
            }
            Subject::Default(option) => {
                model.defaults.set(option, self.on);
                let values = json!({ option.as_str(): value });
                // A device with no defaults section runs firewall4's built-in
                // baseline; writing one of its options means creating it.
                if model.defaults.section.is_empty() {
                    commit_new(CONFIG, "defaults", values)
                } else {
                    commit(CONFIG, &model.defaults.section, values)
                }
            }
        }
    }
}

/// switches is every switch a listing rendered, by the form name it posts under.
fn switches(model: &Firewall, listing: Listing) -> Vec<(String, Subject)> {
    match listing {
        Listing::Rules => model
            .rules
            .iter()
            .enumerate()
            .map(|(index, rule)| (rule.section.clone(), Subject::Rule(index)))
            .collect(),
        Listing::PortForwards => model
            .redirects
            .iter()
            .enumerate()
            .filter(|(_, redirect)| redirect.is_port_forward())
            .map(|(index, redirect)| (redirect.section.clone(), Subject::Redirect(index)))
            .collect(),
        Listing::Zones => zones::options()
            .map(|option| (option.to_string(), Subject::Default(option.to_string())))
            .collect(),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::Value;
    use verso_plugin::{Snapshot, Ubus};

    fn request(path: &str) -> Request {
        Request {
            path: path.into(),
            snapshot: fixture::snapshot(),
            ubus: Ubus::from_value(Value::Null),
        }
    }

    fn answer(path: &str, body: &str) -> Value {
        let envelope = post(&request(path), &Form::parse(body));
        serde_json::to_value(&envelope).expect("serialize")
    }

    /// listing is the titled region a page answered with, which is how a test
    /// names the listing it expects without counting a stack's children.
    fn listing(body: &Value, title: &str) -> Value {
        fixture::section(body, title)
    }

    #[test]
    fn each_sub_path_answers_with_its_own_listing() {
        for (path, section) in [
            ("/", "Traffic rules"),
            ("/port-forwards", "Port forwards and redirects"),
            ("/zones", "Zones"),
        ] {
            let body = serde_json::to_value(get(&request(path))).expect("serialize");
            assert_eq!(listing(&body, section)["title"], section, "{path}");
        }
    }

    #[test]
    fn an_unpublished_sub_path_answers_with_the_rules_listing() {
        for path in ["/nowhere", "/zones/lan", ""] {
            let body = serde_json::to_value(get(&request(path))).expect("serialize");
            assert_eq!(listing(&body, "Traffic rules")["title"], "Traffic rules", "{path}");
        }
    }

    #[test]
    fn a_read_without_brokered_counters_still_renders() {
        let body = serde_json::to_value(get(&request("/"))).expect("serialize");
        let rows = fixture::table(&body, "Traffic rules")["rows"]
            .as_array()
            .expect("rows")
            .clone();
        assert!(rows
            .iter()
            .all(|row| row["cells"][7] == serde_json::json!({"text": "—", "muted": true})));
    }

    #[test]
    fn flipping_a_rule_writes_its_enabled_option_both_ways() {
        let off = answer("/", "allow_ping=off");
        assert_eq!(
            off["commit"],
            serde_json::json!([{
                "config": "firewall",
                "section": "allow_ping",
                "values": {"enabled": "0"}
            }])
        );
        // The answer is the page with the change already in it.
        let rows = fixture::table(&off, "Traffic rules")["rows"]
            .as_array()
            .expect("rows")
            .clone();
        let ping = rows.iter().find(|row| row["id"] == "allow_ping").expect("row");
        assert_eq!(ping["cells"][8], serde_json::json!({"name": "allow_ping"}));

        let on = answer("/", "block_telnet=on");
        assert_eq!(
            on["commit"],
            serde_json::json!([{
                "config": "firewall",
                "section": "block_telnet",
                "values": {"enabled": "1"}
            }])
        );
    }

    #[test]
    fn flipping_a_port_forward_writes_its_enabled_option() {
        let body = answer("/port-forwards", "https_to_nas=off");
        assert_eq!(
            body["commit"],
            serde_json::json!([{
                "config": "firewall",
                "section": "https_to_nas",
                "values": {"enabled": "0"}
            }])
        );
    }

    #[test]
    fn flipping_a_defaults_option_writes_the_defaults_section() {
        let body = answer("/zones", "flow_offloading=on");
        assert_eq!(
            body["commit"],
            serde_json::json!([{
                "config": "firewall",
                "section": "cfg01e63d",
                "values": {"flow_offloading": "1"}
            }])
        );
        let settings = &fixture::section(&body, "Global defaults")["children"][0];
        assert_eq!(
            settings["items"][3]["toggle"],
            serde_json::json!({"name": "flow_offloading", "on": true})
        );
    }

    #[test]
    fn a_defaults_option_on_a_config_without_one_creates_the_section() {
        let request = Request {
            path: "/zones".into(),
            snapshot: Snapshot::from_value(json!({"firewall": {}, "network": {}})),
            ubus: Ubus::from_value(Value::Null),
        };
        let body =
            serde_json::to_value(post(&request, &Form::parse("drop_invalid=on"))).expect("serialize");
        assert_eq!(
            body["commit"],
            serde_json::json!([{
                "config": "firewall",
                "section": "",
                "type": "defaults",
                "values": {"drop_invalid": "1"}
            }])
        );
    }

    #[test]
    fn a_body_this_page_did_not_draw_changes_nothing() {
        for (path, body) in [
            ("/", "allow_ping=maybe"),
            ("/", "no_such_section=on"),
            ("/", ""),
            // A rule is not a switch the port-forwards page rendered.
            ("/port-forwards", "allow_ping=off"),
            // A defaults option is not a switch the rules page rendered.
            ("/", "flow_offloading=on"),
            // Two flips at once is not something the browser posts.
            ("/", "allow_ping=off&block_telnet=on"),
        ] {
            let answer = answer(path, body);
            assert!(answer.get("commit").is_none(), "{path} {body}");
            assert_eq!(
                answer["notice"],
                serde_json::json!({
                    "level": "danger",
                    "text": "Verso couldn’t tell what that change was, so nothing was saved."
                }),
                "{path} {body}"
            );
        }
    }

    #[test]
    fn a_snat_redirects_switch_is_not_on_the_port_forwards_page() {
        let answer = answer("/port-forwards", "nas_snat=off");
        assert!(answer.get("commit").is_none());
        assert_eq!(answer["notice"]["level"], "danger");
    }

    #[test]
    fn a_rule_sub_path_opens_that_rules_editor() {
        let body = serde_json::to_value(get(&request("/rules/allow_ping"))).expect("serialize");
        assert_eq!(body["title"], "Edit rule");
        assert_eq!(body["subheading"], "Allow-Ping");

        let body = serde_json::to_value(get(&request("/rules/new"))).expect("serialize");
        assert_eq!(body["title"], "New rule");

        // The editor's own sub-path root is still the listing it belongs to.
        for path in ["/rules", "/rules/"] {
            let body = serde_json::to_value(get(&request(path))).expect("serialize");
            assert_eq!(body["title"], "Firewall", "{path}");
            assert_eq!(listing(&body, "Traffic rules")["title"], "Traffic rules", "{path}");
        }
    }

    #[test]
    fn a_rule_sub_path_naming_no_rule_answers_with_the_listing() {
        for path in ["/rules/no_such_rule", "/rules/https_to_nas", "/rules/a/b"] {
            for body in [
                serde_json::to_value(get(&request(path))).expect("serialize"),
                answer(path, "target=ACCEPT"),
            ] {
                assert_eq!(body["title"], "Firewall", "{path}");
                assert_eq!(body["notice"]["level"], "danger", "{path}");
                assert!(body.get("commit").is_none(), "{path}");
            }
        }
    }

    #[test]
    fn a_port_forward_sub_path_opens_that_forwards_editor() {
        let body = serde_json::to_value(get(&request("/port-forwards/https_to_nas")))
            .expect("serialize");
        assert_eq!(body["title"], "Edit port forward");
        assert_eq!(body["subheading"], "HTTPS-to-NAS");

        let body = serde_json::to_value(get(&request("/port-forwards/new"))).expect("serialize");
        assert_eq!(body["title"], "New port forward");

        // The editor's own sub-path root is still the listing it belongs to.
        for path in ["/port-forwards", "/port-forwards/"] {
            let body = serde_json::to_value(get(&request(path))).expect("serialize");
            assert_eq!(body["title"], "Firewall", "{path}");
            assert_eq!(
                listing(&body, "Port forwards and redirects")["title"],
                "Port forwards and redirects",
                "{path}"
            );
        }
    }

    #[test]
    fn a_port_forward_sub_path_naming_no_forward_answers_with_the_listing() {
        for path in [
            "/port-forwards/no_such_forward",
            "/port-forwards/nas_snat",
            "/port-forwards/allow_ping",
        ] {
            for body in [
                serde_json::to_value(get(&request(path))).expect("serialize"),
                answer(path, "src=wan"),
            ] {
                assert_eq!(body["title"], "Firewall", "{path}");
                assert_eq!(body["notice"]["level"], "danger", "{path}");
                assert!(body.get("commit").is_none(), "{path}");
            }
        }
    }

    #[test]
    fn a_port_forward_editor_submission_is_the_editors_and_not_a_switchs() {
        let body = answer("/port-forwards/https_to_nas", "_delete=1");
        assert_eq!(
            body["commit"],
            serde_json::json!([{
                "config": "firewall",
                "section": "https_to_nas",
                "delete": true
            }])
        );
    }

    #[test]
    fn a_rules_editor_submission_is_the_editors_and_not_a_switchs() {
        let body = answer("/rules/allow_ping", "_delete=1");
        assert_eq!(
            body["commit"],
            serde_json::json!([{
                "config": "firewall",
                "section": "allow_ping",
                "delete": true
            }])
        );
    }
}
