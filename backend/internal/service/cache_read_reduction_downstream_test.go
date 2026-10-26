package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestReduceCacheReadUsagePayloadAnthropicSemantics(t *testing.T) {
	body := []byte(`{"id":"msg_1","usage":{"input_tokens":10,"output_tokens":3,"cache_read_input_tokens":90,"cache_creation_input_tokens":5}}`)

	got := reduceCacheReadUsagePayload(body, 50)

	require.Equal(t, 55, int(gjson.GetBytes(got, "usage.input_tokens").Int()), "挪回的部分必须计入 input_tokens")
	require.Equal(t, 45, int(gjson.GetBytes(got, "usage.cache_read_input_tokens").Int()))
	require.Equal(t, 5, int(gjson.GetBytes(got, "usage.cache_creation_input_tokens").Int()), "cache_creation 不受影响")
	require.Equal(t, 3, int(gjson.GetBytes(got, "usage.output_tokens").Int()))
}

func TestReduceCacheReadUsagePayloadAnthropicFullReduction(t *testing.T) {
	body := []byte(`{"usage":{"input_tokens":7,"cache_read_input_tokens":93}}`)

	got := reduceCacheReadUsagePayload(body, 100)

	require.Equal(t, 100, int(gjson.GetBytes(got, "usage.input_tokens").Int()))
	require.Equal(t, 0, int(gjson.GetBytes(got, "usage.cache_read_input_tokens").Int()))
}

func TestReduceCacheReadUsagePayloadAnthropicKeepsCachedTokensAliasInSync(t *testing.T) {
	body := []byte(`{"usage":{"input_tokens":0,"cache_read_input_tokens":40,"cached_tokens":40}}`)

	got := reduceCacheReadUsagePayload(body, 25)

	require.Equal(t, 10, int(gjson.GetBytes(got, "usage.input_tokens").Int()))
	require.Equal(t, 30, int(gjson.GetBytes(got, "usage.cache_read_input_tokens").Int()))
	require.Equal(t, 30, int(gjson.GetBytes(got, "usage.cached_tokens").Int()))
}

func TestReduceCacheReadUsagePayloadOpenAISemanticsKeepsTotals(t *testing.T) {
	body := []byte(`{"type":"response.completed","response":{"usage":{"input_tokens":100,"output_tokens":5,"input_tokens_details":{"cached_tokens":80}}}}`)

	got := reduceCacheReadUsagePayload(body, 25)

	require.Equal(t, 100, int(gjson.GetBytes(got, "response.usage.input_tokens").Int()), "OpenAI 语义下总输入不变")
	require.Equal(t, 60, int(gjson.GetBytes(got, "response.usage.input_tokens_details.cached_tokens").Int()))
	require.Equal(t, 5, int(gjson.GetBytes(got, "response.usage.output_tokens").Int()))
}

func TestReduceCacheReadUsagePayloadChatCompletions(t *testing.T) {
	body := []byte(`{"usage":{"prompt_tokens":200,"completion_tokens":10,"prompt_tokens_details":{"cached_tokens":120}}}`)

	got := reduceCacheReadUsagePayload(body, 50)

	require.Equal(t, 200, int(gjson.GetBytes(got, "usage.prompt_tokens").Int()))
	require.Equal(t, 60, int(gjson.GetBytes(got, "usage.prompt_tokens_details.cached_tokens").Int()))
}

func TestReduceCacheReadUsagePayloadMessageUsage(t *testing.T) {
	body := []byte(`{"type":"message_start","message":{"usage":{"input_tokens":12,"cache_read_input_tokens":8,"output_tokens":1}}}`)

	got := reduceCacheReadUsagePayload(body, 50)

	require.Equal(t, 16, int(gjson.GetBytes(got, "message.usage.input_tokens").Int()))
	require.Equal(t, 4, int(gjson.GetBytes(got, "message.usage.cache_read_input_tokens").Int()))
}

func TestReduceCacheReadUsagePayloadDisabled(t *testing.T) {
	body := []byte(`{"usage":{"input_tokens":10,"cache_read_input_tokens":90}}`)

	require.Equal(t, body, reduceCacheReadUsagePayload(body, 0))
	require.Equal(t, body, reduceCacheReadUsagePayload(body, -5))
}

func TestReduceCacheReadUsagePayloadWithoutUsage(t *testing.T) {
	body := []byte(`{"id":"resp_1","output":[]}`)

	require.Equal(t, body, reduceCacheReadUsagePayload(body, 40))
}

func TestReduceCacheReadUsageEventAnthropicMessageStart(t *testing.T) {
	event := map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"usage": map[string]any{
				"input_tokens":            float64(20),
				"cache_read_input_tokens": float64(80),
			},
		},
	}

	changed := reduceCacheReadUsageEvent(event, 50)

	require.True(t, changed)
	usage := event["message"].(map[string]any)["usage"].(map[string]any)
	require.Equal(t, 60, usageIntValue(usage["input_tokens"]))
	require.Equal(t, 40, usageIntValue(usage["cache_read_input_tokens"]))
}

