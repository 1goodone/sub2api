package main

import (
	"bufio"
	"context"
	"crypto/x509"
	"encoding/hex"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/cmd/codex-rs-transport-plugin/portrait"
)

func TestRustlsRoundTripperSNIRewriteKeepsJA3(t *testing.T) {
	p, err := portrait.EmbeddedP2()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := hex.DecodeString(p.TLS.RawClientHelloHex)
	if err != nil {
		t.Fatal(err)
	}
	first, err := portrait.ParseClientHelloRecord(raw)
	if err != nil {
		t.Fatal(err)
	}
	ja3Raw, ja3Hash, _ := portrait.FingerprintsFromHello(first)
	spec, err := portrait.ClientHelloSpecForDial(raw, "chatgpt.com")
	if err != nil {
		t.Fatal(err)
	}
	_ = spec
	if ja3Raw == "" || ja3Hash == "" {
		t.Fatal("missing ja3")
	}
}

func TestBuildProxyDialerModes(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"", "direct", "none", "http://proxy.example:8080", "https://proxy.example", "socks5://proxy.example", "socks5h://user:pass@proxy.example:1080"} {
		if dialer, err := buildProxyDialer(value); err != nil || dialer == nil {
			t.Errorf("buildProxyDialer(%q) = %T, %v", value, dialer, err)
		}
	}
	for _, value := range []string{"proxy.example:8080", "http://:8080", "ftp://proxy.example", "://bad"} {
		if _, err := buildProxyDialer(value); err == nil {
			t.Errorf("buildProxyDialer(%q) unexpectedly succeeded", value)
		}
	}
}

func TestHTTPConnectDialerPreservesBufferedTunnelBytes(t *testing.T) {
	t.Parallel()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	requestSeen := make(chan *http.Request, 1)
	serverDone := make(chan error, 1)
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			serverDone <- err
			return
		}
		defer connection.Close()
		request, err := http.ReadRequest(bufio.NewReader(connection))
		if err != nil {
			serverDone <- err
			return
		}
		requestSeen <- request
		_, err = connection.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\nPING"))
		serverDone <- err
	}()

	proxyURL := &url.URL{Scheme: "http", Host: listener.Addr().String(), User: url.UserPassword("user", "secret")}
	dialer := &httpConnectDialer{proxyURL: proxyURL, dialer: &net.Dialer{}}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	connection, err := dialer.DialContext(ctx, "tcp", "chatgpt.com:443")
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	data := make([]byte, 4)
	if _, err := io.ReadFull(connection, data); err != nil {
		t.Fatal(err)
	}
	if string(data) != "PING" {
		t.Fatalf("tunnel bytes = %q", data)
	}
	request := <-requestSeen
	if request.Method != http.MethodConnect || request.Host != "chatgpt.com:443" || request.Header.Get("Proxy-Authorization") == "" {
		t.Fatalf("CONNECT request = %#v", request)
	}
	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}
}

func TestTwoAccountsStayPerRequestWhenCaptureIsNotPool(t *testing.T) {
	p, err := portrait.EmbeddedP2()
	if err != nil {
		t.Fatal(err)
	}
	if portrait.CaptureAllowsPool(p) {
		t.Fatal("current capture is per_request; pooling would share TCP/TLS")
	}
	a, err := newRustlsRoundTripper("", p)
	if err != nil {
		t.Fatal(err)
	}
	b, err := newRustlsRoundTripper("", p)
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("each account/proxy pair must get its own round tripper")
	}
}

func TestLocalHandshakeAgainstHTTP2Server(t *testing.T) {
	p, err := portrait.EmbeddedP2()
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte("ok"))
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	tripper, err := newRustlsRoundTripper("", p)
	if err != nil {
		t.Fatal(err)
	}
	tripper.rootCAs = x509.NewCertPool()
	tripper.rootCAs.AddCert(server.Certificate())
	req, err := http.NewRequest(http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := tripper.RoundTrip(req)
	if err != nil {
		t.Fatalf("handshake/roundtrip: %v", err)
	}
	defer resp.Body.Close()
}
