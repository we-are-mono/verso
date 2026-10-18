// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! The root side of the trusted certificate (ADR-017 §6). OpenWrt's acme
//! package obtains and renews the certificate of `config cert 'verso'`; this
//! module carries it to the shell and says how that went.
//!
//! `deliver` runs as root, from acme's hotplug (each outcome acme.sh reports),
//! from the `verso` init before the shell starts, and from a config trigger on
//! `acme` (acme.sh sends nothing when a renewal is not yet due, so a section
//! turned back on over a certificate already on disk is caught up there). It
//! records the outcome in RAM and, when acme holds a certificate other than the
//! one served, hands it to `verso certificate install`, which checks it as a
//! pasted pair is checked. `state` is what the plugin's part of Access reads:
//! the record, whether a request is running, and acme's certificate, public
//! material only.

use serde_json::{json, Value};
use std::fs;
use std::io::Read;
use std::path::{Path, PathBuf};
use std::process::{Command, Stdio};
use std::time::{SystemTime, UNIX_EPOCH};

use crate::Failure;

/// The cert section whose certificate Verso serves.
const SECTION: &str = "verso";
/// acme's procd service, whose instances are named after their sections.
const SERVICE: &str = "acme";
/// Where acme-common links each certificate (CERT_DIR).
const CERT_DIR: &str = "/etc/ssl/acme";
/// The served pair (ADR-017 §3).
const SERVED: &str = "/etc/verso/tls.crt";
const TLS_DIR: &str = "/etc/verso";
/// The record of the last outcome: a few lines in RAM, gone on reboot.
const RECORD_DIR: &str = "/var/run/verso-acme";
/// acme.sh's account settings under acme-common's `--home /etc/acme`.
const ACCOUNT_CONF: &str = "/etc/acme/account.conf";
const VERSO: &str = "/usr/bin/verso";
/// A chain of a few certificates fits many times over.
const LIMIT: u64 = 64 * 1024;

/// Event is what deliver is run for: an action acme's notify hands hotplug, or
/// `sync`, a catch-up that reports nothing new.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum Event {
    Prepare,
    Issued,
    Renewed,
    IssueFailed,
    RenewFailed,
    Sync,
}

impl Event {
    pub fn parse(name: &str) -> Option<Event> {
        Some(match name {
            "prepare" => Event::Prepare,
            "issued" => Event::Issued,
            "renewed" => Event::Renewed,
            "issue-failed" => Event::IssueFailed,
            "renew-failed" => Event::RenewFailed,
            "sync" => Event::Sync,
            _ => return None,
        })
    }

    fn name(self) -> &'static str {
        match self {
            Event::Prepare => "prepare",
            Event::Issued => "issued",
            Event::Renewed => "renewed",
            Event::IssueFailed => "issue-failed",
            Event::RenewFailed => "renew-failed",
            Event::Sync => "sync",
        }
    }

    /// An outcome is what the part says last happened. `prepare` is a request
    /// starting, which the running instance already says; `sync` is no news.
    fn outcome(self) -> bool {
        !matches!(self, Event::Prepare | Event::Sync)
    }

    /// Whether acme may now hold a certificate to serve.
    fn delivers(self) -> bool {
        matches!(self, Event::Issued | Event::Renewed | Event::Sync)
    }
}

/// Router is what deliver and state need of the router, so each can be
/// exercised without one.
pub trait Router {
    /// The committed `acme` config, sections by name as uci_committed reads it.
    fn acme(&self) -> Result<Value, Failure>;
    /// The names of service's running procd instances.
    fn running(&self, service: &str) -> Vec<String>;
    /// `verso certificate install` with the pair at cert and key.
    fn install(&self, cert: &Path, key: &Path) -> Result<(), Failure>;
    /// Tell the running shell to read its pair again.
    fn signal_verso(&self) -> Result<(), Failure>;
    /// Turn `acme.verso` off and commit acme, through root's save directory.
    fn disable(&self) -> Result<(), Failure>;
    fn now(&self) -> u64;
}

/// Places are the directories and files deliver and state read and write.
pub struct Places {
    pub certs: PathBuf,
    pub served: PathBuf,
    pub records: PathBuf,
    /// acme.sh's account.conf, where its DNS scripts save what they were given.
    pub account_conf: PathBuf,
}

