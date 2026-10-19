// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! The browser's local wall-clock time, `YYYY-MM-DDTHH:MM:SS`, as the clock is
//! set from it. The System plugin checks it before it asks, and verso-rpcd
//! (which includes this file by path) checks it again at the root boundary
//! before BusyBox `date` sees it.

/// normalize_local_datetime reads a calendar date from 1970 to 9999 and a time
/// of day, and writes it as `date -s` takes it; anything else is None.
pub fn normalize_local_datetime(value: &str) -> Option<String> {
    let bytes = value.as_bytes();
    if bytes.len() != 19
        || bytes[4] != b'-'
        || bytes[7] != b'-'
        || bytes[10] != b'T'
        || bytes[13] != b':'
        || bytes[16] != b':'
    {
        return None;
    }
    let number = |from: usize, to: usize| value[from..to].parse::<u32>().ok();
    let (year, month, day, hour, minute, second) = (
        number(0, 4)?,
        number(5, 7)?,
        number(8, 10)?,
        number(11, 13)?,
        number(14, 16)?,
        number(17, 19)?,
    );
    if !(1970..=9999).contains(&year)
        || !(1..=12).contains(&month)
        || hour > 23
        || minute > 59
        || second > 59
    {
        return None;
    }
    let leap = year % 4 == 0 && (year % 100 != 0 || year % 400 == 0);
    let days = match month {
        2 if leap => 29,
        2 => 28,
        4 | 6 | 9 | 11 => 30,
        _ => 31,
    };
    if !(1..=days).contains(&day) {
        return None;
    }
    Some(format!(
        "{year:04}-{month:02}-{day:02} {hour:02}:{minute:02}:{second:02}"
    ))
}

#[cfg(test)]
mod tests {
    use super::normalize_local_datetime;

    #[test]
    fn a_real_day_is_written_as_date_takes_it() {
        assert_eq!(
            normalize_local_datetime("2024-02-29T23:59:59").as_deref(),
            Some("2024-02-29 23:59:59")
        );
    }

    #[test]
    fn anything_but_a_real_day_and_time_is_refused() {
        for s in [
            "2026-02-29T00:00:00",
            "2026-04-31T12:00:00",
            "2026-01-01T25:00:00",
            "2026-01-01T00:00:00;reboot",
        ] {
            assert!(normalize_local_datetime(s).is_none(), "{s}");
        }
    }
}
