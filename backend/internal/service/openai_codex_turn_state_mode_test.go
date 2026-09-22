package service

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func poolTestState(plan string, issued time.Time) string {
	size := 217
	if plan == "team" {
		size = 249
	}
	raw := make([]byte, size)
	raw[0] = 0x80
	binary.BigEndian.PutUint64(raw[1:9], uint64(issued.Unix()))
	return base64.URLEncoding.EncodeToString(raw)
}

func poolTestAccount(plan string) *Account {
	a := ticketTestAccount(41)
	a.Status = StatusActive
	a.Credentials["plan_type"] = plan
	a.Credentials["model_mapping"] = map[string]any{"alias": "gpt-6-astra"}
	now := time.Now().UTC().Truncate(time.Second)
	a.Extra = map[string]any{
		CodexTurnStateModeExtraKey:          CodexTurnStateModeStatePool,
		CodexTurnStateRoutingCookieExtraKey: liveTestRoutingCookies(now).extraValue(),
		CodexTurnStateProbeCacheExtraKey: codexTurnStateCacheToExtra(map[string]codexTurnStateCacheEntry{
			"gpt-6-astra": {State: poolTestState(plan, now), ObtainedAt: now, ExpiresAt: now.Add(codexTurnStateTTL), ProxyID: 1, CredentialHash: codexTurnStateCredentialHash("tok")},
		}),
		openAICodexTicketExtraKey("gpt-6-astra"): &openAICodexTicket{
			AccountID: a.ID, Model: "gpt-6-astra", State: fakeCodexTicketState(292), Length: 292, CapturedAt: now, ExpiresAt: now.Add(time.Hour),
		},
	}
	return a
}

func TestCodexTurnStateHealthyLengthBusinessPlans(t *testing.T) {
	for _, plan := range []string{"team", "business", "self_serve_business_prolite", "self_serve_business_usage_based"} {
		a := ticketTestAccount(41)
		a.Credentials["plan_type"] = plan
		require.Equal(t, 332, CodexTurnStateHealthyLength(a), plan)
	}

	a := ticketTestAccount(41)
	a.Credentials["plan_type"] = "enterprise"
	require.Zero(t, CodexTurnStateHealthyLength(a))
}

func TestFairCodexStateProbeTasksRotatesAccounts(t *testing.T) {
	tasks := []codexStateProbeTask{
		{account: Account{ID: 1}, model: "a"},
		{account: Account{ID: 1}, model: "b"},
		{account: Account{ID: 2}, model: "a"},
		{account: Account{ID: 3}, model: "a"},
		{account: Account{ID: 4}, model: "a"},
		{account: Account{ID: 5}, model: "a"},
	}

	first, cursor := fairCodexStateProbeTasks(tasks, 0)
	require.Equal(t, []int64{1, 2, 3, 4, 5}, []int64{first[0].account.ID, first[1].account.ID, first[2].account.ID, first[3].account.ID, first[4].account.ID})
	require.Equal(t, "a", first[0].model, "only the earliest due model per account should be dispatched")

	second, cursor := fairCodexStateProbeTasks(tasks, cursor)
	require.Equal(t, []int64{2, 3, 4, 5, 1}, []int64{second[0].account.ID, second[1].account.ID, second[2].account.ID, second[3].account.ID, second[4].account.ID})
	require.Equal(t, 2, cursor)
}

