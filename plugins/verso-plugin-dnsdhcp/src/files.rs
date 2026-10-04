// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
//! dnsmasq's option files, listed and edited as every set of hand-edited files
//! is (verso_plugin::files): the page names where they live and what they are
//! called, the SDK draws them.
use verso_plugin::files::{self, FileSet};
use verso_plugin::{Envelope, Form, Grid, Request, Tone, Value, Widget};

const ROOT: &str = "/plugins/dnsdhcp/dns";

/// PREFIX is the sub-path a file's editor lives under: on the DNS page, whose
/// custom options they are.
pub const PREFIX: &str = "dns/files/";

const SET: FileSet = FileSet {
    page: ROOT,
    editors: "/plugins/dnsdhcp/dns/files/",
    dir: "/etc/dnsmasq.d/",
    suffix: ".conf",
    main: Some("/etc/dnsmasq.conf"),
    line: ("option", "options"),
    empty: "No custom option files",
    too_large: "Custom option files must be at most 32 KiB of text.",
    placeholder: "10-local",
};

fn listed(r: &Request) -> Vec<Value> {
    files::files(r.ubus.get("dnsState"))
}

/// name is the file an editor's address names, after its prefix.
fn name(r: &Request) -> Option<&str> {
    r.path.trim_matches('/').strip_prefix(PREFIX)
}

pub fn listing(r: &Request) -> Widget {
    SET.listing(&listed(r))
}

/// with_editor is the DNS page with a file's editor open over it.
fn with_editor(r: &Request, editor: Widget) -> Envelope {
    let mut result = crate::dns::page(r);
    if let Widget::Grid(Grid { children, .. }) = &mut result.widget {
        children.push(editor);
    }
    result.with_back("Cancel", ROOT)
}

pub fn get(r: &Request) -> Envelope {
    let Some(name) = name(r) else {
        return crate::dns::page(r);
    };
    match SET.open(&listed(r), name) {
        Ok(editor) => with_editor(r, editor),
        Err(why) => crate::dns::page(r).with_notice(Tone::Danger, &why),
    }
}

pub fn post(r: &Request, f: &Form) -> Envelope {
    let Some((editor, command)) = name(r).and_then(|name| SET.save(&listed(r), name, f)) else {
        return crate::dns::page(r);
    };
    let mut result = with_editor(r, editor);
    result.commands.extend(command);
    result
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::fixture;

    // With no files, the band still stands — its name, its count, its add —
    // and the table says it has nothing under it in its own words, rather than
    // a made-up row in the path column.
    #[test]
    fn with_no_files_the_band_stands_alone_and_the_table_says_so() {
        let r = Request {
            path: "/".into(),
            query: Form::default(),
            snapshot: fixture::snapshot(),
            ubus: fixture::ubus(),
        };
        let body = serde_json::to_value(listing(&r)).expect("serialize");
        let rows = body["rows"].as_array().expect("rows");
        assert_eq!(rows.len(), 1);
        assert_eq!(rows[0]["group"]["label"], "Files");
        assert_eq!(rows[0]["group"]["chain"], "0");
        assert!(rows[0]["cells"].as_array().is_none_or(Vec::is_empty));
        assert_eq!(body["empty_text"], "No custom option files");
    }
}
