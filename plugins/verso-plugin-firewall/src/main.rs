// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! The Verso firewall plugin: three listings over `/etc/config/firewall`, and a
//! page apiece for editing one rule, one port forward, and one zone.
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

use verso_plugin::{
    commit, json, serve_described, CommitOp, Envelope, Form, Request, Snapshot, Tone,
};

mod activity;
mod conditions;
mod counters;
mod crossings;
mod describe;
mod fields;
mod format;
mod model;
mod page;
mod redirect_drawer;
mod redirect_form;
mod redirects;
mod rename;
mod rule_drawer;
mod rule_form;
mod rules;
mod settings;
// What firewall4 accepts, read out of its own parser. The guard beside it reads
// this today; the forms and the config preview will derive from it rather than
// restating it, which is the point of having it. Until they do, nothing in the
// shipped binary looks at it.
#[allow(dead_code)]
mod vocabulary;
mod zone_drawer;
mod zone_form;
mod zones;

#[cfg(test)]
mod fixture;
// The reckoning: every option firewall4 supports is rendered, deliberately left
// out with a reason, or unsupported upstream. It reads the widget trees the plugin
// really builds, so it needs the serializer the tests have.
#[cfg(test)]
mod reckoning;

use counters::Counters;
use model::{Firewall, CONFIG};

fn main() {
    serve_described("firewall", get, post, describe::describe);
}

fn get(request: &Request) -> Envelope {
    let model = Firewall::read(&request.snapshot).with_rule_files(&request.ubus);
    let counters = Counters::read(&request.ubus);
    match Route::of(&request.path) {
        Route::NotFound => Envelope::not_found(),
        Route::RuleFile(name) => settings::open_file(&model, &name),
        // Rules, port forwards and zones read their own query: each carries which
        // of its objects is open beside it (and, for rules and zones, on which
        // reading). Every other listing is the same page whatever the query says.
        Route::Listing(Listing::Rules) => {
            rules::page_open(&request.snapshot, &model, &counters, &request.query)
        }
        Route::Listing(Listing::PortForwards) => {
            redirects::page_open(&request.snapshot, &model, &counters, &request.query)
        }
        Route::Listing(Listing::Zones) => {
            zones::page_open(&request.snapshot, &model, &request.query)
        }
        Route::Listing(listing) => render(listing, &request.snapshot, &model, &counters),
    }
}

fn post(request: &Request, form: &Form) -> Envelope {
    let mut model = Firewall::read(&request.snapshot).with_rule_files(&request.ubus);
    let counters = Counters::read(&request.ubus);
    match Route::of(&request.path) {
        Route::NotFound => Envelope::not_found(),
        Route::RuleFile(name) => settings::save_file(&model, &name, form),
        // The settings are one form, submitted whole, so they are saved whole
        // rather than read as a single flip.
        Route::Listing(Listing::Settings) => {
            let (envelope, ops) = settings::save(&mut model, form);
            envelope.with_commit(ops)
        }
        // Editing and confirmed removal carry explicit markers. A power act
        // posts to this same address, even while an editor is open, so the
        // query alone must never turn a row action into a form submission.
        Route::Listing(Listing::Rules)
            if fields::from_panel(form) || !form.get(fields::REMOVE_FIELD).is_empty() =>
        {
            rules::save(
                &request.snapshot,
                &mut model,
                &counters,
                &request.query,
                form,
            )
        }
        Route::Listing(Listing::PortForwards)
            if fields::from_panel(form) || !form.get(fields::REMOVE_FIELD).is_empty() =>
        {
            redirects::save(&mut model, &counters, &request.query, form)
        }
        Route::Listing(Listing::Zones)
            if fields::from_panel(form) || !form.get(fields::REMOVE_FIELD).is_empty() =>
        {
            zones::save(&request.snapshot, &mut model, &request.query, form)
        }
        Route::Listing(listing) => flip(&request.snapshot, &mut model, &counters, listing, form),
    }
}

/// flip answers a listing's switch. A body a listing did not draw changes
/// nothing and says so.
fn flip(
    snapshot: &Snapshot,
    model: &mut Firewall,
    counters: &Counters,
    listing: Listing,
    form: &Form,
) -> Envelope {
    let Some(switch) = Switch::read(model, listing, form) else {
        return render(listing, snapshot, model, counters).with_notice(
            Tone::Danger,
            "That change wasn’t recognized, so nothing was saved.",
        );
    };
    let operation = switch.apply(model);
    render(listing, snapshot, model, counters).with_commit(vec![operation])
}