func TestCodexTurnStateModesSelectOneOwner(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, mode := range []string{CodexTurnStateModeInherit, CodexTurnStateModeLegacyTicket, CodexTurnStateModeStatePool, CodexTurnStateModeNative} {
			t.Run(fmt.Sprintf("%s/global=%t", mode, enabled), func(t *testing.T) {
				svc := ticketTestService(t, config.OpenAICodexTicketConfig{Enabled: enabled, FailClosed: true}, nil)
				account := poolTestAccount("plus")
				account.Extra[CodexTurnStateModeExtraKey] = mode
				want := mode
				if mode == CodexTurnStateModeInherit {
					want = CodexTurnStateModeNative
					if enabled {
						want = CodexTurnStateModeLegacyTicket
					}
				}
				ctx := svc.PinCodexTurnStateOwner(context.Background(), account)
				require.Equal(t, want, svc.codexTurnStateOwner(ctx, account))
				header := http.Header{}
				header.Set(openAICodexTurnStateHeader, "client-state")
				require.NoError(t, svc.applyCodexTurnStateForHTTPRequest(ctx, account, "gpt-6-astra", "tok", header))
				expected := "client-state"
				if want == CodexTurnStateModeLegacyTicket {
					expected = fakeCodexTicketState(292)
				} else if want == CodexTurnStateModeStatePool {
					expected = codexTurnStateCacheFromExtra(account.Extra)["gpt-6-astra"].State
				}
				require.Equal(t, expected, header.Get(openAICodexTurnStateHeader))

				// Changing account/global configuration cannot switch an in-flight owner.
				account.Extra[CodexTurnStateModeExtraKey] = CodexTurnStateModeNative
				svc.cfg.Gateway.OpenAICodexTicket.Enabled = !enabled
				require.Same(t, ctx, svc.PinCodexTurnStateOwner(ctx, account))
				header.Set(openAICodexTurnStateHeader, "client-state")
				require.NoError(t, svc.applyCodexTurnStateForHTTPRequest(ctx, account, "gpt-6-astra", "tok", header))
				require.Equal(t, expected, header.Get(openAICodexTurnStateHeader))
				require.Equal(t, want == CodexTurnStateModeNative, svc.codexTurnStateOwnerCompatible(ctx, account))
				require.Contains(t, account.Extra, CodexTurnStateProbeCacheExtraKey)
				require.Contains(t, account.Extra, openAICodexTicketExtraKey("gpt-6-astra"))
			})
		}
	}
}

func TestCodexTurnStateUnsupportedAccountsAndModes(t *testing.T) {
	for _, mutate := range []func(*Account){
		func(a *Account) { a.Type = AccountTypeAPIKey },
		func(a *Account) { a.Platform = PlatformAnthropic },
		func(a *Account) { id := int64(9); a.ParentAccountID = &id },
		func(a *Account) { a.Extra[CodexTurnStateModeExtraKey] = "invalid" },
	} {
		a := poolTestAccount("plus")
		mutate(a)
		require.Equal(t, CodexTurnStateModeNative, resolveCodexTurnStateMode(a, true))
	}
	a := poolTestAccount("plus")
	a.Type = AccountTypeSetupToken
	require.Equal(t, CodexTurnStateModeStatePool, resolveCodexTurnStateMode(a, false))
	for _, invalid := range []any{true, 1, "", "unknown"} {
		_, err := NormalizeCodexTurnStateProbeExtra(PlatformOpenAI, AccountTypeOAuth, false, map[string]any{CodexTurnStateModeExtraKey: invalid})
		require.Error(t, err)
	}
	extra, err := NormalizeCodexTurnStateProbeExtra(PlatformOpenAI, AccountTypeOAuth, true, map[string]any{CodexTurnStateModeExtraKey: CodexTurnStateModeStatePool})
	require.NoError(t, err)
	require.NotContains(t, extra, CodexTurnStateModeExtraKey)
}