impl Places {
    pub fn router() -> Places {
        Places {
            certs: CERT_DIR.into(),
            served: SERVED.into(),
            records: RECORD_DIR.into(),
            account_conf: ACCOUNT_CONF.into(),
        }
    }

    fn pair(&self, domain: &str) -> (PathBuf, PathBuf) {
        (
            self.certs.join(format!("{domain}.fullchain.crt")),
            self.certs.join(format!("{domain}.key")),
        )
    }

    fn record(&self) -> PathBuf {
        self.records.join(SECTION)
    }
}

/// Done is what a deliver did, for its caller and its tests.
#[derive(Debug, PartialEq, Eq)]
pub enum Done {
    /// Not for Verso's certificate, or nothing to do.
    Nothing,
    /// The outcome was recorded; nothing was installed.
    Recorded,
    /// acme's certificate is the one already served.
    Unchanged,
    /// acme's certificate is now on disk for the shell.
    Installed,
}

/// Section is what deliver reads of `acme.verso`.
struct Section {
    domain: String,
    enabled: bool,
    /// Each `credentials` entry as acme-common hands it to acme.sh: the
    /// variable and its value, unquoted.
    credentials: Vec<(String, String)>,
}

/// section reads `config cert 'verso'`: its first domain, when it names one
/// acme-common could have made files for, and whether it is on. acme-common
/// takes an absent `enabled` as on.
fn section(acme: &Value) -> Option<Section> {
    let s = acme.get(SECTION)?;
    if s[".type"] != "cert" {
        return None;
    }
    let domain = match &s["domains"] {
        Value::Array(list) => list.first()?.as_str()?,
        Value::String(one) => one.split_whitespace().next()?,
        _ => return None,
    };
    if !file_name(domain) {
        return None;
    }
    let enabled = !matches!(s["enabled"].as_str(), Some("0" | "no" | "off" | "false"));
    let entries = match &s["credentials"] {
        Value::Array(list) => list.iter().filter_map(Value::as_str).collect(),
        Value::String(one) => vec![one.as_str()],
        _ => Vec::new(),
    };
    let credentials = entries
        .into_iter()
        .filter_map(|entry| {
            let (name, value) = entry.split_once('=')?;
            Some((name.trim().to_owned(), unquote(value.trim()).to_owned()))
        })
        .collect();
    Some(Section { domain: domain.to_owned(), enabled, credentials })
}

fn unquote(value: &str) -> &str {
    ['"', '\'']
        .iter()
        .find_map(|q| value.strip_prefix(*q).and_then(|v| v.strip_suffix(*q)))
        .unwrap_or(value)
}

/// forget_stale drops from acme.sh's account.conf every line that sets one of
/// the section's credentials to another value. acme.sh's DNS scripts save
/// what they were given there, and acme.sh reads that file at every start
/// over what acme-common hands it, so a credential changed in Verso would lose
/// to the one saved before it. The plugin's config trigger runs this ahead of
/// acme-common's, which waits procd's second. A file left as it was is not
/// written.
fn forget_stale(path: &Path, credentials: &[(String, String)]) -> Result<(), Failure> {
    let Some(saved) = read_public(path)? else {
        return Ok(());
    };
    let stale = |line: &str| {
        let Some((name, value)) = line.split_once('=') else {
            return false;
        };
        credentials
            .iter()
            .any(|(variable, wanted)| variable == name.trim() && unquote(value.trim()) != wanted)
    };
    let kept: Vec<&str> = saved.split_inclusive('\n').filter(|line| !stale(line)).collect();
    if kept.len() == saved.split_inclusive('\n').count() {
        return Ok(());
    }
    use std::os::unix::fs::PermissionsExt;
    let mode = fs::metadata(path)
        .map(|m| m.permissions().mode() & 0o777)
        .unwrap_or(0o600);
    let tmp = crate::access::write_new(path, kept.concat().as_bytes(), mode, None)?;
    fs::rename(&tmp, path).map_err(|e| {
        let _ = fs::remove_file(&tmp);
        Failure::unknown(e.to_string())
    })
}

