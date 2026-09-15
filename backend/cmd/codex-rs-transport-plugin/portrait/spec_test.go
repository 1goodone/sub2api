package portrait

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"io"
	"net"
	"testing"
	"time"

	utls "github.com/refraction-networking/utls"
)

func TestClientHelloSpecForDialRewritesSNIAndKeepsJA3(t *testing.T) {
	raw, err := captureUTLSHello(t, "inspector.example")
	if err != nil {
		t.Fatal(err)
	}
	first, err := ParseClientHelloRecord(raw)
	if err != nil {
		t.Fatal(err)
	}
	if first.SNI != "inspector.example" {
		t.Fatalf("captured SNI=%q", first.SNI)
	}
	ja3Raw, ja3Hash, _ := FingerprintsFromHello(first)

	spec, err := ClientHelloSpecForDial(raw, "chatgpt.com")
	if err != nil {
		t.Fatal(err)
	}
	replay, err := dialWithSpec(t, spec, "chatgpt.com")
	if err != nil {
		t.Fatal(err)
	}
	second, err := ParseClientHelloRecord(replay)
	if err != nil {
		t.Fatal(err)
	}
	if second.SNI != "chatgpt.com" {
		t.Fatalf("replay SNI=%q want chatgpt.com", second.SNI)
	}
	gotRaw, gotHash, _ := FingerprintsFromHello(second)
	if gotRaw != ja3Raw || gotHash != ja3Hash {
		t.Fatalf("JA3 changed after SNI rewrite\n first=%s\n replay=%s", ja3Raw, gotRaw)
	}
	if len(first.ExtensionTypes) != len(second.ExtensionTypes) {
		t.Fatalf("extension order length %d vs %d", len(first.ExtensionTypes), len(second.ExtensionTypes))
	}
	for i := range first.ExtensionTypes {
		a, b := first.ExtensionTypes[i], second.ExtensionTypes[i]
		if isGREASE(a) && isGREASE(b) {
			continue
		}
		if a != b {
			t.Fatalf("extension type order changed at %d: %d vs %d", i, a, b)
		}
	}
}

func TestClientHelloSpecForDialRejectsEmpty(t *testing.T) {
	if _, err := ClientHelloSpecForDial(nil, "chatgpt.com"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := ClientHelloSpecForDial([]byte{0x16}, ""); err == nil {
		t.Fatal("expected error")
	}
}

func TestAssertionsFromRawRoundTrip(t *testing.T) {
	raw, err := captureUTLSHello(t, "chatgpt.com")
	if err != nil {
		t.Fatal(err)
	}
	tlsCap, err := AssertionsFromRaw(raw)
	if err != nil {
		t.Fatal(err)
	}
	if tlsCap.RawClientHelloHex != hex.EncodeToString(raw) && tlsCap.RawClientHelloHex != hex.EncodeToString(raw) {
		decoded, err := hex.DecodeString(tlsCap.RawClientHelloHex)
		if err != nil || len(decoded) != len(raw) {
			t.Fatalf("raw hex mismatch")
		}
	}
	if tlsCap.JA3Hash == "" || tlsCap.JA3Raw == "" {
		t.Fatal("missing ja3")
	}
	if _, err := ClientHelloSpecForDial(raw, "chatgpt.com"); err != nil {
		t.Fatal(err)
	}
}

func captureUTLSHello(t *testing.T, sni string) ([]byte, error) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	defer ln.Close()

	errCh := make(chan error, 1)
	helloCh := make(chan []byte, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			errCh <- err
			return
		}
		defer conn.Close()
		raw, err := readTLSRecordHello(conn)
		if err != nil {
			errCh <- err
			return
		}
		helloCh <- raw
	}()

	tcp, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		return nil, err
	}
	uconn := utls.UClient(tcp, &utls.Config{
		ServerName:         sni,
		InsecureSkipVerify: true,
		NextProtos:         []string{"h2", "http/1.1"},
	}, utls.HelloChrome_Auto)
	_ = uconn.HandshakeContext(context.Background())
	_ = uconn.Close()

	select {
	case err := <-errCh:
		return nil, err
	case raw := <-helloCh:
		return raw, nil
	case <-time.After(5 * time.Second):
		return nil, context.DeadlineExceeded
	}
}

func dialWithSpec(t *testing.T, spec *utls.ClientHelloSpec, sni string) ([]byte, error) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	defer ln.Close()
	helloCh := make(chan []byte, 1)
	errCh := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			errCh <- err
			return
		}
		defer conn.Close()
		raw, err := readTLSRecordHello(conn)
		if err != nil {
			errCh <- err
			return
		}
		helloCh <- raw
	}()

	tcp, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		return nil, err
	}
	uconn := utls.UClient(tcp, &utls.Config{
		ServerName:         sni,
		InsecureSkipVerify: true,
		NextProtos:         []string{"h2", "http/1.1"},
	}, utls.HelloCustom)
	if err := uconn.ApplyPreset(spec); err != nil {
		_ = tcp.Close()
		return nil, err
	}
	_ = uconn.HandshakeContext(context.Background())
	_ = uconn.Close()

	select {
	case err := <-errCh:
		return nil, err
	case raw := <-helloCh:
		return raw, nil
	case <-time.After(5 * time.Second):
		return nil, context.DeadlineExceeded
	}
}

func readTLSRecordHello(conn net.Conn) ([]byte, error) {
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var raw []byte
	var hs []byte
	for {
		hdr := make([]byte, 5)
		if _, err := io.ReadFull(conn, hdr); err != nil {
			return nil, err
		}
		n := int(binary.BigEndian.Uint16(hdr[3:5]))
		body := make([]byte, n)
		if _, err := io.ReadFull(conn, body); err != nil {
			return nil, err
		}
		raw = append(raw, hdr...)
		raw = append(raw, body...)
		hs = append(hs, body...)
		if len(hs) >= 4 {
			hsLen := int(hs[1])<<16 | int(hs[2])<<8 | int(hs[3])
			if len(hs) >= 4+hsLen {
				return raw, nil
			}
		}
	}
}


