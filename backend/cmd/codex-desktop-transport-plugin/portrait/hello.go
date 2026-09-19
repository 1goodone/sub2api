package portrait

import (
	"encoding/binary"
	"fmt"
)

// ClientHelloView is a decoded ClientHello used for JA3/assertions.
// SNI hostname, session_id bytes, and key_share public keys are parsed but
// excluded from CanonicalFingerprint.
type ClientHelloView struct {
	LegacyVersion      uint16
	SessionID          []byte
	CipherSuites       []uint16
	CompressionMethods []uint8
	ExtensionTypes     []uint16
	SNI                string
	ALPN               []string
	SupportedGroups    []uint16
	SupportedVersions  []uint16
	SignatureSchemes   []uint16
	PointFormats       []uint16
	PSKModes           []uint16
	KeyShareGroups     []uint16
	GREASECiphers      []uint16
	GREASEExtensions   []uint16
	RawRecord          []byte
}

func ParseClientHelloRecord(raw []byte) (*ClientHelloView, error) {
	if len(raw) < 9 {
		return nil, fmt.Errorf("client hello too short: %d", len(raw))
	}
	body, err := handshakeFromTLSRecords(raw)
	if err != nil {
		return nil, err
	}
	if len(body) < 4 || body[0] != 0x01 {
		return nil, fmt.Errorf("not a client hello handshake")
	}
	hsLen := int(body[1])<<16 | int(body[2])<<8 | int(body[3])
	if 4+hsLen > len(body) {
		return nil, fmt.Errorf("truncated client hello handshake")
	}
	msg := body[4 : 4+hsLen]
	view, err := parseClientHelloBody(msg)
	if err != nil {
		return nil, err
	}
	view.RawRecord = append([]byte(nil), raw...)
	return view, nil
}

func handshakeFromTLSRecords(raw []byte) ([]byte, error) {
	var out []byte
	off := 0
	for off < len(raw) {
		if off+5 > len(raw) {
			return nil, fmt.Errorf("truncated TLS record header")
		}
		typ := raw[off]
		n := int(binary.BigEndian.Uint16(raw[off+3 : off+5]))
		if off+5+n > len(raw) {
			return nil, fmt.Errorf("truncated TLS record body")
		}
		if typ != 0x16 {
			return nil, fmt.Errorf("expected handshake record, got type %d", typ)
		}
		out = append(out, raw[off+5:off+5+n]...)
		off += 5 + n
		if len(out) >= 4 {
			hsLen := int(out[1])<<16 | int(out[2])<<8 | int(out[3])
			if len(out) >= 4+hsLen {
				return out, nil
			}
		}
	}
	return out, nil
}

func parseClientHelloBody(msg []byte) (*ClientHelloView, error) {
	if len(msg) < 34 {
		return nil, fmt.Errorf("client hello body too short")
	}
	v := &ClientHelloView{LegacyVersion: binary.BigEndian.Uint16(msg[0:2])}
	off := 34 // version + random
	if off >= len(msg) {
		return nil, fmt.Errorf("truncated after random")
	}
	sidLen := int(msg[off])
	off++
	if off+sidLen > len(msg) {
		return nil, fmt.Errorf("truncated session id")
	}
	v.SessionID = append([]byte(nil), msg[off:off+sidLen]...)
	off += sidLen
	if off+2 > len(msg) {
		return nil, fmt.Errorf("truncated cipher length")
	}
	csLen := int(binary.BigEndian.Uint16(msg[off : off+2]))
	off += 2
	if off+csLen > len(msg) || csLen%2 != 0 {
		return nil, fmt.Errorf("truncated cipher suites")
	}
	for i := 0; i < csLen; i += 2 {
		id := binary.BigEndian.Uint16(msg[off+i : off+i+2])
		v.CipherSuites = append(v.CipherSuites, id)
		if isGREASE(id) {
			v.GREASECiphers = append(v.GREASECiphers, id)
		}
	}
	off += csLen
	if off >= len(msg) {
		return nil, fmt.Errorf("truncated compression length")
	}
	cmLen := int(msg[off])
	off++
	if off+cmLen > len(msg) {
		return nil, fmt.Errorf("truncated compression methods")
	}
	v.CompressionMethods = append([]uint8(nil), msg[off:off+cmLen]...)
	off += cmLen
	if off == len(msg) {
		return v, nil
	}
	if off+2 > len(msg) {
		return nil, fmt.Errorf("truncated extensions length")
	}
	exLen := int(binary.BigEndian.Uint16(msg[off : off+2]))
	off += 2
	end := off + exLen
	if end > len(msg) {
		return nil, fmt.Errorf("truncated extensions")
	}
	for off+4 <= end {
		extType := binary.BigEndian.Uint16(msg[off : off+2])
		l := int(binary.BigEndian.Uint16(msg[off+2 : off+4]))
		off += 4
		if off+l > end {
			return nil, fmt.Errorf("truncated extension %d", extType)
		}
		data := msg[off : off+l]
		off += l
		v.ExtensionTypes = append(v.ExtensionTypes, extType)
		if isGREASE(extType) {
			v.GREASEExtensions = append(v.GREASEExtensions, extType)
		}
		parseExtension(v, extType, data)
	}
	return v, nil
}

