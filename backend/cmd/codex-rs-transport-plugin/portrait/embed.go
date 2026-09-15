package portrait

import (
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
)

//go:embed captures/P2.json captures/portraits.json captures/capture.manifest.json
var captureFS embed.FS

var (
	embeddedOnce     sync.Once
	embeddedPortrait *Portrait
	embeddedIndex    *PortraitsIndex
	embeddedManifest *CaptureManifest
	embeddedErr      error
)

func loadEmbedded() {
	embeddedOnce.Do(func() {
		embeddedPortrait, embeddedErr = loadEmbeddedPortrait("captures/P2.json")
		if embeddedErr != nil {
			return
		}
		embeddedIndex, embeddedErr = loadEmbeddedIndex("captures/portraits.json")
		if embeddedErr != nil {
			return
		}
		embeddedManifest, embeddedErr = loadEmbeddedManifest("captures/capture.manifest.json")
		if embeddedErr != nil {
			return
		}
		if err := ValidateReleasePortrait(embeddedPortrait, embeddedManifest, "captures/P2.json"); err != nil {
			embeddedErr = err
			return
		}
		if embeddedManifest.CodexCLIVersion != RequiredHostCodexCLIVersion {
			embeddedErr = fmt.Errorf("embedded capture %q != host pin %s", embeddedManifest.CodexCLIVersion, RequiredHostCodexCLIVersion)
		}
	})
}

func loadEmbeddedPortrait(name string) (*Portrait, error) {
	raw, err := captureFS.ReadFile(name)
	if err != nil {
		return nil, err
	}
	var p Portrait
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func loadEmbeddedIndex(name string) (*PortraitsIndex, error) {
	raw, err := captureFS.ReadFile(name)
	if err != nil {
		return nil, err
	}
	var idx PortraitsIndex
	if err := json.Unmarshal(raw, &idx); err != nil {
		return nil, err
	}
	return &idx, nil
}

func loadEmbeddedManifest(name string) (*CaptureManifest, error) {
	raw, err := captureFS.ReadFile(name)
	if err != nil {
		return nil, err
	}
	var m CaptureManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func EmbeddedP2() (*Portrait, error) {
	loadEmbedded()
	if embeddedErr != nil {
		return nil, embeddedErr
	}
	return embeddedPortrait, nil
}

func EmbeddedManifest() (*CaptureManifest, error) {
	loadEmbedded()
	if embeddedErr != nil {
		return nil, embeddedErr
	}
	return embeddedManifest, nil
}

func EmbeddedRawHello() ([]byte, error) {
	p, err := EmbeddedP2()
	if err != nil {
		return nil, err
	}
	raw, err := hex.DecodeString(p.TLS.RawClientHelloHex)
	if err != nil {
		return nil, fmt.Errorf("raw_client_hello_hex: %w", err)
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty hello")
	}
	return raw, nil
}

func EmbeddedIndex() (*PortraitsIndex, error) {
	loadEmbedded()
	if embeddedErr != nil {
		return nil, embeddedErr
	}
	return embeddedIndex, nil
}

func PortraitForAccount(accountID int64, pin string) (*Portrait, string, error) {
	idx, err := EmbeddedIndex()
	if err != nil {
		return nil, "", err
	}
	fallback := idx.Fallback
	if fallback == "" {
		fallback = FallbackPortrait
	}
	id := AssignPortrait(accountID, pin, idx.Filled, fallback)
	p, err := EmbeddedP2()
	if err != nil {
		return nil, "", err
	}
	if id != p.ID {
		id = p.ID
	}
	return p, id, nil
}

func FixtureALPN(p *Portrait) []string {
	if p == nil {
		return nil
	}
	for _, ext := range p.TLS.Extensions {
		if ext.Type == 16 && len(ext.Protocols) > 0 {
			return append([]string(nil), ext.Protocols...)
		}
	}
	return nil
}
