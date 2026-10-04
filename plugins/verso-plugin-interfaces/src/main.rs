// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
use verso_plugin::{
    serve_described, Change, Description, Envelope, Form, Request, Snapshot, Tone, Widget,
};
mod editor;
mod model;
mod page;
mod routes;
use model::Model;
pub const ROOT: &str = "/plugins/interfaces/";
fn main() {
    serve_described("interfaces", get, post, describe);
}
fn get(r: &Request) -> Envelope {
    let m = Model::read(r);
    match r.path.trim_end_matches('/') {
        "" => page::listing(&m),
        "/new" => editor::new(&m, &r.query.get("kind")),
        "/edit" => editor::edit(&m, &r.query.get("network"), &r.query.get("device")),
        "/delete" => editor::delete_page(&m, &r.query.get("network"), &r.query.get("device")),
        "/routes" => routes::page(&m, &r.query.get("open"), &r.query.get("family")),
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
        "/new" => editor::save(&m, &editor::new_kind(f, &r.query), "", "", f),
        "/edit" => editor::save(&m, "", &r.query.get("network"), &r.query.get("device"), f),
        "/delete" => editor::delete(&m, &r.query.get("network"), &r.query.get("device"), f),
        "/routes" => routes::post(&m, &r.query.get("open"), f),
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
// describe names a route by its target and any other network change as the
// interface settings it is. A removed section is gone from the staged
// snapshot, so nothing says whether it was an interface, a device or a route:
// its change keeps its raw line.
fn describe(changes: &[Change], s: &Snapshot) -> Vec<Description> {
    let mut out = routes::describe(changes, s);
    let covered: Vec<usize> = out.iter().flat_map(|d| d.covers.clone()).collect();
    out.extend(
        changes
            .iter()
            .enumerate()
            .filter(|(i, c)| {
                c.config == "network" && c.op != "remove-section" && !covered.contains(i)
            })
            .map(|(i, _)| Description::one("Changed interface settings", i)),
    );
    out
}
#[cfg(test)]
mod tests;
