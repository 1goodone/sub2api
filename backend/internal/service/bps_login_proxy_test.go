package service

import "testing"

func TestNormalizeBPSLoginProxy(t *testing.T) {
	got, err := NormalizeBPSLoginProxy("alice:secret@proxy.example:8080")
	if err != nil || got != "http://alice:secret@proxy.example:8080" {
		t.Fatalf("plain proxy = %q, %v", got, err)
	}
	got, err = NormalizeBPSLoginProxy("socks5://alice:p%40ss@203.0.113.10:1080")
	if err != nil || got != "socks5://alice:p%40ss@203.0.113.10:1080" {
		t.Fatalf("socks proxy = %q, %v", got, err)
	}
	got, err = NormalizeBPSLoginProxy("alice:p:ss@word@proxy.example:8080")
	if err != nil || got != "http://alice:p%3Ass%40word@proxy.example:8080" {
		t.Fatalf("password punctuation = %q, %v", got, err)
	}
	for _, raw := range []string{"", "proxy.example:8080", "alice@proxy.example:8080", "alice:secret@proxy.example", "file://alice:secret@proxy.example:8080"} {
		if _, err = NormalizeBPSLoginProxy(raw); err == nil {
			t.Fatalf("accepted invalid proxy %q", raw)
		}
	}
}
