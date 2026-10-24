package repository

import (
	"context"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

const excelBPSPolicy403CounterPrefix = "excel_bps_policy_403_count:account:"

var _ service.ExcelBPSPolicyCounterCache = (*openAI403CounterCache)(nil)

func (c *openAI403CounterCache) IncrementExcelBPSPolicy403Count(ctx context.Context, accountID int64, windowMinutes int) (int64, error) {
	key := fmt.Sprintf("%s%d", excelBPSPolicy403CounterPrefix, accountID)
	ttlSeconds := windowMinutes * 60
	if ttlSeconds < 60 {
		ttlSeconds = 60
	}
	count, err := openAI403CounterIncrScript.Run(ctx, c.rdb, []string{key}, ttlSeconds).Int64()
	if err != nil {
		return 0, fmt.Errorf("increment excel BPS policy 403 count: %w", err)
	}
	return count, nil
}

func (c *openAI403CounterCache) ResetExcelBPSPolicy403Count(ctx context.Context, accountID int64) error {
	return c.rdb.Del(ctx, fmt.Sprintf("%s%d", excelBPSPolicy403CounterPrefix, accountID)).Err()
}
