package middleware

import (
	"context"
	"net/http"
	"strings"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const (
	bpsImageMaxBodyBytes   = 64 << 20
	bpsImageBudgetBytes    = 512 << 20
	bpsImageBodyMultiplier = 8
	bpsImageMinBodyBytes   = 1 << 20
	bpsImageMaxRequests    = 32
)

type excelBPSImageSettingsReader interface {
	GetExcelBPSImageRelaySettings(context.Context) (service.ExcelBPSImageRelaySettings, error)
}

// This budget accounts for request bodies and their processing copies, not RSS.
// Reserve before any body-reading middleware and hold until the request ends,
// including upstream streaming and scheduler waits. Never queue large bodies.
type bpsImageAdmissionBudget struct {
	mu       sync.Mutex
	bytes    int64
	requests int
}

func (b *bpsImageAdmissionBudget) acquire(weight int64) (func(), bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.requests >= bpsImageMaxRequests || weight > bpsImageBudgetBytes-b.bytes {
		return nil, false
	}
	b.bytes += weight
	b.requests++
	var once sync.Once
	return func() {
		once.Do(func() {
			b.mu.Lock()
			defer b.mu.Unlock()
			b.bytes -= weight
			b.requests--
		})
	}, true
}

// ExcelBPSImageAdmission must be shared across the gateway route aliases.
// Account selection occurs after reading JSON, so enabling image relay applies
// this guard to OpenAI/Composite Responses, Chat and Messages HTTP requests,
// including text-only requests. Disabled relay leaves existing limits intact.
func ExcelBPSImageAdmission(settings excelBPSImageSettingsReader, configuredMax int64) gin.HandlerFunc {
	budget := &bpsImageAdmissionBudget{}
	maxBody := int64(bpsImageMaxBodyBytes)
	if configuredMax > 0 && configuredMax < maxBody {
		maxBody = configuredMax
	}
	return func(c *gin.Context) {
		if settings == nil || !bpsImageAdmissionRoute(c) {
			c.Next()
			return
		}
		key, ok := GetAPIKeyFromContext(c)
		if !ok || key == nil || (key.Group != nil && key.Group.Platform != service.PlatformOpenAI && key.Group.Platform != service.PlatformComposite) {
			c.Next()
			return
		}
		relay, err := settings.GetExcelBPSImageRelaySettings(c.Request.Context())
		if err != nil {
			bpsImageAdmissionError(c, http.StatusServiceUnavailable, "basispoints_image_settings_unavailable", "Image relay settings are unavailable")
			return
		}
		if !relay.Enabled {
			c.Next()
			return
		}
		length := c.Request.ContentLength
		if length > maxBody {
			bpsImageAdmissionError(c, http.StatusRequestEntityTooLarge, "basispoints_image_body_too_large", "Request body exceeds the image relay ingress limit")
			return
		}
		readLimit := maxBody
		if length > 0 {
			readLimit = length
		}
		// Unknown/chunked and compressed inputs reserve their full possible size.
		// The decompressor's own cap is 64 MiB even with a smaller wire limit.
		accounted := readLimit
		encoding := strings.TrimSpace(c.GetHeader("Content-Encoding"))
		if encoding != "" && !strings.EqualFold(encoding, "identity") {
			accounted = bpsImageMaxBodyBytes
		}
		if accounted < bpsImageMinBodyBytes {
			accounted = bpsImageMinBodyBytes
		}
		release, acquired := budget.acquire(accounted * bpsImageBodyMultiplier)
		if !acquired {
			bpsImageAdmissionError(c, http.StatusServiceUnavailable, "basispoints_image_request_busy", "Image relay request capacity is busy; retry later")
			return
		}
		defer release()
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, readLimit)
		c.Next()
	}
}

func bpsImageAdmissionRoute(c *gin.Context) bool {
	if c.Request.Method != http.MethodPost {
		return false
	}
	switch c.FullPath() {
	case "/responses", "/responses/*subpath", "/v1/responses", "/v1/responses/*subpath",
		"/backend-api/codex/responses", "/backend-api/codex/responses/*subpath",
		"/chat/completions", "/v1/chat/completions", "/v1/messages":
		return true
	}
	return false
}

func bpsImageAdmissionError(c *gin.Context, status int, code, message string) {
	errorType := "server_error"
	if status == http.StatusRequestEntityTooLarge {
		errorType = "invalid_request_error"
	}
	if status == http.StatusServiceUnavailable {
		c.Header("Retry-After", "1")
	}
	c.AbortWithStatusJSON(status, gin.H{"error": gin.H{"type": errorType, "code": code, "message": message}})
}