func TestReduceCacheReadUsageEventOpenAICompleted(t *testing.T) {
	event := map[string]any{
		"type": "response.completed",
		"response": map[string]any{
			"usage": map[string]any{
				"input_tokens": float64(100),
				"input_tokens_details": map[string]any{
					"cached_tokens": float64(40),
				},
			},
		},
	}

	changed := reduceCacheReadUsageEvent(event, 25)

	require.True(t, changed)
	usage := event["response"].(map[string]any)["usage"].(map[string]any)
	require.Equal(t, 100, usageIntValue(usage["input_tokens"]))
	details := usage["input_tokens_details"].(map[string]any)
	require.Equal(t, 30, usageIntValue(details["cached_tokens"]))
}

func TestReduceCacheReadUsageEventNoUsage(t *testing.T) {
	event := map[string]any{"type": "content_block_delta", "delta": map[string]any{"type": "text_delta", "text": "hi"}}

	require.False(t, reduceCacheReadUsageEvent(event, 50))
}

func TestReduceCacheReadUsagePayloadRoundsHalfUp(t *testing.T) {
	body := []byte(`{"usage":{"input_tokens":0,"cache_read_input_tokens":3}}`)

	got := reduceCacheReadUsagePayload(body, 50)

	// 3 * 50% = 1.5，四舍五入为 2。
	require.Equal(t, 2, int(gjson.GetBytes(got, "usage.input_tokens").Int()))
	require.Equal(t, 1, int(gjson.GetBytes(got, "usage.cache_read_input_tokens").Int()))
}

func TestReduceCacheReadUsageEventPreservesOtherFields(t *testing.T) {
	event := map[string]any{
		"type": "message_delta",
		"usage": map[string]any{
			"input_tokens":            float64(0),
			"cache_read_input_tokens": float64(10),
			"output_tokens":           float64(4),
			"server_tool_use":         map[string]any{"web_search_requests": float64(1)},
		},
	}

	require.True(t, reduceCacheReadUsageEvent(event, 10))

	usage := event["usage"].(map[string]any)
	require.Equal(t, 1, usageIntValue(usage["input_tokens"]))
	require.Equal(t, 9, usageIntValue(usage["cache_read_input_tokens"]))
	require.Equal(t, float64(4), usage["output_tokens"])

	raw, err := json.Marshal(event)
	require.NoError(t, err)
	require.Contains(t, string(raw), "server_tool_use")
}

func TestReduceCacheReadUsagePayloadReducesImageCacheReadTokens(t *testing.T) {
	body := []byte(`{"usage":{"input_tokens":200,"image_input_tokens":160,"image_cache_read_tokens":120,"output_tokens":10,"input_tokens_details":{"cached_tokens":160}}}`)

	got := reduceCacheReadUsagePayload(body, 50)

	require.Equal(t, 60, int(gjson.GetBytes(got, "usage.image_cache_read_tokens").Int()))
	require.Equal(t, 80, int(gjson.GetBytes(got, "usage.input_tokens_details.cached_tokens").Int()))
	require.Equal(t, 200, int(gjson.GetBytes(got, "usage.input_tokens").Int()), "OpenAI 语义总量不变")
}

func TestReduceCacheReadUsagePayloadOpenAIWithCacheReadMirrorKeepsTotal(t *testing.T) {
	body := []byte(`{"usage":{"input_tokens":100,"cache_read_input_tokens":80,"input_tokens_details":{"cached_tokens":80}}}`)

	got := reduceCacheReadUsagePayload(body, 50)

	require.Equal(t, 100, int(gjson.GetBytes(got, "usage.input_tokens").Int()), "有 details 时 input 是总量，不能加挪回量")
	require.Equal(t, 40, int(gjson.GetBytes(got, "usage.cache_read_input_tokens").Int()))
	require.Equal(t, 40, int(gjson.GetBytes(got, "usage.input_tokens_details.cached_tokens").Int()))
}

func TestReduceCacheReadUsagePayloadHandlesPromptCacheHitAlias(t *testing.T) {
	body := []byte(`{"usage":{"input_tokens":20,"prompt_tokens":820,"prompt_cache_hit_tokens":800,"output_tokens":10}}`)

	got := reduceCacheReadUsagePayload(body, 25)

	require.Equal(t, 600, int(gjson.GetBytes(got, "usage.prompt_cache_hit_tokens").Int()))
	require.Equal(t, 220, int(gjson.GetBytes(got, "usage.input_tokens").Int()), "挪回量计入普通输入")
	require.Equal(t, 820, int(gjson.GetBytes(got, "usage.prompt_tokens").Int()), "总量字段保持不变")
}