/// file_name holds a domain to what can name a file under CERT_DIR and nothing
/// that leaves it: a hostname, or one under a leading wildcard.
fn file_name(domain: &str) -> bool {
    let name = domain.strip_prefix("*.").unwrap_or(domain);
    !name.is_empty()
        && name.len() <= 253
        && !name.starts_with(['.', '-'])
        && !name.contains("..")
        && name.bytes().all(|b| b.is_ascii_alphanumeric() || matches!(b, b'.' | b'-'))
}

/// deliver carries one event through: for Verso's section alone, and only while
/// it is on, it records an outcome and, unless acme said it failed, hands a
/// certificate the shell does not serve yet to `verso certificate install`,
/// then signals the shell if it runs. main_domain is acme's own name for the
/// certificate an event is about; a catch-up has none.
pub fn deliver(event: Event, main_domain: Option<&str>, router: &impl Router, places: &Places) -> Result<Done, Failure> {
    if event == Event::Prepare {
        return Ok(Done::Nothing);
    }
    let Some(section) = section(&router.acme()?) else {
        return Ok(Done::Nothing);
    };
    if event == Event::Sync {
        forget_stale(&places.account_conf, &section.credentials)?;
    }
    if !section.enabled {
        return Ok(Done::Nothing);
    }
    if event != Event::Sync && main_domain != Some(section.domain.as_str()) {
        return Ok(Done::Nothing);
    }
    let mut done = Done::Nothing;
    if event.outcome() {
        record(places, &section.domain, event, router.now())?;
        done = Done::Recorded;
    }
    if !event.delivers() {
        return Ok(done);
    }
    let (cert, key) = places.pair(&section.domain);
    let Some(held) = read_public(&cert)? else {
        return Ok(done);
    };
    if read_public(&places.served)?.is_some_and(|served| served.trim() == held.trim()) {
        return Ok(Done::Unchanged);
    }
    router.install(&cert, &key)?;
    if !router.running("verso").is_empty() {
        router.signal_verso()?;
    }
    Ok(Done::Installed)
}

/// record replaces the record with the event and its time, by rename, so a
/// reader never meets half of one.
fn record(places: &Places, domain: &str, event: Event, at: u64) -> Result<(), Failure> {
    use std::os::unix::fs::DirBuilderExt;
    fs::DirBuilder::new()
        .recursive(true)
        .mode(0o755)
        .create(&places.records)
        .map_err(|e| Failure::unknown(format!("record directory: {e}")))?;
    let path = places.record();
    let text = format!("domain={domain}\nevent={}\nat={at}\n", event.name());
    let tmp = crate::access::write_new(&path, text.as_bytes(), 0o644, None)?;
    fs::rename(&tmp, &path).map_err(|e| {
        let _ = fs::remove_file(&tmp);
        Failure::unknown(e.to_string())
    })
}

/// read_public is a certificate file's text, or none where there is no file. A
/// certificate is public: the key beside it is never read here.
fn read_public(path: &Path) -> Result<Option<String>, Failure> {
    let file = match fs::File::open(path) {
        Ok(f) => f,
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => return Ok(None),
        Err(e) => return Err(Failure::unknown(format!("{}: {e}", path.display()))),
    };
    let mut text = String::new();
    file.take(LIMIT + 1)
        .read_to_string(&mut text)
        .map_err(|e| Failure::unknown(format!("{}: {e}", path.display())))?;
    if text.len() as u64 > LIMIT {
        return Err(Failure::invalid("certificate file too large"));
    }
    Ok(Some(text))
}

/// state is the trusted certificate as the plugin's part shows it: the last
/// outcome recorded for the section's domain, whether acme is requesting one
/// now, acme's certificate for that domain (the shell turns it into facts), and
/// whether it is the one served. An absent section is an empty answer.
pub fn state(router: &impl Router, places: &Places) -> Result<Value, Failure> {
    let Some(section) = section(&router.acme()?) else {
        return Ok(json!({}));
    };
    let mut out = json!({
        "domain": section.domain,
        "requesting": router.running(SERVICE).iter().any(|name| name == SECTION),
    });
    if let Some((event, at)) = read_record(&places.record(), &section.domain) {
        out["outcome"] = json!(event);
        out["at"] = json!(at);
    }
    let (cert, _) = places.pair(&section.domain);
    if let Some(held) = read_public(&cert)? {
        let served = read_public(&places.served)?.is_some_and(|served| served.trim() == held.trim());
        out["certificate"] = json!(held);
        out["serving"] = json!(served);
    }
    Ok(out)
}

