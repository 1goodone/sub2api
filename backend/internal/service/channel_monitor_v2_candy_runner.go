package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

const channelMonitorV2CandyMaxAttempts = 3

var errChannelMonitorV2CandyAccountBusy = errors.New("every routed account stayed at its concurrency limit")

type channelMonitorV2CandyRunner interface {
	GetGroup(context.Context, int64) (*Group, error)
	Run(context.Context, *Group, ChannelMonitorV2CandyProbe) (*ScheduledTestResult, error)
}

type gatewayChannelMonitorV2CandyRunner struct {
	groups      GroupRepository
	gateway     *GatewayService
	openai      *OpenAIGatewayService
	concurrency *ConcurrencyService
	runAccount  func(context.Context, int64, string, *PelicanTestConfig) (*ScheduledTestResult, error)
	slotWait    time.Duration
}

func newGatewayChannelMonitorV2CandyRunner(groups GroupRepository, gateway *GatewayService, openai *OpenAIGatewayService, concurrency *ConcurrencyService, accountTest *AccountTestService) channelMonitorV2CandyRunner {
	if groups == nil || gateway == nil || accountTest == nil {
		return nil
	}
	return &gatewayChannelMonitorV2CandyRunner{groups: groups, gateway: gateway, openai: openai, concurrency: concurrency, runAccount: accountTest.RunPelicanBackground, slotWait: time.Minute}
}

func (r *gatewayChannelMonitorV2CandyRunner) GetGroup(ctx context.Context, id int64) (*Group, error) {
	return r.groups.GetByID(ctx, id)
}

func (r *gatewayChannelMonitorV2CandyRunner) Run(ctx context.Context, group *Group, probe ChannelMonitorV2CandyProbe) (*ScheduledTestResult, error) {
	excluded := map[int64]struct{}{}
	var last *ScheduledTestResult
	for attempt := 0; attempt < channelMonitorV2CandyMaxAttempts; attempt++ {
		route, err := r.route(ctx, group, probe.Model, excluded)
		if err != nil {
			if last != nil {
				return last, nil
			}
			return nil, err
		}
		var sample *ScheduledTestResult
		func() {
			defer route.release()
			sample, err = r.runAccount(ctx, route.account.ID, route.model, &PelicanTestConfig{
				QuestionKind: "candy", Prompt: CandyPrompt, ReasoningEffort: probe.ReasoningEffort, ParallelCount: 1, ModelID: probe.Model,
			})
		}()
		if err != nil || sample == nil {
			sample = &ScheduledTestResult{Status: "failed", ErrorMessage: fmt.Sprint(err)}
			if err == nil {
				sample.ErrorMessage = "candy probe returned no result"
			}
		}
		last = sample
		if !channelMonitorV2CandyFailedBeforeOutput(sample) {
			return sample, nil
		}
		excluded[route.account.ID] = struct{}{}
	}
	return last, nil
}

type channelMonitorV2CandyRoute struct {
	account *Account
	model   string
	release func()
}

func (r *gatewayChannelMonitorV2CandyRunner) route(ctx context.Context, group *Group, model string, excluded map[int64]struct{}) (*channelMonitorV2CandyRoute, error) {
	platform, upstreamModel := group.Platform, model
	if platform == PlatformComposite {
		decision, matched, err := r.gateway.resolveCompositeRouteDecision(ctx, group, model, CompositeRouteEndpointAny)
		if err != nil {
			return nil, err
		}
		if !matched {
			return nil, fmt.Errorf("%w supporting model: %s (composite target platform unknown)", ErrNoAvailableAccounts, model)
		}
		ctx = WithCompositeRouteDecision(ctx, decision)
		platform = decision.TargetPlatform
		if rewritten := strings.TrimSpace(decision.UpstreamModel); rewritten != "" {
			upstreamModel = rewritten
		}
	}
	var selection *AccountSelectionResult
	var err error
	if isOpenAIResponsesGatewayPlatform(platform) {
		if r.openai == nil {
			return nil, ErrNoAvailableAccounts
		}
		selection, _, err = r.openai.SelectAccountWithSchedulerForCapability(ctx, &group.ID, "", "", upstreamModel, excluded,
			OpenAIUpstreamTransportAny, OpenAIEndpointCapabilityChatCompletions, false, false, true, NormalizeOpenAICompatiblePlatform(platform))
	} else {
		selection, err = r.gateway.SelectAccountWithLoadAwareness(ctx, &group.ID, "", upstreamModel, excluded, "", 0)
	}
	if err != nil {
		return nil, err
	}
	if selection == nil || selection.Account == nil {
		return nil, ErrNoAvailableAccounts
	}
	release, err := r.holdSlot(ctx, selection)
	if err != nil {
		return nil, err
	}
	return &channelMonitorV2CandyRoute{account: selection.Account, model: upstreamModel, release: release}, nil
}

func (r *gatewayChannelMonitorV2CandyRunner) holdSlot(ctx context.Context, selection *AccountSelectionResult) (func(), error) {
	if selection.Acquired || selection.WaitPlan == nil || r.concurrency == nil {
		return channelMonitorV2CandyReleaseOnce(selection.ReleaseFunc), nil
	}
	plan := selection.WaitPlan
	wait := r.slotWait
	if plan.Timeout > 0 && plan.Timeout < wait {
		wait = plan.Timeout
	}
	deadline := time.Now().Add(wait)
	for {
		acquired, err := r.concurrency.AcquireAccountSlot(ctx, plan.AccountID, plan.MaxConcurrency)
		if err == nil && acquired != nil && acquired.Acquired {
			return channelMonitorV2CandyReleaseOnce(acquired.ReleaseFunc), nil
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, errChannelMonitorV2CandyAccountBusy
		}
		if remaining > 500*time.Millisecond {
			remaining = 500 * time.Millisecond
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(remaining):
		}
	}
}

func channelMonitorV2CandyReleaseOnce(release func()) func() {
	if release == nil {
		return func() {}
	}
	var once sync.Once
	return func() { once.Do(release) }
}

func channelMonitorV2CandyFailedBeforeOutput(result *ScheduledTestResult) bool {
	if result == nil || result.Status == "success" || result.ResponseText != "" {
		return false
	}
	switch result.ErrorMessage {
	case "Model returned empty output", "Response exceeds 4 MiB capture limit", "Output exceeds 2 MiB history limit":
		return false
	default:
		return true
	}
}

func isOpenAIResponsesGatewayPlatform(platform string) bool {
	switch platform {
	case PlatformOpenAI, PlatformGrok, PlatformKimi, PlatformZhipu, PlatformDeepseek, PlatformMiniMax, PlatformOpenCodeGo:
		return true
	default:
		return false
	}
}
