// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

// Package ubus is a minimal, pure-Go client for OpenWrt's ubus, speaking the
// native blob/blobmsg protocol over the ubus unix socket. It implements only
// what Verso needs (lookup + invoke of no-argument methods), which keeps the
// request side to plain blob attributes; blobmsg decoding handles the replies.
//
// Wire format is per libubox/blob.h, blobmsg.h and ubus/ubusmsg.h: a blob_attr
// is a big-endian uint32 id_len (bit 31 = extended, bits 30-24 = id/type,
// bits 23-0 = length INCLUDING the 4-byte header) followed by the payload,
// padded to 4 bytes. Integers are big-endian.
package ubus

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

// ubus message types (ubusmsg.h: enum ubus_msg_type).
const (
	msgHello  = 0
	msgStatus = 1
	msgData   = 2
	msgLookup = 4
	msgInvoke = 5
)

// ubus message attribute ids (ubusmsg.h: enum ubus_msg_attr).
const (
	attrStatus  = 1
	attrObjPath = 2
	attrObjID   = 3
	attrMethod  = 4
	attrData    = 7
)

// blobmsg value types (blobmsg.h: enum blobmsg_type).
const (
	bmArray  = 1
	bmTable  = 2
	bmString = 3
	bmInt64  = 4
	bmInt32  = 5
	bmInt16  = 6
	bmInt8   = 7
	bmDouble = 8
)

const (
	attrExtended = 0x80000000
	attrIDShift  = 24
	attrIDMask   = 0x7f
	attrLenMask  = 0x00ffffff
	attrAlign    = 4
)

var errShort = errors.New("ubus: short attribute")

func pad4(n int) int { return (n + attrAlign - 1) &^ (attrAlign - 1) }

func packID(id int, extended bool, rawLen int) uint32 {
	v := uint32(rawLen) & attrLenMask
	v |= uint32(id&attrIDMask) << attrIDShift
	if extended {
		v |= attrExtended
	}
	return v
}

// msg accumulates child blob attributes for a ubus message body.
type msg struct{ children []byte }

func (m *msg) putString(id int, s string) {
	p := make([]byte, len(s)+1) // NUL-terminated, like blob_put_string
	copy(p, s)
	m.put(id, false, p)
}

func (m *msg) putU32(id int, v uint32) {
	var p [4]byte
	binary.BigEndian.PutUint32(p[:], v)
	m.put(id, false, p[:])
}

func (m *msg) put(id int, extended bool, payload []byte) {
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], packID(id, extended, len(payload)+4))
	m.children = append(m.children, hdr[:]...)
	m.children = append(m.children, payload...)
	for len(m.children)%attrAlign != 0 {
		m.children = append(m.children, 0)
	}
}

// bytes returns the full message body: the top-level container (id 0) wrapping
// the children, exactly the blob_raw_len bytes libubus writes after the header.
func (m *msg) bytes() []byte {
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], packID(0, false, len(m.children)+4))
	return append(hdr[:], m.children...)
}

// encodeArgs encodes named string arguments as the blobmsg table libubus expects
// in UBUS_ATTR_DATA. It is the flat-string counterpart of decodeTable, covering
// calls whose arguments are all strings (session.login, uci.get, session.access).
func encodeArgs(args map[string]string) []byte {
	var body []byte
	for name, val := range args {
		body = appendBlobmsgAttr(body, bmString, name, nulTerminated(val))
	}
	return body
}

// encodeTable encodes named arguments that may include nested tables — the shape
// uci.set needs for its values:{} — as a blobmsg table body. Values may be a
// string, a map[string]string, or a map[string]any (nested table); any other
// type is a hard error rather than silent corruption on the wire.
func encodeTable(args map[string]any) ([]byte, error) {
	var body []byte
	for name, val := range args {
		var err error
		if body, err = appendBlobmsgValue(body, name, val); err != nil {
			return nil, err
		}
	}
	return body, nil
}

// appendBlobmsgValue appends one named blobmsg attribute, dispatching on the Go
// type of val. Nested maps recurse as blobmsg tables; unsupported types error.
func appendBlobmsgValue(dst []byte, name string, val any) ([]byte, error) {
	switch v := val.(type) {
	case string:
		return appendBlobmsgAttr(dst, bmString, name, nulTerminated(v)), nil
	case map[string]string:
		return appendBlobmsgAttr(dst, bmTable, name, encodeArgs(v)), nil
	case map[string]any:
		body, err := encodeTable(v)
		if err != nil {
			return nil, err
		}
		return appendBlobmsgAttr(dst, bmTable, name, body), nil
	case []string:
		return appendBlobmsgAttr(dst, bmArray, name, encodeStringArray(v)), nil
	case []any:
		body, err := encodeArray(v)
		if err != nil {
			return nil, err
		}
		return appendBlobmsgAttr(dst, bmArray, name, body), nil
	default:
		return nil, fmt.Errorf("ubus: cannot encode arg %q of unsupported type %T", name, val)
	}
}

// encodeStringArray encodes a blobmsg array body of strings. Array elements carry
// an empty name, per blobmsg.
func encodeStringArray(items []string) []byte {
	var body []byte
	for _, s := range items {
		body = appendBlobmsgAttr(body, bmString, "", nulTerminated(s))
	}
	return body
}

