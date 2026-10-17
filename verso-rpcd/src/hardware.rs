// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! What the board says of itself that only root may read: the backup battery
//! of the clock that set the system time at boot, through that RTC's own
//! ioctl, and the variables of U-Boot's environment that say which slot it
//! boots. The shell turns them into facts; nothing here grades them.

use serde_json::{json, Map, Value};
use std::ffi::c_ulong;
use std::fs::{self, File};
use std::os::fd::AsRawFd;
use std::path::Path;
use std::process::Command;
use std::time::Duration;

/// RTC_VL_READ: _IOR('p', 0x13, unsigned int).
const RTC_VL_READ: c_ulong = 0x8004_7013;
const RTC_VL_BACKUP_LOW: u32 = 1 << 1;
const RTC_VL_BACKUP_EMPTY: u32 = 1 << 2;

/// fw_printenv reads a few kilobytes of flash; it never takes long.
const PRINTENV_TIMEOUT: Duration = Duration::from_secs(5);
const PRINTENV_MAX: usize = 64 * 1024;

unsafe extern "C" {
    fn ioctl(fd: i32, request: c_ulong, ...) -> i32;
}

/// state is the clock (null on a board with no clock that keeps time) and
/// the boot slot's variables (empty where U-Boot's environment says none).
pub fn state() -> Value {
    json!({
        "clock": clock(Path::new("/sys/class/rtc"), Path::new("/dev")),
        "boot": boot_env(&printenv()),
    })
}

/// clock names the RTC that set the system time at boot and what it says of
/// its backup battery; a clock that does not say has no battery.
fn clock(class: &Path, dev: &Path) -> Value {
    let Some(name) = system_clock(class) else {
        return Value::Null;
    };
    let battery = File::open(dev.join(&name))
        .ok()
        .and_then(|file| vl_read(&file))
        .map(battery);
    json!({"part": clock_part(&class.join(&name)), "battery": battery})
}

/// system_clock is the RTC the kernel set the system time from (hctosys).
fn system_clock(class: &Path) -> Option<String> {
    let mut names: Vec<String> = fs::read_dir(class)
        .ok()?
        .filter_map(Result::ok)
        .map(|entry| entry.file_name().to_string_lossy().into_owned())
        .collect();
    names.sort();
    names.into_iter().find(|name| {
        fs::read_to_string(class.join(name).join("hctosys")).is_ok_and(|v| v.trim() == "1")
    })
}

/// clock_part names the chip as its device tree's compatible does
/// ("nxp,pcf2131" is "pcf2131"), else as the RTC registered itself
/// ("rtc_cmos").
fn clock_part(dir: &Path) -> String {
    if let Ok(compatible) = fs::read(dir.join("device/of_node/compatible")) {
        let first = compatible.split(|b| *b == 0).next().unwrap_or_default();
        if let Some((_, model)) = String::from_utf8_lossy(first).split_once(',') {
            if !model.is_empty() {
                return model.to_string();
            }
        }
    }
    fs::read_to_string(dir.join("name"))
        .ok()
        .and_then(|name| name.split_whitespace().next().map(str::to_string))
        .unwrap_or_default()
}

fn vl_read(file: &File) -> Option<u32> {
    let mut flags: u32 = 0;
    // SAFETY: RTC_VL_READ writes one unsigned int through the pointer, which
    // outlives the call; the descriptor is open for its duration.
    let status = unsafe { ioctl(file.as_raw_fd(), RTC_VL_READ, &mut flags as *mut u32) };
    (status == 0).then_some(flags)
}

fn battery(flags: u32) -> &'static str {
    if flags & RTC_VL_BACKUP_EMPTY != 0 {
        "empty"
    } else if flags & RTC_VL_BACKUP_LOW != 0 {
        "low"
    } else {
        "good"
    }
}

fn printenv() -> String {
    crate::run_bounded_within(
        Command::new("fw_printenv"),
        "fw_printenv",
        PRINTENV_MAX,
        PRINTENV_TIMEOUT,
    )
    .map(|output| String::from_utf8_lossy(&output.stdout).into_owned())
    .unwrap_or_default()
}

