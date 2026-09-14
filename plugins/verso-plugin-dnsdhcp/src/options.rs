// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! The option catalogue: what each settings row is, in one declaration that both
//! draws it and reads it back.
//!
//! Most of this config is options on a section, and every page here is mostly
//! rows of them. Declaring a row once — its plain name, what it does, the option
//! behind it, and what kind of control it is — is what keeps the page and the
//! save from drifting: a control the page did not draw cannot be read back, and
//! a value the save does not understand cannot be drawn.
//!
//! Three rules the rows encode. A switch states what the daemon does when the
//! option is **absent**, because that is the state the operator is looking at
//! before they touch anything. A row may be the **inverse** of its option —
//! "Hand out addresses" is `ignore`, turned around, because nobody thinks in
//! negatives. And a save writes only what **changed**: an option the operator
//! did not touch keeps whatever it was, and one they emptied is cleared rather
//! than written blank, since an empty option is a value to a daemon.

use std::collections::BTreeMap;

use verso_plugin::{
    commit, commit_new, CommitOp, Form, Map, SettingsItem, SettingsSeam, SettingsToggle, Value,
};

use crate::form;
use crate::model::Options;

/// Check is what a value has to look like for the daemon to read it.
#[derive(Clone, Copy)]
pub enum Check {
    /// A whole number no larger than the bound.
    Number(u32),
    /// A lease length: seconds, a count with a unit, or forever.
    Leasetime,
    /// A name this router may answer under.
    Hostname,
}

impl Check {
    fn accepts(self, value: &str) -> bool {
        match self {
            Check::Number(max) => form::valid_number(value, max),
            Check::Leasetime => form::valid_leasetime(value),
            Check::Hostname => form::valid_hostname(value),
        }
    }

    fn refusal(self) -> &'static str {
        match self {
            Check::Number(max) => match max {
                65535 => "has to be a whole number from 0 to 65535",
                _ => "has to be a whole number",
            },
            Check::Leasetime => "has to be a length such as 12h, 30m, or infinite",
            Check::Hostname => "has to be a name, such as lan or corp.example.com",
        }
    }
}

/// Control is how a row carries its option's state.
#[derive(Clone, Copy)]
pub enum Control {
    /// A switch. On_when_absent is what the daemon does without the option;
    /// Invert makes the row the opposite of the option, for an option written as
    /// a refusal.
    Toggle { on_when_absent: bool, invert: bool },
    /// A value edited in place.
    Value(Check),
    /// A value the page states but does not edit: visible truth, not editable
    /// surface. A list is shown the way it reads, not the way uci stores it.
    Read,
}

/// Row is one option row: its plain name, what it does, the option behind it,
/// and its control. The option name is also the form name it posts under, so the
/// catalogue is the contract in both directions. Code overrides the chip for the
/// rare row that moves more than one option together; "" names the option.
pub struct Row {
    pub option: &'static str,
    pub code: &'static str,
    pub title: &'static str,
    pub desc: &'static str,
    pub control: Control,
}

/// on is a switch that is off until the config says otherwise.
pub const fn on(option: &'static str, title: &'static str, desc: &'static str) -> Row {
    toggle(option, title, desc, false, false)
}

/// on_by_default is a switch the daemon already has on with no option written.
pub const fn on_by_default(option: &'static str, title: &'static str, desc: &'static str) -> Row {
    toggle(option, title, desc, true, false)
}

/// inverted is a switch that reads as the opposite of its option.
pub const fn inverted(option: &'static str, title: &'static str, desc: &'static str) -> Row {
    toggle(option, title, desc, false, true)
}

const fn toggle(
    option: &'static str,
    title: &'static str,
    desc: &'static str,
    on_when_absent: bool,
    invert: bool,
) -> Row {
    Row {
        option,
        code: "",
        title,
        desc,
        control: Control::Toggle {
            on_when_absent,
            invert,
        },
    }
}

/// value is a row edited in place.
pub const fn value(
    option: &'static str,
    title: &'static str,
    desc: &'static str,
    check: Check,
) -> Row {
    paired(option, "", title, desc, check)
}

/// paired is a value row whose option does not stand alone: the chip names every
/// option the row governs, because the row moves them together.
pub const fn paired(
    option: &'static str,
    code: &'static str,
    title: &'static str,
    desc: &'static str,
    check: Check,
) -> Row {
    Row {
        option,
        code,
        title,
        desc,
        control: Control::Value(check),
    }
}

/// read is a row the page states and does not edit.
pub const fn read(option: &'static str, title: &'static str, desc: &'static str) -> Row {
    Row {
        option,
        code: "",
        title,
        desc,
        control: Control::Read,
    }
}

/// items renders a run of rows from one section's values. Prefix namespaces the
/// form names, so the same catalogue draws one network's card without colliding
/// with the next one's.
pub fn items(rows: &[Row], options: &Options, prefix: &str) -> Vec<SettingsItem> {
    rows.iter().map(|row| item(row, options, prefix)).collect()
}

