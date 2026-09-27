// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! A package installed from Verso adds a setting; it never changes what the
//! router does. Two packages a page offers would, on their own defaults, the
//! moment they land: https-dns-proxy points every dnsmasq at itself, takes
//! over the LAN's DNS and blocks iCloud Private Relay, and adblock starts
//! blocking. Each arrives off instead. The page that offered it shows the
//! setting unticked, and turning it on is a change staged, applied and rolled
//! back like any other (ADR-010).

use std::process::Command;

/// One command of an arrival.
#[derive(Debug, PartialEq, Eq)]
pub enum Step {
    /// Runs once and must succeed.
    Run(&'static [&'static str]),
    /// Runs until it fails: the removal of every copy of something.
    Drain(&'static [&'static str]),
    /// Runs until it succeeds, a second apart, for as long as a package's
    /// own first run may take: a package refuses while it is under way.
    Await(&'static [&'static str]),
}

/// How many seconds an Await step waits at most. The shell gives an install
/// 90 seconds in all, and apk takes some of them.
const AWAIT_TRIES: usize = 50;

/// What turns a package's own defaults off once it has landed.
pub struct Arrival {
    pub package: &'static str,
    pub steps: &'static [Step],
}

pub const ARRIVALS: &[Arrival] = &[
    Arrival {
        package: "https-dns-proxy",
        steps: &[
            // Stopping hands dnsmasq back the servers it had before.
            Step::Run(&["/etc/init.d/https-dns-proxy", "stop"]),
            Step::Run(&[
                "/sbin/uci",
                "set",
                "https-dns-proxy.config.dnsmasq_config_update=-",
            ]),
            Step::Run(&["/sbin/uci", "set", "https-dns-proxy.config.force_dns=0"]),
            Step::Run(&[
                "/sbin/uci",
                "set",
                "https-dns-proxy.config.canary_domains_icloud=0",
            ]),
            Step::Run(&[
                "/sbin/uci",
                "set",
                "https-dns-proxy.config.canary_domains_mozilla=0",
            ]),
            // The page keeps one resolver; the package ships a second (Google).
            Step::Drain(&[
                "/sbin/uci",
                "-q",
                "delete",
                "https-dns-proxy.@https-dns-proxy[1]",
            ]),
            Step::Run(&["/sbin/uci", "commit", "https-dns-proxy"]),
            // Running idle, so an applied change reaches it through its trigger.
            Step::Run(&["/etc/init.d/https-dns-proxy", "start"]),
        ],
    },
    Arrival {
        package: "adblock",
        steps: &[
            Step::Run(&["/sbin/uci", "set", "adblock.global.adb_enabled=0"]),
            Step::Run(&["/sbin/uci", "commit", "adblock"]),
            // Refused while the run the install started loads its lists, then
            // clears them.
            Step::Await(&["/etc/init.d/adblock", "reload"]),
        ],
    },
];

/// The arrivals of the packages present now that were absent before.
pub fn due(absent_before: &[&str], present: impl Fn(&str) -> bool) -> Vec<&'static Arrival> {
    ARRIVALS
        .iter()
        .filter(|a| absent_before.contains(&a.package) && present(a.package))
        .collect()
}

/// The packages with an arrival that aren't installed.
pub fn absent(present: impl Fn(&str) -> bool) -> Vec<&'static str> {
    ARRIVALS
        .iter()
        .map(|a| a.package)
        .filter(|p| !present(p))
        .collect()
}

pub fn installed(package: &str) -> bool {
    Command::new("apk")
        .args(["info", "-e", package])
        .output()
        .is_ok_and(|o| o.status.success())
}

/// Runs an arrival's steps in order, stopping at the first that fails.
pub fn settle(
    arrival: &Arrival,
    run: impl Fn(&[&str]) -> Result<(), String>,
    pause: impl Fn(),
) -> Result<(), String> {
    let still_on =
        |error: String| format!("{} was installed but is still on: {error}", arrival.package);
    for step in arrival.steps {
        match step {
            Step::Run(argv) => run(argv).map_err(still_on)?,
            Step::Drain(argv) => {
                // A package ships a handful of copies at most.
                for _ in 0..16 {
                    if run(argv).is_err() {
                        break;
                    }
                }
            }
            Step::Await(argv) => {
                let mut outcome = run(argv);
                for _ in 1..AWAIT_TRIES {
                    if outcome.is_ok() {
                        break;
                    }
                    pause();
                    outcome = run(argv);
                }
                outcome.map_err(still_on)?;
            }
        }
    }
    Ok(())
}

