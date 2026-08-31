package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestChromeRoundTripperNegotiatesHTTP2(t *testing.T) {
	t.Parallel()

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.ProtoMajor != 2 || request.Header.Get("X-Test") != "chrome" {
			t.Errorf("request protocol=%s headers=%v", request.Proto, request.Header)
		}
		writer.Header().Set("X-Protocol", request.Proto)
		_, _ = writer.Write([]byte("ok"))
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()

	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	transport := &chromeRoundTripper{dialer: &net.Dialer{}, rootCAs: roots}
	request, err := http.NewRequest(http.MethodPost, server.URL, bytes.NewBufferString("body"))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-Test", "chrome")
	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if response.ProtoMajor != 2 || response.Header.Get("X-Protocol") != "HTTP/2.0" || string(body) != "ok" {
		t.Fatalf("response protocol=%s headers=%v body=%q", response.Proto, response.Header, body)
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
