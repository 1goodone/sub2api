package main

import (
	"bufio"
	"context"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"

	"github.com/Wei-Shaw/sub2api/cmd/codex-rs-transport-plugin/portrait"
	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/http2"
	"golang.org/x/net/proxy"
)

type rustlsRoundTripper struct {
	dialer     proxy.ContextDialer
	rootCAs    *x509.CertPool
	rawHello   []byte
	profileID  string
	ja3        string
	ja4        string
	requireH2  bool
	observe    func(alpn string)
}

func newRustlsRoundTripper(proxyURL string, p *portrait.Portrait) (*rustlsRoundTripper, error) {
	if p == nil {
		return nil, errors.New("missing rustls portrait")
	}
	raw, err := hex.DecodeString(p.TLS.RawClientHelloHex)
	if err != nil || len(raw) == 0 {
		return nil, errors.New("portrait raw_client_hello_hex is invalid")
	}
	dialer, err := buildProxyDialer(proxyURL)
	if err != nil {
		return nil, err
	}
	contextDialer, ok := dialer.(proxy.ContextDialer)
	if !ok {
		return nil, errors.New("proxy does not support cancellation")
	}
	return &rustlsRoundTripper{
		dialer:    contextDialer,
		rawHello:  raw,
		profileID: p.ID,
		ja3:       p.TLS.JA3Hash,
		ja4:       p.TLS.JA4,
		requireH2: len(portrait.FixtureALPN(p)) > 0,
	}, nil
}

func (t *rustlsRoundTripper) createConnection(ctx context.Context, host, address string) (roundTripConn, string, error) {
	connection, err := t.dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, "", fmt.Errorf("dial upstream: %w", err)
	}
	owned := true
	defer func() {
		if owned {
			_ = connection.Close()
		}
	}()

	spec, err := portrait.ClientHelloSpecForDial(t.rawHello, host)
	if err != nil {
		return nil, "", err
	}
	tlsConnection := utls.UClient(connection, &utls.Config{ServerName: host, RootCAs: t.rootCAs}, utls.HelloCustom)
	if err := tlsConnection.ApplyPreset(spec); err != nil {
		return nil, "", fmt.Errorf("apply rustls hello: %w", err)
	}
	if err := tlsConnection.HandshakeContext(ctx); err != nil {
		return nil, "", fmt.Errorf("TLS handshake: %w", err)
	}
	alpn := tlsConnection.ConnectionState().NegotiatedProtocol
	if t.requireH2 && alpn != "h2" {
		return nil, alpn, errors.New("upstream did not negotiate HTTP/2")
	}
	if alpn == "h2" {
		h2Transport, err := http2.ConfigureTransports(&http.Transport{})
		if err != nil {
			return nil, alpn, fmt.Errorf("configure HTTP/2: %w", err)
		}
		if t.profileID != "" {
			if p, err := portrait.EmbeddedP2(); err == nil {
				portrait.ApplyMappedHTTP2Settings(h2Transport, p)
			}
		}
		h2Connection, err := h2Transport.NewClientConn(tlsConnection)
		if err != nil {
			return nil, alpn, fmt.Errorf("initialize HTTP/2: %w", err)
		}
		owned = false
		return h2Connection, alpn, nil
	}
	owned = false
	return &http11Conn{conn: tlsConnection}, alpn, nil
}

func (t *rustlsRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	if request == nil || request.URL == nil || request.URL.Scheme != "https" {
		return nil, errors.New("rustls transport requires HTTPS")
	}
	host := request.URL.Hostname()
	port := request.URL.Port()
	if port == "" {
		port = "443"
	}
	connection, alpn, err := t.createConnection(request.Context(), host, net.JoinHostPort(host, port))
	if err != nil {
		return nil, err
	}
	if t.observe != nil {
		t.observe(alpn)
	}
	response, err := connection.RoundTrip(request)
	if err != nil {
		_ = connection.Close()
		return nil, err
	}
	if response == nil {
		_ = connection.Close()
		return nil, errors.New("upstream returned no response")
	}
	if response.Body == nil {
		response.Body = http.NoBody
	}
	response.Body = &closeConnectionBody{ReadCloser: response.Body, closeConnection: connection.Close}
	return response, nil
}

type roundTripConn interface {
	RoundTrip(*http.Request) (*http.Response, error)
	Close() error
}

type http11Conn struct{ conn net.Conn }

func (c *http11Conn) RoundTrip(request *http.Request) (*http.Response, error) {
	if err := request.Write(c.conn); err != nil {
		return nil, err
	}
	return http.ReadResponse(bufio.NewReader(c.conn), request)
}

func (c *http11Conn) Close() error { return c.conn.Close() }

type closeConnectionBody struct {
	io.ReadCloser
	closeConnection func() error
	once            sync.Once
	err             error
}

func (b *closeConnectionBody) Close() error {
	if b == nil {
		return nil
	}
	b.once.Do(func() {
		var bodyErr, connectionErr error
		if b.ReadCloser != nil {
			bodyErr = b.ReadCloser.Close()
		}
		if b.closeConnection != nil {
			connectionErr = b.closeConnection()
		}
		b.err = errors.Join(bodyErr, connectionErr)
	})
	return b.err
}
