// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! Network diagnostics run from the router itself: ping, traceroute and a DNS
//! lookup, so what they find is the router's view of the network and not the
//! browser's. A run is a job its operator reads back by cursor while it runs,
//! as the firewall log is read: the helper answers one request with one
//! response, so a live run is polled rather than pushed. Only the three fixed
//! programs ever run, with arguments built here from validated parts (ADR-007):
//! no request names a program or passes a flag.

use crate::Failure;
use serde_json::{json, Value};
use std::collections::HashMap;
use std::io::{BufRead, BufReader, Read};
use std::net::IpAddr;
use std::path::Path;
use std::process::{Child, Command, Stdio};
use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant};

/// A traceroute of thirty silent hops at two seconds each is the longest
/// honest run; past this a run is stuck, not slow.
const RUN_TIMEOUT: Duration = Duration::from_secs(90);
const MAX_LINES: usize = 2000;
const MAX_LINE: usize = 1024;
/// Every operator has at most one run going; this bounds the threads all of
/// them together can hold in the root helper.
const MAX_JOBS: usize = 16;
const KEEP_FINISHED: Duration = Duration::from_secs(600);

/// The argument vector for one run, program first. `interface_exists` is the
/// kernel's say on whether a device name is real, injected so the rules can be
/// tested without one.
pub fn command(
    tool: &str,
    target: &str,
    interface: &str,
    family: &str,
    interface_exists: impl Fn(&str) -> bool,
) -> Result<Vec<String>, Failure> {
    if !valid_target(target) {
        return Err(Failure::invalid("enter a hostname or an IP address"));
    }
    let family = match family {
        "" => None,
        "4" | "6" => Some(family),
        _ => return Err(Failure::invalid("family must be 4 or 6")),
    };
    if let (Ok(address), Some(family)) = (target.parse::<IpAddr>(), family) {
        if (family == "4") != address.is_ipv4() {
            return Err(Failure::invalid(
                "the address is not of the chosen IP family",
            ));
        }
    }
    if !interface.is_empty() && (!valid_interface(interface) || !interface_exists(interface)) {
        return Err(Failure::invalid("no such interface"));
    }
    let mut argv: Vec<String> = match tool {
        "ping" => vec!["/bin/ping", "-c", "5", "-w", "15"],
        "traceroute" => vec!["/bin/traceroute", "-n", "-q", "1", "-w", "2", "-m", "30"],
        "nslookup" => {
            if !interface.is_empty() {
                return Err(Failure::invalid("a lookup is not bound to an interface"));
            }
            vec!["/usr/bin/nslookup"]
        }
        _ => return Err(Failure::invalid("unknown tool")),
    }
    .into_iter()
    .map(String::from)
    .collect();
    match (tool, family) {
        ("nslookup", Some("4")) => argv.push("-type=A".into()),
        ("nslookup", Some(_)) => argv.push("-type=AAAA".into()),
        (_, Some(family)) => argv.push(format!("-{family}")),
        (_, None) => {}
    }
    if !interface.is_empty() {
        argv.push(if tool == "ping" { "-I" } else { "-i" }.into());
        argv.push(interface.into());
    }
    argv.push(target.into());
    Ok(argv)
}

/// A hostname (letters, digits, hyphens and underscores in dot-separated
/// labels, none leading with a hyphen, so it can never read as a flag) or an IP
/// address.
pub fn valid_target(target: &str) -> bool {
    if target.parse::<IpAddr>().is_ok() {
        return true;
    }
    let name = target.strip_suffix('.').unwrap_or(target);
    !name.is_empty()
        && name.len() <= 253
        && name.split('.').all(|label| {
            !label.is_empty()
                && label.len() <= 63
                && !label.starts_with('-')
                && !label.ends_with('-')
                && label
                    .bytes()
                    .all(|b| b.is_ascii_alphanumeric() || b == b'-' || b == b'_')
        })
}

/// A kernel device name: at most fifteen characters, none of them a slash or
/// space, never leading with a hyphen.
pub fn valid_interface(name: &str) -> bool {
    !name.is_empty()
        && name.len() <= 15
        && !name.starts_with('-')
        && name
            .bytes()
            .all(|b| b.is_ascii_alphanumeric() || matches!(b, b'.' | b'_' | b'-'))
}

