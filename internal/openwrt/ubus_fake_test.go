// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package openwrt

import (
	"encoding/binary"
	"errors"
	"io"
	"maps"
	"net"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/we-are-mono/verso/internal/ubus"
)

// The ubus message types and attributes a fake ubusd speaks (ubusmsg.h).
const (
	ubusMsgHello  = 0
	ubusMsgStatus = 1
	ubusMsgData   = 2
	ubusMsgLookup = 4
	ubusMsgInvoke = 5

	ubusAttrStatus  = 1
	ubusAttrObjPath = 2
	ubusAttrObjID   = 3
	ubusAttrMethod  = 4
	ubusAttrData    = 7
)

// The ubus statuses a fake object answers with (enum ubus_msg_status).
const (
	ubusOK           = 0
	ubusUnknownError = 9
)

// ubusCall is one method a fake ubusd ran: the object, the method, and the
// arguments as the wire carried them (every integer and boolean as int64,
// every array as []any).
type ubusCall struct {
	Object string
	Method string
	Args   map[string]any
}

// ubusObject answers the methods of one object: the reply table, and the ubus
// status the reply ends with.
type ubusObject func(method string, args map[string]any) (map[string]any, int)

// fakeUbus is a ubusd at a socket of its own. It serves the objects it was
// given, answers a lookup of any other with UBUS_STATUS_NOT_FOUND as ubusd
// does, and records every invoke. Unless told otherwise, rpcd's session object
// grants every access probe.
type fakeUbus struct {
	socket  string
	objects map[string]ubusObject

	mu    sync.Mutex
	calls []ubusCall
}

// grantAll is the session object of an operator every ACL allows.
func grantAll(string, map[string]any) (map[string]any, int) {
	return map[string]any{"access": true}, ubusOK
}

// denyAll is the session object of an operator no ACL allows.
func denyAll(string, map[string]any) (map[string]any, int) {
	return map[string]any{"access": false}, ubusOK
}

// replyWith answers every method of an object with one reply.
func replyWith(reply map[string]any) ubusObject {
	return func(string, map[string]any) (map[string]any, int) { return reply, ubusOK }
}

// failWith answers every method of an object with one non-zero status.
func failWith(status int) ubusObject {
	return func(string, map[string]any) (map[string]any, int) { return nil, status }
}

func newFakeUbus(t *testing.T, objects map[string]ubusObject) *fakeUbus {
	t.Helper()
	f := &fakeUbus{socket: filepath.Join(t.TempDir(), "ubus.sock"), objects: map[string]ubusObject{"session": grantAll}}
	maps.Copy(f.objects, objects)
	listener, err := net.Listen("unix", f.socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go f.serve(t, conn)
		}
	}()
	return f
}

// backend is a NativeBackend whose ubus is this fake.
func (f *fakeUbus) backend() *NativeBackend { return &NativeBackend{ubusSocket: f.socket} }

// invoked lists the calls made to object, in order.
func (f *fakeUbus) invoked(object string) []ubusCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []ubusCall
	for _, c := range f.calls {
		if c.Object == object {
			out = append(out, c)
		}
	}
	return out
}

// only is the one call made to object, failing the test unless there was
// exactly one.
func (f *fakeUbus) only(t *testing.T, object string) ubusCall {
	t.Helper()
	calls := f.invoked(object)
	if len(calls) != 1 {
		t.Fatalf("%s calls = %+v, want exactly one", object, calls)
	}
	return calls[0]
}