fn item(row: &Row, options: &Options, prefix: &str) -> SettingsItem {
    let mut item = SettingsItem {
        title: row.title.into(),
        desc: row.desc.into(),
        code: match row.code {
            "" => row.option.into(),
            code => code.into(),
        },
        ..SettingsItem::default()
    };
    match row.control {
        Control::Toggle {
            on_when_absent,
            invert,
        } => {
            let state = options.flag(row.option, on_when_absent);
            item.toggle = Some(SettingsToggle {
                name: format!("{prefix}{}", row.option),
                on: state != invert,
            });
        }
        Control::Value(_) => {
            item.value = options.scalar(row.option).into();
            item.name = format!("{prefix}{}", row.option);
        }
        Control::Read => item.value = options.list(row.option).join(", "),
    }
    item
}

/// fold puts a run of rows behind a collapsed line that counts them, so a
/// block's long tail is present and honest without carrying the block.
pub fn fold(lead: &str, items: Vec<SettingsItem>) -> Option<SettingsSeam> {
    if items.is_empty() {
        return None;
    }
    let count = items.len();
    let noun = match count {
        1 => "option",
        _ => "options",
    };
    Some(SettingsSeam {
        summary: format!("{lead}{count} more {noun}"),
        items,
    })
}

/// Changes is what one page form's submission amounts to: the options whose
/// value the operator actually changed, by section, and the values they typed
/// that no daemon would read.
#[derive(Default)]
pub struct Changes {
    edits: BTreeMap<String, Edit>,
    refusals: Vec<String>,
}

/// Edit is one section's pending writes. Type carries the section's uci type for
/// a config that does not have the section yet — writing one of its options is
/// what creates it.
#[derive(Default)]
struct Edit {
    section_type: String,
    values: Map<String, Value>,
}

impl Changes {
    /// write records one option's new value; a JSON null clears it. A page whose
    /// rows do not stand alone — an option that has to move with another — states
    /// the companion here.
    pub fn write(&mut self, section: &str, section_type: &str, option: &str, new: Value) {
        let edit = self.edits.entry(section.to_string()).or_default();
        edit.section_type = section_type.to_string();
        edit.values.insert(option.to_string(), new);
    }

    /// refuse records a value the daemon would not read, named the way the row
    /// names it, so the notice says which row to look at.
    fn refuse(&mut self, title: &str, complaint: &str) {
        self.refusals.push(format!("“{title}” {complaint}"));
    }

    /// refusal is the first thing wrong with the submission, or None. A refusal
    /// stops the whole save: a page form is one Save, and half of one is worse
    /// than none.
    pub fn refusal(&self) -> Option<&str> {
        self.refusals.first().map(String::as_str)
    }

    /// is_empty reports that the submission changed nothing.
    pub fn is_empty(&self) -> bool {
        self.edits.is_empty()
    }

    /// commit states the writes: one operation per section, creating the section
    /// when the config does not have it yet.
    pub fn commit(&self, config: &str) -> Vec<CommitOp> {
        self.edits
            .iter()
            .map(|(section, edit)| {
                let values = Value::Object(edit.values.clone());
                match section.is_empty() {
                    true => commit_new(config, &edit.section_type, values),
                    false => commit(config, section, values),
                }
            })
            .collect()
    }
}