func parseExtension(v *ClientHelloView, extType uint16, data []byte) {
	switch extType {
	case 0: // server_name
		v.SNI = parseSNI(data)
	case 10: // supported_groups
		v.SupportedGroups = parseUint16List(data)
	case 11: // ec_point_formats
		if len(data) >= 1 {
			n := int(data[0])
			if 1+n <= len(data) {
				v.PointFormats = append([]uint16(nil), bytesToUint16s(data[1:1+n])...)
			}
		}
	case 13: // signature_algorithms
		v.SignatureSchemes = parseUint16List(data)
	case 16: // alpn
		v.ALPN = parseALPN(data)
	case 43: // supported_versions
		v.SupportedVersions = parseVersions(data)
	case 45: // psk_key_exchange_modes
		for _, b := range data {
			if len(v.PSKModes) == 0 && b == byte(len(data)-1) && len(data) > 1 {
				// first byte is length
				continue
			}
			v.PSKModes = append(v.PSKModes, uint16(b))
		}
		if len(data) > 0 {
			n := int(data[0])
			v.PSKModes = v.PSKModes[:0]
			for i := 0; i < n && 1+i < len(data); i++ {
				v.PSKModes = append(v.PSKModes, uint16(data[1+i]))
			}
		}
	case 51: // key_share
		v.KeyShareGroups = parseKeyShareGroups(data)
	}
}

func parseSNI(data []byte) string {
	if len(data) < 5 {
		return ""
	}
	listLen := int(binary.BigEndian.Uint16(data[0:2]))
	off := 2
	end := 2 + listLen
	if end > len(data) {
		end = len(data)
	}
	for off+3 <= end {
		nameType := data[off]
		n := int(binary.BigEndian.Uint16(data[off+1 : off+3]))
		off += 3
		if off+n > end {
			return ""
		}
		if nameType == 0 {
			return string(data[off : off+n])
		}
		off += n
	}
	return ""
}

func parseUint16List(data []byte) []uint16 {
	if len(data) < 2 {
		return nil
	}
	n := int(binary.BigEndian.Uint16(data[0:2]))
	off := 2
	if off+n > len(data) {
		n = len(data) - off
	}
	var out []uint16
	for i := 0; i+1 < n; i += 2 {
		out = append(out, binary.BigEndian.Uint16(data[off+i:off+i+2]))
	}
	return out
}

func parseVersions(data []byte) []uint16 {
	if len(data) < 1 {
		return nil
	}
	n := int(data[0])
	off := 1
	var out []uint16
	for i := 0; i+1 < n && off+i+1 < len(data); i += 2 {
		out = append(out, binary.BigEndian.Uint16(data[off+i:off+i+2]))
	}
	return out
}

func parseALPN(data []byte) []string {
	if len(data) < 2 {
		return nil
	}
	n := int(binary.BigEndian.Uint16(data[0:2]))
	off := 2
	end := 2 + n
	if end > len(data) {
		end = len(data)
	}
	var out []string
	for off < end {
		l := int(data[off])
		off++
		if off+l > end {
			break
		}
		out = append(out, string(data[off:off+l]))
		off += l
	}
	return out
}

func parseKeyShareGroups(data []byte) []uint16 {
	if len(data) < 2 {
		return nil
	}
	n := int(binary.BigEndian.Uint16(data[0:2]))
	off := 2
	end := 2 + n
	if end > len(data) {
		end = len(data)
	}
	var out []uint16
	for off+4 <= end {
		group := binary.BigEndian.Uint16(data[off : off+2])
		kl := int(binary.BigEndian.Uint16(data[off+2 : off+4]))
		off += 4
		if off+kl > end {
			break
		}
		out = append(out, group)
		off += kl
	}
	return out
}

func bytesToUint16s(b []byte) []uint16 {
	out := make([]uint16, len(b))
	for i, v := range b {
		out[i] = uint16(v)
	}
	return out
}

func isGREASE(v uint16) bool {
	return v&0x0f0f == 0x0a0a && v>>8 == v&0xff
}