/// boot_env keeps the variables that say which slot U-Boot boots and how
/// its fallback stands: the slot, its boot count against its limit, whether a
/// new image is on trial, and each slot's partitions. Nothing else of the
/// environment leaves.
fn boot_env(printed: &str) -> Value {
    let mut env = Map::new();
    for line in printed.lines() {
        let Some((key, value)) = line.split_once('=') else {
            continue;
        };
        if matches!(
            key,
            "slot" | "bootcount" | "bootlimit" | "upgrade_available"
        ) || key.starts_with("set_slot_")
        {
            env.insert(key.to_string(), Value::String(value.to_string()));
        }
    }
    Value::Object(env)
}

#[cfg(test)]
mod tests {
    use super::*;

    fn write(path: &Path, body: &[u8]) {
        fs::create_dir_all(path.parent().unwrap()).unwrap();
        fs::write(path, body).unwrap();
    }

    fn scratch(name: &str) -> std::path::PathBuf {
        let dir =
            std::env::temp_dir().join(format!("verso-hardware-{name}-{}", std::process::id()));
        let _ = fs::remove_dir_all(&dir);
        dir
    }

    #[test]
    fn the_clock_is_the_one_that_set_the_system_time() {
        let class = scratch("hctosys");
        write(&class.join("rtc0/hctosys"), b"1\n");
        write(&class.join("rtc0/name"), b"rtc-pcf2127-i2c 14-0053\n");
        write(
            &class.join("rtc0/device/of_node/compatible"),
            b"nxp,pcf2131\0",
        );
        write(&class.join("rtc1/hctosys"), b"0\n");
        write(&class.join("rtc1/name"), b"ftm-alarm 29d0000.rtc\n");
        assert_eq!(system_clock(&class).as_deref(), Some("rtc0"));
        assert_eq!(clock_part(&class.join("rtc0")), "pcf2131");
        // A clock with no device tree is named as it registered itself.
        write(&class.join("rtc2/name"), b"rtc_cmos\n");
        assert_eq!(clock_part(&class.join("rtc2")), "rtc_cmos");
        // No device node to ask: the clock stands, its battery unsaid.
        let clock = clock(&class, &class.join("no-dev"));
        assert_eq!(clock, json!({"part": "pcf2131", "battery": null}));
        let _ = fs::remove_dir_all(&class);
    }

    #[test]
    fn no_clock_set_the_time_is_no_clock() {
        let class = scratch("none");
        write(&class.join("rtc0/hctosys"), b"0\n");
        assert_eq!(clock(&class, Path::new("/nonexistent")), Value::Null);
        let _ = fs::remove_dir_all(&class);
    }

    #[test]
    fn the_battery_reads_its_worst_flag() {
        assert_eq!(battery(0), "good");
        assert_eq!(battery(RTC_VL_BACKUP_LOW), "low");
        assert_eq!(battery(RTC_VL_BACKUP_LOW | RTC_VL_BACKUP_EMPTY), "empty");
        assert_eq!(battery(1), "good"); // the time invalid says nothing of the battery
    }

    #[test]
    fn only_the_slot_variables_leave_the_environment() {
        let printed = "bootcmd=run set_slot_${slot}; sysboot mmc 0:${bootpart} any\n\
                       bootlimit=3\nbootcount=0\nslot=b\nupgrade_available=0\n\
                       set_slot_a=setenv bootpart 1; setenv rootpart 2\n\
                       set_slot_b=setenv bootpart 3; setenv rootpart 4\n\
                       ethaddr=00:11:22:33:44:55\nbroken line\n";
        assert_eq!(
            boot_env(printed),
            json!({
                "bootlimit": "3", "bootcount": "0", "slot": "b", "upgrade_available": "0",
                "set_slot_a": "setenv bootpart 1; setenv rootpart 2",
                "set_slot_b": "setenv bootpart 3; setenv rootpart 4",
            })
        );
        assert_eq!(boot_env(""), json!({}));
    }
}
