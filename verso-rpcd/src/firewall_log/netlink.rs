// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! Linux's documented NFLOG wire protocol. Copies are capped at SNAPLEN bytes;
//! only header metadata is retained and sent to the shell.

use super::{now_nanos, GROUP, SNAPLEN};
use std::ffi::{c_void, CStr};
use std::io;
use std::net::{Ipv4Addr, Ipv6Addr};
use std::os::fd::{AsRawFd, FromRawFd, OwnedFd};

#[repr(C)]
struct Address {
    family: u16,
    pad: u16,
    pid: u32,
    groups: u32,
}
unsafe extern "C" {
    fn socket(family: i32, kind: i32, protocol: i32) -> i32;
    fn bind(fd: i32, addr: *const Address, len: u32) -> i32;
    fn sendto(
        fd: i32,
        data: *const c_void,
        len: usize,
        flags: i32,
        addr: *const Address,
        addrlen: u32,
    ) -> isize;
    fn recvfrom(
        fd: i32,
        data: *mut c_void,
        len: usize,
        flags: i32,
        addr: *mut Address,
        addrlen: *mut u32,
    ) -> isize;
    fn setsockopt(fd: i32, level: i32, option: i32, value: *const c_void, len: u32) -> i32;
    fn if_indextoname(index: u32, name: *mut std::ffi::c_char) -> *mut std::ffi::c_char;
    fn poll(fds: *mut PollFd, count: usize, timeout: i32) -> i32;
}
#[repr(C)]
struct PollFd {
    fd: i32,
    events: i16,
    revents: i16,
}

pub(super) struct Socket(OwnedFd);

// Check support before changing fw4's generator. The module publishes this
// per-netns proc entry when loaded; probing an occupied group would return
// EPERM and would incorrectly fail every reload while our collector is running.
pub(super) fn ensure_supported() -> Result<(), String> {
    let loaded = || std::path::Path::new("/proc/net/netfilter/nfnetlink_log").is_file();
    if loaded() {
        return Ok(());
    }
    // Opening/configuring the socket requests module autoload on first use.
    match Socket::open() {
        Ok(_) => Ok(()),
        Err(_) if loaded() => Ok(()),
        Err(error) => Err(error),
    }
}

