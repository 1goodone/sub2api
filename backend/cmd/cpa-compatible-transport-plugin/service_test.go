package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

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

func TestConfigIsStrictAndDefaultsDisabled(t *testing.T) {
	t.Parallel()

	config, normalized, err := parseConfig(nil)
	if err != nil || config.Enabled || string(normalized) != `{"enabled":false}` {
		t.Fatalf("default config = %#v, %s, %v", config, normalized, err)
	}
	for _, raw := range []string{`null`, `[]`, `{"enabled":true,"extra":1}`, `{"enabled":"yes"}`, `{} {}`} {
		if _, _, err := parseConfig([]byte(raw)); err == nil {
			t.Fatalf("parseConfig(%s) unexpectedly succeeded", raw)
		}
	}
}

func TestForwardStreamsRequestAndResponse(t *testing.T) {
	t.Parallel()

	service := newTransportService()
	service.enabled.Store(true)
	var gotProxy string
	service.newRoundTripper = func(proxyURL string) (http.RoundTripper, error) {
		gotProxy = proxyURL
		return roundTripFunc(func(request *http.Request) (*http.Response, error) {
			body, err := io.ReadAll(request.Body)
			if err != nil {
				return nil, err
			}
			if string(body) != "payload" || request.Header.Get("X-Test") != "one" || request.ContentLength != 7 {
				t.Fatalf("upstream request body=%q headers=%v length=%d", body, request.Header, request.ContentLength)
			}
			return &http.Response{
				StatusCode:    http.StatusCreated,
				Status:        "201 Created",
				Proto:         "HTTP/2.0",
				ProtoMajor:    2,
				Header:        http.Header{"X-Upstream": []string{"ok"}},
				Body:          io.NopCloser(bytes.NewBufferString("response")),
				ContentLength: 8,
			}, nil
		}), nil
	}
	stream := &fakeForwardStream{ctx: context.Background(), requests: requestFrames(true, 7,
		&pluginv1.ForwardRequest{Frame: &pluginv1.ForwardRequest_BodyChunk{BodyChunk: []byte("pay")}},
		&pluginv1.ForwardRequest{Frame: &pluginv1.ForwardRequest_BodyChunk{BodyChunk: []byte("load")}},
	)}

	if err := service.Forward(stream); err != nil {
		t.Fatal(err)
	}
	if gotProxy != "socks5h://proxy.example:1080" {
		t.Fatalf("proxy = %q", gotProxy)
	}
	if len(stream.responses) != 3 {
		t.Fatalf("response frame count = %d", len(stream.responses))
	}
	if start := stream.responses[0].GetStart(); start == nil || start.StatusCode != 201 || start.ProtocolMajor != 2 || start.Headers["X-Upstream"].Values[0] != "ok" {
		t.Fatalf("start frame = %#v", start)
	}
	if body := stream.responses[1].GetBodyChunk(); string(body) != "response" {
		t.Fatalf("response body = %q", body)
	}
	if end := stream.responses[2].GetEnd(); end == nil || end.BytesReceived != 8 {
		t.Fatalf("end frame = %#v", end)
	}
}

func TestForwardRequestSentSemantics(t *testing.T) {
	t.Parallel()

	t.Run("disabled is before transport", func(t *testing.T) {
		service := newTransportService()
		stream := &fakeForwardStream{ctx: context.Background(), requests: requestFrames(false, 0)}
		if err := service.Forward(stream); err != nil {
			t.Fatal(err)
		}
		assertForwardError(t, stream, false, "TRANSPORT_DISABLED")
	})

	t.Run("invalid URL is before transport", func(t *testing.T) {
		service := newTransportService()
		service.enabled.Store(true)
		frames := requestFrames(false, 0)
		frames[0].GetStart().Url = "http://chatgpt.com/"
		stream := &fakeForwardStream{ctx: context.Background(), requests: frames}
		if err := service.Forward(stream); err != nil {
			t.Fatal(err)
		}
		assertForwardError(t, stream, false, "INVALID_REQUEST")
	})

	t.Run("round trip failure may have sent", func(t *testing.T) {
		service := newTransportService()
		service.enabled.Store(true)
		service.newRoundTripper = func(string) (http.RoundTripper, error) {
			return roundTripFunc(func(*http.Request) (*http.Response, error) {
				return nil, errors.New("dial failed")
			}), nil
		}
		stream := &fakeForwardStream{ctx: context.Background(), requests: requestFrames(false, 0)}
		if err := service.Forward(stream); err != nil {
			t.Fatal(err)
		}
		assertForwardError(t, stream, true, "UPSTREAM_TRANSPORT_ERROR")
	})

	t.Run("body failure after round trip starts may have sent", func(t *testing.T) {
		service := newTransportService()
		service.enabled.Store(true)
		service.newRoundTripper = func(string) (http.RoundTripper, error) {
			return roundTripFunc(func(request *http.Request) (*http.Response, error) {
				_, err := io.ReadAll(request.Body)
				return nil, err
			}), nil
		}
		stream := &fakeForwardStream{ctx: context.Background(), requests: requestFrames(true, 7,
			&pluginv1.ForwardRequest{Frame: &pluginv1.ForwardRequest_BodyChunk{BodyChunk: []byte("short")}},
		)}
		if err := service.Forward(stream); err != nil {
			t.Fatal(err)
		}
		assertForwardError(t, stream, true, "UPSTREAM_TRANSPORT_ERROR")
	})
}

func TestForwardCancelsUpstream(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	service := newTransportService()
	service.enabled.Store(true)
	started := make(chan struct{})
	service.newRoundTripper = func(string) (http.RoundTripper, error) {
		return roundTripFunc(func(request *http.Request) (*http.Response, error) {
			close(started)
			<-request.Context().Done()
			return nil, request.Context().Err()
		}), nil
	}
	stream := &fakeForwardStream{ctx: ctx, requests: requestFrames(false, 0)}
	done := make(chan error, 1)
	go func() { done <- service.Forward(stream) }()
	<-started
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Forward did not stop after cancellation")
	}
	assertForwardError(t, stream, true, "UPSTREAM_TRANSPORT_ERROR")
}

func requestFrames(hasBody bool, contentLength int64, chunks ...*pluginv1.ForwardRequest) []*pluginv1.ForwardRequest {
	frames := []*pluginv1.ForwardRequest{{Frame: &pluginv1.ForwardRequest_Start{Start: &pluginv1.ForwardRequestStart{
		RequestId:     "request-1",
		Method:        http.MethodPost,
		Url:           "https://chatgpt.com/backend-api/codex/responses",
		Headers:       map[string]*pluginv1.HeaderValues{"X-Test": {Values: []string{"one"}}},
		ProxyUrl:      "socks5h://proxy.example:1080",
		Platform:      "openai",
		AccountType:   "oauth",
		ContentLength: contentLength,
		HasBody:       hasBody,
	}}}}
	frames = append(frames, chunks...)
	frames = append(frames, &pluginv1.ForwardRequest{Frame: &pluginv1.ForwardRequest_BodyEnd{BodyEnd: true}})
	return frames
}

func assertForwardError(t *testing.T, stream *fakeForwardStream, requestSent bool, code string) {
	t.Helper()
	if len(stream.responses) != 1 {
		t.Fatalf("response frame count = %d", len(stream.responses))
	}
	frame := stream.responses[0].GetError()
	if frame == nil || frame.RequestSent != requestSent || frame.Code != code {
		t.Fatalf("error frame = %#v", frame)
	}
}
