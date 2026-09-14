// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
use verso_plugin::{
    serve_described, Change, Description, Envelope, Form, Request, Snapshot, Tone, Widget,
};
mod editor;
mod model;
mod page;
use model::Model;
pub const ROOT: &str = "/plugins/interfaces/";
fn main() {
    serve_described("interfaces", get, post, describe);
}
fn get(r: &Request) -> Envelope {
    let m = Model::read(r);
    match r.path.trim_end_matches('/') {
        "" => page::listing(&m, r.query.get("new") == "1"),
        "/new" => editor::new(&m, &r.query.get("kind")),
        "/edit" => editor::edit(&m, &r.query.get("network"), &r.query.get("device")),
        "/delete" => editor::delete_page(&m, &r.query.get("network"), &r.query.get("device")),
        "/vpn" => {
            let body = Widget::stack(vec![
                Widget::text("WireGuard and OpenVPN are configured by their tunnel plugins. Install a compatible tunnel plugin from System → Packages."),
                Widget::link("Install plugins", "/system/packages/discover?q=verso-plugin", "button"),
            ]);
            Envelope::page("A VPN tunnel", body).with_back("Interfaces", ROOT)
        }
        _ => missing(),
    }
}
fn post(r: &Request, f: &Form) -> Envelope {
    let m = Model::read(r);
    match r.path.trim_end_matches('/') {
        "/new" => editor::save(&m, &r.query.get("kind"), "", "", f),
        "/edit" => editor::save(&m, "", &r.query.get("network"), &r.query.get("device"), f),
        "/delete" => editor::delete(&m, &r.query.get("network"), &r.query.get("device"), f),
        "" => editor::action(&m, f),
        _ => missing(),
    }
}
fn missing() -> Envelope {
    Envelope::page("Interfaces", Widget::form("", vec![]))
        .with_notice(
            Tone::Danger,
            "This interface no longer exists. Return to Interfaces and try again.",
        )
        .with_back("Interfaces", ROOT)
}
fn describe(changes: &[Change], _: &Snapshot) -> Vec<Description> {
    changes
        .iter()
        .enumerate()
        .filter(|(_, c)| c.config == "network")
        .map(|(i, c)| {
            Description::one(
                if c.op == "remove-section" {
                    "Removed interface"
                } else {
                    "Changed interface settings"
                },
                i,
            )
        })
        .collect()
}
#[cfg(test)]
mod tests;
