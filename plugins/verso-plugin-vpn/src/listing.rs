// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! Every tunnel on the router, whatever runs it, one row apiece: what it is,
//! the server it reaches, the address it was given, whether it is up, and how
//! much has crossed it. An OpenVPN instance opens to its own panel; a tunnel
//! another program keeps (Tailscale) is listed as it stands.

use verso_plugin::{
    commit, json, ColumnWidth, CommitOp, Envelope, Form, HeadingAct, RowDrawer, Table, TableCell,
    TableChip, TableColumn, TableRow, TableRowAct, Widget,
};

use crate::add;
use crate::model::{Instance, Traffic, Tunnel, Vpn, CONFIG};

pub const HEADING: &str = "VPN";
const DASH: &str = "—";

/// FIRST_TITLE and FIRST_BODY are the page before any tunnel: what goes here
/// and why, then the way in, which offers both ways of setting one up.
const FIRST_TITLE: &str = "No VPN yet";
const FIRST_BODY: &str = "Send the devices on your network through a VPN provider or a server of \
     your own. Drop in the profile your provider offers for routers, or enter the server's details \
     yourself.";
const ADD_LABEL: &str = add::TITLE;

/// UNREAD_TITLE and UNREAD_BODY are the page when the router's tunnels could
/// not be read, which is not the same as there being none.
const UNREAD_TITLE: &str = "This router’s tunnels can’t be read";
const UNREAD_BODY: &str = "Sign out and back in: a VPN package installed or updated since you \
     signed in is allowed in at your next sign-in.";

/// OPEN is the query key naming the instance whose panel is open.
pub const OPEN: &str = "open";

pub fn href(name: &str) -> String {
    format!("/plugins/vpn/?{OPEN}={name}")
}

/// page is the listing, with one instance's panel in front of it when
/// `drawer` carries one, or the import panel when it is the new one's. A
/// router with no tunnels opens on how to make the first; one whose tunnels
/// could not be read says so rather than claiming none.
pub fn page(vpn: &Vpn, drawer: Option<(&str, RowDrawer)>) -> Envelope {
    let (importing, mut drawer) = match drawer {
        Some((name, panel)) if name == add::NEW => (Some(panel), None),
        other => (None, other),
    };
    if !vpn.known {
        return Envelope::page(
            HEADING,
            Widget::empty("lock", UNREAD_TITLE, UNREAD_BODY, Vec::new()),
        )
        .with_width("narrow")
        .with_tone("neutral");
    }
    // The empty page says what goes here and why; the way in is the page's
    // own act on the heading, as over a listing, which opens its panel in
    // place rather than loading a page that then opens it.
    if vpn.instances.is_empty() && vpn.others.is_empty() {
        return Envelope::page(
            HEADING,
            Widget::empty("lock", FIRST_TITLE, FIRST_BODY, Vec::new()),
        )
        .with_width("narrow")
        .with_tone("neutral")
        .with_act(act(importing));
    }
    let mut rows: Vec<TableRow> = Vec::new();
    for instance in &vpn.instances {
        let open = match &drawer {
            Some((name, _)) if *name == instance.name => drawer.take().map(|(_, d)| d),
            _ => None,
        };
        rows.push(instance_row(instance, open));
    }
    rows.extend(vpn.others.iter().map(tunnel_row));
    Envelope::page(
        HEADING,
        Widget::Table(Table {
            dense: true,
            columns: columns(),
            rows,
            ..Default::default()
        }),
    )
    .with_width("wide")
    .with_tone("neutral")
    .with_act(act(importing))
}

/// act is the page's one act: a new tunnel, which opens its panel over the
/// page rather than a page of its own. It makes a new thing, so it wears the
/// plus every add does.
fn act(panel: Option<RowDrawer>) -> HeadingAct {
    HeadingAct {
        label: ADD_LABEL.into(),
        href: add::href(),
        opens_panel: true,
        drawer: panel,
        ..Default::default()
    }
}

fn columns() -> Vec<TableColumn> {
    [
        ("Name", "name", ColumnWidth::Name),
        ("Kind", "text", ColumnWidth::Name),
        ("Server", "mono", ColumnWidth::Address),
        ("Address", "mono", ColumnWidth::Address),
        ("State", "status", ColumnWidth::Word),
        ("Traffic", "runtime", ColumnWidth::Grow),
        ("", "actions", ColumnWidth::Short),
    ]
    .into_iter()
    .map(|(label, kind, width)| TableColumn {
        label: label.into(),
        kind: kind.into(),
        width,
    })
    .collect()
}