func TestCodexTurnStatePoolFailsClosed(t *testing.T) {
	for _, scenario := range []string{"missing", "wrong_model", "wrong_token", "wrong_plan", "unknown_plan", "expired", "future", "invalid_format", "account_expiry"} {
		t.Run(scenario, func(t *testing.T) {
			svc := ticketTestService(t, config.OpenAICodexTicketConfig{Enabled: true, FailClosed: true}, nil)
			a := poolTestAccount("plus")
			cache := codexTurnStateCacheFromExtra(a.Extra)
			entry := cache["gpt-6-astra"]
			model, token := "gpt-6-astra", "tok"
			switch scenario {
			case "missing":
				cache = nil
			case "wrong_model":
				model = "gpt-5.5"
			case "wrong_token":
				token = "another-account-token"
			case "wrong_plan":
				a.Credentials["plan_type"] = "team"
			case "unknown_plan":
				a.Credentials["plan_type"] = "enterprise"
			case "expired":
				entry.State = poolTestState("plus", time.Now().Add(-2*time.Hour))
			case "future":
				entry.State = poolTestState("plus", time.Now().Add(2*time.Minute))
			case "invalid_format":
				entry.State = fakeCodexTicketState(292)
			case "account_expiry":
				entry.ExpiresAt = time.Now().Add(-time.Second)
			}
			if cache != nil {
				cache["gpt-6-astra"] = entry
			}
			a.Extra[CodexTurnStateProbeCacheExtraKey] = codexTurnStateCacheToExtra(cache)
			header := http.Header{}
			header.Set(openAICodexTurnStateHeader, "untrusted-client-state")
			err := svc.applyCodexTurnStateForHTTPRequest(context.Background(), a, model, token, header)
			require.ErrorIs(t, err, ErrCodexTurnStateUnavailable)
			require.Empty(t, header.Get(openAICodexTurnStateHeader))
			frame, err := applyCodexTurnStateToWSFrame(a, CodexTurnStateModeStatePool, token, []byte(fmt.Sprintf(`{"type":"response.create","model":%q}`, model)))
			require.ErrorIs(t, err, ErrCodexTurnStateUnavailable)
			require.Nil(t, frame)
		})
	}
}

func TestCodexTurnStatePoolWebSocketAndScheduler(t *testing.T) {
	svc := ticketTestService(t, config.OpenAICodexTicketConfig{Enabled: true, FailClosed: true}, nil)
	for _, plan := range []string{"plus", "pro", "team"} {
		a := poolTestAccount(plan)
		delete(a.Extra, openAICodexTicketExtraKey("gpt-6-astra"))
		ctx := svc.PinCodexTurnStateOwner(context.Background(), a)
		require.False(t, svc.isOpenAIAccountRequestRuntimeBlocked(ctx, a, "alias", false), "pool must bypass the legacy gate")
		native := poolTestAccount(plan)
		native.Extra[CodexTurnStateModeExtraKey] = CodexTurnStateModeNative
		require.True(t, svc.isOpenAIAccountRequestRuntimeBlocked(ctx, native, "alias", false), "retry cannot change owner")
		header := http.Header{}
		header.Set(openAICodexTurnStateHeader, "client-state")
		require.NoError(t, svc.applyCodexTurnStateForWSHandshake(ctx, a, "gpt-6-astra", header))
		require.Empty(t, header.Get(openAICodexTurnStateHeader))
		frame := []byte(`{"type":"response.create","model":"gpt-6-astra","client_metadata":{"keep":"value","x-codex-turn-state":"client-state"}}`)
		out, err := applyCodexTurnStateToWSFrame(a, CodexTurnStateModeStatePool, "tok", frame)
		require.NoError(t, err)
		require.Equal(t, codexTurnStateCacheFromExtra(a.Extra)["gpt-6-astra"].State, gjson.GetBytes(out, "client_metadata.x-codex-turn-state").String())
		require.Equal(t, "value", gjson.GetBytes(out, "client_metadata.keep").String())
		for _, owner := range []string{CodexTurnStateModeNative, CodexTurnStateModeLegacyTicket} {
			out, err := applyCodexTurnStateToWSFrame(a, owner, "tok", frame)
			require.NoError(t, err)
			require.Equal(t, frame, out)
		}
		_, err = applyCodexTurnStateToWSFrame(a, CodexTurnStateModeStatePool, "tok", []byte(`{"type":"response.create","model":"gpt-6-astra","client_metadata":[]}`))
		require.ErrorContains(t, err, "client_metadata")
		cancel := []byte(`{"type":"response.cancel"}`)
		out, err = applyCodexTurnStateToWSFrame(a, CodexTurnStateModeStatePool, "tok", cancel)
		require.NoError(t, err)
		require.Equal(t, cancel, out)
	}
}

