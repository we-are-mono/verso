# ubus wire protocol (as implemented by Verso)

Reference for `internal/ubus`, Verso's minimal pure-Go ubus client. Derived from
the upstream source: libubox `blob.h` / `blobmsg.h` and ubus `ubusmsg.h` /
`libubus-io.c`. This documents the subset Verso speaks; ubus is larger.

## Transport

- Unix **stream** socket, default `/var/run/ubus/ubus.sock`.
- Synchronous request → reply. On connect, the server sends a `HELLO` first.

## Message framing

Each message is an 8-byte header followed by exactly one blob container.

`struct ubus_msghdr` (packed, 8 bytes):

| field   | size | notes            |
|---------|------|------------------|
| version | u8   | always `0`       |
| type    | u8   | `ubus_msg_type`  |
| seq     | u16  | **big-endian**   |
| peer    | u32  | **big-endian**   |

The body is one `blob_attr` container; its length is carried in the container's
own `id_len`, so there is no separate length prefix.

## blob_attr (blob.h)

A big-endian `id_len` (u32), then the payload, padded to a 4-byte boundary.

- bit 31 `0x80000000` — **extended** (blobmsg: payload begins with a name)
- bits 30–24 `0x7f000000` — **id / type**
- bits 23–0 `0x00ffffff` — **length, INCLUDING the 4-byte header**

Integers are big-endian. A container's payload is its child attributes packed
back-to-back, each padded to 4 bytes.

## blobmsg (blobmsg.h)

When `extended` is set, the payload starts with a name header, then the value:

- `namelen`: u16 big-endian (excludes the trailing NUL)
- `name`: `namelen` bytes + one NUL
- padded to 4 bytes — `hdrlen = pad4(2 + namelen + 1)`
- then the value

blobmsg types (the attr id when extended): `1` ARRAY, `2` TABLE, `3` STRING,
`4` INT64, `5` INT32, `6` INT16, `7` INT8/BOOL, `8` DOUBLE. Tables and arrays
nest as child attributes (array elements have an empty name).

## Enums

- **message types:** 0 HELLO, 1 STATUS, 2 DATA, 3 PING, 4 LOOKUP, 5 INVOKE,
  6 ADD_OBJECT, 7 REMOVE_OBJECT, 8 SUBSCRIBE, 9 UNSUBSCRIBE, 10 NOTIFY, 11 MONITOR.
- **attribute ids:** 0 UNSPEC, 1 STATUS, 2 OBJPATH, 3 OBJID, 4 METHOD, 5 OBJTYPE,
  6 SIGNATURE, 7 DATA, 8 TARGET, 9 ACTIVE, 10 NO_REPLY, 11 SUBSCRIBERS, 12 USER,
  13 GROUP.
- **status codes:** 0 OK, 1 INVALID_COMMAND, 2 INVALID_ARGUMENT,
  3 METHOD_NOT_FOUND, 4 NOT_FOUND, 5 NO_DATA, 6 PERMISSION_DENIED, 7 TIMEOUT,
  8 NOT_SUPPORTED, 9 UNKNOWN_ERROR.

## Call flow

1. **Connect**, read the server's `HELLO` — `hdr.peer` is our assigned client id.
2. **LOOKUP** — send `type=LOOKUP`, body `{ OBJPATH: name }`. The server replies
   with one or more `DATA` messages carrying `OBJID` (plus `OBJPATH`, `SIGNATURE`),
   then a `STATUS`. Keep the `OBJID`.
3. **INVOKE** — send `type=INVOKE`, body `{ OBJID: id, METHOD: name, DATA: <args> }`.
   The server replies with a `DATA` message carrying the result in `UBUS_ATTR_DATA`
   (a blobmsg table), then a `STATUS` with the return code (`0` = OK).
4. **Decode** `UBUS_ATTR_DATA`'s payload as a blobmsg table.

Requests carry only plain blob attributes (string / int32); blobmsg appears only
when decoding results.

## Gotchas (learned the hard way)

- **INVOKE must include a `DATA` attribute even for a no-argument method.**
  `ubus call` always sends an empty args table; omitting `UBUS_ATTR_DATA` makes
  ubusd return status `2` (INVALID_ARGUMENT).
- `seq` and `peer` in the header are **big-endian**.
- The 24-bit length field **includes** the 4-byte attribute header; advance by the
  4-byte-padded length when iterating a container's children.

## Scope and extension

`internal/ubus` implements connect + hello + lookup + no-argument invoke — enough
for `system info`. To support argument-carrying calls, encode a blobmsg args table
into `UBUS_ATTR_DATA` (only a blobmsg *decoder* exists today). Event subscriptions
(`SUBSCRIBE` / `NOTIFY`) and object registration are not implemented; the
persistent connection could later carry them for live UI updates.
