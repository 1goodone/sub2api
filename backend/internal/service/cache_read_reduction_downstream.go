package service

import (
	"encoding/json"
	"math"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// downstreamCacheReadReductionPercent 返回当前请求分组的「缓存读取降低比例」。
// 该比例已经用于本地计费（RecordUsage），这里用于把同一口径下发给下游，
// 使下游（自算账的客户端或二级站）看到的 usage 与本地扣费一致。
func downstreamCacheReadReductionPercent(c *gin.Context) float64 {
	if c == nil {
		return 0
	}
	apiKey := getAPIKeyFromContext(c)
	if apiKey == nil || apiKey.Group == nil {
		return 0
	}
	return apiKey.Group.CacheReadReductionPercent
}

// cacheReadUsageJSONPaths 列出各协议客户端可见 usage 所在的 JSON 路径：
// OpenAI 非流式/流式为 usage、response.usage；Anthropic message_start 为 message.usage。
var cacheReadUsageJSONPaths = []string{"usage", "response.usage", "message.usage"}

// reduceCacheReadUsagePayload 改写一个客户端可见 JSON 负载（响应体或单条 SSE data）
// 中的 usage，返回改写后的负载；未命中 usage 时原样返回。
//
// 调用约定：只能在解析完计费 usage 之后调用。RecordUsage 仍按上游原始 usage
// 应用一次比例，因此本函数只改下发字节，不参与本地记账。
func reduceCacheReadUsagePayload(payload []byte, percent float64) []byte {
	if len(payload) == 0 || percent <= 0 || math.IsNaN(percent) {
		return payload
	}
	for _, path := range cacheReadUsageJSONPaths {
		node := gjson.GetBytes(payload, path)
		if !node.IsObject() {
			continue
		}
		var usage map[string]any
		if err := json.Unmarshal([]byte(node.Raw), &usage); err != nil {
			continue
		}
		if !reduceCacheReadUsageNode(usage, percent) {
			continue
		}
		raw, err := json.Marshal(usage)
		if err != nil {
			continue
		}
		updated, err := sjson.SetRawBytes(payload, path, raw)
		if err != nil {
			continue
		}
		payload = updated
	}
	return payload
}

// reduceClientVisibleSSELine 改写一条 SSE 文本行（或一个完整 data 帧）中
// data 负载里的 usage，保留原有的换行/空行分隔。
// 只用于写给下游的副本：调用方必须继续用原始行做本地计费解析。
func reduceClientVisibleSSELine(line string, percent float64) string {
	if percent <= 0 || !strings.HasPrefix(line, "data:") {
		return line
	}
	rest := line[len("data:"):]
	body := strings.TrimRight(rest, "\r\n")
	suffix := rest[len(body):]
	payload := strings.TrimSpace(body)
	if payload == "" || payload == "[DONE]" {
		return line
	}
	reduced := reduceCacheReadUsagePayload([]byte(payload), percent)
	if string(reduced) == payload {
		return line
	}
	return "data: " + string(reduced) + suffix
}

// reduceClientVisibleSSEBlock 改写一个完整 SSE 事件块（可能同时含 event: 行）
// 中 data 负载里的 usage。
func reduceClientVisibleSSEBlock(block string, percent float64) string {
	if percent <= 0 || !strings.Contains(block, "data:") {
		return block
	}
	lines := strings.Split(block, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "data:") {
			lines[i] = reduceClientVisibleSSELine(line, percent)
		}
	}
	return strings.Join(lines, "\n")
}

// reduceCacheReadUsageEvent 改写一个已解析 SSE 事件中的 usage，返回是否发生变化。
func reduceCacheReadUsageEvent(event map[string]any, percent float64) bool {
	if event == nil || percent <= 0 {
		return false
	}
	changed := false
	if usage, ok := event["usage"].(map[string]any); ok {
		changed = reduceCacheReadUsageNode(usage, percent) || changed
	}
	if message, ok := event["message"].(map[string]any); ok {
		if usage, ok := message["usage"].(map[string]any); ok {
			changed = reduceCacheReadUsageNode(usage, percent) || changed
		}
	}
	if response, ok := event["response"].(map[string]any); ok {
		if usage, ok := response["usage"].(map[string]any); ok {
			changed = reduceCacheReadUsageNode(usage, percent) || changed
		}
	}
	return changed
}

