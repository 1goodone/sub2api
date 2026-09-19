package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/cmd/codex-desktop-transport-plugin/portrait"
)

const inspectorName = "codex-desktop-transport-plugin/tools/capture"

type captureState struct {
	mu           sync.Mutex
	hellos       [][]byte
	h2           portrait.HTTP2Capture
	tcp          int
	httpRequests int
}

type inspectorConfig struct {
	Listen     string
	ServerName string
	Certificate tls.Certificate
	MinHTTP    int
	IdleAfter  time.Duration
}

func runInspector(ctx context.Context, cfg inspectorConfig) (*captureState, net.Listener, error) {
	ln, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return nil, nil, err
	}
	state := &captureState{}
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	go acceptLoop(ctx, ln, cfg, state)
	return state, ln, nil
}

func acceptLoop(ctx context.Context, ln net.Listener, cfg inspectorConfig, state *captureState) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go handleConn(ctx, conn, cfg, state)
	}
}

func handleConn(ctx context.Context, conn net.Conn, cfg inspectorConfig, state *captureState) {
	defer conn.Close()
	state.mu.Lock()
	state.tcp++
	state.mu.Unlock()

	raw, replay, err := captureClientHelloRecord(conn, time.Now().Add(10*time.Second))
	if err != nil {
		fmt.Fprintf(os.Stderr, "capture client hello: %v\n", err)
		return
	}
	state.mu.Lock()
	state.hellos = append(state.hellos, append([]byte(nil), raw...))
	state.mu.Unlock()

	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cfg.Certificate},
		NextProtos:   []string{"h2", "http/1.1"},
		MinVersion:   tls.VersionTLS12,
	}
	server := tls.Server(replay, tlsCfg)
	if err := server.HandshakeContext(ctx); err != nil {
		kind := "other"
		if looksLikeRustls(raw) {
			kind = "rustls-like"
		}
		fmt.Fprintf(os.Stderr, "tls handshake (%s): %v\n", kind, err)
		return
	}
	alpn := server.ConnectionState().NegotiatedProtocol
	switch alpn {
	case "h2":
		var h2 portrait.HTTP2Capture
		n, err := serveHTTP2Capture(server, &h2)
		if err != nil && n == 0 {
			fmt.Fprintf(os.Stderr, "http2: %v\n", err)
		}
		state.mu.Lock()
		state.httpRequests += n
		if len(state.h2.Settings) == 0 {
			state.h2 = h2
		}
		state.mu.Unlock()
	default:
		n := serveHTTP1Capture(server)
		state.mu.Lock()
		state.httpRequests += n
		state.mu.Unlock()
	}
}

func (s *captureState) snapshot() (hellos [][]byte, h2 portrait.HTTP2Capture, tcp, reqs int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	hellos = append([][]byte(nil), s.hellos...)
	h2 = s.h2
	return hellos, h2, s.tcp, s.httpRequests
}

func dumpHellos(dir string, state *captureState) {
	hellos, _, tcp, reqs := state.snapshot()
	helloDir := filepath.Join(dir, "hellos")
	_ = os.MkdirAll(helloDir, 0o755)
	fmt.Fprintf(os.Stderr, "dump hellos=%d tcp=%d http=%d -> %s\n", len(hellos), tcp, reqs, helloDir)
	for i, raw := range hellos {
		_ = os.WriteFile(filepath.Join(helloDir, fmt.Sprintf("%02d.bin", i)), raw, 0o644)
		view, err := portrait.ParseClientHelloRecord(raw)
		if err != nil {
			fmt.Fprintf(os.Stderr, "hello[%d] len=%d parse=%v rustls=%v\n", i, len(raw), err, false)
			continue
		}
		fmt.Fprintf(os.Stderr, "hello[%d] len=%d rustls=%v legacy=0x%04x versions=%v key_share=%v groups=%v ciphers=%d grease_c=%d grease_e=%d ext=%v sni=%q alpn=%v\n",
			i, len(raw), looksLikeRustls(raw), view.LegacyVersion, view.SupportedVersions, view.KeyShareGroups, view.SupportedGroups, len(view.CipherSuites), len(view.GREASECiphers), len(view.GREASEExtensions), view.ExtensionTypes, view.SNI, view.ALPN)
	}
}

func waitForRequests(ctx context.Context, state *captureState, minHTTP int) error {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	lastLog := time.Time{}
	for {
		select {
		case <-ctx.Done():
			hellos, _, tcp, reqs := state.snapshot()
			return fmt.Errorf("timeout waiting for %d HTTP requests (got %d, tcp=%d hellos=%d rustls=%d): %w", minHTTP, reqs, tcp, len(hellos), rustlsHelloCount(hellos), ctx.Err())
		case <-ticker.C:
			hellos, _, tcp, reqs := state.snapshot()
			if reqs >= minHTTP || rustlsHelloCount(hellos) >= 2 {
				return nil
			}
			if time.Since(lastLog) >= 3*time.Second {
				fmt.Fprintf(os.Stderr, "waiting: tcp=%d hellos=%d rustls=%d http=%d/%d\n", tcp, len(hellos), rustlsHelloCount(hellos), reqs, minHTTP)
				lastLog = time.Now()
			}
		}
	}
}

