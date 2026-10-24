package basispoints

import (
	"context"
	"io"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBPSRateLimitHintValidation(t *testing.T) {
	for _, tc := range []struct {
		raw       string
		at, after int64
	}{
		{`{"error":{"resets_at":2000000000,"resets_in_seconds":"120"}}`, 2000000000, 120},
		{`{"response":{"error":{"resets_at":"2000000001","resets_in_seconds":60}}}`, 2000000001, 60},
		{`{"resets_in_seconds":"9223372036854775807"}`, 0, 9223372036854775807},
		{`{"error":{"resets_at":"99999999999999999999","resets_in_seconds":1.5}}`, 0, 0},
		{`{"error":{"resets_at":-1,"resets_in_seconds":"PRIVATE_TOKEN"}}`, 0, 0},
		{`{"error":{"resets_at":true,"resets_in_seconds":{}}}`, 0, 0},
		{`not json`, 0, 0},
	} {
		hint := ParseRateLimitHint([]byte(tc.raw))
		if tc.at == 0 && tc.after == 0 {
			require.Nil(t, hint)
			continue
		}
		require.Equal(t, &RateLimitHint{ResetAtUnix: tc.at, ResetAfterSeconds: tc.after}, hint)
	}
}

func TestBPSFailureTimingIsInternalOnly(t *testing.T) {
	for _, kind := range []string{"error", "response.failed", "response.cancelled"} {
		t.Run(kind, func(t *testing.T) {
			_, bridge := mustPrepare(t, testSource(), "", nil)
			detail := object{"code": "rate_limit_exceeded", "type": "rate_limit_exceeded", "message": "PRIVATE_TOKEN", "resets_at": 2000000000, "resets_in_seconds": 120}
			payload := object{"type": kind}
			if kind == "error" {
				payload["error"] = detail
			} else {
				payload["response"] = object{"id": "resp_limit", "status": "failed", "output": []any{}, "error": detail}
			}
			var observed atomic.Pointer[UpstreamFailure]
			ctx := WithUpstreamFailureObserver(context.Background(), func(f *UpstreamFailure) { observed.Store(f) })
			reader := bridge.StreamWithRepairs(ctx, io.NopCloser(strings.NewReader(sse(payload))), nil, nil)
			t.Cleanup(func() { require.NoError(t, reader.Close()) })
			raw, err := io.ReadAll(reader)
			require.NoError(t, err)
			f := observed.Load()
			require.NotNil(t, f)
			require.Equal(t, 429, f.Status)
			require.Equal(t, &RateLimitHint{ResetAtUnix: 2000000000, ResetAfterSeconds: 120}, f.RateLimit)
			require.Contains(t, string(raw), "rate_limit_exceeded")
			for _, private := range []string{"PRIVATE_TOKEN", "resets_at", "resets_in_seconds", "2000000000"} {
				require.NotContains(t, string(raw), private)
				require.NotContains(t, f.Message, private)
			}
		})
	}
}

func TestBPSCompactionRetainsSafeRateLimitHint(t *testing.T) {
	raw := sse(object{"type": "response.failed", "response": object{"status": "failed", "output": []any{}, "error": object{"code": "rate_limit_exceeded", "resets_in_seconds": 120, "message": "PRIVATE"}}})
	_, err := ReadImageCompaction(strings.NewReader(raw), nil)
	var failure *UpstreamFailure
	require.ErrorAs(t, err, &failure)
	require.Equal(t, int64(120), failure.RateLimit.ResetAfterSeconds)
	require.NotContains(t, err.Error(), "PRIVATE")
}
