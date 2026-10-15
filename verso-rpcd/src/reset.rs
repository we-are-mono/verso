// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! Factory reset, and with it, unless the operator keeps it, the router's
//! history (ADR-021). `firstboot` clears the overlay and leaves /data, so the
//! history directory (config history, events, charts) and nlbwmon's
//! per-device usage there would outlive a reset: a router handed on would
//! carry its past to its next owner.
//!
//! A reset empties every place the history can be found in after it: the
//! defaults, which the shell and nlbwmon open again once `firstboot` has put
//! their configs back, whatever the operator chose before, and the directories
//! the operator chose, the one set and the one the running shell was started
//! with, each reached as the shell reaches it. A default that is a directory
//! of its own, reached through no link and no mount's root, is the history's
//! alone and is emptied whole. Anywhere else the history shares its directory
//! with whatever its owner keeps there, so only the files bearing the names its
//! writer gives them are removed: the option is the shell's to set, and a
//! shell that could have root empty a directory it names could have root
//! empty any. Each directory is emptied held open, as the one reached, so
//! what is moved there after it was reached is left as it is.

use std::ffi::{OsStr, OsString};
use std::fs;
use std::io::ErrorKind;
use std::os::fd::AsRawFd;
use std::os::unix::ffi::{OsStrExt, OsStringExt};
use std::os::unix::fs::{MetadataExt, OpenOptionsExt};
use std::path::{Component, Path, PathBuf};
use std::process::Command;
use std::time::{Duration, Instant};

use serde_json::{json, Value};

use crate::Failure;

/// Names says whether a file bears a name a writer of the history gives the
/// files it keeps.
type Names = fn(&OsStr) -> bool;

/// Where history is kept without a choice of the operator's, on a router
/// with /data and on one without: the shell's, and nlbwmon's as usage-setup
/// sets it, each with the names its writer gives its files.
const DEFAULTS: [(&str, Names); 4] = [
    ("/data/verso", shells),
    ("/usr/share/verso/history", shells),
    ("/data/nlbwmon", nlbwmons),
    ("/usr/share/nlbwmon/db", nlbwmons),
];

/// How many links a path is followed through before its open fails, as the
/// kernel's MAXSYMLINKS.
const LINKS: usize = 40;

/// The services that write the history: the shell, which marks its stop,
/// and nlbwmon, which saves the day's usage as it stops.
const WRITERS: [&str; 2] = ["verso", "nlbwmon"];

/// The files the shell keeps in its history directory: the database, the
/// files SQLite keeps beside it, and the mark of a clean stop.
const SHELL_FILES: [&str; 5] = [
    "history.db",
    "history.db-wal",
    "history.db-shm",
    "history.db-journal",
    "stopped",
];

const FIRSTBOOT: &str = "/sbin/firstboot";
const SHELL: &str = "/usr/bin/verso";

/// How long a stopped service's processes are waited for, and those killed
/// then: procd sends SIGTERM and, five seconds on, SIGKILL to an instance,
/// but not to what the instance started, the shell below its jail.
const GONE_WITHIN: Duration = Duration::from_secs(10);
const POLL: Duration = Duration::from_millis(100);
const SIGKILL: i32 = 9;

unsafe extern "C" {
    #[link_name = "kill"]
    fn signal_process(pid: i32, signal: i32) -> i32;
}

/// start resets the router to its defaults, after emptying its history if
/// erase is set. The history is emptied before the answer, so an erase that
/// fails is answered and the reset never starts; the reset itself runs after
/// the answer, since it stops the shell that asked and restarts the router.
pub fn start(erase: bool) -> Result<(), Failure> {
    if !erase {
        return crate::spawn_system_action(FIRSTBOOT, &["-r", "-y"]);
    }
    // A directory applied while the shell runs reaches it as a SIGHUP, not
    // a restart: it writes on where it was started until it stops.
    let chosen = uci_get("verso.history.directory")
        .map(PathBuf::from)
        .into_iter()
        .chain(Device.running_history())
        .collect();
    let history = History {
        root: PathBuf::from("/"),
        shell: crate::VERSO_UID,
        superuser: 0,
        chosen,
        device,
        mounts: mounted,
    };
    reset(history, Device, GONE_WITHIN)
}

/// reset empties the history, then, once the answer is out, stops the shell
/// and nlbwmon and waits until they are gone, empties it again and runs
/// firstboot. The shell writes until it stops and both write as they stop,
/// after procd has signalled them, so the second emptying waits for their
/// exit and takes what came after the first. Past the answer nothing is held
/// back: every place that can be emptied is, and the others are logged. A
/// firstboot that returns has not restarted the router, which is left with
/// the services that ran running again.
fn reset(history: History, router: impl Router, within: Duration) -> Result<(), Failure> {
    history.erase().map_err(Failure::invalid)?;
    let then = router.clone();
    router.after_answer(Box::new(move || {
        let ran: Vec<&str> = WRITERS
            .into_iter()
            .filter(|service| stop(&then, service, within))
            .collect();
        for error in history.erase_what_it_can() {
            eprintln!("verso-rpcd: reset: {error}");
        }
        if let Some(outcome) = then.firstboot() {
            eprintln!(
                "verso-rpcd: reset: firstboot returned ({outcome}) without restarting the router"
            );
            for service in ran {
                then.start(service);
            }
        }
    }));
    Ok(())
}

/// stop stops a service and returns once every process it had is gone, or
/// killed after within, saying whether it may have run: it had processes, or
/// they could not be read. Killing loses only what it would still write,
/// history the reset erases; a write landing after the second emptying is
/// what must not happen.
fn stop(router: &impl Router, service: &str, within: Duration) -> bool {
    let processes = router.processes(service);
    router.stop(service);
    let ran = processes.as_ref().is_none_or(|found| !found.is_empty());
    let processes = processes.unwrap_or_default();
    if gone(router, &processes, within) {
        return ran;
    }
    for process in processes.iter().filter(|p| router.running(p)) {
        eprintln!(
            "verso-rpcd: reset: {service} process {} outlived its stop; killing it",
            process.pid
        );
        router.kill(process);
    }
    if !gone(router, &processes, within) {
        eprintln!("verso-rpcd: reset: {service} still runs after a kill");
    }
    ran
}

/// gone waits up to within for every process to exit, and says whether all
/// did.
fn gone(router: &impl Router, processes: &[Process], within: Duration) -> bool {
    let until = Instant::now() + within;
    loop {
        if !processes.iter().any(|p| router.running(p)) {
            return true;
        }
        if Instant::now() >= until {
            return false;
        }
        std::thread::sleep(POLL);
    }
}

/// Work is what a reset does once it has answered.
type Work = Box<dyn FnOnce() + Send>;

/// Process is one running program: its id, and when it began, which tells
/// it from a later one given the same id.
#[derive(Clone, Copy, Debug, PartialEq)]
struct Process {
    pid: u32,
    began: u64,
}

/// Router is what a reset does beyond the history's files.
trait Router: Clone + Send + 'static {
    /// after_answer runs work once the answer has had time to reach the shell.
    fn after_answer(&self, work: Work);
    /// processes are a service's processes: procd's instances and theirs, or
    /// None when procd cannot be asked.
    fn processes(&self, service: &str) -> Option<Vec<Process>>;
    /// stop asks procd to stop a service. procd signals it and returns: the
    /// service may still be writing.
    fn stop(&self, service: &str);
    /// running says whether a process has yet to exit.
    fn running(&self, process: &Process) -> bool;
    /// kill ends a process at once.
    fn kill(&self, process: &Process);
    /// firstboot restores the defaults and restarts the router. It returns
    /// only when the router is not restarting, with what came of it.
    fn firstboot(&self) -> Option<String>;
    /// start starts a service again.
    fn start(&self, service: &str);
}

#[derive(Clone)]
struct Device;

impl Device {
    /// running_history is the directory the running shell was started with.
    fn running_history(&self) -> Option<PathBuf> {
        self.processes("verso")
            .into_iter()
            .flatten()
            .find_map(|process| {
                shell_history(&fs::read(format!("/proc/{}/cmdline", process.pid)).ok()?)
            })
    }
}

/// shell_history is the directory a process's /proc cmdline names, if the
/// process is the shell. Each argument ends in a NUL, an empty one too.
fn shell_history(cmdline: &[u8]) -> Option<PathBuf> {
    let cmdline = cmdline.strip_suffix(b"\0").unwrap_or(cmdline);
    let args: Vec<&OsStr> = cmdline
        .split(|byte| *byte == 0)
        .map(OsStr::from_bytes)
        .collect();
    let (program, args) = args.split_first()?;
    if *program != SHELL {
        return None;
    }
    history_flag(args)
}

impl Router for Device {
    fn after_answer(&self, work: Work) {
        std::thread::spawn(move || {
            // Long enough for the answer to reach the shell before it stops.
            std::thread::sleep(Duration::from_secs(1));
            work();
        });
    }

    fn processes(&self, service: &str) -> Option<Vec<Process>> {
        let output = Command::new("/bin/ubus")
            .args([
                "call",
                "service",
                "list",
                &json!({"name": service}).to_string(),
            ])
            .output()
            .ok()
            .filter(|output| output.status.success())?;
        let roots = instance_pids(&output.stdout, service)?;
        let table: Vec<(u32, Stat)> = fs::read_dir("/proc")
            .into_iter()
            .flatten()
            .flatten()
            .filter_map(|entry| {
                let pid = entry.file_name().to_str()?.parse().ok()?;
                Some((pid, read_stat(pid)?))
            })
            .collect();
        Some(family(&roots, &table))
    }