fn instance_row(instance: &Instance, drawer: Option<RowDrawer>) -> TableRow {
    let door = href(&instance.name);
    let (state, variant) = instance_state(instance);
    // A profile not yet on disk (imported, waiting on the stage) is read from
    // the staged copy instead.
    let staged = instance.file.as_ref().map(|f| add::read(&f.content));
    let kind = match (&instance.profile, &staged) {
        (Some(p), _) if p.client => "OpenVPN client",
        (Some(_), _) => "OpenVPN server",
        (None, Some(r)) if r.client => "OpenVPN client",
        (None, Some(_)) => "OpenVPN server",
        (None, None) => "OpenVPN",
    };
    // The server it reaches now, or else the first its profile would try.
    let server = match instance.live.server.as_str() {
        "" => instance
            .profile
            .as_ref()
            .and_then(|p| p.remotes.first())
            .map(|r| r.endpoint())
            .or_else(|| staged.as_ref().and_then(|r| r.remotes.first().cloned()))
            .unwrap_or_default(),
        s => s.to_string(),
    };
    let (power, title, value) = match instance.enabled {
        true => ("power", "Stop", "off"),
        false => ("power-off", "Start", "on"),
    };
    TableRow {
        id: instance.name.clone(),
        muted: !instance.enabled,
        cells: vec![
            TableCell {
                text: instance.name.clone(),
                href: door.clone(),
                // A network named as the tunnel is says nothing the name
                // does not; one named otherwise is cited.
                chips: match instance.network == instance.name {
                    true => Vec::new(),
                    false => network_chip(&instance.network),
                },
                ..Default::default()
            },
            text_cell(kind),
            mono_cell(&server),
            mono_cell(&instance.live.address),
            TableCell {
                text: state.into(),
                variant: variant.into(),
                ..Default::default()
            },
            traffic_cell(instance.traffic.as_ref()),
            TableCell {
                actions: vec![
                    TableRowAct {
                        icon: power.into(),
                        title: title.into(),
                        name: instance.name.clone(),
                        value: value.into(),
                        ..Default::default()
                    },
                    TableRowAct {
                        icon: "square-pen".into(),
                        title: "Edit".into(),
                        href: door.clone(),
                        ..Default::default()
                    },
                ],
                ..Default::default()
            },
        ],
        drawer,
        panel: door,
        ..Default::default()
    }
}

/// instance_state is the instance's state in a word and its tone. Switched
/// off is the resting state; switched on and not running is a failure.
pub fn instance_state(instance: &Instance) -> (&'static str, &'static str) {
    if !instance.enabled {
        return ("off", "");
    }
    match instance.live.state.as_str() {
        "connected" => ("connected", "success"),
        "connecting" => ("connecting", "warning"),
        "auth-failed" => ("sign-in refused", "danger"),
        "stopped" => ("not running", "danger"),
        "pending" => ("not applied yet", ""),
        _ => ("unknown", ""),
    }
}

fn tunnel_row(tunnel: &Tunnel) -> TableRow {
    let kind = match tunnel.kind.as_str() {
        "tailscale" => "Tailscale",
        "wireguard" => "WireGuard",
        _ => "Tunnel",
    };
    let (state, variant) = match tunnel.up {
        true => ("up", "success"),
        false => ("down", ""),
    };
    TableRow {
        id: tunnel.device.clone(),
        cells: vec![
            text_cell(&tunnel.device),
            text_cell(kind),
            mono_cell(""),
            mono_cell(&tunnel.address),
            TableCell {
                text: state.into(),
                variant: variant.into(),
                ..Default::default()
            },
            traffic_cell(Some(&tunnel.traffic)),
            TableCell::default(),
        ],
        ..Default::default()
    }
}

fn network_chip(network: &str) -> Vec<TableChip> {
    if network.is_empty() {
        return Vec::new();
    }
    vec![TableChip {
        icon: "network".into(),
        label: network.into(),
        ..Default::default()
    }]
}

fn text_cell(text: &str) -> TableCell {
    TableCell {
        text: text.into(),
        ..Default::default()
    }
}

