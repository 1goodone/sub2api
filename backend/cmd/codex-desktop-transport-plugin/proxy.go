package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/net/proxy"
)

func buildProxyDialer(raw string) (proxy.Dialer, error) {
	direct := &net.Dialer{}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || strings.EqualFold(trimmed, "direct") || strings.EqualFold(trimmed, "none") {
		return direct, nil
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.Hostname() == "" {
		return nil, errors.New("proxy URL must include a supported scheme and host")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	switch parsed.Scheme {
	case "http", "https":
		return &httpConnectDialer{proxyURL: parsed, dialer: direct}, nil
	case "socks5", "socks5h":
		dialer, err := proxy.FromURL(parsed, direct)
		if err != nil {
			return nil, errors.New("invalid SOCKS proxy configuration")
		}
		return dialer, nil
	default:
		return nil, fmt.Errorf("unsupported proxy scheme: %s", parsed.Scheme)
	}
}

type httpConnectDialer struct {
	proxyURL *url.URL
	dialer   *net.Dialer
}

func (d *httpConnectDialer) Dial(network, address string) (net.Conn, error) {
	return d.DialContext(context.Background(), network, address)
}

func (d *httpConnectDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	connection, err := d.dialer.DialContext(ctx, network, proxyAddress(d.proxyURL))
	if err != nil {
		return nil, errors.New("failed to connect to proxy")
	}
	closeConnection := true
	defer func() {
		if closeConnection {
			_ = connection.Close()
		}
	}()

	canceled := make(chan struct{})
	stopCancel := context.AfterFunc(ctx, func() {
		_ = connection.Close()
		close(canceled)
	})
	defer func() {
		if !stopCancel() {
			<-canceled
		}
	}()

	if d.proxyURL.Scheme == "https" {
		tlsConnection := tls.Client(connection, &tls.Config{ServerName: d.proxyURL.Hostname()})
		if err := tlsConnection.HandshakeContext(ctx); err != nil {
			return nil, errors.New("HTTPS proxy TLS handshake failed")
		}
		connection = tlsConnection
	}

	connectRequest := (&http.Request{
		Method: http.MethodConnect,
		URL:    &url.URL{Host: address},
		Host:   address,
		Header: make(http.Header),
	}).WithContext(ctx)
	if d.proxyURL.User != nil {
		connectRequest.Header.Set("Proxy-Authorization", proxyAuthorization(d.proxyURL.User))
	}
	if err := connectRequest.Write(connection); err != nil {
		return nil, errors.New("failed to write proxy CONNECT request")
	}

	reader := bufio.NewReader(connection)
	response, err := http.ReadResponse(reader, connectRequest)
	if err != nil {
		return nil, errors.New("failed to read proxy CONNECT response")
	}
	if response.StatusCode != http.StatusOK {
		if response.Body != nil {
			_ = response.Body.Close()
		}
		return nil, fmt.Errorf("proxy CONNECT returned status %d", response.StatusCode)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	closeConnection = false
	if reader.Buffered() > 0 {
		return &bufferedConnection{Conn: connection, reader: reader}, nil
	}
	return connection, nil
}

func proxyAddress(proxyURL *url.URL) string {
	port := proxyURL.Port()
	if port == "" {
		port = "80"
		if proxyURL.Scheme == "https" {
			port = "443"
		}
	}
	return net.JoinHostPort(proxyURL.Hostname(), port)
}

func proxyAuthorization(user *url.Userinfo) string {
	password, _ := user.Password()
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user.Username()+":"+password))
}

type bufferedConnection struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedConnection) Read(data []byte) (int, error) {
	if c.reader.Buffered() > 0 {
		return c.reader.Read(data)
	}
	return c.Conn.Read(data)
}
