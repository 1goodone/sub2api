package main

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/cmd/codex-desktop-transport-plugin/portrait"
	pluginv1 "github.com/Wei-Shaw/sub2api/pkg/pluginapi/v1"
	"google.golang.org/grpc"
)

const (
	pluginID      = "even.sub2api.codex-desktop-transport"
	pluginVersion = "0.1.0"
	capabilityID  = "openai.oauth.outbound_transport.v1"
)

type pluginConfig struct {
	Enabled            bool   `json:"enabled"`
	ConnectionReuse    string `json:"connection_reuse,omitempty"`
	TLSObservedEnabled bool   `json:"tls_observed_enabled,omitempty"`
	TLSObservedPath    string `json:"tls_observed_path,omitempty"`
	PinProfile         string `json:"pin_profile,omitempty"`
}

type transportService struct {
	pluginv1.UnimplementedTransportPluginServer
	enabled         atomic.Bool
	observedPath    atomic.Pointer[string]
	observedOn      atomic.Bool
	pinProfile      atomic.Pointer[string]
	newRoundTripper func(string, *portrait.Portrait) (http.RoundTripper, error)
}

func newTransportService() *transportService {
	return &transportService{newRoundTripper: func(proxyURL string, p *portrait.Portrait) (http.RoundTripper, error) {
		return newRustlsRoundTripper(proxyURL, p)
	}}
}

func (s *transportService) GetInfo(context.Context, *pluginv1.GetInfoRequest) (*pluginv1.GetInfoResponse, error) {
	return &pluginv1.GetInfoResponse{
		PluginId:            pluginID,
		PluginVersion:       pluginVersion,
		ProtocolVersion:     pluginv1.ProtocolVersion,
		TransportApiVersion: pluginv1.TransportAPIVersion,
		Capabilities:        []string{capabilityID},
	}, nil
}

func (s *transportService) Health(context.Context, *pluginv1.HealthRequest) (*pluginv1.HealthResponse, error) {
	return &pluginv1.HealthResponse{Healthy: true, Message: "ready"}, nil
}

func (s *transportService) ValidateConfig(_ context.Context, request *pluginv1.ValidateConfigRequest) (*pluginv1.ValidateConfigResponse, error) {
	config, normalized, err := parseConfig(request.GetConfigJson())
	if err != nil {
		return &pluginv1.ValidateConfigResponse{Valid: false, Message: err.Error()}, nil
	}
	_ = config
	return &pluginv1.ValidateConfigResponse{Valid: true, Message: "valid", NormalizedConfigJson: normalized}, nil
}

func (s *transportService) ApplyConfig(_ context.Context, request *pluginv1.ApplyConfigRequest) (*pluginv1.ApplyConfigResponse, error) {
	config, _, err := parseConfig(request.GetConfigJson())
	if err != nil {
		return &pluginv1.ApplyConfigResponse{Applied: false, Message: err.Error()}, nil
	}
	s.enabled.Store(config.Enabled)
	s.observedOn.Store(config.TLSObservedEnabled)
	path := config.TLSObservedPath
	s.observedPath.Store(&path)
	pin := config.PinProfile
	s.pinProfile.Store(&pin)
	return &pluginv1.ApplyConfigResponse{Applied: true, Message: "applied"}, nil
}

func (s *transportService) TestConfig(_ context.Context, request *pluginv1.TestConfigRequest) (*pluginv1.TestConfigResponse, error) {
	config, _, err := parseConfig(request.GetConfigJson())
	if err != nil {
		return &pluginv1.TestConfigResponse{Success: false, Message: err.Error()}, nil
	}
	if !config.Enabled {
		return &pluginv1.TestConfigResponse{Success: false, Message: "enabled=false; Enable is refused until transport is turned on"}, nil
	}
	started := time.Now()
	if err := localHandshake(config); err != nil {
		return &pluginv1.TestConfigResponse{Success: false, Message: err.Error()}, nil
	}
	return &pluginv1.TestConfigResponse{Success: true, Message: "rustls HelloCustom handshake ok", LatencyMs: time.Since(started).Milliseconds()}, nil
}

func parseConfig(raw []byte) (pluginConfig, []byte, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		raw = []byte(`{}`)
	} else if trimmed[0] != '{' {
		return pluginConfig{}, nil, errors.New("configuration must be a JSON object")
	}
	var config pluginConfig
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return pluginConfig{}, nil, errors.New("configuration must be an object with enabled, optional connection_reuse, tls_observed_enabled, tls_observed_path")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return pluginConfig{}, nil, errors.New("configuration must contain one JSON object")
	}
	reuse := strings.TrimSpace(config.ConnectionReuse)
	if reuse == "" {
		reuse = "per_request"
	}
	switch reuse {
	case "per_request", "capture":
		config.ConnectionReuse = reuse
	case "pool":
		p, _ := portrait.EmbeddedP2()
		if !portrait.CaptureAllowsPool(p) {
			return pluginConfig{}, nil, errors.New("connection_reuse=pool is not allowed until captured reuse=pool")
		}
	default:
		return pluginConfig{}, nil, fmt.Errorf("unsupported connection_reuse %q", reuse)
	}
	normalized, err := json.Marshal(config)
	if err != nil {
		return pluginConfig{}, nil, errors.New("failed to normalize configuration")
	}
	return config, normalized, nil
}

