// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! What the kernel says about the network devices, asked over rtnetlink rather
//! than read from a command's output: every link with its kind, flags and
//! counters (RTM_GETLINK), and every address on it (RTM_GETADDR). Two dumps on
//! one socket, with nothing but the C library's socket calls.

use std::collections::BTreeMap;
use std::ffi::c_void;
use std::net::{Ipv4Addr, Ipv6Addr};
use std::os::fd::{AsRawFd, FromRawFd, OwnedFd};

const AF_NETLINK: i32 = 16;
const AF_INET: u8 = 2;
const AF_INET6: u8 = 10;
const SOCK_RAW: i32 = 3;
const SOCK_CLOEXEC: i32 = 0o2_000_000;
const NETLINK_ROUTE: i32 = 0;

const NLMSG_ERROR: u16 = 2;
const NLMSG_DONE: u16 = 3;
const NLM_F_REQUEST: u16 = 0x1;
const NLM_F_DUMP: u16 = 0x300;
const RTM_NEWLINK: u16 = 16;
const RTM_GETLINK: u16 = 18;
const RTM_NEWADDR: u16 = 20;
const RTM_GETADDR: u16 = 22;

const IFLA_IFNAME: u16 = 3;
const IFLA_OPERSTATE: u16 = 16;
const IFLA_LINKINFO: u16 = 18;
const IFLA_INFO_KIND: u16 = 1;
const IFLA_STATS64: u16 = 23;
const IFA_ADDRESS: u16 = 1;
const IFA_LOCAL: u16 = 2;

const IFF_UP: u32 = 0x1;
const IFF_LOWER_UP: u32 = 0x1_0000;
/// IF_OPER_DOWN: the kernel's word for a link that carries nothing.
const OPER_DOWN: u8 = 2;

/// Link is one network device as the kernel holds it.
#[derive(Debug, Default, Clone, PartialEq)]
pub struct Link {
    pub name: String,
    /// The ARPHRD type: 65534 for a tun device, 768 and on for the kernel's
    /// own tunnels.
    pub link_type: u16,
    /// The driver behind it, where it names itself ("tun", "wireguard").
    pub kind: String,
    /// Administratively up, and with carrier: for a tun device, a process
    /// holds it open.
    pub up: bool,
    pub carrier: bool,
    pub rx: u64,
    pub tx: u64,
    pub addresses: Vec<String>,
}

/// links is every device with its addresses, by name. A kernel that cannot be
/// asked gives none.
pub fn links() -> BTreeMap<String, Link> {
    let Some(socket) = Socket::open() else {
        return BTreeMap::new();
    };
    let mut by_index = BTreeMap::new();
    for message in socket.dump(RTM_GETLINK, &[0u8; 16]) {
        if let Some((index, link)) = parse_link(&message) {
            by_index.insert(index, link);
        }
    }
    for message in socket.dump(RTM_GETADDR, &[0u8; 8]) {
        if let Some((index, address)) = parse_address(&message) {
            if let Some(link) = by_index.get_mut(&index) {
                link.addresses.push(address);
            }
        }
    }
    by_index.into_values().map(|l| (l.name.clone(), l)).collect()
}

/// parse_link reads one RTM_NEWLINK body: struct ifinfomsg, then its
/// attributes.
fn parse_link(message: &Message) -> Option<(u32, Link)> {
    if message.kind != RTM_NEWLINK || message.body.len() < 16 {
        return None;
    }
    let body = &message.body;
    let link_type = u16::from_ne_bytes([body[2], body[3]]);
    let index = u32::from_ne_bytes(body[4..8].try_into().ok()?);
    let flags = u32::from_ne_bytes(body[8..12].try_into().ok()?);
    let mut link = Link { link_type, up: flags & IFF_UP != 0, carrier: flags & IFF_LOWER_UP != 0, ..Link::default() };
    for (kind, value) in attributes(&body[16..]) {
        match kind {
            IFLA_IFNAME => link.name = text(value),
            IFLA_OPERSTATE if value.first() == Some(&OPER_DOWN) => link.carrier = false,
            IFLA_LINKINFO => {
                if let Some((_, kind)) = attributes(value).find(|(k, _)| *k == IFLA_INFO_KIND) {
                    link.kind = text(kind);
                }
            }
            // struct rtnl_link_stats64: rx_packets, tx_packets, rx_bytes, tx_bytes, …
            IFLA_STATS64 if value.len() >= 32 => {
                link.rx = u64::from_ne_bytes(value[16..24].try_into().ok()?);
                link.tx = u64::from_ne_bytes(value[24..32].try_into().ok()?);
            }
            _ => {}
        }
    }
    (!link.name.is_empty()).then_some((index, link))
}