pub fn pause() {
    std::thread::sleep(std::time::Duration::from_secs(1));
}

pub fn run(argv: &[&str]) -> Result<(), String> {
    let output = Command::new(argv[0])
        .args(&argv[1..])
        .output()
        .map_err(|error| format!("{}: {error}", argv.join(" ")))?;
    if output.status.success() {
        Ok(())
    } else {
        Err(format!(
            "{}: {}",
            argv.join(" "),
            String::from_utf8_lossy(&output.stderr).trim()
        ))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::cell::RefCell;

    fn arrival(package: &str) -> &'static Arrival {
        ARRIVALS.iter().find(|a| a.package == package).unwrap()
    }

    // Only a package that landed now arrives off: one that was already there
    // keeps whatever its owner set.
    #[test]
    fn only_a_package_that_just_landed_is_turned_off() {
        let due = due(&["adblock"], |_| true);
        assert_eq!(
            due.iter().map(|a| a.package).collect::<Vec<_>>(),
            ["adblock"]
        );
        assert!(super::due(&["adblock"], |_| false).is_empty());
    }

    #[test]
    fn absent_names_only_packages_with_an_arrival() {
        assert_eq!(absent(|p| p == "adblock"), ["https-dns-proxy"]);
    }

    // The proxy is stopped before its config changes, so dnsmasq gets its own
    // servers back from the proxy's backup, and started after the commit.
    #[test]
    fn the_proxy_hands_dnsmasq_back_before_it_is_reconfigured() {
        let ran = RefCell::new(vec![]);
        settle(
            arrival("https-dns-proxy"),
            |argv| {
                ran.borrow_mut().push(argv.join(" "));
                // The package ships one extra resolver.
                let extras = ran.borrow().iter().filter(|c| c.contains("delete")).count();
                if argv.contains(&"delete") && extras > 1 {
                    return Err("no such entry".into());
                }
                Ok(())
            },
            || {},
        )
        .unwrap();
        let ran = ran.into_inner();
        assert_eq!(ran.first().unwrap(), "/etc/init.d/https-dns-proxy stop");
        assert!(
            ran.contains(&"/sbin/uci set https-dns-proxy.config.dnsmasq_config_update=-".into())
        );
        assert!(ran.contains(&"/sbin/uci set https-dns-proxy.config.force_dns=0".into()));
        assert_eq!(ran.iter().filter(|c| c.contains("delete")).count(), 2);
        assert_eq!(ran[ran.len() - 2], "/sbin/uci commit https-dns-proxy");
        assert_eq!(ran.last().unwrap(), "/etc/init.d/https-dns-proxy start");
    }

    #[test]
    fn adblock_arrives_disabled() {
        let ran = RefCell::new(vec![]);
        settle(
            arrival("adblock"),
            |argv| {
                ran.borrow_mut().push(argv.join(" "));
                Ok(())
            },
            || {},
        )
        .unwrap();
        assert_eq!(
            ran.into_inner(),
            [
                "/sbin/uci set adblock.global.adb_enabled=0",
                "/sbin/uci commit adblock",
                "/etc/init.d/adblock reload",
            ]
        );
    }

    // adblock refuses every command while the run its install started is
    // still loading lists, so the reload waits that run out.
    #[test]
    fn adblock_waits_out_its_first_run() {
        let reloads = RefCell::new(0);
        let pauses = RefCell::new(0);
        settle(
            arrival("adblock"),
            |argv| {
                if argv.contains(&"reload") {
                    *reloads.borrow_mut() += 1;
                    if *reloads.borrow() < 4 {
                        return Err(String::new());
                    }
                }
                Ok(())
            },
            || *pauses.borrow_mut() += 1,
        )
        .unwrap();
        assert_eq!((reloads.into_inner(), pauses.into_inner()), (4, 3));
    }

    #[test]
    fn a_wait_ends_and_says_the_package_is_still_on() {
        let tries = RefCell::new(0);
        let error = settle(
            arrival("adblock"),
            |argv| {
                if argv.contains(&"reload") {
                    *tries.borrow_mut() += 1;
                    return Err("busy".into());
                }
                Ok(())
            },
            || {},
        )
        .unwrap_err();
        assert_eq!(tries.into_inner(), AWAIT_TRIES);
        assert_eq!(error, "adblock was installed but is still on: busy");
    }

    #[test]
    fn a_step_that_fails_says_the_package_is_still_on() {
        let error =
            settle(arrival("adblock"), |_| Err("uci: I/O error".into()), || {}).unwrap_err();
        assert_eq!(
            error,
            "adblock was installed but is still on: uci: I/O error"
        );
    }
}
