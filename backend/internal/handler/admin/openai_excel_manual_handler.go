package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

func (h *OpenAIOAuthReauthHandler) BeginExcelManual(c *gin.Context) {
	id, ok := parseReauthAccountID(c)
	if !ok {
		return
	}
	auth, err := h.service.BeginExcelManualAuthorization(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, auth)
}

func (h *OpenAIOAuthReauthHandler) CompleteExcelManual(c *gin.Context) {
	id, ok := parseReauthAccountID(c)
	if !ok {
		return
	}
	var input struct {
		SessionID   string `json:"session_id" binding:"required"`
		CallbackURL string `json:"callback_url" binding:"required"`
	}
	if c.ShouldBindJSON(&input) != nil {
		response.BadRequest(c, "Session and callback URL are required")
		return
	}
	if err := h.service.CompleteExcelManualAuthorization(c.Request.Context(), id, input.SessionID, input.CallbackURL); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"saved": true})
}
