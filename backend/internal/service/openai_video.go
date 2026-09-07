package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	openAIVideosEndpoint            = "/v1/videos"
	openAIVideoOmitDurationExtraKey = "openai_video_omit_duration"
)

func buildOpenAIVideoURL(base string, endpoint GrokMediaEndpoint, requestID string) string {
	videosURL := buildOpenAIEndpointURL(base, openAIVideosEndpoint)
	switch endpoint {
	case GrokMediaEndpointVideosGenerations:
		return videosURL
	case GrokMediaEndpointVideoStatus, GrokMediaEndpointVideoContent:
		requestID = strings.Trim(requestID, "/")
		if requestID == "" {
			return ""
		}
		target := strings.TrimRight(videosURL, "/") + "/" + url.PathEscape(requestID)
		if endpoint == GrokMediaEndpointVideoContent {
			target += "/content"
		}
		return target
	default:
		return ""
	}
}

// ForwardOpenAIVideo relays the canonical OpenAI-compatible async video API.
// It intentionally supports only API-key accounts; OpenAI OAuth/Codex accounts
// do not expose /v1/videos.
func (s *OpenAIGatewayService) ForwardOpenAIVideo(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	endpoint GrokMediaEndpoint,
	requestID string,
	body []byte,
	contentType string,
	channelMappedModel string,
) (*OpenAIForwardResult, error) {
	startTime := time.Now()
	if account == nil || !account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityVideos) {
		return nil, fmt.Errorf("openai video requires an OpenAI API-key account")
	}
	if endpoint != GrokMediaEndpointVideosGenerations && endpoint != GrokMediaEndpointVideoStatus && endpoint != GrokMediaEndpointVideoContent {
		return nil, fmt.Errorf("unsupported openai video endpoint: %s", endpoint)
	}

	baseURL, err := s.validateUpstreamBaseURL(account.GetOpenAIBaseURL())
	if err != nil {
		return nil, err
	}
	targetURL := buildOpenAIVideoURL(baseURL, endpoint, requestID)
	if targetURL == "" {
		return nil, fmt.Errorf("openai video request_id is required")
	}

	requestInfo := ParseGrokMediaRequest(contentType, body)
	requestModel := strings.TrimSpace(channelMappedModel)
	if requestModel == "" {
		requestModel = requestInfo.Model
	}
	upstreamModel := requestModel
	if endpoint == GrokMediaEndpointVideosGenerations {
		upstreamModel = strings.TrimSpace(account.GetMappedModel(requestModel))
		if upstreamModel == "" {
			upstreamModel = requestModel
		}
		body, contentType, err = rewriteOpenAIImagesModel(body, contentType, upstreamModel)
		if err != nil {
			return nil, fmt.Errorf("rewrite openai video model: %w", err)
		}
		if account.getExtraBool(openAIVideoOmitDurationExtraKey) && gjson.ValidBytes(body) {
			body, err = sjson.DeleteBytes(body, "duration")
			if err == nil {
				body, err = sjson.DeleteBytes(body, "seconds")
			}
			if err != nil {
				return nil, fmt.Errorf("strip openai video duration: %w", err)
			}
		}
		SetOpsUpstreamModel(c, upstreamModel)
	}

	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	defer releaseUpstreamCtx()
	token, _, err := s.GetAccessToken(upstreamCtx, account)
	if err != nil {
		return nil, err
	}
	upstreamReq, err := s.buildOpenAIVideoRequest(upstreamCtx, c, account, endpoint, targetURL, body, contentType, token)
	if err != nil {
		return nil, err
	}

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	upstreamStart := time.Now()
	resp, err := s.doOpenAIUpstream(upstreamReq, proxyURL, account)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
	if err != nil {
		return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, false)
	}
	defer func() { _ = resp.Body.Close() }()

	requestIDHeader := firstNonEmpty(resp.Header.Get("x-request-id"), resp.Header.Get("request-id"), resp.Header.Get("xai-request-id"))
	if endpoint == GrokMediaEndpointVideoContent {
		if resp.StatusCode >= 400 && resp.StatusCode != http.StatusRequestedRangeNotSatisfiable {
			return s.handleOpenAIVideoErrorResponse(upstreamCtx, c, account, resp, targetURL, requestIDHeader, upstreamModel)
		}
		if err := writeGrokMediaContentResponse(c, resp); err != nil {
			return nil, err
		}
		result := &OpenAIForwardResult{
			RequestID:       requestIDHeader,
			ResponseID:      strings.TrimSpace(requestID),
			ResponseHeaders: resp.Header.Clone(),
			Duration:        time.Since(startTime),
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			result.VideoCount = 1
		}
		return result, nil
	}
	if resp.StatusCode >= 400 {
		return s.handleOpenAIVideoErrorResponse(upstreamCtx, c, account, resp, targetURL, requestIDHeader, upstreamModel)
	}

	respBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, err
	}
	if endpoint == GrokMediaEndpointVideoStatus {
		respBody = rewriteOpenAIVideoContentURLs(respBody, requestID, grokMediaContentProxyURL(c, requestID))
	}
	writeGrokMediaResponse(c, resp, respBody, s.responseHeaderFilter)
	usage := openAIVideoUsageFromResponse(endpoint, requestInfo, respBody, requestID)
	resultModel := requestModel
	billingModel := requestModel
	if endpoint == GrokMediaEndpointVideoStatus {
		resultModel = usage.Model
		billingModel = usage.BillingModel
	}
	return &OpenAIForwardResult{
		RequestID:            requestIDHeader,
		ResponseID:           usage.ResponseID,
		Usage:                usage.Usage,
		Model:                resultModel,
		BillingModel:         billingModel,
		UpstreamModel:        upstreamModel,
		ResponseHeaders:      resp.Header.Clone(),
		Duration:             time.Since(startTime),
		VideoCount:           usage.VideoCount,
		VideoResolution:      usage.VideoResolution,
		VideoDurationSeconds: usage.VideoDurationSeconds,
	}, nil
}

