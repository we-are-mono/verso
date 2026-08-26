// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package ubus

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"
)

const (
	defaultSocket  = "/var/run/ubus/ubus.sock"
	maxMsgLen      = 1 << 24 // the length field is 24 bits
	defaultTimeout = 5 * time.Second
)

// Client is a synchronous ubus client over the unix socket. It is not safe for
// concurrent use; dial one per call or guard with a mutex.
type Client struct {
	conn    net.Conn
	timeout time.Duration
	localID uint32
	seq     uint16
}

// Dial connects to the ubus socket ("" = the default path) and completes the
// server's HELLO handshake.
func Dial(socket string) (*Client, error) {
	if socket == "" {
		socket = defaultSocket
	}
	conn, err := net.DialTimeout("unix", socket, defaultTimeout)
	if err != nil {
		return nil, fmt.Errorf("ubus: dial %s: %w", socket, err)
	}
	c := &Client{conn: conn, timeout: defaultTimeout}
	if err := c.hello(); err != nil {
		conn.Close()
		return nil, err
	}
	return c, nil
}

// SetTimeout widens (or narrows) this client's per-message deadline. The
// default suits status reads; a call that runs a package fetch on the far
// side needs the room its work takes.
func (c *Client) SetTimeout(d time.Duration) { c.timeout = d }

// Close releases the connection.
func (c *Client) Close() error { return c.conn.Close() }

type header struct {
	typ  byte
	seq  uint16
	peer uint32
}

// hello reads the server's initial HELLO and records our assigned peer id.
func (c *Client) hello() error {
	h, _, err := c.readMessage()
	if err != nil {
		return fmt.Errorf("ubus: hello: %w", err)
	}
	if h.typ != msgHello {
		return fmt.Errorf("ubus: expected HELLO, got message type %d", h.typ)
	}
	c.localID = h.peer
	return nil
}

// send writes a ubus message: the 8-byte header followed by the blob body.
func (c *Client) send(typ byte, peer uint32, body []byte) error {
	c.seq++
	buf := make([]byte, 8+len(body))
	buf[0] = 0 // version
	buf[1] = typ
	binary.BigEndian.PutUint16(buf[2:4], c.seq)
	binary.BigEndian.PutUint32(buf[4:8], peer)
	copy(buf[8:], body)

	if err := c.conn.SetWriteDeadline(time.Now().Add(c.timeout)); err != nil {
		return err
	}
	_, err := c.conn.Write(buf)
	return err
}

// readMessage reads one whole ubus message: header, then the blob container
// whose length is carried in the first attribute's id_len.
func (c *Client) readMessage() (header, []byte, error) {
	if err := c.conn.SetReadDeadline(time.Now().Add(c.timeout)); err != nil {
		return header{}, nil, err
	}

	var hb [8]byte
	if _, err := io.ReadFull(c.conn, hb[:]); err != nil {
		return header{}, nil, err
	}
	if hb[0] != 0 {
		return header{}, nil, fmt.Errorf("ubus: unsupported version %d", hb[0])
	}
	h := header{
		typ:  hb[1],
		seq:  binary.BigEndian.Uint16(hb[2:4]),
		peer: binary.BigEndian.Uint32(hb[4:8]),
	}

	var lb [4]byte
	if _, err := io.ReadFull(c.conn, lb[:]); err != nil {
		return h, nil, err
	}
	rawLen := int(binary.BigEndian.Uint32(lb[:]) & attrLenMask)
	if rawLen < 4 || rawLen > maxMsgLen {
		return h, nil, fmt.Errorf("ubus: message length %d out of range", rawLen)
	}

	body := make([]byte, rawLen)
	copy(body, lb[:])
	if rawLen > 4 {
		if _, err := io.ReadFull(c.conn, body[4:]); err != nil {
			return h, nil, err
		}
	}
	return h, body, nil
}

// Lookup resolves an object name to its numeric id.
func (c *Client) Lookup(name string) (uint32, error) {
	var m msg
	m.putString(attrObjPath, name)
	if err := c.send(msgLookup, 0, m.bytes()); err != nil {
		return 0, err
	}

	var (
		id    uint32
		found bool
	)
	for {
		h, body, err := c.readMessage()
		if err != nil {
			return 0, err
		}
		switch h.typ {
		case msgData:
			attrs, err := container(body)
			if err != nil {
				return 0, err
			}
			for _, a := range attrs {
				if a.id == attrObjID && len(a.payload) >= 4 {
					id = binary.BigEndian.Uint32(a.payload[:4])
					found = true
				}
			}
		case msgStatus:
			if code := statusCode(body); code != 0 {
				return 0, fmt.Errorf("ubus: lookup %q: status %d", name, code)
			}
			if !found {
				return 0, fmt.Errorf("ubus: object %q not found", name)
			}
			return id, nil
		}
	}
}

// Invoke calls a no-argument method on an object and returns the decoded result
// table (int64, float64, string, map[string]any, or []any values).
func (c *Client) Invoke(objID uint32, method string) (map[string]any, error) {
	return c.invoke(objID, method, nil)
}

// InvokeArgs calls a method with named string arguments — encoded as a blobmsg
// table in UBUS_ATTR_DATA — and returns the decoded result table. String args
// cover the flat calls Verso needs (e.g. session.login's username/password);
// InvokeTable handles arguments carrying nested tables.
func (c *Client) InvokeArgs(objID uint32, method string, args map[string]string) (map[string]any, error) {
	return c.invoke(objID, method, encodeArgs(args))
}

// InvokeTable calls a method whose arguments may include nested tables — the
// shape uci.set needs for values:{} — and returns the decoded result table.
// Argument values may be string, map[string]string, or map[string]any; any other
// type is rejected before anything is sent.
func (c *Client) InvokeTable(objID uint32, method string, args map[string]any) (map[string]any, error) {
	body, err := encodeTable(args)
	if err != nil {
		return nil, err
	}
	return c.invoke(objID, method, body)
}

// invoke sends INVOKE with a pre-encoded blobmsg args table (nil or empty sends
// the empty args table ubusd requires even for a no-argument method) and returns
// the decoded result table.
func (c *Client) invoke(objID uint32, method string, tableBody []byte) (map[string]any, error) {
	var m msg
	m.putU32(attrObjID, objID)
	m.putString(attrMethod, method)
	m.put(attrData, false, tableBody)
	if err := c.send(msgInvoke, 0, m.bytes()); err != nil {
		return nil, err
	}

	var result map[string]any
	for {
		h, body, err := c.readMessage()
		if err != nil {
			return nil, err
		}
		switch h.typ {
		case msgData:
			attrs, err := container(body)
			if err != nil {
				return nil, err
			}
			for _, a := range attrs {
				if a.id == attrData {
					result, err = decodeTable(a.payload)
					if err != nil {
						return nil, err
					}
				}
			}
		case msgStatus:
			if code := statusCode(body); code != 0 {
				return nil, fmt.Errorf("ubus: invoke %q: status %d", method, code)
			}
			if result == nil {
				result = map[string]any{}
			}
			return result, nil
		}
	}
}

func statusCode(body []byte) int {
	attrs, err := container(body)
	if err != nil {
		return -1
	}
	for _, a := range attrs {
		if a.id == attrStatus && len(a.payload) >= 4 {
			return int(int32(binary.BigEndian.Uint32(a.payload[:4])))
		}
	}
	return -1
}