impl Socket {
    pub(super) fn open() -> Result<Self, String> {
        // AF_NETLINK, SOCK_RAW|SOCK_CLOEXEC, NETLINK_NETFILTER.
        let fd = unsafe { socket(16, 3 | 0x80000, 12) };
        if fd < 0 {
            return Err(io::Error::last_os_error().to_string());
        }
        let socket = Self(unsafe { OwnedFd::from_raw_fd(fd) });
        let local = Address {
            family: 16,
            pad: 0,
            pid: 0,
            groups: 0,
        };
        if unsafe { bind(fd, &local, 12) } < 0 {
            return Err(io::Error::last_os_error().to_string());
        }
        let size = 256 * 1024i32;
        unsafe { setsockopt(fd, 1, 8, (&size as *const i32).cast(), 4) };
        // Bind only our group. Never unbind the protocol family or another
        // consumer: ulogd and other collectors can coexist in their own groups.
        socket.configure(1, &[1], 1)?;
        let mut mode = SNAPLEN.to_be_bytes().to_vec();
        mode.extend_from_slice(&[2, 0]);
        socket.configure(2, &mode, 2)?;
        socket.configure(4, &1u32.to_be_bytes(), 3)?;
        socket.configure(5, &1u32.to_be_bytes(), 4)?;
        Ok(socket)
    }
    fn configure(&self, attribute: u16, value: &[u8], sequence: u32) -> Result<(), String> {
        let mut message = vec![0u8; 20];
        message[4..6].copy_from_slice(&0x401u16.to_ne_bytes());
        message[6..8].copy_from_slice(&5u16.to_ne_bytes()); // request + ACK
        message[8..12].copy_from_slice(&sequence.to_ne_bytes());
        message[18..20].copy_from_slice(&GROUP.to_be_bytes());
        message.extend_from_slice(&((4 + value.len()) as u16).to_ne_bytes());
        message.extend_from_slice(&attribute.to_ne_bytes());
        message.extend_from_slice(value);
        while !message.len().is_multiple_of(4) {
            message.push(0);
        }
        let len = message.len() as u32;
        message[..4].copy_from_slice(&len.to_ne_bytes());
        let kernel = Address {
            family: 16,
            pad: 0,
            pid: 0,
            groups: 0,
        };
        if unsafe {
            sendto(
                self.0.as_raw_fd(),
                message.as_ptr().cast(),
                message.len(),
                0,
                &kernel,
                12,
            )
        } < 0
        {
            return Err(io::Error::last_os_error().to_string());
        }
        let mut wait = PollFd {
            fd: self.0.as_raw_fd(),
            events: 1,
            revents: 0,
        };
        let mut response = [0u8; 4096];
        let deadline = std::time::Instant::now() + std::time::Duration::from_secs(2);
        while std::time::Instant::now() < deadline {
            if unsafe { poll(&mut wait, 1, 200) } <= 0 {
                continue;
            }
            let len = self.receive(&mut response).map_err(|e| e.to_string())?;
            for item in messages(&response[..len]) {
                if item.len() >= 20
                    && native_u16(&item[4..]) == 2
                    && native_u32(&item[8..]) == sequence
                {
                    let error = i32::from_ne_bytes(item[16..20].try_into().unwrap());
                    return if error == 0 {
                        Ok(())
                    } else {
                        Err(io::Error::from_raw_os_error(-error).to_string())
                    };
                }
            }
        }
        Err("NFLOG configuration timed out".into())
    }
    pub(super) fn receive(&self, buffer: &mut [u8]) -> io::Result<usize> {
        let mut peer = Address {
            family: 0,
            pad: 0,
            pid: 0,
            groups: 0,
        };
        let mut length = 12;
        let result = unsafe {
            recvfrom(
                self.0.as_raw_fd(),
                buffer.as_mut_ptr().cast(),
                buffer.len(),
                0x20,
                &mut peer,
                &mut length,
            )
        }; // MSG_TRUNC
        if result < 0 {
            return Err(io::Error::last_os_error());
        }
        if result as usize > buffer.len() {
            return Err(io::Error::from_raw_os_error(105));
        }
        if peer.pid != 0 {
            return Ok(0);
        } // only kernel-originated messages
        Ok(result as usize)
    }
}

fn native_u16(data: &[u8]) -> u16 {
    u16::from_ne_bytes(data[..2].try_into().unwrap())
}
fn native_u32(data: &[u8]) -> u32 {
    u32::from_ne_bytes(data[..4].try_into().unwrap())
}
fn big_u32(data: &[u8]) -> u32 {
    u32::from_be_bytes(data[..4].try_into().unwrap())
}

fn messages(mut bytes: &[u8]) -> Vec<&[u8]> {
    let mut result = Vec::new();
    while bytes.len() >= 16 {
        let length = native_u32(bytes) as usize;
        if length < 16 || length > bytes.len() {
            break;
        }
        result.push(&bytes[..length]);
        let next = (length + 3) & !3;
        if next > bytes.len() {
            break;
        }
        bytes = &bytes[next..];
    }
    result
}

pub(super) fn decode(bytes: &[u8]) -> Vec<(u64, String)> {
    let mut result = Vec::new();
    for message in messages(bytes) {
        if message.len() < 20
            || native_u16(&message[4..]) != 0x400
            || u16::from_be_bytes([message[18], message[19]]) != GROUP
        {
            continue;
        }
        let mut data = &message[20..];
        let mut prefix = String::new();
        let mut input = 0;
        let mut output = 0;
        let mut payload = None;
        let mut timestamp = (now_nanos() / 1_000_000) as u64;
        let mut valid = true;
        while data.len() >= 4 {
            let length = native_u16(data) as usize;
            if length < 4 || length > data.len() {
                valid = false;
                break;
            }
            let kind = native_u16(&data[2..]) & 0x3fff;
            let value = &data[4..length];
            match kind {
                3 if value.len() >= 16 => {
                    let sec = u64::from_be_bytes(value[..8].try_into().unwrap());
                    let usec = u64::from_be_bytes(value[8..16].try_into().unwrap());
                    timestamp = sec.saturating_mul(1000).saturating_add(usec / 1000);
                }
                4 if value.len() >= 4 => input = big_u32(value),
                5 if value.len() >= 4 => output = big_u32(value),
                9 => payload = Some(value),
                10 => {
                    prefix =
                        String::from_utf8_lossy(value.split(|b| *b == 0).next().unwrap_or_default())
                            .chars()
                            .take(128)
                            .filter(|c| !c.is_control())
                            .collect()
                }
                _ => {}
            }
            let next = (length + 3) & !3;
            if next > data.len() {
                break;
            }
            data = &data[next..];
        }
        if valid {
            if let Some(fields) = payload.and_then(packet_fields) {
                result.push((
                    timestamp,
                    format!(
                        "{prefix} IN={} OUT={} {fields}",
                        interface_name(input),
                        interface_name(output)
                    ),
                ));
            }
        }
    }
    result
}