// reduceCacheReadUsageNode 按协议语义改写单个 usage 对象。
//
// Anthropic 语义：cache_read_input_tokens 与 input_tokens 互斥，挪回的部分要计入
// input_tokens，保持 input + cache_read + cache_creation 总量不变。
// OpenAI 语义：cached_tokens 只是 input/prompt 的子集明细，总量不动。
func reduceCacheReadUsageNode(usage map[string]any, percent float64) bool {
	if usage == nil || percent <= 0 || math.IsNaN(percent) {
		return false
	}
	if _, ok := usage["cache_creation_input_tokens"]; ok {
		return reduceAnthropicCacheReadUsage(usage, percent)
	}
	// OpenAI 协议的缓存明细只出现在 *_tokens_details 里，此时 input/prompt 是总量，
	// 不能把挪回量再加到 input_tokens（否则会把总量算大）。
	if _, ok := usage["input_tokens_details"]; ok {
		return reduceOpenAICacheReadUsage(usage, percent)
	}
	if _, ok := usage["prompt_tokens_details"]; ok {
		return reduceOpenAICacheReadUsage(usage, percent)
	}
	if _, ok := usageIntField(usage, "cache_read_input_tokens"); ok {
		return reduceAnthropicCacheReadUsage(usage, percent)
	}
	// 国产 Anthropic 兼容商用 prompt_cache_hit_tokens 表示缓存读取，按 Anthropic 语义处理。
	if _, ok := usageIntField(usage, "prompt_cache_hit_tokens"); ok {
		return reduceAnthropicCacheReadUsage(usage, percent)
	}
	return reduceOpenAICacheReadUsage(usage, percent)
}

func reduceAnthropicCacheReadUsage(usage map[string]any, percent float64) bool {
	// 上游可能是原生 Claude（cache_read_input_tokens），也可能是国产兼容商
	// （prompt_cache_hit_tokens / cached_tokens）。各别名按同比例缩减，
	// input_tokens 按其中最大值的挪回量补齐，保证 input + cache_read 总量守恒。
	original := 0
	present := false
	for _, key := range []string{"cache_read_input_tokens", "prompt_cache_hit_tokens", "cached_tokens"} {
		if v, ok := usageIntField(usage, key); ok {
			present = true
			if v > original {
				original = v
			}
		}
	}
	if !present {
		return false
	}
	_, moved := reduceCacheReadTokens(original, percent)
	if moved <= 0 {
		return false
	}
	for _, key := range []string{"cache_read_input_tokens", "prompt_cache_hit_tokens", "cached_tokens"} {
		if v, ok := usageIntField(usage, key); ok {
			remaining, _ := reduceCacheReadTokens(v, percent)
			usage[key] = remaining
		}
	}
	usage["input_tokens"] = usageIntValue(usage["input_tokens"]) + moved
	return true
}

func reduceOpenAICacheReadUsage(usage map[string]any, percent float64) bool {
	changed := false
	for _, details := range []string{"input_tokens_details", "prompt_tokens_details"} {
		node, ok := usage[details].(map[string]any)
		if !ok {
			continue
		}
		if v, ok := usageIntField(node, "cached_tokens"); ok {
			if remaining, moved := reduceCacheReadTokens(v, percent); moved > 0 {
				node["cached_tokens"] = remaining
				changed = true
			}
		}
	}
	for _, key := range []string{"cached_tokens", "image_cache_read_tokens", "cache_read_input_tokens"} {
		if v, ok := usageIntField(usage, key); ok {
			if remaining, moved := reduceCacheReadTokens(v, percent); moved > 0 {
				usage[key] = remaining
				changed = true
			}
		}
	}
	return changed
}

// usageIntField 读取 usage 对象里的整数字段，兼容 JSON 解码后的 float64 与
// 直接构造的 int。
func usageIntField(usage map[string]any, key string) (int, bool) {
	if usage == nil {
		return 0, false
	}
	value, ok := usage[key]
	if !ok {
		return 0, false
	}
	switch n := value.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	case json.Number:
		parsed, err := n.Int64()
		if err != nil {
			return 0, false
		}
		return int(parsed), true
	}
	return 0, false
}

func usageIntValue(value any) int {
	switch n := value.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	case json.Number:
		parsed, err := n.Int64()
		if err != nil {
			return 0
		}
		return int(parsed)
	}
	return 0
}
