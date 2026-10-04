// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! The Activity view: what the firewall is deciding, as it decides it.
//!
//! The other three pages are the configuration. This one is the consequence — a
//! live listing of verdicts, each row naming the traffic it judged and linking
//! to the rule that judged it. The plugin declares the columns, the source, and
//! the words; the shell owns the stream, the pause, the ring, and the narrowing
//! that clicking a value in a row does.
//!
//! The honesty this page has to keep: **firewall4 logs only what is asked of
//! it**. A rule logs when its "Log matching packets" is on and a zone logs its
//! own policy when told to; everything else is decided in silence. So an empty
//! stream with nothing configured does not mean a quiet network — it means
//! nothing is being recorded, and the page says exactly that instead of showing
//! an empty table that reads like calm.

use verso_plugin::{
    ActionBar, ColumnWidth, Envelope, HeadingAct, Section, Snapshot, Table,
    TableColumn, TableStream, Widget, STREAM_FIREWALL_LOG,
};

use crate::model;
use crate::model::CONFIG;
use crate::page;
use crate::zone_form;

const HEADING: &str = "Activity";

const EMPTY_TEXT: &str = "Waiting for the first logged event…";

const NOTHING_LOGGED: &str = "Nothing is being logged";

const NOTHING_LOGGED_BODY: &str = "The firewall decides quietly by default. Turn on **Log \
matching packets** on a rule you're curious about, and its verdicts appear here the moment they \
happen.";

/// RING is how many verdicts the browser keeps. Enough to hold a burst and read
/// back through it; not so many that the page becomes a log file.
const RING: u32 = 200;

/// page renders the live view, or the teaching that has to come before it.
/// Either way the page is a pure read — there is nothing here to stage — so
/// the envelope is immediate: nothing on it stages.
pub fn page(snapshot: &Snapshot) -> Envelope {
    if !logging_configured(snapshot) {
        return page::envelope(HEADING, teaching()).immediate();
    }
    // The stream is the page, so the page is the window: it fills the height,
    // scrolls inside itself, and runs to the right and bottom edges. A log has
    // no natural end, and a margin under one would say otherwise.
    page::envelope(HEADING, live())
        .with_act(download())
        .with_width("full")
        .immediate()
}

/// live is the stream itself: the bar that cuts it and holds it still, and the
/// console it is printed into.
fn live() -> Widget {
    Widget::stack(vec![bar(), console()])
}

/// bar is the console's own controls: the free-text lens over everything on
/// screen and the control that holds the stream still, which the shell stands
/// on the heading line.
fn bar() -> Widget {
    Widget::ActionBar(ActionBar {
        tabs: Vec::new(),
        filter: "Find an address, port or rule".into(),
        live: "Live".into(),
    })
}

/// download is the page's act. Taking the buffer away with you is not the
/// forward act this page is for — the page is for watching — so it wears the
/// quiet dress, beside the live control as its equal: words alone, like it.
fn download() -> HeadingAct {
    HeadingAct {
        label: "Download".into(),
        href: format!("{}/{}", page::MOUNT, DOWNLOAD),
        style: "quiet".into(),
        ..Default::default()
    }
}

/// DOWNLOAD is the sub-path the buffer is fetched from.
const DOWNLOAD: &str = "download";

/// teaching is the page with nothing to show and a reason: the firewall is not
/// being asked to record anything, and the door to the page where that is
/// turned on.
fn teaching() -> Widget {
    Widget::empty(
        "activity",
        NOTHING_LOGGED,
        NOTHING_LOGGED_BODY,
        vec![Widget::link(
            "Open traffic rules",
            &page::rules_href(),
            "button",
        )],
    )
}

/// console is where the verdicts are printed: one line per event, no grid, on
/// the ground a terminal writes to. It renders with no lines on purpose — they
/// arrive afterwards, newest on top. The columns are still declared, because
/// they are what a line is made of even where no heading names them.
fn console() -> Widget {
    Widget::Table(Table {
        style: "console".into(),
        dense: true,
        columns: columns(),
        empty_text: EMPTY_TEXT.into(),
        stream: Some(TableStream {
            source: STREAM_FIREWALL_LOG.into(),
            ring: RING,
        }),
        ..Default::default()
    })
}

