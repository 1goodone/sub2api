package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/service/basispoints"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/sjson"
)

const excelBPSImagesEndpoint = "/basispoints/api/images/generations"

// excelBPSImagesUnsupportedReason reports why a request must stay on Codex.
// BPS answers 422 for any field outside model, prompt, size, quality, n,
// background and output_format, and only png output was accepted in testing.
func excelBPSImagesUnsupportedReason(parsed *OpenAIImagesRequest) string {
	switch {
	case parsed == nil:
		return "missing request"
	case parsed.IsEdits():
		return "image edits"
	case parsed.Stream:
		return "stream"
	case parsed.PartialImages != nil:
		return "partial_images"
	case parsed.OutputCompression != nil:
		return "output_compression"
	case strings.TrimSpace(parsed.Style) != "":
		return "style"
	case strings.TrimSpace(parsed.InputFidelity) != "":
		return "input_fidelity"
	}
	switch strings.ToLower(strings.TrimSpace(parsed.OutputFormat)) {
	case "", "png":
	default:
		return "output_format"
	}
	switch strings.ToLower(strings.TrimSpace(parsed.Moderation)) {
	case "", "auto":
	default:
		return "moderation"
	}
	return ""
}

// buildExcelBPSImagesPayload reuses the Codex generations body. moderation=auto
// is the upstream default, and BPS rejects the field itself.
func buildExcelBPSImagesPayload(parsed *OpenAIImagesRequest, model string) ([]byte, error) {
	body, _, err := buildOpenAIImagesOAuthPayload(parsed, model)
	if err != nil {
		return nil, err
	}
	return sjson.DeleteBytes(body, "moderation")
}

// The images endpoint checks the full Excel client identity, not only the
// product and agent profile that the Responses endpoint needs.
func newExcelBPSImagesRequest(ctx context.Context, body []byte, token, accountID string) (*http.Request, error) {
	req, err := newExcelBPSRequestTo(ctx, basispoints.ImagesGenerationsURL, "application/json", body, token, accountID)
	if err != nil {
		return nil, err
	}
	for key, value := range map[string]string{
		"X-Openai-Internal-Basispoints-Client-Platform":       "excel",
		"X-Openai-Internal-Basispoints-Client-Editor":         "excel",
		"X-Openai-Internal-Basispoints-Client-Host":           "office",
		"X-Openai-Internal-Basispoints-Client-Runtime":        "desktop",
		"X-Openai-Internal-Basispoints-Client-Platform-Class": "PC",
		"X-Openai-Internal-Basispoints-Office-Host":           "Excel",
		"X-Openai-Internal-Basispoints-Office-Platform":       "PC",
	} {
		req.Header.Set(key, value)
	}
	return req, nil
}

// excelBPSImagesFallbackStatus lists rejections of the request format. Nothing
// was generated or written yet, so the same request can still go to Codex.
func excelBPSImagesFallbackStatus(status int) bool {
	switch status {
	case http.StatusBadRequest, http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusUnprocessableEntity:
		return true
	}
	return false
}