func TestCodexTurnStateExplicitLegacyHarvestsWithGlobalOff(t *testing.T) {
	account := ticketTestAccount(41)
	account.Status = StatusActive
	account.Extra = map[string]any{CodexTurnStateModeExtraKey: CodexTurnStateModeLegacyTicket}
	upstream := &codexTicketFuncUpstream{do: func(*http.Request) (*http.Response, error) { return codexTicketResponse(), nil }}
	svc := ticketTestService(t, config.OpenAICodexTicketConfig{Enabled: false, HarvestProxyURL: "http://proxy.example.com:8080", Models: []string{"gpt-6-astra"}}, upstream)
	svc.accountRepo = &codexTicketLifecycleRepo{account: *account}
	svc.refreshOpenAICodexTickets(context.Background())
	require.True(t, svc.lookupOpenAICodexTicket(account, "gpt-6-astra").valid(time.Now(), 292))
}

func TestCodexTurnStateWebSocketConnectionsStayWithTheirOwner(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
	pool := newOpenAIWSConnPool(cfg)
	dialer := &openAIWSCountingDialer{}
	pool.setClientDialerForTest(dialer)
	account := poolTestAccount("plus")
	previous := ""
	for _, owner := range []string{CodexTurnStateModeLegacyTicket, CodexTurnStateModeStatePool, CodexTurnStateModeNative, CodexTurnStateModeLegacyTicket} {
		request := openAIWSAcquireRequest{Account: account, WSURL: "wss://example.com/v1/responses", TurnStateOwner: owner}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		lease, err := pool.Acquire(ctx, request)
		cancel()
		require.NoError(t, err)
		require.False(t, lease.Reused())
		require.NotEqual(t, previous, lease.ConnID())
		previous = lease.ConnID()
		lease.Release()
		lease, err = pool.Acquire(context.Background(), request)
		require.NoError(t, err)
		require.True(t, lease.Reused(), "same-engine requests should retain connection reuse")
		lease.Release()
	}
	require.Equal(t, 4, dialer.DialCount())
}

type codexPoolAccountRepo struct {
	AccountRepository
	account *Account
}

func (r *codexPoolAccountRepo) GetByID(context.Context, int64) (*Account, error) {
	return r.account, nil
}

func TestCodexTurnStateImportRetriesBusyAccountAndStops(t *testing.T) {
	for _, stopWhileBusy := range []bool{false, true} {
		t.Run(fmt.Sprintf("stop_busy=%t", stopWhileBusy), func(t *testing.T) {
			a := poolTestAccount("plus")
			svc := NewCodexTurnStateProbeService(&codexPoolAccountRepo{account: a}, nil, nil, nil)
			t.Cleanup(svc.Stop)
			require.True(t, svc.reserveStateAccount(a.ID))
			job, err := svc.QueueImportState(context.Background(), a.ID, "gpt-6-astra", codexTurnStateCacheFromExtra(a.Extra)["gpt-6-astra"].State)
			require.NoError(t, err)
			// Wait through at least one busy retry before releasing the account.
			require.Eventually(t, func() bool {
				current, _ := svc.ImportJob(job.ID)
				return current.Status == "queued" && current.UpdatedAt.After(current.CreatedAt)
			}, time.Second, time.Millisecond)
			if stopWhileBusy {
				svc.Stop()
			} else {
				svc.releaseStateAccount(a.ID)
			}
			require.Eventually(t, func() bool {
				current, _ := svc.ImportJob(job.ID)
				return current.Status == "failed"
			}, 2*time.Second, time.Millisecond)
			current, _ := svc.ImportJob(job.ID)
			if stopWhileBusy {
				require.Equal(t, "import_cancelled", current.Reason)
				_, err = svc.QueueImportState(context.Background(), a.ID, "gpt-6-astra", "state")
				require.ErrorIs(t, err, context.Canceled)
			} else {
				require.Equal(t, "proxy_unavailable", current.Reason)
			}
			svc.importMu.Lock()
			pending := svc.importPending[a.ID]
			svc.importMu.Unlock()
			require.False(t, pending)
		})
	}
}