/// columns are the facts a verdict carries, in the order a person reads them:
/// when it happened and how often, what was decided, and then the traffic path
/// — where from, by whom, to what, over which protocol and port — ending at the
/// rule that decided it.
fn columns() -> Vec<TableColumn> {
    [
        ("When", "runtime", ColumnWidth::Short),
        ("Count", "num", ColumnWidth::Count),
        ("Verdict", "pill", ColumnWidth::Short),
        ("From", "endpoint", ColumnWidth::Word),
        ("Source", "mono", ColumnWidth::Address),
        ("To", "endpoint", ColumnWidth::Word),
        ("Protocol", "keyword", ColumnWidth::Short),
        ("Port", "mono", ColumnWidth::Short),
        ("Rule", "link", ColumnWidth::Grow),
    ]
    .into_iter()
    .map(|(label, kind, width)| TableColumn {
        label: label.into(),
        kind: kind.into(),
        width,
    })
    .collect()
}

/// logging_configured reports whether anything in the firewall config asks to
/// be recorded — a rule, a port forward, or a zone's own policy. It is read
/// from the snapshot rather than from the model because it is this page's own
/// question about the whole config, not a fact any one listing shows.
fn logging_configured(snapshot: &Snapshot) -> bool {
    ["rule", "redirect", "zone"].into_iter().any(|typ| {
        snapshot
            .sections_of_type(CONFIG, typ)
            .iter()
            .any(|section| logs(typ, section))
    })
}