    fn stop(&self, service: &str) {
        let _ = Command::new(format!("/etc/init.d/{service}"))
            .arg("stop")
            .status();
    }

    fn running(&self, process: &Process) -> bool {
        read_stat(process.pid).is_some_and(|stat| stat.began == process.began && !stat.zombie)
    }

    fn kill(&self, process: &Process) {
        if self.running(process) {
            if let Ok(pid) = i32::try_from(process.pid) {
                // SAFETY: kill(2) takes plain integers and touches no memory.
                unsafe { signal_process(pid, SIGKILL) };
            }
        }
    }

    fn firstboot(&self) -> Option<String> {
        // On success firstboot restarts the router and never returns.
        Some(match Command::new(FIRSTBOOT).args(["-r", "-y"]).status() {
            Ok(status) => status.to_string(),
            Err(error) => error.to_string(),
        })
    }

    fn start(&self, service: &str) {
        let _ = Command::new(format!("/etc/init.d/{service}"))
            .arg("start")
            .status();
    }
}

/// Stat is what a process's /proc stat says that a reset reads.
#[derive(Debug, PartialEq)]
struct Stat {
    parent: u32,
    began: u64,
    zombie: bool,
}

fn read_stat(pid: u32) -> Option<Stat> {
    stat(&fs::read_to_string(format!("/proc/{pid}/stat")).ok()?)
}

/// stat reads a /proc stat line. Its fields follow the program's name, in
/// parentheses, which may hold anything, a ')' too.
fn stat(line: &str) -> Option<Stat> {
    let (_, rest) = line.rsplit_once(')')?;
    let fields: Vec<&str> = rest.split_whitespace().collect();
    Some(Stat {
        zombie: *fields.first()? == "Z",
        parent: fields.get(1)?.parse().ok()?,
        began: fields.get(19)?.parse().ok()?,
    })
}

/// instance_pids reads the running instances' ids from `ubus call service
/// list` for one service, or None for an answer it cannot read. procd answers
/// with no entry for a service it does not know.
fn instance_pids(answer: &[u8], service: &str) -> Option<Vec<u32>> {
    let answer = serde_json::from_slice::<Value>(answer).ok()?;
    if !answer.is_object() {
        return None;
    }
    let Some(instances) = answer[service]["instances"].as_object() else {
        return Some(Vec::new());
    };
    Some(
        instances
            .values()
            .filter(|instance| instance["running"].as_bool() == Some(true))
            .filter_map(|instance| u32::try_from(instance["pid"].as_u64()?).ok())
            .collect(),
    )
}

/// family is the processes of roots and every one they started, below them.
fn family(roots: &[u32], table: &[(u32, Stat)]) -> Vec<Process> {
    let mut found: Vec<Process> = table
        .iter()
        .filter(|(pid, _)| roots.contains(pid))
        .map(|(pid, stat)| Process {
            pid: *pid,
            began: stat.began,
        })
        .collect();
    let mut next = 0;
    while next < found.len() {
        let parent = found[next].pid;
        for (pid, stat) in table {
            if stat.parent == parent && !found.iter().any(|p| p.pid == *pid) {
                found.push(Process {
                    pid: *pid,
                    began: stat.began,
                });
            }
        }
        next += 1;
    }
    found
}

/// history_flag is the directory a shell's arguments name, as Go's flag
/// package reads them. Flags, with one dash or two, end at "--" or at the
/// first argument that is none. Every flag the shell has takes a value, after
/// '=' or as the next argument, whatever that looks like. The last
/// -history-dir counts, and an empty one names none; arguments the shell
/// refuses to start with name none either.
fn history_flag(args: &[&OsStr]) -> Option<PathBuf> {
    let mut dir = None;
    let mut args = args.iter();
    while let Some(arg) = args.next() {
        let name = match arg.as_bytes() {
            b"--" => break,
            [b'-', b'-', name @ ..] | [b'-', name @ ..] if !name.is_empty() => name,
            _ => break,
        };
        if name.starts_with(b"-") || name.starts_with(b"=") {
            return None;
        }
        let (name, value) = match name.iter().position(|byte| *byte == b'=') {
            Some(at) => (&name[..at], &name[at + 1..]),
            None => (name, args.next()?.as_bytes()),
        };
        if name == b"history-dir" {
            dir = (!value.is_empty()).then(|| PathBuf::from(OsStr::from_bytes(value)));
        }
    }
    dir
}

/// History is where a reset finds the router's history: below root, "/" on
/// the router, kept by the shell's account or the superuser's, and in the
/// directories the operator chose, if any.
struct History {
    root: PathBuf,
    shell: u32,
    superuser: u32,
    chosen: Vec<PathBuf>,
    /// device says which device a path is on.
    device: fn(&Path) -> std::io::Result<u64>,
    /// mounts are the places filesystems are mounted at.
    mounts: fn() -> std::io::Result<Vec<PathBuf>>,
}

/// Emptying is how a directory reached is emptied.
#[derive(Clone, Copy)]
enum Emptying {
    /// Of everything: a default that is the history's alone.
    Whole,
    /// Of the files bearing its writer's names: a directory the history
    /// shares with whatever else its owner keeps there.
    Named(Names),
}

/// Reached is a directory below root as the shell's open reaches it: its
/// path, the directory it was then, by device and inode, its owner, and
/// whether it is its own, to empty whole.
struct Reached {
    dir: PathBuf,
    id: (u64, u64),
    owner: u32,
    own: bool,
}

/// Found is a place to empty: the path it is named by, the directory reached
/// there and how that is emptied.
struct Found {
    place: PathBuf,
    reached: Reached,
    emptying: Emptying,
}

impl History {
    /// erase empties every place and keeps the directories. Each is reached
    /// before any is emptied, so a place that cannot be reached leaves the
    /// history whole, and the first place that fails is the answer.
    fn erase(&self) -> Result<(), String> {
        let mounts = (self.mounts)().map_err(unreadable_mounts)?;
        let found: Vec<Found> = self.find(&mounts).into_iter().collect::<Result<_, _>>()?;
        found.iter().try_for_each(|found| found.empty(&mounts))
    }

    /// erase_what_it_can empties every place it can, and says why each of
    /// the others could not be.
    fn erase_what_it_can(&self) -> Vec<String> {
        let mounts = match (self.mounts)() {
            Ok(mounts) => mounts,
            Err(error) => return vec![unreadable_mounts(error)],
        };
        self.find(&mounts)
            .into_iter()
            .filter_map(|found| found.and_then(|found| found.empty(&mounts)).err())
            .collect()
    }

    /// find reaches every place, the defaults, then the chosen directories:
    /// each the directory reached there and how it is emptied, or why it
    /// cannot be. A default that is a directory of its own, reached through
    /// no link and no mount's root, and the shell's or the superuser's (a
    /// sysupgrade that keeps settings makes the shell's again as root's), is
    /// emptied whole. Any other default and every chosen directory, whoever
    /// owns it, loses only the files bearing its writer's names: the shell
    /// writes in a directory of another account's that lets it, a stick's
    /// root say, beside the owner's own files. A chosen directory inside a
    /// default emptied whole is gone by its turn, and one a link there led
    /// to is emptied where it leads.
    fn find(&self, mounts: &[PathBuf]) -> Vec<Result<Found, String>> {
        let defaults = DEFAULTS
            .into_iter()
            .map(|(place, names)| (PathBuf::from(place), names, true));
        let chosen = self
            .chosen
            .iter()
            .map(|place| (place.clone(), shells as Names, false));
        defaults
            .chain(chosen)
            .filter_map(|(place, names, default)| {
                let reached = match self.reach(&place, mounts) {
                    Ok(reached) => reached?,
                    Err(error) => return Some(Err(refused(&place, &error))),
                };
                let ours = reached.owner == self.shell || reached.owner == self.superuser;
                let emptying = if default && reached.own && ours {
                    Emptying::Whole
                } else {
                    Emptying::Named(names)
                };
                Some(Ok(Found {
                    place,
                    reached,
                    emptying,
                }))
            })
            .collect()
    }

    /// reach finds the directory the shell's open reaches at path: path made
    /// clean as Go's filepath.Clean makes it, from the root where it is
    /// relative (procd starts the shell there), then followed name by name
    /// as the kernel follows it, each link to where it leads. None is where
    /// the shell's open fails too: a name absent or too long, a file where a
    /// directory must be, or too many links.
    ///
    /// It is its own, to empty whole, only when reached through no link, no
    /// mount's root, by its device or the mount table, which shows a bind
    /// mount on the same filesystem too, and in a directory only the
    /// superuser may change, where no other account can have put it. The
    /// directory it was is emptied, held open, whatever its path names by
    /// then, so one reached through a link or in a directory others may
    /// rearrange loses no more than its writer's files.
    fn reach(&self, path: &Path, mounts: &[PathBuf]) -> Result<Option<Reached>, String> {
        let mut at = self.root.clone();
        let mut ahead: Vec<OsString> = names(&cleaned(path)).rev().collect();
        let mut links = 0;
        while let Some(name) = ahead.pop() {
            if name == ".." {
                if at != self.root {
                    at.pop();
                }
                continue;
            }
            let next = at.join(&name);
            let meta = match fs::symlink_metadata(&next) {
                Ok(meta) => meta,
                Err(error) if fails_the_open(&error) => return Ok(None),
                Err(error) => return Err(error.to_string()),
            };
            if meta.is_dir() {
                at = next;
                continue;
            }
            if !meta.file_type().is_symlink() {
                return Ok(None);
            }
            links += 1;
            if links > LINKS {
                return Ok(None);
            }
            let target = fs::read_link(&next).map_err(|e| e.to_string())?;
            if target.has_root() {
                at = self.root.clone();
            }
            ahead.extend(names(&target).rev());
        }
        let above = match at.parent() {
            Some(above) if at != self.root => above,
            _ => &self.root,
        };
        let device = |dir: &Path| (self.device)(dir).map_err(|e| e.to_string());
        let mount = device(&at)? != device(above)? || mounts.contains(&at);
        let meta = fs::symlink_metadata(&at).map_err(|e| e.to_string())?;
        Ok(Some(Reached {
            own: links == 0 && !mount && self.root_only(above)?,
            id: (meta.dev(), meta.ino()),
            owner: meta.uid(),
            dir: at,
        }))
    }