pub fn interface_exists(name: &str) -> bool {
    Path::new("/sys/class/net").join(name).exists()
}

struct Output {
    lines: Vec<String>,
    /// How many of the oldest lines the cap has let go; a cursor counts them.
    dropped: usize,
    done: bool,
    code: Option<i32>,
    ended: &'static str,
    finished: Option<Instant>,
}

struct Job {
    owner: String,
    output: Mutex<Output>,
    child: Mutex<Option<Child>>,
}

impl Job {
    fn push(&self, line: &[u8]) {
        let mut text = String::from_utf8_lossy(line)
            .trim_end_matches('\r')
            .to_string();
        if text.len() > MAX_LINE {
            let mut end = MAX_LINE;
            while !text.is_char_boundary(end) {
                end -= 1;
            }
            text.truncate(end);
        }
        let mut output = self.output.lock().unwrap_or_else(|e| e.into_inner());
        output.lines.push(text);
        if output.lines.len() > MAX_LINES {
            output.lines.remove(0);
            output.dropped += 1;
        }
    }

    fn running(&self) -> bool {
        !self.output.lock().unwrap_or_else(|e| e.into_inner()).done
    }

    fn stop(&self) {
        {
            let mut output = self.output.lock().unwrap_or_else(|e| e.into_inner());
            if output.done {
                return;
            }
            output.ended = "stopped";
        }
        if let Some(child) = self
            .child
            .lock()
            .unwrap_or_else(|e| e.into_inner())
            .as_mut()
        {
            let _ = child.kill();
        }
    }
}

/// The runs in flight and recently finished, each readable by its operator
/// alone: a run another session started answers as if it did not exist.
#[derive(Default)]
pub struct Jobs {
    jobs: Mutex<HashMap<String, Arc<Job>>>,
}

impl Jobs {
    pub fn start(&self, owner: &str, argv: &[String]) -> Result<Value, Failure> {
        let program = argv
            .first()
            .ok_or_else(|| Failure::invalid("nothing to run"))?;
        if !Path::new(program).exists() {
            let name = Path::new(program)
                .file_name()
                .unwrap_or_default()
                .to_string_lossy();
            return Err(Failure::unknown(format!(
                "{name} is not installed on this router"
            )));
        }
        let mut jobs = self.jobs.lock().unwrap_or_else(|e| e.into_inner());
        jobs.retain(|_, job| {
            let output = job.output.lock().unwrap_or_else(|e| e.into_inner());
            output
                .finished
                .is_none_or(|at| at.elapsed() < KEEP_FINISHED)
        });
        // One run per operator: a new run replaces the one still going.
        for job in jobs.values().filter(|job| job.owner == owner) {
            job.stop();
        }
        if jobs.values().filter(|job| job.running()).count() >= MAX_JOBS {
            return Err(Failure::unknown("too many diagnostics are running"));
        }
        let mut child = Command::new(program)
            .args(&argv[1..])
            .env_clear()
            .env("PATH", "/usr/sbin:/usr/bin:/sbin:/bin")
            .stdin(Stdio::null())
            .stdout(Stdio::piped())
            .stderr(Stdio::piped())
            .spawn()
            .map_err(|error| Failure::unknown(format!("{program}: {error}")))?;
        let streams: Vec<Box<dyn Read + Send>> = vec![
            Box::new(child.stdout.take().expect("piped stdout")),
            Box::new(child.stderr.take().expect("piped stderr")),
        ];
        let job = Arc::new(Job {
            owner: owner.into(),
            output: Mutex::new(Output {
                lines: Vec::new(),
                dropped: 0,
                done: false,
                code: None,
                ended: "exited",
                finished: None,
            }),
            child: Mutex::new(Some(child)),
        });
        let id = random_id()?;
        jobs.insert(id.clone(), Arc::clone(&job));
        drop(jobs);
        std::thread::spawn(move || watch(job, streams));
        Ok(json!({"job": id}))
    }

    pub fn read(&self, owner: &str, id: &str, after: usize) -> Result<Value, Failure> {
        let job = self.owned(owner, id)?;
        let output = job.output.lock().unwrap_or_else(|e| e.into_inner());
        let from = after.saturating_sub(output.dropped).min(output.lines.len());
        Ok(json!({
            "lines": output.lines[from..],
            "next": output.dropped + output.lines.len(),
            "done": output.done,
            "code": output.code,
            "ended": output.ended,
        }))
    }

