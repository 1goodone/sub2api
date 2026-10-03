//go:build unit

package service

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func pelicanPromptTestResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestAccountTestService_PelicanOpenAIResponsesUsesPromptAndReasoning(t *testing.T) {
	account := &Account{
		ID:          401,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{"access_token": "test-token"},
	}
	repo := &openAIAccountTestRepo{
		mockAccountRepoForGemini: mockAccountRepoForGemini{
			accountsByID: map[int64]*Account{account.ID: account},
		},
	}
	upstream := &queuedHTTPUpstream{responses: []*http.Response{
		pelicanPromptTestResponse("data: {\"type\":\"response.completed\"}\n\n"),
	}}
	svc := &AccountTestService{accountRepo: repo, httpUpstream: upstream}
	c, _ := newTestContext()

	err := svc.TestPelicanAccountConnection(c, account.ID, "gpt-5.4", "2+2=4", "high")

	require.NoError(t, err)
	require.Len(t, upstream.requests, 1)
	body, err := io.ReadAll(upstream.requests[0].Body)
	require.NoError(t, err)
	require.Equal(t, "2+2=4", gjson.GetBytes(body, "input.0.content.0.text").String())
	require.Equal(t, "high", gjson.GetBytes(body, "reasoning.effort").String())
}

func TestAccountTestService_PelicanClaudeUsesPrompt(t *testing.T) {
	account := &Account{
		ID:          402,
		Platform:    PlatformAnthropic,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "http://anthropic.example",
		},
	}
	repo := &openAIAccountTestRepo{
		mockAccountRepoForGemini: mockAccountRepoForGemini{
			accountsByID: map[int64]*Account{account.ID: account},
		},
	}
	upstream := &queuedHTTPUpstream{responses: []*http.Response{
		pelicanPromptTestResponse("data: {\"type\":\"message_stop\"}\n\n"),
	}}
	svc := &AccountTestService{
		accountRepo:  repo,
		httpUpstream: upstream,
		cfg:          rawChatCompletionsTestConfig(),
	}
	c, _ := newTestContext()

	err := svc.TestPelicanAccountConnection(c, account.ID, "claude-sonnet-4-6", "糖果题：有 4 颗糖，吃掉 1 颗，还剩几颗？", "medium")

	require.NoError(t, err)
	require.Len(t, upstream.requests, 1)
	body, err := io.ReadAll(upstream.requests[0].Body)
	require.NoError(t, err)
	require.Equal(t, "糖果题：有 4 颗糖，吃掉 1 颗，还剩几颗？", gjson.GetBytes(body, "messages.0.content.0.text").String())
}
