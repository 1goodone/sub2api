package service

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestHintAllowlistExcludesRoutingHint(t *testing.T) {
	if openaiAllowedHeaders[openAICodexRoutingHintHeader] || openaiPassthroughAllowedHeaders[openAICodexRoutingHintHeader] {
		t.Fatal("routing-hint must stay off the client allowlists")
	}
}

func TestEvaluateOpenAIOutboundAssertions(t *testing.T) {
	account := &Account{ID: 7, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	headers := make(http.Header)
	setOpenAICodexRoutingHint(headers, account, "gpt-5.4", "priority")
	enforceCodexIdentityHeaders(headers)
	got := evaluateOpenAIOutboundAssertions(account, headers, "gpt-5.4", "priority", true)
	for id, ok := range got {
		if !ok {
			t.Fatalf("%s failed: %+v hint=%q", id, got, headers.Get(openAICodexRoutingHintHeader))
		}
	}
}

func TestHintHasNoRegionRejectsEurope(t *testing.T) {
	if hintHasNoRegion("model=gpt-5.4;region=eu") || hintHasNoRegion("model=gpt-5.4;europe") {
		t.Fatal("region/europe must fail")
	}
	if !hintHasNoRegion("model=gpt-5.4;tier=priority") {
		t.Fatal("valid hint failed")
	}
}

func TestCopyOutboundDiagHeadersRedactsSecrets(t *testing.T) {
	h := make(http.Header)
	h.Set("Authorization", "Bearer secret-token")
	h.Set("Cookie", "session=abc")
	h.Set("Proxy-Authorization", "Basic abc")
	h.Set("User-Agent", "codex_cli_rs/0.146.0")
	h.Set(openAICodexRoutingHintHeader, "model=gpt-5.4")
	copied := copyOutboundDiagHeaders(h)
	raw, err := json.Marshal(copied)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "secret-token") || strings.Contains(string(raw), "session=abc") {
		t.Fatalf("secrets leaked: %s", raw)
	}
	if outboundDiagContainsSecrets(raw) {
		t.Fatalf("secret scanner: %s", raw)
	}
	if copied["user-agent"] == "" || copied["x-codex-routing-hint"] == "" {
		t.Fatalf("allowlist headers missing: %+v", copied)
	}
	if copied["authorization"] != "[redacted]" {
		t.Fatalf("authorization: %+v", copied)
	}
}

func TestOutboundDiagWriterOmitsProxyUserinfo(t *testing.T) {
	var buf bytes.Buffer
	rec := openaiOutboundDiagRecord{Kind: "observed", Headers: map[string]string{"user-agent": "codex_cli_rs/0.146.0"}}
	writeDiagLine(&buf, rec)
	if outboundDiagContainsSecrets(buf.Bytes()) {
		t.Fatal(buf.String())
	}
	if !outboundDiagContainsSecrets([]byte(`{"url":"http://user:pass@proxy"}`)) {
		t.Fatal("userinfo must be detected")
	}
}

func TestInboundRoutingHintPresent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set(openAICodexRoutingHintHeader, "model=caller-secret")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req
	if !inboundOpenAICodexRoutingHintPresent(c) {
		t.Fatal("expected inbound hint")
	}
}
