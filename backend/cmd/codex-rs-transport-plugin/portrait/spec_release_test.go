//go:build capture_release

package portrait

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestReleaseCapturesPresent(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	dir := filepath.Join(filepath.Dir(file), "captures")
	p, err := LoadPortraitFile(filepath.Join(dir, "P2.json"))
	if err != nil {
		t.Fatalf("release capture P2.json missing: %v", err)
	}
	manifest, err := LoadManifestFile(filepath.Join(dir, "capture.manifest.json"))
	if err != nil {
		t.Fatalf("capture.manifest.json missing: %v", err)
	}
	idx, err := LoadPortraitsIndex(filepath.Join(dir, "portraits.json"))
	if err != nil {
		t.Fatalf("portraits.json missing: %v", err)
	}
	if idx.Fallback != FallbackPortrait {
		t.Fatalf("fallback %q", idx.Fallback)
	}
	if err := ValidateReleasePortrait(p, manifest, filepath.Join(dir, "P2.json")); err != nil {
		t.Fatal(err)
	}
	raw, err := hex.DecodeString(p.TLS.RawClientHelloHex)
	if err != nil {
		t.Fatal(err)
	}
	spec, err := ClientHelloSpecForDial(raw, "chatgpt.com")
	if err != nil {
		t.Fatal(err)
	}
	if spec == nil {
		t.Fatal("nil spec")
	}
	view, err := ParseClientHelloRecord(raw)
	if err != nil {
		t.Fatal(err)
	}
	ja3Raw, ja3Hash, _ := FingerprintsFromHello(view)
	if p.TLS.JA3Raw != "" && p.TLS.JA3Raw != ja3Raw {
		t.Fatalf("stored ja3_raw does not match decoder")
	}
	if p.TLS.JA3Hash != "" && p.TLS.JA3Hash != ja3Hash {
		t.Fatalf("stored ja3_hash does not match decoder")
	}
	if _, err := os.Stat(filepath.Join(dir, "portraits.json")); err != nil {
		t.Fatal(err)
	}
}
