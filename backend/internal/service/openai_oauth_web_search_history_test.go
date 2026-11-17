package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// Codex local compaction replays hosted web_search_call history with tools:[];
// ChatGPT then fails with "response protection is unavailable" (#7927).
const openAIWebSearchHistoryCompactionBody = `{"model":"gpt-5.5","instructions":"x","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"look it up"}]},{"type":"web_search_call","id":"ws_123","status":"completed","action":{"type":"search","query":"kwin inputmethod"}},{"type":"message","role":"assistant","content":[{"type":"output_text","text":"found"}]},{"type":"message","role":"user","content":[{"type":"input_text","text":"summarize"}]}],"tools":[],"tool_choice":"auto","parallel_tool_calls":false,"stream":true,"store":false}`

func TestOAuthWebSearchHistoryToolAcrossPaths(t *testing.T) {
	oauth := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	transform := func(compact bool) func([]byte) ([]byte, bool, error) {
		return func(b []byte) ([]byte, bool, error) {
			var req map[string]any
			if err := json.Unmarshal(b, &req); err != nil {
				return nil, false, err
			}
			result := applyCodexOAuthTransform(req, true, compact)
			out, err := json.Marshal(req)
			return out, result.Modified, err
		}
	}
	tests := []struct {
		name      string
		normalize func([]byte) ([]byte, bool, error)
		inject    bool
	}{
		{"transformed OAuth", transform(false), true},
		{"transformed OAuth compact", transform(true), false},
		{"OAuth websocket", func(b []byte) ([]byte, bool, error) {
			return normalizeOpenAIResponsesWebSocketCompatibilityBody(b, oauth, false)
		}, true},
		{"OAuth passthrough compact", func(b []byte) ([]byte, bool, error) {
			return normalizeOpenAIResponsesCompatibilityBodyWithOptions(b, oauth, openAIResponsesCompatibilityOptions{Compact: true})
		}, false},
		{"API key websocket", func(b []byte) ([]byte, bool, error) {
			return normalizeOpenAIResponsesWebSocketCompatibilityBody(b, &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, false)
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, _, err := tt.normalize([]byte(openAIWebSearchHistoryCompactionBody))
			require.NoError(t, err)
			require.Equal(t, "web_search_call", gjson.GetBytes(out, "input.1.type").String())
			if !tt.inject {
				require.False(t, gjsonToolsContainWebSearch(gjson.GetBytes(out, "tools")))
				return
			}
			tools := gjson.GetBytes(out, "tools").Array()
			require.Len(t, tools, 1)
			require.Equal(t, "web_search", tools[0].Get("type").String())
			require.False(t, tools[0].Get("external_web_access").Exists(), "native OAuth rejects this API-only field")
			require.Equal(t, "none", gjson.GetBytes(out, "tool_choice").String())
		})
	}
}