/// parse_address reads one RTM_NEWADDR body: struct ifaddrmsg, then its
/// attributes, as `ip` writes it (address/prefix). A point-to-point link
/// carries its own end as IFA_LOCAL and the peer's as IFA_ADDRESS.
fn parse_address(message: &Message) -> Option<(u32, String)> {
    if message.kind != RTM_NEWADDR || message.body.len() < 8 {
        return None;
    }
    let body = &message.body;
    let (family, prefix) = (body[0], body[1]);
    let index = u32::from_ne_bytes(body[4..8].try_into().ok()?);
    let (mut local, mut address) = (None, None);
    for (kind, value) in attributes(&body[8..]) {
        match kind {
            IFA_LOCAL => local = Some(value),
            IFA_ADDRESS => address = Some(value),
            _ => {}
        }
    }
    let bytes = local.or(address)?;
    let ip = match (family, bytes.len()) {
        (AF_INET, 4) => Ipv4Addr::from(<[u8; 4]>::try_from(bytes).ok()?).to_string(),
        (AF_INET6, 16) => Ipv6Addr::from(<[u8; 16]>::try_from(bytes).ok()?).to_string(),
        _ => return None,
    };
    Some((index, format!("{ip}/{prefix}")))
}

/// attributes walks a run of netlink attributes (struct rtattr: length, type,
/// value, padded to four bytes).
fn attributes(mut data: &[u8]) -> impl Iterator<Item = (u16, &[u8])> {
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

fn align(len: usize) -> usize {
    (len + 3) & !3
}

fn text(value: &[u8]) -> String {
    let end = value.iter().position(|b| *b == 0).unwrap_or(value.len());
    String::from_utf8_lossy(&value[..end]).into_owned()
}

/// Message is one netlink message: its type and what follows the header.
pub struct Message {
    kind: u16,
    body: Vec<u8>,
}

/// messages splits a datagram into its messages (struct nlmsghdr: length,
/// type, flags, sequence, port, then the body).
fn messages(mut data: &[u8]) -> Vec<Message> {
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

#[repr(C)]
struct SockaddrNl {
    family: u16,
    pad: u16,
    pid: u32,
    groups: u32,
}

unsafe extern "C" {
    fn socket(family: i32, kind: i32, protocol: i32) -> i32;
    fn sendto(fd: i32, data: *const c_void, len: usize, flags: i32, addr: *const SockaddrNl, addrlen: u32) -> isize;
    fn recv(fd: i32, data: *mut c_void, len: usize, flags: i32) -> isize;
}

/// Socket is a NETLINK_ROUTE socket, closed when dropped.
struct Socket(OwnedFd);

impl Socket {
    fn open() -> Option<Socket> {
        // SAFETY: socket(2) with constant arguments; a negative return is the
        // only failure and is checked, and the descriptor is owned from here.
        let fd = unsafe { socket(AF_NETLINK, SOCK_RAW | SOCK_CLOEXEC, NETLINK_ROUTE) };
        (fd >= 0).then(|| Socket(unsafe { OwnedFd::from_raw_fd(fd) }))
    }

    /// dump asks for every object of one kind and gathers the answer until
    /// the kernel says it is done. A failed send or receive ends it with what
    /// arrived.
    fn dump(&self, request: u16, header: &[u8]) -> Vec<Message> {
        let len = 16 + header.len();
        let mut packet = Vec::with_capacity(len);
        packet.extend_from_slice(&(len as u32).to_ne_bytes());
        packet.extend_from_slice(&request.to_ne_bytes());
        packet.extend_from_slice(&(NLM_F_REQUEST | NLM_F_DUMP).to_ne_bytes());
        packet.extend_from_slice(&1u32.to_ne_bytes());
        packet.extend_from_slice(&0u32.to_ne_bytes());
        packet.extend_from_slice(header);
        let kernel = SockaddrNl { family: AF_NETLINK as u16, pad: 0, pid: 0, groups: 0 };
        // SAFETY: the buffer and the address outlive the call and their lengths
        // are the ones passed.
        let sent = unsafe {
            sendto(
                self.0.as_raw_fd(),
                packet.as_ptr().cast(),
                packet.len(),
                0,
                &kernel,
                std::mem::size_of::<SockaddrNl>() as u32,
            )
        };
        if sent < 0 {
            return Vec::new();
        }
        let mut out = Vec::new();
        let mut buf = vec![0u8; 32 * 1024];
        loop {
            // SAFETY: recv(2) writes at most buf.len() bytes into buf.
            let got = unsafe { recv(self.0.as_raw_fd(), buf.as_mut_ptr().cast(), buf.len(), 0) };
            if got <= 0 {
                return out;
            }
            for message in messages(&buf[..got as usize]) {
                match message.kind {
                    NLMSG_DONE | NLMSG_ERROR => return out,
                    _ => out.push(message),
                }
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn attribute(kind: u16, value: &[u8]) -> Vec<u8> {
        let mut out = ((4 + value.len()) as u16).to_ne_bytes().to_vec();
        out.extend_from_slice(&kind.to_ne_bytes());
        out.extend_from_slice(value);
        out.resize(align(out.len()), 0);
        out
    }

    #[test]
    fn a_link_message_names_its_device_kind_state_and_counters() {
        let mut body = vec![0u8; 16];
        body[2..4].copy_from_slice(&65534u16.to_ne_bytes());
        body[4..8].copy_from_slice(&22u32.to_ne_bytes());
        body[8..12].copy_from_slice(&(IFF_UP | IFF_LOWER_UP).to_ne_bytes());
        body.extend(attribute(IFLA_IFNAME, b"tun0\0"));
        body.extend(attribute(IFLA_LINKINFO, &attribute(IFLA_INFO_KIND, b"tun\0")));
        let mut stats = vec![0u8; 64];
        stats[16..24].copy_from_slice(&1200u64.to_ne_bytes());
        stats[24..32].copy_from_slice(&3400u64.to_ne_bytes());
        body.extend(attribute(IFLA_STATS64, &stats));
        let (index, link) = parse_link(&Message { kind: RTM_NEWLINK, body }).expect("a link");
        assert_eq!(index, 22);
        assert_eq!(
            link,
            Link { name: "tun0".into(), link_type: 65534, kind: "tun".into(), up: true, carrier: true, rx: 1200, tx: 3400, addresses: Vec::new() }
        );
    }

    #[test]
    fn a_link_the_kernel_calls_down_has_no_carrier() {
        let mut body = vec![0u8; 16];
        body[8..12].copy_from_slice(&(IFF_UP | IFF_LOWER_UP).to_ne_bytes());
        body.extend(attribute(IFLA_IFNAME, b"tun0\0"));
        body.extend(attribute(IFLA_OPERSTATE, &[OPER_DOWN]));
        let (_, link) = parse_link(&Message { kind: RTM_NEWLINK, body }).expect("a link");
        assert!(link.up && !link.carrier);
    }

    #[test]
    fn a_point_to_point_address_is_its_own_end() {
        let mut body = vec![AF_INET, 16, 0, 0];
        body.extend_from_slice(&22u32.to_ne_bytes());
        body.extend(attribute(IFA_ADDRESS, &[10, 96, 0, 1]));
        body.extend(attribute(IFA_LOCAL, &[10, 96, 0, 112]));
        assert_eq!(parse_address(&Message { kind: RTM_NEWADDR, body }), Some((22, "10.96.0.112/16".into())));
    }

    #[test]
    fn a_truncated_message_is_read_no_further() {
        let mut data = 64u32.to_ne_bytes().to_vec();
        data.extend_from_slice(&[0u8; 12]);
        assert!(messages(&data).is_empty());
        assert_eq!(attributes(&[8, 0, 3, 0, b't']).count(), 0);
    }

    /// The kernel this runs on answers: loopback is always there, up, with
    /// its address.
    #[test]
    fn the_kernel_answers_with_loopback() {
        let links = links();
        let lo = links.get("lo").expect("loopback");
        assert!(lo.up);
        assert!(lo.addresses.iter().any(|a| a == "127.0.0.1/8"), "{lo:?}");
    }
}
