package portrait

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

func FingerprintsFromHello(v *ClientHelloView) (ja3Raw, ja3Hash, ja4 string) {
	ja3Raw = ja3RawString(v)
	sum := md5.Sum([]byte(ja3Raw))
	ja3Hash = hex.EncodeToString(sum[:])
	ja4 = ja4String(v)
	return ja3Raw, ja3Hash, ja4
}

func ja3RawString(v *ClientHelloView) string {
	ciphers := joinUint16(filterGREASE(v.CipherSuites), "-")
	exts := joinUint16(filterGREASE(v.ExtensionTypes), "-")
	groups := joinUint16(filterGREASE(v.SupportedGroups), "-")
	points := joinUint16(v.PointFormats, "-")
	return fmt.Sprintf("%d,%s,%s,%s,%s", v.LegacyVersion, ciphers, exts, groups, points)
}

func ja4String(v *ClientHelloView) string {
	ver := "00"
	maxVer := v.LegacyVersion
	for _, sv := range v.SupportedVersions {
		if !isGREASE(sv) && sv > maxVer {
			maxVer = sv
		}
	}
	switch maxVer {
	case 0x0304:
		ver = "13"
	case 0x0303:
		ver = "12"
	case 0x0302:
		ver = "11"
	case 0x0301:
		ver = "10"
	}
	sni := "i"
	if v.SNI != "" && !isLiteralIP(v.SNI) {
		sni = "d"
	}
	ciphers := filterGREASE(v.CipherSuites)
	exts := make([]uint16, 0, len(v.ExtensionTypes))
	for _, t := range v.ExtensionTypes {
		if isGREASE(t) || t == 0x0000 || t == 0x0010 {
			continue
		}
		exts = append(exts, t)
	}
	alpn := "00"
	if len(v.ALPN) > 0 {
		p := v.ALPN[0]
		alpn = ja4ALPN(p)
	}
	a := fmt.Sprintf("t%s%s%02d%02d%s", ver, sni, min99(len(ciphers)), min99(len(exts)), alpn)

	cipherHex := make([]string, len(ciphers))
	for i, c := range ciphers {
		cipherHex[i] = fmt.Sprintf("%04x", c)
	}
	sort.Strings(cipherHex)
	bSum := sha256.Sum256([]byte(strings.Join(cipherHex, ",")))
	b := hex.EncodeToString(bSum[:])[:12]

	extHex := make([]string, len(exts))
	for i, e := range exts {
		extHex[i] = fmt.Sprintf("%04x", e)
	}
	sort.Strings(extHex)
	sig := joinUint16(v.SignatureSchemes, ",")
	cInput := strings.Join(extHex, ",")
	if sig != "" {
		cInput += "_" + toHexList(v.SignatureSchemes)
	}
	cSum := sha256.Sum256([]byte(cInput))
	c := hex.EncodeToString(cSum[:])[:12]
	return a + "_" + b + "_" + c
}

func ja4ALPN(p string) string {
	var chars []rune
	for _, r := range p {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			chars = append(chars, r)
		}
	}
	if len(chars) == 0 {
		return "00"
	}
	return strings.ToLower(string([]rune{chars[0], chars[len(chars)-1]}))
}

func filterGREASE(in []uint16) []uint16 {
	out := make([]uint16, 0, len(in))
	for _, v := range in {
		if !isGREASE(v) {
			out = append(out, v)
		}
	}
	return out
}

func joinUint16(in []uint16, sep string) string {
	if len(in) == 0 {
		return ""
	}
	parts := make([]string, len(in))
	for i, v := range in {
		parts[i] = strconv.FormatUint(uint64(v), 10)
	}
	return strings.Join(parts, sep)
}

func toHexList(in []uint16) string {
	parts := make([]string, len(in))
	for i, v := range in {
		parts[i] = fmt.Sprintf("%04x", v)
	}
	return strings.Join(parts, ",")
}

func min99(n int) int {
	if n > 99 {
		return 99
	}
	return n
}

func isLiteralIP(s string) bool {
	if strings.Contains(s, ":") {
		return true
	}
	dots := 0
	for _, c := range s {
		if c == '.' {
			dots++
			continue
		}
		if c < '0' || c > '9' {
			return false
		}
	}
	return dots == 3
}