    pub fn stop(&self, owner: &str, id: &str) -> Result<Value, Failure> {
        self.owned(owner, id)?.stop();
        Ok(json!({"result": true}))
    }

    fn owned(&self, owner: &str, id: &str) -> Result<Arc<Job>, Failure> {
        let jobs = self.jobs.lock().unwrap_or_else(|e| e.into_inner());
        jobs.get(id)
            .filter(|job| job.owner == owner)
            .cloned()
            .ok_or_else(|| Failure::invalid("no such run"))
    }
}

/// Collects a run's lines as it writes them, ends it at the deadline, and
/// marks it done only once every line it wrote is in.
fn watch(job: Arc<Job>, streams: Vec<Box<dyn Read + Send>>) {
    let readers: Vec<_> = streams
        .into_iter()
        .map(|stream| {
            let job = Arc::clone(&job);
            std::thread::spawn(move || {
                for line in BufReader::new(stream).split(b'\n').map_while(Result::ok) {
                    job.push(&line);
                }
            })
        })
        .collect();
    let deadline = Instant::now() + RUN_TIMEOUT;
    let code = loop {
        {
            let mut slot = job.child.lock().unwrap_or_else(|e| e.into_inner());
            let Some(child) = slot.as_mut() else {
                break None;
            };
            match child.try_wait() {
                Ok(Some(status)) => {
                    slot.take();
                    break status.code();
                }
                Ok(None) if Instant::now() >= deadline => {
                    let _ = child.kill();
                    let _ = child.wait();
                    slot.take();
                    job.output.lock().unwrap_or_else(|e| e.into_inner()).ended = "timeout";
                    break None;
                }
                Ok(None) => {}
                Err(_) => {
                    slot.take();
                    break None;
                }
            }
        }
        std::thread::sleep(Duration::from_millis(100));
    };
    for reader in readers {
        let _ = reader.join();
    }
    let mut output = job.output.lock().unwrap_or_else(|e| e.into_inner());
    output.done = true;
    output.code = code;
    output.finished = Some(Instant::now());
}

fn random_id() -> Result<String, Failure> {
    let mut bytes = [0u8; 12];
    std::fs::File::open("/dev/urandom")
        .and_then(|mut source| source.read_exact(&mut bytes))
        .map_err(|error| Failure::unknown(format!("random id: {error}")))?;
    Ok(bytes.iter().map(|b| format!("{b:02x}")).collect())
}

#[cfg(test)]
mod tests {
    use super::*;

    fn argv(
        tool: &str,
        target: &str,
        interface: &str,
        family: &str,
    ) -> Result<Vec<String>, Failure> {
        command(tool, target, interface, family, |name| name == "br-lan")
    }

    fn words(v: &[&str]) -> Vec<String> {
        v.iter().map(|s| s.to_string()).collect()
    }

    #[test]
    fn each_tool_runs_its_fixed_program() {
        assert_eq!(
            argv("ping", "example.com", "", "").unwrap(),
            words(&["/bin/ping", "-c", "5", "-w", "15", "example.com"])
        );
        assert_eq!(
            argv("traceroute", "1.1.1.1", "br-lan", "4").unwrap(),
            words(&[
                "/bin/traceroute",
                "-n",
                "-q",
                "1",
                "-w",
                "2",
                "-m",
                "30",
                "-4",
                "-i",
                "br-lan",
                "1.1.1.1"
            ])
        );
        assert_eq!(
            argv("ping", "2606:4700::1111", "br-lan", "6").unwrap(),
            words(&[
                "/bin/ping",
                "-c",
                "5",
                "-w",
                "15",
                "-6",
                "-I",
                "br-lan",
                "2606:4700::1111"
            ])
        );
        assert_eq!(
            argv("nslookup", "openwrt.org", "", "6").unwrap(),
            words(&["/usr/bin/nslookup", "-type=AAAA", "openwrt.org"])
        );
        assert_eq!(
            argv("nslookup", "openwrt.org", "", "4").unwrap(),
            words(&["/usr/bin/nslookup", "-type=A", "openwrt.org"])
        );
    }

