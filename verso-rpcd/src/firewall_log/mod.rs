// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! Packet logging has its own bounded RAM history. It never writes packet
//! contents to syslog or disk, and reads still pass the helper's SID gate.

mod netlink;
mod route;
#[cfg(test)]
mod tests;

use crate::Failure;
use serde_json::{json, Value};
use std::collections::VecDeque;
use std::sync::{Arc, Mutex};
use std::time::{Duration, SystemTime, UNIX_EPOCH};

const GROUP: u16 = 4242;
const SNAPLEN: u32 = 256;
const CAPACITY: usize = 2048;
const MAX_MESSAGE: usize = 1024;

struct Entry {
    id: i64,
    time: u64,
    message: String,
}

struct Buffer {
    entries: VecDeque<Entry>,
    capacity: usize,
    next: i64,
    generation: String,
    ready: bool,
    error: String,
    overruns: u64,
}

impl Buffer {
    fn new(capacity: usize) -> Self {
        Self {
            entries: VecDeque::new(),
            capacity,
            next: 1,
            generation: format!("{}-{}", std::process::id(), now_nanos()),
            ready: false,
            error: "Packet collector is starting".into(),
            overruns: 0,
        }
    }
    fn push(&mut self, time: u64, mut message: String) {
        if message.len() > MAX_MESSAGE {
            let mut end = MAX_MESSAGE;
            while !message.is_char_boundary(end) {
                end -= 1;
            }
            message.truncate(end);
        }
        if self.entries.len() == self.capacity {
            self.entries.pop_front();
        }
        self.entries.push_back(Entry {
            id: self.next,
            time,
            message,
        });
        self.next += 1;
    }
    fn read(&self, generation: &str, after: i64, limit: usize) -> Value {
        let reset = generation != self.generation;
        let after = if reset { -1 } else { after };
        let entries: Vec<Value> = self
            .entries
            .iter()
            .rev()
            .filter(|e| e.id > after)
            .take(limit.clamp(1, 500))
            .collect::<Vec<_>>()
            .into_iter()
            .rev()
            .map(|e| json!({"id":e.id,"time":e.time,"msg":e.message,"source":0,"priority":4}))
            .collect();
        let lost = !reset
            && after >= 0
            && entries
                .first()
                .and_then(|e| e["id"].as_i64())
                .is_some_and(|id| id.saturating_sub(after) > 1);
        json!({"entries":entries,"generation":self.generation,"available":self.ready,
            "error":self.error,"reset":reset,"lost":lost,"overruns":self.overruns})
    }
}

fn now_nanos() -> u128 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_nanos()
}

#[derive(Clone)]
pub struct Collector {
    buffer: Arc<Mutex<Buffer>>,
}

impl Collector {
    pub fn new() -> Self {
        Self {
            buffer: Arc::new(Mutex::new(Buffer::new(CAPACITY))),
        }
    }
    pub fn start(&self) {
        let collector = self.clone();
        std::thread::spawn(move || {
            let mut reported = String::new();
            loop {
                let result = collector.collect();
                let error = result
                    .err()
                    .unwrap_or_else(|| "Packet collector stopped".into());
                if let Ok(mut buffer) = collector.buffer.lock() {
                    buffer.ready = false;
                    buffer.error = error.clone();
                }
                if error != reported {
                    eprintln!("verso-rpcd: firewall packet collector: {error}");
                    reported = error;
                }
                std::thread::sleep(Duration::from_secs(5));
            }
        });
    }
    fn collect(&self) -> Result<(), String> {
        let socket = netlink::Socket::open()?;
        {
            let mut buffer = self
                .buffer
                .lock()
                .map_err(|_| "packet buffer lock poisoned")?;
            buffer.ready = true;
            buffer.error.clear();
        }
        let mut bytes = vec![0u8; 65536];
        loop {
            match socket.receive(&mut bytes) {
                Ok(length) => {
                    let entries = netlink::decode(&bytes[..length]);
                    let mut buffer = self
                        .buffer
                        .lock()
                        .map_err(|_| "packet buffer lock poisoned")?;
                    for (time, message) in entries {
                        buffer.push(time, message);
                    }
                }
                Err(error) if error.raw_os_error() == Some(105) => {
                    if let Ok(mut buffer) = self.buffer.lock() {
                        buffer.overruns += 1;
                    }
                }
                Err(error) if error.kind() == std::io::ErrorKind::Interrupted => continue,
                Err(error) => return Err(error.to_string()),
            }
        }
    }
    pub fn read(&self, args: &Value) -> Result<Value, Failure> {
        let generation = args["generation"].as_str().unwrap_or("");
        let after = args["after"]
            .as_str()
            .unwrap_or("-1")
            .parse::<i64>()
            .map_err(|_| Failure::invalid("invalid packet log cursor"))?;
        let limit = args["limit"]
            .as_str()
            .unwrap_or("200")
            .parse::<usize>()
            .map_err(|_| Failure::invalid("invalid packet log limit"))?;
        let buffer = self
            .buffer
            .lock()
            .map_err(|_| Failure::unknown("packet buffer lock poisoned"))?;
        let mut result = buffer.read(generation, after, limit);
        // Root-owned boot/reload scripts record routing failure outside the
        // group-writable Verso runtime directory. Other helper verbs keep working.
        if std::path::Path::new("/var/run/verso-firewall-logging.failed")
            .try_exists()
            .unwrap_or(true)
        {
            result["available"] = json!(false);
            result["error"] = json!("Firewall log routing failed; see system logs");
        }
        Ok(result)
    }
}

// Local boot/package automation only. There is no corresponding remote write
// method: a shell or plugin cannot edit firewall templates through the helper.
pub fn command_line() -> bool {
    let args: Vec<_> = std::env::args().skip(1).collect();
    let Some(mode) = args.first() else {
        return false;
    };
    if !matches!(
        mode.as_str(),
        "--configure-firewall-logging" | "--restore-firewall-logging" | "--route-firewall-logging"
    ) {
        return false;
    }
    unsafe extern "C" {
        fn geteuid() -> u32;
    }
    if args.len() != 1 || unsafe { geteuid() } != 0 {
        eprintln!("verso-rpcd: firewall logging setup requires local root");
        std::process::exit(2);
    }
    let result = match mode.as_str() {
        "--configure-firewall-logging" => route::configure(true),
        "--restore-firewall-logging" => route::configure(false).and_then(|_| route::apply(false)),
        _ => route::configure(true).and_then(|_| route::apply(true)),
    };
    if let Err(error) = result {
        eprintln!("verso-rpcd: firewall logging setup: {}", error.message);
        std::process::exit(1);
    }
    true
}
