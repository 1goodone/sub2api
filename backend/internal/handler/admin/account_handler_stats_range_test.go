package admin

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/stretchr/testify/require"
)

// 账号统计的时间筛选与官方结算页保持同一组档位：7d / 30d / 90d / all。
func TestParseAccountStatsRange(t *testing.T) {
	tests := []struct {
		name       string
		rangeKey   string
		daysParam  string
		wantDays   int
		wantErrMsg string
	}{
		{name: "默认近30天", rangeKey: "", daysParam: "", wantDays: 30},
		{name: "旧参数days", rangeKey: "", daysParam: "7", wantDays: 7},
		{name: "旧参数days上限90", rangeKey: "", daysParam: "365", wantDays: 30},
		{name: "旧参数days非法回退默认", rangeKey: "", daysParam: "abc", wantDays: 30},
		{name: "range优先于days", rangeKey: "7d", daysParam: "90", wantDays: 7},
		{name: "30d", rangeKey: "30d", daysParam: "", wantDays: 30},
		{name: "90d", rangeKey: "90d", daysParam: "", wantDays: 90},
		{name: "all", rangeKey: "all", daysParam: "", wantDays: 0},
		{name: "大小写与空格", rangeKey: " ALL ", daysParam: "", wantDays: 0},
		{name: "非法range", rangeKey: "1y", daysParam: "", wantErrMsg: "range must be one of 7d, 30d, 90d, all"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			days, err := parseAccountStatsRange(tt.rangeKey, tt.daysParam)
			if tt.wantErrMsg != "" {
				require.EqualError(t, err, tt.wantErrMsg)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantDays, days)
		})
	}
}

// 窗口边界与官方结算一致：按配置时区的自然日切分，结束于次日 00:00。
func TestResolveAccountStatsWindow_Days(t *testing.T) {
	now := time.Date(2026, 10, 25, 15, 4, 5, 0, time.UTC)
	window := resolveAccountStatsWindow(30, now, time.Time{})

	require.True(t, window.End.Equal(timezone.StartOfDay(now.AddDate(0, 0, 1))), "窗口应结束于次日零点")
	require.True(t, window.Start.Equal(timezone.StartOfDay(now.AddDate(0, 0, -29))), "近30天应含今天共30个自然日")
}

func TestResolveAccountStatsWindow_All(t *testing.T) {
	now := time.Date(2026, 10, 25, 15, 4, 5, 0, time.UTC)
	createdAt := time.Date(2026, 3, 7, 9, 30, 0, 0, time.UTC)

	window := resolveAccountStatsWindow(0, now, createdAt)

	require.True(t, window.Start.Equal(timezone.StartOfDay(createdAt)), "全部应自账号创建当天起算")
	require.True(t, window.End.Equal(timezone.StartOfDay(now.AddDate(0, 0, 1))))

	// 账号创建时间未知时退化为无下界，查询仍可执行。
	openWindow := resolveAccountStatsWindow(0, now, time.Time{})
	require.True(t, openWindow.Start.IsZero())
	require.True(t, openWindow.End.Equal(window.End))
}