// forwardExcelBPSImages sends a text-to-image request to BPS. fallback=true
// means BPS rejected the format before generating anything; the caller then
// uses Codex. BPS failures never write Codex account state, and a request that
// may have reached BPS is never replayed.
func (s *OpenAIGatewayService) forwardExcelBPSImages(ctx context.Context, c *gin.Context, account *Account, parsed *OpenAIImagesRequest, requestModel, upstreamModel string, startTime time.Time) (*OpenAIForwardResult, bool, error) {
	fail := func(status int, code, message string) (*OpenAIForwardResult, bool, error) {
		SetActualOpenAIUpstreamEndpoint(c, excelBPSImagesEndpoint)
		errorType := "invalid_request_error"
		if status >= 500 {
			errorType = "server_error"
		}
		upErr := &OpenAIImagesUpstreamError{StatusCode: status, ErrorType: errorType, Code: code, Message: message}
		writeOpenAIImagesUpstreamErrorResponse(c, upErr)
		return nil, false, upErr
	}
	body, err := buildExcelBPSImagesPayload(parsed, upstreamModel)
	if err != nil {
		return nil, false, err
	}
	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, false, err
	}
	accountID := excelBPSAccountID(account, token)
	if accountID == "" {
		return fail(http.StatusBadRequest, "basispoints_account_id_missing", "Excel BPS requires a ChatGPT account ID")
	}
	scope := fmt.Sprintf("transient:account:%d/key:%d/images", account.ID, getAPIKeyIDFromContext(c))
	requestCtx := WithHTTPUpstreamRedirectsDisabled(WithHTTPUpstreamProfile(ctx, HTTPUpstreamProfileExcelBPS))
	build := func(ctx context.Context) (*http.Request, error) {
		return newExcelBPSImagesRequest(ctx, body, token, accountID)
	}
	sent := time.Now()
	resp, lease, _, err := s.doExcelBPSRequestTo(requestCtx, c, account, scope, basispoints.ImagesGenerationsURL, build, s.excelBPSAcquireFor(account))
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(sent).Milliseconds())
	if err != nil {
		if isExcelBPSClientCancellation(c, err) {
			MarkOpsClientCancellation(c, false)
			return nil, false, context.Canceled
		}
		if errors.Is(err, errExcelBPSProxyUnavailable) {
			return fail(http.StatusServiceUnavailable, "basispoints_proxy_unavailable", "No healthy BPS session proxy is available; retry later")
		}
		return fail(http.StatusBadGateway, "basispoints_transport_error", "Excel BPS connection failed; request was not replayed after sending")
	}
	if lease != nil {
		defer lease.Release()
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if lease != nil && resp.StatusCode >= 500 && ctx.Err() == nil {
			lease.ReportUpstreamFailure()
		}
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 512<<10))
		upstreamMessage, upstreamDetail := s.excelBPSUpstreamErrorDetails(resp.StatusCode, raw, token, account)
		event := OpsUpstreamErrorEvent{
			Platform: account.Platform, AccountID: account.ID, AccountName: account.Name,
			ProxyID: opsUpstreamProxyID(account), ProxyName: opsUpstreamProxyName(account),
			UpstreamStatusCode: resp.StatusCode, UpstreamRequestID: resp.Header.Get("x-request-id"),
			UpstreamURL: basispoints.ImagesGenerationsURL, Kind: "http_error",
			Message: upstreamMessage, Detail: upstreamDetail, UpstreamResponseBody: upstreamDetail,
		}
		if excelBPSImagesFallbackStatus(resp.StatusCode) {
			event.Kind = "retry"
			appendOpsUpstreamError(c, event)
			logger.LegacyPrintf("service.openai_excel_bps", "images rejected by BPS, using Codex: account_id=%d status=%d", account.ID, resp.StatusCode)
			return nil, true, nil
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			// Like text BPS: cool only the BPS route and let the handler try
			// another account; Codex quota state is left alone.
			event.Kind = "failover"
			appendOpsUpstreamError(c, event)
			s.coolDownExcelBPS(ctx, account, resp.Header, raw)
			// The next account may use Codex, so the endpoint is recorded only
			// for final BPS outcomes; the image path never clears it.
			failover := newOpenAIUpstreamFailoverError(
				http.StatusTooManyRequests,
				resp.Header,
				[]byte(`{"error":{"type":"rate_limit_error","code":"basispoints_rate_limited","message":"Excel BPS rate limit exceeded"}}`),
				"Excel BPS rate limit exceeded",
				false,
			)
			failover.SuppressScheduleFailure = true
			failover.SkipOAuth429AttemptLimit = true
			failover.ClientMessage = "Excel BPS rate limit exceeded"
			return nil, false, failover
		}
		setOpsUpstreamError(c, resp.StatusCode, upstreamMessage, upstreamDetail)
		appendOpsUpstreamError(c, event)
		switch resp.StatusCode {
		case http.StatusUnauthorized:
			s.handleExcelBPSUnauthorized(ctx, account, resp.StatusCode, resp.Header, raw)
			return fail(resp.StatusCode, "basispoints_upstream_error", "Excel BPS authentication failed; request was not replayed")
		case http.StatusForbidden:
			message := "Excel BPS rejected this request; request was not replayed"
			moved := s.moveExcelBPSOn403(ctx, account)
			disabled := s.disableExcelBPSOn403(ctx, account)
			switch {
			case moved && disabled:
				message = "Excel BPS rejected this request; Excel BPS was automatically disabled and account groups were updated; request was not replayed"
			case disabled:
				message = "Excel BPS rejected this request; Excel BPS was automatically disabled for this account; request was not replayed"
			case moved:
				message = "Excel BPS rejected this request; account groups were automatically updated; request was not replayed"
			}
			return fail(resp.StatusCode, "basispoints_upstream_error", message)
		}
		return fail(resp.StatusCode, "basispoints_upstream_error", "Excel BPS rejected this request; account scheduling was not changed")
	}

	SetActualOpenAIUpstreamEndpoint(c, excelBPSImagesEndpoint)
	usage, imageCount, imageOutputSizes, err := s.handleCodexDirectImagesNonStreamingResponse(resp, c, parsed)
	if err != nil {
		if _, _, ok := OpenAIUpstreamStreamReadErrorDetails(err); ok {
			if lease != nil && ctx.Err() == nil {
				lease.ReportStreamFailure()
			}
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform: account.Platform, AccountID: account.ID, AccountName: account.Name,
				ProxyID: opsUpstreamProxyID(account), ProxyName: opsUpstreamProxyName(account),
				UpstreamStatusCode: resp.StatusCode, UpstreamRequestID: resp.Header.Get("x-request-id"),
				UpstreamURL: basispoints.ImagesGenerationsURL, Kind: "stream_error",
				Message: "Excel BPS image response was interrupted",
			})
			return fail(http.StatusBadGateway, "basispoints_transport_error", "Excel BPS image response was interrupted; request was not replayed")
		}
		var upErr *OpenAIImagesUpstreamError
		if errors.As(err, &upErr) {
			writeOpenAIImagesUpstreamErrorResponse(c, upErr)
		}
		return nil, false, err
	}
	if lease != nil {
		lease.ReportSuccess()
	}
	if imageCount <= 0 {
		imageCount = parsed.N
	}
	return &OpenAIForwardResult{
		RequestID:                     resp.Header.Get("x-request-id"),
		UpstreamHeaders:               resp.Header,
		Usage:                         usage,
		Model:                         requestModel,
		UpstreamModel:                 upstreamModel,
		UpstreamEndpoint:              excelBPSImagesEndpoint,
		UpstreamResponseModel:         observedUpstreamResponseModel(c),
		UpstreamResponseModelConflict: observedUpstreamResponseModelConflict(c),
		Stream:                        false,
		ResponseHeaders:               resp.Header.Clone(),
		Duration:                      time.Since(startTime),
		ImageCount:                    imageCount,
		ImageSize:                     parsed.SizeTier,
		ImageInputSize:                parsed.Size,
		ImageOutputSizes:              imageOutputSizes,
	}, false, nil
}

// excelBPSUpstreamErrorDetails keeps the raw BPS rejection for Ops only when
// body logging is enabled, redacted the same way as the text route.
func (s *OpenAIGatewayService) excelBPSUpstreamErrorDetails(status int, raw []byte, token string, account *Account) (string, string) {
	message := fmt.Sprintf("Excel BPS returned HTTP %d", status)
	detail := ""
	if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
		safeBody := excelBPSSanitizeErrorBody(string(raw), token, account)
		maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
		if maxBytes <= 0 {
			maxBytes = 2048
		}
		detail, _ = sanitizeErrorBodyForStorage(safeBody, maxBytes)
		if upstream := strings.TrimSpace(extractUpstreamErrorMessage([]byte(safeBody))); upstream != "" {
			message = truncateString(upstream, 2048)
		}
	}
	return message, detail
}
