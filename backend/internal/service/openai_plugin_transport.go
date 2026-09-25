package service

import (
	"net/http"
	"strings"
)

func (s *OpenAIGatewayService) SetPluginManager(manager *PluginManager) {
	s.pluginManager = manager
}

// doOpenAIUpstream 只在 OpenAI OAuth 能力绑定已启用时把真实请求交给插件。
// 插件返回标准 http.Response，响应解析、错误映射、SSE 和计费仍由现有核心链处理。
func (s *OpenAIGatewayService) doOpenAIUpstream(request *http.Request, proxyURL string, account *Account) (result *http.Response, resultErr error) {
	request = s.stickBoundCodexTicketRequest(request, account)
	boundTicket := s.boundCodexTicket(request, account)
	releaseChat := func() {}
	if boundTicket != nil {
		releaseChat = s.holdCodexTicketChat(account)
	}
	proxyURL, release, err := s.pinCodexTicketEgress(request, account, proxyURL)
	if err != nil {
		releaseChat()
		return nil, err
	}
	defer func() {
		result, resultErr = attachCodexTicketEgressRelease(result, resultErr, func() {
			release()
			releaseChat()
		})
		if resultErr == nil {
			s.observeCodexTicketResponse(request, result, account)
		}
	}()
	if s.codexTurnStateOwner(request.Context(), account) == CodexTurnStateModeStatePool {
		current, stateErr := s.currentCodexTurnStateAccount(request.Context(), account)
		if stateErr != nil {
			return nil, stateErr
		}
		model, _ := request.Context().Value(codexTurnStateModelContextKey{}).(string)
		token := strings.TrimSpace(strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer "))
		if stateErr := s.applyCodexTurnStateForHTTPRequest(request.Context(), current, model, token, request.Header); stateErr != nil {
			return nil, stateErr
		}
		observation := s.codexTurnStateObservation(request.Context(), current, model)
		defer func() { observeCodexTurnStateHTTPResponse(result, observation) }()
	}
	if s.pluginManager != nil {
		response, handled, err := s.pluginManager.RoundTripOpenAIOAuth(request.Context(), request, proxyURL, account)
		if handled {
			return response, err
		}
	}
	return s.httpUpstream.Do(request, proxyURL, account.ID, account.Concurrency)
}

// doOpenAIAccountTestUpstream 让 OpenAI OAuth 账号测试与真实转发使用同一插件路径。
// API Key 和未命中插件的账号保持各自原有的 HTTPUpstream 行为。
func (s *AccountTestService) doOpenAIAccountTestUpstream(
	request *http.Request,
	proxyURL string,
	account *Account,
	useTLSFallback bool,
) (*http.Response, error) {
	if s.pluginManager != nil {
		response, handled, err := s.pluginManager.RoundTripOpenAIOAuth(request.Context(), request, proxyURL, account)
		if handled {
			return response, err
		}
	}
	if useTLSFallback {
		return s.httpUpstream.DoWithTLS(
			request,
			proxyURL,
			account.ID,
			account.Concurrency,
			s.tlsFPProfileService.ResolveTLSProfile(account),
		)
	}
	return s.httpUpstream.Do(request, proxyURL, account.ID, account.Concurrency)
}
