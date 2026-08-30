// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

//! Minimal native ubus client used only for `session.access` authorization.
//! Keeping the probe in-process means the persistent helper never shells out to
//! `ubus` and never trusts the unprivileged shell's claim that a session may act.

use std::io::{self, Read, Write};
use std::os::unix::net::UnixStream;
use std::time::Duration;

const DEFAULT_SOCKET: &str = "/var/run/ubus/ubus.sock";
const MAX_MESSAGE: usize = 1 << 24;

const MSG_HELLO: u8 = 0;
const MSG_STATUS: u8 = 1;
const MSG_DATA: u8 = 2;
const MSG_LOOKUP: u8 = 4;
const MSG_INVOKE: u8 = 5;

const ATTR_STATUS: u8 = 1;
const ATTR_OBJ_PATH: u8 = 2;
const ATTR_OBJ_ID: u8 = 3;
const ATTR_METHOD: u8 = 4;
const ATTR_DATA: u8 = 7;

const BLOBMSG_STRING: u8 = 3;
const BLOBMSG_INT32: u8 = 5;
const BLOBMSG_INT16: u8 = 6;
const BLOBMSG_INT8: u8 = 7;

const ATTR_EXTENDED: u32 = 0x8000_0000;
const ATTR_ID_SHIFT: u32 = 24;
const ATTR_LEN_MASK: u32 = 0x00ff_ffff;

#[derive(Debug)]
pub struct Error(String);

impl std::fmt::Display for Error {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.write_str(&self.0)
    }
}

impl std::error::Error for Error {}

impl From<io::Error> for Error {
    fn from(value: io::Error) -> Self {
        Self(value.to_string())
    }
}

struct Client {
    stream: UnixStream,
    local_id: u32,
    sequence: u16,
}

#[derive(Clone, Copy)]
struct Header {
    kind: u8,
    peer: u32,
}

#[derive(Clone, Copy)]
struct Attr<'a> {
    id: u8,
    payload: &'a [u8],
}

/// Reports whether `sid` may invoke `verso.<method>` according to rpcd's ACLs.
/// Every error fails closed at the caller.
pub fn access(sid: &str, method: &str) -> Result<bool, Error> {
    if sid.is_empty() {
        return Ok(false);
    }
    let mut client = Client::connect(DEFAULT_SOCKET)?;
    let session = client.lookup("session")?;
    client.invoke_access(session, sid, method)
}

impl Client {
    fn connect(path: &str) -> Result<Self, Error> {
        let stream =
            UnixStream::connect(path).map_err(|error| Error(format!("connect {path}: {error}")))?;
        let timeout = Some(Duration::from_secs(5));
        stream.set_read_timeout(timeout)?;
        stream.set_write_timeout(timeout)?;
        let mut client = Self {
            stream,
            local_id: 0,
            sequence: 0,
        };
        let (header, _) = client.read_message()?;
        if header.kind != MSG_HELLO {
            return Err(Error(format!(
                "expected ubus HELLO, received type {}",
                header.kind
            )));
        }
        client.local_id = header.peer;
        Ok(client)
    }

    fn lookup(&mut self, name: &str) -> Result<u32, Error> {
        let mut message = Message::default();
        message.put_string(ATTR_OBJ_PATH, name);
        self.send(MSG_LOOKUP, 0, &message.finish())?;

        let mut object_id = None;
        loop {
            let (header, body) = self.read_message()?;
            match header.kind {
                MSG_DATA => {
                    for attr in container(&body)? {
                        if attr.id == ATTR_OBJ_ID && attr.payload.len() >= 4 {
                            object_id = Some(u32::from_be_bytes(
                                attr.payload[..4].try_into().expect("four bytes"),
                            ));
                        }
                    }
                }
                MSG_STATUS => {
                    let status = status_code(&body)?;
                    if status != 0 {
                        return Err(Error(format!("lookup {name:?}: ubus status {status}")));
                    }
                    return object_id
                        .ok_or_else(|| Error(format!("ubus object {name:?} not found")));
                }
                _ => {}
            }
        }
    }

