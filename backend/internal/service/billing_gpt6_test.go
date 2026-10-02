package service

import (
	"os"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestGPT6ModelPricingAcrossSourcesAndLongContext(t *testing.T) {
	data, err := os.ReadFile("../../resources/model-pricing/model_prices_and_context_window.json")
	require.NoError(t, err)
	catalog := &PricingService{}
	catalog.pricingData, err = catalog.parsePricingData(data)
	require.NoError(t, err)

	sources := map[string]*BillingService{
		"billing fallback": NewBillingService(&config.Config{}, nil),
		"pricing fallback": NewBillingService(&config.Config{}, &PricingService{}),
		"catalog":          NewBillingService(&config.Config{}, catalog),
	}
	models := []struct {
		model                      string
		input, output, write, read float64
	}{
		{"gpt-6.1-sol", 2e-6, 10e-6, 2.5e-6, 0.1e-6},
		{"gpt-6-sol", 2e-6, 10e-6, 2.5e-6, 0.2e-6},
		{"gpt-6-luna", 0.1e-6, 0.5e-6, 0.125e-6, 0.01e-6},
	}

	for source, svc := range sources {
		for _, tc := range models {
			t.Run(source+"/"+tc.model, func(t *testing.T) {
				for _, totalInput := range []int{271_999, 272_000, 272_001} {
					for tier, tierMultiplier := range map[string]float64{"": 1, "priority": 2, "fast": 2, "flex": 0.5} {
						tokens := UsageTokens{InputTokens: totalInput - 3000, CacheReadTokens: 2000, CacheCreationTokens: 1000, OutputTokens: 500}
						cost, err := svc.CalculateCostWithServiceTier(tc.model, tokens, 1, tier)
						require.NoError(t, err)
						inputMultiplier, outputMultiplier := 1.0, 1.0
						if totalInput > 272_000 {
							inputMultiplier, outputMultiplier = 2, 1.5
						}
						require.InDelta(t, float64(tokens.InputTokens)*tc.input*inputMultiplier*tierMultiplier, cost.InputCost, 1e-10)
						require.InDelta(t, 1000*tc.write*inputMultiplier*tierMultiplier, cost.CacheCreationCost, 1e-10)
						require.InDelta(t, 2000*tc.read*inputMultiplier*tierMultiplier, cost.CacheReadCost, 1e-10)
						require.InDelta(t, 500*tc.output*outputMultiplier*tierMultiplier, cost.OutputCost, 1e-10)
						require.Equal(t, totalInput > 272_000, cost.LongContextBillingApplied)
					}
				}
			})
		}
	}
}

func TestGPT6PricingAliasesUseDedicatedFallbacks(t *testing.T) {
	svc := &PricingService{}
	for model, expected := range map[string]*LiteLLMModelPricing{
		"gpt-6.1-sol-max":                  openAIGPT61SolFallbackPricing,
		"openai/gpt-6-sol-openai-compact":  openAIGPT6SolFallbackPricing,
		"openai/gpt-6-luna-openai-compact": openAIGPT6LunaFallbackPricing,
	} {
		require.Same(t, expected, svc.GetModelPricing(model), model)
	}
}

func TestAstraUltrafastPricingUsesSixTimesStandard(t *testing.T) {
	data, err := os.ReadFile("../../resources/model-pricing/model_prices_and_context_window.json")
	require.NoError(t, err)
	catalog := &PricingService{}
	catalog.pricingData, err = catalog.parsePricingData(data)
	require.NoError(t, err)

	for _, svc := range []*BillingService{NewBillingService(&config.Config{}, nil), NewBillingService(&config.Config{}, &PricingService{}), NewBillingService(&config.Config{}, catalog)} {
		for _, model := range []string{"gpt-6-astra", "gpt-6", "openai/gpt-6-astra"} {
			cost, err := svc.CalculateCostWithServiceTier(model, UsageTokens{InputTokens: 1000, OutputTokens: 1000, CacheReadTokens: 1000, CacheCreationTokens: 1000}, 1, "ultrafast")
			require.NoError(t, err)
			require.InDelta(t, (10e-6+50e-6+1e-6+12.5e-6)*6000, cost.TotalCost, 1e-10)
		}
	}

	custom := 1e-6
	fast := 3.0
	pricing, err := NewBillingService(&config.Config{}, nil).GetModelPricingWithChannel("gpt-6-astra", &ChannelModelPricing{
		InputPrice: &custom, OutputPrice: &custom, CacheWritePrice: &custom, CacheReadPrice: &custom, FastMultiplier: &fast,
	})
	require.NoError(t, err)
	require.Equal(t, 6.0, configuredServiceTierMultiplier("ultrafast", pricing))
	require.Equal(t, 3.0, configuredServiceTierMultiplier("priority", pricing))
}

func TestGPT6ExplicitZeroCacheWriteIsPreservedAcrossTiers(t *testing.T) {
	pricing := &PricingService{}
	var err error
	pricing.pricingData, err = pricing.parsePricingData([]byte(`{
		"gpt-6.1-sol":{"litellm_provider":"openai","input_cost_per_token":0.000002,"output_cost_per_token":0.00001,"cache_creation_input_token_cost":0},
		"gpt-6-sol":{"litellm_provider":"openai","input_cost_per_token":0.000002,"output_cost_per_token":0.00001,"cache_creation_input_token_cost":0},
		"gpt-6-luna":{"litellm_provider":"openai","input_cost_per_token":0.0000001,"output_cost_per_token":0.0000005,"cache_creation_input_token_cost":0}
	}`))
	require.NoError(t, err)
	billing := NewBillingService(&config.Config{}, pricing)
	for _, model := range []string{"gpt-6.1-sol", "gpt-6-sol", "gpt-6-luna"} {
		for _, tier := range []string{"", "fast", "priority", "flex"} {
			cost, err := billing.CalculateCostWithServiceTier(model, UsageTokens{CacheCreationTokens: 300_000}, 1, tier)
			require.NoError(t, err)
			require.Zero(t, cost.CacheCreationCost, model+"/"+tier)
		}
	}
}
