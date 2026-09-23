// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
use std::collections::BTreeMap;
use std::mem::MaybeUninit;
use std::net::{IpAddr, Ipv6Addr};
use std::time::{SystemTime, UNIX_EPOCH};
use verso_plugin::{
    commit, commit_new, json, serve, ApplyAction, Envelope, Form, Request, SelectOption, Snapshot,
    Tone, Widget,
};
mod access;
mod credentials;
mod sshkey;
mod timezones;
use timezones::ZONES;
#[derive(Default)]
struct Facts {
    hostname: String,
    zonename: String,
    timezone: String,
    // Where the options live, by the snapshot's section names, so each control
    // can say it (Widget::at) and the shell can mark what waits on the stage.
    system_section: String,
    globals_section: String,
    ntp_section: String,
    ntp_enabled: bool,
    servers: Vec<String>,
    ula: String,
    steering: bool,
    serve: bool,
}
fn main() {
    serve("system", get, post);
}
fn get(r: &Request) -> Envelope {
    if r.path.starts_with("/access") {
        return access::get(r);
    }
    page(facts(&r.snapshot), &BTreeMap::new())
}
fn post(r: &Request, f: &Form) -> Envelope {
    if r.path.starts_with("/access") {
        return access::post(r, f);
    }
    let before = facts(&r.snapshot);
    let mut values = facts(&r.snapshot);
    let mut errors = BTreeMap::new();
    values.hostname = f.get("hostname").trim().into();
    values.ula = f.get("ula_prefix").trim().into();
    values.steering = f.get("packet_steering") == "1";
    values.zonename = f.get("zonename");
    values.ntp_enabled = f.get("ntp_enabled") == "1";
    values.serve = f.get("enable_server") == "1";
    let mut seen = std::collections::BTreeSet::new();
    values.servers = f
        .all("server")
        .into_iter()
        .map(|s| s.trim().to_string())
        .filter(|s| !s.is_empty() && seen.insert(s.clone()))
        .collect();
    if f.get("_action") == "clock" {
        let datetime = f.get("_client_time");
        if !valid_local_datetime(&datetime) {
            errors.insert(
                "clock".into(),
                "The computer's time could not be read. Try again.".into(),
            );
            return page(values, &errors);
        }
        let mut result = page(values, &errors).with_notice(Tone::Success, "Router time updated.");
        result.commands = vec![ApplyAction {
            name: "set-system-time".into(),
            args: BTreeMap::from([
                ("datetime".into(), datetime),
                ("timezone".into(), "UTC".into()),
            ]),
        }];
        return result;
    }
    if !valid_hostname(&values.hostname) {
        errors.insert(
            "hostname".into(),
            "Use 1–63 letters, numbers or hyphens, without a leading or trailing hyphen.".into(),
        );
    }
    if !values.ula.is_empty() && !valid_ula(&values.ula) {
        errors.insert(
            "ula_prefix".into(),
            "Enter a private IPv6 prefix, such as fd42:1:1::/48, or leave it empty.".into(),
        );
    }
    let timezone = ZONES
        .iter()
        .find(|(name, _)| *name == values.zonename)
        .map(|(_, tz)| tz.to_string())
        .or_else(|| {
            (values.zonename == before.zonename && valid_posix_tz(&before.timezone))
                .then_some(before.timezone)
        });
    if let Some(tz) = timezone {
        values.timezone = tz;
    } else {
        errors.insert(
            "zonename".into(),
            "Choose one of the available timezones.".into(),
        );
    }
    if values.servers.iter().any(|s| !valid_server(s)) {
        errors.insert(
            "server".into(),
            "Enter valid hostnames or IP addresses for time servers.".into(),
        );
    }
    if values.ntp_enabled && values.servers.is_empty() {
        errors.insert(
            "server".into(),
            "Add at least one time server, or turn off internet time synchronization.".into(),
        );
    }
    if !errors.is_empty() {
        return page(values, &errors);
    }
    let Some(system) = r
        .snapshot
        .sections_of_type("system", "system")
        .into_iter()
        .next()
    else {
        return page(
            values,
            &BTreeMap::from([(
                "form".into(),
                "The router's system configuration could not be read.".into(),
            )]),
        );
    };
    let mut operations = vec![commit(
        "system",
        &system.name(),
        json!({"hostname":values.hostname,"zonename":values.zonename,"timezone":values.timezone}),
    )];
    let globals = json!({"ula_prefix":if values.ula.is_empty(){serde_json::Value::Null}else{json!(values.ula)},"packet_steering":if values.steering{"1"}else{"0"}});
    if let Some(g) = r.snapshot.sections_of_type("network", "globals").first() {
        operations.push(commit("network", &g.name(), globals));
    } else {
        let mut op = commit_new("network", "globals", globals);
        op.section = "globals".into();
        operations.push(op);
    }
    let ntp = json!({"enabled":if values.ntp_enabled{"1"}else{"0"},"server":if values.servers.is_empty(){serde_json::Value::Null}else{json!(values.servers)},"enable_server":if values.serve{"1"}else{"0"}});
    operations.push(if values.ntp_section.is_empty() {
        commit_new("system", "timeserver", ntp)
    } else {
        commit("system", &values.ntp_section, ntp)
    });
    page(values, &errors)
        .with_commit(operations)
        .with_notice(Tone::Success, "General settings saved.")
}
fn facts(s: &Snapshot) -> Facts {
    let system = s.sections_of_type("system", "system");
    let system = system.first();
    let ntp = s.sections_of_type("system", "timeserver");
    let ntp = ntp.first();
    let globals = s.sections_of_type("network", "globals");
    let globals = globals.first();
    let timezone = system.map(|s| s.scalar("timezone")).unwrap_or_default();
    let mut zonename = system.map(|s| s.scalar("zonename")).unwrap_or_default();
    if zonename.is_empty() && matches!(timezone.as_str(), "UTC" | "GMT0" | "") {
        zonename = "UTC".into();
    }
    Facts {
        hostname: system.map(|s| s.scalar("hostname")).unwrap_or_default(),
        zonename,
        timezone,
        system_section: system.map(|s| s.name()).unwrap_or_default(),
        globals_section: globals.map(|s| s.name()).unwrap_or_default(),
        ntp_section: ntp.map(|s| s.name()).unwrap_or_default(),
        ntp_enabled: ntp.is_some_and(|s| s.scalar("enabled") != "0"),
        servers: ntp.map(|s| s.list("server")).unwrap_or_default(),
        ula: globals.map(|s| s.scalar("ula_prefix")).unwrap_or_default(),
        steering: globals.is_some_and(|s| s.scalar("packet_steering") == "1"),
        serve: ntp.is_some_and(|s| s.scalar("enable_server") == "1"),
    }
}
fn keyed(
    name: &str,
    label: &str,
    key: &str,
    value: &str,
    errors: &BTreeMap<String, String>,
) -> Widget {
    let mut w = Widget::field(name, label, value, "", "").writes(key);
    if let Widget::Field { error, .. } = &mut w {
        *error = errors.get(name).cloned().unwrap_or_default();
    }
    w
}
fn page(v: Facts, e: &BTreeMap<String, String>) -> Envelope {
    let (clock, _) = local_time();
    let mut zones: Vec<_> = ZONES.iter().map(|(n, _)| SelectOption::new(n, n)).collect();
    if !v.zonename.is_empty() && !zones.iter().any(|z| z.value == v.zonename) {
        zones.insert(0, SelectOption::new(&v.zonename, &v.zonename));
    }
    let mut servers =
        Widget::list("server", "Time servers", "host", &v.servers, "").writes("server");
    if let Widget::List {
        style,
        prompt,
        errors,
        ..
    } = &mut servers
    {
        *style = "rows".into();
        *prompt = "Add a server".into();
        if let Some(error) = e.get("server") {
            errors.insert("0".into(), error.clone());
        }
    }
    // An act on the section's clock, dressed as every act on a part of a
    // section is (the certificate's on Access).
    let mut clock_button = Widget::button("Use my computer's time", "act");
    if let Widget::Button {
        icon, name, value, ..
    } = &mut clock_button
    {
        *icon = "clock".into();
        *name = "_action".into();
        *value = "clock".into();
    }
    let mut time = Widget::section(
        "Time",
        "",
        vec![
            Widget::hidden("_client_time", ""),
            Widget::select(
                "zonename",
                "Time zone",
                &v.zonename,
                zones,
                e.get("zonename").map(String::as_str).unwrap_or(""),
            )
            .writes("zonename")
            .at("system", &v.system_section),
            Widget::switch_keyed(
                "ntp_enabled",
                "Keep the clock synced over the internet",
                "enabled",
                "",
                v.ntp_enabled,
            ),
            servers,
            Widget::switch_keyed(
                "enable_server",
                "Provide time to devices on this network",
                "enable_server",
                "",
                v.serve,
            ),
        ],
    )
    .ruled()
    .at("system", &v.ntp_section);
    if let Widget::Section {
        meta,
        meta_position,
        control,
        ..
    } = &mut time
    {
        *meta = clock;
        *meta_position = "inline".into();
        *control = Some(Box::new(clock_button));
    }
    let form = Widget::Form {
        style: "page".into(),
        submit: "Save".into(),
        note: String::new(),
        target: String::new(),
        error: if e.is_empty() {
            String::new()
        } else {
            e.values().next().cloned().unwrap_or_default()
        },
        fields: vec![
            Widget::section(
                "",
                "",
                vec![
                    keyed("hostname", "Router name", "hostname", &v.hostname, e)
                        .at("system", &v.system_section),
                    keyed("ula_prefix", "Private IPv6 prefix", "ula_prefix", &v.ula, e)
                        .explained(
                            "Devices get a stable local address even without an ISP prefix.",
                            "network globals",
                        )
                        .at("network", &v.globals_section),
                    Widget::switch_keyed(
                        "packet_steering",
                        "Spread packet handling over all cores",
                        "packet_steering",
                        "",
                        v.steering,
                    )
                    .at("network", &v.globals_section),
                ],
            )
            .ruled(),
            time,
        ],
    };
    Envelope::page("General", form)
        .with_width("form")
        .with_tone("neutral")
}
fn valid_ula(value: &str) -> bool {
    let Some((address, prefix)) = value.split_once('/') else {
        return false;
    };
    let Ok(address) = address.parse::<Ipv6Addr>() else {
        return false;
    };
    let Ok(prefix) = prefix.parse::<u32>() else {
        return false;
    };
    (7..=64).contains(&prefix)
        && address.octets()[0] & 0xfe == 0xfc
        && (u128::from(address) & (u128::MAX >> prefix)) == 0
}
fn valid_server(v: &str) -> bool {
    v.parse::<IpAddr>().is_ok() || v.len() <= 253 && v.split('.').all(valid_hostname)
}
// valid_hostname accepts a single DNS label: letters, digits and hyphens, not
// empty, not starting or ending with a hyphen, at most 63 characters.
fn valid_hostname(name: &str) -> bool {
    !name.is_empty()
        && name.len() <= 63
        && !name.starts_with('-')
        && !name.ends_with('-')
        && name.chars().all(|c| c.is_ascii_alphanumeric() || c == '-')
}

