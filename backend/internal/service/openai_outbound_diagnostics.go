package service

import (
	"encoding/json"
	"io"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	openai "github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	openaiOutboundDiagFailTotal atomic.Int64
	openaiCapacityShedCounts    sync.Map // key: "code|0/1" -> *atomic.Int64
)

func IncOpenAICodexCapacityShed(code string, pluginRouted bool) {
	code = strings.TrimSpace(code)
	if code == "" {
		code = "server_is_overloaded"
	}
	routed := "0"
	if pluginRouted {
		routed = "1"
	}
	key := code + "|" + routed
	actual, _ := openaiCapacityShedCounts.LoadOrStore(key, &atomic.Int64{})
	actual.(*atomic.Int64).Add(1)
}

func OpenAICodexCapacityShedCount(code string, pluginRouted bool) int64 {
	routed := "0"
	if pluginRouted {
		routed = "1"
	}
	v, ok := openaiCapacityShedCounts.Load(code + "|" + routed)
	if !ok {
		return 0
	}
	return v.(*atomic.Int64).Load()
}

func OpenAICodexOutboundDiagFailTotal() int64 {
	return openaiOutboundDiagFailTotal.Load()
}

type openaiOutboundDiagRecord struct {
	Kind                 string            `json:"kind"`
	TS                   string            `json:"ts"`
	CorrelationID        string            `json:"correlation_id"`
	Transport            string            `json:"transport"`
	AccountID            int64             `json:"account_id"`
	PluginRouted         bool              `json:"plugin_routed"`
	FinalModel           string            `json:"final_model,omitempty"`
	FinalServiceTier     string            `json:"final_service_tier,omitempty"`
	ExpectedHint         string            `json:"expected_hint,omitempty"`
	ObservedHint         string            `json:"observed_hint,omitempty"`
	InboundHintPresent   bool              `json:"inbound_hint_present"`
	Identity             map[string]string `json:"identity,omitempty"`
	Headers              map[string]string `json:"headers,omitempty"`
	SessionPresent       bool              `json:"session_id_present"`
	ConversationPresent  bool              `json:"conversation_id_present"`
	WindowPresent        bool              `json:"x_codex_window_id_present"`
	ChatGPTAccountPresent bool             `json:"chatgpt_account_id_present"`
	Assertions           map[string]bool   `json:"assertions"`
}

var outboundDiagHeaderAllowlist = map[string]bool{
	"user-agent":            true,
	"originator":            true,
	"version":               true,
	"openai-beta":           true,
	"x-codex-routing-hint":  true,
}

var outboundDiagAlwaysRedact = map[string]bool{
	"authorization":         true,
	"proxy-authorization":   true,
	"cookie":                true,
	"set-cookie":            true,
	"x-api-key":             true,
	"x-goog-api-key":        true,
}

func expectedOpenAICodexRoutingHint(account *Account, model, serviceTier string) string {
	headers := make(http.Header)
	setOpenAICodexRoutingHint(headers, account, model, serviceTier)
	return strings.TrimSpace(headers.Get(openAICodexRoutingHintHeader))
}

func inboundOpenAICodexRoutingHintPresent(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	for key := range c.Request.Header {
		if strings.EqualFold(strings.TrimSpace(key), openAICodexRoutingHintHeader) {
			return true
		}
	}
	return false
}

func evaluateOpenAIOutboundAssertions(account *Account, outbound http.Header, model, serviceTier string, inboundHint bool) map[string]bool {
	expected := expectedOpenAICodexRoutingHint(account, model, serviceTier)
	observed := ""
	if outbound != nil {
		observed = strings.TrimSpace(outbound.Get(openAICodexRoutingHintHeader))
	}
	out := map[string]bool{
		"hint_matches_formula":      expected == observed,
		"hint_allowlist_excludes":   !openaiAllowedHeaders[openAICodexRoutingHintHeader] && !openaiPassthroughAllowedHeaders[openAICodexRoutingHintHeader],
		"hint_rewritten_if_inbound": !inboundHint || expected == observed,
		"identity_triple":           identityTripleOK(outbound),
		"hint_no_region":            hintHasNoRegion(observed),
	}
	return out
}

func identityTripleOK(headers http.Header) bool {
	if headers == nil {
		return true
	}
	originator := strings.TrimSpace(headers.Get("originator"))
	if originator == "" {
		return true
	}
	ua := strings.TrimSpace(headers.Get("User-Agent"))
	if ua == "" {
		ua = strings.TrimSpace(headers.Get("user-agent"))
	}
	gotOriginator, _, ok := openai.PairCodexClientIdentity(ua)
	if !ok || !strings.EqualFold(gotOriginator, originator) {
		return false
	}
	version := strings.TrimSpace(headers.Get("version"))
	uaVersion := extractCodexUAVersion(ua)
	if version != "" && uaVersion != "" && version != uaVersion {
		return false
	}
	if version != "" && CompareCodexVersions(version, codexUpstreamMinVersion) < 0 {
		return false
	}
	return true
}

