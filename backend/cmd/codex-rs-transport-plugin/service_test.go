package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/cmd/codex-rs-transport-plugin/portrait"
	pluginv1 "github.com/Wei-Shaw/sub2api/pkg/pluginapi/v1"
	"google.golang.org/grpc/metadata"
)

func TestManifestMatchesRuntime(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("manifest.source.json")
	if err != nil {
		t.Fatal(err)
	}
	var value struct {
		ID           string `json:"id"`
		Version      string `json:"version"`
		Capabilities []struct {
			ID string `json:"id"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	if value.ID != pluginID || value.Version != pluginVersion || len(value.Capabilities) != 1 || value.Capabilities[0].ID != capabilityID {
		t.Fatalf("manifest identity does not match runtime: %#v", value)
	}
}

func TestParseConfigRejectsPool(t *testing.T) {
	t.Parallel()
	if _, _, err := parseConfig([]byte(`{"enabled":true,"connection_reuse":"pool"}`)); err == nil {
		t.Fatal("pool must be rejected")
	}
	cfg, _, err := parseConfig([]byte(`{"enabled":true}`))
	if err != nil || cfg.ConnectionReuse != "per_request" {
		t.Fatalf("default reuse: %+v %v", cfg, err)
	}
}

func TestTestConfigFailsWhenDisabled(t *testing.T) {
	svc := newTransportService()
	resp, err := svc.TestConfig(context.Background(), &pluginv1.TestConfigRequest{ConfigJson: []byte(`{"enabled":false}`)})
	if err != nil || resp.GetSuccess() {
		t.Fatalf("disabled test should fail: %+v %v", resp, err)
	}
}

func TestForwardPassesHeadersAndRejectsWhenDisabled(t *testing.T) {
	svc := newTransportService()
	stream := &fakeForwardStream{
		ctx: context.Background(),
		requests: []*pluginv1.ForwardRequest{{
			Frame: &pluginv1.ForwardRequest_Start{Start: &pluginv1.ForwardRequestStart{
				RequestId: "r1", Method: http.MethodPost, Url: "https://chatgpt.com/backend-api/codex",
				Platform: "openai", AccountType: "oauth",
				Headers: map[string]*pluginv1.HeaderValues{"X-Test": {Values: []string{"keep"}}},
			}},
		}},
	}
	if err := svc.Forward(stream); err != nil {
		t.Fatal(err)
	}
	if len(stream.responses) == 0 || stream.responses[0].GetError() == nil || stream.responses[0].GetError().GetCode() != "TRANSPORT_DISABLED" {
		t.Fatalf("expected disabled error, got %#v", stream.responses)
	}

	svc.enabled.Store(true)
	var sawHeader string
	svc.newRoundTripper = func(string, *portrait.Portrait) (http.RoundTripper, error) {
		return roundTripFunc(func(req *http.Request) (*http.Response, error) {
			sawHeader = req.Header.Get("X-Test")
			return &http.Response{StatusCode: 200, Status: "200 OK", Proto: "HTTP/2.0", ProtoMajor: 2, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader([]byte("ok"))), Request: req}, nil
		}), nil
	}
	stream = &fakeForwardStream{
		ctx: context.Background(),
		requests: []*pluginv1.ForwardRequest{
			{Frame: &pluginv1.ForwardRequest_Start{Start: &pluginv1.ForwardRequestStart{
				RequestId: "r2", Method: http.MethodPost, Url: "https://chatgpt.com/backend-api/codex",
				Platform: "openai", AccountType: "oauth",
				Headers: map[string]*pluginv1.HeaderValues{"X-Test": {Values: []string{"keep"}}},
			}}},
			{Frame: &pluginv1.ForwardRequest_BodyEnd{BodyEnd: true}},
		},
	}
	if err := svc.Forward(stream); err != nil {
		t.Fatal(err)
	}
	if sawHeader != "keep" {
		t.Fatalf("headers were not passed through: %q", sawHeader)
	}
}

func TestTLSObservedJSONLOmitsSecrets(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "obs.jsonl")
	event := tlsObservedEvent{ProfileID: "P2", JA3: "abc", JA4: "t13", ALPN: "h2", RequestID: "rid", AccountID: 7}
	raw, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	if !redactedObservedOK(raw) {
		t.Fatal(raw)
	}
	if err := appendTLSObserved(path, event); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !redactedObservedOK(got) {
		t.Fatalf("observed file leaked secrets: %s", got)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

type fakeForwardStream struct {
	ctx       context.Context
	requests  []*pluginv1.ForwardRequest
	responses []*pluginv1.ForwardResponse
	index     int
	mu        sync.Mutex
}

func (s *fakeForwardStream) Recv() (*pluginv1.ForwardRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.index == len(s.requests) {
		return nil, io.EOF
	}
	request := s.requests[s.index]
	s.index++
	return request, nil
}

func (s *fakeForwardStream) Send(response *pluginv1.ForwardResponse) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.responses = append(s.responses, response)
	return nil
}

func (s *fakeForwardStream) SetHeader(metadata.MD) error  { return nil }
func (s *fakeForwardStream) SendHeader(metadata.MD) error { return nil }
func (s *fakeForwardStream) SetTrailer(metadata.MD)       {}
func (s *fakeForwardStream) Context() context.Context     { return s.ctx }
func (s *fakeForwardStream) SendMsg(any) error            { return errors.New("unexpected SendMsg") }
func (s *fakeForwardStream) RecvMsg(any) error            { return errors.New("unexpected RecvMsg") }
