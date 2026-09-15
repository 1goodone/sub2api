package portrait

import (
	"fmt"
	"strings"

	utls "github.com/refraction-networking/utls"
)

// ClientHelloSpecForDial builds a HelloCustom spec from a captured TLS record.
//
// Encoder (unique): FingerprintClientHello(raw) then ONLY:
//  1. regenerate key_share public keys (keep group order; GREASE shares untouched)
//  2. set SNIExtension to sni (utls.Config.ServerName does NOT override HelloCustom)
//  3. drop session_id and session_ticket material
//
// Unknown groups/extensions fail closed — no silent stripping.
func ClientHelloSpecForDial(raw []byte, sni string) (*utls.ClientHelloSpec, error) {
	sni = strings.TrimSpace(sni)
	if sni == "" {
		return nil, fmt.Errorf("sni is required")
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty client hello")
	}
	spec, err := (&utls.Fingerprinter{}).FingerprintClientHello(raw)
	if err != nil {
		return nil, fmt.Errorf("fingerprint client hello (unknown group/extension is fail-closed): %w", err)
	}
	spec.GetSessionID = func([]byte) [32]byte { return [32]byte{} }
	for _, ext := range spec.Extensions {
		switch e := ext.(type) {
		case *utls.SNIExtension:
			e.ServerName = sni
		case *utls.KeyShareExtension:
			for i := range e.KeyShares {
				if e.KeyShares[i].Group != utls.GREASE_PLACEHOLDER {
					e.KeyShares[i].Data = nil
				}
			}
		case *utls.SessionTicketExtension:
			e.Ticket = nil
			e.Session = nil
			e.Initialized = false
		}
	}
	return spec, nil
}

// AssertionsFromRaw fills the structured TLS assertion fields from a captured record.
func AssertionsFromRaw(raw []byte) (TLSCapture, error) {
	view, err := ParseClientHelloRecord(raw)
	if err != nil {
		return TLSCapture{}, err
	}
	ja3Raw, ja3Hash, ja4 := FingerprintsFromHello(view)
	exts := make([]ExtensionNote, 0, len(view.ExtensionTypes))
	for _, t := range view.ExtensionTypes {
		note := ExtensionNote{Type: t, Name: extensionName(t)}
		switch t {
		case 10:
			note.Groups = append([]uint16(nil), view.SupportedGroups...)
		case 13:
			note.Schemes = append([]uint16(nil), view.SignatureSchemes...)
		case 16:
			note.Protocols = append([]string(nil), view.ALPN...)
		case 43:
			note.Versions = append([]uint16(nil), view.SupportedVersions...)
		case 45:
			note.Modes = append([]uint16(nil), view.PSKModes...)
		case 51:
			note.Groups = append([]uint16(nil), view.KeyShareGroups...)
		}
		exts = append(exts, note)
	}
	grease := GREASENote{Present: len(view.GREASECiphers) > 0 || len(view.GREASEExtensions) > 0}
	if grease.Present {
		if len(view.GREASECiphers) > 0 {
			grease.Positions = append(grease.Positions, "cipher_suites")
		}
		if len(view.GREASEExtensions) > 0 {
			grease.Positions = append(grease.Positions, "extensions")
		}
	}
	return TLSCapture{
		JA3Raw:             ja3Raw,
		JA3Hash:            ja3Hash,
		JA4:                ja4,
		CipherSuites:       append([]uint16(nil), view.CipherSuites...),
		CompressionMethods: uint8sToInts(view.CompressionMethods),
		Extensions:         exts,
		GREASE:             grease,
		RawClientHelloHex:  strings.ToLower(fmt.Sprintf("%x", raw)),
	}, nil
}

func uint8sToInts(in []uint8) []int {
	out := make([]int, len(in))
	for i, v := range in {
		out[i] = int(v)
	}
	return out
}

func extensionName(t uint16) string {
	switch t {
	case 0:
		return "server_name"
	case 5:
		return "status_request"
	case 10:
		return "supported_groups"
	case 11:
		return "ec_point_formats"
	case 13:
		return "signature_algorithms"
	case 16:
		return "alpn"
	case 18:
		return "signed_certificate_timestamp"
	case 21:
		return "padding"
	case 23:
		return "extended_master_secret"
	case 27:
		return "compress_certificate"
	case 35:
		return "session_ticket"
	case 43:
		return "supported_versions"
	case 45:
		return "psk_key_exchange_modes"
	case 51:
		return "key_share"
	case 0xfe0d:
		return "encrypted_client_hello"
	case 0xff01:
		return "renegotiation_info"
	default:
		if isGREASE(t) {
			return "grease"
		}
		return ""
	}
}
