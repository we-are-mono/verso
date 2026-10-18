// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! A netlink dump asked of the kernel with nothing but the C library's socket
//! calls: every object of one kind from any netlink family that answers one,
//! as conntrack does for per-device usage, and the message and attribute walk
//! the firewall log's NFLOG socket shares. A plugin's root half reads links
//! and addresses through the SDK's own reader (ADR-027).

use std::ffi::c_void;
use std::os::fd::{AsRawFd, FromRawFd, OwnedFd};

pub(crate) const AF_NETLINK: i32 = 16;
pub(crate) const SOCK_RAW: i32 = 3;
pub(crate) const SOCK_CLOEXEC: i32 = 0o2_000_000;
pub const NETLINK_NETFILTER: i32 = 12;

pub(crate) const NLMSG_ERROR: u16 = 2;
const NLMSG_DONE: u16 = 3;
const NLM_F_REQUEST: u16 = 0x1;
const NLM_F_DUMP: u16 = 0x300;

/// dump asks one netlink family for every object of one kind (`request`, its
/// family header after the message header) on a socket of its own, and
/// answers every message of the reply, or why the kernel refused.
pub fn dump(protocol: i32, request: u16, header: &[u8]) -> Result<Vec<Message>, String> {
    Socket::open(protocol)?.dump(request, header)
}

/// attributes walks a run of netlink attributes (struct rtattr: length, type,
/// value, padded to four bytes). The type's two top bits are flags (nested,
/// byte order) and are dropped.
pub fn attributes(mut data: &[u8]) -> impl Iterator<Item = (u16, &[u8])> {
    std::iter::from_fn(move || {
        if data.len() < 4 {
            return None;
        }
        let len = u16::from_ne_bytes([data[0], data[1]]) as usize;
        let kind = u16::from_ne_bytes([data[2], data[3]]) & 0x3fff;
        if len < 4 || len > data.len() {
            return None;
        }
        let value = &data[4..len];
        data = &data[align(len).min(data.len())..];
        Some((kind, value))
    })
}

pub(crate) fn align(len: usize) -> usize {
    (len + 3) & !3
}

/// Message is one netlink message: its type and what follows the header.
pub struct Message {
    pub kind: u16,
    pub body: Vec<u8>,
}

/// messages splits a datagram into its messages (struct nlmsghdr: length,
/// type, flags, sequence, port, then the body).
pub(crate) fn messages(mut data: &[u8]) -> Vec<Message> {
    let mut out = Vec::new();
    while data.len() >= 16 {
        let len = u32::from_ne_bytes([data[0], data[1], data[2], data[3]]) as usize;
        if len < 16 || len > data.len() {
            break;
        }
        out.push(Message { kind: u16::from_ne_bytes([data[4], data[5]]), body: data[16..len].to_vec() });
        data = &data[align(len).min(data.len())..];
    }
    out
}

/// SockaddrNl is struct sockaddr_nl: a netlink address, the kernel's when
/// every field but the family is zero.
#[repr(C)]
pub(crate) struct SockaddrNl {
    pub(crate) family: u16,
    pub(crate) pad: u16,
    pub(crate) pid: u32,
    pub(crate) groups: u32,
}

impl SockaddrNl {
    /// KERNEL is the kernel's own address, and an unbound socket's.
    pub(crate) const KERNEL: SockaddrNl = SockaddrNl { family: AF_NETLINK as u16, pad: 0, pid: 0, groups: 0 };
    pub(crate) const LEN: u32 = std::mem::size_of::<SockaddrNl>() as u32;
}

unsafe extern "C" {
    pub(crate) fn socket(family: i32, kind: i32, protocol: i32) -> i32;
    pub(crate) fn sendto(fd: i32, data: *const c_void, len: usize, flags: i32, addr: *const SockaddrNl, addrlen: u32) -> isize;
    fn recv(fd: i32, data: *mut c_void, len: usize, flags: i32) -> isize;
}

