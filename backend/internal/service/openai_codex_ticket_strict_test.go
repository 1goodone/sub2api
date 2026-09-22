package service

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestParseOpenAICodexTicketShapeStrictLengths(t *testing.T) {
	personal, err := parseOpenAICodexTicketShape(fakeCodexTicketState(292))
	require.NoError(t, err)
	require.Equal(t, openAICodexTicketPersonalBlocks, personal.Blocks)
	require.False(t, personal.IssuedAt.IsZero())

	team, err := parseOpenAICodexTicketShape(fakeCodexTicketState(332))
	require.NoError(t, err)
	require.Equal(t, openAICodexTicketTeamBlocks, team.Blocks)

	for _, state := range []string{
		fakeCodexTicketState(312),
		openAICodexTicketStatePrefix + strings.Repeat("!", 286),
		"",
	} {
		_, err := parseOpenAICodexTicketShape(state)
		require.Error(t, err)
	}
}

func TestOpenAICodexTicketTeamPlanUses332Envelope(t *testing.T) {
	svc := ticketTestService(t, config.OpenAICodexTicketConfig{Enabled: true, FailClosed: true}, nil)
	account := ticketTestAccount(41)
	account.Credentials["plan_type"] = "self_serve_business_prolite"
	state := fakeCodexTicketState(332)
	svc.storeOpenAICodexTicket(context.Background(), account, &openAICodexTicket{
		Model: "gpt-6-astra", State: state, CapturedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour),
	})

	header := http.Header{}
	require.NoError(t, svc.applyOpenAICodexTicket(context.Background(), account, "gpt-6-astra", header))
	require.Equal(t, state, header.Get(openAICodexTurnStateHeader))
	account.Extra = map[string]any{openAICodexTicketExtraKey("gpt-6-astra"): svc.lookupOpenAICodexTicket(account, "gpt-6-astra")}
	statuses := OpenAICodexTicketStatuses(account, svc.openAICodexTicketConfig(), time.Now())
	require.Len(t, statuses, 2)
	require.Equal(t, 332, statuses[0].Length)
}

func TestOpenAICodexTicketIdentityBinding(t *testing.T) {
	svc := ticketTestService(t, config.OpenAICodexTicketConfig{Enabled: true, FailClosed: true}, nil)
	original := ticketTestAccount(41)
	original.Credentials["email"] = "one@example.com"
	svc.storeOpenAICodexTicket(context.Background(), original, &openAICodexTicket{
		Model:      "gpt-6-astra",
		State:      fakeCodexTicketState(292),
		CapturedAt: time.Now(),
		ExpiresAt:  time.Now().Add(time.Hour),
	})

	rotated := ticketTestAccount(41)
	rotated.Credentials["email"] = "two@example.com"
	require.Nil(t, svc.lookupOpenAICodexTicket(rotated, "gpt-6-astra"))
	require.True(t, svc.openAICodexTicketBlocksAccount(rotated, "gpt-6-astra"))
}

func TestOpenAICodexTicketStandbyPromotesAfterPrimaryExpires(t *testing.T) {
	svc := ticketTestService(t, config.OpenAICodexTicketConfig{Enabled: true, FailClosed: true}, nil)
	account := ticketTestAccount(41)
	first := fakeCodexTicketStateSalt(292, 1)
	second := fakeCodexTicketStateSalt(292, 2)
	now := time.Now()
	svc.storeOpenAICodexTicket(context.Background(), account, &openAICodexTicket{
		Model:      "gpt-6-astra",
		State:      first,
		CapturedAt: now,
		ExpiresAt:  now.Add(-time.Minute),
	})
	// The first ticket is already expired, so the second becomes primary.
	svc.storeOpenAICodexTicket(context.Background(), account, &openAICodexTicket{
		Model:      "gpt-6-astra",
		State:      second,
		CapturedAt: now.Add(time.Second),
		ExpiresAt:  now.Add(time.Hour),
	})
	got := svc.lookupOpenAICodexTicket(account, "gpt-6-astra")
	require.NotNil(t, got)
	require.Equal(t, second, got.State)

	// A live primary is retained while a newer capture is kept as standby.
	third := fakeCodexTicketStateSalt(292, 3)
	svc.storeOpenAICodexTicket(context.Background(), account, &openAICodexTicket{
		Model:      "gpt-6-astra",
		State:      third,
		CapturedAt: time.Now(),
		ExpiresAt:  time.Now().Add(time.Hour),
	})
	current := svc.lookupOpenAICodexTicket(account, "gpt-6-astra")
	require.Equal(t, second, current.State)
	require.NotNil(t, current.Standby)
	require.Equal(t, third, current.Standby.State)
}