// encodeArray encodes a blobmsg array body from arbitrary elements (each may be a
// string, table, or nested array). Array elements carry an empty name.
func encodeArray(items []any) ([]byte, error) {
	var body []byte
	for _, el := range items {
		var err error
		if body, err = appendBlobmsgValue(body, "", el); err != nil {
			return nil, err
		}
	}
	return body, nil
}

// appendBlobmsgAttr appends one extended (blobmsg) attribute — a padded name
// header followed by the value payload — to a blobmsg table body. The payload is
// pre-encoded by the caller: a NUL-terminated string, or a nested table's body.
func appendBlobmsgAttr(dst []byte, id int, name string, value []byte) []byte {
	hdrLen := pad4(2 + len(name) + 1) // blobmsg_hdrlen: u16 namelen + name + NUL, padded
	p := make([]byte, hdrLen+len(value))
	binary.BigEndian.PutUint16(p[0:2], uint16(len(name)))
	copy(p[2:], name)
	copy(p[hdrLen:], value)
	rawLen := 4 + len(p)
	attr := make([]byte, pad4(rawLen))
	binary.BigEndian.PutUint32(attr[0:4], packID(id, true, rawLen))
	copy(attr[4:], p)
	return append(dst, attr...)
}

// nulTerminated returns s as the NUL-terminated byte payload blobmsg strings use.
func nulTerminated(s string) []byte {
	b := make([]byte, len(s)+1)
	copy(b, s)
	return b
}

// rawAttr is a decoded blob attribute: its id/type, extended flag, and payload.
type rawAttr struct {
	id       int
	extended bool
	payload  []byte
}

// splitAttrs iterates the TLV blob attributes packed back-to-back in body.
func splitAttrs(body []byte) ([]rawAttr, error) {
	var attrs []rawAttr
	for len(body) >= attrAlign {
		idLen := binary.BigEndian.Uint32(body[:4])
		rawLen := int(idLen & attrLenMask)
		if rawLen < 4 || rawLen > len(body) {
			return nil, fmt.Errorf("ubus: attr length %d out of bounds (have %d)", rawLen, len(body))
		}
		attrs = append(attrs, rawAttr{
			id:       int((idLen >> attrIDShift) & attrIDMask),
			extended: idLen&attrExtended != 0,
			payload:  body[4:rawLen],
		})
		adv := pad4(rawLen)
		if adv > len(body) {
			adv = len(body)
		}
		body = body[adv:]
	}
	return attrs, nil
}

// container parses a top-level blob container (4-byte header + children) into
// its child attributes — used for a received ubus message body.
func container(b []byte) ([]rawAttr, error) {
	if len(b) < 4 {
		return nil, errors.New("ubus: short message body")
	}
	rawLen := int(binary.BigEndian.Uint32(b[:4]) & attrLenMask)
	if rawLen < 4 || rawLen > len(b) {
		return nil, fmt.Errorf("ubus: container length %d out of bounds (have %d)", rawLen, len(b))
	}
	return splitAttrs(b[4:rawLen])
}

// decodeTable decodes the children of a blobmsg table into name→value pairs.
// Values are int64, float64, string, map[string]any (table) or []any (array).
func decodeTable(body []byte) (map[string]any, error) {
	attrs, err := splitAttrs(body)
	if err != nil {
		return nil, err
	}
	out := make(map[string]any, len(attrs))
	for _, a := range attrs {
		name, val, err := decodeBlobmsg(a)
		if err != nil {
			return nil, err
		}
		out[name] = val
	}
	return out, nil
}

func decodeArray(body []byte) ([]any, error) {
	attrs, err := splitAttrs(body)
	if err != nil {
		return nil, err
	}
	out := make([]any, 0, len(attrs))
	for _, a := range attrs {
		_, val, err := decodeBlobmsg(a)
		if err != nil {
			return nil, err
		}
		out = append(out, val)
	}
	return out, nil
}

// decodeBlobmsg splits one blobmsg attribute into its name and typed value.
func decodeBlobmsg(a rawAttr) (name string, val any, err error) {
	p := a.payload
	if len(p) < 2 {
		return "", nil, errShort
	}
	nameLen := int(binary.BigEndian.Uint16(p[:2]))
	hdrLen := pad4(2 + nameLen + 1) // blobmsg_hdrlen: u16 + name + NUL, padded
	if 2+nameLen > len(p) || hdrLen > len(p) {
		return "", nil, errors.New("ubus: blobmsg name out of bounds")
	}
	name = string(p[2 : 2+nameLen])
	data := p[hdrLen:]

	switch a.id {
	case bmTable:
		val, err = decodeTable(data)
	case bmArray:
		val, err = decodeArray(data)
	case bmString:
		val = string(trimNUL(data))
	case bmInt64:
		if len(data) < 8 {
			return "", nil, errShort
		}
		val = int64(binary.BigEndian.Uint64(data))
	case bmInt32:
		if len(data) < 4 {
			return "", nil, errShort
		}
		val = int64(int32(binary.BigEndian.Uint32(data)))
	case bmInt16:
		if len(data) < 2 {
			return "", nil, errShort
		}
		val = int64(int16(binary.BigEndian.Uint16(data)))
	case bmInt8:
		if len(data) < 1 {
			return "", nil, errShort
		}
		val = int64(int8(data[0]))
	case bmDouble:
		if len(data) < 8 {
			return "", nil, errShort
		}
		val = math.Float64frombits(binary.BigEndian.Uint64(data))
	default:
		val = nil
	}
	return name, val, err
}

func trimNUL(b []byte) []byte {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		return b[:i]
	}
	return b
}