fn interface_name(index: u32) -> String {
    if index == 0 {
        return String::new();
    }
    let mut name = [0 as std::ffi::c_char; 16];
    if unsafe { if_indextoname(index, name.as_mut_ptr()) }.is_null() {
        return format!("if{index}");
    }
    unsafe { CStr::from_ptr(name.as_ptr()) }
        .to_string_lossy()
        .into_owned()
}

pub(super) fn packet_fields(packet: &[u8]) -> Option<String> {
    let version = packet.first()? >> 4;
    let (src, dst, mut protocol, mut offset, mut fragment) = match version {
        4 if packet.len() >= 20 => {
            let length = usize::from(packet[0] & 15) * 4;
            if length < 20 || packet.len() < length {
                return None;
            }
            (
                Ipv4Addr::from(<[u8; 4]>::try_from(&packet[12..16]).ok()?).to_string(),
                Ipv4Addr::from(<[u8; 4]>::try_from(&packet[16..20]).ok()?).to_string(),
                packet[9],
                length,
                (u16::from_be_bytes([packet[6], packet[7]]) & 0x1fff) != 0,
            )
        }
        6 if packet.len() >= 40 => (
            Ipv6Addr::from(<[u8; 16]>::try_from(&packet[8..24]).ok()?).to_string(),
            Ipv6Addr::from(<[u8; 16]>::try_from(&packet[24..40]).ok()?).to_string(),
            packet[6],
            40,
            false,
        ),
        _ => return None,
    };
    if version == 6 {
        for _ in 0..8 {
            if !matches!(protocol, 0 | 43 | 44 | 51 | 60) {
                break;
            }
            let Some(header) = packet.get(offset..offset + 2) else {
                return Some(format!("SRC={src} DST={dst} PROTO={protocol}"));
            };
            let length = match protocol {
                44 => 8,
                51 => (usize::from(header[1]) + 2) * 4,
                _ => (usize::from(header[1]) + 1) * 8,
            };
            if packet.len() < offset + length {
                return Some(format!("SRC={src} DST={dst} PROTO={protocol}"));
            }
            if protocol == 44 {
                fragment =
                    (u16::from_be_bytes([packet[offset + 2], packet[offset + 3]]) & 0xfff8) != 0;
            }
            protocol = header[0];
            offset += length;
            if fragment {
                break;
            }
        }
    }
    let name = match protocol {
        1 => "ICMP".into(),
        6 => "TCP".into(),
        17 => "UDP".into(),
        58 => "ICMPv6".into(),
        132 => "SCTP".into(),
        _ => protocol.to_string(),
    };
    let mut fields = format!("SRC={src} DST={dst} PROTO={name}");
    if !fragment {
        if let Some(ports) = packet
            .get(offset..offset + 4)
            .filter(|_| matches!(protocol, 6 | 17 | 132))
        {
            fields.push_str(&format!(
                " SPT={} DPT={}",
                u16::from_be_bytes([ports[0], ports[1]]),
                u16::from_be_bytes([ports[2], ports[3]])
            ));
        } else if let Some(icmp) = packet
            .get(offset..offset + 2)
            .filter(|_| matches!(protocol, 1 | 58))
        {
            fields.push_str(&format!(" TYPE={} CODE={}", icmp[0], icmp[1]));
        }
    }
    Some(fields)
}
