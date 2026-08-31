package main

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"

	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/http2"
	"golang.org/x/net/proxy"
)

// chromeRoundTripper follows CLIProxyAPI v7.2.142: Chrome_Auto uTLS,
// a dedicated HTTP/2 connection per request, and connection close with body.
type chromeRoundTripper struct {
	dialer  proxy.ContextDialer
	rootCAs *x509.CertPool
}

func newChromeRoundTripper(proxyURL string) (*chromeRoundTripper, error) {
	dialer, err := buildProxyDialer(proxyURL)
	if err != nil {
		return nil, err
	}
	contextDialer, ok := dialer.(proxy.ContextDialer)
	if !ok {
		return nil, errors.New("proxy does not support cancellation")
	}
	return &chromeRoundTripper{dialer: contextDialer}, nil
}

func (t *chromeRoundTripper) createConnection(ctx context.Context, host, address string) (*http2.ClientConn, error) {
	connection, err := t.dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, fmt.Errorf("dial upstream: %w", err)
	}
	owned := true
	defer func() {
		if owned {
			_ = connection.Close()
		}
	}()

	tlsConnection := utls.UClient(connection, &utls.Config{ServerName: host, RootCAs: t.rootCAs}, utls.HelloChrome_Auto)
	if err := tlsConnection.HandshakeContext(ctx); err != nil {
		return nil, fmt.Errorf("TLS handshake: %w", err)
	}
	if tlsConnection.ConnectionState().NegotiatedProtocol != "h2" {
		return nil, errors.New("upstream did not negotiate HTTP/2")
	}
	h2Transport, err := http2.ConfigureTransports(&http.Transport{})
	if err != nil {
		return nil, fmt.Errorf("configure HTTP/2: %w", err)
	}
	h2Connection, err := h2Transport.NewClientConn(tlsConnection)
	if err != nil {
		return nil, fmt.Errorf("initialize HTTP/2: %w", err)
	}
	owned = false
	return h2Connection, nil
}

func (t *chromeRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	if request == nil || request.URL == nil || request.URL.Scheme != "https" {
		return nil, errors.New("Chrome transport requires HTTPS")
	}
	host := request.URL.Hostname()
	port := request.URL.Port()
	if port == "" {
		port = "443"
	}
	connection, err := t.createConnection(request.Context(), host, net.JoinHostPort(host, port))
	if err != nil {
		return nil, err
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
