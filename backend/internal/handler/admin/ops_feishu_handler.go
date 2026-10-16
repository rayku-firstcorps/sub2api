package admin

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *OpsHandler) requireFeishuOps(c *gin.Context) bool {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return false
	}
	if err := h.opsService.RequireMonitoringEnabled(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return false
	}
	return true
}

func (h *OpsHandler) GetFeishuNotificationConfig(c *gin.Context) {
	if !h.requireFeishuOps(c) {
		return
	}
	cfg, err := h.opsService.GetFeishuNotificationConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}

func (h *OpsHandler) UpdateFeishuNotificationConfig(c *gin.Context) {
	if !h.requireFeishuOps(c) {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8192)
	var req service.OpsFeishuConfigUpdate
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	cfg, err := h.opsService.UpdateFeishuNotificationConfig(c.Request.Context(), &req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}

func (h *OpsHandler) TestFeishuNotification(c *gin.Context) {
	if !h.requireFeishuOps(c) {
		return
	}
	if err := h.opsService.TestFeishuNotification(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"success": true})
}
