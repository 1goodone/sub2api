package portrait

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	ReusePool        = "pool"
	ReusePerRequest  = "per_request"
	MethodInspector  = "local_inspector"
	MethodPCAP       = "pcap"
	MethodSSLKeyLog  = "sslkeylog"
	FallbackPortrait = "P2"
)

var PortraitSlots = []string{"P2", "P3", "P4", "P5", "P6"}

// CaptureManifest is portrait/captures/capture.manifest.json.
type CaptureManifest struct {
	SchemaVersion        int               `json:"schema_version"`
	CodexCLIVersion      string            `json:"codex_cli_version"`
	CodexCLIGITSHA       string            `json:"codex_cli_git_sha,omitempty"`
	CodexRSCrateVersions map[string]string `json:"codex_rs_crate_versions,omitempty"`
	CapturedAt           string            `json:"captured_at"`
	OS                   string            `json:"os"`
	Arch                 string            `json:"arch"`
	Method               string            `json:"method"`
	Inspector            string            `json:"inspector"`
	Notes                string            `json:"notes,omitempty"`
}

// PortraitsIndex is portrait/captures/portraits.json.
type PortraitsIndex struct {
	Slots    []string `json:"slots"`
	Filled   []string `json:"filled"`
	Fallback string   `json:"fallback"`
}

// Portrait is one P*.json file.
type Portrait struct {
	ID            string     `json:"id"`
	SourceCapture string     `json:"source_capture"`
	TLS           TLSCapture `json:"tls"`
	HTTP2         HTTP2Capture `json:"http2"`
	Connection    ConnCapture `json:"connection"`
}

type TLSCapture struct {
	JA3Raw             string          `json:"ja3_raw"`
	JA3Hash            string          `json:"ja3_hash"`
	JA4                string          `json:"ja4"`
	CipherSuites       []uint16        `json:"cipher_suites"`
	CompressionMethods []int           `json:"compression_methods"`
	Extensions         []ExtensionNote `json:"extensions"`
	GREASE             GREASENote      `json:"grease"`
	RawClientHelloHex  string          `json:"raw_client_hello_hex"`
}

type ExtensionNote struct {
	Type      uint16   `json:"type"`
	Name      string   `json:"name,omitempty"`
	Groups    []uint16 `json:"groups,omitempty"`
	Schemes   []uint16 `json:"schemes,omitempty"`
	Protocols []string `json:"protocols,omitempty"`
	Versions  []uint16 `json:"versions,omitempty"`
	Modes     []uint16 `json:"modes,omitempty"`
}

type GREASENote struct {
	Present   bool     `json:"present"`
	Positions []string `json:"positions"`
}

type HTTP2Capture struct {
	Settings               []H2Setting `json:"settings"`
	SettingsOrder          []uint16    `json:"settings_order"`
	ConnectionWindowUpdate uint32      `json:"connection_window_update"`
	StreamInitialWindow    uint32      `json:"stream_initial_window"`
	PriorityFrames         []string    `json:"priority_frames"`
	PseudoHeaderOrder      []string    `json:"pseudo_header_order"`
	MaxHeaderListSize      uint32      `json:"max_header_list_size"`
	MaxFrameSize           uint32      `json:"max_frame_size"`
}

type H2Setting struct {
	ID    uint16 `json:"id"`
	Name  string `json:"name"`
	Value uint32 `json:"value"`
}

type ConnCapture struct {
	Reuse                    string `json:"reuse"`
	ObservedNewConnPerRequest bool   `json:"observed_new_conn_per_request"`
	SecondRequestClientHello bool   `json:"second_request_client_hello"`
	IdleTimeoutHintSeconds   *int   `json:"idle_timeout_hint_seconds"`
}

func LoadPortraitFile(path string) (*Portrait, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var p Portrait
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	return &p, nil
}

func LoadManifestFile(path string) (*CaptureManifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m CaptureManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	return &m, nil
}

func LoadPortraitsIndex(path string) (*PortraitsIndex, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var idx PortraitsIndex
	if err := json.Unmarshal(raw, &idx); err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	return &idx, nil
}

// ValidateCIFixture checks schema for testdata/fake files. It does not require
// host CLI version equality or local_inspector method.
func ValidateCIFixture(p *Portrait) error {
	if p == nil {
		return fmt.Errorf("nil portrait")
	}
	if strings.TrimSpace(p.ID) == "" {
		return fmt.Errorf("id is required")
	}
	if strings.TrimSpace(p.TLS.RawClientHelloHex) == "" {
		return fmt.Errorf("tls.raw_client_hello_hex is required")
	}
	if p.Connection.Reuse != ReusePool && p.Connection.Reuse != ReusePerRequest {
		return fmt.Errorf("connection.reuse must be %q or %q", ReusePool, ReusePerRequest)
	}
	return nil
}

// ValidateReleasePortrait is the capture_release / tools/release gate.
func ValidateReleasePortrait(p *Portrait, manifest *CaptureManifest, srcPath string) error {
	if err := ValidateCIFixture(p); err != nil {
		return err
	}
	base := filepath.Base(srcPath)
	if strings.Contains(strings.ToLower(base), "fake") {
		return fmt.Errorf("release path rejects filename containing fake: %s", srcPath)
	}
	if manifest == nil {
		return fmt.Errorf("capture manifest is required")
	}
	if manifest.SchemaVersion != SchemaVersion {
		return fmt.Errorf("manifest schema_version: got %d want %d", manifest.SchemaVersion, SchemaVersion)
	}
	if manifest.Method != MethodInspector {
		return fmt.Errorf("manifest.method must be %q, got %q (pcap/sslkeylog cannot be embedded)", MethodInspector, manifest.Method)
	}
	if CompareVersions(manifest.CodexCLIVersion, CodexUpstreamMinVersion) < 0 {
		return fmt.Errorf("codex_cli_version %q below floor %s", manifest.CodexCLIVersion, CodexUpstreamMinVersion)
	}
	if manifest.CodexCLIVersion != RequiredHostCodexCLIVersion {
		return fmt.Errorf("codex_cli_version %q must equal host pin %s", manifest.CodexCLIVersion, RequiredHostCodexCLIVersion)
	}
	if !p.Connection.SecondRequestClientHello && p.Connection.Reuse != ReusePool {
		// pool means the second request reused the conn so a second hello is absent.
	}
	if p.Connection.Reuse == ReusePool && p.Connection.SecondRequestClientHello {
		return fmt.Errorf("reuse=pool but second_request_client_hello=true (inconsistent probe)")
	}
	if p.Connection.Reuse == ReusePerRequest && !p.Connection.SecondRequestClientHello {
		return fmt.Errorf("reuse=per_request requires second_request_client_hello from the inspector probe")
	}
	return nil
}

// CompareVersions is a three-part numeric compare (major.minor.patch).
func CompareVersions(a, b string) int {
	ap, bp := parseSemver(a), parseSemver(b)
	for i := 0; i < 3; i++ {
		if ap[i] < bp[i] {
			return -1
		}
		if ap[i] > bp[i] {
			return 1
		}
	}
	return 0
}

func parseSemver(v string) [3]int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexByte(v, '-'); i >= 0 {
		v = v[:i]
	}
	var out [3]int
	parts := strings.Split(v, ".")
	for i := 0; i < len(parts) && i < 3; i++ {
		n := 0
		for _, c := range parts[i] {
			if c < '0' || c > '9' {
				break
			}
			n = n*10 + int(c-'0')
		}
		out[i] = n
	}
	return out
}