func (s *OpenAIGatewayService) buildOpenAIVideoRequest(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	endpoint GrokMediaEndpoint,
	targetURL string,
	body []byte,
	contentType string,
	token string,
) (*http.Request, error) {
	var bodyReader io.Reader
	if endpoint.RequiresRequestBody() {
		bodyReader = bytes.NewReader(body)
	}
	requestCtx := WithHTTPUpstreamProfile(ctx, HTTPUpstreamProfileOpenAI)
	if endpoint.IsVideoLookupRequest() {
		requestCtx = WithHTTPUpstreamRedirectsDisabled(requestCtx)
	}
	req, err := http.NewRequestWithContext(requestCtx, endpoint.httpMethod(), targetURL, bodyReader)
	if err != nil {
		return nil, err
	}
	authHeaders, err := s.buildOpenAIAuthenticationHeaders(ctx, account, token)
	if err != nil {
		return nil, fmt.Errorf("build openai video authentication headers: %w", err)
	}
	for key, values := range authHeaders {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	if endpoint.RequiresRequestBody() {
		if c != nil && c.Request != nil {
			for key, values := range c.Request.Header {
				if !openaiPassthroughAllowedHeaders[strings.ToLower(key)] {
					continue
				}
				for _, value := range values {
					req.Header.Add(key, value)
				}
			}
		}
		if strings.TrimSpace(contentType) == "" {
			contentType = "application/json"
		}
		req.Header.Set("Content-Type", contentType)
		req.Header.Set("Accept", "application/json")
	} else if endpoint == GrokMediaEndpointVideoContent {
		req.Header.Set("Accept", "*/*")
		if c != nil {
			if rangeHeader := strings.TrimSpace(c.GetHeader("Range")); rangeHeader != "" {
				req.Header.Set("Range", rangeHeader)
			}
		}
	} else {
		req.Header.Set("Accept", "application/json")
	}
	req.Header.Del("User-Agent")
	if customUA := account.GetOpenAIUserAgent(); customUA != "" {
		req.Header.Set("User-Agent", customUA)
	}
	account.ApplyHeaderOverrides(req.Header)
	return req, nil
}

func (s *OpenAIGatewayService) handleOpenAIVideoErrorResponse(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	resp *http.Response,
	targetURL string,
	requestIDHeader string,
	upstreamModel string,
) (*OpenAIForwardResult, error) {
	body := s.readUpstreamErrorBody(resp)
	upstreamMsg := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(body)))
	if upstreamMsg == "" {
		upstreamMsg = fmt.Sprintf("OpenAI-compatible video upstream returned status %d", resp.StatusCode)
	}
	upstreamDetail := ""
	if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
		maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
		if maxBytes <= 0 {
			maxBytes = 2048
		}
		upstreamDetail = truncateString(string(body), maxBytes)
	}
	setOpsUpstreamError(c, resp.StatusCode, upstreamMsg, upstreamDetail)
	if s.shouldFailoverOpenAIUpstreamResponse(account, resp.StatusCode, upstreamMsg, body) {
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: resp.StatusCode,
			UpstreamRequestID:  requestIDHeader,
			UpstreamURL:        safeUpstreamURL(targetURL),
			Kind:               "failover",
			Message:            upstreamMsg,
			Detail:             upstreamDetail,
		})
		shouldDisable := s.handleFailoverSideEffects(ctx, resp, account, body, upstreamModel)
		retryableOnSameAccount := !shouldDisable && account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode)
		if account.IsOpenAIOAuthLike() && resp.StatusCode == http.StatusTooManyRequests {
			return nil, s.newOpenAIAccountFailoverError(account, resp.StatusCode, resp.Header, body, upstreamMsg, shouldDisable, retryableOnSameAccount)
		}
		if isOpenAIHTTPUpstreamAccessStateError(resp.StatusCode, upstreamMsg, body) {
			return nil, newOpenAIUpstreamFailoverError(resp.StatusCode, resp.Header, body, upstreamMsg, retryableOnSameAccount)
		}
		return nil, &UpstreamFailoverError{
			StatusCode:             resp.StatusCode,
			ResponseBody:           body,
			ResponseHeaders:        resp.Header.Clone(),
			RetryableOnSameAccount: retryableOnSameAccount,
		}
	}

	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform:           account.Platform,
		AccountID:          account.ID,
		AccountName:        account.Name,
		UpstreamStatusCode: resp.StatusCode,
		UpstreamRequestID:  requestIDHeader,
		UpstreamURL:        safeUpstreamURL(targetURL),
		Kind:               "http_error",
		Message:            upstreamMsg,
		Detail:             upstreamDetail,
	})
	MarkResponseCommitted(c)
	writeGrokMediaResponse(c, resp, body, s.responseHeaderFilter)
	return nil, fmt.Errorf("openai video upstream error: %d %s", resp.StatusCode, upstreamMsg)
}