/// Route is what a request's sub-path asks for, by its first segment. A path
/// below a listing that names no section there is a stale link from when a
/// section had a page, and lands on the listing; a path that names no listing
/// at all has nothing at it, and says so rather than passing for Rules.
enum Route {
    Listing(Listing),
    /// A rule file's editor, open over the settings: the name after
    /// `settings/files/`, "new" for one not yet made.
    RuleFile(String),
    NotFound,
}

/// Listing is which of this plugin's pages a request is for. Three are the
/// configuration; the fourth is what it decides, live.
#[derive(Clone, Copy)]
enum Listing {
    Rules,
    PortForwards,
    Zones,
    Settings,
    Activity,
}

impl Route {
    fn of(path: &str) -> Route {
        let path = path.trim_matches('/');
        let (first, rest) = path.split_once('/').unwrap_or((path, ""));
        match first {
            "" => Route::Listing(Listing::Rules),
            // A port forward and a zone are read and edited in the panel beside
            // their listing, at the listing's own address, so a path below either
            // is a stale link from when there was a page, and lands on the listing.
            page::PORT_FORWARDS => Route::Listing(Listing::PortForwards),
            page::ZONES => Route::Listing(Listing::Zones),
            // Settings and Activity have nothing below them but the rule files:
            // the settings are one form, and a verdict's only useful door is the
            // rule that decided it, which is a rule editor's address.
            page::SETTINGS => match rest.strip_prefix("files/") {
                Some(name) => Route::RuleFile(name.to_string()),
                None => Route::Listing(Listing::Settings),
            },
            page::ACTIVITY => Route::Listing(Listing::Activity),
            _ => Route::NotFound,
        }
    }
}

