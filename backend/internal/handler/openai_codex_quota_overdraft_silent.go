package handler

import (
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const codexQuotaOverdraftSilentRetryBudget = 30 * time.Second

type openAIResponsesForwardAttempt func([]byte) (*service.OpenAIForwardResult, error)

func (h *OpenAIGatewayHandler) forwardResponsesWithCodexQuotaOverdraftSilentRetry(
	c *gin.Context,
	account *service.Account,
	body []byte,
	stream bool,
	reqLog *zap.Logger,
	forward openAIResponsesForwardAttempt,
) (*service.OpenAIForwardResult, error) {
	silent := h.gatewayService.StartCodexQuotaOverdraftSilentResponse(c, account, stream)
	if silent == nil {
		return forward(body)
	}
	defer func() {
		if err := silent.Close(); err != nil {
			reqLog.Warn("openai.codex_overdraft_silent_cleanup_failed", zap.Error(err))
		}
	}()

	body = h.gatewayService.PrepareCodexQuotaOverdraftSilentRetryBody(c.Request.Context(), account, body)
	startedAt := time.Now()
	discard := func() {
		if err := silent.Discard(); err != nil {
			reqLog.Warn("openai.codex_overdraft_silent_discard_failed", zap.Error(err))
		}
	}
	stopNextAccount := func(failoverErr *service.UpstreamFailoverError) error {
		stopped := *failoverErr
		stopped.NextAccountAction = service.NextAccountStop
		stopped.RetryableOnSameAccount = false
		return &stopped
	}

	for attempt := 1; attempt <= service.CodexQuotaOverdraftSilentMaxAttempts; attempt++ {
		result, err := forward(body)
		if attemptErr := silent.AttemptError(); attemptErr != nil {
			discard()
			return result, attemptErr
		}
		if err == nil {
			if commitErr := silent.Commit(); commitErr != nil {
				return result, commitErr
			}
			if attempt > 1 {
				reqLog.Info("openai.codex_overdraft_silent_retry_recovered",
					zap.Int64("account_id", account.ID),
					zap.Int("attempt", attempt),
				)
			}
			return result, nil
		}

		var failoverErr *service.UpstreamFailoverError
		if !errors.As(err, &failoverErr) || !service.IsCodexQuotaOverdraftSilentRetryable(err) {
			if failoverErr != nil || !silent.HasBufferedResponse() {
				discard()
			} else if commitErr := silent.Commit(); commitErr != nil {
				return result, commitErr
			}
			return result, err
		}

		if attempt == service.CodexQuotaOverdraftSilentMaxAttempts {
			discard()
			reqLog.Warn("openai.codex_overdraft_silent_retry_exhausted",
				zap.Int64("account_id", account.ID),
				zap.Int("attempts", attempt),
				zap.Int("upstream_status", failoverErr.StatusCode),
			)
			return nil, stopNextAccount(failoverErr)
		}

		delayErr := *failoverErr
		delayErr.RequestScopedTransient = true
		delay := sameAccountRetryDelayFor(&delayErr, attempt)
		if time.Since(startedAt)+delay >= codexQuotaOverdraftSilentRetryBudget {
			discard()
			reqLog.Warn("openai.codex_overdraft_silent_retry_budget_exhausted",
				zap.Int64("account_id", account.ID),
				zap.Int("attempts", attempt),
				zap.Duration("budget", codexQuotaOverdraftSilentRetryBudget),
			)
			return nil, stopNextAccount(failoverErr)
		}
		if resetErr := silent.ResetAttempt(); resetErr != nil {
			discard()
			return nil, resetErr
		}
		reqLog.Warn("openai.codex_overdraft_silent_retry",
			zap.Int64("account_id", account.ID),
			zap.Int("attempt", attempt+1),
			zap.Int("max_attempts", service.CodexQuotaOverdraftSilentMaxAttempts),
			zap.Int("upstream_status", failoverErr.StatusCode),
			zap.Duration("retry_delay", delay),
		)
		select {
		case <-c.Request.Context().Done():
			discard()
			return nil, failoverErr
		case <-time.After(delay):
		}
	}

	panic("unreachable")
}
