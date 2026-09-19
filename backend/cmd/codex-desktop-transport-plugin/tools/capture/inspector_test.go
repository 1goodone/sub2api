package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/cmd/codex-desktop-transport-plugin/portrait"
)

func TestInspectorSelftestWritesFakePortrait(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	pki, err := generateLocalCA("chatgpt.com")
	if err != nil {
		t.Fatal(err)
	}
	cert := pki.Certificate
	state, ln, err := runInspector(ctx, inspectorConfig{
		Listen:      "127.0.0.1:0",
		ServerName:  "chatgpt.com",
		Certificate: cert,
		MinHTTP:     2,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	if err := runSelftest(ctx, ln.Addr().String(), "chatgpt.com", cert); err != nil {
		t.Fatal(err)
	}
	if err := waitForRequests(ctx, state, 2); err != nil {
		t.Fatal(err)
	}
	p, manifest, err := buildPortrait(state, "fake-0.0.0")
	if err != nil {
		t.Fatal(err)
	}
	p.ID = "fake-P2"
	if err := portrait.ValidateCIFixture(p); err != nil {
		t.Fatal(err)
	}
	if err := writeCaptureDir(dir, p, manifest); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "P2.json")); err != nil {
		t.Fatal(err)
	}
	loaded, err := portrait.LoadPortraitFile(filepath.Join(dir, "P2.json"))
	if err != nil {
		t.Fatal(err)
	}
	rawHello := loaded.TLS.RawClientHelloHex
	if rawHello == "" {
		t.Fatal("empty raw hello")
	}
	if loaded.Connection.Reuse != portrait.ReusePool && loaded.Connection.Reuse != portrait.ReusePerRequest {
		t.Fatalf("reuse %q", loaded.Connection.Reuse)
	}
}