/// logs reads one section's `log` option the way firewall4 does — which is not
/// one reading but two, because the option is not one type.
///
/// On a rule or a port forward it is a **string**: a written truth value means
/// what it says, and anything else that is not empty is a literal log prefix,
/// which is logging with a name of its own. On a **zone** it is an integer
/// bitfield — bit 1 records refusals, bit 2 records MSS clamps — and a value fw4
/// cannot read as a number makes it skip the zone entirely, which records
/// nothing at all. `log 'on'` is therefore logging on a rule and a dead zone on
/// a zone, and this page must not promise verdicts the second will never write.
fn logs(typ: &str, section: &Section) -> bool {
    let value = section.scalar("log");
    if value.is_empty() {
        return false;
    }
    if typ == "zone" {
        let bits = zone_form::log_bits(&value);
        return bits.refused || bits.mss;
    }
    model::boolean(&value).unwrap_or(true)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::fixture;
    use serde_json::{json, Value};

    fn body(snapshot: &Snapshot) -> Value {
        serde_json::to_value(page(snapshot)).expect("serialize")
    }

    /// logging_snapshot is the fixture's router with one rule asked to log —
    /// the difference between the teaching state and the live one.
    fn logging_snapshot() -> Snapshot {
        Snapshot::from_value(json!({
            "firewall": {
                "block_telnet": {
                    ".type": "rule", ".name": "block_telnet",
                    "name": "Block-Telnet", "src": "wan", "target": "DROP", "log": "1"
                }
            },
            "network": {}
        }))
    }

    /// There is nothing to stage on a listing of what already happened, in
    /// either of its states — nothing on it stages.
    #[test]
    fn the_page_is_immediate_in_both_states() {
        assert_eq!(body(&logging_snapshot())["immediate"], true);
        assert_eq!(body(&fixture::snapshot())["immediate"], true);
    }

    /// The stream is the page, so the page is the window: it fills the height
    /// and runs to the edges, and the console is what fills it.
    #[test]
    fn the_live_page_is_the_window_and_the_console_fills_it() {
        let body = body(&logging_snapshot());
        assert_eq!(body["width"], "full");
        let table = fixture::listing(&body);
        assert_eq!(table["style"], "console");
        assert_eq!(table["stream"]["ring"], RING);
    }

    /// The bar is the search and the control that holds the stream still,
    /// which the shell stands on the heading line; the log is read whole.
    #[test]
    fn the_bar_offers_the_search_and_the_pause() {
        let body = body(&logging_snapshot());
        let bar = &body["widget"]["children"][0];
        assert_eq!(bar["type"], "actionbar");
        assert_eq!(bar["live"], "Live");
        assert_eq!(bar["filter"], "Find an address, port or rule");
        assert!(bar.get("tabs").is_none(), "{bar}");
        // Nothing is added here — the page is a read. The page's one act takes
        // the buffer away with you, so it wears the quiet weight, in words alone
        // like the live control it stands beside on the heading line.
        assert_eq!(
            body["act"],
            json!({
                "label": "Download",
                "href": "/plugins/firewall/download",
                "style": "quiet"
            })
        );
    }

    #[test]
    fn the_live_listing_declares_its_source_and_its_ring() {
        let body = body(&logging_snapshot());
        let table = fixture::listing(&body);
        assert_eq!(
            table["stream"],
            json!({"source": "firewall-log", "ring": 200})
        );
        assert_eq!(table["dense"], true);
        // The rows arrive afterwards; the page renders none of its own.
        assert_eq!(table["rows"], json!([]));
        assert_eq!(table["empty_text"], "Waiting for the first logged event…");
    }

    #[test]
    fn the_columns_are_the_verdict_and_the_path_it_judged() {
        let body = body(&logging_snapshot());
        let table = fixture::listing(&body);
        let kinds: Vec<(String, String)> = table["columns"]
            .as_array()
            .expect("columns")
            .iter()
            .map(|c| {
                (
                    c["label"].as_str().unwrap_or_default().to_string(),
                    c["kind"].as_str().unwrap_or_default().to_string(),
                )
            })
            .collect();
        assert_eq!(
            kinds,
            vec![
                ("When".to_string(), "runtime".to_string()),
                ("Count".to_string(), "num".to_string()),
                ("Verdict".to_string(), "pill".to_string()),
                ("From".to_string(), "endpoint".to_string()),
                ("Source".to_string(), "mono".to_string()),
                ("To".to_string(), "endpoint".to_string()),
                ("Protocol".to_string(), "keyword".to_string()),
                ("Port".to_string(), "mono".to_string()),
                ("Rule".to_string(), "link".to_string()),
            ]
        );
    }

    /// The stream's values are its controls, but the open-ended rest — a port,
    /// an address fragment — needs a free-text lens, and it belongs on the
    /// console's own bar rather than floating above the page.
    #[test]
    fn the_live_page_declares_the_one_free_text_lens() {
        let body = body(&logging_snapshot());
        assert_eq!(
            fixture::widget(&body, "actionbar")["filter"],
            "Find an address, port or rule"
        );
    }

    /// Silence with nothing configured to log is not a quiet network — it is
    /// nothing being recorded, and the page has to say which.
    #[test]
    fn a_config_that_asks_for_no_logging_teaches_instead_of_showing_an_empty_table() {
        let body = body(&fixture::snapshot());
        let empty = fixture::widget(&body, "empty");
        assert_eq!(empty["title"], "Nothing is being logged");
        assert_eq!(empty["icon"], "activity");
        assert_eq!(empty["children"][0]["href"], "/plugins/firewall/");
        assert_eq!(empty["children"][0]["label"], "Open traffic rules");
        // No table at all: an empty listing here would read like calm.
        assert!(serde_json::to_string(&body)
            .expect("json")
            .find("\"stream\"")
            .is_none());
    }

    #[test]
    fn any_section_asked_to_log_is_enough_to_make_the_page_live() {
        for (typ, value) in [
            ("rule", "1"),
            ("redirect", "on"),
            // A zone's log is a bitfield: either bit writes lines.
            ("zone", "1"),
            ("zone", "2"),
            ("zone", "0x3"),
            // fw4 reads a value that is not a truth value as a literal log
            // prefix, which is still logging.
            ("rule", "scan-audit: "),
        ] {
            let snapshot = Snapshot::from_value(json!({
                "firewall": { "cfg01": { ".type": typ, ".name": "cfg01", "log": value } },
                "network": {}
            }));
            assert!(
                logging_configured(&snapshot),
                "{typ} log={value} should be logging"
            );
        }
        for value in ["0", "off", "false", "no", ""] {
            let snapshot = Snapshot::from_value(json!({
                "firewall": { "cfg01": { ".type": "rule", ".name": "cfg01", "log": value } },
                "network": {}
            }));
            assert!(!logging_configured(&snapshot), "log={value} is not logging");
        }
    }

    /// A zone's `log` is an integer to firewall4, and `on` is not an integer:
    /// fw4 skips such a zone entirely, so it writes no verdicts at all. Promising
    /// a live listing for one would leave a person watching an empty table for
    /// events that can never come — the exact silence this page exists to
    /// explain.
    #[test]
    fn a_zone_whose_log_firewall4_cannot_read_is_not_logging() {
        for value in ["on", "yes", "true", "maybe", "1e2"] {
            let snapshot = Snapshot::from_value(json!({
                "firewall": { "cfg01": { ".type": "zone", ".name": "cfg01", "name": "lan", "log": value } },
                "network": {}
            }));
            assert!(
                !logging_configured(&snapshot),
                "a zone with log={value} records nothing"
            );
            assert_eq!(
                fixture::widget(&body(&snapshot), "empty")["title"],
                "Nothing is being logged"
            );
        }
    }
}