func TestEnsureOpenAIOAuthWebSearchToolForHistory(t *testing.T) {
	const (
		user    = `{"type":"message","role":"user","content":[{"type":"input_text","text":"q"}]}`
		search  = `{"type":"web_search_call","id":"ws_1","status":"completed","action":{"type":"search","query":"q"}}`
		trigger = `{"type":"compaction_trigger"}`
		history = `[` + user + `,` + search + `]`
	)
	tests := []struct {
		name       string
		body       string
		lite       bool
		changed    bool
		toolsPath  string
		toolCount  int
		toolChoice string
		itemTypes  []string
	}{
		{"tools absent", `{"input":` + history + `}`, false, true, "tools", 1, "none", nil},
		{"tools null", `{"input":` + history + `,"tools":null,"tool_choice":null}`, false, true, "tools", 1, "none", nil},
		{"explicit none kept", `{"input":` + history + `,"tools":[],"tool_choice":"none"}`, false, true, "tools", 1, "none", nil},
		{"required not rewritten", `{"input":` + history + `,"tools":[],"tool_choice":"required"}`, false, true, "tools", 1, "none", nil},
		{"caller tools restrict auto", `{"input":` + history + `,"tools":[{"type":"function","name":"echo"}],"tool_choice":"auto"}`, false, true, "tools", 2, `{"type":"allowed_tools","mode":"auto","tools":[{"type":"function","name":"echo"}]}`, nil},
		{"web_search already declared", `{"input":` + history + `,"tools":[{"type":"web_search"}]}`, false, false, "", 0, "", nil},
		{"web_search_preview declared", `{"input":` + history + `,"tools":[{"type":"web_search_preview"}]}`, false, false, "", 0, "", nil},
		{"declared via additional_tools", `{"input":[{"type":"additional_tools","tools":[{"type":"web_search"}]},` + search + `]}`, false, false, "", 0, "", nil},
		{"no web_search_call item", `{"input":[{"type":"message","role":"user","content":"web_search_call"}],"tools":[]}`, false, false, "", 0, "", nil},
		{"string input", `{"input":"web_search_call"}`, false, false, "", 0, "", nil},
		{"lite inserts before compaction trigger", `{"input":[` + user + `,` + search + `,` + trigger + `]}`, true, true, "input.2.tools", 1, "none",
			[]string{"message", "web_search_call", "additional_tools", "compaction_trigger"}},
		{"lite appends without trigger", `{"input":` + history + `,"tool_choice":"auto"}`, true, true, "input.2.tools", 1, "none",
			[]string{"message", "web_search_call", "additional_tools"}},
		{"lite extends caller additional_tools", `{"input":[{"type":"additional_tools","role":"developer","tools":[{"type":"function","name":"echo"}]},` + search + `,` + trigger + `],"tool_choice":"auto"}`, true, true, "input.0.tools", 2, `{"type":"allowed_tools","mode":"auto","tools":[{"type":"function","name":"echo"}]}`,
			[]string{"additional_tools", "web_search_call", "compaction_trigger"}},
		{"lite fills empty additional_tools", `{"input":[{"type":"additional_tools","role":"developer"},` + search + `]}`, true, true, "input.0.tools", 1, "none",
			[]string{"additional_tools", "web_search_call"}},
		{"lite keeps top-level tools untouched", `{"input":` + history + `,"tools":[{"type":"function","name":"echo"}]}`, true, true, "input.2.tools", 1, `{"type":"allowed_tools","mode":"auto","tools":[{"type":"function","name":"echo"}]}`,
			[]string{"message", "web_search_call", "additional_tools"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, changed, err := ensureOpenAIOAuthWebSearchToolForHistoryBody([]byte(tt.body), tt.lite)
			require.NoError(t, err)
			require.Equal(t, tt.changed, changed)
			var req map[string]any
			require.NoError(t, json.Unmarshal([]byte(tt.body), &req))
			require.Equal(t, tt.changed, ensureOpenAIOAuthWebSearchToolForHistory(req, tt.lite))
			if !tt.changed {
				require.JSONEq(t, tt.body, string(out))
				return
			}
			require.True(t, gjson.ValidBytes(out))
			tools := gjson.GetBytes(out, tt.toolsPath).Array()
			require.Len(t, tools, tt.toolCount)
			require.Equal(t, "web_search", tools[len(tools)-1].Get("type").String())
			require.False(t, tools[len(tools)-1].Get("external_web_access").Exists())
			if strings.HasPrefix(tt.toolChoice, "{") {
				require.JSONEq(t, tt.toolChoice, gjson.GetBytes(out, "tool_choice").Raw)
			} else {
				require.Equal(t, tt.toolChoice, gjson.GetBytes(out, "tool_choice").String())
			}
			if tt.lite {
				require.False(t, gjsonToolsContainWebSearch(gjson.GetBytes(out, "tools")), "Lite rejects top-level hosted tools")
				var types []string
				for _, item := range gjson.GetBytes(out, "input").Array() {
					types = append(types, item.Get("type").String())
				}
				require.Equal(t, tt.itemTypes, types)
			}

			// The map variant used by the Codex transform must agree.
			mapOut, err := json.Marshal(req)
			require.NoError(t, err)
			require.JSONEq(t, string(out), string(mapOut))
		})
	}
}

func TestEnsureOpenAIOAuthWebSearchToolForHistoryDoesNotShareToolMap(t *testing.T) {
	first := map[string]any{"input": []any{map[string]any{"type": "web_search_call"}}}
	second := map[string]any{"input": []any{map[string]any{"type": "web_search_call"}}}
	require.True(t, ensureOpenAIOAuthWebSearchToolForHistory(first, false))
	require.True(t, ensureOpenAIOAuthWebSearchToolForHistory(second, false))
	firstTools, ok := first["tools"].([]any)
	require.True(t, ok)
	require.Len(t, firstTools, 1)
	firstTool, ok := firstTools[0].(map[string]any)
	require.True(t, ok)
	secondTools, ok := second["tools"].([]any)
	require.True(t, ok)
	require.Len(t, secondTools, 1)
	secondTool, ok := secondTools[0].(map[string]any)
	require.True(t, ok)
	firstTool["type"] = "modified"
	require.Equal(t, "web_search", secondTool["type"])
	require.Equal(t, "web_search", openAIWebSearchHistoryTool["type"])
}

