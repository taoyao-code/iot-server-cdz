package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/taoyao-code/iot-server/internal/protocol/bkv"
	redisstorage "github.com/taoyao-code/iot-server/internal/storage/redis"
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

	// 1. 验证设备存在
	devID, err := h.repo.EnsureDevice(ctx, devicePhyID)
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

	// 2. 构造组网命令（0x0005/0x08）
	// 根据协议文档2.2.5格式
	if h.outboundQ != nil {
		msgID := uint32(time.Now().Unix() % 0xFFFFFFFF)

		// 构造内层payload
		// 格式：[信道(1)] + [插座号(1)+MAC(6)]*N
		innerPayload := make([]byte, 1+len(req.Nodes)*7)
		innerPayload[0] = byte(req.Channel) // 信道

		pos := 1
		for _, node := range req.Nodes {
			innerPayload[pos] = byte(node.SocketNo) // 插座编号
			pos++

			// 插座MAC（6字节）
			macBytes, err := hexToBytes(node.SocketMAC)
			if err != nil || len(macBytes) != 6 {
				h.logger.Error("invalid socket MAC", zap.String("mac", node.SocketMAC))
				c.JSON(http.StatusBadRequest, StandardResponse{
					Code:      400,
					Message:   fmt.Sprintf("invalid socket MAC: %s", node.SocketMAC),
					RequestID: requestID,
					Timestamp: time.Now().Unix(),
				})
				return
			}
			copy(innerPayload[pos:], macBytes)
			pos += 6
		}

		// 添加长度字段
		payload := make([]byte, 2+len(innerPayload))
		payload[0] = byte(len(innerPayload) >> 8)
		payload[1] = byte(len(innerPayload))
		copy(payload[2:], innerPayload)

		// 构造BKV帧
		frame := bkv.Build(0x0005, msgID, devicePhyID, payload)

		h.logger.Info("network config command generated",
			zap.Int("frame_len", len(frame)),
			zap.String("frame_hex", fmt.Sprintf("%x", frame)))

		// 3. 下发组网命令
		err = h.outboundQ.Enqueue(ctx, &redisstorage.OutboundMessage{
			ID:        fmt.Sprintf("network_%d", msgID),
			DeviceID:  devID,
			PhyID:     devicePhyID,
			Command:   frame,
			Priority:  10, // 最高优先级
			MaxRetry:  3,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Timeout:   10000,
		})
		if err != nil {
			h.logger.Error("failed to enqueue network config", zap.Error(err))
			c.JSON(http.StatusInternalServerError, StandardResponse{
				Code:      500,
				Message:   "failed to send network config",
				RequestID: requestID,
				Timestamp: time.Now().Unix(),
			})
			return
		}
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
