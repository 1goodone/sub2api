package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	pluginv1 "github.com/Wei-Shaw/sub2api/pkg/pluginapi/v1"
	"google.golang.org/grpc"
)

const (
	pluginID      = "even.sub2api.cpa-compatible-transport"
	pluginVersion = "0.1.2"
	capabilityID  = "openai.oauth.outbound_transport.v1"
)

type pluginConfig struct {
	Enabled bool `json:"enabled"`
}

type transportService struct {
	pluginv1.UnimplementedTransportPluginServer
	enabled         atomic.Bool
	newRoundTripper func(string) (http.RoundTripper, error)
}

func newTransportService() *transportService {
	return &transportService{newRoundTripper: func(proxyURL string) (http.RoundTripper, error) {
		return newChromeRoundTripper(proxyURL)
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
	return &pluginv1.ApplyConfigResponse{Applied: true, Message: "applied"}, nil
}

func (s *transportService) TestConfig(_ context.Context, request *pluginv1.TestConfigRequest) (*pluginv1.TestConfigResponse, error) {
	config, _, err := parseConfig(request.GetConfigJson())
	if err != nil {
		return &pluginv1.TestConfigResponse{Success: false, Message: err.Error()}, nil
	}
	message := "configuration is valid; transport is disabled"
	if config.Enabled {
		message = "configuration is valid; Chrome uTLS and HTTP/2 are enabled"
	}
	return &pluginv1.TestConfigResponse{Success: true, Message: message}, nil
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
		return pluginConfig{}, nil, errors.New("configuration must be an object containing only enabled")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return pluginConfig{}, nil, errors.New("configuration must contain one JSON object")
	}
	normalized, err := json.Marshal(config)
	if err != nil {
		return pluginConfig{}, nil, errors.New("failed to normalize configuration")
	}
	return config, normalized, nil
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
		return sendTransportError(stream, "TRANSPORT_DISABLED", "CPA-compatible transport is disabled", false)
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

	roundTripper, err := s.newRoundTripper(start.GetProxyUrl())
	if err != nil {
		return sendTransportError(stream, "INVALID_PROXY", err.Error(), false)
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

func publicTransportError(ctx context.Context, err error) string {
	if ctx.Err() != nil {
		return "request canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "upstream transport timed out"
	}
	return "CPA-compatible upstream transport failed"
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
