package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service/basispoints"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const codexDirectImagesURL = "https://chatgpt.com/backend-api/codex/images/generations"

func excelBPSImagesAccount(models ...any) *Account {
	account := excelAccount()
	if len(models) == 0 {
		models = []any{"gpt-image-2"}
	}
	account.Extra["openai_excel_bps_models"] = models
	return account
}

// excelBPSImagesUpstream answers BPS with status and Codex with a normal image.
func excelBPSImagesUpstream(status int, urls *[]string) *codexModelsHTTPUpstreamStub {
	return &codexModelsHTTPUpstreamStub{do: func(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		*urls = append(*urls, req.URL.String())
		if req.URL.Host == "bps.openai.com" && status != http.StatusOK {
			return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"detail":"PRIVATE_UPSTREAM"}`))}, nil
		}
		return openAIImagesJSONResponse(), nil
	}}
}

func TestExcelBPSImagesRequiresExplicitImageModel(t *testing.T) {
	legacy := excelAccount()
	require.True(t, legacy.IsExcelBPSEnabledForModel("gpt-image-2"))
	require.False(t, legacy.IsExcelBPSImagesEnabledForModel("gpt-image-2"), "legacy all-models routing keeps images on Codex")
	require.False(t, excelBPSImagesAccount("gpt-6-astra").IsExcelBPSImagesEnabledForModel("gpt-image-2"))
	require.True(t, excelBPSImagesAccount().IsExcelBPSImagesEnabledForModel("gpt-image-2"))
	require.False(t, excelBPSImagesAccount("gpt-image-1").IsExcelBPSImagesEnabledForModel("gpt-image-1"), "the Responses image tool has no BPS route")

	mapped := excelBPSImagesAccount()
	mapped.Credentials["model_mapping"] = map[string]any{"gpt-image-1": "gpt-image-2"}
	require.True(t, mapped.IsExcelBPSImagesEnabledForModel("gpt-image-1"), "the list matches the mapped model")

	disabled := excelBPSImagesAccount()
	disabled.Extra["openai_excel_bps"] = false
	require.False(t, disabled.IsExcelBPSImagesEnabledForModel("gpt-image-2"))
	apiKey := excelBPSImagesAccount()
	apiKey.Type = AccountTypeAPIKey
	require.False(t, apiKey.IsExcelBPSImagesEnabledForModel("gpt-image-2"))
}

func TestExcelBPSImagesUnsupportedReason(t *testing.T) {
	one := 1
	base := func() *OpenAIImagesRequest {
		return &OpenAIImagesRequest{Endpoint: openAIImagesGenerationsEndpoint, Model: "gpt-image-2", Prompt: "draw",
			N: 2, Size: "1024x1024", Quality: "high", Background: "opaque", OutputFormat: "PNG", Moderation: "auto", ResponseFormat: "url"}
	}
	require.Empty(t, excelBPSImagesUnsupportedReason(base()))
	for want, mutate := range map[string]func(*OpenAIImagesRequest){
		"image edits":        func(r *OpenAIImagesRequest) { r.Endpoint = openAIImagesEditsEndpoint },
		"stream":             func(r *OpenAIImagesRequest) { r.Stream = true },
		"partial_images":     func(r *OpenAIImagesRequest) { r.PartialImages = &one },
		"output_compression": func(r *OpenAIImagesRequest) { r.OutputCompression = &one },
		"style":              func(r *OpenAIImagesRequest) { r.Style = "vivid" },
		"input_fidelity":     func(r *OpenAIImagesRequest) { r.InputFidelity = "high" },
		"output_format":      func(r *OpenAIImagesRequest) { r.OutputFormat = "jpeg" },
		"moderation":         func(r *OpenAIImagesRequest) { r.Moderation = "low" },
	} {
		parsed := base()
		mutate(parsed)
		require.Equal(t, want, excelBPSImagesUnsupportedReason(parsed))
	}
	require.Equal(t, "missing request", excelBPSImagesUnsupportedReason(nil))
}

func TestExcelBPSImagesForwardContract(t *testing.T) {
	body := []byte(`{"model":"gpt-image-2","prompt":"  draw  ","size":"1024x1024","quality":"low","n":2,"background":"opaque","output_format":"png","moderation":"auto","response_format":"url"}`)
	c, rec := newOpenAIImagesTestContext(t, body)
	upstream := &httpUpstreamRecorder{resp: openAIImagesJSONResponse()}
	svc := newOpenAIImagesTestService(upstream)
	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
	require.NoError(t, err)
	// Like Codex images, BPS generation must outlive a client disconnect.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := svc.ForwardImages(ctx, c, excelBPSImagesAccount(), body, parsed, "")

	require.NoError(t, err)
	require.Len(t, upstream.requests, 1)
	req := upstream.lastReq
	require.Equal(t, basispoints.ImagesGenerationsURL, req.URL.String())
	require.NoError(t, req.Context().Err())
	for key, want := range map[string]string{
		"Authorization": "Bearer test-token", "Chatgpt-Account-Id": "test-account", "X-Openai-Account-Id": "test-account",
		"X-Basispoints-Auth-Mode": "chatgpt", "Accept": "application/json", "Content-Type": "application/json",
		"Origin": "https://bps.openai.com", "User-Agent": "Mozilla/5.0",
		"X-Openai-Internal-Basispoints-Client-Product":        "basispoints-excel-plugin",
		"X-Openai-Internal-Basispoints-Client-Agent-Profile":  "excel",
		"X-Openai-Internal-Basispoints-Client-Platform":       "excel",
		"X-Openai-Internal-Basispoints-Client-Editor":         "excel",
		"X-Openai-Internal-Basispoints-Client-Host":           "office",
		"X-Openai-Internal-Basispoints-Client-Runtime":        "desktop",
		"X-Openai-Internal-Basispoints-Client-Platform-Class": "PC",
		"X-Openai-Internal-Basispoints-Office-Host":           "Excel",
		"X-Openai-Internal-Basispoints-Office-Platform":       "PC",
	} {
		require.Equal(t, want, req.Header.Get(key), key)
	}
	require.Empty(t, req.Header.Get("Originator"), "Codex identity headers must not reach BPS")
	require.Equal(t, HTTPUpstreamProfileExcelBPS, HTTPUpstreamProfileFromContext(req.Context()))
	require.True(t, HTTPUpstreamRedirectsDisabled(req.Context()))

	sent := upstream.lastBody
	require.Equal(t, "gpt-image-2", gjson.GetBytes(sent, "model").String())
	require.Equal(t, "  draw  ", gjson.GetBytes(sent, "prompt").String())
	require.Equal(t, "low", gjson.GetBytes(sent, "quality").String())
	require.Equal(t, "1024x1024", gjson.GetBytes(sent, "size").String())
	require.Equal(t, int64(2), gjson.GetBytes(sent, "n").Int())
	for _, key := range []string{"moderation", "response_format", "stream", "user", "style", "input_fidelity"} {
		require.False(t, gjson.GetBytes(sent, key).Exists(), key)
	}

	require.Equal(t, excelBPSImagesEndpoint, result.UpstreamEndpoint)
	require.Equal(t, excelBPSImagesEndpoint, GetActualOpenAIUpstreamEndpoint(c))
	require.Equal(t, "gpt-image-2", result.UpstreamModel)
	require.Equal(t, 1, result.ImageCount)
	require.Equal(t, 20, result.Usage.ImageOutputTokens)
	require.Equal(t, "data:image/png;base64,aGVsbG8=", gjson.GetBytes(rec.Body.Bytes(), "data.0.url").String())
}

func TestExcelBPSImagesFormatRejectionFallsBackToCodex(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusUnprocessableEntity} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var urls []string
			svc := newOpenAIImagesTestService(excelBPSImagesUpstream(status, &urls))
			body := []byte(`{"model":"gpt-image-2","prompt":"draw"}`)
			c, rec := newOpenAIImagesTestContext(t, body)
			parsed, err := svc.ParseOpenAIImagesRequest(c, body)
			require.NoError(t, err)
			account := excelBPSImagesAccount()

			result, err := svc.ForwardImages(context.Background(), c, account, body, parsed, "")

			require.NoError(t, err)
			require.Equal(t, []string{basispoints.ImagesGenerationsURL, codexDirectImagesURL}, urls)
			require.Equal(t, "/backend-api/codex/images/generations", result.UpstreamEndpoint)
			require.NotEqual(t, excelBPSImagesEndpoint, GetActualOpenAIUpstreamEndpoint(c))
			require.False(t, svc.isExcelBPSRateLimited(account, "gpt-image-2"))
			require.NotContains(t, rec.Body.String(), "PRIVATE_UPSTREAM")
		})
	}
}

func TestExcelBPSImagesStayOnCodexWhenBPSCannotServe(t *testing.T) {
	for name, tc := range map[string]struct {
		account *Account
		body    string
	}{
		"legacy all models": {account: excelAccount(), body: `{"model":"gpt-image-2","prompt":"draw"}`},
		"model not listed":  {account: excelBPSImagesAccount("gpt-6-astra"), body: `{"model":"gpt-image-2","prompt":"draw"}`},
		"jpeg output":       {account: excelBPSImagesAccount(), body: `{"model":"gpt-image-2","prompt":"draw","output_format":"jpeg"}`},
	} {
		t.Run(name, func(t *testing.T) {
			var urls []string
			svc := newOpenAIImagesTestService(excelBPSImagesUpstream(http.StatusOK, &urls))
			body := []byte(tc.body)
			c, _ := newOpenAIImagesTestContext(t, body)
			parsed, err := svc.ParseOpenAIImagesRequest(c, body)
			require.NoError(t, err)

			_, err = svc.ForwardImages(context.Background(), c, tc.account, body, parsed, "")

			require.NoError(t, err)
			require.Equal(t, []string{codexDirectImagesURL}, urls)
		})
	}
}

func TestExcelBPSImages429CoolsBPSAndFailsOver(t *testing.T) {
	upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusTooManyRequests, Header: http.Header{"Retry-After": {"30"}},
		Body: io.NopCloser(strings.NewReader(`{"error":{"message":"PRIVATE_UPSTREAM"}}`))}}
	svc := newOpenAIImagesTestService(upstream)
	body := []byte(`{"model":"gpt-image-2","prompt":"draw"}`)
	c, rec := newOpenAIImagesTestContext(t, body)
	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
	require.NoError(t, err)
	account := excelBPSImagesAccount()

	result, err := svc.ForwardImages(context.Background(), c, account, body, parsed, "")

	require.Nil(t, result)
	var failover *UpstreamFailoverError
	require.ErrorAs(t, err, &failover)
	require.Len(t, upstream.requests, 1, "a request that reached BPS is never replayed")
	require.True(t, svc.isExcelBPSRateLimited(account, "gpt-image-2"))
	require.True(t, failover.SuppressScheduleFailure)
	require.True(t, failover.SkipOAuth429AttemptLimit)
	require.Equal(t, "Excel BPS rate limit exceeded", failover.ClientMessage)
	// The next account may use Codex, so nothing BPS-specific is left behind.
	require.Empty(t, GetActualOpenAIUpstreamEndpoint(c))
	require.False(t, c.Writer.Written())
	require.NotContains(t, rec.Body.String(), "PRIVATE_UPSTREAM")
}

func TestExcelBPSImagesFinalErrorsAreNotReplayed(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError, http.StatusBadGateway} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var urls []string
			svc := newOpenAIImagesTestService(excelBPSImagesUpstream(status, &urls))
			body := []byte(`{"model":"gpt-image-2","prompt":"draw"}`)
			c, rec := newOpenAIImagesTestContext(t, body)
			parsed, err := svc.ParseOpenAIImagesRequest(c, body)
			require.NoError(t, err)
			account := excelBPSImagesAccount()

			result, err := svc.ForwardImages(context.Background(), c, account, body, parsed, "")

			require.Nil(t, result)
			var upErr *OpenAIImagesUpstreamError
			require.ErrorAs(t, err, &upErr)
			require.Equal(t, status, upErr.StatusCode)
			require.Equal(t, []string{basispoints.ImagesGenerationsURL}, urls)
			require.Equal(t, status, rec.Code)
			require.Equal(t, "basispoints_upstream_error", gjson.GetBytes(rec.Body.Bytes(), "error.code").String())
			require.Contains(t, gjson.GetBytes(rec.Body.Bytes(), "error.message").String(), "Excel BPS")
			require.NotContains(t, rec.Body.String(), "PRIVATE_UPSTREAM")
			require.Equal(t, excelBPSImagesEndpoint, GetActualOpenAIUpstreamEndpoint(c))
			require.False(t, svc.isExcelBPSRateLimited(account, "gpt-image-2"))
		})
	}
}

func TestExcelBPSImagesTransportErrorIsNotReplayed(t *testing.T) {
	upstream := &httpUpstreamRecorder{err: errors.New("connection reset by peer")}
	svc := newOpenAIImagesTestService(upstream)
	body := []byte(`{"model":"gpt-image-2","prompt":"draw"}`)
	c, rec := newOpenAIImagesTestContext(t, body)
	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
	require.NoError(t, err)

	_, err = svc.ForwardImages(context.Background(), c, excelBPSImagesAccount(), body, parsed, "")

	var upErr *OpenAIImagesUpstreamError
	require.ErrorAs(t, err, &upErr)
	require.Equal(t, http.StatusBadGateway, upErr.StatusCode)
	require.Equal(t, "basispoints_transport_error", upErr.Code)
	require.Len(t, upstream.requests, 1)
	require.Equal(t, http.StatusBadGateway, rec.Code)
}

func TestExcelBPSImagesAccountTest(t *testing.T) {
	for name, tc := range map[string]struct {
		account  *Account
		status   int
		urls     []string
		contains []string
	}{
		"bps": {account: excelBPSImagesAccount(), status: http.StatusOK, urls: []string{basispoints.ImagesGenerationsURL},
			contains: []string{"Calling Excel BPS /images/generations; image model: gpt-image-2"}},
		"bps rejects format": {account: excelBPSImagesAccount(), status: http.StatusUnprocessableEntity, urls: []string{basispoints.ImagesGenerationsURL, codexDirectImagesURL},
			contains: []string{"Excel BPS rejected this request format", "Calling Codex /images/generations"}},
		// Before this change an image model on a BPS account went to the text
		// BPS test and was sent to /responses.
		"legacy all models": {account: excelAccount(), status: http.StatusOK, urls: []string{codexDirectImagesURL},
			contains: []string{"Calling Codex /images/generations"}},
	} {
		t.Run(name, func(t *testing.T) {
			var urls []string
			upstream := excelBPSImagesUpstream(tc.status, &urls)
			svc := &AccountTestService{httpUpstream: upstream, openaiGatewayService: newOpenAIImagesTestService(upstream)}
			c, rec := newOpenAIImagesTestContext(t, nil)

			require.NoError(t, svc.testOpenAIAccountConnection(c, tc.account, "gpt-image-2", "draw", ""))

			require.Equal(t, tc.urls, urls)
			for _, text := range tc.contains {
				require.Contains(t, rec.Body.String(), text)
			}
			require.Contains(t, rec.Body.String(), `"type":"image"`)
			require.Contains(t, rec.Body.String(), `"success":true`)
			require.NotContains(t, rec.Body.String(), "PRIVATE_UPSTREAM")
		})
	}
}

func TestExcelBPSImagesAccountTestReportsRateLimit(t *testing.T) {
	upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusTooManyRequests, Header: http.Header{"Retry-After": {"30"}},
		Body: io.NopCloser(strings.NewReader(`{"error":{"message":"PRIVATE_UPSTREAM"}}`))}}
	svc := &AccountTestService{httpUpstream: upstream, openaiGatewayService: newOpenAIImagesTestService(upstream)}
	c, rec := newOpenAIImagesTestContext(t, nil)

	err := svc.testOpenAIAccountConnection(c, excelBPSImagesAccount(), "gpt-image-2", "draw", "")

	// A single-account test shows the rate limit instead of a failover signal.
	require.EqualError(t, err, "Excel BPS rate limit exceeded")
	require.Len(t, upstream.requests, 1)
	require.NotContains(t, rec.Body.String(), "PRIVATE_UPSTREAM")
}
