package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	pgstorage "github.com/taoyao-code/iot-server/internal/storage/pg"
	"go.uber.org/zap"
)

// NetworkHandler 组网管理API处理器
type NetworkHandler struct {
	repo   *pgstorage.Repository
	logger *zap.Logger
}

// NewNetworkHandler 创建组网管理Handler
func NewNetworkHandler(repo *pgstorage.Repository, logger *zap.Logger) *NetworkHandler {
	return &NetworkHandler{
		repo:   repo,
		logger: logger,
	}
}

// ListGatewaySockets 查询网关所有插座
// @Summary 查询网关插座列表
// @Description 查询指定网关下的所有插座
// @Tags 网络管理
// @Produce json
// @Param gateway_id path string true "网关ID"
// @Success 200 {object} map[string]interface{} "成功"
// @Router /api/gateway/{gateway_id}/sockets [get]
func (h *NetworkHandler) ListGatewaySockets(c *gin.Context) {
	gatewayID := c.Param("gateway_id")
	if gatewayID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "gateway_id is required"})
		return
	}

	sockets, err := h.repo.GetGatewaySockets(c.Request.Context(), gatewayID)
	if err != nil {
		h.logger.Error("failed to query sockets", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query sockets", "detail": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"gateway_id": gatewayID,
		"count":      len(sockets),
		"sockets":    sockets,
	})
}

// GetGatewaySocket 查询单个插座
// @Summary 查询插座详情
// @Description 查询指定网关下的单个插座
// @Tags 网络管理
// @Produce json
// @Param gateway_id path string true "网关ID"
// @Param socket_no path int true "插座编号"
// @Success 200 {object} map[string]interface{} "成功"
// @Failure 404 {object} map[string]interface{} "插座不存在"
// @Router /api/gateway/{gateway_id}/sockets/{socket_no} [get]
func (h *NetworkHandler) GetGatewaySocket(c *gin.Context) {
	gatewayID := c.Param("gateway_id")
	socketNoStr := c.Param("socket_no")

	socketNo, err := strconv.Atoi(socketNoStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid socket_no"})
		return
	}

	socket, err := h.repo.GetGatewaySocket(c.Request.Context(), gatewayID, socketNo)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "socket not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"gateway_id": gatewayID,
		"socket":     socket,
	})
}

// DeleteGatewaySocket 删除插座
// @Summary 删除插座
// @Description 从网关中删除指定插座
// @Tags 网络管理
// @Param gateway_id path string true "网关ID"
// @Param socket_no path int true "插座编号"
// @Success 200 {object} map[string]interface{} "成功"
// @Failure 404 {object} map[string]interface{} "插座不存在"
// @Router /api/gateway/{gateway_id}/sockets/{socket_no} [delete]
func (h *NetworkHandler) DeleteGatewaySocket(c *gin.Context) {
	gatewayID := c.Param("gateway_id")
	socketNoStr := c.Param("socket_no")

	socketNo, err := strconv.Atoi(socketNoStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid socket_no"})
		return
	}

	// 验证插座是否存在
	_, err = h.repo.GetGatewaySocket(c.Request.Context(), gatewayID, socketNo)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "socket not found"})
		return
	}

	// 删除插座
	if err := h.repo.DeleteGatewaySocket(c.Request.Context(), gatewayID, socketNo); err != nil {
		h.logger.Error("failed to delete socket", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete socket", "detail": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "socket deleted successfully",
		"gateway_id": gatewayID,
		"socket_no":  socketNo,
	})
}