func extractCodexUAVersion(ua string) string {
	ua = strings.TrimSpace(ua)
	slash := strings.IndexByte(ua, '/')
	if slash < 0 || slash+1 >= len(ua) {
		return ""
	}
	rest := ua[slash+1:]
	if i := strings.IndexByte(rest, ' '); i >= 0 {
		rest = rest[:i]
	}
	return NormalizeCodexClientVersion(rest)
}

func CompareCodexVersions(a, b string) int {
	ap, bp := parseCodexSemver(a), parseCodexSemver(b)
	for i := 0; i < 3; i++ {
		if ap[i] < bp[i] {
			return -1
		}
		if ap[i] > bp[i] {
			return 1
		}
	}
	return 0
}

func parseCodexSemver(v string) [3]int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexByte(v, '-'); i >= 0 {
		v = v[:i]
	}
	var out [3]int
	parts := strings.Split(v, ".")
	for i := 0; i < len(parts) && i < 3; i++ {
		n := 0
		for _, c := range parts[i] {
			if c < '0' || c > '9' {
				break
			}
			n = n*10 + int(c-'0')
		}
		out[i] = n
	}
	return out
}

func hintHasNoRegion(hint string) bool {
	if hint == "" {
		return true
	}
	lower := strings.ToLower(hint)
	if strings.Contains(lower, "region") || strings.Contains(lower, "europe") {
		return false
	}
	for _, part := range strings.Split(hint, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key := part
		if i := strings.IndexByte(part, '='); i >= 0 {
			key = strings.TrimSpace(part[:i])
		}
		switch strings.ToLower(key) {
		case "model", "tier":
		default:
			return false
		}
	}
	return true
}

func copyOutboundDiagHeaders(headers http.Header) map[string]string {
	out := make(map[string]string)
	if headers == nil {
		return out
	}
	for key, values := range headers {
		lower := strings.ToLower(strings.TrimSpace(key))
		if outboundDiagAlwaysRedact[lower] {
			out[lower] = "[redacted]"
			continue
		}
		if !outboundDiagHeaderAllowlist[lower] {
			continue
		}
		if len(values) > 0 {
			out[lower] = values[0]
		}
	}
	return out
}

func outboundDiagContainsSecrets(raw []byte) bool {
	s := strings.ToLower(string(raw))
	if strings.Contains(s, "bearer ") || strings.Contains(s, "session=") || strings.Contains(s, "user:pass@") {
		return true
	}
	if i := strings.Index(s, "://"); i >= 0 {
		rest := s[i+3:]
		if at := strings.IndexByte(rest, '@'); at > 0 && strings.Contains(rest[:at], ":") {
			return true
		}
	}
	return false
}

type openaiOutboundDiagWriter struct {
	enabled    bool
	sampleRate float64
	intended   *lumberjack.Logger
	observed   *lumberjack.Logger
}

func newOpenAIOutboundDiagWriter(cfg *config.Config) *openaiOutboundDiagWriter {
	if cfg == nil || !cfg.Gateway.OpenAICodexOutboundDiagnostics.Enabled {
		return nil
	}
	dir, err := resolveOpenAIOutboundDiagDir(cfg)
	if err != nil {
		logger.L().Warn("openai outbound diagnostics disabled", zap.String("component", "service.openai_outbound_diag"), zap.Error(err))
		return nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		logger.L().Warn("openai outbound diagnostics mkdir failed", zap.String("component", "service.openai_outbound_diag"), zap.Error(err))
		return nil
	}
	maxMB := cfg.Gateway.OpenAICodexOutboundDiagnostics.MaxSizeMB
	if maxMB <= 0 {
		maxMB = 32
	}
	backups := cfg.Gateway.OpenAICodexOutboundDiagnostics.MaxBackups
	if backups <= 0 {
		backups = 3
	}
	age := cfg.Gateway.OpenAICodexOutboundDiagnostics.MaxAgeDays
	if age <= 0 {
		age = 3
	}
	rate := cfg.Gateway.OpenAICodexOutboundDiagnostics.SampleRate
	if rate <= 0 {
		rate = 1
	}
	return &openaiOutboundDiagWriter{
		enabled:    true,
		sampleRate: rate,
		intended:   &lumberjack.Logger{Filename: filepath.Join(dir, "intended.jsonl"), MaxSize: maxMB, MaxBackups: backups, MaxAge: age},
		observed:   &lumberjack.Logger{Filename: filepath.Join(dir, "observed.jsonl"), MaxSize: maxMB, MaxBackups: backups, MaxAge: age},
	}
}

