package admin

import (
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

func (h *AccountHandler) codexStateServiceReady(c *gin.Context) bool {
	if h.codexTurnStateProbe == nil {
		response.InternalError(c, "Codex state service unavailable")
		return false
	}
	return true
}

func (h *AccountHandler) GetCodexStatePool(c *gin.Context) {
	if !h.codexStateServiceReady(c) {
		return
	}
	pool, err := h.codexTurnStateProbe.Pool(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, pool)
}

func (h *AccountHandler) SetCodexStateGlobalProxy(c *gin.Context) {
	if !h.codexStateServiceReady(c) {
		return
	}
	var input struct {
		ProxyID *int64 `json:"proxy_id"`
	}
	if err := c.ShouldBindJSON(&input); err != nil || input.ProxyID == nil {
		response.BadRequest(c, "proxy_id is required")
		return
	}
	config, err := h.codexTurnStateProbe.SetGlobalProxy(c.Request.Context(), *input.ProxyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, config)
}

func codexStateRequestAccountID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid account ID")
		return 0, false
	}
	return id, true
}

func (h *AccountHandler) RefreshCodexState(c *gin.Context) {
	if !h.codexStateServiceReady(c) {
		return
	}
	id, ok := codexStateRequestAccountID(c)
	if !ok {
		return
	}
	var input struct {
		Model string `json:"model"`
	}
	if err := c.ShouldBindJSON(&input); err != nil || strings.TrimSpace(input.Model) == "" {
		response.BadRequest(c, "model is required")
		return
	}
	if err := h.codexTurnStateProbe.QueueRefresh(c.Request.Context(), id, input.Model); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"queued": true})
}

func (h *AccountHandler) GetCodexStateValue(c *gin.Context) {
	if !h.codexStateServiceReady(c) {
		return
	}
	id, ok := codexStateRequestAccountID(c)
	if !ok {
		return
	}
	model := strings.TrimSpace(c.Query("model"))
	if model == "" {
		response.BadRequest(c, "model is required")
		return
	}
	state, err := h.codexTurnStateProbe.RawState(c.Request.Context(), id, model)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, gin.H{"state": state})
}

func (h *AccountHandler) ImportCodexState(c *gin.Context) {
	if !h.codexStateServiceReady(c) {
		return
	}
	id, ok := codexStateRequestAccountID(c)
	if !ok {
		return
	}
	var input struct {
		Model string `json:"model"`
		State string `json:"state"`
	}
	if c.ShouldBindJSON(&input) != nil || strings.TrimSpace(input.Model) == "" || len(input.State) > 4096 {
		response.BadRequest(c, "invalid state input")
		return
	}
	job, err := h.codexTurnStateProbe.QueueImportState(c.Request.Context(), id, strings.TrimSpace(input.Model), input.State)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, job)
}

func (h *AccountHandler) GetCodexStateImportJob(c *gin.Context) {
	if !h.codexStateServiceReady(c) {
		return
	}
	accountID, ok := codexStateRequestAccountID(c)
	if !ok {
		return
	}
	jobID := strings.TrimSpace(c.Param("job_id"))
	if jobID == "" {
		response.BadRequest(c, "job_id is required")
		return
	}
	job, ok := h.codexTurnStateProbe.ImportJob(jobID)
	if !ok || job.AccountID != accountID {
		response.NotFound(c, "import job not found")
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, job)
}
func (h *AccountHandler) GetCodexStateEvents(c *gin.Context) {
	if !h.codexStateServiceReady(c) {
		return
	}
	id, ok := codexStateRequestAccountID(c)
	if !ok {
		return
	}
	model := strings.TrimSpace(c.Query("model"))
	before, _ := strconv.ParseInt(c.Query("before"), 10, 64)
	if model == "" || before < 0 {
		response.BadRequest(c, "invalid model or cursor")
		return
	}
	items, err := h.codexTurnStateProbe.Events(c.Request.Context(), id, model, before)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, gin.H{"items": items})
}