func (f *fakeUbus) serve(t *testing.T, conn net.Conn) {
	defer conn.Close()
	if writeUbusMessage(conn, ubusMsgHello, 0, emptyUbusBody()) != nil {
		return
	}
	names := slices.Sorted(maps.Keys(f.objects))
	for {
		typ, seq, attrs, err := readUbusMessage(conn)
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
				t.Errorf("fake ubusd: %v", err)
			}
			return
		}
		switch typ {
		case ubusMsgLookup:
			name := ubusString(attrs[ubusAttrObjPath])
			id := slices.Index(names, name)
			if id < 0 {
				_ = writeUbusMessage(conn, ubusMsgStatus, seq, ubusStatusBody(ubus.StatusNotFound))
				continue
			}
			_ = writeUbusMessage(conn, ubusMsgData, seq, ubusBody(
				ubusAttr(ubusAttrObjPath, append([]byte(name), 0)),
				ubusAttr(ubusAttrObjID, ubusU32(uint32(id+1))),
			))
			_ = writeUbusMessage(conn, ubusMsgStatus, seq, ubusStatusBody(ubusOK))
		case ubusMsgInvoke:
			name := names[binary.BigEndian.Uint32(attrs[ubusAttrObjID])-1]
			method := ubusString(attrs[ubusAttrMethod])
			args, err := ubus.DecodeTable(attrs[ubusAttrData])
			if err != nil {
				t.Errorf("fake ubusd: %s.%s args: %v", name, method, err)
				return
			}
			f.mu.Lock()
			f.calls = append(f.calls, ubusCall{Object: name, Method: method, Args: args})
			f.mu.Unlock()
			reply, status := f.objects[name](method, args)
			if reply != nil {
				table, err := ubus.EncodeTable(reply)
				if err != nil {
					t.Errorf("fake ubusd: %s.%s reply: %v", name, method, err)
					return
				}
				_ = writeUbusMessage(conn, ubusMsgData, seq, ubusBody(ubusAttr(ubusAttrData, table)))
			}
			_ = writeUbusMessage(conn, ubusMsgStatus, seq, ubusStatusBody(status))
		default:
			t.Errorf("fake ubusd: message type %d", typ)
			return
		}
	}
}

// ubusAttr is one plain blob attribute: its id_len word, then the payload
// padded to four bytes.
func ubusAttr(id int, payload []byte) []byte {
	out := make([]byte, 4+(len(payload)+3)&^3)
	binary.BigEndian.PutUint32(out, uint32(id)<<24|uint32(4+len(payload)))
	copy(out[4:], payload)
	return out
}

// ubusBody is a message body: a blob container around its attributes.
func ubusBody(attrs ...[]byte) []byte {
	children := slices.Concat(attrs...)
	out := make([]byte, 4, 4+len(children))
	binary.BigEndian.PutUint32(out, uint32(4+len(children)))
	return append(out, children...)
}

func emptyUbusBody() []byte { return ubusBody() }

func ubusStatusBody(status int) []byte {
	return ubusBody(ubusAttr(ubusAttrStatus, ubusU32(uint32(int32(status)))))
}

func ubusU32(v uint32) []byte { return binary.BigEndian.AppendUint32(nil, v) }

func ubusString(payload []byte) string {
	if i := slices.Index(payload, 0); i >= 0 {
		payload = payload[:i]
	}
	return string(payload)
}

func writeUbusMessage(w io.Writer, typ byte, seq uint16, body []byte) error {
	header := []byte{0, typ, 0, 0, 0, 0, 0, 0}
	binary.BigEndian.PutUint16(header[2:], seq)
	_, err := w.Write(append(header, body...))
	return err
}

// readUbusMessage reads one request: its type, its sequence, and its
// attributes by id.
func readUbusMessage(r io.Reader) (byte, uint16, map[int][]byte, error) {
	var header [12]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return 0, 0, nil, err
	}
	length := int(binary.BigEndian.Uint32(header[8:]) & 0x00ffffff)
	if length < 4 {
		return 0, 0, nil, errors.New("short container")
	}
	children := make([]byte, length-4)
	if _, err := io.ReadFull(r, children); err != nil {
		return 0, 0, nil, err
	}
	attrs := map[int][]byte{}
	for len(children) >= 4 {
		word := binary.BigEndian.Uint32(children)
		size := int(word & 0x00ffffff)
		if size < 4 || size > len(children) {
			return 0, 0, nil, errors.New("attribute out of bounds")
		}
		attrs[int(word>>24&0x7f)] = children[4:size]
		children = children[min((size+3)&^3, len(children)):]
	}
	return header[1], binary.BigEndian.Uint16(header[2:]), attrs, nil
}