func TestOAuthHistoricalSearchPreservesExecutableToolScope(t *testing.T) {
	const declarations = `[{"type":"function","name":"record","parameters":{"type":"object"}},{"type":"function","name":"verify"}]`
	const refs = `[{"type":"function","name":"record"},{"type":"function","name":"verify"}]`
	for _, tc := range []struct{ name, tools, choice, expected string }{
		{"auto", declarations, `"auto"`, `{"type":"allowed_tools","mode":"auto","tools":` + refs + `}`},
		{"required", declarations, `"required"`, `{"type":"allowed_tools","mode":"required","tools":` + refs + `}`},
		{"null", declarations, `null`, `{"type":"allowed_tools","mode":"auto","tools":` + refs + `}`},
		{"none", declarations, `"none"`, `"none"`},
		{"forced_function", declarations, `{"type":"function","name":"record"}`, `{"type":"function","name":"record"}`},
		{"restricted", declarations, `{"type":"allowed_tools","mode":"required","tools":[{"type":"function","name":"verify"}]}`, `{"type":"allowed_tools","mode":"required","tools":[{"type":"function","name":"verify"}]}`},
		{"undeclared_search_reference", declarations, `{"type":"allowed_tools","mode":"auto","tools":[{"type":"function","name":"record"},{"type":"web_search"}]}`, `{"type":"allowed_tools","mode":"auto","tools":[{"type":"function","name":"record"}]}`},
		{"undeclared_forced_search", declarations, `{"type":"web_search"}`, `"none"`},
		{"no_tools_required", `[]`, `"required"`, `"none"`},
		{"legacy_function", `[{"type":"function","function":{"name":"record","parameters":{"type":"object"}}}]`, `"auto"`, `{"type":"allowed_tools","mode":"auto","tools":[{"type":"function","name":"record"}]}`},
		{"custom_and_mcp", `[{"type":"custom","name":"edit","format":{"type":"text"}},{"type":"mcp","server_label":"fixture"}]`, `"required"`, `{"type":"allowed_tools","mode":"required","tools":[{"type":"custom","name":"edit"},{"type":"mcp","server_label":"fixture"}]}`},
		{"string_auto", `["edit"]`, `"auto"`, `{"type":"allowed_tools","mode":"auto","tools":[{"type":"custom","name":"edit"}]}`},
		{"string_required", `["edit"]`, `"required"`, `{"type":"allowed_tools","mode":"required","tools":[{"type":"custom","name":"edit"}]}`},
		{"mixed_strings", `["edit",{"type":"function","name":"verify"}]`, `"required"`, `{"type":"allowed_tools","mode":"required","tools":[{"type":"custom","name":"edit"},{"type":"function","name":"verify"}]}`},
		{"string_explicit", `["edit"]`, `{"type":"custom","name":"edit"}`, `{"type":"custom","name":"edit"}`},
		{"string_allowlist", `["edit", "other"]`, `{"type":"allowed_tools","mode":"auto","tools":[{"type":"custom","name":"edit"}]}`, `{"type":"allowed_tools","mode":"auto","tools":[{"type":"custom","name":"edit"}]}`},
		{"string_named_search", `["web_search"]`, `"auto"`, `{"type":"allowed_tools","mode":"auto","tools":[{"type":"custom","name":"web_search"}]}`},
	} {
		for _, lite := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/lite=%t", tc.name, lite), func(t *testing.T) {
				history := `{"type":"web_search_call","id":"ws_scope","status":"completed"}`
				body := `{"input":[` + history + `],"tools":` + tc.tools + `,"tool_choice":` + tc.choice + `}`
				if lite {
					body = `{"input":[{"type":"additional_tools","role":"developer","tools":` + tc.tools + `},` + history + `],"tool_choice":` + tc.choice + `}`
				}
				wire, changed, err := ensureOpenAIOAuthWebSearchToolForHistoryBody([]byte(body), lite)
				require.NoError(t, err)
				require.True(t, changed)
				require.JSONEq(t, tc.expected, gjson.GetBytes(wire, "tool_choice").Raw)
				var decoded map[string]any
				require.NoError(t, decodeOpenAIJSONUseNumber([]byte(body), &decoded))
				require.True(t, ensureOpenAIOAuthWebSearchToolForHistory(decoded, lite))
				mapped, err := json.Marshal(decoded)
				require.NoError(t, err)
				require.JSONEq(t, string(wire), string(mapped))
				again, changed, err := ensureOpenAIOAuthWebSearchToolForHistoryBody(wire, lite)
				require.NoError(t, err)
				require.False(t, changed)
				require.Equal(t, string(wire), string(again))
			})
		}
	}
}

func TestOAuthHistoricalSearchSelectionTracksReservedAlias(t *testing.T) {
	for _, lite := range []bool{false, true} {
		body := []byte(`{"model":"gpt-6.1-sol","input":[{"type":"web_search_call","id":"ws_alias","status":"completed"}],"tools":[{"type":"function","name":"python"}],"tool_choice":"required"}`)
		var decoded map[string]any
		require.NoError(t, decodeOpenAIJSONUseNumber(body, &decoded))
		require.NoError(t, applyCodexOAuthTransformWithOptions(decoded, codexOAuthTransformOptions{ResponsesLite: lite}).Error)
		wire, err := json.Marshal(decoded)
		require.NoError(t, err)
		require.Equal(t, codexPythonToolAlias, gjson.GetBytes(wire, "tools.0.name").String())
		require.Equal(t, codexPythonToolAlias, gjson.GetBytes(wire, "tool_choice.tools.0.name").String())
		require.Equal(t, "required", gjson.GetBytes(wire, "tool_choice.mode").String())
		require.Len(t, gjson.GetBytes(wire, "tool_choice.tools").Array(), 1)
	}
}
