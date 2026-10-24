package service

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

const (
	excelBPSPolicy403DisableThreshold = 3
	excelBPSPolicy403WindowMinutes    = 180
)

// ExcelBPSPolicyCounterCache is deliberately separate from native OpenAI 403
// accounting. Native requests must neither advance nor reset this counter.
type ExcelBPSPolicyCounterCache interface {
	IncrementExcelBPSPolicy403Count(ctx context.Context, accountID int64, windowMinutes int) (int64, error)
	ResetExcelBPSPolicy403Count(ctx context.Context, accountID int64) error
}

func (s *OpenAIGatewayService) excelBPSPolicyCounter(ctx context.Context, account *Account) ExcelBPSPolicyCounterCache {
	if s == nil || s.rateLimitService == nil || account == nil || ctx.Err() != nil ||
		!account.IsExcelBPSAutoDisableOn403Enabled() || !s.excelBPSRecoveryEnabled(ctx) ||
		isQualityObservation(ctx) || isControlledExperiment(ctx) {
		return nil
	}
	// The production Redis counter implements both interfaces, with independent
	// keys. Older/custom caches can omit the BPS extension without disabling accounts.
	counter, _ := s.rateLimitService.openAI403CounterCache.(ExcelBPSPolicyCounterCache)
	return counter
}

func (s *OpenAIGatewayService) recordExcelBPSPolicyRejection(ctx context.Context, account *Account) {
	counter := s.excelBPSPolicyCounter(ctx, account)
	if counter == nil {
		return
	}
	stateCtx, cancel := openAIAccountStateContext(ctx)
	defer cancel()
	count, err := counter.IncrementExcelBPSPolicy403Count(stateCtx, account.ID, excelBPSPolicy403WindowMinutes)
	if err != nil {
		// Cache outages must not turn a single request rejection into a ban.
		logger.LegacyPrintf("service.openai_excel_bps", "policy counter failed: account_id=%d error_type=%T", account.ID, err)
		return
	}
	disabled := false
	if count >= excelBPSPolicy403DisableThreshold {
		// Use the existing opt-in protocol switch and its atomic credential check.
		// Do not set account errors, move groups or block ordinary OpenAI requests.
		disabled = s.disableExcelBPSOn403(stateCtx, account)
	}
	logger.LegacyPrintf("service.openai_excel_bps", "usage policy rejection: account_id=%d count=%d threshold=%d bps_disabled=%t", account.ID, count, excelBPSPolicy403DisableThreshold, disabled)
}

func (s *OpenAIGatewayService) resetExcelBPSPolicyRejections(ctx context.Context, account *Account) {
	counter := s.excelBPSPolicyCounter(ctx, account)
	if counter == nil {
		return
	}
	stateCtx, cancel := openAIAccountStateContext(ctx)
	defer cancel()
	if err := counter.ResetExcelBPSPolicy403Count(stateCtx, account.ID); err != nil {
		logger.LegacyPrintf("service.openai_excel_bps", "policy counter reset failed: account_id=%d error_type=%T", account.ID, err)
	}
}
