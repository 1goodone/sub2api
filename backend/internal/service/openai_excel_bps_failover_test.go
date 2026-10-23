package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service/basispoints"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestExcelBPSCredentialFailurePausesWholeAccount(t *testing.T) {
	svc := openAIClientToolsTestService(nil)
	repo, _ := setupExcelBPSAuth(svc)
	a := excelAccount()
	svc.pauseExcelBPSCredentialError(context.Background(), a, errors.New("access_token not found in credentials"))
	require.Equal(t, 1, repo.errorCalls)
	require.Equal(t, a.ID, repo.accountID)
	require.Contains(t, repo.reason, "resume this account")
	require.True(t, svc.isOpenAIAccountRuntimeBlocked(a))
	require.True(t, a.IsSchedulable(), "the shared account snapshot must not be mutated")
	require.NoError(t, repo.ctxErr)
}

func TestExcelBPSCredentialInfrastructureFailuresDoNotDisableAccounts(t *testing.T) {
	for _, err := range []error{context.Canceled, context.DeadlineExceeded, infraerrors.ServiceUnavailable("OPENAI_EXCEL_AUTH_PENDING", "pending"), infraerrors.ServiceUnavailable("OPENAI_EXCEL_AUTH_UNAVAILABLE", "unavailable"), errors.New("failed to read Excel credentials")} {
		svc := openAIClientToolsTestService(nil)
		repo, _ := setupExcelBPSAuth(svc)
		svc.pauseExcelBPSCredentialError(context.Background(), excelAccount(), err)
		require.Zero(t, repo.errorCalls, err.Error())
	}
}

func TestExcelBPSRetryClassifiesKnownFailuresOnly(t *testing.T) {
	for _, tc := range []struct {
		status int
		code   string
		retry  bool
	}{
		{429, "rate_limit_exceeded", true}, {500, "server_error", true}, {503, "basispoints_upstream_error", true},
		{502, "basispoints_upstream_error", false}, {502, "api_error", true}, {401, "token_revoked", true},
		{403, "model_not_allowed", true}, {404, "model_not_found", true}, {403, "permission_denied", false},
		{500, "cyber_policy", false}, {429, "invalid_prompt", false},
	} {
		require.Equal(t, tc.retry, excelBPSFailureCanRecover(&basispoints.UpstreamFailure{Status: tc.status, Code: tc.code}), tc.code)
	}
}

func TestExcelBPSPreoutputFailureLeavesClientResponseOpen(t *testing.T) {
	upstream := &httpUpstreamRecorder{resp: bpsCompletionResponse(200, bpsFailureWire(t, "response.failed", "rate_limit_exceeded", "", 0))}
	svc := openAIClientToolsTestService(upstream)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	_, err := svc.Forward(context.Background(), c, excelAccount(), []byte(`{"model":"gpt-6-astra","input":"hello","stream":true}`))
	var failure *UpstreamFailoverError
	require.ErrorAs(t, err, &failure)
	require.False(t, IsResponseCommitted(c))
	require.Empty(t, rec.Body.String())
	require.False(t, failure.SafeToFailoverAfterWrite, "an unwritten response must not consume the post-heartbeat switch budget")
	require.True(t, failure.SuppressScheduleFailure)
	require.True(t, failure.SkipOAuth429AttemptLimit)
}

func TestExcelBPSCorrectionRetriesOnlyRejectedSubrequest(t *testing.T) {
	u := &httpUpstreamRecorder{responses: []*http.Response{bpsCompletionResponse(429, "PRIVATE_ERROR"), bpsCompletionResponse(200, "OK")}}
	svc := openAIClientToolsTestService(u)
	req, err := newExcelBPSRequest(context.Background(), []byte(`{"input":"same correction"}`), "token", "account")
	require.NoError(t, err)
	r, err := svc.doExcelBPSRepairRequest(req, "", excelAccount())
	require.NoError(t, err)
	defer r.Body.Close()
	require.Equal(t, 200, r.StatusCode)
	require.Len(t, u.requests, 2)
	for _, attempt := range u.requests {
		body, err := attempt.GetBody()
		require.NoError(t, err)
		raw, err := io.ReadAll(body)
		body.Close()
		require.NoError(t, err)
		require.Contains(t, string(raw), "same correction")
	}
}

func TestExcelBPSCorrectionHonorsLongRetryAfter(t *testing.T) {
	limited := bpsCompletionResponse(429, "PRIVATE_ERROR")
	limited.Header.Set("Retry-After", "60")
	u := &httpUpstreamRecorder{resp: limited}
	svc := openAIClientToolsTestService(u)
	req, err := newExcelBPSRequest(context.Background(), []byte(`{"input":"correction"}`), "token", "account")
	require.NoError(t, err)
	r, err := svc.doExcelBPSRepairRequest(req, "", excelAccount())
	require.NoError(t, err)
	require.Equal(t, 429, r.StatusCode)
	raw, err := io.ReadAll(r.Body)
	r.Body.Close()
	require.NoError(t, err)
	require.Equal(t, "PRIVATE_ERROR", strings.TrimSpace(string(raw)))
	require.Len(t, u.requests, 1)
}

func TestExcelBPSCorrectionCancellationDoesNotRetry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	upstream := &bpsTestUpstream{send: func(_ *http.Request, _ string) (*http.Response, error) {
		calls++
		cancel()
		return bpsCompletionResponse(429, "PRIVATE_ERROR"), nil
	}}
	svc := openAIClientToolsTestService(nil)
	svc.httpUpstream = upstream
	req, err := newExcelBPSRequest(ctx, []byte(`{"input":"correction"}`), "token", "account")
	require.NoError(t, err)
	_, err = svc.doExcelBPSRepairRequest(req, "", excelAccount())
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 1, calls)
}