    #[test]
    fn nothing_from_the_request_can_become_a_flag_or_a_program() {
        for target in [
            "-f",
            "--help",
            "a b",
            "a;reboot",
            "$(reboot)",
            "",
            "a..b",
            "-x.com",
            "x-.com",
        ] {
            assert!(argv("ping", target, "", "").is_err(), "target {target:?}");
        }
        for interface in ["-i", "../../etc", "br lan", "eth0;x", "abcdefghijklmnop"] {
            assert!(
                argv("ping", "1.1.1.1", interface, "").is_err(),
                "interface {interface:?}"
            );
        }
        assert!(
            argv("ping", "1.1.1.1", "eth9", "").is_err(),
            "an interface the kernel lacks"
        );
        assert!(argv("sh", "1.1.1.1", "", "").is_err());
        assert!(argv("ping", "1.1.1.1", "", "5").is_err());
    }

    #[test]
    fn an_address_must_match_its_family_and_a_lookup_takes_no_interface() {
        assert!(argv("ping", "1.1.1.1", "", "6").is_err());
        assert!(argv("ping", "::1", "", "4").is_err());
        assert!(argv("ping", "example.com", "", "6").is_ok());
        assert!(argv("nslookup", "example.com", "br-lan", "").is_err());
    }

    #[test]
    fn hostnames_and_addresses_are_targets() {
        for target in [
            "example.com",
            "example.com.",
            "_dmarc.example.com",
            "localhost",
            "10.0.0.1",
            "fe80::1",
        ] {
            assert!(valid_target(target), "{target}");
        }
    }

    fn wait(jobs: &Jobs, owner: &str, id: &str) -> Value {
        for _ in 0..100 {
            let read = jobs.read(owner, id, 0).unwrap();
            if read["done"] == true {
                return read;
            }
            std::thread::sleep(Duration::from_millis(50));
        }
        panic!("run never finished");
    }

    #[test]
    fn a_run_is_read_back_by_cursor_and_only_by_its_operator() {
        let jobs = Jobs::default();
        let started = jobs.start("alice", &words(&["/bin/echo", "one"])).unwrap();
        let id = started["job"].as_str().unwrap().to_string();
        let read = wait(&jobs, "alice", &id);
        assert_eq!(read["lines"], json!(["one"]));
        assert_eq!(read["next"], 1);
        assert_eq!(read["code"], 0);
        assert_eq!(read["ended"], "exited");
        let after = jobs.read("alice", &id, 1).unwrap();
        assert_eq!(after["lines"], json!([]));
        assert!(
            jobs.read("mallory", &id, 0).is_err(),
            "another session sees no run"
        );
        assert!(jobs.stop("mallory", &id).is_err());
    }

    #[test]
    fn a_stopped_run_says_so_and_a_new_run_replaces_the_last() {
        let jobs = Jobs::default();
        let first = jobs.start("alice", &words(&["/bin/sleep", "30"])).unwrap();
        let first = first["job"].as_str().unwrap().to_string();
        let second = jobs.start("alice", &words(&["/bin/sleep", "30"])).unwrap();
        let second = second["job"].as_str().unwrap().to_string();
        assert_eq!(wait(&jobs, "alice", &first)["ended"], "stopped");
        jobs.stop("alice", &second).unwrap();
        let read = wait(&jobs, "alice", &second);
        assert_eq!(read["ended"], "stopped");
        assert_eq!(read["code"], Value::Null);
    }

    #[test]
    fn a_program_the_router_lacks_is_named() {
        let jobs = Jobs::default();
        let failure = jobs
            .start("alice", &words(&["/nonexistent/traceroute"]))
            .unwrap_err();
        assert_eq!(
            failure.message,
            "traceroute is not installed on this router"
        );
    }

    #[test]
    fn the_oldest_lines_give_way_and_the_cursor_counts_them() {
        let job = Job {
            owner: "alice".into(),
            output: Mutex::new(Output {
                lines: Vec::new(),
                dropped: 0,
                done: false,
                code: None,
                ended: "exited",
                finished: None,
            }),
            child: Mutex::new(None),
        };
        for n in 0..MAX_LINES + 5 {
            job.push(n.to_string().as_bytes());
        }
        let output = job.output.lock().unwrap();
        assert_eq!(output.dropped, 5);
        assert_eq!(output.lines[0], "5");
        drop(output);
        job.push(&[b'x'; MAX_LINE + 10]);
        assert_eq!(
            job.output.lock().unwrap().lines.last().unwrap().len(),
            MAX_LINE
        );
    }
}