/// Socket is a netlink socket of one family, closed when dropped.
struct Socket(OwnedFd);

impl Socket {
    fn open(protocol: i32) -> Result<Socket, String> {
        // SAFETY: socket(2) with plain integer arguments; a negative return is
        // the only failure and is checked, and the descriptor is owned from here.
        let fd = unsafe { socket(AF_NETLINK, SOCK_RAW | SOCK_CLOEXEC, protocol) };
        if fd < 0 {
            return Err(format!("netlink socket: {}", std::io::Error::last_os_error()));
        }
        Ok(Socket(unsafe { OwnedFd::from_raw_fd(fd) }))
    }

    /// dump asks for every object of one kind and gathers the answer until
    /// the kernel says it is done. A refusal (an error message carrying an
    /// errno) or a failed send or receive is an error, never a short answer.
    fn dump(&self, request: u16, header: &[u8]) -> Result<Vec<Message>, String> {
        let len = 16 + header.len();
        let mut packet = Vec::with_capacity(len);
        packet.extend_from_slice(&(len as u32).to_ne_bytes());
        packet.extend_from_slice(&request.to_ne_bytes());
        packet.extend_from_slice(&(NLM_F_REQUEST | NLM_F_DUMP).to_ne_bytes());
        packet.extend_from_slice(&1u32.to_ne_bytes());
        packet.extend_from_slice(&0u32.to_ne_bytes());
        packet.extend_from_slice(header);
        // SAFETY: the buffer and the address outlive the call and their lengths
        // are the ones passed.
        let sent = unsafe {
            sendto(self.0.as_raw_fd(), packet.as_ptr().cast(), packet.len(), 0, &SockaddrNl::KERNEL, SockaddrNl::LEN)
        };
        if sent < 0 {
            return Err(format!("netlink send: {}", std::io::Error::last_os_error()));
        }
        let mut out = Vec::new();
        // The kernel sizes a dump's datagrams to the largest read it has seen,
        // up to 32 KiB, so a read this size always takes a whole one.
        let mut buf = vec![0u8; 32 * 1024];
        loop {
            // SAFETY: recv(2) writes at most buf.len() bytes into buf.
            let got = unsafe { recv(self.0.as_raw_fd(), buf.as_mut_ptr().cast(), buf.len(), 0) };
            if got < 0 {
                return Err(format!("netlink receive: {}", std::io::Error::last_os_error()));
            }
            if got == 0 {
                return Err("netlink receive: the kernel closed the dump".into());
            }
            for message in messages(&buf[..got as usize]) {
                match message.kind {
                    NLMSG_DONE => return Ok(out),
                    NLMSG_ERROR => return refusal(&message.body).map_or(Ok(out), Err),
                    _ => out.push(message),
                }
            }
        }
    }
}

/// refusal reads an error message's errno (struct nlmsgerr: a negative errno,
/// then the request it answers); zero is an acknowledgement, not an error.
fn refusal(body: &[u8]) -> Option<String> {
    let errno = i32::from_ne_bytes(body.get(..4)?.try_into().ok()?);
    (errno != 0).then(|| format!("netlink: {}", std::io::Error::from_raw_os_error(-errno)))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_truncated_message_is_read_no_further() {
        let mut data = 64u32.to_ne_bytes().to_vec();
        data.extend_from_slice(&[0u8; 12]);
        assert!(messages(&data).is_empty());
        assert_eq!(attributes(&[8, 0, 3, 0, b't']).count(), 0);
    }

    #[test]
    fn an_error_message_is_a_refusal_unless_it_acknowledges() {
        let mut body = (-1i32).to_ne_bytes().to_vec();
        body.extend_from_slice(&[0u8; 16]);
        assert!(refusal(&body).expect("refused").contains("not permitted"));
        assert_eq!(refusal(&0i32.to_ne_bytes()), None);
        assert_eq!(refusal(&[0u8; 2]), None);
    }
}
