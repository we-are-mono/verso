// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

use std::mem::MaybeUninit;
use std::time::{SystemTime, UNIX_EPOCH};

use verso_plugin::{json, serve, Form, Snapshot, Value};

mod timezones;

use timezones::ZONES;

#[derive(Default)]
struct Facts {
    hostname: String,
    zonename: String,
    timezone: String,
    ntp_section: String,
    ntp_enabled: bool,
    servers: Vec<String>,
    datetime: String,
}

fn main() {
    serve("system", get, post);
}

fn get(snapshot: &Snapshot) -> Value {
    page(facts(snapshot), "", "")
}

fn post(form: &Form) -> Value {
    let hostname = form.get("hostname").trim().to_string();
    let zonename = form.get("zonename").trim().to_string();
    let original_zonename = form.get("original_zonename");
    let original_timezone = form.get("original_timezone");
    let timezone = ZONES
        .iter()
        .find(|(name, _)| *name == zonename)
        .map(|(_, value)| (*value).to_string())
        .or_else(|| {
            // Preserve an existing custom TZ the catalog doesn't list, but only if
            // the browser-supplied value is a well-formed POSIX TZ — never an
            // arbitrary string (newlines, control chars) into system config.
            (zonename == original_zonename
                && !original_timezone.is_empty()
                && valid_posix_tz(&original_timezone))
            .then_some(original_timezone)
        });
    let servers: Vec<String> = form
        .all("server")
        .into_iter()
        .map(|server| server.trim().to_string())
        .filter(|server| !server.is_empty())
        .collect();
    let datetime = form.get("datetime").trim().to_string();
    let values = Facts {
        hostname: hostname.clone(),
        zonename: zonename.clone(),
        timezone: timezone.clone().unwrap_or_default(),
        ntp_section: form.get("ntp_section"),
        ntp_enabled: form.get("ntp_enabled") == "1",
        servers: servers.clone(),
        datetime: datetime.clone(),
    };

    if timezone.is_none() {
        return page(values, "Choose one of the available timezones.", "");
    }
    if !values.ntp_enabled && !valid_local_datetime(&datetime) {
        return page(
            values,
            "",
            "Enter a valid local date and time, including seconds.",
        );
    }
    let timezone = timezone.unwrap_or_default();
    let ntp_enabled = values.ntp_enabled;
    let ntp_section = values.ntp_section.clone();

    let mut operations = vec![json!({
        "config": "system",
        "section": "@system[0]",
        "values": {
            "hostname": hostname,
            "zonename": zonename,
            "timezone": timezone.clone()
        }
    })];
    let ntp_values = json!({
        "enabled": if ntp_enabled { "1" } else { "0" },
        "server": servers
    });
    if ntp_section.is_empty() {
        operations.push(json!({
            "config": "system", "section": "", "type": "timeserver", "values": ntp_values
        }));
    } else {
        operations.push(json!({
            "config": "system", "section": ntp_section, "values": ntp_values
        }));
    }

    let mut result = page(values, "", "");
    result["commit"] = Value::Array(operations);
    if !ntp_enabled {
        result["apply"] = json!([{
            "name": "set-system-time",
            "args": {
                "datetime": datetime,
                "timezone": timezone
            }
        }]);
    }
    result
}

fn facts(snapshot: &Snapshot) -> Facts {
    let system = snapshot.sections_of_type("system", "system");
    let hostname = system
        .first()
        .map(|section| section.scalar("hostname"))
        .unwrap_or_default();
    let timezone = system
        .first()
        .map(|section| section.scalar("timezone"))
        .unwrap_or_default();
    let mut zonename = system
        .first()
        .map(|section| section.scalar("zonename"))
        .unwrap_or_default();
    if zonename.is_empty() && (timezone == "UTC" || timezone == "GMT0") {
        zonename = "UTC".into();
    }

    let ntp = snapshot.section("system", "ntp").or_else(|| {
        snapshot
            .sections_of_type("system", "timeserver")
            .into_iter()
            .next()
    });
    Facts {
        hostname,
        zonename,
        timezone,
        ntp_section: ntp
            .as_ref()
            .map(|section| section.name())
            .unwrap_or_default(),
        ntp_enabled: ntp
            .as_ref()
            .map(|section| section.scalar("enabled") != "0")
            .unwrap_or(true),
        servers: ntp
            .as_ref()
            .map(|section| section.list("server"))
            .unwrap_or_default(),
        datetime: String::new(),
    }
}

