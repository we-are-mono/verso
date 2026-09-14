// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
//! Narrow read of protected dnsmasq configuration for the shell's service
//! observation. Never reads the file stage or changes active configuration.
use crate::Failure;
use serde_json::{json, Map, Value};
use std::{fs, io::Read, path::Path};

fn read(path: &Path) -> Option<String> {
    // These roots contain only DNS configuration. Symlink targets are checked
    // too, so a redirected include cannot turn this into a root file reader.
    let resolved = fs::canonicalize(path).ok()?;
    let allowed = resolved == Path::new("/etc/dnsmasq.conf")
        || resolved.starts_with("/etc/dnsmasq.d")
        || resolved.starts_with("/usr/share/dnsmasq");
    if !allowed {
        return None;
    }
    let file = fs::File::open(&resolved).ok()?;
    if !file.metadata().ok()?.is_file() {
        return None;
    }
    let mut body = String::new();
    file.take(32_769).read_to_string(&mut body).ok()?;
    if body.len() > 32_768 {
        return None;
    }
    // Forward only directives that can affect service scope or its includes.
    // Host reservations, option contents and DNS records are not disclosed.
    Some(relevant(&body))
}
fn relevant(body: &str) -> String {
    body.lines()
        .filter(|line| {
            let key = line.trim().split('=').next().unwrap_or("");
            matches!(
                key,
                "dhcp-range"
                    | "dhcp-leasefile"
                    | "no-dhcp-interface"
                    | "interface"
                    | "except-interface"
                    | "conf-file"
                    | "conf-dir"
                    | "conf-script"
            )
        })
        .collect::<Vec<_>>()
        .join("\n")
}
pub fn state() -> Result<Value, Failure> {
    let mut files = Map::new();
    let mut directories = Map::new();
    for path in [
        "/etc/dnsmasq.conf",
        "/usr/share/dnsmasq/dhcpbogushostname.conf",
        "/usr/share/dnsmasq/rfc6761.conf",
    ] {
        if let Some(body) = read(Path::new(path)) {
            files.insert(path.into(), json!(body));
        }
    }
    // The editor's supported custom-options directory. Other readable includes
    // can still be observed by the shell without expanding this root boundary.
    if let Ok(entries) = fs::read_dir("/etc/dnsmasq.d") {
        let mut names = Vec::new();
        for entry in entries.flatten().take(64) {
            if entry.file_type().is_ok_and(|t| t.is_dir()) {
                continue;
            }
            let path = entry.path();
            names.push(entry.file_name().to_string_lossy().into_owned());
            if let Some(body) = read(&path) {
                files.insert(path.to_string_lossy().into_owned(), json!(body));
            }
        }
        names.sort();
        directories.insert("/etc/dnsmasq.d".into(), json!(names));
    }
    Ok(json!({"files":files,"directories":directories}))
}
#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn runtime_read_does_not_disclose_reservations_or_dns_records() {
        let text = "# comment\naddress=/private.example/10.0.0.1\ndhcp-host=aa:bb:cc:dd:ee:ff,private-host\nno-dhcp-interface=br-lan\nconf-file=/etc/dnsmasq.d/local.conf\ndhcp-range=set:lan,10.0.0.100,10.0.0.200,12h\n";
        assert_eq!(relevant(text), "no-dhcp-interface=br-lan\nconf-file=/etc/dnsmasq.d/local.conf\ndhcp-range=set:lan,10.0.0.100,10.0.0.200,12h");
    }
}