    /// root_only says whether only the superuser may change what dir holds.
    fn root_only(&self, dir: &Path) -> Result<bool, String> {
        let meta = fs::metadata(dir).map_err(|e| e.to_string())?;
        Ok(meta.uid() == self.superuser && meta.mode() & 0o022 == 0)
    }
}

impl Found {
    /// empty empties the directory reached, held open, so what is removed
    /// is removed there whatever its path names by then. One gone since was
    /// inside a default emptied whole.
    fn empty(&self, mounts: &[PathBuf]) -> Result<(), String> {
        let dir = &self.reached.dir;
        let emptied = match Pinned::open(dir, self.reached.id) {
            Ok(None) => Ok(()),
            Ok(Some(open)) => match self.emptying {
                Emptying::Whole => clear(&open, dir, mounts).map(drop),
                Emptying::Named(names) => remove_files(&open, dir, names),
            },
            Err(error) => Err(error),
        };
        emptied.map_err(|error| refused(&self.place, &error))
    }
}

/// refused says why the history in a place cannot be emptied.
fn refused(place: &Path, error: &str) -> String {
    format!(
        "the history in {} cannot be emptied: {error}",
        place.display()
    )
}

fn unreadable_mounts(error: std::io::Error) -> String {
    format!("the history cannot be found: the mount table cannot be read: {error}")
}

/// Pinned is a directory held open. Its names are reached through the open,
/// in /proc/self/fd, so what is removed in it is removed there, whatever its
/// path names by then.
struct Pinned {
    open: fs::File,
    dev: u64,
}

impl Pinned {
    /// open opens the directory at path, never through a link at its last
    /// name, and holds it only if it is the directory id names, the one
    /// reached there. None is one gone since.
    fn open(path: &Path, id: (u64, u64)) -> Result<Option<Self>, String> {
        let open = match fs::OpenOptions::new()
            .read(true)
            .custom_flags(crate::O_DIRECTORY | crate::O_NOFOLLOW)
            .open(path)
        {
            Ok(open) => open,
            Err(error) if error.kind() == ErrorKind::NotFound => return Ok(None),
            Err(error) => return Err(error.to_string()),
        };
        let meta = open.metadata().map_err(|e| e.to_string())?;
        if (meta.dev(), meta.ino()) != id {
            return Err("it was replaced after it was reached".into());
        }
        Ok(Some(Self {
            open,
            dev: meta.dev(),
        }))
    }

    /// here is the directory, through its open.
    fn here(&self) -> PathBuf {
        PathBuf::from(format!("/proc/self/fd/{}", self.open.as_raw_fd()))
    }

    /// at is a name in the directory, through its open.
    fn at(&self, name: &OsStr) -> PathBuf {
        self.here().join(name)
    }

    /// names are the names the directory holds.
    fn names(&self) -> std::io::Result<Vec<OsString>> {
        fs::read_dir(self.here())?
            .map(|entry| entry.map(|entry| entry.file_name()))
            .collect()
    }
}

/// cleaned is path as Go's filepath.Clean leaves it, from the root: "." gone,
/// ".." taking the name before it, or none at the root.
fn cleaned(path: &Path) -> PathBuf {
    let mut kept: Vec<&OsStr> = Vec::new();
    for component in path.components() {
        match component {
            Component::Normal(name) => kept.push(name),
            Component::ParentDir => {
                kept.pop();
            }
            _ => {}
        }
    }
    kept.into_iter()
        .fold(PathBuf::from("/"), |path, name| path.join(name))
}

/// names are the names a path goes through, ".." among them; "." is none.
fn names(path: &Path) -> impl DoubleEndedIterator<Item = OsString> + '_ {
    path.components().filter_map(|component| match component {
        Component::Normal(name) => Some(name.to_owned()),
        Component::ParentDir => Some("..".into()),
        _ => None,
    })
}

/// fails_the_open says whether what a name's lstat met fails the shell's
/// open there too, so nothing was written there. Its directory is reached
/// through no link, so too many links are reach's to count.
fn fails_the_open(error: &std::io::Error) -> bool {
    matches!(
        error.kind(),
        ErrorKind::NotFound | ErrorKind::NotADirectory | ErrorKind::InvalidFilename
    )
}

/// shells says whether a file bears a name the shell gives its history's.
fn shells(name: &OsStr) -> bool {
    SHELL_FILES.iter().any(|file| name == *file)
}

/// nlbwmons says whether a file bears a name nlbwmon gives a period's
/// database, the day it began then ".db" or, compressed, ".db.gz": the names
/// nlbwmon itself prunes, so a stick mounted where it keeps them keeps the
/// owner's other files. A day nlbwmon could not read bears ".broken" after
/// its name, as the usage watchdog moves it aside.
fn nlbwmons(name: &OsStr) -> bool {
    let name = name.as_bytes();
    let name = name.strip_suffix(b".broken").unwrap_or(name);
    let digits = name.iter().take_while(|byte| byte.is_ascii_digit()).count();
    digits > 0 && matches!(&name[digits..], b".db" | b".db.gz")
}

/// device is the device a path is on, as the shell's stat reads it.
fn device(path: &Path) -> std::io::Result<u64> {
    fs::metadata(path).map(|meta| meta.dev())
}

/// mounted are the places filesystems are mounted at, as the kernel's mount
/// table lists them.
fn mounted() -> std::io::Result<Vec<PathBuf>> {
    fs::read("/proc/self/mountinfo").map(|table| mount_points(&table))
}

/// mount_points reads a mountinfo table, where each line's fifth field is a
/// place a filesystem is mounted at, a space, a tab, a newline or a
/// backslash in it written as an octal escape.
fn mount_points(table: &[u8]) -> Vec<PathBuf> {
    table
        .split(|byte| *byte == b'\n')
        .filter_map(|line| line.split(|byte| *byte == b' ').nth(4))
        .map(|field| PathBuf::from(OsString::from_vec(unescaped(field))))
        .collect()
}

/// unescaped reads a mountinfo field's octal escapes back.
fn unescaped(field: &[u8]) -> Vec<u8> {
    let mut out = Vec::with_capacity(field.len());
    let mut rest = field;
    while let Some((&byte, after)) = rest.split_first() {
        let escaped = after
            .get(..3)
            .filter(|digits| byte == b'\\' && digits.iter().all(|d| (b'0'..=b'7').contains(d)))
            .and_then(|digits| {
                let value = digits.iter().fold(0u32, |n, d| n * 8 + u32::from(d - b'0'));
                u8::try_from(value).ok()
            });
        match escaped {
            Some(value) => {
                out.push(value);
                rest = &after[3..];
            }
            None => {
                out.push(byte);
                rest = after;
            }
        }
    }
    out
}

/// clear removes everything a directory held open holds and keeps it,
/// saying whether it kept anything. A link in it is removed, never followed;
/// a directory in it is held open in turn, emptied the same way and removed.
/// A filesystem mounted in it, on a device of its own or in the mount table,
/// is left with all it holds, and so is the directory it is in. path is
/// where the directory was reached, to name what is in it.
fn clear(dir: &Pinned, path: &Path, mounts: &[PathBuf]) -> Result<bool, String> {
    let mut kept = false;
    for name in dir
        .names()
        .map_err(|e| format!("{}: {e}", path.display()))?
    {
        let (at, path) = (dir.at(&name), path.join(&name));
        let failed = |error: std::io::Error| format!("{}: {error}", path.display());
        let meta = match fs::symlink_metadata(&at) {
            Ok(meta) => meta,
            Err(error) if error.kind() == ErrorKind::NotFound => continue,
            Err(error) => return Err(failed(error)),
        };
        if !meta.is_dir() {
            fs::remove_file(&at).or_else(already_gone).map_err(failed)?;
            continue;
        }
        if meta.dev() != dir.dev || mounts.contains(&path) {
            eprintln!(
                "verso-rpcd: reset: {} left as it is: a filesystem is mounted there",
                path.display()
            );
            kept = true;
            continue;
        }
        let Some(inner) = Pinned::open(&at, (meta.dev(), meta.ino()))
            .map_err(|error| format!("{}: {error}", path.display()))?
        else {
            continue;
        };
        if clear(&inner, &path, mounts)? {
            kept = true;
            continue;
        }
        fs::remove_dir(&at).or_else(already_gone).map_err(failed)?;
    }
    Ok(kept)
}

