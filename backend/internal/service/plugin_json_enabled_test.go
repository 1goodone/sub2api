package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

func TestAccountInOpenAIOAuthAllowlist(t *testing.T) {
	if !accountInOpenAIOAuthAllowlist(nil, 1) {
		t.Fatal("nil config allows all")
	}
	cfg := &config.Config{}
	if !accountInOpenAIOAuthAllowlist(cfg, 1) {
		t.Fatal("empty allowlist allows all")
	}
	cfg.Plugins.OpenAIOAuthAccountAllowlist = []int64{2, 3}
	if accountInOpenAIOAuthAllowlist(cfg, 1) || !accountInOpenAIOAuthAllowlist(cfg, 2) {
		t.Fatal("allowlist intersection")
	}
}

func TestPluginJSONEnabled(t *testing.T) {
	if pluginJSONEnabled(nil) || pluginJSONEnabled([]byte(`{}`)) || pluginJSONEnabled([]byte(`{"enabled":false}`)) {
		t.Fatal("expected disabled")
	}
	if !pluginJSONEnabled([]byte(`{"enabled":true}`)) {
		t.Fatal("expected enabled")
	}
}
