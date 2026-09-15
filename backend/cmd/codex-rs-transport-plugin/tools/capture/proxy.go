package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// serveConnectProxy accepts HTTP CONNECT and forwards chatgpt.com:443 to inspectorAddr.
// Used when the machine cannot edit the system hosts file.
func serveConnectProxy(ln net.Listener, inspectorAddr string, allowHost string) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go handleConnect(conn, inspectorAddr, allowHost)
	}
}

func handleConnect(client net.Conn, inspectorAddr, allowHost string) {
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(30 * time.Second))
	br := bufio.NewReader(client)
	req, err := http.ReadRequest(br)
	if err != nil {
		return
	}
	if req.Method != http.MethodConnect {
		_, _ = io.WriteString(client, "HTTP/1.1 405 Method Not Allowed\r\nConnection: close\r\n\r\n")
		return
	}
	host, _, err := net.SplitHostPort(req.Host)
	if err != nil {
		host = req.Host
	}
	if !strings.EqualFold(host, allowHost) {
		_, _ = io.WriteString(client, "HTTP/1.1 403 Forbidden\r\nConnection: close\r\n\r\n")
		return
	}
	upstream, err := net.DialTimeout("tcp", inspectorAddr, 5*time.Second)
	if err != nil {
		_, _ = io.WriteString(client, "HTTP/1.1 502 Bad Gateway\r\nConnection: close\r\n\r\n")
		return
	}
	defer upstream.Close()
	if _, err := io.WriteString(client, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return
	}
	_ = client.SetDeadline(time.Time{})
	errCh := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(upstream, br); errCh <- struct{}{} }()
	go func() { _, _ = io.Copy(client, upstream); errCh <- struct{}{} }()
	<-errCh
}

func listenConnectProxy(addr string) (net.Listener, error) {
	if strings.TrimSpace(addr) == "" {
		return nil, fmt.Errorf("empty proxy addr")
	}
	return net.Listen("tcp", addr)
}