    fn invoke_access(&mut self, object_id: u32, sid: &str, method: &str) -> Result<bool, Error> {
        let table = encode_string_table(&[
            ("ubus_rpc_session", sid),
            ("scope", "ubus"),
            ("object", "verso"),
            ("function", method),
        ]);
        let mut message = Message::default();
        message.put_u32(ATTR_OBJ_ID, object_id);
        message.put_string(ATTR_METHOD, "access");
        message.put(ATTR_DATA, false, &table);
        self.send(MSG_INVOKE, 0, &message.finish())?;

        let mut allowed = false;
        loop {
            let (header, body) = self.read_message()?;
            match header.kind {
                MSG_DATA => {
                    for attr in container(&body)? {
                        if attr.id == ATTR_DATA {
                            allowed = decode_access(attr.payload)?;
                        }
                    }
                }
                MSG_STATUS => {
                    let status = status_code(&body)?;
                    if status != 0 {
                        return Err(Error(format!("session.access: ubus status {status}")));
                    }
                    return Ok(allowed);
                }
                _ => {}
            }
        }
    }

    fn send(&mut self, kind: u8, peer: u32, body: &[u8]) -> Result<(), Error> {
        self.sequence = self.sequence.wrapping_add(1);
        let mut packet = Vec::with_capacity(8 + body.len());
        packet.extend_from_slice(&[0, kind]);
        packet.extend_from_slice(&self.sequence.to_be_bytes());
        packet.extend_from_slice(&peer.to_be_bytes());
        packet.extend_from_slice(body);
        self.stream.write_all(&packet)?;
        Ok(())
    }

    fn read_message(&mut self) -> Result<(Header, Vec<u8>), Error> {
        let mut header = [0u8; 8];
        self.stream.read_exact(&mut header)?;
        if header[0] != 0 {
            return Err(Error(format!("unsupported ubus version {}", header[0])));
        }
        let parsed = Header {
            kind: header[1],
            peer: u32::from_be_bytes(header[4..8].try_into().expect("four bytes")),
        };
        let mut length = [0u8; 4];
        self.stream.read_exact(&mut length)?;
        let raw_len = (u32::from_be_bytes(length) & ATTR_LEN_MASK) as usize;
        if !(4..=MAX_MESSAGE).contains(&raw_len) {
            return Err(Error(format!("ubus message length {raw_len} is invalid")));
        }
        let mut body = vec![0u8; raw_len];
        body[..4].copy_from_slice(&length);
        self.stream.read_exact(&mut body[4..])?;
        Ok((parsed, body))
    }
}

#[derive(Default)]
struct Message {
    children: Vec<u8>,
}

impl Message {
    fn put_string(&mut self, id: u8, value: &str) {
        let mut bytes = value.as_bytes().to_vec();
        bytes.push(0);
        self.put(id, false, &bytes);
    }

    fn put_u32(&mut self, id: u8, value: u32) {
        self.put(id, false, &value.to_be_bytes());
    }

    fn put(&mut self, id: u8, extended: bool, payload: &[u8]) {
        append_attr(&mut self.children, id, extended, payload);
    }

    fn finish(self) -> Vec<u8> {
        let mut output = Vec::with_capacity(self.children.len() + 4);
        output.extend_from_slice(&pack_id(0, false, self.children.len() + 4).to_be_bytes());
        output.extend_from_slice(&self.children);
        output
    }
}

fn pad4(length: usize) -> usize {
    (length + 3) & !3
}

fn pack_id(id: u8, extended: bool, raw_len: usize) -> u32 {
    let mut value = (raw_len as u32) & ATTR_LEN_MASK;
    value |= u32::from(id & 0x7f) << ATTR_ID_SHIFT;
    if extended {
        value |= ATTR_EXTENDED;
    }
    value
}

fn append_attr(output: &mut Vec<u8>, id: u8, extended: bool, payload: &[u8]) {
    let raw_len = payload.len() + 4;
    output.extend_from_slice(&pack_id(id, extended, raw_len).to_be_bytes());
    output.extend_from_slice(payload);
    output.resize(output.len() + (pad4(raw_len) - raw_len), 0);
}