func TestOpenAICodexTicketRejectsForeignStandbyIdentity(t *testing.T) {
	svc := ticketTestService(t, config.OpenAICodexTicketConfig{Enabled: true, FailClosed: true}, nil)
	account := ticketTestAccount(41)
	foreign := ticketTestAccount(42)
	now := time.Now()
	primary := &openAICodexTicket{
		Model:      "gpt-6-astra",
		State:      fakeCodexTicketStateSalt(292, 1),
		CapturedAt: now,
		ExpiresAt:  now.Add(-time.Minute),
	}
	primary.Identity = ticketIdentity(account)
	primary.Standby = &openAICodexTicket{
		Model:      primary.Model,
		State:      fakeCodexTicketStateSalt(292, 2),
		CapturedAt: now,
		ExpiresAt:  now.Add(time.Hour),
		Identity:   ticketIdentity(foreign),
	}
	svc.openaiCodexTickets.Store(openAICodexTicketKey(account.ID, primary.Model), primary)

	require.Nil(t, svc.lookupOpenAICodexTicket(account, primary.Model))
	require.True(t, svc.openAICodexTicketBlocksAccount(account, primary.Model))
}

func TestOpenAICodexTicketRepeatedProbeKeepsStandby(t *testing.T) {
	svc := ticketTestService(t, config.OpenAICodexTicketConfig{Enabled: true}, nil)
	account := ticketTestAccount(41)
	now := time.Now()
	first := fakeCodexTicketStateSalt(292, 1)
	second := fakeCodexTicketStateSalt(292, 2)
	svc.storeOpenAICodexTicket(context.Background(), account, &openAICodexTicket{
		Model: "gpt-6-astra", State: first, CapturedAt: now, ExpiresAt: now.Add(time.Hour),
	})
	svc.storeOpenAICodexTicket(context.Background(), account, &openAICodexTicket{
		Model: "gpt-6-astra", State: second, CapturedAt: now.Add(time.Second), ExpiresAt: now.Add(time.Hour),
	})
	svc.storeOpenAICodexTicket(context.Background(), account, &openAICodexTicket{
		Model: "gpt-6-astra", State: first, CapturedAt: now.Add(2 * time.Second), ExpiresAt: now.Add(time.Hour),
	})

	got := svc.lookupOpenAICodexTicket(account, "gpt-6-astra")
	require.NotNil(t, got)
	require.Equal(t, first, got.State)
	require.NotNil(t, got.Standby)
	require.Equal(t, second, got.Standby.State)
}

func TestValidateCodexProbeResponse(t *testing.T) {
	valid := `data: {"type":"response.completed","response":{"model":"gpt-6-astra","status":"completed"}}

data: [DONE]

`
	for _, body := range [][]byte{
		[]byte(`{"status":"completed"}`),
		[]byte(valid),
	} {
		require.NoError(t, validateCodexProbeResponse(body, "gpt-6-astra"))
	}
	for _, body := range [][]byte{
		[]byte(""),
		[]byte("data: [DONE]\n\n"),
		[]byte("data: {\"type\":\"response.incomplete\"}\n\n"),
		[]byte("data: {\"type\":\"response.completed\",\"response\":{\"model\":\"gpt-5.6-sol\",\"status\":\"completed\"}}\n\n"),
	} {
		require.Error(t, validateCodexProbeResponse(body, "gpt-6-astra"))
	}
}

func TestObserveCodexTicketResponseRotatesAndRevokes(t *testing.T) {
	svc := ticketTestService(t, config.OpenAICodexTicketConfig{Enabled: true, FailClosed: true}, nil)
	account := ticketTestAccount(41)
	sent := fakeCodexTicketStateSalt(292, 1)
	rotated := fakeCodexTicketStateSalt(292, 2)
	svc.storeOpenAICodexTicket(context.Background(), account, &openAICodexTicket{
		Model:      "gpt-6-astra",
		State:      sent,
		CapturedAt: time.Now(),
		ExpiresAt:  time.Now().Add(time.Hour),
	})
	req, err := http.NewRequest(http.MethodPost, "https://chatgpt.com/backend-api/codex/responses", nil)
	require.NoError(t, err)
	req.Header.Set(openAICodexTurnStateHeader, sent)
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}}
	resp.Header.Set(openAICodexTurnStateHeader, rotated)
	svc.observeCodexTicketResponse(req, resp, account)
	require.Equal(t, rotated, svc.lookupOpenAICodexTicket(account, "gpt-6-astra").State)

	req.Header.Set(openAICodexTurnStateHeader, rotated)
	resp.Header.Set(openAICodexTurnStateHeader, openAICodexTicketStatePrefix+strings.Repeat("!", 286))
	svc.observeCodexTicketResponse(req, resp, account)
	require.True(t, svc.lookupOpenAICodexTicket(account, "gpt-6-astra").Revoked)
}