func buildPortrait(state *captureState, cliVersion string) (*portrait.Portrait, *portrait.CaptureManifest, error) {
	hellos, h2, tcp, reqs := state.snapshot()
	if len(hellos) == 0 {
		return nil, nil, fmt.Errorf("no ClientHello captured")
	}
	if reqs < 2 && rustlsHelloCount(hellos) < 2 {
		return nil, nil, fmt.Errorf("reuse probe requires >=2 HTTP requests or >=2 rustls ClientHellos, got http=%d rustls=%d", reqs, rustlsHelloCount(hellos))
	}
	raw, rustlsHellos := pickRustlsHello(hellos)
	if raw == nil && strings.HasPrefix(cliVersion, "fake") && len(hellos) > 0 {
		raw = hellos[0]
		rustlsHellos = len(hellos)
	}
	if raw == nil {
		return nil, nil, fmt.Errorf("captured %d ClientHellos but none look like Desktop engine TLS (rustls TLS1.3 or Schannel TLS1.2, no GREASE)", len(hellos))
	}
	tlsCap, err := portrait.AssertionsFromRaw(raw)
	if err != nil {
		return nil, nil, err
	}
	secondHello := rustlsHellos > 1
	reuse := portrait.ReusePerRequest
	if rustlsHellos == 1 && tcp >= 1 && reqs >= 2 {
		reuse = portrait.ReusePool
		secondHello = false
	}
	p := &portrait.Portrait{
		ID:            "P2",
		SourceCapture: "capture.manifest.json",
		TLS:           tlsCap,
		HTTP2:         h2,
		Connection: portrait.ConnCapture{
			Reuse:                     reuse,
			ObservedNewConnPerRequest: secondHello || tcp > 1,
			SecondRequestClientHello:  secondHello,
		},
	}
	now := time.Now().UTC().Format(time.RFC3339)
	manifest := &portrait.CaptureManifest{
		SchemaVersion:   portrait.SchemaVersion,
		CodexCLIVersion: cliVersion,
		CapturedAt:      now,
		OS:              runtime.GOOS,
		Arch:            runtime.GOARCH,
		Method:          portrait.MethodInspector,
		Inspector:       inspectorName,
		Notes:           notesFor(reqs, rustlsHellos),
	}
	return p, manifest, nil
}

func serveHTTP1Capture(conn net.Conn) int {
	_ = conn.SetDeadline(time.Now().Add(8 * time.Second))
	br := bufio.NewReader(conn)
	n := 0
	for n < 8 {
		req, err := http.ReadRequest(br)
		if err != nil {
			break
		}
		_, _ = io.Copy(io.Discard, req.Body)
		_ = req.Body.Close()
		body := inspectorJSON(req.URL.Path)
		_, _ = io.WriteString(conn, fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", len(body), body))
		n++
		break
	}
	return n
}

func notesFor(httpReqs, rustlsHellos int) string {
	if httpReqs >= 2 {
		return ">=2 HTTP requests same inspector session; no production load"
	}
	return fmt.Sprintf("TLS reuse probe: %d rustls ClientHellos; HTTP/2 SETTINGS unavailable (ECH handshake with Go crypto/tls failed); no production load", rustlsHellos)
}

func rustlsHelloCount(hellos [][]byte) int {
	_, n := pickRustlsHello(hellos)
	return n
}

func pickRustlsHello(hellos [][]byte) (raw []byte, rustlsCount int) {
	for _, h := range hellos {
		if !looksLikeRustls(h) {
			continue
		}
		rustlsCount++
		if raw == nil {
			raw = h
		}
	}
	return raw, rustlsCount
}

func looksLikeRustls(raw []byte) bool {
	return looksLikeDesktopHello(raw)
}

func looksLikeDesktopHello(raw []byte) bool {
	view, err := portrait.ParseClientHelloRecord(raw)
	if err != nil {
		return false
	}
	if len(view.GREASECiphers) > 0 || len(view.GREASEExtensions) > 0 {
		return false
	}
	if view.SNI == "" || len(view.SupportedGroups) == 0 {
		return false
	}
	has13 := false
	for _, ver := range view.SupportedVersions {
		if ver == 0x0304 {
			has13 = true
			break
		}
	}
	if has13 && len(view.KeyShareGroups) > 0 {
		return true
	}
	// Desktop-bundled Windows engine (0.153.4) uses Schannel TLS 1.2: no
	// supported_versions, no key_share, X25519/P-256/P-384, no GREASE.
	return view.LegacyVersion == 0x0303 && len(view.KeyShareGroups) == 0
}

func writeCaptureDir(dir string, p *portrait.Portrait, manifest *portrait.CaptureManifest) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	idx := portrait.PortraitsIndex{
		Slots:    append([]string(nil), portrait.PortraitSlots...),
		Filled:   []string{"P2"},
		Fallback: portrait.FallbackPortrait,
	}
	files := map[string]any{
		"P2.json":                p,
		"capture.manifest.json":  manifest,
		"portraits.json":         idx,
	}
	for name, v := range files {
		raw, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return err
		}
		raw = append(raw, '\n')
		if err := os.WriteFile(filepath.Join(dir, name), raw, 0o644); err != nil {
			return err
		}
	}
	return nil
}
