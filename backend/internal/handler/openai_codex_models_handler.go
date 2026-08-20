package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// CodexModels serves the Codex models manifest for Codex clients.
//
// Codex CLI and the Codex desktop app refresh their model picker from
// GET {base_url}/models?client_version=... (custom provider mode) or
// GET /backend-api/codex/models (chatgpt_base_url mode). Both routes land
// here. ChatGPT manifests are proxied verbatim; custom API key manifests receive
// provider-compatibility normalization and use a short-lived, asynchronously
// revalidated cache to tolerate canceled client requests.
func (h *OpenAIGatewayHandler) CodexModels(c *gin.Context) {
	if c.Request.Context().Err() != nil {
		return
	}
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey.Group == nil {
		h.errorResponse(c, http.StatusUnauthorized, "invalid_request_error", "API key group is required")
		return
	}
	if apiKey.Group.Platform != service.PlatformOpenAI && apiKey.Group.Platform != service.PlatformComposite {
		h.errorResponse(c, http.StatusNotFound, "not_found_error", "Codex models manifest is only available for OpenAI and Composite groups")
		return
	}

	maxAccountSwitches := h.maxAccountSwitches
	if maxAccountSwitches <= 0 {
		maxAccountSwitches = 3
	}
	failedAccountIDs := make(map[int64]struct{})
	switchCount := 0
	var lastUpstreamErr error
	ifNoneMatch := c.GetHeader("If-None-Match")
	upstreamIfNoneMatch := ifNoneMatch
	filterDisabled := len(apiKey.Group.DisabledModels) > 0
	if filterDisabled {
		upstreamIfNoneMatch = ""
	}

	for {
		account, err := h.gatewayService.SelectAccountForModelWithExclusions(c.Request.Context(), apiKey.GroupID, "", "", failedAccountIDs)
		if err != nil {
			if c.Request.Context().Err() != nil {
				return
			}
			if lastUpstreamErr != nil {
				h.errorResponse(c, infraerrors.Code(lastUpstreamErr), "upstream_error", infraerrors.Message(lastUpstreamErr))
				return
			}
			h.errorResponse(c, http.StatusServiceUnavailable, "upstream_error", "No available OpenAI accounts")
			return
		}
		// 让 ops 错误日志携带实际选中的上游账号，便于定位失效账号（#4544）。
		setOpsSelectedAccount(c, account.ID, account.Platform)

		manifest, err := h.gatewayService.FetchCodexModelsManifest(c.Request.Context(), account, c.Query("client_version"), upstreamIfNoneMatch)
		if err != nil {
			if c.Request.Context().Err() != nil {
				return
			}
			if service.IsRetryableCodexModelsManifestError(err) && switchCount < maxAccountSwitches {
				failedAccountIDs[account.ID] = struct{}{}
				switchCount++
				lastUpstreamErr = err
				continue
			}
			h.errorResponse(c, infraerrors.Code(err), "upstream_error", infraerrors.Message(err))
			return
		}
		if c.Request.Context().Err() != nil {
			return
		}

		if manifest.NotModified {
			if manifest.ETag != "" {
				c.Header("ETag", manifest.ETag)
			}
			c.Status(http.StatusNotModified)
			return
		}
		body := manifest.Body
		etag := manifest.ETag
		if filterDisabled {
			body, err = filterCodexModelsManifest(body, apiKey.Group)
			if err != nil {
				h.errorResponse(c, http.StatusBadGateway, "upstream_error", "Invalid Codex models manifest")
				return
			}
			etag = codexFilteredModelsETag(body)
			if codexETagMatches(ifNoneMatch, etag) {
				c.Header("ETag", etag)
				c.Status(http.StatusNotModified)
				return
			}
		}
		if etag != "" {
			c.Header("ETag", etag)
		}
		c.Data(http.StatusOK, "application/json", body)
		return
	}
}

func filterCodexModelsManifest(body []byte, group *service.Group) ([]byte, error) {
	if group == nil || len(group.DisabledModels) == 0 {
		return body, nil
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, err
	}
	var models []json.RawMessage
	if err := json.Unmarshal(envelope["models"], &models); err != nil {
		return nil, err
	}
	filtered := make([]json.RawMessage, 0, len(models))
	for _, rawModel := range models {
		var model struct {
			Slug string `json:"slug"`
		}
		if err := json.Unmarshal(rawModel, &model); err != nil || !group.IsModelDisabled(model.Slug) {
			filtered = append(filtered, rawModel)
		}
	}
	if len(filtered) == len(models) {
		return body, nil
	}
	encoded, err := json.Marshal(filtered)
	if err != nil {
		return nil, err
	}
	envelope["models"] = encoded
	return json.Marshal(envelope)
}

func codexFilteredModelsETag(body []byte) string {
	sum := sha256.Sum256(body)
	return `W/"` + hex.EncodeToString(sum[:]) + `"`
}

func codexETagMatches(header, etag string) bool {
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" || candidate == etag {
			return true
		}
	}
	return false
}