func openAIVideoUsageFromResponse(endpoint GrokMediaEndpoint, requestInfo GrokMediaRequestInfo, responseBody []byte, requestID string) grokMediaUsageMetadata {
	usage, _ := extractOpenAIUsageFromJSONBytes(responseBody)
	meta := grokMediaUsageMetadata{
		Usage:      usage,
		ResponseID: firstNonEmpty(extractGrokMediaVideoRequestID(responseBody), strings.TrimSpace(requestID)),
	}
	switch endpoint {
	case GrokMediaEndpointVideosGenerations:
		meta.Model = requestInfo.Model
		meta.BillingModel = requestInfo.Model
		meta.VideoResolution = requestInfo.Resolution
		meta.VideoDurationSeconds = requestInfo.DurationSeconds
	case GrokMediaEndpointVideoStatus:
		if !isOpenAIVideoStatusBillable(responseBody) {
			return meta
		}
		meta.Model = openAIVideoString(responseBody, "model", "data.model", "video.model", "data.video.model")
		meta.BillingModel = meta.Model
		meta.VideoCount = 1
		if resolution := openAIVideoString(responseBody, "resolution", "resolution_name", "video.resolution", "data.resolution", "data.resolution_name", "data.video.resolution"); resolution != "" {
			meta.VideoResolution = NormalizeVideoBillingResolutionOrDefault(resolution)
		}
		if duration := openAIVideoInt(responseBody, "duration", "seconds", "video.duration", "video.seconds", "data.duration", "data.seconds", "data.video.duration", "data.video.seconds"); duration > 0 {
			meta.VideoDurationSeconds = NormalizeVideoBillingDurationSecondsOrDefault(duration)
		}
	}
	return meta
}

func isOpenAIVideoStatusBillable(body []byte) bool {
	status := strings.ToLower(openAIVideoString(body, "status", "state", "task_status", "data.status", "data.state", "data.task_status"))
	switch status {
	case "done", "completed", "succeeded", "success", "finished":
	default:
		return false
	}
	return openAIVideoString(body,
		"video.url", "video_url", "url", "output_url", "download_url", "output.url",
		"data.video.url", "data.video_url", "data.url", "data.output_url", "data.download_url", "data.output.url",
	) != ""
}

func openAIVideoString(body []byte, paths ...string) string {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return ""
	}
	for _, path := range paths {
		if value := strings.TrimSpace(gjson.GetBytes(body, path).String()); value != "" {
			return value
		}
	}
	return ""
}

func openAIVideoInt(body []byte, paths ...string) int {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return 0
	}
	for _, path := range paths {
		value := gjson.GetBytes(body, path)
		if !value.Exists() {
			continue
		}
		if value.Type == gjson.Number {
			return int(value.Int())
		}
		if parsed, err := strconv.Atoi(strings.TrimSpace(value.String())); err == nil {
			return parsed
		}
	}
	return 0
}