/// read_record is the recorded event and time, when the record is for domain:
/// one left by a domain since changed says nothing about this one.
fn read_record(path: &Path, domain: &str) -> Option<(String, u64)> {
    let text = fs::read_to_string(path).ok()?;
    let mut fields = std::collections::HashMap::new();
    for line in text.lines() {
        if let Some((key, value)) = line.split_once('=') {
            fields.insert(key, value);
        }
    }
    if fields.get("domain") != Some(&domain) {
        return None;
    }
    let event = Event::parse(fields.get("event")?).filter(|e| e.outcome())?;
    Some((event.name().to_owned(), fields.get("at")?.parse().ok()?))
}

/// hand_over keeps a certificate installed or made by hand from being replaced
/// at the next renewal: when `acme.verso` is on, it is turned off (ADR-017 §6).
pub fn hand_over(router: &impl Router) -> Result<(), Failure> {
    match section(&router.acme()?) {
        Some(section) if section.enabled => router.disable(),
        _ => Ok(()),
    }
}

/// running_instances is the names of service's running instances in a procd
/// `service list` answer.
fn running_instances(answer: &[u8], service: &str) -> Vec<String> {
    let Ok(answer) = serde_json::from_slice::<Value>(answer) else {
        return Vec::new();
    };
    let Some(instances) = answer[service]["instances"].as_object() else {
        return Vec::new();
    };
    instances
        .iter()
        .filter(|(_, instance)| instance["running"].as_bool() == Some(true))
        .map(|(name, _)| name.clone())
        .collect()
}

/// System is the router itself.
pub struct System;

impl Router for System {
    fn acme(&self) -> Result<Value, Failure> {
        crate::uci_committed::read("acme")
    }

    fn running(&self, service: &str) -> Vec<String> {
        Command::new("/bin/ubus")
            .args(["call", "service", "list", &json!({ "name": service }).to_string()])
            .stdin(Stdio::null())
            .output()
            .ok()
            .filter(|out| out.status.success())
            .map(|out| running_instances(&out.stdout, service))
            .unwrap_or_default()
    }

    fn install(&self, cert: &Path, key: &Path) -> Result<(), Failure> {
        let output = Command::new(VERSO)
            .args(["certificate", "install", TLS_DIR])
            .arg(cert)
            .arg(key)
            .stdin(Stdio::null())
            .output()
            .map_err(|e| Failure::unknown(format!("{VERSO}: {e}")))?;
        if !output.status.success() {
            return Err(crate::command_failure("verso certificate install", output));
        }
        Ok(())
    }

    fn signal_verso(&self) -> Result<(), Failure> {
        crate::access::announce().map(|_| ())
    }

    fn disable(&self) -> Result<(), Failure> {
        let uci = |args: &[&str]| -> Result<(), Failure> {
            let status = Command::new("/sbin/uci")
                .args(args)
                .stdout(Stdio::null())
                .status()
                .map_err(|e| Failure::unknown(e.to_string()))?;
            if !status.success() {
                return Err(Failure::unknown(
                    "certificate saved, but the trusted certificate could not be turned off",
                ));
            }
            Ok(())
        };
        uci(&["set", "acme.verso.enabled=0"])?;
        // Verso's own act, not an edit made outside it (ADR-021).
        crate::config_watch::own_write("acme");
        uci(&["commit", "acme"])
    }

    fn now(&self) -> u64 {
        SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .map(|d| d.as_secs())
            .unwrap_or(0)
    }
}