func localHandshake(config pluginConfig) error {
	p, err := portrait.EmbeddedP2()
	if err != nil {
		return err
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	tripper, err := newRustlsRoundTripper("", p)
	if err != nil {
		return err
	}
	tripper.rootCAs = x509PoolFromTestServer(server)
	req, err := http.NewRequest(http.MethodGet, server.URL, nil)
	if err != nil {
		return err
	}
	resp, err := tripper.RoundTrip(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	if config.TLSObservedEnabled {
		path := strings.TrimSpace(config.TLSObservedPath)
		if path == "" {
			path = "tls-observed-test.jsonl"
		}
		event := tlsObservedEvent{ProfileID: p.ID, JA3: p.TLS.JA3Hash, JA4: p.TLS.JA4, ALPN: resp.Proto, RequestID: "testconfig", AccountID: 0}
		raw, _ := json.Marshal(event)
		if !redactedObservedOK(raw) {
			return errors.New("tls observed payload failed redaction")
		}
		if err := appendTLSObserved(path, event); err != nil {
			return err
		}
	}
	return nil
}

func (s *transportService) Forward(stream grpc.BidiStreamingServer[pluginv1.ForwardRequest, pluginv1.ForwardResponse]) error {
	startedAt := time.Now()
	first, err := stream.Recv()
	if err != nil {
		return err
	}
	start := first.GetStart()
	if start == nil {
		return sendTransportError(stream, "INVALID_REQUEST", "first frame must be start", false)
	}
	if !s.enabled.Load() {
		return sendTransportError(stream, "TRANSPORT_DISABLED", "codex-desktop transport is disabled", false)
	}

	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()
	request, bodyReader, bodyWriter, err := requestFromStart(ctx, start)
	if err != nil {
		return sendTransportError(stream, "INVALID_REQUEST", err.Error(), false)
	}
	if bodyReader != nil {
		defer func() { _ = bodyReader.Close() }()
	}
	if bodyWriter != nil {
		defer func() { _ = bodyWriter.Close() }()
	}

	pin := ""
	if v := s.pinProfile.Load(); v != nil {
		pin = *v
	}
	p, _, err := portrait.PortraitForAccount(start.GetAccountId(), pin)
	if err != nil {
		return sendTransportError(stream, "MISSING_PORTRAIT", err.Error(), false)
	}
	roundTripper, err := s.newRoundTripper(start.GetProxyUrl(), p)
	if err != nil {
		return sendTransportError(stream, "INVALID_PROXY", err.Error(), false)
	}
	if rt, ok := roundTripper.(*rustlsRoundTripper); ok && s.observedOn.Load() {
		pathPtr := s.observedPath.Load()
		path := ""
		if pathPtr != nil {
			path = *pathPtr
		}
		rt.observe = func(alpn string) {
			_ = appendTLSObserved(path, tlsObservedEvent{
				ProfileID: p.ID,
				JA3:       p.TLS.JA3Hash,
				JA4:       p.TLS.JA4,
				ALPN:      alpn,
				RequestID: start.GetRequestId(),
				AccountID: start.GetAccountId(),
			})
		}
	}

	if bodyWriter == nil {
		if err := receiveRequestBody(stream, nil, start.GetContentLength()); err != nil {
			return sendTransportError(stream, "INVALID_REQUEST_BODY", err.Error(), false)
		}
	} else {
		go func() {
			err := receiveRequestBody(stream, bodyWriter, start.GetContentLength())
			if err != nil {
				_ = bodyWriter.CloseWithError(err)
				return
			}
			_ = bodyWriter.Close()
		}()
	}

	response, err := roundTripper.RoundTrip(request)
	if err != nil {
		return sendTransportError(stream, "UPSTREAM_TRANSPORT_ERROR", publicTransportError(ctx, err), true)
	}
	if response == nil {
		return sendTransportError(stream, "UPSTREAM_TRANSPORT_ERROR", "upstream returned no response", true)
	}
	if response.Body == nil {
		response.Body = http.NoBody
	}
	defer func() { _ = response.Body.Close() }()

	status := response.Status
	if status == "" {
		status = fmt.Sprintf("%d %s", response.StatusCode, http.StatusText(response.StatusCode))
	}
	if err := stream.Send(&pluginv1.ForwardResponse{Frame: &pluginv1.ForwardResponse_Start{Start: &pluginv1.ForwardResponseStart{
		StatusCode:    int32(response.StatusCode),
		Status:        status,
		Protocol:      response.Proto,
		ProtocolMajor: int32(response.ProtoMajor),
		ProtocolMinor: int32(response.ProtoMinor),
		Headers:       headersToPlugin(response.Header),
		ContentLength: response.ContentLength,
	}}}); err != nil {
		return err
	}

	var bytesReceived int64
	buffer := make([]byte, 32*1024)
	for {
		read, readErr := response.Body.Read(buffer)
		if read > 0 {
			bytesReceived += int64(read)
			chunk := append([]byte(nil), buffer[:read]...)
			if err := stream.Send(&pluginv1.ForwardResponse{Frame: &pluginv1.ForwardResponse_BodyChunk{BodyChunk: chunk}}); err != nil {
				return err
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return sendTransportError(stream, "UPSTREAM_RESPONSE_ERROR", "failed to read upstream response", true)
		}
	}

	return stream.Send(&pluginv1.ForwardResponse{Frame: &pluginv1.ForwardResponse_End{End: &pluginv1.ForwardResponseEnd{
		BytesReceived: bytesReceived,
		DurationMs:    time.Since(startedAt).Milliseconds(),
	}}})
}

func requestFromStart(ctx context.Context, start *pluginv1.ForwardRequestStart) (*http.Request, *io.PipeReader, *io.PipeWriter, error) {
	if start.GetRequestId() == "" || start.GetMethod() == "" {
		return nil, nil, nil, errors.New("request ID and method are required")
	}
	if start.GetPlatform() != "openai" || start.GetAccountType() != "oauth" {
		return nil, nil, nil, errors.New("only OpenAI OAuth requests are supported")
	}
	parsedURL, err := url.Parse(start.GetUrl())
	if err != nil || parsedURL.Scheme != "https" || parsedURL.Host == "" || parsedURL.User != nil {
		return nil, nil, nil, errors.New("request URL must be an absolute HTTPS URL without user info")
	}
	if start.GetContentLength() < -1 {
		return nil, nil, nil, errors.New("content length is invalid")
	}
	if !start.GetHasBody() && start.GetContentLength() > 0 {
		return nil, nil, nil, errors.New("body metadata is inconsistent")
	}

	var body io.Reader
	var bodyReader *io.PipeReader
	var bodyWriter *io.PipeWriter
	if start.GetHasBody() {
		bodyReader, bodyWriter = io.Pipe()
		body = bodyReader
	}
	request, err := http.NewRequestWithContext(ctx, start.GetMethod(), parsedURL.String(), body)
	if err != nil {
		if bodyReader != nil {
			_ = bodyReader.Close()
			_ = bodyWriter.Close()
		}
		return nil, nil, nil, errors.New("request metadata is invalid")
	}
	request.Header = headersFromPlugin(start.GetHeaders())
	request.Host = start.GetHost()
	request.ContentLength = start.GetContentLength()
	if !start.GetHasBody() {
		request.Body = http.NoBody
		request.ContentLength = 0
	}
	return request, bodyReader, bodyWriter, nil
}

func receiveRequestBody(stream grpc.BidiStreamingServer[pluginv1.ForwardRequest, pluginv1.ForwardResponse], writer io.Writer, expected int64) error {
	var received int64
	for {
		frame, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				return errors.New("request ended before body_end")
			}
			return err
		}
		switch value := frame.Frame.(type) {
		case *pluginv1.ForwardRequest_BodyChunk:
			if writer == nil && len(value.BodyChunk) > 0 {
				return errors.New("body chunk received for a bodyless request")
			}
			received += int64(len(value.BodyChunk))
			if expected >= 0 && received > expected {
				return errors.New("request body exceeds declared content length")
			}
			if writer != nil && len(value.BodyChunk) > 0 {
				if _, err := writer.Write(value.BodyChunk); err != nil {
					return err
				}
			}
		case *pluginv1.ForwardRequest_BodyEnd:
			if !value.BodyEnd {
				return errors.New("body_end must be true")
			}
			if expected >= 0 && received != expected {
				return errors.New("request body length does not match content length")
			}
			return nil
		default:
			return errors.New("unexpected request frame")
		}
	}
}

func sendTransportError(stream grpc.BidiStreamingServer[pluginv1.ForwardRequest, pluginv1.ForwardResponse], code, message string, requestSent bool) error {
	message = strings.TrimSpace(message)
	if len(message) > 500 {
		message = message[:500]
	}
	return stream.Send(&pluginv1.ForwardResponse{Frame: &pluginv1.ForwardResponse_Error{Error: &pluginv1.ForwardResponseError{
		Code: code, Message: message, RequestSent: requestSent,
	}}})
}

func x509PoolFromTestServer(server *httptest.Server) *x509.CertPool {
	pool := x509.NewCertPool()
	if server != nil && server.Certificate() != nil {
		pool.AddCert(server.Certificate())
	}
	return pool
}

func publicTransportError(ctx context.Context, err error) string {
	if ctx.Err() != nil {
		return "request canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "upstream transport timed out"
	}
	return "codex-desktop upstream transport failed"
}

func headersFromPlugin(headers map[string]*pluginv1.HeaderValues) http.Header {
	out := make(http.Header, len(headers))
	for name, values := range headers {
		if values != nil {
			out[name] = append([]string(nil), values.Values...)
		}
	}
	return out
}

func headersToPlugin(headers http.Header) map[string]*pluginv1.HeaderValues {
	out := make(map[string]*pluginv1.HeaderValues, len(headers))
	for name, values := range headers {
		out[name] = &pluginv1.HeaderValues{Values: append([]string(nil), values...)}
	}
	return out
}