/// remove_files removes from a directory held open, which the history
/// shares, the files bearing its writer's names: what else bears one, a
/// link or a directory, is left, never followed or gone into. path is where
/// the directory was reached, to name what is in it.
fn remove_files(dir: &Pinned, path: &Path, names: Names) -> Result<(), String> {
    let found = dir
        .names()
        .map_err(|e| format!("{}: {e}", path.display()))?;
    for name in found.into_iter().filter(|name| names(name)) {
        let (at, path) = (dir.at(&name), path.join(&name));
        let removed = match fs::symlink_metadata(&at) {
            Ok(meta) if meta.is_file() => fs::remove_file(&at).or_else(already_gone),
            Ok(_) => {
                eprintln!(
                    "verso-rpcd: reset: {} left as it is: not a file",
                    path.display()
                );
                Ok(())
            }
            Err(error) => already_gone(error),
        };
        removed.map_err(|e| format!("{}: {e}", path.display()))?;
    }
    Ok(())
}

/// already_gone takes a name found gone as removed: its writer removed it
/// first.
fn already_gone(error: std::io::Error) -> std::io::Result<()> {
    match error.kind() {
        ErrorKind::NotFound => Ok(()),
        _ => Err(error),
    }
}

fn uci_get(key: &str) -> Option<String> {
    let output = Command::new("/sbin/uci")
        .args(["-q", "get", key])
        .output()
        .ok()?;
    let value = String::from_utf8(output.stdout).ok()?.trim().to_owned();
    (output.status.success() && !value.is_empty()).then_some(value)
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::os::unix::fs::{symlink, PermissionsExt};
    use std::sync::{Arc, Mutex};

    /// A wait long enough for any stand-in service to exit.
    const PATIENT: Duration = Duration::from_secs(5);

    /// Stand is a router's paths below a fresh directory, owned by whoever
    /// runs the test, who stands in for both the shell and the superuser.
    struct Stand {
        base: PathBuf,
        me: u32,
    }

    impl Stand {
        /// base is where the stand named so is, before its links are read.
        fn base(name: &str) -> PathBuf {
            std::env::temp_dir().join(format!("verso-reset-{name}-{}", std::process::id()))
        }

        fn new(name: &str) -> Self {
            let base = Self::base(name);
            let _ = fs::remove_dir_all(&base);
            fs::create_dir_all(&base).unwrap();
            // The root, which only the superuser rearranges.
            fs::set_permissions(&base, fs::Permissions::from_mode(0o755)).unwrap();
            let base = fs::canonicalize(&base).unwrap();
            let me = fs::metadata(&base).unwrap().uid();
            Self { base, me }
        }

        fn at(&self, path: &str) -> PathBuf {
            self.base.join(path.trim_start_matches('/'))
        }

        /// dir makes a directory and those above it, each 0755.
        fn dir(&self, path: &str) {
            let dir = self.at(path);
            fs::create_dir_all(&dir).unwrap();
            let mut up = dir.as_path();
            while up != self.base {
                fs::set_permissions(up, fs::Permissions::from_mode(0o755)).unwrap();
                up = up.parent().unwrap();
            }
        }

        fn file(&self, path: &str, body: &str) {
            let (dir, _) = path.rsplit_once('/').unwrap();
            self.dir(dir);
            fs::write(self.at(path), body).unwrap();
        }

        /// link makes a link at path to target, read from the stand's root
        /// as the router reads it from "/".
        fn link(&self, path: &str, target: &str) {
            let (dir, _) = path.rsplit_once('/').unwrap();
            self.dir(dir);
            symlink(target, self.at(path)).unwrap();
        }

        fn entries(&self, path: &str) -> usize {
            fs::read_dir(self.at(path)).unwrap().count()
        }

        fn history(&self, chosen: Option<&str>) -> History {
            History {
                root: self.base.clone(),
                shell: self.me,
                superuser: self.me,
                chosen: chosen.into_iter().map(PathBuf::from).collect(),
                device,
                mounts: || Ok(Vec::new()),
            }
        }
    }

    impl Drop for Stand {
        fn drop(&mut self) {
            let _ = fs::remove_dir_all(&self.base);
        }
    }

    /// Steps records what a reset does and holds the work after the answer
    /// for the test to run. Each writer runs as one process, which writes a
    /// file and exits when stopped: before stop returns, or lag after it, as
    /// procd's SIGTERM has a service do.
    #[derive(Clone, Default)]
    struct Steps {
        log: Arc<Mutex<Vec<String>>>,
        later: Arc<Mutex<Option<Work>>>,
        exited: Arc<Mutex<Vec<u32>>>,
        writes_on_stop: Vec<(&'static str, PathBuf)>,
        lag: Option<Duration>,
        /// A writer that ignores the stop and exits only when killed.
        stubborn: Option<&'static str>,
        /// What firstboot returns with, when it does not restart the router.
        firstboot_returns: Option<&'static str>,
        /// A writer that is not running.
        idle: Option<&'static str>,
        /// A writer whose processes cannot be read.
        unknown: Option<&'static str>,
    }

    impl Steps {
        fn log(&self) -> Vec<String> {
            self.log.lock().unwrap().clone()
        }

        fn note(&self, step: String) {
            self.log.lock().unwrap().push(step);
        }

        fn run_later(&self) {
            let work = self
                .later
                .lock()
                .unwrap()
                .take()
                .expect("work after the answer");
            work();
        }

        fn pid(service: &str) -> u32 {
            WRITERS.iter().position(|s| *s == service).unwrap() as u32 + 1
        }
    }

    impl Router for Steps {
        fn after_answer(&self, work: Work) {
            self.note("answered".into());
            *self.later.lock().unwrap() = Some(work);
        }

        fn processes(&self, service: &str) -> Option<Vec<Process>> {
            if self.unknown == Some(service) {
                return None;
            }
            if self.idle == Some(service) {
                return Some(Vec::new());
            }
            Some(vec![Process {
                pid: Steps::pid(service),
                began: 0,
            }])
        }

        fn stop(&self, service: &str) {
            self.note(format!("stop {service}"));
            if self.stubborn == Some(service) {
                return;
            }
            let steps = self.clone();
            let service = service.to_owned();
            let exit = move || {
                for (writer, path) in &steps.writes_on_stop {
                    if *writer == service {
                        fs::write(path, "written as it stops").unwrap();
                        steps.note(format!("{service} wrote"));
                    }
                }
                steps.exited.lock().unwrap().push(Steps::pid(&service));
            };
            match self.lag {
                None => exit(),
                Some(lag) => {
                    std::thread::spawn(move || {
                        std::thread::sleep(lag);
                        exit();
                    });
                }
            }
        }

        fn running(&self, process: &Process) -> bool {
            !self.exited.lock().unwrap().contains(&process.pid)
        }

        fn kill(&self, process: &Process) {
            let service = WRITERS[process.pid as usize - 1];
            self.note(format!("kill {service}"));
            self.exited.lock().unwrap().push(process.pid);
        }

        fn firstboot(&self) -> Option<String> {
            self.note("firstboot".into());
            self.firstboot_returns.map(String::from)
        }

        fn start(&self, service: &str) {
            self.note(format!("start {service}"));
        }
    }

    #[test]
    fn a_moved_history_leaves_no_default_behind() {
        // The history was moved off /data/verso by hand. After the reset the
        // shell opens /data/verso again, so it is emptied with the chosen one.
        let stand = Stand::new("moved");
        stand.file("/data/verso/history.db", "the previous owner's");
        stand.file("/data/nlbwmon/20261015.db.gz", "usage");
        stand.file("/mnt/usb/verso/history.db", "the history");
        stand.file("/usr/share/verso/history/history.db", "older");
        stand.history(Some("/mnt/usb/verso")).erase().unwrap();
        for dir in [
            "/data/verso",
            "/data/nlbwmon",
            "/mnt/usb/verso",
            "/usr/share/verso/history",
        ] {
            assert!(stand.at(dir).is_dir(), "{dir} is kept");
            assert_eq!(stand.entries(dir), 0, "{dir} is emptied");
        }
    }

    #[test]
    fn the_directory_the_running_shell_writes_in_is_emptied_with_the_one_set() {
        // A directory applied while the shell runs reaches it as a SIGHUP:
        // the shell writes on where it was started.
        let stand = Stand::new("running");
        stand.file("/data/history/history.db", "written until the shell stops");
        stand.file("/mnt/usb/verso/history.db", "the one set");
        let history = History {
            chosen: vec!["/mnt/usb/verso".into(), "/data/history".into()],
            ..stand.history(None)
        };
        history.erase().unwrap();
        assert_eq!(stand.entries("/data/history"), 0);
        assert_eq!(stand.entries("/mnt/usb/verso"), 0);
    }

    #[test]
    fn a_place_not_there_is_already_empty() {
        let stand = Stand::new("absent");
        stand.history(None).erase().unwrap();
        stand.history(Some("/mnt/usb/verso")).erase().unwrap();
    }

    #[test]
    fn a_chosen_directory_is_made_clean_as_the_shell_makes_it() {
        // The shell writes at filepath.Join(dir, "history.db"), from "/"
        // where procd starts it.
        for (dir, clean) in [
            ("/data/history", "/data/history"),
            ("/data//history/", "/data/history"),
            ("/mnt/usb/stick/.", "/mnt/usb/stick"),
            ("/mnt/sda1/x/..", "/mnt/sda1"),
            ("/mnt/sda1/nonexistent/..", "/mnt/sda1"),
            ("/data/verso/..", "/data"),
            ("/data/verso/../x", "/data/x"),
            ("/..", "/"),
            ("/../../etc", "/etc"),
            ("mnt/sda1", "/mnt/sda1"),
            ("./x/../y", "/y"),
            ("../etc", "/etc"),
        ] {
            assert_eq!(cleaned(Path::new(dir)), Path::new(clean), "{dir}");
        }
    }

    #[test]
    fn a_chosen_directory_loses_only_the_shells_files() {
        // The option is the shell's to set: whatever it names, root removes
        // there only files bearing the shell's names, never anything else.
        let stand = Stand::new("named");
        stand.file("/data/history.db", "the shell's");
        stand.file("/data/dropbear/key", "root's");
        stand.file("/data/a/b/history.db", "the shell's");
        stand.file("/data/a/b/notes.txt", "root's");
        stand.file("/data/a/b/stopped/kept", "root's");
        for chosen in ["/data", "/data/a/b"] {
            stand.history(Some(chosen)).erase().unwrap();
        }
        assert_eq!(names(&stand.at("/data")), ["a", "dropbear"]);
        assert!(stand.at("/data/dropbear/key").exists());
        assert_eq!(names(&stand.at("/data/a/b")), ["notes.txt", "stopped"]);
        assert!(
            stand.at("/data/a/b/stopped/kept").exists(),
            "never gone into"
        );
    }

    #[test]
    fn a_chosen_directory_is_emptied_where_the_shell_writes_with_dot_dot() {
        let stand = Stand::new("dot-dot");
        for (chosen, at) in [
            ("/mnt/sda1/x/..", "/mnt/sda1/history.db"),
            ("/data/verso/..", "/data/history.db"),
            ("/mnt/sda1/nonexistent/..", "/mnt/sda1/history.db"),
            ("/data/verso/../x", "/data/x/history.db"),
        ] {
            stand.file(at, "the shell's");
            stand.dir("/mnt/sda1/x");
            stand.file("/mnt/sda1/photos.jpg", "the owner's");
            let history = History {
                device: |path| device(path).map(|dev| dev + u64::from(path.ends_with("mnt/sda1"))),
                ..stand.history(Some(chosen))
            };
            history.erase().unwrap();
            assert!(!stand.at(at).exists(), "{chosen}: {at} is removed");
            assert!(stand.at("/mnt/sda1/photos.jpg").exists());
        }
    }

    #[test]
    fn a_relative_directory_is_read_from_the_root() {
        let stand = Stand::new("relative");
        stand.file("/mnt/sda1/history.db", "the shell's");
        stand.history(Some("mnt/sda1")).erase().unwrap();
        assert!(!stand.at("/mnt/sda1/history.db").exists());
    }

    #[test]
    fn a_directory_the_shells_open_fails_in_holds_nothing() {
        // A file where a directory must be, a name too long, a link to
        // itself: the shell's open fails there, as root's lstat does.
        let stand = Stand::new("open-fails");
        stand.file("/data/x", "a file");
        stand.link("/data/loop", "loop");
        let long = format!("/data/{}", "a".repeat(300));
        for chosen in [
            "/data/x",
            "/data/x/history.db",
            "/data/loop",
            "/data/loop/a",
            &long,
        ] {
            stand.history(Some(chosen)).erase().unwrap();
        }
        assert_eq!(fs::read_to_string(stand.at("/data/x")).unwrap(), "a file");
    }

    #[test]
    fn a_chosen_directory_below_a_default_goes_with_it() {
        // /data/verso is the shell's own, which it may rearrange; a chosen
        // directory below it is emptied with it, never refused.
        let stand = Stand::new("below-a-default");
        stand.file("/data/verso/old/history.db", "the shell's");
        fs::set_permissions(stand.at("/data/verso"), fs::Permissions::from_mode(0o777)).unwrap();
        stand.history(Some("/data/verso/old")).erase().unwrap();
        assert_eq!(stand.entries("/data/verso"), 0);
    }

    #[test]
    fn a_stick_where_block_mount_puts_it_loses_the_shells_files() {
        // block mounts a stick it has no mount point for at /mnt/<device>.
        // The init script passes /mnt/sda1 on as it is and hands nothing
        // over; the shell takes the stick's root as shared and writes its
        // files there all the same.
        let stand = Stand::new("block-mount");
        stand.file("/mnt/sda1/history.db", "the shell's");
        stand.file("/mnt/sda1/stopped", "");
        stand.file("/mnt/sda1/photos.jpg", "the owner's");
        stand.file("/mnt/sda1/trips/a.jpg", "the owner's");
        let history = History {
            shell: stand.me.wrapping_add(1),
            device: |path| device(path).map(|dev| dev + u64::from(path.ends_with("mnt/sda1"))),
            ..stand.history(Some("/mnt/sda1"))
        };
        history.erase().unwrap();
        assert_eq!(names(&stand.at("/mnt/sda1")), ["photos.jpg", "trips"]);
        assert!(stand.at("/mnt/sda1/trips/a.jpg").exists());
    }

    #[test]
    fn a_mounts_root_loses_the_shells_files_whatever_the_directory_above() {
        // No account may move or replace a mount's root while it is mounted.
        let stand = Stand::new("block-mount-open");
        stand.file("/mnt/sda1/history.db", "the shell's");
        stand.file("/mnt/sda1/photos.jpg", "the owner's");
        fs::set_permissions(stand.at("/mnt"), fs::Permissions::from_mode(0o777)).unwrap();
        let history = History {
            device: |path| device(path).map(|dev| dev + u64::from(path.ends_with("mnt/sda1"))),
            ..stand.history(Some("/mnt/sda1"))
        };
        history.erase().unwrap();
        assert_eq!(names(&stand.at("/mnt/sda1")), ["photos.jpg"]);
    }

    #[test]
    fn a_chosen_directory_of_another_accounts_loses_only_the_shells_files() {
        // A FAT stick mounted with quiet lets the shell's chmod succeed
        // without effect, and the shell writes in a directory of root's.
        let stand = Stand::new("not-the-shells");
        stand.file("/data/verso/history.db", "past");
        stand.file("/data/dropbear/key", "root's");
        stand.file("/mnt/sda1/verso/history.db", "the shell's");
        stand.file("/mnt/sda1/verso/notes.txt", "the stick owner's");
        let history = History {
            shell: stand.me.wrapping_add(1),
            chosen: vec!["/data/dropbear".into(), "/mnt/sda1/verso".into()],
            ..stand.history(None)
        };
        history.erase().unwrap();
        assert!(stand.at("/data/dropbear/key").exists());
        assert_eq!(names(&stand.at("/mnt/sda1/verso")), ["notes.txt"]);
        assert_eq!(stand.entries("/data/verso"), 0, "the defaults are emptied");
    }

    #[test]
    fn a_directory_the_shell_owns_in_tmp_loses_the_shells_files() {
        // /tmp is sticky: only root and the directory's owner, the shell,
        // may move it, and the shell moving it only names another.
        let stand = Stand::new("tmp");
        stand.file("/tmp/verso/history.db", "the shell's");
        fs::set_permissions(stand.at("/tmp"), fs::Permissions::from_mode(0o1777)).unwrap();
        let history = History {
            shell: stand.me,
            superuser: stand.me,
            ..stand.history(Some("/tmp/verso"))
        };
        history.erase().unwrap();
        assert_eq!(stand.entries("/tmp/verso"), 0);
    }

    /// stick_mounted stands in for a stick mounted at /mnt/usb/stick: its
    /// root is on a device of its own.
    fn stick_mounted(path: &Path) -> std::io::Result<u64> {
        device(path).map(|dev| dev + u64::from(path.ends_with("mnt/usb/stick")))
    }

    /// sda1_mounted stands in for a stick mounted at /mnt/sda1: it and all
    /// it holds are on a device of its own.
    fn sda1_mounted(path: &Path) -> std::io::Result<u64> {
        let on = path.ancestors().any(|dir| dir.ends_with("mnt/sda1"));
        device(path).map(|dev| dev + u64::from(on))
    }

    /// names is what a directory holds, sorted.
    fn names(dir: &Path) -> Vec<String> {
        let mut names: Vec<String> = fs::read_dir(dir)
            .unwrap()
            .map(|entry| entry.unwrap().file_name().to_string_lossy().into_owned())
            .collect();
        names.sort();
        names
    }

    #[test]
    fn a_stick_root_the_shell_owns_loses_only_the_shells_files() {
        // The shell keeps the mode of a mount's root it is given and writes
        // its files beside the stick's own, which are the owner's.
        let stand = Stand::new("stick-root");
        for name in SHELL_FILES {
            stand.file(&format!("/mnt/usb/stick/{name}"), "the shell's");
        }
        stand.file("/mnt/usb/stick/photos.jpg", "the owner's");
        stand.file("/mnt/usb/stick/trips/a.jpg", "the owner's");
        let history = History {
            device: stick_mounted,
            ..stand.history(Some("/mnt/usb/stick"))
        };
        history.erase().unwrap();
        assert_eq!(names(&stand.at("/mnt/usb/stick")), ["photos.jpg", "trips"]);
        assert!(stand.at("/mnt/usb/stick/trips/a.jpg").exists());
    }

    #[test]
    fn a_stick_root_of_another_accounts_loses_the_shells_files_too() {
        // An ext4 stick made elsewhere brings its own owner; the shell writes
        // in a root left open to every account all the same.
        let stand = Stand::new("stick-root-another");
        stand.file("/mnt/usb/stick/history.db", "the shell's");
        stand.file("/mnt/usb/stick/stopped", "");
        stand.file("/mnt/usb/stick/photos.jpg", "the owner's");
        stand.file("/mnt/usb/stick/history.db-shm/inside", "the owner's");
        stand.file("/elsewhere/kept", "kept");
        symlink(
            stand.at("/elsewhere/kept"),
            stand.at("/mnt/usb/stick/history.db-wal"),
        )
        .unwrap();
        fs::set_permissions(
            stand.at("/mnt/usb/stick"),
            fs::Permissions::from_mode(0o777),
        )
        .unwrap();
        let history = History {
            shell: stand.me.wrapping_add(1),
            device: stick_mounted,
            ..stand.history(Some("/mnt/usb/stick"))
        };
        history.erase().unwrap();
        assert_eq!(
            names(&stand.at("/mnt/usb/stick")),
            ["history.db-shm", "history.db-wal", "photos.jpg"],
            "only files are removed, by name"
        );
        assert!(
            stand.at("/mnt/usb/stick/history.db-shm/inside").exists(),
            "never recursed into"
        );
        assert_eq!(
            fs::read_to_string(stand.at("/elsewhere/kept")).unwrap(),
            "kept",
            "never followed"
        );
    }

    #[test]
    fn a_stick_mounted_at_a_default_keeps_the_owners_files() {
        // A stick mounted at /data/verso or /data/nlbwmon is the owner's;
        // only the files bearing the shell's or nlbwmon's names go.
        let stand = Stand::new("default-mounted");
        stand.file("/data/verso/history.db", "the shell's");
        stand.file("/data/verso/stopped", "");
        stand.file("/data/verso/photos.jpg", "the owner's");
        stand.file("/data/verso/trips/a.jpg", "the owner's");
        stand.file("/data/nlbwmon/20261015.db.gz", "usage");
        stand.file("/data/nlbwmon/20261014.db", "usage");
        stand.file("/data/nlbwmon/notes.txt", "the owner's");
        let history = History {
            device: |path| {
                let mounted = path.ends_with("data/verso") || path.ends_with("data/nlbwmon");
                device(path).map(|dev| dev + u64::from(mounted))
            },
            ..stand.history(None)
        };
        history.erase().unwrap();
        assert_eq!(names(&stand.at("/data/verso")), ["photos.jpg", "trips"]);
        assert!(stand.at("/data/verso/trips/a.jpg").exists());
        assert_eq!(names(&stand.at("/data/nlbwmon")), ["notes.txt"]);
    }

    #[test]
    fn an_open_stick_root_holds_no_reset_back() {
        // ntfs3 shows a volume's root as 0777, and a FAT stick shared over
        // Samba is mounted umask=000: any account may rearrange the stick's
        // root, and the history in a directory on it is emptied all the same.
        let stand = Stand::new("open-stick-root");
        stand.file("/data/verso/history.db", "the previous owner's");
        stand.file("/mnt/sda1/verso/history.db", "the shell's");
        stand.file("/mnt/sda1/verso/photos.jpg", "the owner's");
        fs::set_permissions(stand.at("/mnt/sda1"), fs::Permissions::from_mode(0o777)).unwrap();
        let history = History {
            device: sda1_mounted,
            ..stand.history(Some("/mnt/sda1/verso"))
        };
        reset(history, Steps::default(), PATIENT).unwrap();
        assert_eq!(stand.entries("/data/verso"), 0);
        assert_eq!(names(&stand.at("/mnt/sda1/verso")), ["photos.jpg"]);
    }

    #[test]
    fn a_day_nlbwmon_could_not_read_goes_with_the_others() {
        // The usage watchdog moves a day nlbwmon cannot read aside with
        // ".broken" after its name; a stick at /data/nlbwmon loses it too.
        let stand = Stand::new("broken-day");
        stand.file("/data/nlbwmon/20261015.db.gz", "usage");
        stand.file("/data/nlbwmon/20261014.db.gz.broken", "the past owner's");
        stand.file("/data/nlbwmon/20261013.db.broken", "the past owner's");
        stand.file("/data/nlbwmon/notes.txt", "the owner's");
        let history = History {
            device: |path| device(path).map(|dev| dev + u64::from(path.ends_with("data/nlbwmon"))),
            ..stand.history(None)
        };
        history.erase().unwrap();
        assert_eq!(names(&stand.at("/data/nlbwmon")), ["notes.txt"]);
    }

    #[test]
    fn a_chosen_link_below_a_default_is_emptied_where_it_leads() {
        // /data/verso is emptied whole, its link with it, but the shell
        // writes where the link leads.
        let stand = Stand::new("link-below-a-default");
        stand.file("/mnt/sda1/verso/history.db", "the previous owner's");
        stand.file("/mnt/sda1/verso/photos.jpg", "the owner's");
        stand.link("/data/verso/usb", "/mnt/sda1/verso");
        stand.history(Some("/data/verso/usb")).erase().unwrap();
        assert_eq!(stand.entries("/data/verso"), 0);
        assert_eq!(names(&stand.at("/mnt/sda1/verso")), ["photos.jpg"]);
    }

    #[test]
    fn nlbwmons_files_are_named_by_the_day_their_period_began() {
        for name in [
            "20261015.db.gz",
            "20261015.db",
            "0.db",
            "20261015.db.broken",
            "20261015.db.gz.broken",
        ] {
            assert!(nlbwmons(OsStr::new(name)), "{name}");
        }
        for name in [
            "notes.txt",
            ".db",
            "a.db.gz",
            "20261015.db.gz.bak",
            "20261015.broken",
            "20261015.db.broken.broken",
            "2026 .db",
        ] {
            assert!(!nlbwmons(OsStr::new(name)), "{name}");
        }
    }

    #[test]
    fn a_reset_with_a_chosen_directory_not_the_shells_goes_ahead() {
        let stand = Stand::new("stick");
        stand.file("/data/verso/history.db", "past");
        stand.dir("/mnt/sda1/verso");
        let history = History {
            shell: stand.me.wrapping_add(1),
            superuser: stand.me,
            ..stand.history(Some("/mnt/sda1/verso"))
        };
        let steps = Steps::default();
        reset(history, steps.clone(), PATIENT).unwrap();
        assert_eq!(stand.entries("/data/verso"), 0);
        assert_eq!(steps.log(), ["answered"]);
    }

    #[test]
    fn a_default_of_another_accounts_loses_only_the_history_files() {
        let stand = Stand::new("default-of-another");
        stand.file("/data/verso/history.db", "x");
        stand.file("/data/verso/notes.txt", "another account's");
        stand.file("/mnt/usb/verso/history.db", "x");
        let history = History {
            shell: stand.me.wrapping_add(1),
            superuser: stand.me.wrapping_add(2),
            ..stand.history(Some("/mnt/usb/verso"))
        };
        history.erase().unwrap();
        assert_eq!(names(&stand.at("/data/verso")), ["notes.txt"]);
        assert_eq!(stand.entries("/mnt/usb/verso"), 0);
    }

    #[test]
    fn a_place_that_cannot_be_reached_is_answered_and_nothing_emptied() {
        let stand = Stand::new("unreachable");
        stand.file("/data/verso/history.db", "x");
        stand.file("/mnt/usb/verso/history.db", "x");
        let history = History {
            device: |path| match path.ends_with("mnt/usb/verso") {
                true => Err(std::io::Error::other("the stick does not answer")),
                false => device(path),
            },
            ..stand.history(Some("/mnt/usb/verso"))
        };
        assert_eq!(
            history.erase().unwrap_err(),
            "the history in /mnt/usb/verso cannot be emptied: the stick does not answer"
        );
        assert_eq!(
            stand.entries("/data/verso"),
            1,
            "every place is reached before any is emptied"
        );
    }

    #[test]
    fn past_the_answer_every_place_that_can_be_is_emptied() {
        let stand = Stand::new("what-it-can");
        stand.file("/data/verso/history.db", "x");
        stand.file("/mnt/usb/verso/history.db", "x");
        stand.file("/mnt/sda1/verso/history.db", "x");
        let history = History {
            device: |path| match path.ends_with("mnt/usb/verso") {
                true => Err(std::io::Error::other("the stick does not answer")),
                false => device(path),
            },
            chosen: vec!["/mnt/usb/verso".into(), "/mnt/sda1/verso".into()],
            ..stand.history(None)
        };
        assert_eq!(
            history.erase_what_it_can(),
            ["the history in /mnt/usb/verso cannot be emptied: the stick does not answer"]
        );
        assert_eq!(stand.entries("/data/verso"), 0);
        assert_eq!(stand.entries("/mnt/sda1/verso"), 0);
    }

    #[test]
    fn a_directory_replaced_after_it_was_reached_is_left() {
        // The shell owns its directory in sticky /tmp and may swap it, after
        // it was reached, for a link to a folder of the operator's, or move
        // such a folder there: nothing outside the directory reached goes.
        for name in ["link", "moved"] {
            let stand = Stand::new(&format!("replaced-{name}"));
            stand.file("/tmp/verso/history.db", "the shell's");
            fs::set_permissions(stand.at("/tmp"), fs::Permissions::from_mode(0o1777)).unwrap();
            stand.file("/mnt/sda1/photos/history.db", "the operator's own");
            stand.file("/mnt/sda1/photos/a.jpg", "the operator's");
            let history = stand.history(Some("/tmp/verso"));
            let found = history.find(&[]);
            fs::rename(stand.at("/tmp/verso"), stand.at("/tmp/verso.old")).unwrap();
            let (photos, verso) = (stand.at("/mnt/sda1/photos"), stand.at("/tmp/verso"));
            match name {
                "link" => symlink(photos, verso),
                _ => fs::rename(photos, verso),
            }
            .unwrap();
            let chosen = found.last().unwrap().as_ref().unwrap();
            let error = chosen.empty(&[]).unwrap_err();
            assert!(
                error.starts_with("the history in /tmp/verso cannot be emptied: "),
                "{name}: {error}"
            );
            let operators = ["/tmp/verso/history.db", "/tmp/verso/a.jpg"];
            assert!(
                operators.iter().all(|file| stand.at(file).exists()),
                "{name}"
            );
        }
    }

    #[test]
    fn a_filesystem_mounted_inside_a_default_is_left_whole() {
        // A stick mounted at /data/nlbwmon/usb is the owner's: emptying
        // /data/nlbwmon leaves it and every directory it is in.
        let stand = Stand::new("mounted-inside");
        stand.file("/data/nlbwmon/20261015.db.gz", "usage");
        stand.file("/data/nlbwmon/usb/trips/a.jpg", "the owner's");
        stand.file("/data/nlbwmon/old/usb/b.jpg", "the owner's");
        stand.file("/data/nlbwmon/old/20261001.db", "usage");
        stand.file("/data/nlbwmon/gone/c", "usage");
        let history = History {
            mounts: || {
                let base = fs::canonicalize(Stand::base("mounted-inside"))?;
                Ok(vec![
                    base.join("data/nlbwmon/usb"),
                    base.join("data/nlbwmon/old/usb"),
                ])
            },
            ..stand.history(None)
        };
        history.erase().unwrap();
        assert_eq!(names(&stand.at("/data/nlbwmon")), ["old", "usb"]);
        assert_eq!(names(&stand.at("/data/nlbwmon/old")), ["usb"]);
        assert!(stand.at("/data/nlbwmon/usb/trips/a.jpg").exists());
        assert!(stand.at("/data/nlbwmon/old/usb/b.jpg").exists());
    }

    #[test]
    fn a_default_the_mount_table_lists_loses_only_the_history_files() {
        // A bind mount of a directory on /data at /data/verso is on /data's
        // device; the mount table shows it is the owner's.
        let stand = Stand::new("bound-default");
        stand.file("/data/verso/history.db", "the shell's");
        stand.file("/data/verso/photos.jpg", "the owner's");
        let history = History {
            mounts: || {
                let base = fs::canonicalize(Stand::base("bound-default"))?;
                Ok(vec![base.join("data/verso")])
            },
            ..stand.history(None)
        };
        history.erase().unwrap();
        assert_eq!(names(&stand.at("/data/verso")), ["photos.jpg"]);
    }

    #[test]
    fn a_mount_table_that_cannot_be_read_is_answered() {
        let stand = Stand::new("no-mount-table");
        stand.file("/data/verso/history.db", "x");
        let history = History {
            mounts: || Err(std::io::Error::other("no /proc")),
            ..stand.history(None)
        };
        assert_eq!(
            history.erase().unwrap_err(),
            "the history cannot be found: the mount table cannot be read: no /proc"
        );
        assert_eq!(stand.entries("/data/verso"), 1);
    }

    #[test]
    fn the_mount_table_is_read_with_its_escapes() {
        let table = b"22 1 0:20 / /proc rw,nosuid shared:12 - proc proc rw\n\
            36 25 8:1 / /mnt/my\\040stick rw,noatime shared:1 - vfat /dev/sda1 rw\n\
            37 25 8:2 /verso /data/verso rw - ext4 /dev/sda2 rw\n\
            38 25 8:3 / /mnt/a\\134b\\0119 rw - ext4 /dev/sda3 rw\n";
        assert_eq!(
            mount_points(table),
            ["/proc", "/mnt/my stick", "/data/verso", "/mnt/a\\b\t9"].map(PathBuf::from)
        );
    }

    #[test]
    fn a_link_only_root_may_change_is_followed_as_the_shell_writes_through_it() {
        // OpenWrt links /var to /tmp, and an owner may link /data/verso to a
        // stick by hand: the shell writes where the link leads, and only
        // its files go there, never the link.
        let stand = Stand::new("links");
        stand.link("/var", "tmp");
        stand.file("/tmp/history.db", "the shell's");
        stand.file("/tmp/other", "another daemon's");
        stand.file("/tmp/verso/history.db", "the shell's");
        stand.link("/data/verso", "/mnt/sda1/verso");
        stand.file("/mnt/sda1/verso/history.db", "the shell's");
        stand.file("/mnt/sda1/verso/photos.jpg", "the owner's");
        stand.link("/data/hist", "../mnt/sda1");
        stand.file("/mnt/sda1/history.db", "the shell's");
        stand.file("/stick/verso/history.db", "the shell's");
        stand.link("/mnt/usb", "/stick");
        let history = History {
            chosen: ["/var", "/var/verso", "/data/hist", "/mnt/usb/verso"]
                .map(PathBuf::from)
                .into(),
            device: |path| {
                let mounted = path.ends_with("tmp") || path.ends_with("mnt/sda1");
                device(path).map(|dev| dev + u64::from(mounted))
            },
            ..stand.history(None)
        };
        history.erase().unwrap();
        assert_eq!(names(&stand.at("/tmp")), ["other", "verso"]);
        assert_eq!(stand.entries("/tmp/verso"), 0);
        assert_eq!(names(&stand.at("/mnt/sda1/verso")), ["photos.jpg"]);
        assert_eq!(names(&stand.at("/mnt/sda1")), ["verso"]);
        assert_eq!(stand.entries("/stick/verso"), 0);
        for link in ["/var", "/data/verso", "/data/hist", "/mnt/usb"] {
            assert!(stand.at(link).is_symlink(), "{link} is kept");
        }
    }

    #[test]
    fn a_directory_others_may_rearrange_loses_only_the_shells_files() {
        // Through a link in a directory any account may change, or in one,
        // the directory reached is held open and loses only the shell's
        // files: whatever is put there after it was reached is left.
        let stand = Stand::new("rearranged");
        stand.file("/stick/verso/history.db", "the shell's");
        stand.file("/stick/verso/notes.txt", "the owner's");
        stand.link("/mnt/usb", "/stick");
        stand.file("/open/hist/history.db", "the shell's");
        stand.file("/open/hist/notes.txt", "the owner's");
        stand.link("/data/hist", "/open/hist");
        stand.file("/open/verso/history.db", "the shell's");
        for open in ["/mnt", "/open"] {
            fs::set_permissions(stand.at(open), fs::Permissions::from_mode(0o777)).unwrap();
        }
        let history = History {
            chosen: ["/mnt/usb/verso", "/data/hist", "/open/verso"]
                .map(PathBuf::from)
                .into(),
            ..stand.history(None)
        };
        history.erase().unwrap();
        assert_eq!(names(&stand.at("/stick/verso")), ["notes.txt"]);
        assert_eq!(names(&stand.at("/open/hist")), ["notes.txt"]);
        assert_eq!(stand.entries("/open/verso"), 0);
    }

    #[test]
    fn emptying_keeps_the_directory_and_nothing_in_it() {
        let stand = Stand::new("emptying");
        stand.file("/data/verso/history.db", "x");
        stand.file("/data/verso/usage/day", "x");
        stand.file("/elsewhere/kept", "kept");
        symlink(stand.at("/elsewhere/kept"), stand.at("/data/verso/link")).unwrap();
        symlink(stand.at("/elsewhere"), stand.at("/data/verso/dirlink")).unwrap();

        stand.history(None).erase().unwrap();
        assert!(stand.at("/data/verso").is_dir());
        assert_eq!(stand.entries("/data/verso"), 0);
        assert_eq!(
            fs::read_to_string(stand.at("/elsewhere/kept")).unwrap(),
            "kept",
            "a link is removed, never followed"
        );
    }

    #[test]
    fn a_failing_erase_is_answered_and_the_reset_never_starts() {
        let stand = Stand::new("failing");
        stand.file("/data/verso/history.db", "x");
        let history = History {
            mounts: || Err(std::io::Error::other("no /proc")),
            ..stand.history(None)
        };
        let steps = Steps::default();
        let failure = reset(history, steps.clone(), PATIENT).unwrap_err();
        assert_eq!(
            failure.status,
            crate::STATUS_INVALID_ARGUMENT,
            "the shell names the reason"
        );
        assert!(
            steps.log().is_empty(),
            "nothing stops, firstboot never runs"
        );
        assert!(steps.later.lock().unwrap().is_none());
    }

    #[test]
    fn the_reset_empties_answers_stops_empties_again_and_restarts() {
        let stand = Stand::new("order");
        stand.file("/data/verso/history.db", "past");
        stand.file("/data/nlbwmon/20261015.db.gz", "past");
        let steps = Steps {
            writes_on_stop: vec![
                ("verso", stand.at("/data/verso/stopped")),
                ("nlbwmon", stand.at("/data/nlbwmon/20261015.db.gz")),
            ],
            ..Steps::default()
        };
        reset(stand.history(None), steps.clone(), PATIENT).unwrap();
        assert_eq!(stand.entries("/data/verso"), 0, "emptied before the answer");
        assert_eq!(
            stand.entries("/data/nlbwmon"),
            0,
            "emptied before the answer"
        );
        assert_eq!(steps.log(), ["answered"]);

        stand.file("/data/verso/history.db", "written until the shell stops");
        steps.run_later();
        assert_eq!(
            steps.log(),
            [
                "answered",
                "stop verso",
                "verso wrote",
                "stop nlbwmon",
                "nlbwmon wrote",
                "firstboot"
            ]
        );
        assert_eq!(stand.entries("/data/verso"), 0);
        assert_eq!(stand.entries("/data/nlbwmon"), 0);
    }

    #[test]
    fn what_a_service_writes_after_its_stop_returns_is_emptied_too() {
        // procd signals and returns; nlbwmon saves the day a moment later.
        let stand = Stand::new("late-save");
        stand.file("/data/nlbwmon/20261015.db.gz", "past");
        let steps = Steps {
            writes_on_stop: vec![("nlbwmon", stand.at("/data/nlbwmon/20261015.db.gz"))],
            lag: Some(Duration::from_millis(150)),
            ..Steps::default()
        };
        reset(stand.history(None), steps.clone(), PATIENT).unwrap();
        steps.run_later();
        assert!(
            steps.log().contains(&"nlbwmon wrote".to_owned()),
            "the save landed before the work went on"
        );
        assert_eq!(stand.entries("/data/nlbwmon"), 0, "and was emptied");
        assert_eq!(steps.log().last().unwrap(), "firstboot");
    }

    #[test]
    fn a_service_that_outlives_its_stop_is_killed_before_the_second_emptying() {
        let stand = Stand::new("stubborn");
        let steps = Steps {
            stubborn: Some("nlbwmon"),
            ..Steps::default()
        };
        reset(
            stand.history(None),
            steps.clone(),
            Duration::from_millis(50),
        )
        .unwrap();
        steps.run_later();
        assert_eq!(
            steps.log(),
            [
                "answered",
                "stop verso",
                "stop nlbwmon",
                "kill nlbwmon",
                "firstboot"
            ]
        );
    }

    #[test]
    fn a_firstboot_that_returns_starts_the_services_again() {
        // firstboot returns, the router not restarting, where there is no
        // rootfs_data or it is not mounted.
        let stand = Stand::new("firstboot-returns");
        let steps = Steps {
            firstboot_returns: Some("exit status: 0"),
            ..Steps::default()
        };
        reset(stand.history(None), steps.clone(), PATIENT).unwrap();
        steps.run_later();
        assert_eq!(
            steps.log(),
            [
                "answered",
                "stop verso",
                "stop nlbwmon",
                "firstboot",
                "start verso",
                "start nlbwmon"
            ]
        );
    }

    #[test]
    fn a_firstboot_that_returns_starts_only_what_ran_before() {
        let stand = Stand::new("firstboot-returns-idle");
        let steps = Steps {
            firstboot_returns: Some("exit status: 0"),
            idle: Some("nlbwmon"),
            ..Steps::default()
        };
        reset(stand.history(None), steps.clone(), PATIENT).unwrap();
        steps.run_later();
        assert_eq!(
            steps.log(),
            [
                "answered",
                "stop verso",
                "stop nlbwmon",
                "firstboot",
                "start verso"
            ]
        );
    }

    #[test]
    fn a_firstboot_that_returns_starts_a_service_not_known_to_be_idle() {
        // procd could not be asked: nlbwmon may have run.
        let stand = Stand::new("firstboot-returns-unknown");
        let steps = Steps {
            firstboot_returns: Some("exit status: 0"),
            unknown: Some("nlbwmon"),
            ..Steps::default()
        };
        reset(stand.history(None), steps.clone(), PATIENT).unwrap();
        steps.run_later();
        assert_eq!(
            steps.log(),
            [
                "answered",
                "stop verso",
                "stop nlbwmon",
                "firstboot",
                "start verso",
                "start nlbwmon"
            ]
        );
    }

    #[test]
    fn a_stat_line_reads_past_any_name() {
        let line = "812 (a) b (c)) S 1 812 812 0 -1 4194560 300 0 0 0 5 2 0 0 39 19 1 0 4242 1 2 3";
        assert_eq!(
            stat(line),
            Some(Stat {
                parent: 1,
                began: 4242,
                zombie: false
            })
        );
        assert!(
            stat("9 (z) Z 1 9 9 0 -1 0 0 0 0 0 0 0 0 0 20 0 1 0 77")
                .unwrap()
                .zombie
        );
        assert_eq!(stat("9 (cut) S 1"), None);
    }

    #[test]
    fn a_service_is_its_running_instances_and_what_they_started() {
        let answer = br#"{"verso":{"instances":{
            "verso":{"running":true,"pid":100,"command":["/usr/bin/verso"]},
            "old":{"running":false,"exit_code":0}}}}"#;
        assert_eq!(instance_pids(answer, "verso"), Some(vec![100]));
        assert_eq!(instance_pids(b"{}", "verso"), Some(vec![]), "none running");
        assert_eq!(instance_pids(b"not json", "verso"), None, "unknown");
        assert_eq!(instance_pids(b"", "verso"), None, "unknown");

        let at = |parent, began| Stat {
            parent,
            began,
            zombie: false,
        };
        // ujail 100 runs the shell 101, which runs 102; 200 is someone else.
        let table = [
            (1, at(0, 1)),
            (100, at(1, 10)),
            (101, at(100, 11)),
            (102, at(101, 12)),
            (200, at(1, 20)),
        ];
        assert_eq!(
            family(&[100], &table),
            [
                Process {
                    pid: 100,
                    began: 10
                },
                Process {
                    pid: 101,
                    began: 11
                },
                Process {
                    pid: 102,
                    began: 12
                }
            ]
        );
        assert!(family(&[300], &table).is_empty(), "an instance since gone");
    }

    #[test]
    fn the_running_shells_directory_is_read_as_its_flags_are() {
        let read = |args: &[&str]| {
            let args: Vec<&OsStr> = args.iter().map(OsStr::new).collect();
            history_flag(&args)
        };
        assert_eq!(
            read(&[
                "-listen-https",
                ":443",
                "-history-dir",
                "/data/history",
                "-probe",
                "1.1.1.1"
            ]),
            Some(PathBuf::from("/data/history"))
        );
        assert_eq!(
            read(&["--history-dir=/mnt/usb/verso"]),
            Some(PathBuf::from("/mnt/usb/verso"))
        );
        assert_eq!(
            read(&["-history-dir", "/data/a", "-history-dir=/data/b"]),
            Some(PathBuf::from("/data/b")),
            "the last one counts"
        );
        assert_eq!(read(&["-history-changes", "10"]), None);
        assert_eq!(read(&["-history-dir"]), None);
        assert_eq!(
            read(&[
                "-history-dir",
                "/data/verso",
                "-probe",
                "-history-dir=/data/x"
            ]),
            Some(PathBuf::from("/data/verso")),
            "a flag's value is the next argument, whatever it looks like"
        );
        assert_eq!(
            read(&["-history-dir", "/data/a", "--", "-history-dir", "/data/b"]),
            Some(PathBuf::from("/data/a")),
            "flags end at --"
        );
        assert_eq!(
            read(&[
                "-history-dir",
                "/data/a",
                "extra",
                "-history-dir",
                "/data/b"
            ]),
            Some(PathBuf::from("/data/a")),
            "flags end at the first argument that is none"
        );
        assert_eq!(
            read(&["-history-dir", "/data/a", "-", "-history-dir", "/data/b"]),
            Some(PathBuf::from("/data/a")),
            "a lone dash is no flag"
        );
        assert_eq!(read(&["-history-dir="]), None, "an empty one names none");
        assert_eq!(
            read(&["---history-dir", "/data/a"]),
            None,
            "the shell refuses what it cannot read"
        );
    }

    #[test]
    fn a_command_line_keeps_its_empty_arguments() {
        assert_eq!(
            shell_history(b"/usr/bin/verso\0-alert-url\0\0-history-dir\0/data/x\0"),
            Some(PathBuf::from("/data/x")),
            "an empty argument is a flag's value"
        );
        assert_eq!(
            shell_history(b"/usr/bin/verso\0-history-dir\0\0-probe\0/data/x\0"),
            None
        );
        assert_eq!(
            shell_history(b"/usr/bin/verso\0-history-dir\0/data/x"),
            Some(PathBuf::from("/data/x"))
        );
        assert_eq!(
            shell_history(b"/usr/bin/other\0-history-dir\0/data/x\0"),
            None
        );
        assert_eq!(shell_history(b""), None);
    }

    // A directory is held open only through no link at its last name, read
    // as a path or through a held directory's open as clear reads it, and a
    // FIFO is refused without its open waiting for a writer.
    #[test]
    fn a_held_directory_is_never_a_link_or_a_fifo() {
        let stand = Stand::new("pinned");
        stand.dir("/d");
        stand.file("/outside/x", "x");
        let id = |path: &str| {
            let meta = fs::symlink_metadata(stand.at(path)).unwrap();
            (meta.dev(), meta.ino())
        };
        let outside = id("/outside");
        symlink(stand.at("/outside"), stand.at("/d/link")).unwrap();
        assert!(Pinned::open(&stand.at("/d/link"), outside).is_err());
        let held = Pinned::open(&stand.at("/d"), id("/d")).unwrap().unwrap();
        assert!(Pinned::open(&held.at(OsStr::new("link")), outside).is_err());

        assert!(Command::new("mkfifo")
            .arg(stand.at("/d/fifo"))
            .status()
            .unwrap()
            .success());
        let (fifo, fifo_id) = (stand.at("/d/fifo"), id("/d/fifo"));
        let (opened, waited) = std::sync::mpsc::channel();
        std::thread::spawn(move || {
            let _ = opened.send(Pinned::open(&fifo, fifo_id).is_err());
        });
        assert_eq!(waited.recv_timeout(PATIENT), Ok(true), "a FIFO is refused at once");
    }
}
