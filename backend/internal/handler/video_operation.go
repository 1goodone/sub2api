package handler

import (
	"net/http"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

func derefVideoGroupID(id *int64) int64 {
	if id == nil {
		return 0
	}
	return *id
}

// OpenAIVideoOperation recovers a persisted receipt without sending another POST.
func (h *OpenAIGatewayHandler) OpenAIVideoOperation(c *gin.Context) {
	key, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid user")
		return
	}
	task, err := h.gatewayService.GetVideoOperation(c.Request.Context(), c.Param("operation_key"), subject.UserID, key.ID)
	if err != nil {
		h.errorResponse(c, http.StatusServiceUnavailable, "video_task_storage_unavailable", "Video operation cannot be loaded")
		return
	}
	if task == nil || task.GroupID != derefVideoGroupID(key.GroupID) {
		h.errorResponse(c, http.StatusNotFound, "not_found_error", "Video operation not found")
		return
	}
	if task.TaskID != "" && task.ResponseStatus >= 200 && task.ResponseStatus < 300 {
		c.Data(http.StatusOK, "application/json", task.ResponseBody)
		return
	}
	if task.ResponseStatus >= 400 && task.ResponseStatus < 500 && task.ResponseStatus != 408 && task.ResponseStatus != 409 && task.ResponseStatus != 429 {
		c.JSON(http.StatusOK, gin.H{"status": "failed", "error": "Upstream rejected the original video submission"})
		return
	}
	c.Header("Retry-After", "5")
	h.errorResponse(c, http.StatusConflict, "idempotency_in_progress", "Video submission is awaiting confirmation; keep the original operation")
}