/// command_line runs `verso-rpcd acme deliver EVENT`, acme's hotplug and the
/// init's catch-up, and reports whether it was asked to. acme's environment
/// names the certificate an event is about as `main_domain`.
pub fn command_line() -> bool {
    let args: Vec<String> = std::env::args().skip(1).collect();
    let event = match command(&args) {
        None => return false,
        Some(Ok(event)) => event,
        Some(Err(usage)) => {
            eprintln!("{usage}");
            std::process::exit(2);
        }
    };
    unsafe extern "C" {
        fn geteuid() -> u32;
    }
    if unsafe { geteuid() } != 0 {
        eprintln!("verso-rpcd: acme deliver requires local root");
        std::process::exit(2);
    }
    let main_domain = std::env::var("main_domain").ok();
    match deliver(event, main_domain.as_deref(), &System, &Places::router()) {
        Ok(Done::Installed) => println!("verso-rpcd: acme certificate delivered"),
        Ok(_) => {}
        Err(error) => {
            eprintln!("verso-rpcd: acme deliver: {}", error.message);
            std::process::exit(1);
        }
    }
    true
}

/// command reads the helper's arguments as an acme command: none when they are
/// not one, the event when they are `acme deliver EVENT`, the usage otherwise.
fn command(args: &[String]) -> Option<Result<Event, &'static str>> {
    if args.first().map(String::as_str) != Some("acme") {
        return None;
    }
    let event = match args {
        [_, verb, event] if verb == "deliver" => Event::parse(event),
        _ => None,
    };
    Some(event.ok_or("usage: verso-rpcd acme deliver issued|renewed|issue-failed|renew-failed|prepare|sync"))
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::cell::RefCell;

    fn args(list: &[&str]) -> Vec<String> {
        list.iter().map(|s| s.to_string()).collect()
    }

    #[test]
    fn only_acme_deliver_with_a_known_event_is_an_acme_command() {
        assert_eq!(command(&args(&[])), None);
        assert_eq!(command(&args(&["--socket", "/tmp/s.sock"])), None);
        assert_eq!(command(&args(&["acme", "deliver", "renewed"])), Some(Ok(Event::Renewed)));
        for wrong in [&["acme"][..], &["acme", "deliver"], &["acme", "deliver", "renew"], &["acme", "issue", "issued"], &["acme", "deliver", "issued", "x"]] {
            assert!(matches!(command(&args(wrong)), Some(Err(_))), "{wrong:?}");
        }
    }

    const NOTIFY_ACTIONS: &str = include_str!("../testdata/acme/notify-actions.txt");
    const REQUESTING: &[u8] = include_bytes!("../testdata/acme/service-list-requesting.json");
    const SERVICES: &[u8] = include_bytes!("../../internal/openwrt/testdata/service-list-25.12.json");

    const DOMAIN: &str = "router.example.com";

    struct Fake {
        acme: Value,
        running: Vec<(&'static str, &'static str)>,
        install: Result<(), &'static str>,
        installed: RefCell<Vec<(PathBuf, PathBuf)>>,
        signalled: RefCell<u32>,
        disabled: RefCell<u32>,
    }

    impl Fake {
        fn with(acme: Value) -> Fake {
            Fake {
                acme,
                running: vec![("verso", "instance1")],
                install: Ok(()),
                installed: RefCell::new(Vec::new()),
                signalled: RefCell::new(0),
                disabled: RefCell::new(0),
            }
        }
    }

    impl Router for Fake {
        fn acme(&self) -> Result<Value, Failure> {
            Ok(self.acme.clone())
        }
        fn running(&self, service: &str) -> Vec<String> {
            self.running
                .iter()
                .filter(|(s, _)| *s == service)
                .map(|(_, name)| name.to_string())
                .collect()
        }
        fn install(&self, cert: &Path, key: &Path) -> Result<(), Failure> {
            self.install.map_err(Failure::unknown)?;
            self.installed.borrow_mut().push((cert.to_owned(), key.to_owned()));
            Ok(())
        }
        fn signal_verso(&self) -> Result<(), Failure> {
            *self.signalled.borrow_mut() += 1;
            Ok(())
        }
        fn disable(&self) -> Result<(), Failure> {
            *self.disabled.borrow_mut() += 1;
            Ok(())
        }
        fn now(&self) -> u64 {
            1_760_000_000
        }
    }

    fn verso(enabled: Option<&str>) -> Value {
        let mut s = json!({".type": "cert", ".name": "verso", "domains": [DOMAIN], "validation_method": "dns", "dns": "dns_cf"});
        if let Some(e) = enabled {
            s["enabled"] = json!(e);
        }
        json!({ "verso": s })
    }

    fn scratch(name: &str) -> Places {
        let dir = std::env::temp_dir().join(format!("verso-acme-{name}-{}", std::process::id()));
        let _ = fs::remove_dir_all(&dir);
        for sub in ["certs", "served", "records", "acme"] {
            fs::create_dir_all(dir.join(sub)).expect("scratch dir");
        }
        Places {
            certs: dir.join("certs"),
            served: dir.join("served/tls.crt"),
            records: dir.join("records/verso-acme"),
            account_conf: dir.join("acme/account.conf"),
        }
    }

    fn hold(places: &Places, domain: &str, cert: &str) {
        let (c, k) = places.pair(domain);
        fs::write(c, cert).unwrap();
        fs::write(k, "KEY").unwrap();
    }

    fn recorded(places: &Places) -> String {
        fs::read_to_string(places.record()).unwrap_or_default()
    }

    #[test]
    fn deliver_handles_exactly_the_actions_acme_sends() {
        let sent: Vec<&str> = NOTIFY_ACTIONS
            .lines()
            .map(str::trim)
            .filter(|l| !l.is_empty() && !l.starts_with('#'))
            .collect();
        for action in &sent {
            assert!(Event::parse(action).is_some(), "acme sends {action}, which deliver does not take");
        }
        for event in [Event::Prepare, Event::Issued, Event::Renewed, Event::IssueFailed, Event::RenewFailed] {
            assert!(sent.contains(&event.name()), "deliver takes {}, which acme never sends", event.name());
        }
        assert_eq!(Event::parse("sync"), Some(Event::Sync));
        assert_eq!(Event::parse("renew"), None);
    }

    #[test]
    fn an_issued_certificate_is_recorded_installed_and_announced() {
        let places = scratch("issued");
        hold(&places, DOMAIN, "CERT-NEW\n");
        fs::write(&places.served, "CERT-SELF\n").unwrap();
        let router = Fake::with(verso(None));
        assert_eq!(deliver(Event::Issued, Some(DOMAIN), &router, &places).unwrap(), Done::Installed);
        assert_eq!(*router.installed.borrow(), vec![places.pair(DOMAIN)]);
        assert_eq!(*router.signalled.borrow(), 1);
        assert_eq!(recorded(&places), format!("domain={DOMAIN}\nevent=issued\nat=1760000000\n"));
        let entries: Vec<_> = fs::read_dir(&places.records).unwrap().collect();
        assert_eq!(entries.len(), 1, "the record is replaced by rename, nothing left beside it");
    }

    #[test]
    fn a_shell_that_is_not_running_is_not_signalled() {
        let places = scratch("not-running");
        hold(&places, DOMAIN, "CERT-NEW\n");
        let mut router = Fake::with(verso(Some("1")));
        router.running.clear();
        assert_eq!(deliver(Event::Renewed, Some(DOMAIN), &router, &places).unwrap(), Done::Installed);
        assert_eq!(*router.signalled.borrow(), 0);
    }

    #[test]
    fn an_install_the_command_refuses_sends_no_signal() {
        let places = scratch("refused");
        hold(&places, DOMAIN, "CERT-EXPIRED\n");
        let mut router = Fake::with(verso(None));
        router.install = Err("The certificate is not currently valid.");
        let err = deliver(Event::Sync, None, &router, &places).unwrap_err();
        assert!(err.message.contains("not currently valid"), "{}", err.message);
        assert_eq!(*router.signalled.borrow(), 0);
        assert_eq!(recorded(&places), "", "a catch-up records nothing");
    }

    #[test]
    fn a_failure_is_recorded_and_nothing_installed() {
        let places = scratch("failed");
        hold(&places, DOMAIN, "CERT-OLD\n");
        let router = Fake::with(verso(None));
        assert_eq!(deliver(Event::RenewFailed, Some(DOMAIN), &router, &places).unwrap(), Done::Recorded);
        assert!(router.installed.borrow().is_empty());
        assert!(recorded(&places).contains("event=renew-failed\n"));
    }

    #[test]
    fn another_domain_or_an_off_or_absent_section_changes_nothing() {
        let places = scratch("elsewhere");
        hold(&places, DOMAIN, "CERT-NEW\n");
        hold(&places, "nas.example.com", "CERT-NAS\n");
        for (acme, domain) in [
            (verso(None), "nas.example.com"),
            (verso(Some("0")), DOMAIN),
            (json!({}), DOMAIN),
            (json!({"verso": {".type": "acme"}}), DOMAIN),
        ] {
            let router = Fake::with(acme);
            assert_eq!(deliver(Event::Issued, Some(domain), &router, &places).unwrap(), Done::Nothing);
            assert!(router.installed.borrow().is_empty());
        }
        assert_eq!(recorded(&places), "");
    }

    #[test]
    fn a_catch_up_installs_only_a_certificate_not_yet_served() {
        let places = scratch("sync");
        hold(&places, DOMAIN, "CERT-NEW\n");
        fs::write(&places.served, "CERT-NEW").unwrap();
        let router = Fake::with(verso(None));
        assert_eq!(deliver(Event::Sync, None, &router, &places).unwrap(), Done::Unchanged);
        assert!(router.installed.borrow().is_empty());
        assert_eq!(*router.signalled.borrow(), 0);

        fs::write(&places.served, "CERT-SELF\n").unwrap();
        assert_eq!(deliver(Event::Sync, None, &router, &places).unwrap(), Done::Installed);
        assert_eq!(recorded(&places), "", "a catch-up is no news");
    }

    #[test]
    fn a_catch_up_with_nothing_held_does_nothing() {
        let places = scratch("nothing-held");
        let router = Fake::with(verso(None));
        assert_eq!(deliver(Event::Sync, None, &router, &places).unwrap(), Done::Nothing);
    }

    #[test]
    fn prepare_is_a_request_starting_and_nothing_to_record() {
        let places = scratch("prepare");
        let router = Fake::with(verso(None));
        assert_eq!(deliver(Event::Prepare, Some(DOMAIN), &router, &places).unwrap(), Done::Nothing);
        assert_eq!(recorded(&places), "");
    }

    #[test]
    fn a_domain_that_could_leave_the_certificate_directory_is_no_section() {
        for domain in ["../etc/shadow", "a/b", ".hidden", "", "router..example.com"] {
            let acme = json!({"verso": {".type": "cert", "domains": [domain]}});
            assert!(section(&acme).is_none(), "{domain:?} taken as a domain");
        }
        let wildcard = json!({"verso": {".type": "cert", "domains": ["*.example.com"]}});
        assert_eq!(section(&wildcard).unwrap().domain, "*.example.com");
        let spaced = json!({"verso": {".type": "cert", "domains": "router.example.com www.example.com"}});
        assert_eq!(section(&spaced).unwrap().domain, DOMAIN);
    }

    #[test]
    fn state_says_what_happened_and_whether_the_held_certificate_is_served() {
        let places = scratch("state");
        let mut router = Fake::with(verso(None));
        router.running = vec![("acme", "verso")];
        assert_eq!(state(&router, &places).unwrap(), json!({"domain": DOMAIN, "requesting": true}));

        hold(&places, DOMAIN, "CERT-NEW\n");
        fs::write(&places.served, "CERT-NEW\n").unwrap();
        record(&places, DOMAIN, Event::Renewed, 1_759_000_000).unwrap();
        router.running.clear();
        let s = state(&router, &places).unwrap();
        assert_eq!(s["outcome"], "renewed");
        assert_eq!(s["at"], 1_759_000_000u64);
        assert_eq!(s["requesting"], false);
        assert_eq!(s["certificate"], "CERT-NEW\n");
        assert_eq!(s["serving"], true);
        assert!(!s.to_string().contains("KEY"), "no key material: {s}");

        fs::write(&places.served, "CERT-HAND\n").unwrap();
        assert_eq!(state(&router, &places).unwrap()["serving"], false);
    }

    #[test]
    fn a_record_left_by_another_domain_says_nothing() {
        let places = scratch("stale-record");
        record(&places, "old.example.com", Event::IssueFailed, 1).unwrap();
        let s = state(&Fake::with(verso(Some("0"))), &places).unwrap();
        assert!(s.get("outcome").is_none(), "{s}");
        assert_eq!(state(&Fake::with(json!({})), &places).unwrap(), json!({}));
    }

    // acme.sh's DNS scripts save the credentials they were given in
    // account.conf, which it reads at every start over what acme-common hands
    // it: a token changed in Verso would lose to the one saved before it.
    #[test]
    fn a_catch_up_forgets_the_credentials_acme_sh_saved_that_verso_since_changed() {
        let places = scratch("forget");
        let saved = "LOG_FILE='/var/log/acme.log'\nDNSimple_OAUTH_TOKEN='dnsimple_a_old'\nSAVED_CF_Token='x'\nCF_Zone_ID='z1'\nUPGRADE_HASH='abc'\n";
        fs::write(&places.account_conf, saved).unwrap();
        let mut acme = verso(None);
        acme["verso"]["dns"] = json!("dns_dnsimple");
        acme["verso"]["credentials"] = json!(["DNSimple_OAUTH_TOKEN=\"dnsimple_a_new\"", "CF_Zone_ID=\"z1\""]);
        let router = Fake::with(acme);
        deliver(Event::Sync, None, &router, &places).unwrap();
        assert_eq!(
            fs::read_to_string(&places.account_conf).unwrap(),
            "LOG_FILE='/var/log/acme.log'\nSAVED_CF_Token='x'\nCF_Zone_ID='z1'\nUPGRADE_HASH='abc'\n",
            "the stale token goes, an equal value and everything else stay"
        );
    }

    #[test]
    fn nothing_to_forget_leaves_account_conf_untouched() {
        let places = scratch("forget-none");
        let saved = "DNSimple_OAUTH_TOKEN='dnsimple_a_new'\n";
        fs::write(&places.account_conf, saved).unwrap();
        let before = fs::metadata(&places.account_conf).unwrap().modified().unwrap();
        let mut acme = verso(Some("0"));
        acme["verso"]["credentials"] = json!(["DNSimple_OAUTH_TOKEN=\"dnsimple_a_new\""]);
        deliver(Event::Sync, None, &Fake::with(acme), &places).unwrap();
        assert_eq!(fs::read_to_string(&places.account_conf).unwrap(), saved);
        assert_eq!(fs::metadata(&places.account_conf).unwrap().modified().unwrap(), before);
        // No account.conf yet (acme.sh never ran) is nothing to forget.
        let fresh = scratch("forget-absent");
        deliver(Event::Sync, None, &Fake::with(verso(None)), &fresh).unwrap();
        assert!(!fresh.account_conf.exists());
    }

    #[test]
    fn a_disabled_section_still_forgets_a_stale_copy() {
        let places = scratch("forget-off");
        fs::write(&places.account_conf, "CF_Token='old'\n").unwrap();
        let mut acme = verso(Some("0"));
        acme["verso"]["credentials"] = json!(["CF_Token=\"new\""]);
        deliver(Event::Sync, None, &Fake::with(acme), &places).unwrap();
        assert_eq!(fs::read_to_string(&places.account_conf).unwrap(), "");
    }

    #[test]
    fn a_hand_install_turns_an_enabled_section_off_and_leaves_the_rest() {
        for (acme, turned) in [(verso(None), 1), (verso(Some("1")), 1), (verso(Some("0")), 0), (json!({}), 0)] {
            let router = Fake::with(acme);
            hand_over(&router).unwrap();
            assert_eq!(*router.disabled.borrow(), turned);
        }
    }

    #[test]
    fn running_instances_are_read_from_procd() {
        assert_eq!(running_instances(REQUESTING, "acme"), vec!["verso".to_string()]);
        assert_eq!(running_instances(SERVICES, "verso"), vec!["verso".to_string()]);
        assert!(running_instances(SERVICES, "acme").is_empty());
        assert!(running_instances(b"not json", "acme").is_empty());
    }
}