/// save reads a run of rows back off a submission: what changed is recorded and
/// reflected in the model the answer renders from, what the daemon would not
/// read is refused, and what the operator did not touch is left alone.
pub fn save(
    rows: &[Row],
    options: &mut Options,
    section_type: &str,
    prefix: &str,
    form: &Form,
    changes: &mut Changes,
) {
    let section = options.section.clone();
    for row in rows {
        let name = format!("{prefix}{}", row.option);
        match row.control {
            Control::Toggle {
                on_when_absent,
                invert,
            } => {
                // A switch inside a form posts only while it is on, so an absent
                // field is the operator having turned it off.
                let shown = !form.get(&name).is_empty();
                let wanted = shown != invert;
                if wanted == options.flag(row.option, on_when_absent) {
                    continue;
                }
                let written = match wanted {
                    true => "1",
                    false => "0",
                };
                options.set(row.option, Some(written));
                changes.write(&section, section_type, row.option, Value::from(written));
            }
            Control::Value(check) => {
                let submitted = form.get(&name).trim().to_string();
                if submitted == options.scalar(row.option) {
                    continue;
                }
                // The typed value comes back either way, so a refusal never
                // costs the operator what they wrote.
                if submitted.is_empty() {
                    options.set(row.option, None);
                    changes.write(&section, section_type, row.option, Value::Null);
                    continue;
                }
                options.set(row.option, Some(&submitted));
                if !check.accepts(&submitted) {
                    changes.refuse(row.title, check.refusal());
                    continue;
                }
                changes.write(&section, section_type, row.option, Value::from(submitted));
            }
            Control::Read => {}
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use verso_plugin::Snapshot;

    const ROWS: [Row; 4] = [
        inverted(
            "ignore",
            "Hand out addresses",
            "This router assigns addresses.",
        ),
        on_by_default("ra_slaac", "SLAAC", "Devices derive their own address."),
        value(
            "leasetime",
            "Lease length",
            "How long an address stays.",
            Check::Leasetime,
        ),
        read(
            "dhcp_option",
            "Extra DHCP options",
            "Options handed to clients.",
        ),
    ];

    fn pool() -> Options {
        let snapshot = Snapshot::from_value(serde_json::json!({
            "dhcp": {"lan": {
                ".type": "dhcp", ".name": "lan",
                "interface": "lan", "leasetime": "12h", "dhcp_option": ["6,10.0.0.30", "3,10.0.0.1"]
            }}
        }));
        Options::read(&snapshot.section("dhcp", "lan").expect("section"))
    }

    fn rendered() -> Vec<SettingsItem> {
        items(&ROWS, &pool(), "lan.")
    }

    #[test]
    fn a_row_draws_the_control_its_option_needs() {
        let items = rendered();
        let json = serde_json::to_value(&items).expect("serialize");
        // An inverted switch reads as the opposite of the option behind it, and
        // an absent `ignore` means this network does hand out addresses.
        assert_eq!(
            json[0],
            serde_json::json!({
                "title": "Hand out addresses",
                "desc": "This router assigns addresses.",
                "code": "ignore",
                "toggle": {"name": "lan.ignore", "on": true}
            })
        );
        // A switch the daemon already has on states so without an option.
        assert_eq!(
            json[1]["toggle"],
            serde_json::json!({"name": "lan.ra_slaac", "on": true})
        );
        // A value row is edited in place, under its own name.
        assert_eq!(json[2]["value"], "12h");
        assert_eq!(json[2]["name"], "lan.leasetime");
        // A read row states the option and offers no control.
        assert_eq!(json[3]["value"], "6,10.0.0.30, 3,10.0.0.1");
        assert!(json[3].get("name").is_none());
        assert!(json[3].get("toggle").is_none());
    }

    #[test]
    fn a_fold_counts_what_it_hides() {
        assert!(fold("", Vec::new()).is_none());
        let one = fold("", items(&ROWS[..1], &pool(), "")).expect("seam");
        assert_eq!(one.summary, "1 more option");
        let many = fold("IPv6 & advanced — ", rendered()).expect("seam");
        assert_eq!(many.summary, "IPv6 & advanced — 4 more options");
    }

    fn saved(body: &str) -> (Options, Changes) {
        let mut options = pool();
        let mut changes = Changes::default();
        save(
            &ROWS,
            &mut options,
            "dhcp",
            "lan.",
            &Form::parse(body),
            &mut changes,
        );
        (options, changes)
    }

    #[test]
    fn only_what_changed_is_written() {
        // Everything as it stands: handout on, slaac on, the same lease time.
        let (_, unchanged) = saved("lan.ignore=1&lan.ra_slaac=1&lan.leasetime=12h");
        assert!(unchanged.is_empty());
        assert!(unchanged.commit("dhcp").is_empty());

        let (options, changes) = saved("lan.ra_slaac=1&lan.leasetime=2h");
        assert_eq!(
            serde_json::to_value(changes.commit("dhcp")).expect("serialize"),
            serde_json::json!([{
                "config": "dhcp",
                "section": "lan",
                // The switch that stopped posting turned the option it inverts on.
                "values": {"ignore": "1", "leasetime": "2h"}
            }])
        );
        // The answer renders from the model, so it carries the accepted change.
        assert!(options.flag("ignore", false));
        assert_eq!(options.scalar("leasetime"), "2h");
    }

    #[test]
    fn an_emptied_value_is_cleared_rather_than_written_blank() {
        let (options, changes) = saved("lan.ignore=1&lan.ra_slaac=1&lan.leasetime=");
        assert_eq!(
            serde_json::to_value(changes.commit("dhcp")).expect("serialize")[0]["values"],
            serde_json::json!({"leasetime": null})
        );
        assert_eq!(options.scalar("leasetime"), "");
    }

    #[test]
    fn a_value_the_daemon_would_not_read_stops_the_whole_save() {
        let (options, changes) = saved("lan.ignore=1&lan.ra_slaac=1&lan.leasetime=forever");
        assert_eq!(
            changes.refusal(),
            Some("“Lease length” has to be a length such as 12h, 30m, or infinite")
        );
        // What was typed comes back, so nothing an operator wrote is lost.
        assert_eq!(options.scalar("leasetime"), "forever");
    }

    #[test]
    fn a_config_without_the_section_yet_creates_it() {
        let mut options = Options::default();
        let mut changes = Changes::default();
        save(
            &ROWS[..1],
            &mut options,
            "dhcp",
            "",
            &Form::parse(""),
            &mut changes,
        );
        assert_eq!(
            serde_json::to_value(changes.commit("dhcp")).expect("serialize"),
            serde_json::json!([{
                "config": "dhcp", "section": "", "type": "dhcp", "values": {"ignore": "1"}
            }])
        );
    }
}
