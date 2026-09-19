package portrait

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestValidateCIFixtureRequiresReuseEnum(t *testing.T) {
	p := &Portrait{ID: "fake-P2", TLS: TLSCapture{RawClientHelloHex: "16"}, Connection: ConnCapture{Reuse: "unknown"}}
	if err := ValidateCIFixture(p); err == nil {
		t.Fatal("unknown reuse must fail")
	}
	p.Connection.Reuse = ReusePerRequest
	if err := ValidateCIFixture(p); err != nil {
		t.Fatal(err)
	}
}

func TestValidateReleasePortraitRejectsFakeNameAndPCAP(t *testing.T) {
	p := &Portrait{
		ID: "P2",
		TLS: TLSCapture{RawClientHelloHex: "16"},
		Connection: ConnCapture{
			Reuse:                    ReusePerRequest,
			SecondRequestClientHello: true,
		},
	}
	manifest := &CaptureManifest{
		SchemaVersion:   SchemaVersion,
		CodexCLIVersion: RequiredHostCodexCLIVersion,
		Method:          MethodPCAP,
	}
	if err := ValidateReleasePortrait(p, manifest, "P2.json"); err == nil {
		t.Fatal("pcap method must fail")
	}
	manifest.Method = MethodInspector
	if err := ValidateReleasePortrait(p, manifest, "fake_P2.json"); err == nil {
		t.Fatal("fake filename must fail")
	}
	if err := ValidateReleasePortrait(p, manifest, "P2.json"); err != nil {
		t.Fatal(err)
	}
}

func TestValidateReleasePortraitPinsHostCLIVersion(t *testing.T) {
	p := &Portrait{
		ID:         "P2",
		TLS:        TLSCapture{RawClientHelloHex: "16"},
		Connection: ConnCapture{Reuse: ReusePerRequest, SecondRequestClientHello: true},
	}
	manifest := &CaptureManifest{
		SchemaVersion:   SchemaVersion,
		CodexCLIVersion: "0.145.0",
		Method:          MethodInspector,
	}
	if err := ValidateReleasePortrait(p, manifest, "P2.json"); err == nil {
		t.Fatal("wrong CLI version must fail")
	}
}

func TestCompareVersions(t *testing.T) {
	if CompareVersions("0.146.0", CodexUpstreamMinVersion) < 0 {
		t.Fatal("0.146.0 should be >= floor")
	}
	if CompareVersions("0.143.0", CodexUpstreamMinVersion) >= 0 {
		t.Fatal("0.143.0 should be below floor")
	}
}

func TestFakeTestdataFilenamesContainFake(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	dir := filepath.Join(filepath.Dir(file), "testdata", "fake")
	matches, err := filepath.Glob(filepath.Join(dir, "*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Skip("fake fixtures not generated yet")
	}
	for _, m := range matches {
		base := filepath.Base(m)
		if base == ".gitkeep" {
			continue
		}
		if filepath.Ext(base) == "" {
			continue
		}
		if !containsFake(base) {
			t.Fatalf("CI fixture %s must contain 'fake' in the filename", base)
		}
		if err := ValidateCIFixtureMustLoad(m); err != nil {
			t.Fatal(err)
		}
	}
}

func containsFake(name string) bool {
	for i := 0; i+4 <= len(name); i++ {
		if name[i:i+4] == "fake" {
			return true
		}
	}
	return false
}

func ValidateCIFixtureMustLoad(path string) error {
	if filepath.Ext(path) != ".json" {
		return nil
	}
	base := filepath.Base(path)
	if base == "fake_portraits.json" || base == "fake_capture.manifest.json" {
		return nil
	}
	p, err := LoadPortraitFile(path)
	if err != nil {
		return err
	}
	return ValidateCIFixture(p)
}