fn mono_cell(text: &str) -> TableCell {
    if text.is_empty() {
        return TableCell {
            text: DASH.into(),
            muted: true,
            ..Default::default()
        };
    }
    TableCell {
        text: text.into(),
        emphasis: true,
        ..Default::default()
    }
}

fn traffic_cell(traffic: Option<&Traffic>) -> TableCell {
    match traffic {
        Some(t) => text_cell(&format!("↓ {} · ↑ {}", bytes(t.rx), bytes(t.tx))),
        None => TableCell::default(),
    }
}

/// bytes is a counter the way a person reads it: one decimal from a megabyte
/// up, binary units, as the rest of the shell counts.
pub fn bytes(n: u64) -> String {
    const UNITS: [&str; 5] = ["B", "KB", "MB", "GB", "TB"];
    let mut value = n as f64;
    let mut unit = 0;
    while value >= 1024.0 && unit < UNITS.len() - 1 {
        value /= 1024.0;
        unit += 1;
    }
    match unit {
        0 | 1 => format!("{} {}", value.round() as u64, UNITS[unit]),
        _ => format!("{value:.1} {}", UNITS[unit]),
    }
}

/// switch finds the one start or stop a submission carries and states it as
/// the write the shell stages. OpenVPN runs an instance only when it says
/// `enabled 1`, so both ways are written out.
pub fn switch(vpn: &Vpn, form: &Form) -> Option<CommitOp> {
    let mut named = vpn
        .instances
        .iter()
        .filter_map(|i| match form.get(&i.name).as_str() {
            "on" => Some(commit(CONFIG, &i.name, json!({"enabled": "1"}))),
            "off" => Some(commit(CONFIG, &i.name, json!({"enabled": "0"}))),
            _ => None,
        });
    let op = named.next()?;
    named.next().is_none().then_some(op)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::fixture;

    fn body() -> serde_json::Value {
        let vpn = Vpn::read(&fixture::request("/"));
        serde_json::to_value(page(&vpn, None)).unwrap()
    }

    fn rows(body: &serde_json::Value) -> Vec<serde_json::Value> {
        body["widget"]["rows"].as_array().unwrap().clone()
    }

    #[test]
    fn every_tunnel_is_one_row_whatever_runs_it() {
        let body = body();
        let names: Vec<String> = rows(&body)
            .iter()
            .map(|r| r["cells"][0]["text"].as_str().unwrap().to_string())
            .collect();
        // The package's own samples stay out while they are off; tun0 is
        // proton's, so it is not listed twice.
        assert_eq!(names, ["proton", "work", "tailscale0"]);
        assert_eq!(body["title"], "VPN");
    }

    #[test]
    fn a_connected_instance_reads_out_on_one_line_per_cell() {
        let proton = &rows(&body())[0];
        let cells = &proton["cells"];
        assert_eq!(cells[0]["chips"][0]["label"], "vpn");
        assert_eq!(cells[1]["text"], "OpenVPN client");
        assert_eq!(cells[2]["text"], "185.107.56.234:1194");
        assert_eq!(cells[3]["text"], "10.96.0.14/16");
        assert_eq!(cells[4], json!({"text": "connected", "variant": "success"}));
        assert_eq!(cells[5]["text"], "↓ 1.2 GB · ↑ 182.0 MB");
        assert_eq!(proton["panel"], "/plugins/vpn/?open=proton");
        for cell in cells.as_array().unwrap() {
            assert!(
                cell.get("sub").is_none() && cell.get("detail").is_none(),
                "{cell}"
            );
        }
    }

    #[test]
    fn a_stopped_instance_names_the_server_it_would_try_and_offers_to_start() {
        let work = &rows(&body())[1];
        assert_eq!(work["muted"], true);
        assert_eq!(work["cells"][2]["text"], "vpn.example.com:443");
        assert_eq!(work["cells"][4]["text"], "off");
        let power = &work["cells"][6]["actions"][0];
        assert_eq!(power["name"], "work");
        assert_eq!(power["value"], "on");
    }

    #[test]
    fn a_tunnel_another_program_keeps_is_listed_as_it_stands() {
        let tailscale = &rows(&body())[2];
        assert_eq!(tailscale["cells"][1]["text"], "Tailscale");
        assert_eq!(tailscale["cells"][3]["text"], "100.101.12.7/32");
        assert!(tailscale.get("panel").is_none());
        assert!(tailscale["cells"][6].get("actions").is_none());
    }

    #[test]
    fn an_instance_switched_on_that_does_not_run_says_so() {
        let mut vpn = Vpn::read(&fixture::request("/"));
        vpn.instances[0].live.state = "stopped".into();
        assert_eq!(instance_state(&vpn.instances[0]), ("not running", "danger"));
        vpn.instances[0].live.state = "auth-failed".into();
        assert_eq!(
            instance_state(&vpn.instances[0]),
            ("sign-in refused", "danger")
        );
        vpn.instances[0].live.state = String::new();
        assert_eq!(instance_state(&vpn.instances[0]), ("unknown", ""));
    }

    #[test]
    fn a_router_with_no_tunnels_opens_on_how_to_make_the_first() {
        let mut request = fixture::empty("/");
        request.ubus =
            verso_plugin::Ubus::from_value(json!({"vpnState": {"instances": {}, "tunnels": []}}));
        let body = serde_json::to_value(page(&Vpn::read(&request), None)).unwrap();
        assert_eq!(body["widget"]["type"], "empty");
        assert_eq!(body["widget"]["title"], FIRST_TITLE);
        // One way in, the page's own act, which opens its panel in place: no
        // link in the empty state reloads the page to open it.
        assert_eq!(body["widget"]["children"], json!([]), "{body}");
        assert_eq!(body["act"]["href"], "/plugins/vpn/?open=new");
        assert_eq!(body["act"]["opens_panel"], true);
        // Asked for, the import panel opens over the empty page.
        let panel = add::drawer(
            &Vpn::read(&request),
            &add::Draft::blank(),
            &Default::default(),
        );
        let opened =
            serde_json::to_value(page(&Vpn::read(&request), Some((add::NEW, panel)))).unwrap();
        assert_eq!(opened["act"]["drawer"]["open"], true);
    }

    #[test]
    fn an_import_waiting_on_the_stage_says_so() {
        let mut request = fixture::empty("/");
        request.snapshot = verso_plugin::Snapshot::from_value(json!({"openvpn": {
            "fresh": {".type": "openvpn", ".name": "fresh", ".index": 0, "enabled": "1",
                "config": "/etc/openvpn/fresh.ovpn"}
        }}));
        request.ubus =
            verso_plugin::Ubus::from_value(json!({"vpnState": {"instances": {}, "tunnels": []}}));
        let vpn = Vpn::read(&request);
        assert_eq!(instance_state(&vpn.instances[0]), ("not applied yet", ""));
    }

    #[test]
    fn tunnels_that_cannot_be_read_are_not_called_none() {
        let body = serde_json::to_value(page(&Vpn::read(&fixture::empty("/")), None)).unwrap();
        assert_eq!(body["widget"]["title"], UNREAD_TITLE);
        assert!(body.to_string().contains("Sign out and back in"));
    }

    #[test]
    fn a_listing_offers_another_import_from_its_heading() {
        let body = body();
        assert_eq!(body["act"]["label"], ADD_LABEL);
        assert_eq!(body["act"]["href"], "/plugins/vpn/?open=new");
        assert_eq!(body["act"]["opens_panel"], true);
    }

    #[test]
    fn start_and_stop_write_enabled_both_ways() {
        let vpn = Vpn::read(&fixture::request("/"));
        let op = switch(&vpn, &Form::parse("work=on")).unwrap();
        assert_eq!(
            serde_json::to_value(op).unwrap()["values"],
            json!({"enabled": "1"})
        );
        let op = switch(&vpn, &Form::parse("proton=off")).unwrap();
        assert_eq!(
            serde_json::to_value(op).unwrap()["values"],
            json!({"enabled": "0"})
        );
        assert!(switch(&vpn, &Form::parse("proton=off&work=on")).is_none());
        assert!(switch(&vpn, &Form::parse("nobody=on")).is_none());
    }

    #[test]
    fn counters_read_the_way_a_person_says_them() {
        assert_eq!(bytes(512), "512 B");
        assert_eq!(bytes(2048), "2 KB");
        assert_eq!(bytes(1331439862), "1.2 GB");
    }
}