fn page(values: Facts, timezone_error: &str, datetime_error: &str) -> Value {
    let (now_display, now_input) = local_time();
    let datetime = if values.datetime.is_empty() {
        now_input
    } else {
        values.datetime.clone()
    };
    let mut zones: Vec<Value> = ZONES
        .iter()
        .map(|(value, _)| json!({"value": value, "label": value}))
        .collect();
    if !values.zonename.is_empty() && !ZONES.iter().any(|(name, _)| *name == values.zonename) {
        zones.insert(
            0,
            json!({"value": values.zonename, "label": values.zonename}),
        );
    }
    let servers = if values.servers.is_empty() {
        vec![
            "0.openwrt.pool.ntp.org".to_string(),
            "1.openwrt.pool.ntp.org".to_string(),
        ]
    } else {
        values.servers.clone()
    };

    json!({
        "schema_version": 1,
        "title": "System",
        "subheading": "The name, place, and clock shared by everything on this router.",
        "width": "narrow",
        "widget": {
            "type": "form",
            "style": "page",
            "fields": [
                {
                    "type": "section",
                    "title": "Device identity",
                    "sub": "The name this router uses on your network and in Verso.",
                    "children": [{
                        "type": "stack", "width": "compact", "children": [{
                            "type": "field", "name": "hostname", "label": "Hostname",
                            "value": values.hostname, "datatype": "hostname",
                            "help": "Use letters, numbers, and hyphens. Devices may find it using the local network suffix."
                        }]
                    }]
                },
                {
                    "type": "section",
                    "title": "Time and region",
                    "sub": "Used for logs, schedules, certificates, and every time shown by the router.",
                    "meta": now_display, "meta_icon": "clock",
                    "meta_position": "inline",
                    "children": [
                        {"type": "stack", "width": "compact", "children": [{
                                "type": "field", "name": "zonename", "label": "Timezone", "kind": "select",
                                "value": values.zonename, "options": zones, "error": timezone_error
                        }]},
                        {"type": "field", "name": "original_zonename", "kind": "hidden", "value": values.zonename},
                        {"type": "field", "name": "original_timezone", "kind": "hidden", "value": values.timezone}
                    ]
                },
                {
                    "type": "section",
                    "title": "Time synchronization",
                    "sub": "Keep the clock accurate automatically using trusted time servers.",
                    "meta": "Last synchronization not reported", "meta_icon": "clock",
                    "meta_position": "inline",
                    "children": [
                        {
                            "type": "conditional", "name": "ntp_enabled",
                            "label": "Set the time automatically", "checked": values.ntp_enabled,
                            "fields": [{"type": "stack", "width": "compact", "children": [{
                                    "type": "list", "name": "server", "label": "Time servers",
                                    "kind": "text", "datatype": "host", "items": servers,
                                    "help": "Servers are tried in order; leave several so time still works if one is unavailable."
                            }]}],
                            "otherwise": [{"type": "stack", "width": "compact", "children": [{
                                    "type": "field", "name": "datetime", "label": "Date and time",
                                    "kind": "datetime-local", "value": datetime, "error": datetime_error,
                                    "help": "Interpreted in the selected timezone and applied to the router clock."
                            }]}]
                        },
                        {"type": "field", "name": "ntp_section", "kind": "hidden", "value": values.ntp_section}
                    ]
                }
            ]
        }
    })
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
mod tests {
    use super::{valid_local_datetime, valid_posix_tz, ZONES};
    use std::collections::HashSet;

    #[test]
    fn timezone_catalog_is_complete_and_unique() {
        assert!(
            ZONES.len() >= 400,
            "timezone catalog has only {} entries",
            ZONES.len()
        );

        let mut names = HashSet::new();
        for (name, tzstring) in ZONES {
            assert!(!name.is_empty(), "timezone name must not be empty");
            assert!(!tzstring.is_empty(), "{name} has no POSIX TZ string");
            assert!(names.insert(*name), "duplicate timezone {name}");
        }

        for expected in [
            "UTC",
            "Africa/Johannesburg",
            "America/New_York",
            "Asia/Tokyo",
            "Australia/Sydney",
            "Europe/Ljubljana",
            "Pacific/Auckland",
        ] {
            assert!(
                names.contains(expected),
                "timezone catalog is missing {expected}"
            );
        }
    }

    #[test]
    fn timezone_catalog_keeps_openwrt_posix_values() {
        let lookup = |name| {
            ZONES
                .iter()
                .find(|(candidate, _)| *candidate == name)
                .map(|(_, tzstring)| *tzstring)
        };
        assert_eq!(lookup("UTC"), Some("GMT0"));
        assert_eq!(
            lookup("Europe/Ljubljana"),
            Some("CET-1CEST,M3.5.0,M10.5.0/3")
        );
        assert_eq!(lookup("America/New_York"), Some("EST5EDT,M3.2.0,M11.1.0"));
    }

    #[test]
    fn local_datetime_validation_is_calendar_aware() {
        for value in ["2026-08-30T12:34:56", "2024-02-29T00:00:00"] {
            assert!(valid_local_datetime(value), "rejected {value}");
        }
        for value in [
            "",
            "2026-08-30T12:34",
            "2026-02-29T12:34:56",
            "2026-13-01T12:34:56",
            "2026-08-30T24:00:00",
        ] {
            assert!(!valid_local_datetime(value), "accepted {value}");
        }
    }

    #[test]
    fn posix_tz_charset_rejects_control_and_meta() {
        for value in ["GMT0", "CET-1CEST,M3.5.0,M10.5.0/3", "<+08>-8"] {
            assert!(valid_posix_tz(value), "rejected {value:?}");
        }
        for value in ["", "UTC\nfoo", "US/x;rm -rf /", "a b"] {
            assert!(!valid_posix_tz(value), "accepted {value:?}");
        }
        assert!(!valid_posix_tz(&"A".repeat(65)), "accepted 65-char tz");
    }
}