fn render(
    listing: Listing,
    snapshot: &Snapshot,
    model: &Firewall,
    counters: &Counters,
) -> Envelope {
    match listing {
        Listing::Rules => rules::page(model, counters),
        Listing::PortForwards => redirects::page(model, counters),
        Listing::Zones => zones::page(model),
        Listing::Settings => settings::page(model),
        Listing::Activity => activity::page(snapshot),
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
    /// stages. A rule or redirect is on by default (an absent `enabled`), so "on"
    /// CLEARS the option rather than writing enabled=1: a section toggled off then
    /// back on returns to exactly its committed state and the stage coalesces to
    /// clean, not a useless enabled=1. Off still has to say so explicitly.
    fn apply(&self, model: &mut Firewall) -> CommitOp {
        // A section's default-on state is the absence of the option, so its "on"
        // write is a clear (null); "off" is the explicit 0.
        let enabled = if self.on { json!(null) } else { json!("0") };
        match &self.subject {
            Subject::Rule(index) => {
                let rule = &mut model.rules[*index];
                rule.enabled = self.on;
                commit(CONFIG, &rule.section, json!({ "enabled": enabled }))
            }
            Subject::Redirect(index) => {
                let redirect = &mut model.redirects[*index];
                redirect.enabled = self.on;
                commit(CONFIG, &redirect.section, json!({ "enabled": enabled }))
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
        // A zone is switched in its own panel; the listing carries no switch,
        // and the global defaults are Settings' alone.
        Listing::Zones => Vec::new(),
        // The settings page is one form, submitted whole, so it carries no
        // free-standing switch of the kind a listing posts one at a time.
        Listing::Settings => Vec::new(),
        // Activity changes nothing: it is a view of what the configuration
        // already decided, and it renders no switch to post.
        Listing::Activity => Vec::new(),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::rule_form::RuleForm;
    use serde_json::Value;
    use verso_plugin::Ubus;

    fn request(path: &str) -> Request {
        Request {
            path: path.into(),
            query: Form::default(),
            snapshot: fixture::snapshot(),
            ubus: Ubus::from_value(Value::Null),
        }
    }

    // The rail lists the manifest's subpages on every page; they are the ones
    // every page's envelope names, in the same order.
    #[test]
    fn the_manifest_lists_the_pages_every_page_names() {
        let manifest: Value = serde_json::from_str(include_str!("../manifest.json")).expect("manifest");
        let tabs: Vec<Value> = page::tabs()
            .into_iter()
            .map(|t| serde_json::json!({"label": t.label, "path": t.path}))
            .collect();
        assert_eq!(manifest["nav"][0]["pages"], Value::Array(tabs));
    }

    fn answer(path: &str, body: &str) -> Value {
        let envelope = post(&request(path), &Form::parse(body));
        serde_json::to_value(&envelope).expect("serialize")
    }

    // A rule file's editor lives under the settings; saving one there stages
    // the file and nothing else, from the files the request carried.
    #[test]
    fn a_rule_file_is_edited_under_the_settings() {
        let mut saving = request("/settings/files/new");
        saving.ubus = Ubus::from_value(serde_json::json!({"firewallFiles": {"files": []}}));
        let body = serde_json::to_value(post(
            &saving,
            &Form::parse("filename=20-x&content=chain+x+%7B%7D&expected=af63bd4c8601b7df"),
        ))
        .unwrap();
        assert_eq!(
            body["commands"][0]["args"]["path"],
            "/etc/nftables.d/20-x.nft"
        );
        let opened = serde_json::to_value(get(&request("/settings/files/new"))).unwrap();
        assert!(opened.to_string().contains("\"title\":\"New file\""));
    }

    /// asking is a request carrying a query, for the two listings that read one:
    /// which of their objects is open, and on which reading.
    fn asking(path: &str, query: &str) -> Request {
        Request {
            query: Form::parse(query),
            ..request(path)
        }
    }

    /// answer_asking is the same for a submission to an open panel.
    fn answer_asking(path: &str, query: &str, body: &str) -> Value {
        let envelope = post(&asking(path, query), &Form::parse(body));
        serde_json::to_value(&envelope).expect("serialize")
    }

    #[test]
    fn a_row_removal_names_its_subject_independently_of_the_open_editor() {
        for (path, section) in [("/", "allow_ping"), ("/zones", "guest_zone")] {
            for query in ["", "open=new", "open=elsewhere"] {
                let body = answer_asking(path, query, &format!("_remove={section}"));
                assert_eq!(
                    body["commit"],
                    json!([{"config":"firewall", "section":section, "delete":true}])
                );
            }
            for missing in ["missing", "new"] {
                let body = answer_asking(
                    path,
                    &format!("open={section}"),
                    &format!("_remove={missing}"),
                );
                assert!(
                    body.get("commit").is_none(),
                    "a stale removal must stage nothing: {body}"
                );
            }
        }
    }

    /// Each listing page IS its grid, so a test names the listing it expects by
    /// the heading that page carries rather than by a titled region inside it.
    #[test]
    fn each_sub_path_answers_with_its_own_listing() {
        for (path, heading) in [
            ("/", crate::rules::HEADING),
            ("/port-forwards", crate::redirects::HEADING),
            ("/zones", crate::zones::HEADING),
        ] {
            let body = serde_json::to_value(get(&request(path))).expect("serialize");
            assert_eq!(body["title"], heading, "{path}");
            assert_eq!(fixture::listing(&body)["type"], "table", "{path}");
        }
    }

    #[test]
    fn the_plugins_own_address_is_the_rules_listing() {
        for path in ["", "/"] {
            let body = serde_json::to_value(get(&request(path))).expect("serialize");
            assert_eq!(body["title"], crate::rules::HEADING, "{path}");
            assert_eq!(
                fixture::listing(&body)["reorder_config"],
                "firewall",
                "{path}"
            );
        }
    }

    /// A zone is opened on the listing's own address, by query. The panel is the
    /// whole of a zone's editing, so the query is the only way in.
    #[test]
    fn a_zone_opens_in_the_panel_beside_the_listing() {
        let body = serde_json::to_value(get(&asking("/zones", "open=cfg02dc81"))).expect("ok");
        assert_eq!(body["title"], crate::zones::HEADING);
        let listing = fixture::listing(&body);
        let rows = listing["rows"].as_array().expect("rows");
        let open = rows
            .iter()
            .find(|row| row["id"] == "cfg02dc81")
            .expect("row");
        assert_eq!(open["drawer"]["title"], "lan");

        // And the blank one on the page's act, which is where a zone that does
        // not exist yet is made.
        let body = serde_json::to_value(get(&asking("/zones", "open=new"))).expect("ok");
        assert_eq!(body["act"]["drawer"]["title"], "New zone");
    }

    /// Nothing lives below /zones any more: the editor page is gone, so the old
    /// addresses are dead URL shapes rather than zones that went missing. Each
    /// lands on the listing, which is somewhere real.
    #[test]
    fn a_zone_sub_path_answers_with_the_listing() {
        for path in [
            "/zones",
            "/zones/",
            "/zones/new",
            "/zones/cfg02dc81",
            "/zones/lan",
            "/zones/no_such_zone",
            "/zones/a/b",
        ] {
            let body = serde_json::to_value(get(&request(path))).expect("serialize");
            assert_eq!(body["title"], crate::zones::HEADING, "{path}");
            // No panel opens: a path is not how a zone is addressed.
            for row in fixture::listing(&body)["rows"].as_array().expect("rows") {
                assert!(row.get("drawer").is_none(), "{path}");
            }
        }
    }

    /// A submission that says it is the open panel's is that panel's; one that
    /// does not is a switch someone flipped on the listing where it stands.
    #[test]
    fn a_zone_panel_submission_is_the_panels_and_not_a_switchs() {
        let body = answer_asking(
            "/zones",
            "open=guest_zone",
            "_panel=1&name=guest&input=ACCEPT",
        );
        assert_eq!(body["notice"]["level"], "success", "{body}");
        assert_eq!(body["commit"][0]["section"], "guest_zone", "{body}");
        assert!(body["commit"][0].get("delete").is_none(), "{body}");
        // A zone the config does not hold has nothing to save, and says so rather
        // than writing anything.
        let body = answer_asking("/zones", "open=no_such_zone", "_panel=1&input=ACCEPT");
        assert_eq!(body["title"], crate::zones::HEADING);
        assert_eq!(body["notice"]["level"], "danger");
        assert!(body.get("commit").is_none());
    }

    /// A row's power act posts to the page it sits on, and while a panel is open
    /// that page's address names the panel. The act is still a flip: it does not
    /// say it is the panel's, so it is never read as the panel's form — which
    /// would write a rule stripped of everything but its switch.
    #[test]
    fn a_row_act_at_an_open_panels_address_is_a_flip_and_never_a_save() {
        let body = answer_asking("/", "open=allow_ping", "allow_ping=off");
        assert_eq!(
            body["commit"],
            serde_json::json!([{
                "config": "firewall",
                "section": "allow_ping",
                "values": {"enabled": "0"}
            }])
        );
        // Another rule's act while this one's panel is open: the address names
        // one rule and the act another, and the act is what arrived.
        let body = answer_asking("/", "open=allow_ping&tab=action", "block_telnet=on");
        assert_eq!(
            body["commit"],
            serde_json::json!([{
                "config": "firewall",
                "section": "block_telnet",
                "values": {"enabled": null}
            }])
        );
    }

    /// The panel's own form posts its marker with its fields, and the marker is
    /// what routes the submission to the panel — so a rule or a zone saved from
    /// its panel is written whole, by the router and not only by the save.
    #[test]
    fn a_panels_own_submission_reaches_the_panel_by_its_marker() {
        let page = serde_json::to_value(get(&asking("/", "open=allow_ping"))).expect("serialize");
        let submission = fixture::submission(&page);
        assert!(
            submission.split('&').any(|pair| pair == "_panel=1"),
            "the panel's form posts no marker: {submission}"
        );
        let body = answer_asking("/", "open=allow_ping", &submission);
        assert_eq!(body["notice"]["level"], "success", "{body}");
        let intact = RuleForm::read(
            &fixture::snapshot()
                .section(CONFIG, "allow_ping")
                .expect("the fixture's rule"),
        )
        .values(true);
        assert_eq!(
            body["commit"],
            serde_json::json!([{
                "config": "firewall",
                "section": "allow_ping",
                "values": intact
            }])
        );

        let page =
            serde_json::to_value(get(&asking("/zones", "open=cfg02dc81"))).expect("serialize");
        let body = answer_asking("/zones", "open=cfg02dc81", &fixture::submission(&page));
        assert_eq!(body["notice"]["level"], "success", "{body}");
        assert_eq!(body["commit"][0]["section"], "cfg02dc81");
    }

    #[test]
    fn a_read_without_brokered_counters_still_renders() {
        let body = serde_json::to_value(get(&request("/"))).expect("serialize");
        let rows = fixture::listing(&body)["rows"]
            .as_array()
            .expect("rows")
            .clone();
        assert!(rows
            .iter()
            .all(|row| row["cells"][8] == serde_json::json!({"text": "—", "muted": true})));
    }

    #[test]
    fn flipping_a_rule_writes_0_off_and_clears_the_option_on() {
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
        let rows = fixture::listing(&off)["rows"]
            .as_array()
            .expect("rows")
            .clone();
        let ping = rows
            .iter()
            .find(|row| row["id"] == "allow_ping")
            .expect("row");
        // The row now offers to turn it back on, which is the whole proof that
        // the answer carries the change rather than the state it was read in.
        assert_eq!(ping["cells"][9]["actions"][0]["value"], "on");
        assert_eq!(ping["muted"], true);

        // "on" is a section's default, so it clears the option rather than writing
        // enabled=1 — a rule toggled off then back on returns to its committed
        // state and the stage coalesces to clean instead of leaving a no-op behind.
        let on = answer("/", "block_telnet=on");
        assert_eq!(
            on["commit"],
            serde_json::json!([{
                "config": "firewall",
                "section": "block_telnet",
                "values": {"enabled": null}
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

    /// A defaults switch flipped back to what firewall4 assumes clears the
    /// option rather than writing that assumption out, so on then off returns
    /// the section to its committed state and the stage coalesces to clean.
    // The global defaults are Settings' alone: a switch named after one of
    // them, posted to the Zones listing, is not something that page drew.
    #[test]
    fn the_zones_listing_stages_no_global_default() {
        for body in ["drop_invalid=on", "flow_offloading=on", "synflood_protect=off"] {
            let answer = answer("/zones", body);
            assert!(
                answer["commit"].as_array().is_none_or(Vec::is_empty),
                "{body}: {answer}"
            );
        }
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
                    "text": "That change wasn’t recognized, so nothing was saved."
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

    /// A port forward opens in the panel its listing's address names, and a new
    /// one in the same panel, blank.
    #[test]
    fn a_port_forward_opens_in_the_panel_its_address_names() {
        let mut open = request("/port-forwards");
        open.query = Form::parse("open=https_to_nas");
        let body = serde_json::to_value(get(&open)).expect("serialize");
        assert_eq!(body["title"], crate::redirects::HEADING);
        let drawer = fixture::find_with(&body, &|v| v["title"] == "HTTPS-to-NAS")
            .expect("the forward's panel");
        assert_eq!(drawer["open"], true);

        open.query = Form::parse("open=new");
        let body = serde_json::to_value(get(&open)).expect("serialize");
        assert_eq!(body["act"]["drawer"]["title"], "New port forward");
    }

    /// A path that names nothing the firewall publishes — not a listing, not a
    /// stale link below one — has nothing at it, and says so rather than
    /// passing for the Rules page; a read or a post alike. The plugin's own
    /// address is still Rules.
    #[test]
    fn a_path_that_names_nothing_has_nothing_at_it() {
        for path in ["/nope", "/rule", "/zone-list/lan"] {
            let body = serde_json::to_value(get(&request(path))).expect("serialize");
            assert_eq!(body["not_found"], true, "{path}");
            assert_eq!(answer(path, "allow_ping=off")["not_found"], true, "{path}");
        }
        for path in ["", "/"] {
            let body = serde_json::to_value(get(&request(path))).expect("serialize");
            assert!(body.get("not_found").is_none(), "{path}");
            assert_eq!(body["title"], crate::rules::HEADING, "{path}");
        }
    }

    /// There is nothing below the listing's address any more: a path that names a
    /// forward is a link from when a forward had a page, and lands on the listing.
    #[test]
    fn a_path_below_the_port_forwards_lands_on_the_listing() {
        for path in [
            "/port-forwards",
            "/port-forwards/",
            "/port-forwards/new",
            "/port-forwards/https_to_nas",
            "/port-forwards/no_such_forward",
        ] {
            let body = serde_json::to_value(get(&request(path))).expect("serialize");
            assert_eq!(body["title"], crate::redirects::HEADING, "{path}");
            assert!(
                fixture::find_with(&body, &|v| v.get("drawer").is_some_and(|d| d.is_object()))
                    .is_none(),
                "{path} opened a panel"
            );
        }
        // A body posted there that is neither the panel's nor a row's act is a
        // switch the listing did not draw, and changes nothing.
        let body = answer("/port-forwards/https_to_nas", "src=wan");
        assert!(body.get("commit").is_none());
        assert_eq!(body["notice"]["level"], "danger");
    }

    /// The listing's address takes the panel's form, a row's removal and a row's
    /// switch alike, so a body is the panel's only when it says so — and a
    /// removal is a removal, never read as a switch.
    #[test]
    fn a_port_forward_panel_submission_is_the_panels_and_not_a_switchs() {
        let mut saving = request("/port-forwards");
        saving.query = Form::parse("open=https_to_nas");
        let body = serde_json::to_value(post(
            &saving,
            &Form::parse("_panel=1&enabled=1&src=wan&src_dport=8443&proto=tcp"),
        ))
        .expect("serialize");
        assert_eq!(body["commit"][0]["section"], "https_to_nas");
        assert_eq!(body["commit"][0]["values"]["src_dport"], "8443");

        let body = answer("/port-forwards", "_remove=https_to_nas");
        assert_eq!(
            body["commit"],
            serde_json::json!([{
                "config": "firewall",
                "section": "https_to_nas",
                "delete": true
            }])
        );
    }
}