fn valid_local_datetime(value: &str) -> bool {
    let bytes = value.as_bytes();
    if bytes.len() != 19
        || bytes[4] != b'-'
        || bytes[7] != b'-'
        || bytes[10] != b'T'
        || bytes[13] != b':'
        || bytes[16] != b':'
    {
        return false;
    }
    let number = |from: usize, to: usize| value[from..to].parse::<u32>().ok();
    let (Some(year), Some(month), Some(day), Some(hour), Some(minute), Some(second)) = (
        number(0, 4),
        number(5, 7),
        number(8, 10),
        number(11, 13),
        number(14, 16),
        number(17, 19),
    ) else {
        return false;
    };
    if !(1970..=9999).contains(&year)
        || !(1..=12).contains(&month)
        || hour > 23
        || minute > 59
        || second > 59
    {
        return false;
    }
    let leap = year % 4 == 0 && (year % 100 != 0 || year % 400 == 0);
    let days = match month {
        2 if leap => 29,
        2 => 28,
        4 | 6 | 9 | 11 => 30,
        _ => 31,
    };
    (1..=days).contains(&day)
}

// valid_posix_tz bounds a preserved custom timezone to the POSIX TZ charset —
// letters, digits, and the punctuation a rule string uses — so no control
// character or shell metacharacter reaches system config or the clock helper.
fn valid_posix_tz(tz: &str) -> bool {
    !tz.is_empty()
        && tz.len() <= 64
        && tz
            .bytes()
            .all(|b| b.is_ascii_alphanumeric() || b"+-,.:/<>".contains(&b))
}

fn local_time() -> (String, String) {
    let seconds = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|duration| duration.as_secs() as _)
        .unwrap_or_default();
    let mut broken = MaybeUninit::<libc::tm>::uninit();
    // SAFETY: localtime_r writes one initialized tm to the valid output pointer;
    // seconds remains alive for the duration of the call.
    let result = unsafe { libc::localtime_r(&seconds, broken.as_mut_ptr()) };
    if result.is_null() {
        return ("Unavailable".into(), String::new());
    }
    // SAFETY: a non-null localtime_r result means the output was initialized.
    let broken = unsafe { broken.assume_init() };
    let year = broken.tm_year + 1900;
    let month = broken.tm_mon + 1;
    let display = format!(
        "{year:04}-{month:02}-{:02} {:02}:{:02}:{:02}",
        broken.tm_mday, broken.tm_hour, broken.tm_min, broken.tm_sec
    );
    let input = format!(
        "{year:04}-{month:02}-{:02}T{:02}:{:02}:{:02}",
        broken.tm_mday, broken.tm_hour, broken.tm_min, broken.tm_sec
    );
    (display, input)
}

#[cfg(test)]
mod tests;
