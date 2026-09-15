package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type tlsObservedEvent struct {
	TS        string `json:"ts"`
	ProfileID string `json:"profile_id"`
	JA3       string `json:"ja3"`
	JA4       string `json:"ja4"`
	ALPN      string `json:"alpn"`
	RequestID string `json:"request_id"`
	AccountID int64  `json:"account_id"`
}

var observedMu sync.Mutex

func appendTLSObserved(path string, event tlsObservedEvent) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	if event.TS == "" {
		event.TS = time.Now().UTC().Format(time.RFC3339Nano)
	}
	raw, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	observedMu.Lock()
	defer observedMu.Unlock()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(raw, '\n'))
	return err
}

func redactedObservedOK(raw []byte) bool {
	s := strings.ToLower(string(raw))
	for _, banned := range []string{"bearer ", "authorization", "cookie", "proxy-authorization", "sk-"} {
		if strings.Contains(s, banned) {
			return false
		}
	}
	return true
}
