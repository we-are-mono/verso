// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
use crate::page;
use verso_plugin::{
    ApplyAction, Envelope, Form, Request, RowDrawer, TableCell, TableGroup, TableRow, TableRowAct,
    Value, Widget,
};
const ROOT: &str = "/plugins/dnsdhcp/";
fn files(r: &Request) -> Vec<Value> {
    r.ubus
        .get("dnsState")
        .and_then(|s| s.get("files"))
        .and_then(Value::as_array)
        .cloned()
        .unwrap_or_default()
}
fn text<'a>(f: &'a Value, key: &str) -> &'a str {
    f.get(key).and_then(Value::as_str).unwrap_or("")
}
fn href(path: &str) -> String {
    format!(
        "{ROOT}files/{}",
        if path == "/etc/dnsmasq.conf" {
            "main"
        } else {
            path.strip_prefix("/etc/dnsmasq.d/").unwrap_or("")
        }
    )
}
pub fn listing(r: &Request) -> Widget {
    let files = files(r);
    let mut rows: Vec<_> = files
        .iter()
        .map(|f| {
            let path = text(f, "path");
            let count = text(f, "content")
                .lines()
                .filter(|l| !l.trim().is_empty() && !l.trim().starts_with('#'))
                .count();
            TableRow {
                id: path.into(),
                panel: href(path),
                cells: vec![
                    TableCell {
                        text: path.rsplit('/').next().unwrap_or(path).into(),
                        sub: path
                            .rsplit_once('/')
                            .map(|(d, _)| format!("{d}/"))
                            .unwrap_or_default(),
                        ..Default::default()
                    },
                    TableCell {
                        text: count.to_string(),
                        sub: if count == 1 { "option" } else { "options" }.into(),
                        ..Default::default()
                    },
                    TableCell {
                        actions: vec![TableRowAct {
                            title: "Edit".into(),
                            icon: "chevron-right".into(),
                            href: href(path),
                            ..Default::default()
                        }],
                        ..Default::default()
                    },
                ],
                ..Default::default()
            }
        })
        .collect();
    if rows.is_empty() {
        rows.push(TableRow {
            cells: vec![
                TableCell {
                    text: "No custom option files".into(),
                    ..Default::default()
                },
                TableCell::default(),
            ],
            ..Default::default()
        });
    }
    rows[0].group = Some(TableGroup {
        key: String::new(),
        label: "Files".into(),
        to: String::new(),
        chain: files.len().to_string(),
        tally: String::new(),
        add_label: "New file".into(),
        add_href: format!("{ROOT}files/new"),
        add_panel: true,
    });
    let mut w = page::table(
        page::columns(&[("", "path"), ("", "count"), ("", "actions")]),
        rows,
        "No custom option files",
    );
    if let Widget::Table {
        drawer_icon,
        drawer_label,
        ..
    } = &mut w
    {
        *drawer_icon = "chevron-right".into();
        *drawer_label = "Edit".into();
    }
    w
}
fn path(r: &Request) -> Option<String> {
    let name = r.path.trim_matches('/').strip_prefix("files/")?;
    if name == "main" {
        Some("/etc/dnsmasq.conf".into())
    } else if name == "new" {
        Some(String::new())
    } else if name.strip_suffix(".conf").is_some_and(valid_name) {
        Some(format!("/etc/dnsmasq.d/{name}"))
    } else {
        None
    }
}
fn valid_name(s: &str) -> bool {
    !s.is_empty()
        && s.len() <= 64
        && s.as_bytes()[0].is_ascii_alphanumeric()
        && s.bytes()
            .all(|b| b.is_ascii_lowercase() || b.is_ascii_digit() || b == b'-' || b == b'_')
}
fn editor(
    r: &Request,
    path: &str,
    name: &str,
    content: &str,
    expected: &str,
    error: &str,
) -> Envelope {
    let mut fields = vec![Widget::hidden("expected", expected)];
    if path.is_empty() {
        let mut filename = Widget::field("filename", "File name", name, "", "");
        if let Widget::Field {
            unit, placeholder, ..
        } = &mut filename
        {
            *unit = ".conf".into();
            *placeholder = "10-local".into();
        }
        fields.push(filename);
    }
    let mut body = Widget::field("content", "Contents", content, "", "");
    if let Widget::Field { kind, style, .. } = &mut body {
        *kind = "textarea".into();
        *style = "code".into();
    }
    fields.push(body);
    fields.push(Widget::Text {
        markdown: if path.is_empty() {
            "`/etc/dnsmasq.d/`".into()
        } else {
            format!("`{path}`")
        },
    });
    let form = Widget::Form {
        style: "settings".into(),
        submit: "Save".into(),
        error: error.into(),
        fields,
        note: String::new(),
    };
    let drawer = RowDrawer {
        title: if path.is_empty() {
            "New file"
        } else {
            "Edit file"
        }
        .into(),
        open: true,
        closed: ROOT.into(),
        children: vec![form],
        size: "form".into(),
        ..Default::default()
    };
    let mut result = crate::settings::page(r);
    if let Widget::Grid { children, .. } = &mut result.widget {
        children.push(page::table(
            vec![],
            vec![TableRow {
                drawer: Some(drawer),
                ..Default::default()
            }],
            "",
        ));
    }
    result.with_back("Cancel", ROOT)
}
pub fn get(r: &Request) -> Envelope {
    let Some(path) = path(r) else {
        return crate::settings::page(r);
    };
    if path.is_empty() {
        return editor(r, "", "", "", "af63bd4c8601b7df", "");
    }
    let files = files(r);
    if let Some(file) = files.iter().find(|f| text(f, "path") == path) {
        if !text(file, "error").is_empty() {
            return crate::settings::page(r)
                .with_notice(verso_plugin::Tone::Danger, text(file, "error"));
        }
        editor(
            r,
            &path,
            "",
            text(file, "content"),
            text(file, "version"),
            "",
        )
    } else {
        crate::settings::page(r).with_notice(
            verso_plugin::Tone::Danger,
            "The file is no longer available.",
        )
    }
}
pub fn post(r: &Request, f: &Form) -> Envelope {
    let Some(original) = path(r) else {
        return crate::settings::page(r);
    };
    let name = f.get("filename").trim().to_owned();
    let content = f.get("content");
    let expected = f.get("expected");
    let target = if original.is_empty() {
        format!("/etc/dnsmasq.d/{name}.conf")
    } else {
        original.clone()
    };
    let error = if original.is_empty() && !valid_name(&name) {
        "Use lowercase letters, numbers, hyphens or underscores for the file name."
    } else if content.len() > 32768 || content.contains('\0') {
        "Custom option files must be at most 32 KiB of text."
    } else if original.is_empty() && files(r).iter().any(|file| text(file, "path") == target) {
        "A file with this name already exists."
    } else {
        ""
    };
    let mut result = editor(r, &original, &name, &content, &expected, error);
    if error.is_empty() {
        result.commands.push(ApplyAction {
            name: "config-file-stage".into(),
            args: [
                ("path".into(), target),
                ("expected".into(), expected),
                ("content".into(), content),
            ]
            .into(),
        });
    }
    result
}
