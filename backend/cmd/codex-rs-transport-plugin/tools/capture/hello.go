package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"
)

type prefixConn struct {
	net.Conn
	prefix []byte
}

func (c *prefixConn) Read(p []byte) (int, error) {
	if len(c.prefix) > 0 {
		n := copy(p, c.prefix)
		c.prefix = c.prefix[n:]
		return n, nil
	}
	return c.Conn.Read(p)
}

func captureClientHelloRecord(conn net.Conn, deadline time.Time) ([]byte, net.Conn, error) {
	if !deadline.IsZero() {
		_ = conn.SetReadDeadline(deadline)
		defer func() { _ = conn.SetReadDeadline(time.Time{}) }()
	}
	var raw []byte
	var hs []byte
	for {
		hdr := make([]byte, 5)
		if _, err := io.ReadFull(conn, hdr); err != nil {
			return nil, nil, fmt.Errorf("read TLS record header: %w", err)
		}
		if hdr[0] != 0x16 {
			return nil, nil, fmt.Errorf("first record type %d is not handshake", hdr[0])
		}
		n := int(binary.BigEndian.Uint16(hdr[3:5]))
		if n <= 0 || n > 1<<14+256 {
			return nil, nil, fmt.Errorf("invalid TLS record length %d", n)
		}
		body := make([]byte, n)
		if _, err := io.ReadFull(conn, body); err != nil {
			return nil, nil, fmt.Errorf("read TLS record body: %w", err)
		}
		raw = append(raw, hdr...)
		raw = append(raw, body...)
		hs = append(hs, body...)
		if len(hs) >= 4 {
			hsLen := int(hs[1])<<16 | int(hs[2])<<8 | int(hs[3])
			if hs[0] != 0x01 {
				return nil, nil, fmt.Errorf("handshake type %d is not client hello", hs[0])
			}
			if len(hs) >= 4+hsLen {
				return raw, &prefixConn{Conn: conn, prefix: raw}, nil
			}
		}
	}
}
