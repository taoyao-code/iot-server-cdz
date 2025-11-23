package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/taoyao-code/iot-server/internal/coremodel"
	"go.uber.org/zap"
)

// NetworkNode 组网节点信息
type NetworkNode struct {
	SocketNo  int    `json:"socket_no" binding:"required,min=1,max=250"` // 插座编号
	SocketMAC string `json:"socket_mac" binding:"required,len=12"`       // 插座MAC（6字节hex）
}

// NetworkConfigRequest 组网配置请求
type NetworkConfigRequest struct {
	Channel int           `json:"channel" binding:"required,min=1,max=15"` // 信道
	Nodes   []NetworkNode `json:"nodes" binding:"required,min=1,max=250"`  // 插座列表
}

// ConfigureNetwork 配置组网
// @Summary 配置组网设备
// @Description 为组网版网关配置插座列表（0x0005/0x08命令）
// @Tags 第三方API - 设备管理
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param device_id path string true "网关设备物理ID"
// @Param request body NetworkConfigRequest true "组网配置"
// @Success 200 {object} StandardResponse "成功"
// @Failure 400 {object} StandardResponse "参数错误"
// @Failure 500 {object} StandardResponse "服务器错误"
// @Router /api/v1/third/devices/{device_id}/network/configure [post]
func (h *ThirdPartyHandler) ConfigureNetwork(c *gin.Context) {
	ctx := c.Request.Context()
	devicePhyID := c.Param("device_id")
	requestID := c.GetString("request_id")

	var req NetworkConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Error("invalid request body", zap.Error(err))
		c.JSON(http.StatusBadRequest, StandardResponse{
			Code:      400,
			Message:   fmt.Sprintf("invalid request: %v", err),
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	h.logger.Info("configure network requested",
		zap.String("device_phy_id", devicePhyID),
		zap.Int("channel", req.Channel),
		zap.Int("node_count", len(req.Nodes)))

	if h.driverCmd == nil {
		c.JSON(http.StatusServiceUnavailable, StandardResponse{
			Code:      503,
			Message:   "command dispatcher unavailable",
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	// 1. 验证设备存在
	_, err := h.repo.EnsureDevice(ctx, devicePhyID)
	if err != nil {
		h.logger.Error("failed to get device", zap.Error(err))
		c.JSON(http.StatusInternalServerError, StandardResponse{
			Code:      500,
			Message:   "failed to get device",
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	nodes := make([]coremodel.NetworkNodePayload, 0, len(req.Nodes))
	for _, node := range req.Nodes {
		if _, err := hexToBytes(node.SocketMAC); err != nil {
			h.logger.Error("invalid socket MAC", zap.String("mac", node.SocketMAC))
			c.JSON(http.StatusBadRequest, StandardResponse{
				Code:      400,
				Message:   fmt.Sprintf("invalid socket MAC: %s", node.SocketMAC),
				RequestID: requestID,
				Timestamp: time.Now().Unix(),
			})
			return
		}
		nodes = append(nodes, coremodel.NetworkNodePayload{
			SocketNo:  int32(node.SocketNo),
			SocketMAC: strings.ToLower(node.SocketMAC),
		})
	}

	cmd := &coremodel.CoreCommand{
		Type:      coremodel.CommandConfigureNetwork,
		CommandID: fmt.Sprintf("network:%s:%d", devicePhyID, time.Now().UnixNano()),
		DeviceID:  coremodel.DeviceID(devicePhyID),
		IssuedAt:  time.Now(),
		ConfigureNetwork: &coremodel.ConfigureNetworkPayload{
			Channel: int32(req.Channel),
			Nodes:   nodes,
		},
	}

	if err := h.driverCmd.SendCoreCommand(ctx, cmd); err != nil {
		h.logger.Error("failed to dispatch network config", zap.Error(err))
		c.JSON(http.StatusInternalServerError, StandardResponse{
			Code:      500,
			Message:   "failed to send network config",
			RequestID: requestID,
			Timestamp: time.Now().Unix(),
		})
		return
	}

	// 4. 返回成功
	c.JSON(http.StatusOK, StandardResponse{
		Code:    0,
		Message: "network configuration sent successfully",
		Data: map[string]interface{}{
			"device_id": devicePhyID,
			"channel":   req.Channel,
			"nodes":     len(req.Nodes),
		},
		RequestID: requestID,
		Timestamp: time.Now().Unix(),
	})
}

// hexToBytes 将hex字符串转为字节数组
func hexToBytes(s string) ([]byte, error) {
	if len(s)%2 != 0 {
		return nil, fmt.Errorf("odd length hex string")
	}

	result := make([]byte, len(s)/2)
	for i := 0; i < len(result); i++ {
		_, err := fmt.Sscanf(s[i*2:i*2+2], "%02x", &result[i])
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}
