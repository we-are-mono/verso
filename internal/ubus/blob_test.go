// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package ubus

import (
	"bytes"
	"testing"
)

// TestEncodeLookup anchors the wire format with a hand-computed golden: a
// message carrying one OBJPATH="system" string attribute.
func TestEncodeLookup(t *testing.T) {
	var m msg
	m.putString(attrObjPath, "system")

	want := []byte{
		0x00, 0x00, 0x00, 0x10, // container: id 0, raw len 16
		0x02, 0x00, 0x00, 0x0b, // attr: id 2 (OBJPATH), raw len 11
		's', 'y', 's', 't', 'e', 'm', 0x00, // "system\0"
		0x00, // pad to 4
	}
	if got := m.bytes(); !bytes.Equal(got, want) {
		t.Fatalf("bytes =\n % x\nwant\n % x", got, want)
	}
}

// TestEncodeDecodeInvoke round-trips the invoke attributes through the parser.
func TestEncodeDecodeInvoke(t *testing.T) {
	var m msg
	m.putU32(attrObjID, 0x01020304)
	m.putString(attrMethod, "info")

	attrs, err := container(m.bytes())
	if err != nil {
		t.Fatalf("container: %v", err)
	}
	if len(attrs) != 2 {
		t.Fatalf("got %d attrs, want 2", len(attrs))
	}
	if attrs[0].id != attrObjID || !bytes.Equal(attrs[0].payload, []byte{0x01, 0x02, 0x03, 0x04}) {
		t.Errorf("attr0 = %+v", attrs[0])
	}
	if attrs[1].id != attrMethod || string(trimNUL(attrs[1].payload)) != "info" {
		t.Errorf("attr1 = %+v", attrs[1])
	}
}

// TestDecodeTableGolden decodes a hand-computed blobmsg table: one INT32 field
// "uptime"=1287. This anchors the blobmsg name+value decoding independently.
func TestDecodeTableGolden(t *testing.T) {
	body := []byte{
		0x85, 0x00, 0x00, 0x14, // extended, id 5 (INT32), raw len 20
		0x00, 0x06, // namelen 6
		'u', 'p', 't', 'i', 'm', 'e', 0x00, // "uptime\0"
		0x00, 0x00, 0x00, // pad name header to 12
		0x00, 0x00, 0x05, 0x07, // value 1287
	}

	tbl, err := decodeTable(body)
	if err != nil {
		t.Fatalf("decodeTable: %v", err)
	}
	if got, ok := tbl["uptime"].(int64); !ok || got != 1287 {
		t.Fatalf("uptime = %v (%T), want int64 1287", tbl["uptime"], tbl["uptime"])
	}
}

func TestSplitAttrsRejectsBadLength(t *testing.T) {
	// raw len 0xffffff far exceeds the buffer.
	if _, err := splitAttrs([]byte{0x00, 0xff, 0xff, 0xff, 0x00, 0x00, 0x00, 0x00}); err == nil {
		t.Fatal("splitAttrs: want error on oversized length")
	}
}

// TestEncodeArgsGolden anchors the request-side blobmsg encoder with a
// hand-computed golden: one string field "a"="hi".
func TestEncodeArgsGolden(t *testing.T) {
	want := []byte{
		0x83, 0x00, 0x00, 0x0b, // extended, id 3 (STRING), raw len 11
		0x00, 0x01, // namelen 1
		'a', 0x00, // "a\0" — name header padded to 4
		'h', 'i', 0x00, // "hi\0"
		0x00, // pad attr to 12
	}
	if got := encodeArgs(map[string]string{"a": "hi"}); !bytes.Equal(got, want) {
		t.Fatalf("encodeArgs =\n % x\nwant\n % x", got, want)
	}
}

// TestEncodeArgsRoundTrip: the encoder's output decodes back to the same args,
// including an empty value — the no-password login case.
func TestEncodeArgsRoundTrip(t *testing.T) {
	tbl, err := decodeTable(encodeArgs(map[string]string{"username": "root", "password": ""}))
	if err != nil {
		t.Fatalf("decodeTable: %v", err)
	}
	if tbl["username"] != "root" {
		t.Errorf("username = %v, want root", tbl["username"])
	}
	if v, ok := tbl["password"]; !ok || v != "" {
		t.Errorf("password = %v (present=%v), want empty string", v, ok)
	}
}