func resolveOpenAIOutboundDiagDir(cfg *config.Config) (string, error) {
	dataRoot := strings.TrimSpace(os.Getenv("DATA_DIR"))
	if dataRoot == "" {
		dataRoot = "data"
	}
	absRoot, err := filepath.Abs(dataRoot)
	if err != nil {
		return "", err
	}
	dir := ""
	if cfg != nil {
		dir = strings.TrimSpace(cfg.Gateway.OpenAICodexOutboundDiagnostics.Dir)
	}
	if dir == "" {
		dir = filepath.Join(absRoot, "logs", "codex-outbound")
	} else if !filepath.IsAbs(dir) {
		dir = filepath.Join(absRoot, dir)
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(absRoot, absDir)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", err
	}
	return absDir, nil
}

func (s *OpenAIGatewayService) recordOpenAIOutboundDiagnostics(c *gin.Context, account *Account, headers http.Header, model, serviceTier, transport string) {
	defer func() { _ = recover() }()
	if s == nil || account == nil || !account.UsesOpenAICodexProtocol() {
		return
	}
	writer := s.ensureOpenAIOutboundDiag()
	if writer == nil || !writer.enabled {
		return
	}
	if writer.sampleRate < 1 && rand.Float64() > writer.sampleRate {
		return
	}
	if c != nil {
		if v, ok := c.Get("openai_ws_http_bridge"); ok {
			if bridged, _ := v.(bool); bridged && transport != "http_bridge" {
				transport = "http_bridge"
			}
		}
	}
	inbound := inboundOpenAICodexRoutingHintPresent(c)
	expected := expectedOpenAICodexRoutingHint(account, model, serviceTier)
	observedHint := ""
	if headers != nil {
		observedHint = strings.TrimSpace(headers.Get(openAICodexRoutingHintHeader))
	}
	assertions := evaluateOpenAIOutboundAssertions(account, headers, model, serviceTier, inbound)
	failed := false
	for _, ok := range assertions {
		if !ok {
			failed = true
			break
		}
	}
	if failed {
		openaiOutboundDiagFailTotal.Add(1)
		logger.L().Warn("openai outbound header self-check failed",
			zap.String("component", "service.openai_outbound_diag"),
			zap.Any("assertions", assertions),
		)
	}
	corr := uuid.NewString()
	pluginRouted := s.pluginManager != nil && s.pluginManager.ShouldRouteOpenAIOAuth(account)
	identity := map[string]string{}
	if headers != nil {
		if ua := strings.TrimSpace(headers.Get("User-Agent")); ua != "" {
			identity["user-agent"] = ua
		}
		if o := strings.TrimSpace(headers.Get("originator")); o != "" {
			identity["originator"] = o
		}
		if v := strings.TrimSpace(headers.Get("version")); v != "" {
			identity["version"] = v
		}
	}
	base := openaiOutboundDiagRecord{
		TS:                    time.Now().UTC().Format(time.RFC3339Nano),
		CorrelationID:         corr,
		Transport:             transport,
		AccountID:             account.ID,
		PluginRouted:          pluginRouted,
		FinalModel:            strings.TrimSpace(model),
		FinalServiceTier:      normalizedOpenAIServiceTierValue(serviceTier),
		ExpectedHint:          expected,
		ObservedHint:          observedHint,
		InboundHintPresent:    inbound,
		Identity:              identity,
		SessionPresent:        headerPresentFold(headers, "session_id"),
		ConversationPresent:   headerPresentFold(headers, "conversation_id"),
		WindowPresent:         headerPresentFold(headers, "x-codex-window-id"),
		ChatGPTAccountPresent: headerPresentFold(headers, "chatgpt-account-id"),
		Assertions:            assertions,
	}
	intended := base
	intended.Kind = "intended"
	observed := base
	observed.Kind = "observed"
	observed.Headers = copyOutboundDiagHeaders(headers)
	writeDiagLine(writer.intended, intended)
	writeDiagLine(writer.observed, observed)
}

func headerPresentFold(headers http.Header, name string) bool {
	if headers == nil {
		return false
	}
	for key := range headers {
		if strings.EqualFold(key, name) && strings.TrimSpace(headers.Get(key)) != "" {
			return true
		}
	}
	return false
}

func writeDiagLine(w io.Writer, rec openaiOutboundDiagRecord) {
	if w == nil {
		return
	}
	raw, err := json.Marshal(rec)
	if err != nil || outboundDiagContainsSecrets(raw) {
		return
	}
	raw = append(raw, '\n')
	_, _ = w.Write(raw)
}

func (s *OpenAIGatewayService) ensureOpenAIOutboundDiag() *openaiOutboundDiagWriter {
	if s == nil {
		return nil
	}
	s.openaiOutboundDiagOnce.Do(func() {
		s.openaiOutboundDiag = newOpenAIOutboundDiagWriter(s.cfg)
	})
	return s.openaiOutboundDiag
}