fn append_blobmsg_string(output: &mut Vec<u8>, name: &str, value: &str) {
    let header_len = pad4(2 + name.len() + 1);
    let mut payload = vec![0u8; header_len];
    payload[..2].copy_from_slice(&(name.len() as u16).to_be_bytes());
    payload[2..2 + name.len()].copy_from_slice(name.as_bytes());
    payload.extend_from_slice(value.as_bytes());
    payload.push(0);
    append_attr(output, BLOBMSG_STRING, true, &payload);
}

fn encode_string_table(entries: &[(&str, &str)]) -> Vec<u8> {
    let mut table = Vec::new();
    for (name, value) in entries {
        append_blobmsg_string(&mut table, name, value);
    }
    table
}

fn split_attrs(mut body: &[u8]) -> Result<Vec<Attr<'_>>, Error> {
    let mut attrs = Vec::new();
    while body.len() >= 4 {
        let id_len = u32::from_be_bytes(body[..4].try_into().expect("four bytes"));
        let raw_len = (id_len & ATTR_LEN_MASK) as usize;
        if raw_len < 4 || raw_len > body.len() {
            return Err(Error(format!(
                "ubus attribute length {raw_len} exceeds {} bytes",
                body.len()
            )));
        }
        attrs.push(Attr {
            id: ((id_len >> ATTR_ID_SHIFT) & 0x7f) as u8,
            payload: &body[4..raw_len],
        });
        let advance = pad4(raw_len).min(body.len());
        body = &body[advance..];
    }
    Ok(attrs)
}

fn container(body: &[u8]) -> Result<Vec<Attr<'_>>, Error> {
    if body.len() < 4 {
        return Err(Error("short ubus container".into()));
    }
    let raw_len =
        (u32::from_be_bytes(body[..4].try_into().expect("four bytes")) & ATTR_LEN_MASK) as usize;
    if raw_len < 4 || raw_len > body.len() {
        return Err(Error(format!("ubus container length {raw_len} is invalid")));
    }
    split_attrs(&body[4..raw_len])
}

fn status_code(body: &[u8]) -> Result<i32, Error> {
    for attr in container(body)? {
        if attr.id == ATTR_STATUS && attr.payload.len() >= 4 {
            return Ok(i32::from_be_bytes(
                attr.payload[..4].try_into().expect("four bytes"),
            ));
        }
    }
    Err(Error("ubus status message omitted status".into()))
}

fn decode_access(table: &[u8]) -> Result<bool, Error> {
    for attr in split_attrs(table)? {
        if attr.payload.len() < 2 {
            continue;
        }
        let name_len =
            u16::from_be_bytes(attr.payload[..2].try_into().expect("two bytes")) as usize;
        let header_len = pad4(2 + name_len + 1);
        if 2 + name_len > attr.payload.len() || header_len > attr.payload.len() {
            return Err(Error("blobmsg name exceeds attribute".into()));
        }
        if &attr.payload[2..2 + name_len] != b"access" {
            continue;
        }
        let value = &attr.payload[header_len..];
        return match attr.id {
            BLOBMSG_INT8 if !value.is_empty() => Ok(value[0] != 0),
            BLOBMSG_INT16 if value.len() >= 2 => {
                Ok(u16::from_be_bytes(value[..2].try_into().expect("two bytes")) != 0)
            }
            BLOBMSG_INT32 if value.len() >= 4 => {
                Ok(u32::from_be_bytes(value[..4].try_into().expect("four bytes")) != 0)
            }
            _ => Err(Error("session.access returned an unsupported value".into())),
        };
    }
    Ok(false)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn lookup_encoding_matches_blob_wire_format() {
        let mut message = Message::default();
        message.put_string(ATTR_OBJ_PATH, "system");
        assert_eq!(
            message.finish(),
            vec![
                0x00, 0x00, 0x00, 0x10, 0x02, 0x00, 0x00, 0x0b, b's', b'y', b's', b't', b'e', b'm',
                0, 0,
            ]
        );
    }

    #[test]
    fn access_bool_decodes() {
        let mut table = Vec::new();
        let header_len = pad4(2 + "access".len() + 1);
        let mut payload = vec![0u8; header_len];
        payload[..2].copy_from_slice(&6u16.to_be_bytes());
        payload[2..8].copy_from_slice(b"access");
        payload.push(1);
        append_attr(&mut table, BLOBMSG_INT8, true, &payload);
        assert!(decode_access(&table).expect("decode"));
    }
}
